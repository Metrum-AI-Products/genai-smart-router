package router

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTrafficShapeConfigValidationAndPrecedence(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
	_, _, enabled := resolveTrafficShapeConfig(cfg.Server.TrafficShape, cfg.Callers[0])
	if enabled {
		t.Fatal("traffic shaping should be disabled by default")
	}

	on := true
	cfg.Callers[0].TrafficShape = TrafficShapeConfig{
		Enabled:            &on,
		RequestStartPerSec: 2,
		RequestBurst:       4,
		Queue:              TrafficShapeQueueConfig{Enabled: true, MaxWaitMS: 50, MaxDepth: 2},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid caller traffic shape rejected: %v", err)
	}

	bad := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	bad.Callers[0].TrafficShape = TrafficShapeConfig{Enabled: &on, InputTokensPerSec: -1, InputTokenBurst: 10}
	bad.setDefaults()
	if err := bad.Validate(); err == nil {
		t.Fatal("negative traffic shaping rate should fail validation")
	}

	badQueue := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	badQueue.Callers[0].TrafficShape = TrafficShapeConfig{
		Enabled:            &on,
		RequestStartPerSec: 1,
		RequestBurst:       1,
		Queue:              TrafficShapeQueueConfig{Enabled: true},
	}
	badQueue.setDefaults()
	if err := badQueue.Validate(); err == nil {
		t.Fatal("enabled queue without wait/depth should fail validation")
	}

	off := false
	cfg.Server.TrafficShape = ServerTrafficShapeConfig{
		Enabled: true,
		DefaultCaller: TrafficShapeConfig{
			RequestStartPerSec: 1,
			RequestBurst:       1,
		},
	}
	cfg.Callers[0].TrafficShape = TrafficShapeConfig{Enabled: &off}
	_, scope, enabled := resolveTrafficShapeConfig(cfg.Server.TrafficShape, cfg.Callers[0])
	if enabled || scope != trafficShapeScopeCaller {
		t.Fatalf("caller disabled override should win, scope=%s enabled=%v", scope, enabled)
	}
}

func TestTrafficShapeManagerConcurrentDoesNotOverAdmit(t *testing.T) {
	m := newTrafficShapeManager()
	cfg := TrafficShapeConfig{RequestStartPerSec: 0.001, RequestBurst: 5}
	var wg sync.WaitGroup
	admitted := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := m.Admit(context.Background(), trafficShapeRequest{
				CallerID:            "alice",
				Config:              cfg,
				Scope:               trafficShapeScopeCaller,
				IncludeRequestStart: true,
			})
			admitted <- res.Decision == trafficShapeDecisionAdmitted
		}()
	}
	wg.Wait()
	close(admitted)
	var count int
	for ok := range admitted {
		if ok {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("admitted=%d, want burst capacity 5", count)
	}
}

func TestTrafficShapeManagerQueueWaitsForRefill(t *testing.T) {
	m := newTrafficShapeManager()
	cfg := TrafficShapeConfig{
		RequestStartPerSec: 1000,
		RequestBurst:       1,
		Queue:              TrafficShapeQueueConfig{Enabled: true, MaxWaitMS: 100, MaxDepth: 1},
	}
	req := trafficShapeRequest{CallerID: "alice", Config: cfg, Scope: trafficShapeScopeCaller, IncludeRequestStart: true}
	first := m.Admit(context.Background(), req)
	if first.Decision != trafficShapeDecisionAdmitted {
		t.Fatalf("first decision=%s", first.Decision)
	}
	second := m.Admit(context.Background(), req)
	if second.Decision != trafficShapeDecisionQueued {
		t.Fatalf("second decision=%s retry=%d events=%#v", second.Decision, second.RetryAfterMS, second.Events)
	}
	if second.QueueWaitMS <= 0 {
		t.Fatalf("queue wait not recorded: %#v", second)
	}
	if len(second.Events) != 1 || second.Events[0].Decision != trafficShapeDecisionQueued {
		t.Fatalf("queued event missing: %#v", second.Events)
	}
}

func TestTrafficShapeManagerQueueHonorsContextCancel(t *testing.T) {
	m := newTrafficShapeManager()
	cfg := TrafficShapeConfig{
		RequestStartPerSec: 0.001,
		RequestBurst:       1,
		Queue:              TrafficShapeQueueConfig{Enabled: true, MaxWaitMS: 5000, MaxDepth: 1},
	}
	req := trafficShapeRequest{CallerID: "alice", Config: cfg, Scope: trafficShapeScopeCaller, IncludeRequestStart: true}
	if first := m.Admit(context.Background(), req); first.Decision != trafficShapeDecisionAdmitted {
		t.Fatalf("first decision=%s", first.Decision)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	second := m.Admit(ctx, req)
	if second.Decision != trafficShapeDecisionRejected {
		t.Fatalf("canceled queued admission decision=%s", second.Decision)
	}
}

func TestTrafficShapeManagerRejectsImpossibleReservationWithoutQueueWait(t *testing.T) {
	m := newTrafficShapeManager()
	cfg := TrafficShapeConfig{
		TotalReservedTokensPerSec: 1,
		TotalReservedTokenBurst:   10,
		Queue:                     TrafficShapeQueueConfig{Enabled: true, MaxWaitMS: 250, MaxDepth: 1},
	}
	req := trafficShapeRequest{
		CallerID:                 "alice",
		Config:                   cfg,
		Scope:                    trafficShapeScopeCaller,
		TotalReservedTokens:      50,
		IncludeTokenReservations: true,
	}
	start := time.Now()
	res := m.Admit(context.Background(), req)
	elapsed := time.Since(start)
	if res.Decision != trafficShapeDecisionRejected {
		t.Fatalf("decision=%s, want rejected: %#v", res.Decision, res)
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("impossible reservation waited %s before rejection", elapsed)
	}
	if res.QueueWaitMS != 0 {
		t.Fatalf("queue wait=%d, want 0", res.QueueWaitMS)
	}
	if len(m.QueueDepths()) != 0 {
		t.Fatalf("impossible reservation entered queue: %#v", m.QueueDepths())
	}
}

func TestTrafficShapeManagerPreservesEveryRejectedBucketEvent(t *testing.T) {
	m := newTrafficShapeManager()
	cfg := TrafficShapeConfig{
		InputTokensPerSec:             1,
		InputTokenBurst:               5,
		OutputReservationTokensPerSec: 1,
		OutputReservationTokenBurst:   5,
		TotalReservedTokensPerSec:     1,
		TotalReservedTokenBurst:       8,
	}
	req := trafficShapeRequest{
		CallerID:                 "alice",
		Config:                   cfg,
		Scope:                    trafficShapeScopeCaller,
		EstimatedInputTokens:     10,
		ReservedOutputTokens:     10,
		TotalReservedTokens:      20,
		IncludeTokenReservations: true,
	}
	res := m.Admit(context.Background(), req)
	if res.Decision != trafficShapeDecisionRejected {
		t.Fatalf("decision=%s, want rejected: %#v", res.Decision, res)
	}
	rejected := map[string]bool{}
	for _, event := range res.Events {
		if event.Decision == trafficShapeDecisionRejected {
			rejected[event.Bucket] = true
		}
	}
	for _, bucket := range []string{trafficShapeBucketInputTokens, trafficShapeBucketOutputReservation, trafficShapeBucketTotalReserved} {
		if !rejected[bucket] {
			t.Fatalf("bucket %s was not recorded as rejected; events=%#v", bucket, res.Events)
		}
	}
}
