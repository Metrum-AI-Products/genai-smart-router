package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type requestLogger struct {
	mu   sync.Mutex
	path string
	file *os.File
}

type logRecord struct {
	TS                                 string                          `json:"ts"`
	RequestID                          string                          `json:"request_id"`
	CallerID                           string                          `json:"caller_id"`
	CallerUser                         string                          `json:"caller_user"`
	CallerProject                      string                          `json:"caller_project"`
	CallerEnvironment                  string                          `json:"caller_environment"`
	CallerIP                           string                          `json:"caller_ip"`
	TokenID                            string                          `json:"token_id"`
	Client                             string                          `json:"client"`
	InboundDialect                     string                          `json:"inbound_dialect"`
	RequestedModel                     string                          `json:"requested_model"`
	ResolvedGroup                      string                          `json:"resolved_group"`
	Strategy                           string                          `json:"strategy"`
	ClassLabel                         *string                         `json:"class_label"`
	TargetProvider                     string                          `json:"target_provider"`
	TargetModel                        string                          `json:"target_model"`
	TargetDialect                      string                          `json:"target_dialect"`
	Stream                             bool                            `json:"stream"`
	Cache                              string                          `json:"cache"`
	Status                             int                             `json:"status"`
	Attempts                           int                             `json:"attempts"`
	FallbackUsed                       bool                            `json:"fallback_used"`
	LatencyMS                          int64                           `json:"latency_ms"`
	TTFBMS                             *int64                          `json:"ttfb_ms"`
	UpstreamMS                         *int64                          `json:"upstream_duration_ms"`
	DownstreamMS                       *int64                          `json:"downstream_duration_ms"`
	UpstreamOutputTPS                  *float64                        `json:"upstream_output_tokens_per_sec"`
	UpstreamTotalTPS                   *float64                        `json:"upstream_total_tokens_per_sec"`
	DownstreamOutputTPS                *float64                        `json:"downstream_output_tokens_per_sec"`
	DownstreamTotalTPS                 *float64                        `json:"downstream_total_tokens_per_sec"`
	Usage                              Usage                           `json:"usage"`
	InputHasImage                      bool                            `json:"input_has_image,omitempty"`
	InputImageCount                    int                             `json:"input_image_count,omitempty"`
	InputImageTokens                   int                             `json:"input_image_tokens,omitempty"`
	PIIFilterApplied                   bool                            `json:"pii_filter_applied,omitempty"`
	PIIFilterMode                      string                          `json:"pii_filter_mode,omitempty"`
	PIIFilterReplacements              int                             `json:"pii_filter_replacements,omitempty"`
	PIIFilterRuleCount                 int                             `json:"pii_filter_rule_count,omitempty"`
	ContractPresent                    bool                            `json:"contract_present,omitempty"`
	ContractBucket                     string                          `json:"contract_bucket,omitempty"`
	ContractFailureReason              string                          `json:"contract_failure_reason,omitempty"`
	ContractWorkload                   string                          `json:"contract_workload,omitempty"`
	TargetValidationStatus             string                          `json:"target_validation_status,omitempty"`
	TargetValidationWorkload           string                          `json:"target_validation_workload,omitempty"`
	TargetValidationAgeBucket          string                          `json:"target_validation_age_bucket,omitempty"`
	InputPricePerMillionUSD            float64                         `json:"input_price_per_million_usd,omitempty"`
	OutputPricePerMillionUSD           float64                         `json:"output_price_per_million_usd,omitempty"`
	ImageInputPricePerMillionTokensUSD float64                         `json:"image_input_price_per_million_tokens_usd,omitempty"`
	ImageInputPricePerImageUSD         float64                         `json:"image_input_price_per_image_usd,omitempty"`
	InputCostUSD                       float64                         `json:"input_cost_usd,omitempty"`
	ImageCostUSD                       float64                         `json:"image_cost_usd,omitempty"`
	OutputCostUSD                      float64                         `json:"output_cost_usd,omitempty"`
	TotalCostUSD                       float64                         `json:"total_cost_usd,omitempty"`
	UpstreamReportedInputCostUSD       float64                         `json:"upstream_reported_input_cost_usd,omitempty"`
	UpstreamReportedOutputCostUSD      float64                         `json:"upstream_reported_output_cost_usd,omitempty"`
	UpstreamReportedTotalCostUSD       float64                         `json:"upstream_reported_total_cost_usd,omitempty"`
	PricingSource                      string                          `json:"pricing_source,omitempty"`
	PricingUpdatedAt                   string                          `json:"pricing_updated_at,omitempty"`
	CacheEnabled                       bool                            `json:"cache_enabled"`
	CacheItems                         int64                           `json:"cache_items"`
	CacheBytes                         int64                           `json:"cache_bytes"`
	CacheMaxBytes                      int64                           `json:"cache_max_bytes"`
	CacheOccupancyPct                  float64                         `json:"cache_occupancy_pct"`
	QuotaState                         string                          `json:"quota_state"`
	KeyState                           string                          `json:"key_state"`
	LicenseStatus                      string                          `json:"license_status,omitempty"`
	LicenseReason                      string                          `json:"license_reason,omitempty"`
	LicenseID                          string                          `json:"license_id,omitempty"`
	LicenseCustomerID                  string                          `json:"license_customer_id,omitempty"`
	LicenseSKU                         string                          `json:"license_sku,omitempty"`
	LicenseKeyID                       string                          `json:"license_key_id,omitempty"`
	LicenseExpiry                      string                          `json:"license_expiry,omitempty"`
	LicenseGraceActive                 bool                            `json:"license_grace_active,omitempty"`
	RouterVersion                      string                          `json:"router_version,omitempty"`
	RouterBuildDate                    string                          `json:"router_build_date,omitempty"`
	RoutingConfigFingerprint           string                          `json:"routing_config_fingerprint,omitempty"`
	ModelGroupConfigFingerprint        string                          `json:"model_group_config_fingerprint,omitempty"`
	RoutingPolicyFingerprint           string                          `json:"routing_policy_fingerprint,omitempty"`
	PricingCatalogFingerprint          string                          `json:"pricing_catalog_fingerprint,omitempty"`
	Warnings                           []string                        `json:"warnings"`
	Error                              *string                         `json:"error"`
	ErrorClass                         string                          `json:"error_class,omitempty"`
	ErrorMessage                       string                          `json:"error_message,omitempty"`
	AttemptsDetail                     []attemptLogRecord              `json:"attempts_detail,omitempty"`
	TraceEvents                        []traceLogRecord                `json:"trace_events,omitempty"`
	DecisionShapeFeatures              []decisionShapeFeatureLogRecord `json:"decision_shape_features,omitempty"`
	DecisionCandidates                 []decisionCandidateLogRecord    `json:"decision_candidates,omitempty"`
	DecisionFilterReasons              []decisionFilterReasonLogRecord `json:"decision_filter_reasons,omitempty"`
	RoutingDecisions                   []routingDecisionLogRecord      `json:"routing_decisions,omitempty"`
	RoutingSignals                     []routingSignalLogRecord        `json:"routing_signals,omitempty"`
	DynamicScoreTerms                  []dynamicScoreTermLogRecord     `json:"dynamic_score_terms,omitempty"`
	PolicyExecutions                   []policyExecutionLogRecord      `json:"policy_executions,omitempty"`
	FallbackTransitions                []fallbackTransitionLogRecord   `json:"fallback_transitions,omitempty"`
	CacheReasons                       []cacheReasonLogRecord          `json:"cache_reasons,omitempty"`
}

type attemptLogRecord struct {
	Index            int    `json:"index"`
	TS               string `json:"ts"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Dialect          string `json:"dialect"`
	EndpointHost     string `json:"endpoint_host,omitempty"`
	DurationMS       int64  `json:"duration_ms"`
	StatusCode       int    `json:"status_code,omitempty"`
	ErrorClass       string `json:"error_class,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	Retryable        bool   `json:"retryable,omitempty"`
	TimedOut         bool   `json:"timed_out,omitempty"`
	ClientCanceled   bool   `json:"client_canceled,omitempty"`
	Selected         bool   `json:"selected,omitempty"`
	FallbackReason   string `json:"fallback_reason,omitempty"`
	RequestBytes     int64  `json:"request_bytes,omitempty"`
	ResponseBytes    int64  `json:"response_bytes,omitempty"`
	AttemptTimeoutMS int    `json:"attempt_timeout_ms,omitempty"`
}

type traceLogRecord struct {
	Seq        int    `json:"seq"`
	TS         string `json:"ts"`
	Event      string `json:"event"`
	Message    string `json:"message,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Dialect    string `json:"dialect,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
}

type decisionShapeFeatureLogRecord struct {
	Seq       int    `json:"seq"`
	Name      string `json:"name"`
	BoolValue bool   `json:"bool_value,omitempty"`
	IntValue  int    `json:"int_value,omitempty"`
	TextValue string `json:"text_value,omitempty"`
}

type decisionCandidateLogRecord struct {
	CandidateIndex   int    `json:"candidate_index"`
	GroupTargetIndex int    `json:"group_target_index"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	ModelRef         string `json:"model_ref,omitempty"`
	Dialect          string `json:"dialect"`
	Weight           int    `json:"weight,omitempty"`
	ToolOnly         bool   `json:"tool_only,omitempty"`
	ContextTokens    int    `json:"context_tokens,omitempty"`
	InputImage       bool   `json:"input_image,omitempty"`
	OutputImage      bool   `json:"output_image,omitempty"`
	ToolSupport      bool   `json:"tool_support,omitempty"`
	ForcedToolChoice bool   `json:"forced_tool_choice,omitempty"`
	StructuredOutput bool   `json:"structured_output,omitempty"`
	HonorsMaxTokens  bool   `json:"honors_max_tokens,omitempty"`
	ReasoningSupport bool   `json:"reasoning_support,omitempty"`
	ReasoningMode    string `json:"reasoning_mode,omitempty"`
	ReasoningControl string `json:"reasoning_control,omitempty"`
	ReasoningDefault bool   `json:"reasoning_default,omitempty"`
	ReasoningStream  string `json:"reasoning_stream_block,omitempty"`
	ValidationStatus string `json:"validation_status,omitempty"`
	ValidationAge    string `json:"validation_age_bucket,omitempty"`
	Eligible         bool   `json:"eligible"`
	Selected         bool   `json:"selected,omitempty"`
}

type decisionFilterReasonLogRecord struct {
	Seq            int    `json:"seq"`
	CandidateIndex int    `json:"candidate_index"`
	Stage          string `json:"stage"`
	Reason         string `json:"reason"`
}

type routingDecisionLogRecord struct {
	Seq                    int     `json:"seq"`
	Strategy               string  `json:"strategy"`
	SelectedCandidateIndex int     `json:"selected_candidate_index"`
	Provider               string  `json:"provider"`
	Model                  string  `json:"model"`
	Dialect                string  `json:"dialect"`
	FallbackCount          int     `json:"fallback_count"`
	ClassLabel             *string `json:"class_label,omitempty"`
}

type routingSignalLogRecord struct {
	Seq            int     `json:"seq"`
	Strategy       string  `json:"strategy"`
	SignalName     string  `json:"signal_name"`
	Source         string  `json:"source,omitempty"`
	CandidateIndex int     `json:"candidate_index,omitempty"`
	BoolValue      bool    `json:"bool_value,omitempty"`
	IntValue       int     `json:"int_value,omitempty"`
	FloatValue     float64 `json:"float_value,omitempty"`
	TextValue      string  `json:"text_value,omitempty"`
}

type dynamicScoreTermLogRecord struct {
	Seq                int     `json:"seq"`
	CandidateIndex     int     `json:"candidate_index"`
	Rank               int     `json:"rank"`
	Provider           string  `json:"provider"`
	Model              string  `json:"model"`
	Dialect            string  `json:"dialect"`
	TermName           string  `json:"term_name"`
	ScoreName          string  `json:"score_name,omitempty"`
	Weight             float64 `json:"weight,omitempty"`
	Value              float64 `json:"value,omitempty"`
	Contribution       float64 `json:"contribution,omitempty"`
	FinalScore         float64 `json:"final_score,omitempty"`
	ValueBucket        string  `json:"value_bucket,omitempty"`
	ContributionBucket string  `json:"contribution_bucket,omitempty"`
	FinalScoreBucket   string  `json:"final_score_bucket,omitempty"`
	ObservationCount   int     `json:"observation_count,omitempty"`
	Selected           bool    `json:"selected,omitempty"`
}

type policyExecutionLogRecord struct {
	Seq                    int     `json:"seq"`
	Strategy               string  `json:"strategy"`
	PolicyKind             string  `json:"policy_kind"`
	Outcome                string  `json:"outcome"`
	DurationMS             int64   `json:"duration_ms"`
	EligibleTargetCount    int     `json:"eligible_target_count"`
	AllTargetCount         int     `json:"all_target_count,omitempty"`
	SelectedCandidateIndex int     `json:"selected_candidate_index"`
	FallbackCount          int     `json:"fallback_count"`
	ClassLabel             *string `json:"class_label,omitempty"`
	ErrorClass             string  `json:"error_class,omitempty"`
	ErrorMessage           string  `json:"error_message,omitempty"`
	TerminalErrorType      string  `json:"terminal_error_type,omitempty"`
}

type fallbackTransitionLogRecord struct {
	Seq                    int    `json:"seq"`
	AttemptIndex           int    `json:"attempt_index"`
	FailedCandidateIndex   int    `json:"failed_candidate_index"`
	FallbackCandidateIndex int    `json:"fallback_candidate_index"`
	FailedProvider         string `json:"failed_provider"`
	FailedModel            string `json:"failed_model"`
	FailedDialect          string `json:"failed_dialect"`
	FallbackProvider       string `json:"fallback_provider"`
	FallbackModel          string `json:"fallback_model"`
	FallbackDialect        string `json:"fallback_dialect"`
	FallbackReason         string `json:"fallback_reason"`
	ErrorClass             string `json:"error_class"`
	Retryable              bool   `json:"retryable"`
	FallbackSucceeded      bool   `json:"fallback_succeeded"`
}

type cacheReasonLogRecord struct {
	Seq            int    `json:"seq"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	CandidateIndex int    `json:"candidate_index"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Dialect        string `json:"dialect,omitempty"`
}

func newRequestLogger(path string) (*requestLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &requestLogger{path: path, file: f}, nil
}

func (l *requestLogger) Emit(rec logRecord) {
	if l == nil {
		return
	}
	if rec.TS == "" {
		rec.TS = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(raw, '\n'))
}

func (l *requestLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
