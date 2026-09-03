// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
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
	userCount             int64
	clientCount           int64
	queueWaits            []int64
	retryAfters           []int64
	estimatedInputs       []int64
	reservedOutputs       []int64
	totalReserved         []int64
	latencies             []int64
}

type trafficTuningSQLAggRecord struct {
	Key                   string
	SecondaryKey          string
	Requests              int64
	Successes             int64
	Errors                int64
	Router429             int64
	HardCaller429         int64
	TrafficShaped         int64
	Rejected              int64
	Queued                int64
	Fallbacks             int64
	SuccessAfterFallbacks int64
	Upstream400           int64
	Upstream429           int64
	Upstream5xx           int64
	UpstreamTimeouts      int64
	ClientCanceled        int64
	AffectedUsers         int64
	AffectedClients       int64
	QueueWaitP50MS        int64
	QueueWaitP95MS        int64
	QueueWaitMaxMS        int64
	RetryAfterP50MS       int64
	RetryAfterP95MS       int64
	RetryAfterMaxMS       int64
	EstimatedInputP95     int64
	ReservedOutputP95     int64
	TotalReservedP95      int64
	LatencyP50MS          int64
	LatencyP95MS          int64
	LatencyMaxMS          int64
}

type trafficTuningPercentileRecord struct {
	Key          string
	SecondaryKey string
	P50          int64
	P95          int64
	Max          int64
}

type trafficTuningDimensionSQL struct {
	keyExpr       string
	secondaryExpr string
	groupBy       string
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
	rows, _, _, err := store.trafficTuningAdvisorRowsSQL(opts, 0)
	if err != nil {
		return "", err
	}
	return renderTrafficTuningAdvisorMarkdown(opts.From, opts.To, rows), nil
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
	sortTrafficTuningAdvisorRows(out)
	if len(out) == 0 {
		out = append(out, emptyTrafficTuningAdvisorRow())
	}
	return out
}

func BuildTrafficTuningAdvisorFromSQL(records []trafficTuningSQLAggRecord) []TrafficTuningAdvisorRow {
	out := make([]TrafficTuningAdvisorRow, 0, len(records))
	for _, record := range records {
		row := TrafficTuningAdvisorRow{
			Key:                   record.Key,
			SecondaryKey:          record.SecondaryKey,
			Requests:              record.Requests,
			Successes:             record.Successes,
			SuccessRatePct:        pct(record.Successes, record.Requests),
			TerminalErrors:        record.Errors,
			ErrorRatePct:          pct(record.Errors, record.Requests),
			Router429:             record.Router429,
			TrafficShaped:         record.TrafficShaped,
			Rejected:              record.Rejected,
			Queued:                record.Queued,
			QueueWaitP50MS:        record.QueueWaitP50MS,
			QueueWaitP95MS:        record.QueueWaitP95MS,
			QueueWaitMaxMS:        record.QueueWaitMaxMS,
			RetryAfterP50MS:       record.RetryAfterP50MS,
			RetryAfterP95MS:       record.RetryAfterP95MS,
			RetryAfterMaxMS:       record.RetryAfterMaxMS,
			EstimatedInputP95:     record.EstimatedInputP95,
			ReservedOutputP95:     record.ReservedOutputP95,
			TotalReservedP95:      record.TotalReservedP95,
			Upstream400:           record.Upstream400,
			Upstream429:           record.Upstream429,
			Upstream5xx:           record.Upstream5xx,
			UpstreamTimeouts:      record.UpstreamTimeouts,
			ClientCanceled:        record.ClientCanceled,
			FallbackRatePct:       pct(record.Fallbacks, record.Requests),
			SuccessAfterFallbacks: record.SuccessAfterFallbacks,
			LatencyP50MS:          record.LatencyP50MS,
			LatencyP95MS:          record.LatencyP95MS,
			LatencyMaxMS:          record.LatencyMaxMS,
			AffectedUsers:         record.AffectedUsers,
			AffectedClients:       record.AffectedClients,
			Docs:                  trafficTuningDocsAnchor,
		}
		out = append(out, applyTrafficTuningRecommendation(row, record.HardCaller429, affectedUsersOrDefault(record.AffectedUsers)))
	}
	sortTrafficTuningAdvisorRows(out)
	if len(out) == 0 {
		out = append(out, emptyTrafficTuningAdvisorRow())
	}
	return out
}

func sortTrafficTuningAdvisorRows(rows []TrafficTuningAdvisorRow) {
	sort.Slice(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) == severityRank(rows[j].Severity) {
			if rows[i].Requests == rows[j].Requests {
				return rows[i].Key < rows[j].Key
			}
			return rows[i].Requests > rows[j].Requests
		}
		return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
	})
}

func emptyTrafficTuningAdvisorRow() TrafficTuningAdvisorRow {
	return TrafficTuningAdvisorRow{
		Key:            "selected-window",
		Recommendation: "no_shaping_change_indicated",
		Severity:       "info",
		ObservedValues: "no matching usage rows",
		Threshold:      "advisor needs at least one request in the selected window",
		ConfigFields:   []string{"server.usage_db", "admin report filters"},
		Explanation:    "No request rows matched the selected filters, so the advisor cannot infer a traffic-shaping change.",
		Docs:           trafficTuningDocsAnchor,
	}
}

func (s *usageStore) trafficTuningAdvisorRowsSQL(opts UsageReportOptions, limit int) ([]TrafficTuningAdvisorRow, *agg, bool, error) {
	if s == nil || s.db == nil {
		return nil, nil, false, errors.New("usage store is not open")
	}
	var totalRec tokenScalarAggRecord
	if err := s.usageRowsQuery(opts).Select(adminScalarAggSQLSelectExpr(adminSavingsBaselineDTO{})).Scan(&totalRec).Error; err != nil {
		return nil, nil, false, err
	}
	records := []trafficTuningSQLAggRecord{}
	for _, dim := range trafficTuningSQLDimensions() {
		parent, err := s.trafficTuningParentAggSQL(opts, dim)
		if err != nil {
			return nil, nil, false, err
		}
		if err := s.applyTrafficTuningAttemptAggSQL(parent, opts, dim); err != nil {
			return nil, nil, false, err
		}
		if err := s.applyTrafficTuningUpstreamShapeAggSQL(parent, opts, dim); err != nil {
			return nil, nil, false, err
		}
		percentileSpecs := []struct {
			column string
			apply  func(*trafficTuningSQLAggRecord, trafficTuningPercentileRecord)
		}{
			{"u.traffic_shape_queue_wait_ms", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.QueueWaitP50MS, r.QueueWaitP95MS, r.QueueWaitMaxMS = p.P50, p.P95, p.Max
			}},
			{"u.traffic_shape_retry_after_ms", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.RetryAfterP50MS, r.RetryAfterP95MS, r.RetryAfterMaxMS = p.P50, p.P95, p.Max
			}},
			{"u.traffic_shape_estimated_input_tokens", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.EstimatedInputP95 = p.P95
			}},
			{"u.traffic_shape_reserved_output_tokens", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.ReservedOutputP95 = p.P95
			}},
			{"u.traffic_shape_total_reserved_tokens", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.TotalReservedP95 = p.P95
			}},
			{"u.latency_ms", func(r *trafficTuningSQLAggRecord, p trafficTuningPercentileRecord) {
				r.LatencyP50MS, r.LatencyP95MS, r.LatencyMaxMS = p.P50, p.P95, p.Max
			}},
		}
		for _, spec := range percentileSpecs {
			percentiles, err := s.trafficTuningPercentilesSQL(opts, dim, spec.column)
			if err != nil {
				return nil, nil, false, err
			}
			for key, percentile := range percentiles {
				if record := parent[key]; record != nil {
					spec.apply(record, percentile)
				}
			}
		}
		for _, record := range parent {
			records = append(records, *record)
		}
	}
	rows := BuildTrafficTuningAdvisorFromSQL(records)
	hasMore := false
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
		hasMore = true
	}
	total := aggFromTokenScalarAggRecord(totalRec)
	return rows, &total, hasMore, nil
}

func trafficTuningSQLDimensions() []trafficTuningDimensionSQL {
	groupExpr := "COALESCE(NULLIF(u.resolved_group, ''), NULLIF(u.requested_model, ''), 'unknown')"
	providerExpr := "COALESCE(NULLIF(u.target_provider, ''), 'unknown')"
	modelExpr := "COALESCE(NULLIF(u.target_model, ''), 'unknown')"
	dialectExpr := "COALESCE(NULLIF(u.target_dialect, ''), 'unknown')"
	return []trafficTuningDimensionSQL{
		{
			keyExpr:       "('user=' || COALESCE(NULLIF(u.caller_user, ''), 'unknown') || ' project=' || COALESCE(NULLIF(u.caller_project, ''), 'unknown') || ' client=' || COALESCE(NULLIF(u.client, ''), 'unknown') || ' group=' || " + groupExpr + ")",
			secondaryExpr: "('caller=' || COALESCE(NULLIF(u.caller_id, ''), 'unknown') || ' env=' || COALESCE(NULLIF(u.caller_environment, ''), 'unknown') || ' provider=' || " + providerExpr + " || ' model=' || " + modelExpr + " || ' dialect=' || " + dialectExpr + ")",
			groupBy:       "u.caller_user, u.caller_project, u.client, " + groupExpr + ", u.caller_id, u.caller_environment, " + providerExpr + ", " + modelExpr + ", " + dialectExpr,
		},
		{
			keyExpr:       "('provider=' || " + providerExpr + " || ' model=' || " + modelExpr + " || ' dialect=' || " + dialectExpr + ")",
			secondaryExpr: "('group=' || " + groupExpr + " || ' project=' || COALESCE(NULLIF(u.caller_project, ''), 'unknown') || ' env=' || COALESCE(NULLIF(u.caller_environment, ''), 'unknown'))",
			groupBy:       providerExpr + ", " + modelExpr + ", " + dialectExpr + ", " + groupExpr + ", u.caller_project, u.caller_environment",
		},
	}
}

func (s *usageStore) trafficTuningParentAggSQL(opts UsageReportOptions, dim trafficTuningDimensionSQL) (map[string]*trafficTuningSQLAggRecord, error) {
	var records []trafficTuningSQLAggRecord
	err := s.db.Table("(?) AS u", s.usageRowsQuery(opts)).Select(dim.keyExpr+` AS key,
		`+dim.secondaryExpr+` AS secondary_key,
		COUNT(*) AS requests,
		SUM(CASE WHEN u.status < 400 THEN 1 ELSE 0 END) AS successes,
		SUM(CASE WHEN u.status >= 400 THEN 1 ELSE 0 END) AS errors,
		SUM(CASE WHEN u.status = 429 THEN 1 ELSE 0 END) AS router429,
		SUM(CASE WHEN u.status = 429 AND u.traffic_shape_decision <> ? THEN 1 ELSE 0 END) AS hard_caller429,
		SUM(CASE WHEN u.traffic_shape_applied THEN 1 ELSE 0 END) AS traffic_shaped,
		SUM(CASE WHEN u.traffic_shape_decision = ? THEN 1 ELSE 0 END) AS rejected,
		SUM(CASE WHEN u.traffic_shape_decision = ? THEN 1 ELSE 0 END) AS queued,
		SUM(CASE WHEN u.fallback_used THEN 1 ELSE 0 END) AS fallbacks,
		SUM(CASE WHEN u.fallback_used AND u.status < 400 THEN 1 ELSE 0 END) AS success_after_fallbacks,
		COUNT(DISTINCT CASE WHEN u.caller_user <> '' THEN u.caller_user END) AS affected_users,
		COUNT(DISTINCT CASE WHEN u.client <> '' THEN u.client END) AS affected_clients,
		MAX(u.traffic_shape_queue_wait_ms) AS queue_wait_max_ms,
		MAX(u.traffic_shape_retry_after_ms) AS retry_after_max_ms,
		MAX(u.latency_ms) AS latency_max_ms`, trafficShapeDecisionRejected, trafficShapeDecisionRejected, trafficShapeDecisionQueued).
		Group(dim.groupBy).
		Scan(&records).Error
	if err != nil {
		return nil, err
	}
	return trafficTuningRecordsByKey(records), nil
}

func (s *usageStore) applyTrafficTuningAttemptAggSQL(records map[string]*trafficTuningSQLAggRecord, opts UsageReportOptions, dim trafficTuningDimensionSQL) error {
	var attemptRecords []trafficTuningSQLAggRecord
	q := s.db.Table("request_attempts AS child").
		Joins("JOIN (?) AS u ON u.request_id = child.request_id", s.usageRowsQuery(opts))
	if opts.TargetProvider != "" {
		q = q.Where("child.provider = ?", opts.TargetProvider)
	}
	if opts.TargetModel != "" {
		q = q.Where("child.model = ?", opts.TargetModel)
	}
	if opts.TargetDialect != "" {
		q = q.Where("child.dialect = ?", opts.TargetDialect)
	}
	if err := q.Select(dim.keyExpr + ` AS key,
		` + dim.secondaryExpr + ` AS secondary_key,
		SUM(CASE WHEN child.status_code >= 400 AND child.status_code < 500 AND child.status_code <> 429 THEN 1 ELSE 0 END) AS upstream400,
		SUM(CASE WHEN child.status_code = 429 THEN 1 ELSE 0 END) AS upstream429,
		SUM(CASE WHEN child.status_code >= 500 THEN 1 ELSE 0 END) AS upstream5xx,
		SUM(CASE WHEN child.timed_out THEN 1 ELSE 0 END) AS upstream_timeouts,
		SUM(CASE WHEN child.client_canceled THEN 1 ELSE 0 END) AS client_canceled,
		MAX(child.retry_after_ms) AS retry_after_max_ms`).
		Group(dim.groupBy).
		Scan(&attemptRecords).Error; err != nil {
		return err
	}
	for _, attempt := range attemptRecords {
		record := records[joinKey(attempt.Key, attempt.SecondaryKey)]
		if record == nil {
			continue
		}
		record.Upstream400 += attempt.Upstream400
		record.Upstream429 += attempt.Upstream429
		record.Upstream5xx += attempt.Upstream5xx
		record.UpstreamTimeouts += attempt.UpstreamTimeouts
		record.ClientCanceled += attempt.ClientCanceled
		record.RetryAfterMaxMS = maxInt64Slice([]int64{record.RetryAfterMaxMS, attempt.RetryAfterMaxMS})
	}
	percentiles, err := s.trafficTuningAttemptRetryAfterPercentilesSQL(opts, dim)
	if err != nil {
		return err
	}
	for key, percentile := range percentiles {
		record := records[key]
		if record == nil {
			continue
		}
		record.RetryAfterP50MS = maxInt64Slice([]int64{record.RetryAfterP50MS, percentile.P50})
		record.RetryAfterP95MS = maxInt64Slice([]int64{record.RetryAfterP95MS, percentile.P95})
		record.RetryAfterMaxMS = maxInt64Slice([]int64{record.RetryAfterMaxMS, percentile.Max})
	}
	return nil
}

func (s *usageStore) applyTrafficTuningUpstreamShapeAggSQL(records map[string]*trafficTuningSQLAggRecord, opts UsageReportOptions, dim trafficTuningDimensionSQL) error {
	var shapeRecords []trafficTuningSQLAggRecord
	q := s.db.Table("request_upstream_shape_events AS child").
		Joins("JOIN (?) AS u ON u.request_id = child.request_id", s.usageRowsQuery(opts)).
		Where("child.decision IN (?, ?, ?) AND child.backoff_reason = ?", shapeDecisionSkipped, shapeDecisionCooldownStarted, shapeDecisionRejected, "adaptive-backoff-provider-429")
	if opts.TrafficShapeScope != "" {
		q = q.Where("child.scope = ?", opts.TrafficShapeScope)
	}
	if opts.TrafficShapeBucket != "" {
		q = q.Where("child.bucket = ?", opts.TrafficShapeBucket)
	}
	if opts.TargetProvider != "" {
		q = q.Where("child.provider = ?", opts.TargetProvider)
	}
	if opts.TargetModel != "" {
		q = q.Where("child.model = ?", opts.TargetModel)
	}
	if opts.TargetDialect != "" {
		q = q.Where("child.dialect = ?", opts.TargetDialect)
	}
	if err := q.Select(dim.keyExpr + ` AS key,
		` + dim.secondaryExpr + ` AS secondary_key,
		COUNT(*) AS upstream429`).
		Group(dim.groupBy).
		Scan(&shapeRecords).Error; err != nil {
		return err
	}
	for _, shaped := range shapeRecords {
		record := records[joinKey(shaped.Key, shaped.SecondaryKey)]
		if record != nil {
			record.Upstream429 += shaped.Upstream429
		}
	}
	return nil
}

func (s *usageStore) trafficTuningPercentilesSQL(opts UsageReportOptions, dim trafficTuningDimensionSQL, valueExpr string) (map[string]trafficTuningPercentileRecord, error) {
	base := s.db.Table("(?) AS u", s.usageRowsQuery(opts)).
		Select(dim.keyExpr + " AS key, " + dim.secondaryExpr + " AS secondary_key, " + valueExpr + " AS value").
		Where(valueExpr + " > 0")
	return trafficTuningPercentilesFromBaseSQL(s.db, base)
}

func (s *usageStore) trafficTuningAttemptRetryAfterPercentilesSQL(opts UsageReportOptions, dim trafficTuningDimensionSQL) (map[string]trafficTuningPercentileRecord, error) {
	base := s.db.Table("request_attempts AS child").
		Joins("JOIN (?) AS u ON u.request_id = child.request_id", s.usageRowsQuery(opts)).
		Select(dim.keyExpr + " AS key, " + dim.secondaryExpr + " AS secondary_key, child.retry_after_ms AS value").
		Where("child.retry_after_ms > 0")
	if opts.TargetProvider != "" {
		base = base.Where("child.provider = ?", opts.TargetProvider)
	}
	if opts.TargetModel != "" {
		base = base.Where("child.model = ?", opts.TargetModel)
	}
	if opts.TargetDialect != "" {
		base = base.Where("child.dialect = ?", opts.TargetDialect)
	}
	return trafficTuningPercentilesFromBaseSQL(s.db, base)
}

func trafficTuningPercentilesFromBaseSQL(db *gorm.DB, base *gorm.DB) (map[string]trafficTuningPercentileRecord, error) {
	ranked := db.Table("(?) AS samples", base).
		Select(`key, secondary_key, value,
			ROW_NUMBER() OVER (PARTITION BY key, secondary_key ORDER BY value ASC) AS rn,
			COUNT(*) OVER (PARTITION BY key, secondary_key) AS cnt`)
	var records []trafficTuningPercentileRecord
	if err := db.Table("(?) AS ranked", ranked).Select(`key,
		secondary_key,
		MAX(CASE WHEN rn = ((50 * cnt + 99) / 100) THEN value ELSE 0 END) AS p50,
		MAX(CASE WHEN rn = ((95 * cnt + 99) / 100) THEN value ELSE 0 END) AS p95,
		MAX(value) AS max`).
		Group("key, secondary_key").
		Scan(&records).Error; err != nil {
		return nil, err
	}
	out := map[string]trafficTuningPercentileRecord{}
	for _, record := range records {
		out[joinKey(record.Key, record.SecondaryKey)] = record
	}
	return out, nil
}

func trafficTuningRecordsByKey(records []trafficTuningSQLAggRecord) map[string]*trafficTuningSQLAggRecord {
	out := map[string]*trafficTuningSQLAggRecord{}
	for i := range records {
		out[joinKey(records[i].Key, records[i].SecondaryKey)] = &records[i]
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
		AffectedUsers:         a.affectedUsers(),
		AffectedClients:       a.affectedClients(),
		Docs:                  trafficTuningDocsAnchor,
	}
	return applyTrafficTuningRecommendation(row, a.hardCaller429, a.affectedUsersOrDefault())
}

func applyTrafficTuningRecommendation(row TrafficTuningAdvisorRow, hardCaller429 int64, affectedUsersDefault int64) TrafficTuningAdvisorRow {
	row.ObservedValues = fmt.Sprintf("requests=%d errors=%d router429=%d shaped=%d rejected=%d queued=%d upstream400=%d upstream429=%d upstream5xx=%d timeouts=%d canceled=%d queue_p95_ms=%d retry_after_p95_ms=%d",
		row.Requests, row.TerminalErrors, row.Router429, row.TrafficShaped, row.Rejected, row.Queued, row.Upstream400, row.Upstream429, row.Upstream5xx, row.UpstreamTimeouts, row.ClientCanceled, row.QueueWaitP95MS, row.RetryAfterP95MS)

	errorRate := ratio(row.TerminalErrors, row.Requests)
	rejectRate := ratio(row.Rejected, row.Requests)
	queueRate := ratio(row.Queued, row.Requests)
	upstream400Rate := ratio(row.Upstream400, maxInt64Slice([]int64{row.Requests, 1}))
	upstream429Rate := ratio(row.Upstream429, maxInt64Slice([]int64{row.Requests, 1}))
	upstream5xxRate := ratio(row.Upstream5xx+row.UpstreamTimeouts, maxInt64Slice([]int64{row.Requests, 1}))
	cancelRate := ratio(row.ClientCanceled, maxInt64Slice([]int64{row.Requests, 1}))

	switch {
	case row.Requests == 0:
		row.Recommendation = "no_shaping_change_indicated"
		row.Severity = "info"
		row.Threshold = "no requests in selected bucket"
		row.ConfigFields = []string{"server.usage_db"}
		row.Explanation = "No matching request rows were available for this dimension."
	case upstream400Rate >= 0.10 && row.Rejected == 0 && row.Queued == 0:
		row.Recommendation = "route_around_incompatible_target"
		row.Severity = severityForRate(upstream400Rate)
		row.Threshold = "upstream 400 attempts >= 10% and caller shaping rejected/queued == 0"
		row.ConfigFields = []string{"models.<group>.targets[]", "provider catalog tool_support", "provider catalog input_modalities", "target request-shape eligibility"}
		row.Explanation = "Caller burst or queue tuning is unlikely to help because the router admitted traffic and upstream 400/request-shape failures dominate. Investigate the selected provider/model/dialect and route around incompatible targets."
	case ratio(hardCaller429, row.Requests) >= 0.02:
		row.Recommendation = "adjust_caller_quota_or_rate_limit"
		row.Severity = severityForRate(ratio(hardCaller429, row.Requests))
		row.Threshold = "hard caller admission 429 rate >= 2%"
		row.ConfigFields = []string{"callers[].rpm", "callers[].tpm", "callers[].concurrent", "callers[].quota", "callers[].key.lifetime_tokens"}
		row.Explanation = "The router rejected requests before traffic shaping or upstream attempts because hard caller limits or quotas were reached. Adjust caller rate/quota policy or client pacing; traffic-shaping burst and provider capacity tuning will not fix these rows."
	case upstream429Rate >= 0.05 && affectedUsersDefault > 1:
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
	case row.Rejected > 0 && row.QueueWaitP95MS > 0:
		row.Recommendation = "increase_queue_depth"
		row.Severity = severityForRate(rejectRate)
		row.Threshold = "traffic-shape rejections after observed queue waits"
		row.ConfigFields = []string{"callers[].traffic_shape.queue.max_depth", "server.traffic_shape.default_caller.queue.max_depth"}
		row.Explanation = "Short bursts are reaching a configured queue but still rejecting. Consider a small queue-depth increase if latency and upstream errors remain acceptable."
	case row.Rejected > 0 && row.Queued == 0:
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

func affectedUsersOrDefault(count int64) int64 {
	if count == 0 {
		return 1
	}
	return count
}

func (a *trafficTuningAgg) affectedUsersOrDefault() int64 {
	if a.affectedUsers() == 0 {
		return 1
	}
	return a.affectedUsers()
}

func (a *trafficTuningAgg) affectedUsers() int64 {
	if a.userCount > 0 {
		return a.userCount
	}
	return int64(len(a.users))
}

func (a *trafficTuningAgg) affectedClients() int64 {
	if a.clientCount > 0 {
		return a.clientCount
	}
	return int64(len(a.clients))
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
