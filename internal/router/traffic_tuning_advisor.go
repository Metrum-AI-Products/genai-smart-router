package router

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const trafficTuningDocsAnchor = "/docs/operations/admin-browser-reports#traffic-tuning-advisor"

type TrafficTuningAdvisorRow struct {
	Key                   string   `json:"key"`
	SecondaryKey          string   `json:"secondaryKey,omitempty"`
	Recommendation        string   `json:"recommendation"`
	Severity              string   `json:"severity"`
	Requests              int64    `json:"requests"`
	Successes             int64    `json:"successes"`
	SuccessRatePct        float64  `json:"successRatePct"`
	TerminalErrors        int64    `json:"terminalErrors"`
	ErrorRatePct          float64  `json:"errorRatePct"`
	Router429             int64    `json:"router429"`
	TrafficShaped         int64    `json:"trafficShaped"`
	Rejected              int64    `json:"rejected"`
	Queued                int64    `json:"queued"`
	QueueWaitP50MS        int64    `json:"queueWaitP50Ms"`
	QueueWaitP95MS        int64    `json:"queueWaitP95Ms"`
	QueueWaitMaxMS        int64    `json:"queueWaitMaxMs"`
	RetryAfterP50MS       int64    `json:"retryAfterP50Ms"`
	RetryAfterP95MS       int64    `json:"retryAfterP95Ms"`
	RetryAfterMaxMS       int64    `json:"retryAfterMaxMs"`
	EstimatedInputP95     int64    `json:"estimatedInputP95"`
	ReservedOutputP95     int64    `json:"reservedOutputP95"`
	TotalReservedP95      int64    `json:"totalReservedP95"`
	Upstream400           int64    `json:"upstream400"`
	Upstream429           int64    `json:"upstream429"`
	Upstream5xx           int64    `json:"upstream5xx"`
	UpstreamTimeouts      int64    `json:"upstreamTimeouts"`
	ClientCanceled        int64    `json:"clientCanceled"`
	FallbackRatePct       float64  `json:"fallbackRatePct"`
	SuccessAfterFallbacks int64    `json:"successAfterFallbacks"`
	LatencyP50MS          int64    `json:"latencyP50Ms"`
	LatencyP95MS          int64    `json:"latencyP95Ms"`
	LatencyMaxMS          int64    `json:"latencyMaxMs"`
	AffectedUsers         int64    `json:"affectedUsers"`
	AffectedClients       int64    `json:"affectedClients"`
	ObservedValues        string   `json:"observedValues"`
	Threshold             string   `json:"threshold"`
	ConfigFields          []string `json:"configFields"`
	Explanation           string   `json:"explanation"`
	Docs                  string   `json:"docs"`
}

type trafficTuningAttempt struct {
	RequestID      string
	StatusCode     int
	ErrorClass     string
	TimedOut       bool
	ClientCanceled bool
	RetryAfterMS   int64
}

type trafficTuningAgg struct {
	key                   string
	secondary             string
	requests              int64
	successes             int64
	errors                int64
	router429             int64
	hardCaller429         int64
	trafficShaped         int64
	rejected              int64
	queued                int64
	fallbacks             int64
	successAfterFallbacks int64
	upstream400           int64
	upstream429           int64
	upstream5xx           int64
	upstreamTimeouts      int64
	clientCanceled        int64
	users                 map[string]bool
	clients               map[string]bool
	queueWaits            []int64
	retryAfters           []int64
	estimatedInputs       []int64
	reservedOutputs       []int64
	totalReserved         []int64
	latencies             []int64
}

func GenerateTrafficTuningAdvisorMarkdown(opts UsageReportOptions) (string, error) {
	driver := strings.ToLower(defaultString(opts.Driver, "sqlite"))
	if driver == "sqlite" && opts.DBPath == "" {
		return "", errors.New("usage db path is required")
	}
	if (driver == "postgres" || driver == "postgresql") && opts.DSN == "" {
		return "", errors.New("usage db dsn is required")
	}
	if opts.To.IsZero() {
		opts.To = time.Now().UTC()
	}
	if opts.From.IsZero() {
		opts.From = opts.To.Add(-24 * time.Hour)
	}
	if !opts.From.Before(opts.To) {
		return "", errors.New("from must be before to")
	}
	cfg := UsageDBConfig{Driver: driver, Path: opts.DBPath, DSN: opts.DSN}
	if opts.LogPath != "" {
		if _, err := ImportUsageJSONLTo(cfg, opts.LogPath); err != nil {
			return "", err
		}
	}
	store, err := OpenUsageStore(cfg)
	if err != nil {
		return "", err
	}
	defer store.Close()
	rows, err := store.rows(opts)
	if err != nil {
		return "", err
	}
	upstreamEvents, err := store.upstreamShapeEventsForRows(rows, opts)
	if err != nil {
		return "", err
	}
	attempts, err := store.trafficTuningAttemptsForRows(rows, opts)
	if err != nil {
		return "", err
	}
	return renderTrafficTuningAdvisorMarkdown(opts.From, opts.To, BuildTrafficTuningAdvisor(rows, upstreamEvents, attempts)), nil
}

func BuildTrafficTuningAdvisor(rows []usageRow, upstreamEvents []upstreamShapeJoinedEvent, attempts []trafficTuningAttempt) []TrafficTuningAdvisorRow {
	attemptsByRequest := map[string][]trafficTuningAttempt{}
	for _, attempt := range attempts {
		attemptsByRequest[attempt.RequestID] = append(attemptsByRequest[attempt.RequestID], attempt)
	}
	upstream429ByRequest := map[string]int64{}
	for _, event := range upstreamEvents {
		if event.Event.Decision == shapeDecisionSkipped || event.Event.Decision == shapeDecisionCooldownStarted || event.Event.Decision == shapeDecisionRejected {
			if event.Event.BackoffReason == "adaptive-backoff-provider-429" {
				upstream429ByRequest[event.Row.RequestID]++
			}
		}
	}
	aggs := map[string]*trafficTuningAgg{}
	for _, row := range rows {
		for _, dim := range trafficTuningDimensions(row) {
			agg := aggs[joinKey(dim.key, dim.secondary)]
			if agg == nil {
				agg = &trafficTuningAgg{key: dim.key, secondary: dim.secondary, users: map[string]bool{}, clients: map[string]bool{}}
				aggs[joinKey(dim.key, dim.secondary)] = agg
			}
			agg.addRow(row, attemptsByRequest[row.RequestID], upstream429ByRequest[row.RequestID])
		}
	}
	out := make([]TrafficTuningAdvisorRow, 0, len(aggs))
	for _, agg := range aggs {
		out = append(out, agg.recommendation())
	}
	sort.Slice(out, func(i, j int) bool {
		if severityRank(out[i].Severity) == severityRank(out[j].Severity) {
			if out[i].Requests == out[j].Requests {
				return out[i].Key < out[j].Key
			}
			return out[i].Requests > out[j].Requests
		}
		return severityRank(out[i].Severity) > severityRank(out[j].Severity)
	})
	if len(out) == 0 {
		out = append(out, TrafficTuningAdvisorRow{
			Key:            "selected-window",
			Recommendation: "no_shaping_change_indicated",
			Severity:       "info",
			ObservedValues: "no matching usage rows",
			Threshold:      "advisor needs at least one request in the selected window",
			ConfigFields:   []string{"server.usage_db", "admin report filters"},
			Explanation:    "No request rows matched the selected filters, so the advisor cannot infer a traffic-shaping change.",
			Docs:           trafficTuningDocsAnchor,
		})
	}
	return out
}

func (s *usageStore) trafficTuningAttemptsForRows(rows []usageRow, opts UsageReportOptions) ([]trafficTuningAttempt, error) {
	if s == nil || s.db == nil || len(rows) == 0 {
		return nil, nil
	}
	requestIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RequestID != "" {
			requestIDs = append(requestIDs, row.RequestID)
		}
	}
	if len(requestIDs) == 0 {
		return nil, nil
	}
	q := s.db.Model(&requestAttemptRecord{}).Where("request_id IN ?", requestIDs)
	if opts.TargetProvider != "" {
		q = q.Where("provider = ?", opts.TargetProvider)
	}
	if opts.TargetModel != "" {
		q = q.Where("model = ?", opts.TargetModel)
	}
	if opts.TargetDialect != "" {
		q = q.Where("dialect = ?", opts.TargetDialect)
	}
	var records []requestAttemptRecord
	if err := q.Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]trafficTuningAttempt, 0, len(records))
	for _, record := range records {
		out = append(out, trafficTuningAttempt{
			RequestID:      record.RequestID,
			StatusCode:     record.StatusCode,
			ErrorClass:     record.ErrorClass,
			TimedOut:       record.TimedOut,
			ClientCanceled: record.ClientCanceled,
			RetryAfterMS:   record.RetryAfterMS,
		})
	}
	return out, nil
}

type trafficTuningDimensionKey struct {
	key       string
	secondary string
}

func trafficTuningDimensions(row usageRow) []trafficTuningDimensionKey {
	resolvedGroup := defaultString(row.ResolvedGroup, defaultString(row.RequestedModel, "unknown"))
	provider := defaultString(row.TargetProvider, "unknown")
	model := defaultString(row.TargetModel, "unknown")
	dialect := defaultString(row.TargetDialect, "unknown")
	return []trafficTuningDimensionKey{
		{
			key: fmt.Sprintf("user=%s project=%s client=%s group=%s",
				defaultString(row.CallerUser, "unknown"),
				defaultString(row.CallerProject, "unknown"),
				defaultString(row.Client, "unknown"),
				resolvedGroup),
			secondary: fmt.Sprintf("caller=%s env=%s provider=%s model=%s dialect=%s",
				defaultString(row.CallerID, "unknown"),
				defaultString(row.CallerEnvironment, "unknown"),
				provider,
				model,
				dialect),
		},
		{
			key:       fmt.Sprintf("provider=%s model=%s dialect=%s", provider, model, dialect),
			secondary: fmt.Sprintf("group=%s project=%s env=%s", resolvedGroup, defaultString(row.CallerProject, "unknown"), defaultString(row.CallerEnvironment, "unknown")),
		},
	}
}

func (a *trafficTuningAgg) addRow(row usageRow, attempts []trafficTuningAttempt, upstream429FromShape int64) {
	a.requests++
	if row.Status < 400 {
		a.successes++
	} else {
		a.errors++
	}
	if row.Status == http.StatusTooManyRequests {
		a.router429++
		if row.TrafficShapeDecision != trafficShapeDecisionRejected {
			a.hardCaller429++
		}
	}
	if row.TrafficShapeApplied {
		a.trafficShaped++
	}
	switch row.TrafficShapeDecision {
	case trafficShapeDecisionRejected:
		a.rejected++
	case trafficShapeDecisionQueued:
		a.queued++
	}
	if row.FallbackUsed {
		a.fallbacks++
		if row.Status < 400 {
			a.successAfterFallbacks++
		}
	}
	if row.CallerUser != "" {
		a.users[row.CallerUser] = true
	}
	if row.Client != "" {
		a.clients[row.Client] = true
	}
	appendPositive := func(dst []int64, v int64) []int64 {
		if v > 0 {
			return append(dst, v)
		}
		return dst
	}
	a.queueWaits = appendPositive(a.queueWaits, row.TrafficShapeQueueWaitMS)
	a.retryAfters = appendPositive(a.retryAfters, row.TrafficShapeRetryAfterMS)
	a.estimatedInputs = appendPositive(a.estimatedInputs, int64(row.TrafficShapeEstimatedInputTokens))
	a.reservedOutputs = appendPositive(a.reservedOutputs, int64(row.TrafficShapeReservedOutputTokens))
	a.totalReserved = appendPositive(a.totalReserved, int64(row.TrafficShapeTotalReservedTokens))
	a.latencies = appendPositive(a.latencies, row.LatencyMS)
	a.upstream429 += upstream429FromShape
	for _, attempt := range attempts {
		switch {
		case attempt.StatusCode >= 400 && attempt.StatusCode < 500 && attempt.StatusCode != http.StatusTooManyRequests:
			a.upstream400++
		case attempt.StatusCode == http.StatusTooManyRequests:
			a.upstream429++
		case attempt.StatusCode >= 500:
			a.upstream5xx++
		}
		if attempt.TimedOut {
			a.upstreamTimeouts++
		}
		if attempt.ClientCanceled {
			a.clientCanceled++
		}
		a.retryAfters = appendPositive(a.retryAfters, attempt.RetryAfterMS)
	}
}

func (a *trafficTuningAgg) recommendation() TrafficTuningAdvisorRow {
	row := TrafficTuningAdvisorRow{
		Key:                   a.key,
		SecondaryKey:          a.secondary,
		Requests:              a.requests,
		Successes:             a.successes,
		SuccessRatePct:        pct(a.successes, a.requests),
		TerminalErrors:        a.errors,
		ErrorRatePct:          pct(a.errors, a.requests),
		Router429:             a.router429,
		TrafficShaped:         a.trafficShaped,
		Rejected:              a.rejected,
		Queued:                a.queued,
		QueueWaitP50MS:        percentileInt64(a.queueWaits, 50),
		QueueWaitP95MS:        percentileInt64(a.queueWaits, 95),
		QueueWaitMaxMS:        maxInt64Slice(a.queueWaits),
		RetryAfterP50MS:       percentileInt64(a.retryAfters, 50),
		RetryAfterP95MS:       percentileInt64(a.retryAfters, 95),
		RetryAfterMaxMS:       maxInt64Slice(a.retryAfters),
		EstimatedInputP95:     percentileInt64(a.estimatedInputs, 95),
		ReservedOutputP95:     percentileInt64(a.reservedOutputs, 95),
		TotalReservedP95:      percentileInt64(a.totalReserved, 95),
		Upstream400:           a.upstream400,
		Upstream429:           a.upstream429,
		Upstream5xx:           a.upstream5xx,
		UpstreamTimeouts:      a.upstreamTimeouts,
		ClientCanceled:        a.clientCanceled,
		FallbackRatePct:       pct(a.fallbacks, a.requests),
		SuccessAfterFallbacks: a.successAfterFallbacks,
		LatencyP50MS:          percentileInt64(a.latencies, 50),
		LatencyP95MS:          percentileInt64(a.latencies, 95),
		LatencyMaxMS:          maxInt64Slice(a.latencies),
		AffectedUsers:         int64(len(a.users)),
		AffectedClients:       int64(len(a.clients)),
		Docs:                  trafficTuningDocsAnchor,
	}
	row.ObservedValues = fmt.Sprintf("requests=%d errors=%d router429=%d shaped=%d rejected=%d queued=%d upstream400=%d upstream429=%d upstream5xx=%d timeouts=%d canceled=%d queue_p95_ms=%d retry_after_p95_ms=%d",
		row.Requests, row.TerminalErrors, row.Router429, row.TrafficShaped, row.Rejected, row.Queued, row.Upstream400, row.Upstream429, row.Upstream5xx, row.UpstreamTimeouts, row.ClientCanceled, row.QueueWaitP95MS, row.RetryAfterP95MS)

	errorRate := ratio(a.errors, a.requests)
	rejectRate := ratio(a.rejected, a.requests)
	queueRate := ratio(a.queued, a.requests)
	upstream400Rate := ratio(a.upstream400, maxInt64Slice([]int64{a.requests, 1}))
	upstream429Rate := ratio(a.upstream429, maxInt64Slice([]int64{a.requests, 1}))
	upstream5xxRate := ratio(a.upstream5xx+a.upstreamTimeouts, maxInt64Slice([]int64{a.requests, 1}))
	cancelRate := ratio(a.clientCanceled, maxInt64Slice([]int64{a.requests, 1}))

	switch {
	case a.requests == 0:
		row.Recommendation = "no_shaping_change_indicated"
		row.Severity = "info"
		row.Threshold = "no requests in selected bucket"
		row.ConfigFields = []string{"server.usage_db"}
		row.Explanation = "No matching request rows were available for this dimension."
	case upstream400Rate >= 0.10 && a.rejected == 0 && a.queued == 0:
		row.Recommendation = "route_around_incompatible_target"
		row.Severity = severityForRate(upstream400Rate)
		row.Threshold = "upstream 400 attempts >= 10% and caller shaping rejected/queued == 0"
		row.ConfigFields = []string{"models.<group>.targets[]", "provider catalog tool_support", "provider catalog input_modalities", "target request-shape eligibility"}
		row.Explanation = "Caller burst or queue tuning is unlikely to help because the router admitted traffic and upstream 400/request-shape failures dominate. Investigate the selected provider/model/dialect and route around incompatible targets."
	case ratio(a.hardCaller429, a.requests) >= 0.02:
		row.Recommendation = "adjust_caller_quota_or_rate_limit"
		row.Severity = severityForRate(ratio(a.hardCaller429, a.requests))
		row.Threshold = "hard caller admission 429 rate >= 2%"
		row.ConfigFields = []string{"callers[].rpm", "callers[].tpm", "callers[].concurrent", "callers[].quota", "callers[].key.lifetime_tokens"}
		row.Explanation = "The router rejected requests before traffic shaping or upstream attempts because hard caller limits or quotas were reached. Adjust caller rate/quota policy or client pacing; traffic-shaping burst and provider capacity tuning will not fix these rows."
	case upstream429Rate >= 0.05 && a.affectedUsersOrDefault() > 1:
		row.Recommendation = "investigate_provider_429_capacity"
		row.Severity = severityForRate(upstream429Rate)
		row.Threshold = "upstream/provider 429 signals >= 5% across more than one user"
		row.ConfigFields = []string{"providers.<name>.traffic_shape", "providers.<name>.models.<ref>.traffic_shape", "models.<group>.targets[].traffic_shape", "upstream_429_backoff"}
		row.Explanation = "Provider capacity or account quota appears shared across users, so per-user burst increases may amplify the incident. Tune provider/model shaping, backoff, or routing mix first."
	case cancelRate >= 0.05 && row.QueueWaitP95MS >= 1000:
		row.Recommendation = "disable_queue_for_latency_sensitive_client"
		row.Severity = severityForRate(cancelRate)
		row.Threshold = "client cancellations >= 5% with queue wait p95 >= 1000 ms"
		row.ConfigFields = []string{"callers[].traffic_shape.queue.enabled", "callers[].traffic_shape.queue.max_wait_ms", "server.traffic_shape.default_caller.queue.max_wait_ms"}
		row.Explanation = "The client is canceling while queued. For latency-sensitive agent traffic, fail fast or lower queue wait before increasing depth."
	case a.rejected > 0 && row.QueueWaitP95MS > 0:
		row.Recommendation = "increase_queue_depth"
		row.Severity = severityForRate(rejectRate)
		row.Threshold = "traffic-shape rejections after observed queue waits"
		row.ConfigFields = []string{"callers[].traffic_shape.queue.max_depth", "server.traffic_shape.default_caller.queue.max_depth"}
		row.Explanation = "Short bursts are reaching a configured queue but still rejecting. Consider a small queue-depth increase if latency and upstream errors remain acceptable."
	case a.rejected > 0 && a.queued == 0:
		row.Recommendation = "enable_queue"
		row.Severity = severityForRate(rejectRate)
		row.Threshold = "traffic-shape rejections with no queued admissions"
		row.ConfigFields = []string{"callers[].traffic_shape.queue.enabled", "callers[].traffic_shape.queue.max_wait_ms", "callers[].traffic_shape.queue.max_depth"}
		row.Explanation = "The caller is hitting burst buckets and no bounded queue is smoothing the burst. Enable a small queue or ask the client to slow request starts."
	case rejectRate >= 0.02 && upstream5xxRate < 0.05:
		row.Recommendation = "increase_caller_burst"
		row.Severity = severityForRate(rejectRate)
		row.Threshold = "caller traffic-shape/router 429 rate >= 2% with low upstream 5xx/timeout rate"
		row.ConfigFields = []string{"callers[].traffic_shape.request_burst", "callers[].traffic_shape.input_token_burst", "callers[].traffic_shape.total_reserved_token_burst"}
		row.Explanation = "The router is rejecting a measurable caller burst while upstream failures are not the dominant signal. Increase the specific bucket burst cautiously or slow the caller down."
	case queueRate >= 0.10 && row.QueueWaitP95MS >= 2000:
		row.Recommendation = "increase_queue_wait"
		row.Severity = "medium"
		row.Threshold = "queued admissions >= 10% and queue wait p95 >= 2000 ms"
		row.ConfigFields = []string{"callers[].traffic_shape.queue.max_wait_ms", "server.traffic_shape.default_caller.queue.max_wait_ms"}
		row.Explanation = "Queued traffic is common and waits are near seconds. Increase max wait only for batch-tolerant clients; otherwise reduce request rate."
	case errorRate >= 0.10 && upstream5xxRate >= 0.05:
		row.Recommendation = "decrease_caller_rate"
		row.Severity = severityForRate(errorRate)
		row.Threshold = "terminal error rate >= 10% with upstream 5xx/timeout rate >= 5%"
		row.ConfigFields = []string{"callers[].rpm", "callers[].tpm", "callers[].traffic_shape.*_per_sec", "client retry/backoff settings"}
		row.Explanation = "The caller is seeing substantial errors while upstreams are unhealthy. Slow the caller or reduce concurrency while provider health is investigated."
	default:
		row.Recommendation = "no_shaping_change_indicated"
		row.Severity = "info"
		row.Threshold = "no conservative advisor threshold crossed"
		row.ConfigFields = []string{"usage reports", "traffic-shaping reports", "upstream failure reports"}
		row.Explanation = "No burst, queue, provider-capacity, or request-shape signal crossed the conservative starting thresholds. Do not change shaping based on this window alone."
	}
	return row
}

func (a *trafficTuningAgg) affectedUsersOrDefault() int64 {
	if len(a.users) == 0 {
		return 1
	}
	return int64(len(a.users))
}

func renderTrafficTuningAdvisorMarkdown(from, to time.Time, rows []TrafficTuningAdvisorRow) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Traffic Tuning Advisor")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Window UTC: `%s` to `%s`\n\n", formatUsageTime(from), formatUsageTime(to))
	fmt.Fprintln(&b, "| Recommendation | Severity | Dimension | Requests | Errors | Router 429 | Shaped | Rejected | Queued | Queue p95 ms | Retry-After p95 ms | Upstream 400 | Upstream 429 | Upstream 5xx/timeout | Client canceled | Config fields |")
	fmt.Fprintln(&b, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s / %s | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %s |\n",
			esc(row.Recommendation), esc(row.Severity), esc(row.Key), esc(row.SecondaryKey), row.Requests, row.TerminalErrors, row.Router429, row.TrafficShaped, row.Rejected, row.Queued,
			row.QueueWaitP95MS, row.RetryAfterP95MS, row.Upstream400, row.Upstream429, row.Upstream5xx+row.UpstreamTimeouts, row.ClientCanceled, esc(strings.Join(row.ConfigFields, ", ")))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Evidence")
	fmt.Fprintln(&b)
	for _, row := range rows {
		fmt.Fprintf(&b, "- `%s`: %s. Threshold: %s. %s Docs: `%s`.\n", esc(row.Recommendation), esc(row.ObservedValues), esc(row.Threshold), esc(row.Explanation), esc(row.Docs))
	}
	return b.String()
}

func severityForRate(rate float64) string {
	switch {
	case rate >= 0.25:
		return "high"
	case rate >= 0.10:
		return "medium"
	default:
		return "low"
	}
}

func severityRank(severity string) int {
	switch severity {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func pct(n, d int64) float64 {
	if d <= 0 {
		return 0
	}
	return math.Round(float64(n)*10000/float64(d)) / 100
}

func ratio(n, d int64) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func maxInt64Slice(values []int64) int64 {
	var out int64
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}
