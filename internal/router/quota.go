package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"
)

type quotaStore struct {
	mu      sync.Mutex
	path    string
	callers map[string]*callerRuntime
	state   persistentState
}

type callerRuntime struct {
	cfg        CallerConfig
	allow      map[string]bool
	inFlight   int
	reqTimes   []time.Time
	tokenTimes []tokenEvent
}

type tokenEvent struct {
	At     time.Time `json:"at"`
	Tokens int       `json:"tokens"`
}

type persistentState struct {
	Callers map[string]*callerState `json:"callers"`
}

type callerState struct {
	DayStart       time.Time `json:"day_start"`
	MonthStart     time.Time `json:"month_start"`
	DayRequests    int64     `json:"day_requests"`
	DayTokens      int64     `json:"day_tokens"`
	MonthRequests  int64     `json:"month_requests"`
	MonthTokens    int64     `json:"month_tokens"`
	LifetimeTokens int64     `json:"lifetime_tokens"`
	Disabled       bool      `json:"disabled"`
}

type admission struct {
	OK          bool
	Status      int
	Reason      string
	RetryAfter  string
	QuotaState  string
	KeyState    string
	WarningText string
}

func newQuotaStore(path string, callers []CallerConfig) (*quotaStore, error) {
	qs := &quotaStore{path: path, callers: map[string]*callerRuntime{}, state: persistentState{Callers: map[string]*callerState{}}}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &qs.state); err != nil {
			return nil, err
		}
	}
	if qs.state.Callers == nil {
		qs.state.Callers = map[string]*callerState{}
	}
	now := time.Now().UTC()
	for _, cfg := range callers {
		allow := map[string]bool{}
		for _, group := range cfg.Allow {
			allow[group] = true
		}
		qs.callers[cfg.ID] = &callerRuntime{cfg: cfg, allow: allow}
		if qs.state.Callers[cfg.ID] == nil {
			qs.state.Callers[cfg.ID] = &callerState{DayStart: dayStart(now), MonthStart: monthStart(now)}
		}
	}
	return qs, nil
}

func (q *quotaStore) Admit(c *callerRuntime, estTokens int) admission {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	st := q.stateFor(c.cfg.ID, now)
	q.resetWindows(st, now)
	keyState := q.keyState(c.cfg, st, 0)
	if st.Disabled || keyState == "exhausted" {
		st.Disabled = true
		_ = q.saveLocked()
		return admission{Status: http.StatusForbidden, Reason: "key-exhausted", QuotaState: "ok", KeyState: "exhausted"}
	}
	if c.cfg.Rate.Concurrent > 0 && c.inFlight >= c.cfg.Rate.Concurrent {
		return admission{Status: http.StatusTooManyRequests, Reason: "concurrency-exceeded", RetryAfter: "1", QuotaState: "ok", KeyState: keyState}
	}
	c.reqTimes = pruneTimes(c.reqTimes, now.Add(-time.Minute))
	if c.cfg.Rate.RPM > 0 && len(c.reqTimes) >= c.cfg.Rate.RPM {
		return admission{Status: http.StatusTooManyRequests, Reason: "rpm-exceeded", RetryAfter: "60", QuotaState: "ok", KeyState: keyState}
	}
	c.tokenTimes = pruneTokens(c.tokenTimes, now.Add(-time.Minute))
	if c.cfg.Rate.TPM > 0 && sumTokenEvents(c.tokenTimes)+estTokens > c.cfg.Rate.TPM {
		return admission{Status: http.StatusTooManyRequests, Reason: "tpm-exceeded", RetryAfter: "60", QuotaState: "ok", KeyState: keyState}
	}
	if exceedsBudget(c.cfg.Quota.Day.Requests, st.DayRequests+1) || exceedsBudget(c.cfg.Quota.Day.Tokens, st.DayTokens+int64(estTokens)) ||
		exceedsBudget(c.cfg.Quota.Month.Requests, st.MonthRequests+1) || exceedsBudget(c.cfg.Quota.Month.Tokens, st.MonthTokens+int64(estTokens)) {
		return admission{Status: http.StatusTooManyRequests, Reason: "quota-exhausted", RetryAfter: "3600", QuotaState: "reject", KeyState: keyState}
	}
	c.inFlight++
	c.reqTimes = append(c.reqTimes, now)
	st.DayRequests++
	st.MonthRequests++
	quotaState, warning := q.quotaState(c.cfg, st, int64(estTokens))
	return admission{OK: true, Status: http.StatusOK, QuotaState: quotaState, KeyState: keyState, WarningText: warning}
}

func (q *quotaStore) Release(c *callerRuntime) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if c.inFlight > 0 {
		c.inFlight--
	}
}

func (q *quotaStore) RecordTokens(c *callerRuntime, usage Usage) (string, string) {
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	st := q.stateFor(c.cfg.ID, now)
	q.resetWindows(st, now)
	st.DayTokens += int64(total)
	st.MonthTokens += int64(total)
	st.LifetimeTokens += int64(total)
	c.tokenTimes = append(c.tokenTimes, tokenEvent{At: now, Tokens: total})
	keyState := q.keyState(c.cfg, st, 0)
	if keyState == "exhausted" {
		st.Disabled = true
	}
	_ = q.saveLocked()
	quotaState, _ := q.quotaState(c.cfg, st, 0)
	return quotaState, keyState
}

func (q *quotaStore) Usage(c *callerRuntime) map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	st := q.stateFor(c.cfg.ID, now)
	q.resetWindows(st, now)
	return map[string]any{
		"caller_id": c.cfg.ID,
		"day": map[string]any{
			"requests": st.DayRequests,
			"tokens":   st.DayTokens,
		},
		"month": map[string]any{
			"requests": st.MonthRequests,
			"tokens":   st.MonthTokens,
		},
		"key": map[string]any{
			"lifetime_tokens":   st.LifetimeTokens,
			"remaining_tokens":  remaining(c.cfg.Key.LifetimeTokens, st.LifetimeTokens),
			"exhausted":         st.Disabled,
			"configured_budget": c.cfg.Key.LifetimeTokens,
		},
	}
}

func (q *quotaStore) stateFor(id string, now time.Time) *callerState {
	st := q.state.Callers[id]
	if st == nil {
		st = &callerState{DayStart: dayStart(now), MonthStart: monthStart(now)}
		q.state.Callers[id] = st
	}
	return st
}

func (q *quotaStore) resetWindows(st *callerState, now time.Time) {
	ds := dayStart(now)
	ms := monthStart(now)
	if st.DayStart.Before(ds) {
		st.DayStart = ds
		st.DayRequests = 0
		st.DayTokens = 0
	}
	if st.MonthStart.Before(ms) {
		st.MonthStart = ms
		st.MonthRequests = 0
		st.MonthTokens = 0
	}
}

func (q *quotaStore) quotaState(cfg CallerConfig, st *callerState, est int64) (string, string) {
	soft := cfg.Quota.SoftPct
	if soft == 0 {
		soft = 80
	}
	if overSoft(cfg.Quota.Day.Requests, st.DayRequests, soft) || overSoft(cfg.Quota.Day.Tokens, st.DayTokens+est, soft) ||
		overSoft(cfg.Quota.Month.Requests, st.MonthRequests, soft) || overSoft(cfg.Quota.Month.Tokens, st.MonthTokens+est, soft) {
		return "soft", "rolling quota soft cap reached"
	}
	return "ok", ""
}

func (q *quotaStore) keyState(cfg CallerConfig, st *callerState, est int64) string {
	if cfg.Key.LifetimeTokens <= 0 {
		return "active"
	}
	used := st.LifetimeTokens + est
	if used >= cfg.Key.LifetimeTokens || st.Disabled {
		return "exhausted"
	}
	soft := cfg.Key.SoftPct
	if soft == 0 {
		soft = 90
	}
	if overSoft(cfg.Key.LifetimeTokens, used, soft) {
		return "soft"
	}
	return "active"
}

func (q *quotaStore) saveLocked() error {
	if q.path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(q.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.path, raw, 0600)
}

func (q *quotaStore) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.path == "" {
		return nil
	}
	if q.state.Callers == nil {
		return errors.New("quota state not initialized")
	}
	return q.saveLocked()
}

func pruneTimes(in []time.Time, cutoff time.Time) []time.Time {
	out := in[:0]
	for _, t := range in {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

func pruneTokens(in []tokenEvent, cutoff time.Time) []tokenEvent {
	out := in[:0]
	for _, ev := range in {
		if ev.At.After(cutoff) {
			out = append(out, ev)
		}
	}
	return out
}

func sumTokenEvents(in []tokenEvent) int {
	total := 0
	for _, ev := range in {
		total += ev.Tokens
	}
	return total
}

func exceedsBudget(limit, val int64) bool {
	return limit > 0 && val > limit
}

func overSoft(limit, val int64, pct int) bool {
	return limit > 0 && val*100 >= limit*int64(pct)
}

func remaining(limit, used int64) int64 {
	if limit <= 0 {
		return 0
	}
	if used >= limit {
		return 0
	}
	return limit - used
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func monthStart(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}
