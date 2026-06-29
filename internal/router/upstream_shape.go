package router

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	shapeScopeProvider      = "provider"
	shapeScopeProviderModel = "provider_model"
	shapeScopeTarget        = "target"

	shapeBucketRequestStart  = "request_start"
	shapeBucketInputTokens   = "input_tokens"
	shapeBucketTotalReserved = "total_reserved"
	shapeBucketBackoff       = "adaptive_backoff"

	shapeDecisionAdmitted        = "admitted"
	shapeDecisionSkipped         = "skipped"
	shapeDecisionRejected        = "rejected"
	shapeDecisionCooldownStarted = "cooldown_started"
)

type upstreamShapeManager struct {
	mu       sync.Mutex
	buckets  map[string]*trafficTokenBucket
	backoffs map[string]*trafficBackoffState
	now      func() time.Time
}

type trafficTokenBucket struct {
	tokens  float64
	updated time.Time
}

type trafficBackoffState struct {
	until       time.Time
	failures    int
	reason      string
	lastUpdated time.Time
}

type shapeReservationInput struct {
	EstimatedInputTokens int
	ReservedOutputTokens int
	TotalReservedTokens  int
}

type shapeScope struct {
	Scope    string
	Key      string
	Config   TrafficShapeConfig
	Provider string
	ModelRef string
	Model    string
	Dialect  string
}

type shapeAdmissionResult struct {
	OK         bool
	Reason     string
	Scope      string
	Bucket     string
	RetryAfter time.Duration
	Events     []upstreamShapeEventLogRecord
}

type upstreamCapacityThrottledError struct {
	RetryAfter  time.Duration
	TargetCount int
}

func (e upstreamCapacityThrottledError) Error() string {
	return "upstream capacity throttled"
}

func newUpstreamShapeManager() *upstreamShapeManager {
	return &upstreamShapeManager{
		buckets:  map[string]*trafficTokenBucket{},
		backoffs: map[string]*trafficBackoffState{},
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (m *upstreamShapeManager) Check(scopes []shapeScope, in shapeReservationInput) shapeAdmissionResult {
	return m.admit(scopes, in, false)
}

func (m *upstreamShapeManager) Admit(scopes []shapeScope, in shapeReservationInput) shapeAdmissionResult {
	return m.admit(scopes, in, true)
}

func (m *upstreamShapeManager) admit(scopes []shapeScope, in shapeReservationInput, consume bool) shapeAdmissionResult {
	if m == nil {
		return shapeAdmissionResult{OK: true}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	var planned []shapeBucketDebit
	for _, scope := range scopes {
		if !trafficShapeEnabled(scope.Config) {
			continue
		}
		if state := m.backoffs[scope.Key]; state != nil && now.Before(state.until) {
			retry := state.until.Sub(now)
			return shapeAdmissionResult{
				OK:         false,
				Reason:     state.reason,
				Scope:      scope.Scope,
				Bucket:     shapeBucketBackoff,
				RetryAfter: retry,
				Events:     []upstreamShapeEventLogRecord{shapeEvent(scope, shapeBucketBackoff, shapeDecisionSkipped, retry, in, state.reason)},
			}
		}
		if scope.Config.RequestStartPerSec > 0 {
			debit, ok, retry := m.planDebit(now, scope, shapeBucketRequestStart, scope.Config.RequestStartPerSec, scope.Config.RequestBurst, 1)
			if !ok {
				return shapeAdmissionResult{
					OK:         false,
					Reason:     shapeReason(scope.Scope, shapeBucketRequestStart),
					Scope:      scope.Scope,
					Bucket:     shapeBucketRequestStart,
					RetryAfter: retry,
					Events:     []upstreamShapeEventLogRecord{shapeEvent(scope, shapeBucketRequestStart, shapeDecisionSkipped, retry, in, shapeReason(scope.Scope, shapeBucketRequestStart))},
				}
			}
			planned = append(planned, debit)
		}
		if scope.Config.InputTokensPerSec > 0 && in.EstimatedInputTokens > 0 {
			debit, ok, retry := m.planDebit(now, scope, shapeBucketInputTokens, scope.Config.InputTokensPerSec, scope.Config.InputTokenBurst, float64(in.EstimatedInputTokens))
			if !ok {
				return shapeAdmissionResult{
					OK:         false,
					Reason:     shapeReason(scope.Scope, shapeBucketInputTokens),
					Scope:      scope.Scope,
					Bucket:     shapeBucketInputTokens,
					RetryAfter: retry,
					Events:     []upstreamShapeEventLogRecord{shapeEvent(scope, shapeBucketInputTokens, shapeDecisionSkipped, retry, in, shapeReason(scope.Scope, shapeBucketInputTokens))},
				}
			}
			planned = append(planned, debit)
		}
		if scope.Config.TotalReservedTokensPerSec > 0 && in.TotalReservedTokens > 0 {
			debit, ok, retry := m.planDebit(now, scope, shapeBucketTotalReserved, scope.Config.TotalReservedTokensPerSec, scope.Config.TotalReservedTokenBurst, float64(in.TotalReservedTokens))
			if !ok {
				return shapeAdmissionResult{
					OK:         false,
					Reason:     shapeReason(scope.Scope, shapeBucketTotalReserved),
					Scope:      scope.Scope,
					Bucket:     shapeBucketTotalReserved,
					RetryAfter: retry,
					Events:     []upstreamShapeEventLogRecord{shapeEvent(scope, shapeBucketTotalReserved, shapeDecisionSkipped, retry, in, shapeReason(scope.Scope, shapeBucketTotalReserved))},
				}
			}
			planned = append(planned, debit)
		}
	}
	events := make([]upstreamShapeEventLogRecord, 0, len(planned))
	if consume {
		for _, debit := range planned {
			b := m.bucket(debit.key, now, debit.burst)
			b.tokens = debit.after
			b.updated = now
			events = append(events, shapeEvent(debit.scope, debit.bucket, shapeDecisionAdmitted, 0, in, ""))
		}
	}
	return shapeAdmissionResult{OK: true, Events: events}
}

type shapeBucketDebit struct {
	key    string
	scope  shapeScope
	bucket string
	burst  int
	after  float64
}

func (m *upstreamShapeManager) planDebit(now time.Time, scope shapeScope, bucketName string, rate float64, burst int, cost float64) (shapeBucketDebit, bool, time.Duration) {
	key := scope.Key + "|" + bucketName
	b := m.bucket(key, now, burst)
	refillBucket(b, now, rate, burst)
	if cost <= b.tokens {
		return shapeBucketDebit{key: key, scope: scope, bucket: bucketName, burst: burst, after: b.tokens - cost}, true, 0
	}
	if rate <= 0 {
		return shapeBucketDebit{}, false, time.Second
	}
	deficit := cost - b.tokens
	return shapeBucketDebit{}, false, time.Duration(math.Ceil(deficit/rate*1000)) * time.Millisecond
}

func (m *upstreamShapeManager) bucket(key string, now time.Time, burst int) *trafficTokenBucket {
	b := m.buckets[key]
	if b == nil {
		b = &trafficTokenBucket{tokens: float64(burst), updated: now}
		m.buckets[key] = b
	}
	return b
}

func refillBucket(b *trafficTokenBucket, now time.Time, rate float64, burst int) {
	if b == nil || rate <= 0 || burst <= 0 {
		return
	}
	if b.updated.IsZero() {
		b.updated = now
		b.tokens = float64(burst)
		return
	}
	elapsed := now.Sub(b.updated).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(float64(burst), b.tokens+elapsed*rate)
		b.updated = now
	}
}

func (m *upstreamShapeManager) StartBackoff(scopes []shapeScope, class string, retryAfter time.Duration, in shapeReservationInput) []upstreamShapeEventLogRecord {
	if m == nil {
		return nil
	}
	reason := ""
	switch class {
	case "upstream_rate_limited":
		reason = "adaptive-backoff-provider-429"
	case "upstream_quota_exhausted":
		reason = "adaptive-backoff-provider-quota"
	default:
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	var events []upstreamShapeEventLogRecord
	for _, scope := range scopes {
		cfg := backoffConfigForClass(scope.Config, class)
		if !trafficShapeEnabled(scope.Config) || !trafficBackoffEnabled(cfg) {
			continue
		}
		duration := boundedBackoffDuration(cfg, retryAfter)
		if duration <= 0 {
			continue
		}
		state := m.backoffs[scope.Key]
		if state == nil {
			state = &trafficBackoffState{}
			m.backoffs[scope.Key] = state
		}
		if state.failures > 0 && cfg.Multiplier > 1 {
			prior := state.until.Sub(now)
			if prior < duration {
				prior = duration
			}
			duration = time.Duration(float64(prior) * cfg.Multiplier)
			if max := time.Duration(defaultBackoffMaxMS(cfg)) * time.Millisecond; max > 0 && duration > max {
				duration = max
			}
		}
		state.failures++
		state.until = now.Add(duration)
		state.reason = reason
		state.lastUpdated = now
		events = append(events, shapeEvent(scope, shapeBucketBackoff, shapeDecisionCooldownStarted, duration, in, reason))
	}
	return events
}

func backoffConfigForClass(cfg TrafficShapeConfig, class string) TrafficBackoffConfig {
	if class == "upstream_quota_exhausted" {
		return cfg.UpstreamQuotaBackoff
	}
	return cfg.Upstream429Backoff
}

func boundedBackoffDuration(cfg TrafficBackoffConfig, retryAfter time.Duration) time.Duration {
	min := time.Duration(defaultBackoffMinMS(cfg)) * time.Millisecond
	max := time.Duration(defaultBackoffMaxMS(cfg)) * time.Millisecond
	if min <= 0 {
		min = time.Second
	}
	if max <= 0 {
		max = min
	}
	duration := min
	honorRetryAfter := cfg.HonorRetryAfter == nil || *cfg.HonorRetryAfter
	if honorRetryAfter && retryAfter > 0 {
		if retryAfter > max {
			duration = max
		} else if retryAfter > duration {
			duration = retryAfter
		}
	}
	if duration > max {
		duration = max
	}
	return duration
}

func defaultBackoffMinMS(cfg TrafficBackoffConfig) int {
	if cfg.MinBackoffMS > 0 {
		return cfg.MinBackoffMS
	}
	return 1000
}

func defaultBackoffMaxMS(cfg TrafficBackoffConfig) int {
	if cfg.MaxBackoffMS > 0 {
		return cfg.MaxBackoffMS
	}
	return 60000
}

func shapeReason(scope, bucket string) string {
	switch scope {
	case shapeScopeProvider:
		return "provider-shape-throttled"
	case shapeScopeProviderModel:
		return "model-shape-throttled"
	case shapeScopeTarget:
		return "target-shape-throttled"
	default:
		return bucket + "-shape-throttled"
	}
}

func shapeEvent(scope shapeScope, bucket, decision string, retryAfter time.Duration, in shapeReservationInput, reason string) upstreamShapeEventLogRecord {
	return upstreamShapeEventLogRecord{
		TS:                   time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Scope:                scope.Scope,
		Provider:             scope.Provider,
		ModelRef:             scope.ModelRef,
		Model:                scope.Model,
		Dialect:              scope.Dialect,
		Bucket:               bucket,
		Decision:             decision,
		RetryAfterMS:         int64(math.Ceil(float64(retryAfter) / float64(time.Millisecond))),
		EstimatedInputTokens: in.EstimatedInputTokens,
		ReservedOutputTokens: in.ReservedOutputTokens,
		TotalReservedTokens:  in.TotalReservedTokens,
		BackoffReason:        reason,
	}
}

func parseRetryAfterHeader(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if ts, err := http.ParseTime(value); err == nil {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if ts.After(now) {
			return ts.Sub(now)
		}
	}
	return 0
}

func retryAfterHeaderValue(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", int(math.Ceil(d.Seconds())))
}
