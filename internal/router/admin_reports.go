package router

import (
	"embed"
	"encoding/csv"
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
	Charts       []adminReportChart    `json:"charts"`
	ByToken      []adminReportTableRow `json:"byToken"`
	ByGroup      []adminReportTableRow `json:"byGroup"`
	ByProvider   []adminReportTableRow `json:"byProvider"`
	ByStatus     []adminReportTableRow `json:"byStatus"`
	Cache        adminReportCache      `json:"cache"`
	Requests     []adminReportRequest  `json:"requests"`
	GeneratedUTC string                `json:"generatedUtc"`
}

type adminSavingsResponse struct {
	Period       adminReportPeriod         `json:"period"`
	Baseline     adminSavingsBaselineDTO   `json:"baseline"`
	Baselines    []adminSavingsBaselineDTO `json:"baselines"`
	Summary      adminSavingsRow           `json:"summary"`
	ByTime       []adminSavingsRow         `json:"byTime"`
	ByGroup      []adminSavingsRow         `json:"byGroup"`
	Charts       []adminReportChart        `json:"charts"`
	Warnings     []string                  `json:"warnings,omitempty"`
	GeneratedUTC string                    `json:"generatedUtc"`
}

type adminScalarReportResponse struct {
	Period       adminReportPeriod        `json:"period"`
	Report       string                   `json:"report"`
	Baseline     *adminSavingsBaselineDTO `json:"baseline,omitempty"`
	Summary      adminReportSummary       `json:"summary"`
	Rows         []adminScalarReportRow   `json:"rows"`
	Charts       []adminReportChart       `json:"charts,omitempty"`
	Requests     []adminReportRequest     `json:"requests,omitempty"`
	GeneratedUTC string                   `json:"generatedUtc"`
}

type adminSecurityReportResponse struct {
	Period       adminReportPeriod       `json:"period"`
	Report       string                  `json:"report"`
	Summary      adminSecuritySummary    `json:"summary"`
	Rows         []adminSecurityEventRow `json:"rows"`
	Charts       []adminReportChart      `json:"charts,omitempty"`
	GeneratedUTC string                  `json:"generatedUtc"`
}

type adminSecuritySummary struct {
	Events       int64 `json:"events"`
	Allowed      int64 `json:"allowed"`
	Unauthorized int64 `json:"unauthorized"`
	Forbidden    int64 `json:"forbidden"`
	Denied       int64 `json:"denied"`
	Errors       int64 `json:"errors"`
	UniqueIPs    int64 `json:"uniqueIps"`
}

type adminSecurityEventRow struct {
	TimeUTC             string `json:"timeUtc"`
	RequestID           string `json:"requestId,omitempty"`
	EventType           string `json:"eventType"`
	Surface             string `json:"surface"`
	Method              string `json:"method"`
	Path                string `json:"path"`
	Status              int    `json:"status"`
	Outcome             string `json:"outcome"`
	Reason              string `json:"reason"`
	AuthSubject         string `json:"authSubject,omitempty"`
	AuthSource          string `json:"authSource"`
	CallerID            string `json:"callerId,omitempty"`
	CallerUser          string `json:"callerUser,omitempty"`
	Project             string `json:"project,omitempty"`
	TokenID             string `json:"tokenId,omitempty"`
	AdminSubject        string `json:"adminSubject,omitempty"`
	Client              string `json:"client,omitempty"`
	UserAgentFamily     string `json:"userAgentFamily,omitempty"`
	IPAddress           string `json:"ipAddress,omitempty"`
	IPSource            string `json:"ipSource,omitempty"`
	TrustedProxyApplied bool   `json:"trustedProxyApplied"`
	PrivateIP           bool   `json:"privateIp"`
	LoopbackIP          bool   `json:"loopbackIp"`
	ReservedIP          bool   `json:"reservedIp"`
	ModelGroup          string `json:"modelGroup,omitempty"`
	RequestedModel      string `json:"requestedModel,omitempty"`
	ResolvedGroup       string `json:"resolvedGroup,omitempty"`
	InputTokens         int    `json:"inputTokens"`
	OutputTokens        int    `json:"outputTokens"`
	TotalTokens         int    `json:"totalTokens"`
}

type adminScalarReportRow struct {
	Key                                  string   `json:"key"`
	SecondaryKey                         string   `json:"secondaryKey,omitempty"`
	Requests                             int64    `json:"requests"`
	Errors                               int64    `json:"errors"`
	ErrorRatePct                         float64  `json:"errorRatePct"`
	Streams                              int64    `json:"streams"`
	Attempts                             int64    `json:"attempts"`
	Fallbacks                            int64    `json:"fallbacks"`
	FallbackRatePct                      float64  `json:"fallbackRatePct"`
	CacheHits                            int64    `json:"cacheHits"`
	CacheMisses                          int64    `json:"cacheMisses"`
	CacheBypass                          int64    `json:"cacheBypass"`
	CacheHitRatePct                      float64  `json:"cacheHitRatePct"`
	InputTokens                          int64    `json:"inputTokens"`
	OutputTokens                         int64    `json:"outputTokens"`
	Tokens                               int64    `json:"tokens"`
	TotalTokens                          int64    `json:"totalTokens"`
	InputImageCount                      int64    `json:"inputImageCount"`
	InputImageTokens                     int64    `json:"inputImageTokens"`
	PIIFilteredRequests                  int64    `json:"piiFilteredRequests"`
	PIIFilterReplacements                int64    `json:"piiFilterReplacements"`
	CostUSD                              float64  `json:"costUsd"`
	InputCostUSD                         float64  `json:"inputCostUsd"`
	ImageCostUSD                         float64  `json:"imageCostUsd"`
	OutputCostUSD                        float64  `json:"outputCostUsd"`
	TotalCostUSD                         float64  `json:"totalCostUsd"`
	UpstreamReportedCostUSD              float64  `json:"upstreamReportedCostUsd"`
	BaselineCostUSD                      *float64 `json:"baselineCostUsd,omitempty"`
	SavingsUSD                           *float64 `json:"savingsUsd,omitempty"`
	SavingsPct                           *float64 `json:"savingsPct,omitempty"`
	AvgCostUSD                           float64  `json:"avgCostUsd"`
	AvgLatencyMS                         int64    `json:"avgLatencyMs"`
	MaxLatencyMS                         int64    `json:"maxLatencyMs"`
	AvgTTFBMS                            int64    `json:"avgTtfbMs"`
	MaxTTFBMS                            int64    `json:"maxTtfbMs"`
	AvgUpstreamMS                        int64    `json:"avgUpstreamMs"`
	MaxUpstreamMS                        int64    `json:"maxUpstreamMs"`
	AvgDownstreamMS                      int64    `json:"avgDownstreamMs"`
	MaxDownstreamMS                      int64    `json:"maxDownstreamMs"`
	AvgUpstreamTokensPerSec              float64  `json:"avgUpstreamTokensPerSec"`
	AvgUpstreamOutputTokensPerSec        float64  `json:"avgUpstreamOutputTokensPerSec"`
	AvgUpstreamTotalTokensPerSec         float64  `json:"avgUpstreamTotalTokensPerSec"`
	AvgDownstreamTokensPerSec            float64  `json:"avgDownstreamTokensPerSec"`
	AvgDownstreamWriteOutputTokensPerSec float64  `json:"avgDownstreamWriteOutputTokensPerSec"`
	AvgDownstreamWriteTotalTokensPerSec  float64  `json:"avgDownstreamWriteTotalTokensPerSec"`
	LatestCacheItems                     int64    `json:"latestCacheItems"`
	LatestCacheBytes                     int64    `json:"latestCacheBytes"`
	LatestCacheMaxBytes                  int64    `json:"latestCacheMaxBytes"`
	LatestCacheOccupancyPct              float64  `json:"latestCacheOccupancyPct"`
}

type adminSavingsBaselineDTO struct {
	BaselineID                       string  `json:"baseline_id"`
	BaselineName                     string  `json:"baseline_name"`
	PricingSource                    string  `json:"pricing_source"`
	PricingUpdatedAt                 string  `json:"pricing_updated_at"`
	BaselineInputPricePerMillionUSD  float64 `json:"baseline_input_price_per_million_usd"`
	BaselineOutputPricePerMillionUSD float64 `json:"baseline_output_price_per_million_usd"`
	Notes                            string  `json:"notes,omitempty"`
	Custom                           bool    `json:"custom,omitempty"`
}

type adminSavingsRow struct {
	Key             string  `json:"key"`
	Requests        int64   `json:"requests"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	ActualCostUSD   float64 `json:"actual_cost_usd"`
	BaselineCostUSD float64 `json:"baseline_cost_usd"`
	SavingsUSD      float64 `json:"savings_usd"`
	SavingsPct      float64 `json:"savings_pct"`
}

type adminReportPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type adminReportSummary struct {
	Requests                    int64   `json:"requests"`
	Errors                      int64   `json:"errors"`
	Tokens                      int64   `json:"tokens"`
	TotalTokens                 int64   `json:"totalTokens"`
	InputTokens                 int64   `json:"inputTokens"`
	OutputTokens                int64   `json:"outputTokens"`
	CostUSD                     float64 `json:"costUsd"`
	InputCostUSD                float64 `json:"inputCostUsd"`
	ImageCostUSD                float64 `json:"imageCostUsd"`
	OutputCostUSD               float64 `json:"outputCostUsd"`
	TotalCostUSD                float64 `json:"totalCostUsd"`
	Attempts                    int64   `json:"attempts"`
	Fallbacks                   int64   `json:"fallbacks"`
	Streams                     int64   `json:"streams"`
	AvgLatencyMS                int64   `json:"avgLatencyMs"`
	MaxLatencyMS                int64   `json:"maxLatencyMs"`
	AvgTTFBMS                   int64   `json:"avgTtfbMs"`
	AvgUpstreamTPS              float64 `json:"avgUpstreamTokensPerSec"`
	AvgUpstreamOutputTPS        float64 `json:"avgUpstreamOutputTokensPerSec"`
	AvgUpstreamTotalTPS         float64 `json:"avgUpstreamTotalTokensPerSec"`
	AvgDownstreamTPS            float64 `json:"avgDownstreamTokensPerSec"`
	AvgDownstreamWriteOutputTPS float64 `json:"avgDownstreamWriteOutputTokensPerSec"`
	AvgDownstreamWriteTotalTPS  float64 `json:"avgDownstreamWriteTotalTokensPerSec"`
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

type adminReportChart struct {
	ChartID     string                   `json:"chart_id"`
	Title       string                   `json:"title"`
	XAxis       adminReportChartAxis     `json:"x_axis"`
	YAxis       adminReportChartAxis     `json:"y_axis"`
	Series      []adminReportChartSeries `json:"series"`
	GeneratedAt string                   `json:"generated_at"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	Filters     adminReportFilterDTO     `json:"filters"`
}

type adminReportChartAxis struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	Unit  string `json:"unit,omitempty"`
}

type adminReportChartSeries struct {
	Name     string                  `json:"name"`
	Unit     string                  `json:"unit"`
	ColorKey string                  `json:"color_key"`
	Points   []adminReportChartPoint `json:"points"`
}

type adminReportChartPoint struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
}

type adminReportFilterDTO struct {
	TokenID           string `json:"token_id,omitempty"`
	TokenIDPrefix     string `json:"token_id_prefix,omitempty"`
	CallerUser        string `json:"caller_user,omitempty"`
	CallerProject     string `json:"caller_project,omitempty"`
	CallerEnvironment string `json:"caller_environment,omitempty"`
	ResolvedGroup     string `json:"resolved_group,omitempty"`
	Client            string `json:"client,omitempty"`
}

type adminReportTableRow struct {
	Key           string  `json:"key"`
	Requests      int64   `json:"requests"`
	Errors        int64   `json:"errors"`
	Tokens        int64   `json:"tokens"`
	TotalTokens   int64   `json:"totalTokens"`
	InputTokens   int64   `json:"inputTokens"`
	OutputTokens  int64   `json:"outputTokens"`
	CostUSD       float64 `json:"costUsd"`
	InputCostUSD  float64 `json:"inputCostUsd"`
	ImageCostUSD  float64 `json:"imageCostUsd"`
	OutputCostUSD float64 `json:"outputCostUsd"`
	TotalCostUSD  float64 `json:"totalCostUsd"`
	Attempts      int64   `json:"attempts"`
	Fallbacks     int64   `json:"fallbacks"`
	AvgLatencyMS  int64   `json:"avgLatencyMs"`
	MaxLatencyMS  int64   `json:"maxLatencyMs"`
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
	TotalTokens  int     `json:"totalTokens"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	TotalCostUSD float64 `json:"totalCostUsd"`
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

type adminScalarEndpointSpec struct {
	Report       string
	Dimension    string
	Secondary    string
	Sort         string
	Requests     bool
	Anomalies    bool
	WithBaseline bool
}

func (s *Service) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.AdminReports.Enabled {
		http.NotFound(w, r)
		return
	}
	if s.adminReportBearerForbidden(w, r) {
		return
	}
	prefix := cleanAdminReportsPrefix(s.cfg.Server.AdminReports.PathPrefix)
	relPath := strings.TrimPrefix(r.URL.Path, prefix)
	securityReport := strings.HasPrefix(relPath, "/api/security/") || strings.HasPrefix(relPath, "/security/")
	subject, ok := s.authenticateAdminSubject(w, r)
	if !ok {
		s.recordAdminSecurityAccess(r, adminAuthSubject{}, http.StatusUnauthorized, "unauthorized", authzObjectAdminReports, authzActionRead)
		return
	}
	action := "read"
	if strings.HasSuffix(r.URL.Path, "/export.md") || strings.HasSuffix(r.URL.Path, "/export.csv") {
		action = "export"
	}
	if strings.HasPrefix(relPath, "/api/request/") {
		action = authzActionDrilldown
	}
	object := authzObjectAdminReports
	if securityReport {
		object = authzObjectSecurityReports
	}
	if !s.authorizeAdmin(subject, object, action) {
		s.recordAdminSecurityAccess(r, subject, http.StatusForbidden, "reports-forbidden", object, action)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "reports-forbidden", "message": "reports-forbidden"}})
		return
	}
	s.recordAdminSecurityAccess(r, subject, http.StatusOK, "", object, action)
	s.setAdminReportHeaders(w, strings.HasPrefix(r.URL.Path, cleanAdminReportsPrefix(s.cfg.Server.AdminReports.PathPrefix)+"/static/"))
	scalarSpec, scalarOK := adminScalarEndpointSpecs(strings.TrimPrefix(r.URL.Path, prefix))
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
	case r.URL.Path == prefix+"/api/overview":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportSummary(w, r)
	case r.URL.Path == prefix+"/api/savings":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminReportSavings(w, r)
	case scalarOK:
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		s.handleAdminScalarEndpoint(w, r, scalarSpec)
	case r.URL.Path == prefix+"/api/security/events" || r.URL.Path == prefix+"/api/security/overview":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		if !s.requireAdminSecurityReports(w) {
			return
		}
		s.handleAdminSecurityEvents(w, r)
	case r.URL.Path == prefix+"/security/export.csv":
		if !s.requireAdminReportUsageStore(w) {
			return
		}
		if !s.requireAdminSecurityReports(w) {
			return
		}
		s.handleAdminSecurityCSV(w, r)
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

func adminScalarEndpointSpecs(path string) (adminScalarEndpointSpec, bool) {
	specs := map[string]adminScalarEndpointSpec{
		"/api/savings-by-user":      {Report: "savings-by-user", Dimension: "caller_user", Sort: "savings", WithBaseline: true},
		"/api/savings-by-key":       {Report: "savings-by-key", Dimension: "token_id", Sort: "savings", WithBaseline: true},
		"/api/savings-by-group":     {Report: "savings-by-group", Dimension: "model_group", Sort: "savings", WithBaseline: true},
		"/api/model-groups-by-user": {Report: "model-groups-by-user", Dimension: "caller_user", Secondary: "model_group", Sort: "requests"},
		"/api/usage-by-key":         {Report: "usage-by-key", Dimension: "token_id", Sort: "cost"},
		"/api/provider-model-mix":   {Report: "provider-model-mix", Dimension: "provider_model", Secondary: "dialect", Sort: "tokens"},
		"/api/latency-throughput":   {Report: "latency-throughput", Dimension: "provider_model", Secondary: "client", Sort: "latency"},
		"/api/errors-fallbacks":     {Report: "errors-fallbacks", Dimension: "status_error", Secondary: "provider_model", Sort: "errors"},
		"/api/cache":                {Report: "cache", Dimension: "cache", Secondary: "model_group", Sort: "requests"},
		"/api/quotas-budgets":       {Report: "quotas-budgets", Dimension: "quota_key_state", Secondary: "token_id", Sort: "requests"},
		"/api/routing-decisions":    {Report: "routing-decisions", Dimension: "routing_decision", Secondary: "provider_model", Sort: "requests"},
		"/api/contract-buckets":     {Report: "contract-buckets", Dimension: "contract_bucket", Secondary: "model_group", Sort: "requests"},
		"/api/contract-workloads":   {Report: "contract-workloads", Dimension: "contract_workload", Secondary: "model_group", Sort: "requests"},
		"/api/target-validation":    {Report: "target-validation", Dimension: "target_validation", Secondary: "provider_model", Sort: "requests"},
		"/api/expensive-requests":   {Report: "expensive-requests", Sort: "cost", Requests: true},
		"/api/client-breakdown":     {Report: "client-breakdown", Dimension: "client", Secondary: "inbound_dialect", Sort: "requests"},
		"/api/project-chargeback":   {Report: "project-chargeback", Dimension: "project", Secondary: "environment", Sort: "cost"},
		"/api/capability-usage":     {Report: "capability-usage", Dimension: "capability", Secondary: "model_group", Sort: "image"},
		"/api/anomalies":            {Report: "anomalies", Dimension: "anomaly", Secondary: "provider_model", Sort: "requests", Anomalies: true},
	}
	spec, ok := specs[path]
	return spec, ok
}

func (s *Service) requireAdminReportUsageStore(w http.ResponseWriter) bool {
	if s.usage != nil {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"type": "reports-disabled", "message": "reports-disabled"}})
	return false
}

func (s *Service) requireAdminSecurityReports(w http.ResponseWriter) bool {
	if s.cfg.Server.AdminReports.Security.Enabled {
		return true
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"type": "security-reports-disabled", "message": "security-reports-disabled"}})
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
	subject := authzSubjectForCaller(caller)
	s.recordAdminSecurityAccess(r, adminAuthSubject{subject: subject.subject, domain: subject.domain, source: "caller_token"}, http.StatusForbidden, "reports-forbidden", authzObjectAdminReports, authzActionRead)
	writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "reports-forbidden", "message": "reports-forbidden"}})
	return true
}

func (s *Service) authorizeAdmin(subject adminAuthSubject, object, action string) bool {
	return s.authorizer.enforce(authzSubjectForAdmin(subject), object, action)
}

func (s *Service) authorizeCaller(caller *callerRuntime, object, action string) bool {
	return s.authorizer.enforce(authzSubjectForCaller(caller), object, action)
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

func (s *Service) handleAdminReportSavings(w http.ResponseWriter, r *http.Request) {
	filters, ok := s.parseAdminReportFilters(w, r, false)
	if !ok {
		return
	}
	baseline, ok := s.parseAdminSavingsBaseline(w, r)
	if !ok {
		return
	}
	rows, err := s.usage.rows(filters.UsageReportOptions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	writeJSON(w, http.StatusOK, buildAdminSavingsResponse(filters, rows, baseline, s.adminSavingsBaselines()))
}

func (s *Service) handleAdminScalarEndpoint(w http.ResponseWriter, r *http.Request, spec adminScalarEndpointSpec) {
	filters, ok := s.parseAdminReportFilters(w, r, spec.Requests)
	if !ok {
		return
	}
	rows, err := s.usage.rows(filters.UsageReportOptions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	var baseline adminSavingsBaselineDTO
	if spec.WithBaseline {
		var baselineOK bool
		baseline, baselineOK = s.parseAdminSavingsBaseline(w, r)
		if !baselineOK {
			return
		}
	}
	writeJSON(w, http.StatusOK, buildAdminScalarReportResponse(filters, rows, spec, baseline))
}

func (s *Service) handleAdminSecurityEvents(w http.ResponseWriter, r *http.Request) {
	opts, filters, ok := s.parseAdminSecurityFilters(w, r)
	if !ok {
		return
	}
	events, err := s.usage.securityAccessEvents(opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	writeJSON(w, http.StatusOK, buildAdminSecurityReportResponse(filters, events))
}

func (s *Service) handleAdminSecurityCSV(w http.ResponseWriter, r *http.Request) {
	opts, _, ok := s.parseAdminSecurityFilters(w, r)
	if !ok {
		return
	}
	events, err := s.usage.securityAccessEvents(opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "report-query-failed", "message": "report-query-failed"}})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="security-access-events.csv"`)
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time_utc", "request_id", "event_type", "surface", "method", "path", "status", "outcome", "reason", "auth_subject", "auth_source", "caller_id", "caller_user", "project", "token_id", "admin_subject", "client", "user_agent_family", "ip_address", "ip_source", "trusted_proxy_applied", "private_ip", "loopback_ip", "reserved_ip", "model_group", "requested_model", "resolved_group", "input_tokens", "output_tokens", "total_tokens"})
	for _, row := range adminSecurityRows(events) {
		_ = cw.Write([]string{row.TimeUTC, row.RequestID, row.EventType, row.Surface, row.Method, row.Path, strconv.Itoa(row.Status), row.Outcome, row.Reason, row.AuthSubject, row.AuthSource, row.CallerID, row.CallerUser, row.Project, row.TokenID, row.AdminSubject, row.Client, row.UserAgentFamily, row.IPAddress, row.IPSource, strconv.FormatBool(row.TrustedProxyApplied), strconv.FormatBool(row.PrivateIP), strconv.FormatBool(row.LoopbackIP), strconv.FormatBool(row.ReservedIP), row.ModelGroup, row.RequestedModel, row.ResolvedGroup, strconv.Itoa(row.InputTokens), strconv.Itoa(row.OutputTokens), strconv.Itoa(row.TotalTokens)})
	}
	cw.Flush()
}

func (s *Service) parseAdminSavingsBaseline(w http.ResponseWriter, r *http.Request) (adminSavingsBaselineDTO, bool) {
	q := r.URL.Query()
	if strings.EqualFold(strings.TrimSpace(q.Get("baseline")), "custom") {
		name := strings.TrimSpace(q.Get("baseline_name"))
		if name == "" {
			name = "Custom baseline"
		}
		input, errIn := strconv.ParseFloat(strings.TrimSpace(q.Get("baseline_input_price_per_million_usd")), 64)
		output, errOut := strconv.ParseFloat(strings.TrimSpace(q.Get("baseline_output_price_per_million_usd")), 64)
		if errIn != nil || errOut != nil || input < 0 || output < 0 || input > 100000 || output > 100000 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "invalid custom baseline pricing"}})
			return adminSavingsBaselineDTO{}, false
		}
		return adminSavingsBaselineDTO{
			BaselineID:                       "custom",
			BaselineName:                     name,
			PricingSource:                    "custom-session",
			PricingUpdatedAt:                 formatUsageTime(time.Now().UTC()),
			BaselineInputPricePerMillionUSD:  input,
			BaselineOutputPricePerMillionUSD: output,
			Custom:                           true,
		}, true
	}
	baselineID := strings.TrimSpace(q.Get("baseline"))
	if baselineID == "" {
		baselineID = "gpt-5.5"
	}
	for _, baseline := range s.adminSavingsBaselines() {
		if baseline.BaselineID == baselineID {
			return baseline, true
		}
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid-report-filter", "message": "unknown savings baseline"}})
	return adminSavingsBaselineDTO{}, false
}

func (s *Service) adminSavingsBaselines() []adminSavingsBaselineDTO {
	configured := s.cfg.Server.AdminReports.Baselines
	if len(configured) == 0 {
		configured = defaultAdminReportBaselines()
	}
	out := make([]adminSavingsBaselineDTO, 0, len(configured))
	for _, baseline := range configured {
		out = append(out, adminSavingsBaselineDTO{
			BaselineID:                       baseline.ID,
			BaselineName:                     baseline.Name,
			PricingSource:                    baseline.PricingSource,
			PricingUpdatedAt:                 baseline.PricingUpdatedAt,
			BaselineInputPricePerMillionUSD:  baseline.InputPricePerMillionUSD,
			BaselineOutputPricePerMillionUSD: baseline.OutputPricePerMillionUSD,
			Notes:                            baseline.Notes,
		})
	}
	return out
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
	resp.Charts = nil
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
	decisionSummary := s.usage.decisionTelemetrySummary(rows)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(renderUsageMarkdown(filters.From, filters.To, rows, decisionSummary)))
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

func (s *Service) parseAdminSecurityFilters(w http.ResponseWriter, r *http.Request) (SecurityReportOptions, adminReportFilters, bool) {
	filters, ok := s.parseAdminReportFilters(w, r, true)
	if !ok {
		return SecurityReportOptions{}, adminReportFilters{}, false
	}
	q := r.URL.Query()
	opts := SecurityReportOptions{
		From:          filters.From,
		To:            filters.To,
		Limit:         filters.Limit,
		Outcome:       strings.TrimSpace(q.Get("outcome")),
		ReasonCode:    strings.TrimSpace(q.Get("reason_code")),
		Surface:       strings.TrimSpace(q.Get("surface")),
		IPAddress:     strings.TrimSpace(q.Get("ip_address")),
		CallerID:      strings.TrimSpace(q.Get("caller_id")),
		CallerUser:    strings.TrimSpace(q.Get("caller_user")),
		CallerProject: strings.TrimSpace(q.Get("caller_project")),
		TokenID:       strings.TrimSpace(q.Get("token_id")),
		AdminSubject:  strings.TrimSpace(q.Get("admin_subject")),
		Client:        strings.TrimSpace(q.Get("client")),
	}
	return opts, filters, true
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
	series := adminSeriesFromAgg(byHour)
	generatedAt := formatUsageTime(time.Now().UTC())
	return adminReportResponse{
		Period:       adminReportPeriod{From: formatUsageTime(filters.From), To: formatUsageTime(filters.To)},
		Summary:      adminSummaryFromAgg(total),
		Series:       series,
		Charts:       adminChartsFromSeries(filters, generatedAt, series, adminRowsFromAgg(byProvider)),
		ByToken:      adminRowsFromAgg(byToken),
		ByGroup:      adminRowsFromAgg(byGroup),
		ByProvider:   adminRowsFromAgg(byProvider),
		ByStatus:     adminRowsFromAgg(byStatus),
		Cache:        adminCacheFromAgg(total),
		Requests:     adminRecentRequestsFromRows(rows, filters.Limit),
		GeneratedUTC: generatedAt,
	}
}

func buildAdminSecurityReportResponse(filters adminReportFilters, events []securityAccessEvent) adminSecurityReportResponse {
	generatedAt := formatUsageTime(time.Now().UTC())
	rows := adminSecurityRows(events)
	return adminSecurityReportResponse{
		Period:       adminReportPeriod{From: formatUsageTime(filters.From), To: formatUsageTime(filters.To)},
		Report:       "security-events",
		Summary:      adminSecuritySummaryFromEvents(events),
		Rows:         rows,
		Charts:       adminSecurityCharts(filters, generatedAt, rows),
		GeneratedUTC: generatedAt,
	}
}

func adminSecurityRows(events []securityAccessEvent) []adminSecurityEventRow {
	rows := make([]adminSecurityEventRow, 0, len(events))
	for _, event := range events {
		rows = append(rows, adminSecurityEventRow{
			TimeUTC:             formatUsageTime(event.TS),
			RequestID:           event.RequestID,
			EventType:           event.EventType,
			Surface:             event.Surface,
			Method:              event.HTTPMethod,
			Path:                event.PathTemplate,
			Status:              event.StatusCode,
			Outcome:             event.Outcome,
			Reason:              event.ReasonCode,
			AuthSubject:         event.AuthSubject,
			AuthSource:          event.AuthSource,
			CallerID:            event.CallerID,
			CallerUser:          event.CallerUser,
			Project:             event.CallerProject,
			TokenID:             event.TokenID,
			AdminSubject:        event.AdminSubject,
			Client:              event.Client,
			UserAgentFamily:     event.UserAgentFamily,
			IPAddress:           event.IPAddress,
			IPSource:            event.IPSource,
			TrustedProxyApplied: event.TrustedProxyApplied,
			PrivateIP:           event.RequestIsPrivate,
			LoopbackIP:          event.RequestIsLoopback,
			ReservedIP:          event.RequestIsReserved,
			ModelGroup:          event.ModelGroup,
			RequestedModel:      event.RequestedModel,
			ResolvedGroup:       event.ResolvedGroup,
			InputTokens:         event.InputTokens,
			OutputTokens:        event.OutputTokens,
			TotalTokens:         event.TotalTokens,
		})
	}
	return rows
}

func adminSecuritySummaryFromEvents(events []securityAccessEvent) adminSecuritySummary {
	ips := map[string]bool{}
	var summary adminSecuritySummary
	for _, event := range events {
		summary.Events++
		if event.IPAddress != "" {
			ips[event.IPAddress] = true
		}
		switch event.Outcome {
		case "allowed":
			summary.Allowed++
		case "unauthorized":
			summary.Unauthorized++
		case "forbidden":
			summary.Forbidden++
		case "denied":
			summary.Denied++
		case "error":
			summary.Errors++
		}
	}
	summary.UniqueIPs = int64(len(ips))
	return summary
}

func adminSecurityCharts(filters adminReportFilters, generatedAt string, rows []adminSecurityEventRow) []adminReportChart {
	byOutcome := map[string]float64{}
	bySurface := map[string]float64{}
	byReason := map[string]float64{}
	for _, row := range rows {
		byOutcome[defaultString(row.Outcome, "unknown")]++
		bySurface[defaultString(row.Surface, "unknown")]++
		if row.Reason != "" {
			byReason[row.Reason]++
		}
	}
	return []adminReportChart{
		adminCategoryChart(filters, generatedAt, "security_outcomes", "Security outcomes", "Outcome", "Events", "count", []adminReportChartSeries{adminSeriesFromCounts("Events", "count", "magenta", byOutcome)}),
		adminCategoryChart(filters, generatedAt, "security_surfaces", "Security surfaces", "Surface", "Events", "count", []adminReportChartSeries{adminSeriesFromCounts("Events", "count", "blue", bySurface)}),
		adminCategoryChart(filters, generatedAt, "security_reasons", "Security reasons", "Reason", "Events", "count", []adminReportChartSeries{adminSeriesFromCounts("Events", "count", "red", byReason)}),
	}
}

func adminSeriesFromCounts(name, unit, colorKey string, counts map[string]float64) adminReportChartSeries {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	points := make([]adminReportChartPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, adminReportChartPoint{X: key, Y: counts[key]})
	}
	return adminReportChartSeries{Name: name, Unit: unit, ColorKey: colorKey, Points: points}
}

func buildAdminSavingsResponse(filters adminReportFilters, rows []usageRow, baseline adminSavingsBaselineDTO, baselines []adminSavingsBaselineDTO) adminSavingsResponse {
	total := &adminSavingsAgg{}
	byHour := map[string]*adminSavingsAgg{}
	byGroup := map[string]*adminSavingsAgg{}
	missingTokens := 0
	missingActualCost := 0
	for _, row := range rows {
		if row.InputTokens == 0 && row.OutputTokens == 0 && row.TotalTokens == 0 {
			missingTokens++
		}
		if row.TotalCostUSD == 0 && (row.InputTokens > 0 || row.OutputTokens > 0 || row.TotalTokens > 0) {
			missingActualCost++
		}
		total.add(row, baseline)
		adminSavingsAggFor(byHour, row.TS.UTC().Truncate(time.Hour).Format(time.RFC3339)).add(row, baseline)
		adminSavingsAggFor(byGroup, defaultString(row.ResolvedGroup, row.RequestedModel)).add(row, baseline)
	}
	generatedAt := formatUsageTime(time.Now().UTC())
	byTime := adminSavingsRowsFromAgg(byHour)
	warnings := []string{}
	if missingTokens > 0 {
		warnings = append(warnings, strconv.Itoa(missingTokens)+" row(s) had no stored input/output token usage")
	}
	if missingActualCost > 0 {
		warnings = append(warnings, strconv.Itoa(missingActualCost)+" row(s) had token usage but zero stored actual cost")
	}
	return adminSavingsResponse{
		Period:       adminReportPeriod{From: formatUsageTime(filters.From), To: formatUsageTime(filters.To)},
		Baseline:     baseline,
		Baselines:    baselines,
		Summary:      total.row("total"),
		ByTime:       byTime,
		ByGroup:      adminSavingsRowsFromAgg(byGroup),
		Charts:       adminSavingsCharts(filters, generatedAt, byTime),
		Warnings:     warnings,
		GeneratedUTC: generatedAt,
	}
}

func buildAdminScalarReportResponse(filters adminReportFilters, rows []usageRow, spec adminScalarEndpointSpec, baseline adminSavingsBaselineDTO) adminScalarReportResponse {
	total := &agg{}
	for _, row := range rows {
		total.add(row)
	}
	generatedAt := formatUsageTime(time.Now().UTC())
	resp := adminScalarReportResponse{
		Period:       adminReportPeriod{From: formatUsageTime(filters.From), To: formatUsageTime(filters.To)},
		Report:       spec.Report,
		Summary:      adminSummaryFromAgg(total),
		GeneratedUTC: generatedAt,
	}
	if baseline.BaselineID != "" {
		resp.Baseline = &baseline
	}
	if spec.Requests {
		requestRows := append([]usageRow(nil), rows...)
		sort.Slice(requestRows, func(i, j int) bool {
			if requestRows[i].TotalCostUSD == requestRows[j].TotalCostUSD {
				return requestRows[i].TS.After(requestRows[j].TS)
			}
			return requestRows[i].TotalCostUSD > requestRows[j].TotalCostUSD
		})
		if len(requestRows) > filters.Limit {
			requestRows = requestRows[:filters.Limit]
		}
		resp.Requests = make([]adminReportRequest, 0, len(requestRows))
		for _, row := range requestRows {
			resp.Requests = append(resp.Requests, adminRequestFromRow(row))
		}
		return resp
	}
	table := map[string]*adminScalarAgg{}
	for _, row := range rows {
		keys := adminScalarKeys(row, spec)
		if len(keys) == 0 {
			continue
		}
		for _, key := range keys {
			if key.Key == "" {
				key.Key = "unknown"
			}
			mapKey := joinKey(key.Key, key.Secondary)
			if table[mapKey] == nil {
				table[mapKey] = &adminScalarAgg{Key: key.Key, SecondaryKey: key.Secondary}
			}
			table[mapKey].add(row, baseline)
		}
	}
	resp.Rows = adminScalarRowsFromAgg(table, spec.Sort, filters.Limit)
	resp.Charts = adminScalarCharts(filters, generatedAt, spec, resp.Rows)
	return resp
}

type adminScalarKey struct {
	Key       string
	Secondary string
}

type adminScalarAgg struct {
	Key                     string
	SecondaryKey            string
	Agg                     agg
	InputImageCount         int64
	InputImageTokens        int64
	PIIFilteredRequests     int64
	PIIFilterReplacements   int64
	UpstreamReportedCostUSD float64
	BaselineCostUSD         float64
	HasBaseline             bool
}

func (a *adminScalarAgg) add(row usageRow, baseline adminSavingsBaselineDTO) {
	a.Agg.add(row)
	a.InputImageCount += int64(row.InputImageCount)
	a.InputImageTokens += int64(row.InputImageTokens)
	if row.PIIFilterApplied {
		a.PIIFilteredRequests++
	}
	a.PIIFilterReplacements += int64(row.PIIFilterReplacements)
	a.UpstreamReportedCostUSD += row.UpstreamReportedTotalCostUSD
	if baseline.BaselineID != "" {
		a.HasBaseline = true
		a.BaselineCostUSD += adminBaselineCost(row, baseline)
	}
}

func (a *adminScalarAgg) row() adminScalarReportRow {
	cacheable := a.Agg.CacheHits + a.Agg.CacheMisses
	savings := a.BaselineCostUSD - a.Agg.TotalCostUSD
	row := adminScalarReportRow{
		Key:                                  a.Key,
		SecondaryKey:                         a.SecondaryKey,
		Requests:                             a.Agg.Calls,
		Errors:                               a.Agg.Errors,
		ErrorRatePct:                         ratioPct(a.Agg.Errors, a.Agg.Calls),
		Streams:                              a.Agg.Streams,
		Attempts:                             a.Agg.Attempts,
		Fallbacks:                            a.Agg.Fallbacks,
		FallbackRatePct:                      ratioPct(a.Agg.Fallbacks, a.Agg.Calls),
		CacheHits:                            a.Agg.CacheHits,
		CacheMisses:                          a.Agg.CacheMisses,
		CacheBypass:                          a.Agg.CacheBypass,
		CacheHitRatePct:                      ratioPct(a.Agg.CacheHits, cacheable),
		InputTokens:                          a.Agg.InputTokens,
		OutputTokens:                         a.Agg.OutputTokens,
		Tokens:                               a.Agg.TotalTokens,
		TotalTokens:                          a.Agg.TotalTokens,
		InputImageCount:                      a.InputImageCount,
		InputImageTokens:                     a.InputImageTokens,
		PIIFilteredRequests:                  a.PIIFilteredRequests,
		PIIFilterReplacements:                a.PIIFilterReplacements,
		CostUSD:                              a.Agg.TotalCostUSD,
		InputCostUSD:                         a.Agg.InputCostUSD,
		ImageCostUSD:                         a.Agg.ImageCostUSD,
		OutputCostUSD:                        a.Agg.OutputCostUSD,
		TotalCostUSD:                         a.Agg.TotalCostUSD,
		UpstreamReportedCostUSD:              a.UpstreamReportedCostUSD,
		AvgCostUSD:                           avgFloat(a.Agg.TotalCostUSD, a.Agg.Calls),
		AvgLatencyMS:                         avg(a.Agg.LatencyMS, a.Agg.Calls),
		MaxLatencyMS:                         a.Agg.MaxLatencyMS,
		AvgTTFBMS:                            avg(a.Agg.TTFBMS, a.Agg.TTFBCount),
		MaxTTFBMS:                            a.Agg.MaxTTFBMS,
		AvgUpstreamMS:                        avg(a.Agg.UpstreamMS, a.Agg.UpstreamMSCount),
		MaxUpstreamMS:                        a.Agg.MaxUpstreamMS,
		AvgDownstreamMS:                      avg(a.Agg.DownstreamMS, a.Agg.DownstreamMSCount),
		MaxDownstreamMS:                      a.Agg.MaxDownstreamMS,
		AvgUpstreamTokensPerSec:              avgFloat(a.Agg.UpstreamOutputTPS, a.Agg.UpstreamOutputTPSCount),
		AvgUpstreamOutputTokensPerSec:        avgFloat(a.Agg.UpstreamOutputTPS, a.Agg.UpstreamOutputTPSCount),
		AvgUpstreamTotalTokensPerSec:         avgFloat(a.Agg.UpstreamTotalTPS, a.Agg.UpstreamTotalTPSCount),
		AvgDownstreamTokensPerSec:            avgFloat(a.Agg.DownstreamOutputTPS, a.Agg.DownstreamOutputTPSCount),
		AvgDownstreamWriteOutputTokensPerSec: avgFloat(a.Agg.DownstreamOutputTPS, a.Agg.DownstreamOutputTPSCount),
		AvgDownstreamWriteTotalTokensPerSec:  avgFloat(a.Agg.DownstreamTotalTPS, a.Agg.DownstreamTotalTPSCount),
		LatestCacheItems:                     a.Agg.CacheItemsLatest,
		LatestCacheBytes:                     a.Agg.CacheBytesLatest,
		LatestCacheMaxBytes:                  a.Agg.CacheMaxBytesLatest,
		LatestCacheOccupancyPct:              a.Agg.CacheOccupancyLatest,
	}
	if a.HasBaseline {
		savingsPct := ratioPctFloat(savings, a.BaselineCostUSD)
		row.BaselineCostUSD = &a.BaselineCostUSD
		row.SavingsUSD = &savings
		row.SavingsPct = &savingsPct
	}
	return row
}

func adminScalarRowsFromAgg(data map[string]*adminScalarAgg, sortBy string, limit int) []adminScalarReportRow {
	out := make([]adminScalarReportRow, 0, len(data))
	for _, a := range data {
		out = append(out, a.row())
	}
	sort.Slice(out, func(i, j int) bool {
		switch sortBy {
		case "savings":
			iSavings := scalarSavingsValue(out[i])
			jSavings := scalarSavingsValue(out[j])
			if iSavings != jSavings {
				return iSavings > jSavings
			}
		case "cost":
			if out[i].CostUSD != out[j].CostUSD {
				return out[i].CostUSD > out[j].CostUSD
			}
		case "tokens":
			if out[i].Tokens != out[j].Tokens {
				return out[i].Tokens > out[j].Tokens
			}
		case "latency":
			if out[i].AvgLatencyMS != out[j].AvgLatencyMS {
				return out[i].AvgLatencyMS > out[j].AvgLatencyMS
			}
		case "errors":
			if out[i].Errors != out[j].Errors {
				return out[i].Errors > out[j].Errors
			}
		case "image":
			if out[i].InputImageCount != out[j].InputImageCount {
				return out[i].InputImageCount > out[j].InputImageCount
			}
		default:
			if out[i].Requests != out[j].Requests {
				return out[i].Requests > out[j].Requests
			}
		}
		if out[i].Key == out[j].Key {
			return out[i].SecondaryKey < out[j].SecondaryKey
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func adminScalarKeys(row usageRow, spec adminScalarEndpointSpec) []adminScalarKey {
	if spec.Anomalies {
		return adminAnomalyKeys(row, spec)
	}
	if spec.Dimension == "capability" {
		secondary := adminScalarDimension(row, spec.Secondary)
		caps := adminCapabilityKeys(row)
		keys := make([]adminScalarKey, 0, len(caps))
		for _, cap := range caps {
			keys = append(keys, adminScalarKey{Key: cap, Secondary: secondary})
		}
		return keys
	}
	key := adminScalarDimension(row, spec.Dimension)
	secondary := adminScalarDimension(row, spec.Secondary)
	return []adminScalarKey{{Key: key, Secondary: secondary}}
}

func adminScalarDimension(row usageRow, dimension string) string {
	switch dimension {
	case "":
		return ""
	case "caller_user":
		return defaultString(row.CallerUser, "unknown")
	case "token_id":
		return defaultString(row.TokenID, "unknown")
	case "model_group":
		return defaultString(row.ResolvedGroup, row.RequestedModel)
	case "provider_model":
		return joinKey(defaultString(row.TargetProvider, "unknown"), defaultString(row.TargetModel, "unknown"))
	case "dialect":
		return defaultString(row.TargetDialect, "unknown")
	case "client":
		return defaultString(row.Client, "unknown")
	case "inbound_dialect":
		return defaultString(row.InboundDialect, "unknown")
	case "status_error":
		errorKey := "ok"
		if row.Status >= 400 {
			errorKey = defaultString(row.Error, "error")
		}
		return joinKey(strconv.Itoa(row.Status), errorKey)
	case "cache":
		return defaultString(row.Cache, "bypass")
	case "quota_key_state":
		return joinKey(defaultString(row.QuotaState, "unknown"), defaultString(row.KeyState, "unknown"))
	case "routing_decision":
		return joinKey(defaultString(row.Strategy, "unknown"), defaultString(row.ResolvedGroup, row.RequestedModel))
	case "contract_bucket":
		if !row.ContractPresent {
			return "none"
		}
		return defaultString(row.ContractBucket, "unknown")
	case "contract_workload":
		if !row.ContractPresent {
			return "none"
		}
		return defaultString(row.ContractWorkload, "unspecified")
	case "target_validation":
		return joinKey(defaultString(row.TargetValidationStatus, "missing"), defaultString(row.TargetValidationAgeBucket, "missing"))
	case "project":
		return defaultString(row.CallerProject, "unknown")
	case "environment":
		return defaultString(row.CallerEnvironment, "unknown")
	case "capability":
		return "capability"
	default:
		return "unknown"
	}
}

func adminCapabilityKeys(row usageRow) []string {
	caps := []string{}
	if row.InputHasImage || row.InputImageCount > 0 || row.InputImageTokens > 0 {
		caps = append(caps, "image-input")
	}
	if row.Stream {
		caps = append(caps, "streaming")
	}
	if row.PIIFilterApplied {
		caps = append(caps, "pii-filtered")
	}
	if row.Cache == "hit" || row.Cache == "miss" {
		caps = append(caps, "cacheable")
	}
	if row.TargetDialect != "" {
		caps = append(caps, "dialect:"+row.TargetDialect)
	}
	if len(caps) == 0 {
		caps = append(caps, "text")
	}
	return caps
}

func adminChartSeriesFromScalarRows(name, unit, colorKey string, rows []adminScalarReportRow, value func(adminScalarReportRow) float64) adminReportChartSeries {
	points := make([]adminReportChartPoint, 0, len(rows))
	for _, row := range rows {
		key := row.Key
		if row.SecondaryKey != "" {
			key = joinKey(row.Key, row.SecondaryKey)
		}
		points = append(points, adminReportChartPoint{X: key, Y: value(row)})
	}
	return adminReportChartSeries{Name: name, Unit: unit, ColorKey: colorKey, Points: points}
}

func adminAnomalyKeys(row usageRow, spec adminScalarEndpointSpec) []adminScalarKey {
	secondary := adminScalarDimension(row, spec.Secondary)
	keys := []adminScalarKey{}
	if row.Status >= 400 {
		keys = append(keys, adminScalarKey{Key: "error", Secondary: secondary})
	}
	if row.FallbackUsed {
		keys = append(keys, adminScalarKey{Key: "fallback", Secondary: secondary})
	}
	if row.Attempts > 1 {
		keys = append(keys, adminScalarKey{Key: "multi-attempt", Secondary: secondary})
	}
	if row.LatencyMS >= 30000 {
		keys = append(keys, adminScalarKey{Key: "slow-request", Secondary: secondary})
	}
	if row.TotalCostUSD >= 1 {
		keys = append(keys, adminScalarKey{Key: "expensive-request", Secondary: secondary})
	}
	if row.QuotaState != "" && row.QuotaState != "ok" {
		keys = append(keys, adminScalarKey{Key: "quota-" + row.QuotaState, Secondary: secondary})
	}
	if row.KeyState != "" && row.KeyState != "ok" && row.KeyState != "active" {
		keys = append(keys, adminScalarKey{Key: "key-" + row.KeyState, Secondary: secondary})
	}
	return keys
}

func scalarSavingsValue(row adminScalarReportRow) float64 {
	if row.SavingsUSD == nil {
		return 0
	}
	return *row.SavingsUSD
}

func adminScalarCharts(filters adminReportFilters, generatedAt string, spec adminScalarEndpointSpec, rows []adminScalarReportRow) []adminReportChart {
	if len(rows) == 0 {
		return nil
	}
	charts := []adminReportChart{
		adminCategoryChart(filters, generatedAt, spec.Report+"_requests", spec.Report+" requests", "Key", "Requests", "count", []adminReportChartSeries{
			adminChartSeriesFromScalarRows("Requests", "count", "magenta", rows, func(row adminScalarReportRow) float64 { return float64(row.Requests) }),
		}),
	}
	switch spec.Sort {
	case "cost", "savings":
		charts = append(charts, adminCategoryChart(filters, generatedAt, spec.Report+"_cost", spec.Report+" cost", "Key", "USD", "usd", []adminReportChartSeries{
			adminChartSeriesFromScalarRows("Cost", "usd", "red", rows, func(row adminScalarReportRow) float64 { return row.CostUSD }),
		}))
	case "latency":
		charts = append(charts, adminCategoryChart(filters, generatedAt, spec.Report+"_latency", spec.Report+" latency", "Key", "Milliseconds", "ms", []adminReportChartSeries{
			adminChartSeriesFromScalarRows("Avg latency", "ms", "violet", rows, func(row adminScalarReportRow) float64 { return float64(row.AvgLatencyMS) }),
			adminChartSeriesFromScalarRows("Max latency", "ms", "red", rows, func(row adminScalarReportRow) float64 { return float64(row.MaxLatencyMS) }),
		}))
	case "errors":
		charts = append(charts, adminCategoryChart(filters, generatedAt, spec.Report+"_errors", spec.Report+" errors", "Key", "Errors", "count", []adminReportChartSeries{
			adminChartSeriesFromScalarRows("Errors", "count", "red", rows, func(row adminScalarReportRow) float64 { return float64(row.Errors) }),
			adminChartSeriesFromScalarRows("Fallbacks", "count", "warning", rows, func(row adminScalarReportRow) float64 { return float64(row.Fallbacks) }),
		}))
	default:
		charts = append(charts, adminCategoryChart(filters, generatedAt, spec.Report+"_tokens", spec.Report+" token volume", "Key", "Token count", "tokens", []adminReportChartSeries{
			adminChartSeriesFromScalarRows("Input Tokens", "tokens", "blue", rows, func(row adminScalarReportRow) float64 { return float64(row.InputTokens) }),
			adminChartSeriesFromScalarRows("Output Tokens", "tokens", "magenta", rows, func(row adminScalarReportRow) float64 { return float64(row.OutputTokens) }),
			adminChartSeriesFromScalarRows("Total Tokens", "tokens", "purple", rows, func(row adminScalarReportRow) float64 { return float64(row.TotalTokens) }),
		}))
	}
	return charts
}

type adminSavingsAgg struct {
	Requests        int64
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	ActualCostUSD   float64
	BaselineCostUSD float64
}

func adminSavingsAggFor(data map[string]*adminSavingsAgg, key string) *adminSavingsAgg {
	if data[key] == nil {
		data[key] = &adminSavingsAgg{}
	}
	return data[key]
}

func (a *adminSavingsAgg) add(row usageRow, baseline adminSavingsBaselineDTO) {
	a.Requests++
	a.InputTokens += int64(row.InputTokens)
	a.OutputTokens += int64(row.OutputTokens)
	a.TotalTokens += int64(totalTokens(Usage{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, TotalTokens: row.TotalTokens}))
	a.ActualCostUSD += row.TotalCostUSD
	a.BaselineCostUSD += adminBaselineCost(row, baseline)
}

func (a *adminSavingsAgg) row(key string) adminSavingsRow {
	savings := a.BaselineCostUSD - a.ActualCostUSD
	return adminSavingsRow{
		Key:             key,
		Requests:        a.Requests,
		InputTokens:     a.InputTokens,
		OutputTokens:    a.OutputTokens,
		TotalTokens:     a.TotalTokens,
		ActualCostUSD:   a.ActualCostUSD,
		BaselineCostUSD: a.BaselineCostUSD,
		SavingsUSD:      savings,
		SavingsPct:      ratioPctFloat(savings, a.BaselineCostUSD),
	}
}

func adminBaselineCost(row usageRow, baseline adminSavingsBaselineDTO) float64 {
	return (float64(row.InputTokens) / 1_000_000 * baseline.BaselineInputPricePerMillionUSD) +
		(float64(row.OutputTokens) / 1_000_000 * baseline.BaselineOutputPricePerMillionUSD)
}

func ratioPctFloat(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return part / total * 100
}

func adminSavingsRowsFromAgg(data map[string]*adminSavingsAgg) []adminSavingsRow {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]adminSavingsRow, 0, len(keys))
	for _, key := range keys {
		out = append(out, data[key].row(key))
	}
	return out
}

func adminSavingsCharts(filters adminReportFilters, generatedAt string, rows []adminSavingsRow) []adminReportChart {
	return []adminReportChart{
		adminTimeSavingsChart(filters, generatedAt, "savings_cost", "Actual vs baseline cost", "USD", "usd", []adminReportChartSeries{
			adminSavingsChartSeries("Actual cost", "usd", "red", rows, func(row adminSavingsRow) float64 { return row.ActualCostUSD }),
			adminSavingsChartSeries("Baseline cost", "usd", "blue", rows, func(row adminSavingsRow) float64 { return row.BaselineCostUSD }),
		}),
		adminTimeSavingsChart(filters, generatedAt, "savings_usd", "Savings over time", "USD", "usd", []adminReportChartSeries{
			adminSavingsChartSeries("Savings", "usd", "magenta", rows, func(row adminSavingsRow) float64 { return row.SavingsUSD }),
		}),
		adminTimeSavingsChart(filters, generatedAt, "savings_pct", "Savings rate over time", "Percent", "percent", []adminReportChartSeries{
			adminSavingsChartSeries("Savings rate", "percent", "purple", rows, func(row adminSavingsRow) float64 { return row.SavingsPct }),
		}),
	}
}

func adminTimeSavingsChart(filters adminReportFilters, generatedAt, id, title, yLabel, yUnit string, series []adminReportChartSeries) adminReportChart {
	return adminReportChart{
		ChartID:     id,
		Title:       title,
		XAxis:       adminReportChartAxis{Label: "Time", Type: "time", Unit: "UTC hour"},
		YAxis:       adminReportChartAxis{Label: yLabel, Type: "linear", Unit: yUnit},
		Series:      series,
		GeneratedAt: generatedAt,
		From:        formatUsageTime(filters.From),
		To:          formatUsageTime(filters.To),
		Filters:     adminFilterDTO(filters),
	}
}

func adminSavingsChartSeries(name, unit, colorKey string, rows []adminSavingsRow, value func(adminSavingsRow) float64) adminReportChartSeries {
	points := make([]adminReportChartPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, adminReportChartPoint{X: row.Key, Y: value(row)})
	}
	return adminReportChartSeries{Name: name, Unit: unit, ColorKey: colorKey, Points: points}
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
		Requests:                    a.Calls,
		Errors:                      a.Errors,
		Tokens:                      a.TotalTokens,
		TotalTokens:                 a.TotalTokens,
		InputTokens:                 a.InputTokens,
		OutputTokens:                a.OutputTokens,
		CostUSD:                     a.TotalCostUSD,
		TotalCostUSD:                a.TotalCostUSD,
		ImageCostUSD:                a.ImageCostUSD,
		Attempts:                    a.Attempts,
		Fallbacks:                   a.Fallbacks,
		Streams:                     a.Streams,
		AvgLatencyMS:                avg(a.LatencyMS, a.Calls),
		MaxLatencyMS:                a.MaxLatencyMS,
		AvgTTFBMS:                   avg(a.TTFBMS, a.TTFBCount),
		AvgUpstreamTPS:              avgFloat(a.UpstreamOutputTPS, a.UpstreamOutputTPSCount),
		AvgUpstreamOutputTPS:        avgFloat(a.UpstreamOutputTPS, a.UpstreamOutputTPSCount),
		AvgUpstreamTotalTPS:         avgFloat(a.UpstreamTotalTPS, a.UpstreamTotalTPSCount),
		AvgDownstreamTPS:            avgFloat(a.DownstreamOutputTPS, a.DownstreamOutputTPSCount),
		AvgDownstreamWriteOutputTPS: avgFloat(a.DownstreamOutputTPS, a.DownstreamOutputTPSCount),
		AvgDownstreamWriteTotalTPS:  avgFloat(a.DownstreamTotalTPS, a.DownstreamTotalTPSCount),
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
		out = append(out, adminReportTableRow{Key: key, Requests: a.Calls, Errors: a.Errors, Tokens: a.TotalTokens, TotalTokens: a.TotalTokens, InputTokens: a.InputTokens, OutputTokens: a.OutputTokens, CostUSD: a.TotalCostUSD, InputCostUSD: a.InputCostUSD, ImageCostUSD: a.ImageCostUSD, OutputCostUSD: a.OutputCostUSD, TotalCostUSD: a.TotalCostUSD, Attempts: a.Attempts, Fallbacks: a.Fallbacks, AvgLatencyMS: avg(a.LatencyMS, a.Calls), MaxLatencyMS: a.MaxLatencyMS})
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

func adminChartsFromSeries(filters adminReportFilters, generatedAt string, series []adminReportSeries, providers []adminReportTableRow) []adminReportChart {
	return []adminReportChart{
		adminTimeChart(filters, generatedAt, "requests", "Requests", "Requests", "count", []adminReportChartSeries{
			adminChartSeriesFromTimeRows("Requests", "count", "magenta", series, func(row adminReportSeries) float64 { return float64(row.Requests) }),
			adminChartSeriesFromTimeRows("Errors", "count", "red", series, func(row adminReportSeries) float64 { return float64(row.Errors) }),
		}),
		adminTimeChart(filters, generatedAt, "cost", "Cost", "USD", "usd", []adminReportChartSeries{
			adminChartSeriesFromTimeRows("Cost", "usd", "red", series, func(row adminReportSeries) float64 { return row.CostUSD }),
		}),
		adminTimeChart(filters, generatedAt, "latency", "Latency", "Milliseconds", "ms", []adminReportChartSeries{
			adminChartSeriesFromTimeRows("Latency", "ms", "violet", series, func(row adminReportSeries) float64 { return float64(row.LatencyMS) }),
			adminChartSeriesFromTimeRows("TTFB", "ms", "blue", series, func(row adminReportSeries) float64 { return float64(row.TTFBMS) }),
		}),
		adminTimeChart(filters, generatedAt, "cache", "Cache", "Requests", "count", []adminReportChartSeries{
			adminChartSeriesFromTimeRows("Hits", "count", "success", series, func(row adminReportSeries) float64 { return float64(row.CacheHits) }),
			adminChartSeriesFromTimeRows("Misses", "count", "warning", series, func(row adminReportSeries) float64 { return float64(row.CacheMisses) }),
			adminChartSeriesFromTimeRows("Bypass", "count", "text", series, func(row adminReportSeries) float64 { return float64(row.CacheBypass) }),
		}),
		adminCategoryChart(filters, generatedAt, "provider_tokens", "Provider token volume", "Provider / model", "Token count", "tokens", []adminReportChartSeries{
			adminChartSeriesFromTableRows("Input Tokens", "tokens", "blue", providers, func(row adminReportTableRow) float64 { return float64(row.InputTokens) }),
			adminChartSeriesFromTableRows("Output Tokens", "tokens", "magenta", providers, func(row adminReportTableRow) float64 { return float64(row.OutputTokens) }),
			adminChartSeriesFromTableRows("Total Tokens", "tokens", "purple", providers, func(row adminReportTableRow) float64 { return float64(row.TotalTokens) }),
		}),
		adminTimeChart(filters, generatedAt, "errors_fallbacks", "Errors And Fallbacks", "Requests", "count", []adminReportChartSeries{
			adminChartSeriesFromTimeRows("Errors", "count", "red", series, func(row adminReportSeries) float64 { return float64(row.Errors) }),
			adminChartSeriesFromTimeRows("Fallbacks", "count", "warning", series, func(row adminReportSeries) float64 { return float64(row.Fallbacks) }),
		}),
	}
}

func adminTimeChart(filters adminReportFilters, generatedAt, id, title, yLabel, yUnit string, chartSeries []adminReportChartSeries) adminReportChart {
	return adminReportChart{
		ChartID:     id,
		Title:       title,
		XAxis:       adminReportChartAxis{Label: "Time", Type: "time", Unit: "UTC hour"},
		YAxis:       adminReportChartAxis{Label: yLabel, Type: "linear", Unit: yUnit},
		Series:      chartSeries,
		GeneratedAt: generatedAt,
		From:        formatUsageTime(filters.From),
		To:          formatUsageTime(filters.To),
		Filters:     adminFilterDTO(filters),
	}
}

func adminCategoryChart(filters adminReportFilters, generatedAt, id, title, xLabel, yLabel, yUnit string, chartSeries []adminReportChartSeries) adminReportChart {
	return adminReportChart{
		ChartID:     id,
		Title:       title,
		XAxis:       adminReportChartAxis{Label: xLabel, Type: "category"},
		YAxis:       adminReportChartAxis{Label: yLabel, Type: "linear", Unit: yUnit},
		Series:      chartSeries,
		GeneratedAt: generatedAt,
		From:        formatUsageTime(filters.From),
		To:          formatUsageTime(filters.To),
		Filters:     adminFilterDTO(filters),
	}
}

func adminChartSeriesFromTimeRows(name, unit, colorKey string, rows []adminReportSeries, value func(adminReportSeries) float64) adminReportChartSeries {
	points := make([]adminReportChartPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, adminReportChartPoint{X: row.TimeUTC, Y: value(row)})
	}
	return adminReportChartSeries{Name: name, Unit: unit, ColorKey: colorKey, Points: points}
}

func adminChartSeriesFromTableRows(name, unit, colorKey string, rows []adminReportTableRow, value func(adminReportTableRow) float64) adminReportChartSeries {
	points := make([]adminReportChartPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, adminReportChartPoint{X: row.Key, Y: value(row)})
	}
	return adminReportChartSeries{Name: name, Unit: unit, ColorKey: colorKey, Points: points}
}

func adminFilterDTO(filters adminReportFilters) adminReportFilterDTO {
	return adminReportFilterDTO{
		TokenID:           filters.TokenID,
		TokenIDPrefix:     filters.TokenIDPrefix,
		CallerUser:        filters.CallerUser,
		CallerProject:     filters.CallerProject,
		CallerEnvironment: filters.CallerEnvironment,
		ResolvedGroup:     filters.ResolvedGroup,
		Client:            filters.Client,
	}
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
		TotalTokens:  totalTokens(Usage{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, TotalTokens: row.TotalTokens}),
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		CostUSD:      row.TotalCostUSD,
		TotalCostUSD: row.TotalCostUSD,
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
