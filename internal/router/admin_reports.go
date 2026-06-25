package router

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed all:admindist
var embeddedAdminReports embed.FS

type adminReportFilters struct {
	From time.Time
	To   time.Time
	UsageReportOptions
	Limit int
}

type adminReportResponse struct {
	Period       adminReportPeriod     `json:"period"`
	Summary      adminReportSummary    `json:"summary"`
	Series       []adminReportSeries   `json:"series"`
	ByToken      []adminReportTableRow `json:"byToken"`
	ByGroup      []adminReportTableRow `json:"byGroup"`
	ByProvider   []adminReportTableRow `json:"byProvider"`
	ByStatus     []adminReportTableRow `json:"byStatus"`
	Cache        adminReportCache      `json:"cache"`
	Requests     []adminReportRequest  `json:"requests"`
	GeneratedUTC string                `json:"generatedUtc"`
}

type adminReportPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type adminReportSummary struct {
	Requests         int64   `json:"requests"`
	Errors           int64   `json:"errors"`
	Tokens           int64   `json:"tokens"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CostUSD          float64 `json:"costUsd"`
	Attempts         int64   `json:"attempts"`
	Fallbacks        int64   `json:"fallbacks"`
	Streams          int64   `json:"streams"`
	AvgLatencyMS     int64   `json:"avgLatencyMs"`
	MaxLatencyMS     int64   `json:"maxLatencyMs"`
	AvgTTFBMS        int64   `json:"avgTtfbMs"`
	AvgUpstreamTPS   float64 `json:"avgUpstreamTokensPerSec"`
	AvgDownstreamTPS float64 `json:"avgDownstreamTokensPerSec"`
}

type adminReportCache struct {
	Hits          int64   `json:"hits"`
	Misses        int64   `json:"misses"`
	Bypass        int64   `json:"bypass"`
	HitRate       float64 `json:"hitRate"`
	LatestItems   int64   `json:"latestItems"`
	LatestBytes   int64   `json:"latestBytes"`
	MaxBytes      int64   `json:"maxBytes"`
	LatestPercent float64 `json:"latestPercent"`
}

type adminReportSeries struct {
	TimeUTC     string  `json:"timeUtc"`
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	CostUSD     float64 `json:"costUsd"`
	Tokens      int64   `json:"tokens"`
	LatencyMS   int64   `json:"latencyMs"`
	TTFBMS      int64   `json:"ttfbMs"`
	CacheHits   int64   `json:"cacheHits"`
	CacheMisses int64   `json:"cacheMisses"`
	CacheBypass int64   `json:"cacheBypass"`
	Fallbacks   int64   `json:"fallbacks"`
}

type adminReportTableRow struct {
	Key          string  `json:"key"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	Tokens       int64   `json:"tokens"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	Attempts     int64   `json:"attempts"`
	Fallbacks    int64   `json:"fallbacks"`
	AvgLatencyMS int64   `json:"avgLatencyMs"`
	MaxLatencyMS int64   `json:"maxLatencyMs"`
}

type adminReportRequest struct {
	TimeUTC      string  `json:"timeUtc"`
	RequestID    string  `json:"requestId"`
	CallerID     string  `json:"callerId"`
	CallerUser   string  `json:"callerUser"`
	Project      string  `json:"project"`
	Environment  string  `json:"environment"`
	TokenID      string  `json:"tokenId"`
	CallerIP     string  `json:"callerIp"`
	Client       string  `json:"client"`
	ModelGroup   string  `json:"modelGroup"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Dialect      string  `json:"dialect"`
	Status       int     `json:"status"`
	Error        string  `json:"error,omitempty"`
	Cache        string  `json:"cache"`
	Attempts     int     `json:"attempts"`
	Fallback     bool    `json:"fallback"`
	LatencyMS    int64   `json:"latencyMs"`
	TTFBMS       *int64  `json:"ttfbMs,omitempty"`
	UpstreamMS   *int64  `json:"upstreamMs,omitempty"`
	DownstreamMS *int64  `json:"downstreamMs,omitempty"`
	Tokens       int     `json:"tokens"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
}

type adminReportAttempt struct {
	RequestID        string `json:"requestId"`
	AttemptIndex     int    `json:"attemptIndex"`
	TimeUTC          string `json:"timeUtc"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Dialect          string `json:"dialect"`
	EndpointHost     string `json:"endpointHost"`
	DurationMS       int64  `json:"durationMs"`
	StatusCode       int    `json:"statusCode"`
	ErrorClass       string `json:"errorClass"`
	ErrorMessage     string `json:"errorMessage,omitempty"`
	Retryable        bool   `json:"retryable"`
	TimedOut         bool   `json:"timedOut"`
	ClientCanceled   bool   `json:"clientCanceled"`
	Selected         bool   `json:"selected"`
	FallbackReason   string `json:"fallbackReason,omitempty"`
	RequestBytes     int64  `json:"requestBytes"`
	ResponseBytes    int64  `json:"responseBytes"`
	AttemptTimeoutMS int    `json:"attemptTimeoutMs"`
}

type adminReportTraceEvent struct {
	RequestID  string `json:"requestId"`
	Seq        int    `json:"seq"`
	TimeUTC    string `json:"timeUtc"`
	Event      string `json:"event"`
	Message    string `json:"message,omitempty"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dialect    string `json:"dialect"`
	DurationMS int64  `json:"durationMs"`
	StatusCode int    `json:"statusCode"`
	ErrorClass string `json:"errorClass"`
	Retryable  bool   `json:"retryable"`
	Attempt    int    `json:"attempt"`
}

type adminReportError struct {
	RequestID    string `json:"requestId"`
	TimeUTC      string `json:"timeUtc"`
	Status       int    `json:"status"`
	ErrorType    string `json:"errorType"`
	ErrorClass   string `json:"errorClass"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	Retryable    bool   `json:"retryable"`
	Attempts     int    `json:"attempts"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Dialect      string `json:"dialect"`
}

func (s *Service) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.AdminReports.Enabled {
		http.NotFound(w, r)
		return
	}
	if s.adminReportBearerForbidden(w, r) {
		return
	}
	subject, ok := s.authenticateAdminBasic(w, r)
	if !ok {
		return
	}
	action := "read"
	if strings.HasSuffix(r.URL.Path, "/export.md") {
		action = "export"
	}
	if !s.authorizeAdmin(subject, "admin:reports", action) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "reports-forbidden", "message": "reports-forbidden"}})
		return
	}
	s.setAdminReportHeaders(w, strings.HasPrefix(r.URL.Path, cleanAdminReportsPrefix(s.cfg.Server.AdminReports.PathPrefix)+"/static/"))
	prefix := cleanAdminReportsPrefix(s.cfg.Server.AdminReports.PathPrefix)
	switch {
	case r.URL.Path == prefix:
		http.Redirect(w, r, prefix+"/", http.StatusTemporaryRedirect)
	case r.URL.Path == prefix+"/" || r.URL.Path == prefix+"/usage":
		s.serveAdminReportAsset(w, r, "index.html")
	case strings.HasPrefix(r.URL.Path, prefix+"/static/"):
		s.serveAdminReportAsset(w, r, strings.TrimPrefix(r.URL.Path, prefix+"/"))
	case r.URL.Path == prefix+"/api/summary":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportSummary(w, r)
	case r.URL.Path == prefix+"/api/requests":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportRequests(w, r)
	case strings.HasPrefix(r.URL.Path, prefix+"/api/request/"):
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportRequestDetail(w, r, strings.TrimPrefix(r.URL.Path, prefix+"/api/request/"))
	case r.URL.Path == prefix+"/export.md":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportMarkdown(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) requireAdminReportUsageStore(w http.ResponseWriter) bool {
	if s.usage != nil {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"type": "reports-disabled", "message": "reports-disabled"}})
	return false
}

func (s *Service) adminReportBearerForbidden(w http.ResponseWriter, r *http.Request) bool {
	hasBearer := strings.HasPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	hasAPIKey := strings.TrimSpace(r.Header.Get("X-API-Key")) != ""
	if !hasBearer && !hasAPIKey {
		return false
	}
	caller, _, err := s.authenticate(r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))
	if err != nil || caller == nil {
		return false
	}
	s.setAdminReportHeaders(w, false)
	writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "reports-forbidden", "message": "reports-forbidden"}})
	return true
}

func (s *Service) authorizeAdmin(subject adminBasicRuntime, object, action string) bool {
	if s.adminAuthorizer != nil && s.adminAuthorizer.enforce(subject, object, action) {
		return true
	}
	return false
}

func (s *Service) setAdminReportHeaders(w http.ResponseWriter, static bool) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	if static {
		w.Header().Set("Cache-Control", "private, max-age=3600")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func (s *Service) serveAdminReportAsset(w http.ResponseWriter, r *http.Request, name string) {
	sub, err := fs.Sub(embeddedAdminReports, "admindist")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !serveEmbeddedDoc(w, r, sub, path.Clean("/" + name)[1:]) {
		http.NotFound(w, r)
	}
}

func (s *Service) handleAdminReportSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := s.parseAdminReportFilters(w, r, false)
	if !ok {
		return
	}
	rows, err := s.usage.rows(filters.UsageReportOptions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	writeJSON(w, http.StatusOK, buildAdminReportResponse(filters, rows))
}

func (s *Service) handleAdminReportRequests(w http.ResponseWriter, r *http.Request) {
	filters, ok := s.parseAdminReportFilters(w, r, true)
	if !ok {
		return
	}
	rows, err := s.usage.rows(filters.UsageReportOptions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TS.After(rows[j].TS) })
	if len(rows) > filters.Limit {
		rows = rows[:filters.Limit]
	}
	resp := buildAdminReportResponse(filters, rows)
	resp.ByToken = nil
	resp.ByGroup = nil
	resp.ByProvider = nil
	resp.ByStatus = nil
	resp.Series = nil
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleAdminReportRequestDetail(w http.ResponseWriter, r *http.Request, requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
		return
	}
	var usage usageRecord
	if err := s.usage.db.Where("request_id = ?", requestID).First(&usage).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	row, err := rowFromUsageRecord(usage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	var attempts []requestAttemptRecord
	var traces []requestTraceEventRecord
	var errors []requestErrorRecord
	_ = s.usage.db.Where("request_id = ?", requestID).Order("attempt_index ASC").Find(&attempts).Error
	_ = s.usage.db.Where("request_id = ?", requestID).Order("seq ASC").Find(&traces).Error
	_ = s.usage.db.Where("request_id = ?", requestID).Find(&errors).Error
	writeJSON(w, http.StatusOK, map[string]any{
		"request":  adminRequestFromRow(row),
		"attempts": adminAttemptsFromRecords(attempts),
		"trace":    adminTraceFromRecords(traces),
		"errors":   adminErrorsFromRecords(errors),
	})
}

func (s *Service) handleAdminReportMarkdown(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.AdminReports.ExportMarkdown {
		http.NotFound(w, r)
		return
	}
	filters, ok := s.parseAdminReportFilters(w, r, false)
	if !ok {
		return
	}
	rows, err := s.usage.rows(filters.UsageReportOptions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(renderUsageMarkdown(filters.From, filters.To, rows)))
}

func (s *Service) parseAdminReportFilters(w http.ResponseWriter, r *http.Request, withLimit bool) (adminReportFilters, bool) {
	to := time.Now().UTC()
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
			return adminReportFilters{}, false
		}
		to = parsed.UTC()
	}
	sinceText := defaultString(q.Get("since"), s.cfg.Server.AdminReports.DefaultSince)
	since, err := parseReportDuration(sinceText)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
		return adminReportFilters{}, false
	}
	from := to.Add(-since)
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
			return adminReportFilters{}, false
		}
		from = parsed.UTC()
	}
	maxRange, _ := parseReportDuration(s.cfg.Server.AdminReports.MaxRange)
	if !from.Before(to) || to.Sub(from) > maxRange {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
		return adminReportFilters{}, false
	}
	limit := s.cfg.Server.AdminReports.MaxRows
	if withLimit && strings.TrimSpace(q.Get("limit")) != "" {
		parsed, err := strconv.Atoi(q.Get("limit"))
		if err != nil || parsed <= 0 || parsed > s.cfg.Server.AdminReports.MaxRows {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid-report-filter"}})
			return adminReportFilters{}, false
		}
		limit = parsed
	}
	opts := UsageReportOptions{
		From:              from,
		To:                to,
		TokenID:           q.Get("token_id"),
		TokenIDPrefix:     q.Get("token_id_prefix"),
		CallerUser:        q.Get("caller_user"),
		CallerProject:     q.Get("caller_project"),
		CallerEnvironment: q.Get("caller_environment"),
		ResolvedGroup:     q.Get("resolved_group"),
		Client:            q.Get("client"),
	}
	return adminReportFilters{From: from, To: to, UsageReportOptions: opts, Limit: limit}, true
}

func buildAdminReportResponse(filters adminReportFilters, rows []usageRow) adminReportResponse {
	total := &agg{}
	byHour := map[string]*agg{}
	byToken := map[string]*agg{}
	byGroup := map[string]*agg{}
	byProvider := map[string]*agg{}
	byStatus := map[string]*agg{}
	for _, row := range rows {
		total.add(row)
		addAdminAgg(byHour, row.TS.UTC().Truncate(time.Hour).Format(time.RFC3339), row)
		addAdminAgg(byToken, defaultString(row.TokenID, "unknown"), row)
		addAdminAgg(byGroup, defaultString(row.ResolvedGroup, row.RequestedModel), row)
		addAdminAgg(byProvider, joinKey(defaultString(row.TargetProvider, "unknown"), defaultString(row.TargetModel, "unknown")), row)
		addAdminAgg(byStatus, strconv.Itoa(row.Status), row)
	}
	return adminReportResponse{
		Period:       adminReportPeriod{From: formatUsageTime(filters.From), To: formatUsageTime(filters.To)},
		Summary:      adminSummaryFromAgg(total),
		Series:       adminSeriesFromAgg(byHour),
		ByToken:      adminRowsFromAgg(byToken),
		ByGroup:      adminRowsFromAgg(byGroup),
		ByProvider:   adminRowsFromAgg(byProvider),
		ByStatus:     adminRowsFromAgg(byStatus),
		Cache:        adminCacheFromAgg(total),
		Requests:     adminRecentRequestsFromRows(rows, filters.Limit),
		GeneratedUTC: formatUsageTime(time.Now().UTC()),
	}
}

func adminRecentRequestsFromRows(rows []usageRow, limit int) []adminReportRequest {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	recent := append([]usageRow(nil), rows...)
	sort.Slice(recent, func(i, j int) bool { return recent[i].TS.After(recent[j].TS) })
	if len(recent) > limit {
		recent = recent[:limit]
	}
	requests := make([]adminReportRequest, 0, len(recent))
	for _, row := range recent {
		requests = append(requests, adminRequestFromRow(row))
	}
	return requests
}

func addAdminAgg(m map[string]*agg, key string, row usageRow) {
	a := getAgg(m, key)
	a.add(row)
}

func adminSummaryFromAgg(a *agg) adminReportSummary {
	return adminReportSummary{
		Requests:         a.Calls,
		Errors:           a.Errors,
		Tokens:           a.TotalTokens,
		InputTokens:      a.InputTokens,
		OutputTokens:     a.OutputTokens,
		CostUSD:          a.TotalCostUSD,
		Attempts:         a.Attempts,
		Fallbacks:        a.Fallbacks,
		Streams:          a.Streams,
		AvgLatencyMS:     avg(a.LatencyMS, a.Calls),
		MaxLatencyMS:     a.MaxLatencyMS,
		AvgTTFBMS:        avg(a.TTFBMS, a.TTFBCount),
		AvgUpstreamTPS:   avgFloat(a.UpstreamOutputTPS, a.UpstreamOutputTPSCount),
		AvgDownstreamTPS: avgFloat(a.DownstreamOutputTPS, a.DownstreamOutputTPSCount),
	}
}

func adminCacheFromAgg(a *agg) adminReportCache {
	cacheable := a.CacheHits + a.CacheMisses
	return adminReportCache{
		Hits:          a.CacheHits,
		Misses:        a.CacheMisses,
		Bypass:        a.CacheBypass,
		HitRate:       ratioPct(a.CacheHits, cacheable),
		LatestItems:   a.CacheItemsLatest,
		LatestBytes:   a.CacheBytesLatest,
		MaxBytes:      a.CacheMaxBytesLatest,
		LatestPercent: a.CacheOccupancyLatest,
	}
}

func adminRowsFromAgg(data map[string]*agg) []adminReportTableRow {
	out := make([]adminReportTableRow, 0, len(data))
	for _, key := range sortedAggKeys(data) {
		a := data[key]
		out = append(out, adminReportTableRow{Key: key, Requests: a.Calls, Errors: a.Errors, Tokens: a.TotalTokens, InputTokens: a.InputTokens, OutputTokens: a.OutputTokens, CostUSD: a.TotalCostUSD, Attempts: a.Attempts, Fallbacks: a.Fallbacks, AvgLatencyMS: avg(a.LatencyMS, a.Calls), MaxLatencyMS: a.MaxLatencyMS})
	}
	return out
}

func adminSeriesFromAgg(data map[string]*agg) []adminReportSeries {
	keys := sortedAggKeys(data)
	sort.Strings(keys)
	out := make([]adminReportSeries, 0, len(keys))
	for _, key := range keys {
		a := data[key]
		out = append(out, adminReportSeries{TimeUTC: key, Requests: a.Calls, Errors: a.Errors, CostUSD: a.TotalCostUSD, Tokens: a.TotalTokens, LatencyMS: avg(a.LatencyMS, a.Calls), TTFBMS: avg(a.TTFBMS, a.TTFBCount), CacheHits: a.CacheHits, CacheMisses: a.CacheMisses, CacheBypass: a.CacheBypass, Fallbacks: a.Fallbacks})
	}
	return out
}

func adminRequestFromRow(row usageRow) adminReportRequest {
	return adminReportRequest{
		TimeUTC:      formatUsageTime(row.TS),
		RequestID:    row.RequestID,
		CallerID:     row.CallerID,
		CallerUser:   row.CallerUser,
		Project:      row.CallerProject,
		Environment:  row.CallerEnvironment,
		TokenID:      row.TokenID,
		CallerIP:     row.CallerIP,
		Client:       row.Client,
		ModelGroup:   defaultString(row.ResolvedGroup, row.RequestedModel),
		Provider:     row.TargetProvider,
		Model:        row.TargetModel,
		Dialect:      row.TargetDialect,
		Status:       row.Status,
		Error:        sanitizePersistedDiagnosticText(row.Error),
		Cache:        row.Cache,
		Attempts:     row.Attempts,
		Fallback:     row.FallbackUsed,
		LatencyMS:    row.LatencyMS,
		TTFBMS:       row.TTFBMS,
		UpstreamMS:   row.UpstreamMS,
		DownstreamMS: row.DownstreamMS,
		Tokens:       totalTokens(Usage{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, TotalTokens: row.TotalTokens}),
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		CostUSD:      row.TotalCostUSD,
	}
}

func adminAttemptsFromRecords(records []requestAttemptRecord) []adminReportAttempt {
	out := make([]adminReportAttempt, 0, len(records))
	for _, record := range records {
		out = append(out, adminReportAttempt{
			RequestID:        record.RequestID,
			AttemptIndex:     record.AttemptIndex,
			TimeUTC:          record.TS,
			Provider:         record.Provider,
			Model:            record.Model,
			Dialect:          record.Dialect,
			EndpointHost:     record.EndpointHost,
			DurationMS:       record.DurationMS,
			StatusCode:       record.StatusCode,
			ErrorClass:       record.ErrorClass,
			ErrorMessage:     sanitizePersistedDiagnosticText(record.ErrorMessage),
			Retryable:        record.Retryable,
			TimedOut:         record.TimedOut,
			ClientCanceled:   record.ClientCanceled,
			Selected:         record.Selected,
			FallbackReason:   record.FallbackReason,
			RequestBytes:     record.RequestBytes,
			ResponseBytes:    record.ResponseBytes,
			AttemptTimeoutMS: record.AttemptTimeoutMS,
		})
	}
	return out
}

func adminTraceFromRecords(records []requestTraceEventRecord) []adminReportTraceEvent {
	out := make([]adminReportTraceEvent, 0, len(records))
	for _, record := range records {
		out = append(out, adminReportTraceEvent{
			RequestID:  record.RequestID,
			Seq:        record.Seq,
			TimeUTC:    record.TS,
			Event:      record.Event,
			Message:    sanitizePersistedDiagnosticText(record.Message),
			Provider:   record.Provider,
			Model:      record.Model,
			Dialect:    record.Dialect,
			DurationMS: record.DurationMS,
			StatusCode: record.StatusCode,
			ErrorClass: record.ErrorClass,
			Retryable:  record.Retryable,
			Attempt:    record.Attempt,
		})
	}
	return out
}

func adminErrorsFromRecords(records []requestErrorRecord) []adminReportError {
	out := make([]adminReportError, 0, len(records))
	for _, record := range records {
		out = append(out, adminReportError{
			RequestID:    record.RequestID,
			TimeUTC:      record.TS,
			Status:       record.Status,
			ErrorType:    record.ErrorType,
			ErrorClass:   record.ErrorClass,
			ErrorMessage: sanitizePersistedDiagnosticText(record.ErrorMessage),
			Retryable:    record.Retryable,
			Attempts:     record.Attempts,
			Provider:     record.Provider,
			Model:        record.Model,
			Dialect:      record.Dialect,
		})
	}
	return out
}
