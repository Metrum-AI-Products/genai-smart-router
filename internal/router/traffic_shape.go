package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	trafficShapeDecisionDisabled = "disabled"
	trafficShapeDecisionAdmitted = "admitted"
	trafficShapeDecisionQueued   = "queued"
	trafficShapeDecisionRejected = "rejected"

	trafficShapeScopeCaller        = "caller"
	trafficShapeScopeServerDefault = "server_default"

	trafficShapeBucketRequestStart      = "request_start"
	trafficShapeBucketInputTokens       = "input_tokens"
	trafficShapeBucketOutputReservation = "output_reservation"
	trafficShapeBucketTotalReserved     = "total_reserved"
)

type trafficShapeManager struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets map[string]*trafficShapeBucket
	queues  map[string]int
}

type trafficShapeBucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

type trafficShapeRequest struct {
	CallerID                 string
	ModelGroup               string
	Config                   TrafficShapeConfig
	Scope                    string
	Phase                    string
	EstimatedInputTokens     int
	ReservedOutputTokens     int
	TotalReservedTokens      int
	IncludeRequestStart      bool
	IncludeTokenReservations bool
}

type trafficShapeResult struct {
	Applied              bool
	Decision             string
	Scope                string
	Bucket               string
	RetryAfterMS         int64
	QueueWaitMS          int64
	EstimatedInputTokens int
	ReservedOutputTokens int
	TotalReservedTokens  int
	Events               []trafficShapeEventLogRecord
}

type trafficShapeBucketSpec struct {
	name   string
	suffix string
	rate   float64
	burst  int
	cost   int
}

type trafficShapeBucketEval struct {
	spec         trafficShapeBucketSpec
	decision     string
	retryAfterMS int64
	impossible   bool
}

func newTrafficShapeManager() *trafficShapeManager {
	return &trafficShapeManager{
		now:     time.Now,
		buckets: map[string]*trafficShapeBucket{},
		queues:  map[string]int{},
	}
}

func resolveTrafficShapeConfig(server ServerTrafficShapeConfig, caller CallerConfig) (TrafficShapeConfig, string, bool) {
	if trafficShapeConfigSet(caller.TrafficShape) {
		if trafficShapeExplicitlyDisabled(caller.TrafficShape) {
			return TrafficShapeConfig{}, trafficShapeScopeCaller, false
		}
		return caller.TrafficShape, trafficShapeScopeCaller, trafficShapeHasActiveBucket(caller.TrafficShape)
	}
	if !server.Enabled {
		return TrafficShapeConfig{}, trafficShapeScopeServerDefault, false
	}
	cfg := server.DefaultCaller
	if trafficShapeExplicitlyDisabled(cfg) {
		return TrafficShapeConfig{}, trafficShapeScopeServerDefault, false
	}
	return cfg, trafficShapeScopeServerDefault, trafficShapeHasActiveBucket(cfg)
}

func trafficShapeConfigSet(cfg TrafficShapeConfig) bool {
	return cfg.Enabled != nil || trafficShapeHasAnyBucketField(cfg) || cfg.Queue.Enabled || cfg.Queue.MaxWaitMS != 0 || cfg.Queue.MaxDepth != 0
}

func trafficShapeExplicitlyDisabled(cfg TrafficShapeConfig) bool {
	return cfg.Enabled != nil && !*cfg.Enabled
}

func trafficShapeHasAnyBucketField(cfg TrafficShapeConfig) bool {
	return cfg.RequestStartPerSec != 0 || cfg.RequestBurst != 0 ||
		cfg.InputTokensPerSec != 0 || cfg.InputTokenBurst != 0 ||
		cfg.OutputReservationTokensPerSec != 0 || cfg.OutputReservationTokenBurst != 0 ||
		cfg.TotalReservedTokensPerSec != 0 || cfg.TotalReservedTokenBurst != 0
}

func trafficShapeHasActiveBucket(cfg TrafficShapeConfig) bool {
	for _, spec := range trafficShapeSpecs(cfg, 1, 1, 1, true, true) {
		if spec.rate > 0 && spec.burst > 0 {
			return true
		}
	}
	return false
}

func validateServerTrafficShape(cfg ServerTrafficShapeConfig) error {
	if err := validateTrafficShapeConfig("server traffic_shape.default_caller", cfg.DefaultCaller, cfg.Enabled); err != nil {
		return err
	}
	if !cfg.Enabled && trafficShapeConfigSet(cfg.DefaultCaller) && !trafficShapeExplicitlyDisabled(cfg.DefaultCaller) {
		if err := validateTrafficShapeConfig("server traffic_shape.default_caller", cfg.DefaultCaller, false); err != nil {
			return err
		}
	}
	return nil
}

func validateTrafficShapeConfig(label string, cfg TrafficShapeConfig, requireActive bool) error {
	checkPair := func(rate float64, burst int, rateName, burstName string) error {
		if rate < 0 {
			return fmt.Errorf("%s %s cannot be negative", label, rateName)
		}
		if burst < 0 {
			return fmt.Errorf("%s %s cannot be negative", label, burstName)
		}
		if (rate == 0) != (burst == 0) {
			return fmt.Errorf("%s %s and %s must both be set or both be zero", label, rateName, burstName)
		}
		return nil
	}
	if err := checkPair(cfg.RequestStartPerSec, cfg.RequestBurst, "request_start_per_sec", "request_burst"); err != nil {
		return err
	}
	if err := checkPair(cfg.InputTokensPerSec, cfg.InputTokenBurst, "input_tokens_per_sec", "input_token_burst"); err != nil {
		return err
	}
	if err := checkPair(cfg.OutputReservationTokensPerSec, cfg.OutputReservationTokenBurst, "output_reservation_tokens_per_sec", "output_reservation_token_burst"); err != nil {
		return err
	}
	if err := checkPair(cfg.TotalReservedTokensPerSec, cfg.TotalReservedTokenBurst, "total_reserved_tokens_per_sec", "total_reserved_token_burst"); err != nil {
		return err
	}
	if cfg.Queue.MaxWaitMS < 0 {
		return fmt.Errorf("%s queue max_wait_ms cannot be negative", label)
	}
	if cfg.Queue.MaxDepth < 0 {
		return fmt.Errorf("%s queue max_depth cannot be negative", label)
	}
	if cfg.Queue.Enabled && (cfg.Queue.MaxWaitMS <= 0 || cfg.Queue.MaxDepth <= 0) {
		return fmt.Errorf("%s queue enabled requires positive max_wait_ms and max_depth", label)
	}
	if requireActive && !trafficShapeHasActiveBucket(cfg) {
		return fmt.Errorf("%s enabled requires at least one active bucket", label)
	}
	return nil
}

func (m *trafficShapeManager) Admit(ctx context.Context, req trafficShapeRequest) trafficShapeResult {
	cfg := req.Config
	result := trafficShapeResult{
		Decision:             trafficShapeDecisionDisabled,
		Scope:                req.Scope,
		EstimatedInputTokens: req.EstimatedInputTokens,
		ReservedOutputTokens: req.ReservedOutputTokens,
		TotalReservedTokens:  req.TotalReservedTokens,
	}
	specs := trafficShapeSpecs(cfg, req.EstimatedInputTokens, req.ReservedOutputTokens, req.TotalReservedTokens, req.IncludeRequestStart, req.IncludeTokenReservations)
	if len(specs) == 0 {
		return result
	}
	if m == nil {
		m = newTrafficShapeManager()
	}
	decision, events, retryAfterMS, bucket, impossible := m.tryConsume(req, specs)
	if decision == trafficShapeDecisionAdmitted {
		result.Applied = true
		result.Decision = trafficShapeDecisionAdmitted
		result.Bucket = bucket
		result.Events = events
		return result
	} else if !cfg.Queue.Enabled || impossible {
		result.Applied = true
		result.Decision = trafficShapeDecisionRejected
		result.Bucket = bucket
		result.RetryAfterMS = retryAfterMS
		result.Events = events
		return result
	}
	queueKey := trafficShapeQueueKey(req)
	if !m.enterQueue(queueKey, cfg.Queue.MaxDepth) {
		result.Applied = true
		result.Decision = trafficShapeDecisionRejected
		result.Bucket = bucket
		result.RetryAfterMS = retryAfterMS
		result.Events = events
		return result
	}
	defer m.leaveQueue(queueKey)

	start := m.currentTime()
	deadline := start.Add(time.Duration(cfg.Queue.MaxWaitMS) * time.Millisecond)
	for {
		decision, events, retryAfterMS, rejectedBucket, impossible := m.tryConsume(req, specs)
		if decision == trafficShapeDecisionAdmitted {
			waitMS := durationMillis(m.currentTime().Sub(start))
			result.Applied = true
			result.Decision = trafficShapeDecisionQueued
			result.Bucket = rejectedBucket
			result.QueueWaitMS = waitMS
			result.Events = eventsWithQueueWait(events, waitMS, trafficShapeDecisionQueued)
			return result
		}
		result.Bucket = rejectedBucket
		result.RetryAfterMS = retryAfterMS
		result.Events = events
		now := m.currentTime()
		if impossible || !deadline.After(now) {
			result.Applied = true
			result.Decision = trafficShapeDecisionRejected
			result.QueueWaitMS = durationMillis(now.Sub(start))
			result.Events = eventsWithQueueWait(events, result.QueueWaitMS, trafficShapeDecisionRejected)
			return result
		}
		wait := time.Duration(retryAfterMS) * time.Millisecond
		if wait <= 0 {
			wait = 10 * time.Millisecond
		}
		if max := deadline.Sub(now); wait > max {
			wait = max
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			result.Applied = true
			result.Decision = trafficShapeDecisionRejected
			result.QueueWaitMS = durationMillis(m.currentTime().Sub(start))
			result.Events = eventsWithQueueWait(events, result.QueueWaitMS, trafficShapeDecisionRejected)
			return result
		case <-timer.C:
		}
	}
}

func (m *trafficShapeManager) tryConsume(req trafficShapeRequest, specs []trafficShapeBucketSpec) (string, []trafficShapeEventLogRecord, int64, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	evals := make([]trafficShapeBucketEval, 0, len(specs))
	var rejected *trafficShapeBucketEval
	var impossible bool
	for _, spec := range specs {
		key := trafficShapeBucketKey(req, spec.name)
		b := m.bucket(key, spec, now)
		b.refill(now)
		eval := trafficShapeBucketEval{spec: spec, decision: trafficShapeDecisionAdmitted}
		if spec.cost > int(math.Floor(b.tokens)) {
			eval.decision = trafficShapeDecisionRejected
			eval.retryAfterMS = b.retryAfterMS(spec.cost)
			eval.impossible = float64(spec.cost) > b.burst
			if eval.impossible {
				impossible = true
			}
			if rejected == nil || eval.retryAfterMS > rejected.retryAfterMS || (eval.impossible && !rejected.impossible) {
				copied := eval
				rejected = &copied
			}
		}
		evals = append(evals, eval)
	}
	if rejected != nil {
		events := trafficShapeEvents(req, evals, trafficShapeDecisionRejected, rejected.spec.name, rejected.retryAfterMS, 0)
		return trafficShapeDecisionRejected, events, rejected.retryAfterMS, trafficShapePublicBucket(req.Scope, rejected.spec), impossible
	}
	for _, spec := range specs {
		m.buckets[trafficShapeBucketKey(req, spec.name)].tokens -= float64(spec.cost)
	}
	events := trafficShapeEvents(req, evals, trafficShapeDecisionAdmitted, "", 0, 0)
	return trafficShapeDecisionAdmitted, events, 0, "", false
}

func (m *trafficShapeManager) bucket(key string, spec trafficShapeBucketSpec, now time.Time) *trafficShapeBucket {
	b := m.buckets[key]
	if b == nil || b.rate != spec.rate || b.burst != float64(spec.burst) {
		b = &trafficShapeBucket{rate: spec.rate, burst: float64(spec.burst), tokens: float64(spec.burst), last: now}
		m.buckets[key] = b
	}
	return b
}

func (b *trafficShapeBucket) refill(now time.Time) {
	if b == nil {
		return
	}
	if b.last.IsZero() {
		b.last = now
		return
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens += elapsed * b.rate
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	b.last = now
}

func (b *trafficShapeBucket) retryAfterMS(cost int) int64 {
	if b == nil || b.rate <= 0 || float64(cost) > b.burst {
		return 0
	}
	missing := float64(cost) - b.tokens
	if missing <= 0 {
		return 0
	}
	ms := int64(math.Ceil(missing / b.rate * 1000))
	if ms < 1 {
		ms = 1
	}
	return ms
}

func (m *trafficShapeManager) enterQueue(key string, maxDepth int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if maxDepth <= 0 || m.queues[key] >= maxDepth {
		return false
	}
	m.queues[key]++
	return true
}

func (m *trafficShapeManager) leaveQueue(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queues[key] <= 1 {
		delete(m.queues, key)
		return
	}
	m.queues[key]--
}

func (m *trafficShapeManager) currentTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now()
	}
	return m.now()
}

func (m *trafficShapeManager) QueueDepths() []trafficShapeQueueDepth {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]trafficShapeQueueDepth, 0, len(m.queues))
	for key, depth := range m.queues {
		scope, callerID := parseTrafficShapeQueueKey(key)
		out = append(out, trafficShapeQueueDepth{Scope: scope, CallerID: callerID, Depth: depth})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].CallerID < out[j].CallerID
	})
	return out
}

type trafficShapeQueueDepth struct {
	Scope    string
	CallerID string
	Depth    int
}

func trafficShapeSpecs(cfg TrafficShapeConfig, inputTokens, outputTokens, totalTokens int, includeRequestStart, includeTokenReservations bool) []trafficShapeBucketSpec {
	var specs []trafficShapeBucketSpec
	add := func(name, suffix string, rate float64, burst, cost int) {
		if rate <= 0 || burst <= 0 || cost <= 0 {
			return
		}
		specs = append(specs, trafficShapeBucketSpec{name: name, suffix: suffix, rate: rate, burst: burst, cost: cost})
	}
	if includeRequestStart {
		add(trafficShapeBucketRequestStart, "request_start_per_sec", cfg.RequestStartPerSec, cfg.RequestBurst, 1)
	}
	if includeTokenReservations {
		add(trafficShapeBucketInputTokens, "input_tokens_per_sec", cfg.InputTokensPerSec, cfg.InputTokenBurst, inputTokens)
		add(trafficShapeBucketOutputReservation, "output_reservation_tokens_per_sec", cfg.OutputReservationTokensPerSec, cfg.OutputReservationTokenBurst, outputTokens)
		add(trafficShapeBucketTotalReserved, "total_reserved_tokens_per_sec", cfg.TotalReservedTokensPerSec, cfg.TotalReservedTokenBurst, totalTokens)
	}
	return specs
}

func trafficShapeEvents(req trafficShapeRequest, evals []trafficShapeBucketEval, overallDecision, rejectedBucket string, retryAfterMS, queueWaitMS int64) []trafficShapeEventLogRecord {
	events := make([]trafficShapeEventLogRecord, 0, len(evals))
	for i, eval := range evals {
		decision := eval.decision
		retry := eval.retryAfterMS
		if decision == "" {
			decision = overallDecision
		}
		if decision != trafficShapeDecisionRejected {
			retry = 0
		}
		if eval.spec.name == rejectedBucket {
			retry = retryAfterMS
		}
		events = append(events, trafficShapeEventLogRecord{
			Seq:                  i + 1,
			Scope:                req.Scope,
			Bucket:               eval.spec.name,
			Decision:             decision,
			Cost:                 eval.spec.cost,
			RetryAfterMS:         retry,
			QueueWaitMS:          queueWaitMS,
			EstimatedInputTokens: req.EstimatedInputTokens,
			ReservedOutputTokens: req.ReservedOutputTokens,
			TotalReservedTokens:  req.TotalReservedTokens,
		})
	}
	return events
}

func eventsWithQueueWait(events []trafficShapeEventLogRecord, queueWaitMS int64, decision string) []trafficShapeEventLogRecord {
	out := append([]trafficShapeEventLogRecord(nil), events...)
	for i := range out {
		out[i].QueueWaitMS = queueWaitMS
		if decision == trafficShapeDecisionQueued && out[i].Decision == trafficShapeDecisionAdmitted {
			out[i].Decision = trafficShapeDecisionQueued
		}
	}
	return out
}

func trafficShapeBucketKey(req trafficShapeRequest, bucket string) string {
	return req.Scope + "\x00" + req.CallerID + "\x00" + bucket
}

func trafficShapeQueueKey(req trafficShapeRequest) string {
	return req.Scope + "\x00" + req.CallerID
}

func parseTrafficShapeQueueKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return "unknown", "unknown"
	}
	return parts[0], parts[1]
}

func trafficShapePublicBucket(scope string, spec trafficShapeBucketSpec) string {
	if spec.name == "" {
		return ""
	}
	if spec.suffix == "" {
		return scope + "." + spec.name
	}
	return scope + "." + spec.suffix
}

func trafficShapeOutputReservation(req *IRRequest, dialect string) int {
	total := reservationEstimate(req, dialect)
	input := estimateTokens(req)
	output := total - input
	if output < 0 {
		return 0
	}
	return output
}
