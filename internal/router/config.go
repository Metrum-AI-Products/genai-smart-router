package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server             ServerConfig              `yaml:"server"`
	Users              []UserConfig              `yaml:"users"`
	Projects           []ProjectConfig           `yaml:"projects"`
	ProjectMemberships []ProjectMembershipConfig `yaml:"project_memberships"`
	Provider           map[string]ProviderConfig `yaml:"providers"`
	Models             map[string]ModelGroup     `yaml:"models"`
	Callers            []CallerConfig            `yaml:"callers"`
	StatePath          string                    `yaml:"state_path"`
	baseDir            string
}

type ServerConfig struct {
	Listen            string                  `yaml:"listen"`
	DefaultModelGroup string                  `yaml:"default_model_group"`
	AdminAuth         AdminAuthConfig         `yaml:"admin_auth"`
	AdminReports      AdminReportsConfig      `yaml:"admin_reports"`
	License           LicenseConfig           `yaml:"license" json:"license"`
	ClientIP          ClientIPConfig          `yaml:"client_ip" json:"client_ip"`
	Cache             CacheConfig             `yaml:"cache"`
	Logging           LoggingConfig           `yaml:"logging"`
	UsageDB           UsageDBConfig           `yaml:"usage_db"`
	Upstream          UpstreamConfig          `yaml:"upstream"`
	Diagnostics       DiagnosticsConfig       `yaml:"diagnostics"`
	ContentCapture    ContentCaptureConfig    `yaml:"content_capture"`
	Retention         RetentionConfig         `yaml:"retention"`
	DecisionTelemetry DecisionTelemetryConfig `yaml:"decision_telemetry"`
}

type LicenseConfig struct {
	Enabled                      bool          `yaml:"enabled" json:"enabled"`
	Path                         string        `yaml:"path" json:"path"`
	StatePath                    string        `yaml:"state_path" json:"statePath"`
	RecheckInterval              time.Duration `yaml:"recheck_interval" json:"recheckInterval"`
	GracePeriodOnValidationError time.Duration `yaml:"grace_period_on_validation_error" json:"gracePeriodOnValidationError"`
	FailOpenForDev               bool          `yaml:"fail_open_for_dev" json:"failOpenForDev"`
}

type AdminAuthConfig struct {
	Basic         AdminBasicAuthConfig     `yaml:"basic" json:"basic"`
	OIDC          AdminOIDCConfig          `yaml:"oidc" json:"oidc"`
	Sessions      AdminSessionConfig       `yaml:"sessions" json:"sessions"`
	Authorization AdminAuthorizationConfig `yaml:"authorization" json:"authorization"`
}

type AdminAuthorizationConfig struct {
	Enabled    bool     `yaml:"enabled" json:"enabled"`
	Source     string   `yaml:"source" json:"source"`
	PolicyFile string   `yaml:"policy_file" json:"policy_file"`
	Policy     []string `yaml:"policy" json:"policy"`
}

type AdminReportsConfig struct {
	Enabled        bool                        `yaml:"enabled" json:"enabled"`
	PathPrefix     string                      `yaml:"path_prefix" json:"path_prefix"`
	DefaultSince   string                      `yaml:"default_since" json:"default_since"`
	MaxRange       string                      `yaml:"max_range" json:"max_range"`
	MaxRows        int                         `yaml:"max_rows" json:"max_rows"`
	ExportMarkdown bool                        `yaml:"export_markdown" json:"export_markdown"`
	Baselines      []AdminReportBaselineConfig `yaml:"baselines" json:"baselines"`
	Security       AdminSecurityReportsConfig  `yaml:"security" json:"security"`
}

type AdminSecurityReportsConfig struct {
	Enabled       bool `yaml:"enabled" json:"enabled"`
	RetentionDays int  `yaml:"retention_days" json:"retention_days"`
}

type ClientIPConfig struct {
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs" json:"trusted_proxy_cidrs"`
	HeaderOrder       []string `yaml:"header_order" json:"header_order"`
	StoreIP           *bool    `yaml:"store_ip" json:"store_ip"`
}

type AdminReportBaselineConfig struct {
	ID                       string  `yaml:"id" json:"id"`
	Name                     string  `yaml:"name" json:"name"`
	PricingSource            string  `yaml:"pricing_source" json:"pricing_source"`
	PricingUpdatedAt         string  `yaml:"pricing_updated_at" json:"pricing_updated_at"`
	InputPricePerMillionUSD  float64 `yaml:"input_price_per_million_usd" json:"inputPricePerMillionUsd"`
	OutputPricePerMillionUSD float64 `yaml:"output_price_per_million_usd" json:"outputPricePerMillionUsd"`
	Notes                    string  `yaml:"notes" json:"notes"`
}

type AdminBasicAuthConfig struct {
	Enabled           bool                 `yaml:"enabled" json:"enabled"`
	Realm             string               `yaml:"realm" json:"realm"`
	AllowInsecureHTTP bool                 `yaml:"allow_insecure_http" json:"allowInsecureHttp"`
	TrustedProxyCIDRs []string             `yaml:"trusted_proxy_cidrs" json:"trustedProxyCidrs"`
	Users             []AdminBasicAuthUser `yaml:"users" json:"users"`
}

type AdminBasicAuthUser struct {
	Username        string   `yaml:"username" json:"username"`
	PasswordHashEnv string   `yaml:"password_hash_env" json:"passwordHashEnv,omitempty"`
	Subject         string   `yaml:"subject" json:"subject"`
	Domain          string   `yaml:"domain" json:"domain"`
	Permissions     []string `yaml:"permissions" json:"permissions,omitempty"`
}

type AdminOIDCConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	IssuerURL       string   `yaml:"issuer_url" json:"issuerUrl"`
	ClientIDEnv     string   `yaml:"client_id_env" json:"clientIdEnv"`
	ClientSecretEnv string   `yaml:"client_secret_env" json:"clientSecretEnv"`
	RedirectURL     string   `yaml:"redirect_url" json:"redirectUrl"`
	Scopes          []string `yaml:"scopes" json:"scopes"`
	AllowedDomains  []string `yaml:"allowed_domains" json:"allowedDomains"`
	GroupsClaim     string   `yaml:"groups_claim" json:"groupsClaim"`
	EmailClaim      string   `yaml:"email_claim" json:"emailClaim"`
	SubjectClaim    string   `yaml:"subject_claim" json:"subjectClaim"`
	Domain          string   `yaml:"domain" json:"domain"`
}

type AdminSessionConfig struct {
	CookieName    string        `yaml:"cookie_name" json:"cookieName"`
	TTL           time.Duration `yaml:"ttl" json:"ttl"`
	SecureCookies *bool         `yaml:"secure_cookies" json:"secureCookies"`
	SameSite      string        `yaml:"same_site" json:"sameSite"`
}

type UpstreamConfig struct {
	TimeoutMS               int `yaml:"timeout_ms"`
	DefaultAttemptTimeoutMS int `yaml:"default_attempt_timeout_ms"`
}

type DiagnosticsConfig struct {
	Enabled                     *bool `yaml:"enabled"`
	RetentionDays               int   `yaml:"retention_days"`
	StoreSanitizedUpstreamError bool  `yaml:"store_sanitized_upstream_errors"`
	MaxErrorBytes               int   `yaml:"max_error_bytes"`
}

type ContentCaptureConfig struct {
	Enabled                 bool                           `yaml:"enabled" json:"enabled"`
	RetentionDays           int                            `yaml:"retention_days" json:"retention_days"`
	CaptureRequest          bool                           `yaml:"capture_request" json:"capture_request"`
	CaptureResponse         bool                           `yaml:"capture_response" json:"capture_response"`
	CaptureToolCalls        bool                           `yaml:"capture_tool_calls" json:"capture_tool_calls"`
	CaptureImages           bool                           `yaml:"capture_images" json:"capture_images"`
	CaptureUpstreamErrors   bool                           `yaml:"capture_upstream_errors" json:"capture_upstream_errors"`
	CaptureHeadersAllowlist []string                       `yaml:"capture_headers_allowlist" json:"capture_headers_allowlist"`
	RedactBeforeStorage     *bool                          `yaml:"redact_before_storage" json:"redact_before_storage"`
	RedactionPatterns       []ContentCaptureRedactionRule  `yaml:"redaction_patterns" json:"redaction_patterns"`
	MaxCaptureBytes         int                            `yaml:"max_capture_bytes" json:"max_capture_bytes"`
	Encryption              ContentCaptureEncryptionConfig `yaml:"encryption" json:"encryption"`
}

type ContentCaptureRedactionRule struct {
	Name       string `yaml:"name" json:"name"`
	Expression string `yaml:"expression" json:"expression"`
}

type ContentCaptureEncryptionConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	KMSKeyID string `yaml:"kms_key_id" json:"kms_key_id"`
}

type RetentionConfig struct {
	Enabled          bool                   `yaml:"enabled" json:"enabled"`
	DryRun           *bool                  `yaml:"dry_run" json:"dry_run"`
	DefaultBatchSize int                    `yaml:"default_batch_size" json:"default_batch_size"`
	Classes          []RetentionClassConfig `yaml:"classes" json:"classes"`
}

type RetentionClassConfig struct {
	DataClass              string `yaml:"data_class" json:"data_class"`
	Enabled                *bool  `yaml:"enabled" json:"enabled"`
	RetentionDays          int    `yaml:"retention_days" json:"retention_days"`
	BatchSize              int    `yaml:"batch_size" json:"batch_size"`
	RequireFinalizedRollup bool   `yaml:"require_finalized_rollup" json:"require_finalized_rollup"`
}

type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	MaxBytes   int64         `yaml:"max_bytes"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

type LoggingConfig struct {
	Path     string `yaml:"path"`
	RotateMB int64  `yaml:"rotate_mb"`
	Keep     int    `yaml:"keep"`
}

type UsageDBConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
	DSN    string `yaml:"dsn"`
	Enable *bool  `yaml:"enabled"`
}

type DecisionTelemetryConfig struct {
	Enabled            bool  `yaml:"enabled" json:"enabled"`
	MaxCandidates      int   `yaml:"max_candidates" json:"max_candidates"`
	MaxFilterReasons   int   `yaml:"max_filter_reasons" json:"max_filter_reasons"`
	RecordCandidates   *bool `yaml:"record_candidates" json:"record_candidates"`
	RecordCacheReasons *bool `yaml:"record_cache_reasons" json:"record_cache_reasons"`
}

type ProviderConfig struct {
	BaseURL    string                   `yaml:"base_url"`
	Dialect    string                   `yaml:"dialect"`
	APIKey     string                   `yaml:"api_key"`
	APIKeyEnv  string                   `yaml:"api_key_env"`
	KeyID      string                   `yaml:"key_id"`
	AuthScheme string                   `yaml:"auth_scheme"`
	Headers    map[string]string        `yaml:"headers"`
	Models     map[string]ProviderModel `yaml:"models"`
}

type ProviderModel struct {
	Model                              string      `yaml:"model" json:"model"`
	Dialect                            string      `yaml:"dialect" json:"dialect,omitempty"`
	DisplayName                        string      `yaml:"display_name" json:"displayName,omitempty"`
	ContextTokens                      int         `yaml:"context_tokens" json:"contextTokens,omitempty"`
	InputPricePerMillionUSD            float64     `yaml:"input_price_per_million_usd" json:"inputPricePerMillionUsd,omitempty"`
	OutputPricePerMillionUSD           float64     `yaml:"output_price_per_million_usd" json:"outputPricePerMillionUsd,omitempty"`
	ImageInputPricePerMillionTokensUSD float64     `yaml:"image_input_price_per_million_tokens_usd" json:"imageInputPricePerMillionTokensUsd,omitempty"`
	ImageInputPricePerImageUSD         float64     `yaml:"image_input_price_per_image_usd" json:"imageInputPricePerImageUsd,omitempty"`
	PricingSource                      string      `yaml:"pricing_source" json:"pricingSource,omitempty"`
	PricingUpdatedAt                   string      `yaml:"pricing_updated_at" json:"pricingUpdatedAt,omitempty"`
	PricingNotes                       string      `yaml:"pricing_notes" json:"pricingNotes,omitempty"`
	ToolSupport                        ToolSupport `yaml:"tool_support" json:"toolSupport,omitempty"`
	InputModalities                    []string    `yaml:"input_modalities" json:"inputModalities,omitempty"`
	OutputModalities                   []string    `yaml:"output_modalities" json:"outputModalities,omitempty"`
	HonorsMaxTokens                    *bool       `yaml:"honors_max_tokens" json:"honorsMaxTokens,omitempty"`
	// Weight is accepted for legacy configs but intentionally ignored.
	// Routing weights are group-local and belong on ModelGroup targets.
	Weight int    `yaml:"weight" json:"weight,omitempty"`
	RPM    int    `yaml:"rpm" json:"rpm,omitempty"`
	Tier   string `yaml:"tier" json:"tier,omitempty"`
	Cost   int    `yaml:"cost" json:"cost,omitempty"`
}

type ToolSupport struct {
	OpenAIChat        []string `yaml:"openai_chat" json:"openaiChat,omitempty"`
	OpenAIResponses   []string `yaml:"openai_responses" json:"openaiResponses,omitempty"`
	AnthropicMessages []string `yaml:"anthropic_messages" json:"anthropicMessages,omitempty"`
	ProviderHosted    []string `yaml:"provider_hosted" json:"providerHosted,omitempty"`
}

type ModelGroup struct {
	Strategy         string               `yaml:"strategy"`
	Script           string               `yaml:"script"`
	ScriptHTTP       ScriptHTTPConfig     `yaml:"script_http"`
	ExternalPolicy   ExternalPolicyConfig `yaml:"external_policy"`
	RoutingPolicy    RoutingPolicyConfig  `yaml:"routing_policy"`
	Contract         *ModelGroupContract  `yaml:"contract" json:"contract,omitempty"`
	PIIFilter        PIIFilterConfig      `yaml:"pii_filter"`
	ContentCapture   ContentCaptureConfig `yaml:"content_capture"`
	AttemptTimeoutMS int                  `yaml:"attempt_timeout_ms"`
	Targets          []Target             `yaml:"targets"`
}

type ModelGroupContract struct {
	DisplayName        string                       `yaml:"display_name" json:"displayName,omitempty"`
	CallerVisibleNotes string                       `yaml:"caller_visible_notes" json:"callerVisibleNotes,omitempty"`
	IntendedWorkloads  []string                     `yaml:"intended_workloads" json:"intendedWorkloads,omitempty"`
	SupportedAPIShapes []string                     `yaml:"supported_api_shapes" json:"supportedApiShapes,omitempty"`
	RequiredCaps       ContractRequiredCapabilities `yaml:"required_capabilities" json:"requiredCapabilities,omitempty"`
	QualityFloor       ContractQualityFloor         `yaml:"quality_floor" json:"qualityFloor,omitempty"`
	OperationalTargets ContractOperationalTargets   `yaml:"operational_targets" json:"operationalTargets,omitempty"`
	Reporting          ContractReporting            `yaml:"reporting" json:"reporting,omitempty"`
}

type ContractRequiredCapabilities struct {
	Tools                           bool     `yaml:"tools" json:"tools,omitempty"`
	ForcedToolChoice                bool     `yaml:"forced_tool_choice" json:"forcedToolChoice,omitempty"`
	StructuredOutputs               bool     `yaml:"structured_outputs" json:"structuredOutputs,omitempty"`
	InputModalities                 []string `yaml:"input_modalities" json:"inputModalities,omitempty"`
	OutputModalities                []string `yaml:"output_modalities" json:"outputModalities,omitempty"`
	MinContextTokens                int      `yaml:"min_context_tokens" json:"minContextTokens,omitempty"`
	HonorsMaxTokensWhenCallerCapped bool     `yaml:"honors_max_tokens_when_caller_capped" json:"honorsMaxTokensWhenCallerCapped,omitempty"`
}

type ContractQualityFloor struct {
	RequireTags             []string `yaml:"require_tags" json:"requireTags,omitempty"`
	MinEvalQualityScore     *float64 `yaml:"min_eval_quality_score" json:"minEvalQualityScore,omitempty"`
	MinEvalPassRate         *float64 `yaml:"min_eval_pass_rate" json:"minEvalPassRate,omitempty"`
	MaxEvalAgeDays          int      `yaml:"max_eval_age_days" json:"maxEvalAgeDays,omitempty"`
	AllowedValidationStatus []string `yaml:"allowed_validation_status" json:"allowedValidationStatus,omitempty"`
}

type ContractOperationalTargets struct {
	MaxP95LatencyMS          *float64 `yaml:"max_p95_latency_ms" json:"maxP95LatencyMs,omitempty"`
	MaxErrorRate             *float64 `yaml:"max_error_rate" json:"maxErrorRate,omitempty"`
	MaxTimeoutRate           *float64 `yaml:"max_timeout_rate" json:"maxTimeoutRate,omitempty"`
	MinOutputTokensPerSecond *float64 `yaml:"min_output_tokens_per_second" json:"minOutputTokensPerSecond,omitempty"`
}

type ContractReporting struct {
	ExposeWorkloadLabels     bool `yaml:"expose_workload_labels" json:"exposeWorkloadLabels,omitempty"`
	ExposeQualityFloorBucket bool `yaml:"expose_quality_floor_bucket" json:"exposeQualityFloorBucket,omitempty"`
}

type RoutingPolicyConfig struct {
	DynamicScore DynamicScoreConfig `yaml:"dynamic_score" json:"dynamicScore,omitempty"`
}

type DynamicScoreConfig struct {
	ColdStartPolicy           string                    `yaml:"cold_start_policy" json:"coldStartPolicy,omitempty"`
	MinObservations           int                       `yaml:"min_observations" json:"minObservations,omitempty"`
	ObservationWindowSeconds  int                       `yaml:"observation_window_seconds" json:"observationWindowSeconds,omitempty"`
	MaxScoreAdjustmentPercent float64                   `yaml:"max_score_adjustment_percent" json:"maxScoreAdjustmentPercent,omitempty"`
	LowConfidenceFallback     string                    `yaml:"low_confidence_fallback" json:"lowConfidenceFallback,omitempty"`
	HardFilters               DynamicScoreHardFilters   `yaml:"hard_filters" json:"hardFilters,omitempty"`
	Signals                   DynamicScoreSignals       `yaml:"signals" json:"signals,omitempty"`
	ScoreTerms                []DynamicScoreTerm        `yaml:"score_terms" json:"scoreTerms,omitempty"`
	Thresholds                DynamicScoreThresholds    `yaml:"thresholds" json:"thresholds,omitempty"`
	EvaluationMetadata        []DynamicEvaluationTarget `yaml:"evaluation_metadata" json:"evaluationMetadata,omitempty"`
}

type DynamicScoreHardFilters struct {
	RequireRequestedAPISkin                bool `yaml:"require_requested_api_skin" json:"requireRequestedApiSkin,omitempty"`
	RequireInputModalities                 bool `yaml:"require_input_modalities" json:"requireInputModalities,omitempty"`
	RequireToolSupportWhenToolsPresent     bool `yaml:"require_tool_support_when_tools_present" json:"requireToolSupportWhenToolsPresent,omitempty"`
	RequireForcedToolChoiceSupport         bool `yaml:"require_forced_tool_choice_support" json:"requireForcedToolChoiceSupport,omitempty"`
	RequireStructuredOutputSupport         bool `yaml:"require_structured_output_support" json:"requireStructuredOutputSupport,omitempty"`
	RequireHonorsMaxTokensWhenCallerCapped bool `yaml:"require_honors_max_tokens_when_caller_capped" json:"requireHonorsMaxTokensWhenCallerCapped,omitempty"`
}

type DynamicScoreSignals struct {
	RequestShape        DynamicSignalRequestShape        `yaml:"request_shape" json:"requestShape,omitempty"`
	PromptFeatures      DynamicSignalPromptFeatures      `yaml:"prompt_features" json:"promptFeatures,omitempty"`
	Complexity          DynamicSignalComplexity          `yaml:"complexity" json:"complexity,omitempty"`
	ObservedPerformance DynamicSignalObservedPerformance `yaml:"observed_performance" json:"observedPerformance,omitempty"`
	Cost                DynamicSignalNamedFields         `yaml:"cost" json:"cost,omitempty"`
	BudgetPressure      DynamicSignalBudgetPressure      `yaml:"budget_pressure" json:"budgetPressure,omitempty"`
	EvaluationMetadata  DynamicSignalEvaluationMetadata  `yaml:"evaluation_metadata" json:"evaluationMetadata,omitempty"`
	RecentPenalties     DynamicSignalRecentPenalties     `yaml:"recent_penalties" json:"recentPenalties,omitempty"`
}

type DynamicSignalRequestShape struct {
	Enabled bool     `yaml:"enabled" json:"enabled,omitempty"`
	Fields  []string `yaml:"fields" json:"fields,omitempty"`
}

type DynamicSignalPromptFeatures struct {
	Enabled      bool     `yaml:"enabled" json:"enabled,omitempty"`
	MaxScanBytes int      `yaml:"max_scan_bytes" json:"maxScanBytes,omitempty"`
	Features     []string `yaml:"features" json:"features,omitempty"`
}

type DynamicSignalComplexity struct {
	Enabled bool               `yaml:"enabled" json:"enabled,omitempty"`
	Weights map[string]float64 `yaml:"weights" json:"weights,omitempty"`
}

type DynamicSignalObservedPerformance struct {
	Enabled bool     `yaml:"enabled" json:"enabled,omitempty"`
	Fields  []string `yaml:"fields" json:"fields,omitempty"`
}

type DynamicSignalNamedFields struct {
	Enabled bool     `yaml:"enabled" json:"enabled,omitempty"`
	Fields  []string `yaml:"fields" json:"fields,omitempty"`
}

type DynamicSignalBudgetPressure struct {
	Enabled bool     `yaml:"enabled" json:"enabled,omitempty"`
	Buckets []string `yaml:"buckets" json:"buckets,omitempty"`
}

type DynamicSignalEvaluationMetadata struct {
	Enabled    bool     `yaml:"enabled" json:"enabled,omitempty"`
	MaxAgeDays int      `yaml:"max_age_days" json:"maxAgeDays,omitempty"`
	Fields     []string `yaml:"fields" json:"fields,omitempty"`
}

type DynamicSignalRecentPenalties struct {
	Enabled    bool     `yaml:"enabled" json:"enabled,omitempty"`
	TTLSeconds int      `yaml:"ttl_seconds" json:"ttlSeconds,omitempty"`
	Reasons    []string `yaml:"reasons" json:"reasons,omitempty"`
}

type DynamicScoreTerm struct {
	Name        string             `yaml:"name" json:"name,omitempty"`
	When        map[string]any     `yaml:"when" json:"when,omitempty"`
	RequireTags []string           `yaml:"require_tags" json:"requireTags,omitempty"`
	PreferTags  []string           `yaml:"prefer_tags" json:"preferTags,omitempty"`
	Expression  string             `yaml:"expression" json:"expression,omitempty"`
	Weights     map[string]float64 `yaml:"weights" json:"weights,omitempty"`
}

type DynamicScoreThresholds struct {
	MaxErrorRate             *float64 `yaml:"max_error_rate" json:"maxErrorRate,omitempty"`
	MaxTimeoutRate           *float64 `yaml:"max_timeout_rate" json:"maxTimeoutRate,omitempty"`
	MaxP95LatencyMS          *float64 `yaml:"max_p95_latency_ms" json:"maxP95LatencyMs,omitempty"`
	MaxTTFBMS                *float64 `yaml:"max_ttfb_ms" json:"maxTtfbMs,omitempty"`
	MinOutputTokensPerSecond *float64 `yaml:"min_output_tokens_per_second" json:"minOutputTokensPerSecond,omitempty"`
}

type DynamicEvaluationTarget struct {
	Provider       string   `yaml:"provider" json:"provider,omitempty"`
	Model          string   `yaml:"model" json:"model,omitempty"`
	QualityScore   float64  `yaml:"quality_score" json:"qualityScore,omitempty"`
	PassRate       float64  `yaml:"pass_rate" json:"passRate,omitempty"`
	Workload       string   `yaml:"workload" json:"workload,omitempty"`
	ValidationDate string   `yaml:"validation_date" json:"validationDate,omitempty"`
	Tags           []string `yaml:"tags" json:"tags,omitempty"`
}

type PIIFilterConfig struct {
	Enabled                   bool             `yaml:"enabled" json:"enabled"`
	Mode                      string           `yaml:"mode" json:"mode"`
	FailOnMatch               bool             `yaml:"fail_on_match" json:"failOnMatch"`
	ApplyTo                   PIIFilterApplyTo `yaml:"apply_to" json:"applyTo"`
	RestoreResponse           *bool            `yaml:"restore_response" json:"restoreResponse"`
	MaxReplacementsPerRequest int              `yaml:"max_replacements_per_request" json:"maxReplacementsPerRequest"`
	Rules                     []PIIFilterRule  `yaml:"rules" json:"rules"`
}

type PIIFilterApplyTo struct {
	System         *bool `yaml:"system" json:"system"`
	Messages       *bool `yaml:"messages" json:"messages"`
	ResponsesInput *bool `yaml:"responses_input" json:"responsesInput"`
	ToolResults    *bool `yaml:"tool_results" json:"toolResults"`
	ImageURLs      bool  `yaml:"image_urls" json:"imageUrls"`
}

type PIIFilterRule struct {
	Name              string `yaml:"name" json:"name"`
	Expression        string `yaml:"expression" json:"expression"`
	PlaceholderPrefix string `yaml:"placeholder_prefix" json:"placeholderPrefix"`
}

type ScriptHTTPConfig struct {
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	AllowHosts       []string          `yaml:"allow_hosts" json:"allowHosts"`
	AllowHTTP        bool              `yaml:"allow_http" json:"allowHttp"`
	TimeoutMS        int               `yaml:"timeout_ms" json:"timeoutMs"`
	MaxResponseBytes int64             `yaml:"max_response_bytes" json:"maxResponseBytes"`
	Headers          map[string]string `yaml:"headers" json:"headers"`
}

type ExternalPolicyConfig struct {
	URL              string            `yaml:"url" json:"url"`
	Method           string            `yaml:"method" json:"method"`
	AllowHosts       []string          `yaml:"allow_hosts" json:"allowHosts"`
	AllowHTTP        bool              `yaml:"allow_http" json:"allowHttp"`
	TimeoutMS        int               `yaml:"timeout_ms" json:"timeoutMs"`
	MaxResponseBytes int64             `yaml:"max_response_bytes" json:"maxResponseBytes"`
	Headers          map[string]string `yaml:"headers" json:"headers"`
	OnError          string            `yaml:"on_error" json:"onError"`
	IncludeRequest   bool              `yaml:"include_request" json:"includeRequest"`
}

type Target struct {
	Provider                           string            `yaml:"provider" json:"provider"`
	Model                              string            `yaml:"model" json:"model"`
	ModelRef                           string            `yaml:"model_ref" json:"modelRef,omitempty"`
	Dialect                            string            `yaml:"dialect" json:"dialect"`
	DisplayName                        string            `yaml:"display_name" json:"displayName,omitempty"`
	ContextTokens                      int               `yaml:"context_tokens" json:"contextTokens,omitempty"`
	ToolOnly                           bool              `yaml:"tool_only" json:"toolOnly,omitempty"`
	TimeoutMS                          int               `yaml:"timeout_ms" json:"timeoutMs,omitempty"`
	DefaultThinking                    map[string]any    `yaml:"default_thinking" json:"defaultThinking,omitempty"`
	Tags                               []string          `yaml:"tags" json:"tags,omitempty"`
	Weight                             int               `yaml:"weight" json:"weight"`
	RPM                                int               `yaml:"rpm" json:"rpm"`
	Tier                               string            `yaml:"tier" json:"tier"`
	Cost                               int               `yaml:"cost" json:"cost"`
	InputPricePerMillionUSD            float64           `yaml:"input_price_per_million_usd" json:"inputPricePerMillionUsd,omitempty"`
	OutputPricePerMillionUSD           float64           `yaml:"output_price_per_million_usd" json:"outputPricePerMillionUsd,omitempty"`
	ImageInputPricePerMillionTokensUSD float64           `yaml:"image_input_price_per_million_tokens_usd" json:"imageInputPricePerMillionTokensUsd,omitempty"`
	ImageInputPricePerImageUSD         float64           `yaml:"image_input_price_per_image_usd" json:"imageInputPricePerImageUsd,omitempty"`
	PricingSource                      string            `yaml:"pricing_source" json:"pricingSource,omitempty"`
	PricingUpdatedAt                   string            `yaml:"pricing_updated_at" json:"pricingUpdatedAt,omitempty"`
	PricingNotes                       string            `yaml:"pricing_notes" json:"pricingNotes,omitempty"`
	ToolSupport                        ToolSupport       `yaml:"tool_support" json:"toolSupport,omitempty"`
	InputModalities                    []string          `yaml:"input_modalities" json:"inputModalities,omitempty"`
	OutputModalities                   []string          `yaml:"output_modalities" json:"outputModalities,omitempty"`
	HonorsMaxTokens                    *bool             `yaml:"honors_max_tokens" json:"honorsMaxTokens,omitempty"`
	Validation                         *TargetValidation `yaml:"validation" json:"validation,omitempty"`
}

type TargetValidation struct {
	Status       string  `yaml:"status" json:"status,omitempty"`
	Workload     string  `yaml:"workload" json:"workload,omitempty"`
	ValidatedAt  string  `yaml:"validated_at" json:"validatedAt,omitempty"`
	QualityScore float64 `yaml:"quality_score" json:"qualityScore,omitempty"`
	PassRate     float64 `yaml:"pass_rate" json:"passRate,omitempty"`
	Harness      string  `yaml:"harness" json:"harness,omitempty"`
	Notes        string  `yaml:"notes" json:"notes,omitempty"`
}

type CallerConfig struct {
	ID             string               `yaml:"id" json:"id"`
	OwnerUser      string               `yaml:"owner_user" json:"owner_user"`
	User           string               `yaml:"user" json:"user"`
	Project        string               `yaml:"project" json:"project"`
	Environment    string               `yaml:"environment" json:"environment"`
	Status         string               `yaml:"status" json:"status"`
	TokenSHA256    string               `yaml:"token_sha256" json:"token_sha256"`
	TokenID        string               `yaml:"token_id" json:"token_id"`
	Allow          []string             `yaml:"allow" json:"allow"`
	MetricsAdmin   bool                 `yaml:"metrics_admin" json:"metrics_admin"`
	ContentAdmin   bool                 `yaml:"content_admin" json:"content_admin"`
	ContentCapture ContentCaptureConfig `yaml:"content_capture" json:"content_capture"`
	Rate           RateConfig           `yaml:"rate" json:"rate"`
	Quota          QuotaConfig          `yaml:"quota" json:"quota"`
	Key            KeyConfig            `yaml:"key" json:"key"`
}

type UserConfig struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Email       string `yaml:"email" json:"email"`
	Type        string `yaml:"type" json:"type"`
	Status      string `yaml:"status" json:"status"`
	Description string `yaml:"description" json:"description"`
}

type ProjectConfig struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Status      string `yaml:"status" json:"status"`
	Description string `yaml:"description" json:"description"`
}

type ProjectMembershipConfig struct {
	UserID   string `yaml:"user_id" json:"user_id"`
	Project  string `yaml:"project" json:"project"`
	Role     string `yaml:"role" json:"role"`
	Status   string `yaml:"status" json:"status"`
	Source   string `yaml:"source" json:"source"`
	JoinedAt string `yaml:"joined_at" json:"joined_at"`
}

type RateConfig struct {
	RPM        int `yaml:"rpm" json:"rpm"`
	TPM        int `yaml:"tpm" json:"tpm"`
	Concurrent int `yaml:"concurrent" json:"concurrent"`
}

type QuotaConfig struct {
	Day     BudgetConfig `yaml:"day" json:"day"`
	Month   BudgetConfig `yaml:"month" json:"month"`
	SoftPct int          `yaml:"soft_pct" json:"soft_pct"`
}

type BudgetConfig struct {
	Requests int64 `yaml:"requests" json:"requests"`
	Tokens   int64 `yaml:"tokens" json:"tokens"`
}

type KeyConfig struct {
	LifetimeTokens int64  `yaml:"lifetime_tokens" json:"lifetime_tokens"`
	SoftPct        int    `yaml:"soft_pct" json:"soft_pct"`
	OnExhaust      string `yaml:"on_exhaust" json:"on_exhaust"`
}

func LoadConfig(path string) (*Config, error) {
	if err := loadEnvJSON(filepath.Join(filepath.Dir(path), "env.json")); err != nil {
		return nil, err
	}
	if filepath.Dir(path) != "." {
		if err := loadEnvJSON("env.json"); err != nil {
			return nil, err
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expanded := os.ExpandEnv(string(raw))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	cfg.setDefaults()
	cfg.baseDir = filepath.Dir(path)
	return &cfg, cfg.Validate()
}

func loadEnvJSON(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	env := map[string]string{}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	for k, v := range env {
		if !validEnvName(k) {
			return fmt.Errorf("load %s: invalid env key %q", path, k)
		}
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
	return nil
}

func validEnvName(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		if r == '_' || ('A' <= r && r <= 'Z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (c *Config) setDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = ":8080"
	}
	if c.Server.AdminAuth.Basic.Realm == "" {
		c.Server.AdminAuth.Basic.Realm = "GenAI Smart Router Admin"
	}
	if c.Server.AdminAuth.OIDC.GroupsClaim == "" {
		c.Server.AdminAuth.OIDC.GroupsClaim = "groups"
	}
	if c.Server.AdminAuth.OIDC.EmailClaim == "" {
		c.Server.AdminAuth.OIDC.EmailClaim = "email"
	}
	if c.Server.AdminAuth.OIDC.SubjectClaim == "" {
		c.Server.AdminAuth.OIDC.SubjectClaim = "email"
	}
	if c.Server.AdminAuth.OIDC.Domain == "" {
		c.Server.AdminAuth.OIDC.Domain = "default"
	}
	if c.Server.AdminAuth.Sessions.CookieName == "" {
		c.Server.AdminAuth.Sessions.CookieName = "smart_router_admin_session"
	}
	if c.Server.AdminAuth.Sessions.TTL == 0 {
		c.Server.AdminAuth.Sessions.TTL = 8 * time.Hour
	}
	if c.Server.AdminAuth.Sessions.SameSite == "" {
		c.Server.AdminAuth.Sessions.SameSite = "strict"
	}
	if c.Server.AdminAuth.Sessions.SecureCookies == nil {
		secure := true
		c.Server.AdminAuth.Sessions.SecureCookies = &secure
	}
	if c.Server.AdminReports.PathPrefix == "" {
		c.Server.AdminReports.PathPrefix = "/admin/reports"
	}
	if c.Server.AdminReports.DefaultSince == "" {
		c.Server.AdminReports.DefaultSince = "24h"
	}
	if c.Server.AdminReports.MaxRange == "" {
		c.Server.AdminReports.MaxRange = "31d"
	}
	if c.Server.AdminReports.MaxRows == 0 {
		c.Server.AdminReports.MaxRows = 500
	}
	if c.Server.AdminReports.Security.RetentionDays == 0 {
		c.Server.AdminReports.Security.RetentionDays = 90
	}
	if len(c.Server.AdminReports.Baselines) == 0 {
		c.Server.AdminReports.Baselines = defaultAdminReportBaselines()
	}
	if c.Server.License.RecheckInterval == 0 {
		c.Server.License.RecheckInterval = time.Hour
	}
	if len(c.Server.ClientIP.HeaderOrder) == 0 {
		c.Server.ClientIP.HeaderOrder = []string{"X-Forwarded-For", "X-Real-IP"}
	}
	if c.Server.ClientIP.StoreIP == nil {
		storeIP := true
		c.Server.ClientIP.StoreIP = &storeIP
	}
	if c.Server.Cache.MaxBytes == 0 {
		c.Server.Cache.MaxBytes = 128 << 20
	}
	if c.Server.Cache.DefaultTTL == 0 {
		c.Server.Cache.DefaultTTL = 15 * time.Minute
	}
	if c.StatePath == "" {
		c.StatePath = "router-state.json"
	}
	if c.Server.Logging.Path == "" {
		c.Server.Logging.Path = "requests.jsonl"
	}
	if c.Server.Logging.RotateMB == 0 {
		c.Server.Logging.RotateMB = 256
	}
	if c.Server.Logging.Keep == 0 {
		c.Server.Logging.Keep = 14
	}
	if c.Server.UsageDB.Driver == "" {
		c.Server.UsageDB.Driver = "sqlite"
	}
	if c.Server.UsageDB.Path == "" && c.Server.UsageDB.DSN == "" && strings.EqualFold(c.Server.UsageDB.Driver, "sqlite") {
		dir := filepath.Dir(c.StatePath)
		if dir == "." {
			c.Server.UsageDB.Path = "usage.sqlite"
		} else {
			c.Server.UsageDB.Path = filepath.Join(dir, "usage.sqlite")
		}
	}
	if c.Server.Upstream.TimeoutMS == 0 {
		c.Server.Upstream.TimeoutMS = int((10 * time.Minute).Milliseconds())
	}
	if c.Server.Diagnostics.RetentionDays == 0 {
		c.Server.Diagnostics.RetentionDays = 30
	}
	if c.Server.Diagnostics.MaxErrorBytes == 0 {
		c.Server.Diagnostics.MaxErrorBytes = 2048
	}
	if c.Server.DecisionTelemetry.MaxCandidates == 0 {
		c.Server.DecisionTelemetry.MaxCandidates = 64
	}
	if c.Server.DecisionTelemetry.MaxFilterReasons == 0 {
		c.Server.DecisionTelemetry.MaxFilterReasons = 256
	}
	if c.Server.DecisionTelemetry.RecordCandidates == nil {
		record := true
		c.Server.DecisionTelemetry.RecordCandidates = &record
	}
	if c.Server.DecisionTelemetry.RecordCacheReasons == nil {
		record := true
		c.Server.DecisionTelemetry.RecordCacheReasons = &record
	}
	defaultContentCaptureConfig(&c.Server.ContentCapture)
	defaultRetentionConfig(&c.Server.Retention)
	for i := range c.Users {
		c.Users[i].ID = normalizeAccountID(c.Users[i].ID)
		c.Users[i].Status = normalizeStatusDefault(c.Users[i].Status)
		c.Users[i].Type = normalizeUserTypeDefault(c.Users[i].Type)
	}
	for i := range c.Projects {
		c.Projects[i].ID = normalizeAccountID(c.Projects[i].ID)
		c.Projects[i].Status = normalizeStatusDefault(c.Projects[i].Status)
	}
	for i := range c.ProjectMemberships {
		c.ProjectMemberships[i].UserID = normalizeAccountID(c.ProjectMemberships[i].UserID)
		c.ProjectMemberships[i].Project = normalizeAccountID(c.ProjectMemberships[i].Project)
		c.ProjectMemberships[i].Status = normalizeStatusDefault(c.ProjectMemberships[i].Status)
		c.ProjectMemberships[i].Role = normalizeRoleDefault(c.ProjectMemberships[i].Role)
	}
	for i := range c.Callers {
		if c.Callers[i].OwnerUser == "" {
			c.Callers[i].OwnerUser = c.Callers[i].User
		}
		c.Callers[i].OwnerUser = normalizeAccountID(c.Callers[i].OwnerUser)
		c.Callers[i].Project = normalizeAccountID(c.Callers[i].Project)
		c.Callers[i].Environment = normalizeAccountID(c.Callers[i].Environment)
		c.Callers[i].Status = normalizeStatusDefault(c.Callers[i].Status)
		defaultContentCaptureConfig(&c.Callers[i].ContentCapture)
	}
	for name, group := range c.Models {
		defaultContentCaptureConfig(&group.ContentCapture)
		c.Models[name] = group
	}
}

func (c *Config) Validate() error {
	if len(c.Provider) == 0 {
		return fmt.Errorf("at least one provider is required")
	}
	if c.Server.Upstream.TimeoutMS < 0 {
		return fmt.Errorf("server upstream timeout_ms cannot be negative")
	}
	if c.Server.Upstream.DefaultAttemptTimeoutMS < 0 {
		return fmt.Errorf("server upstream default_attempt_timeout_ms cannot be negative")
	}
	if c.Server.Diagnostics.RetentionDays < 0 {
		return fmt.Errorf("server diagnostics retention_days cannot be negative")
	}
	if c.Server.Diagnostics.MaxErrorBytes < 0 {
		return fmt.Errorf("server diagnostics max_error_bytes cannot be negative")
	}
	if c.Server.DecisionTelemetry.MaxCandidates < 0 || c.Server.DecisionTelemetry.MaxCandidates > 1000 {
		return fmt.Errorf("server decision_telemetry max_candidates must be between 1 and 1000 when set")
	}
	if c.Server.DecisionTelemetry.MaxFilterReasons < 0 || c.Server.DecisionTelemetry.MaxFilterReasons > 10000 {
		return fmt.Errorf("server decision_telemetry max_filter_reasons must be between 1 and 10000 when set")
	}
	if c.Server.DecisionTelemetry.Enabled && c.Server.UsageDB.Enable != nil && !*c.Server.UsageDB.Enable {
		return fmt.Errorf("server decision_telemetry requires usage_db enabled")
	}
	if err := validateLicenseConfig(c.Server.License); err != nil {
		return err
	}
	if err := validateAdminAuth(c.Server.AdminAuth, c.Server.UsageDB); err != nil {
		return err
	}
	if err := validateAdminReports(c.Server.AdminReports, c.Server.AdminAuth, c.Server.UsageDB); err != nil {
		return err
	}
	if err := validateClientIP(c.Server.ClientIP); err != nil {
		return err
	}
	if err := validateContentCapture("server content_capture", c.Server.ContentCapture); err != nil {
		return err
	}
	if c.contentCaptureEnabled() && c.Server.UsageDB.Enable != nil && !*c.Server.UsageDB.Enable {
		return fmt.Errorf("content_capture requires usage_db enabled")
	}
	if err := validateRetentionConfig(c.Server.Retention); err != nil {
		return err
	}
	if c.Server.Retention.Enabled && c.Server.UsageDB.Enable != nil && !*c.Server.UsageDB.Enable {
		return fmt.Errorf("server retention requires usage_db enabled")
	}
	if _, err := c.validateAccounts(); err != nil {
		return err
	}
	for name, p := range c.Provider {
		if p.BaseURL == "" {
			return fmt.Errorf("provider %s missing base_url", name)
		}
		if normalizeDialect(p.Dialect) == "" {
			return fmt.Errorf("provider %s has unsupported dialect %q", name, p.Dialect)
		}
		if p.AuthScheme != "" && normalizeAuthScheme(p.AuthScheme) == "" {
			return fmt.Errorf("provider %s has unsupported auth_scheme %q", name, p.AuthScheme)
		}
		for ref, model := range p.Models {
			if model.Model == "" {
				return fmt.Errorf("provider %s model %s missing model", name, ref)
			}
			if model.Dialect != "" && normalizeDialect(model.Dialect) == "" {
				return fmt.Errorf("provider %s model %s has unsupported dialect %q", name, ref, model.Dialect)
			}
			if model.ContextTokens < 0 {
				return fmt.Errorf("provider %s model %s has negative context_tokens", name, ref)
			}
			if model.InputPricePerMillionUSD < 0 {
				return fmt.Errorf("provider %s model %s has negative input_price_per_million_usd", name, ref)
			}
			if model.OutputPricePerMillionUSD < 0 {
				return fmt.Errorf("provider %s model %s has negative output_price_per_million_usd", name, ref)
			}
			if model.ImageInputPricePerMillionTokensUSD < 0 {
				return fmt.Errorf("provider %s model %s has negative image_input_price_per_million_tokens_usd", name, ref)
			}
			if model.ImageInputPricePerImageUSD < 0 {
				return fmt.Errorf("provider %s model %s has negative image_input_price_per_image_usd", name, ref)
			}
			if err := validateToolSupport(model.ToolSupport); err != nil {
				return fmt.Errorf("provider %s model %s has invalid tool_support: %w", name, ref, err)
			}
			if err := validateModalities(model.InputModalities); err != nil {
				return fmt.Errorf("provider %s model %s has invalid input_modalities: %w", name, ref, err)
			}
			if err := validateModalities(model.OutputModalities); err != nil {
				return fmt.Errorf("provider %s model %s has invalid output_modalities: %w", name, ref, err)
			}
		}
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model group is required")
	}
	if c.Server.DefaultModelGroup != "" {
		if _, ok := c.Models[c.Server.DefaultModelGroup]; !ok {
			return fmt.Errorf("server default_model_group references unknown model group %s", c.Server.DefaultModelGroup)
		}
	}
	for name, m := range c.Models {
		if m.Strategy == "" {
			return fmt.Errorf("model group %s missing strategy", name)
		}
		if strings.EqualFold(m.Strategy, "script") && m.Script == "" {
			return fmt.Errorf("model group %s uses script strategy but has no script path", name)
		}
		if strings.EqualFold(m.Strategy, "external") {
			if strings.TrimSpace(m.ExternalPolicy.URL) == "" {
				return fmt.Errorf("model group %s uses external strategy but has no external_policy.url", name)
			}
			if len(m.ExternalPolicy.AllowHosts) == 0 {
				return fmt.Errorf("model group %s uses external strategy but has no external_policy.allow_hosts", name)
			}
			policyURL, err := url.Parse(m.ExternalPolicy.URL)
			if err != nil || policyURL.Hostname() == "" {
				return fmt.Errorf("model group %s external_policy.url is invalid", name)
			}
			if policyURL.Scheme != "http" && policyURL.Scheme != "https" {
				return fmt.Errorf("model group %s external_policy.url scheme must be http or https", name)
			}
			if !scriptHostAllowed(policyURL.Hostname(), m.ExternalPolicy.AllowHosts) {
				return fmt.Errorf("model group %s external_policy.url host %s is not in allow_hosts", name, policyURL.Hostname())
			}
			if policyURL.Scheme == "http" && !m.ExternalPolicy.AllowHTTP && !egressHostIsTrustedLocal(policyURL.Hostname()) {
				return fmt.Errorf("model group %s external_policy.url uses http; set external_policy.allow_http for non-local plaintext policy services", name)
			}
			if m.ExternalPolicy.Method != "" && strings.ToUpper(strings.TrimSpace(m.ExternalPolicy.Method)) != http.MethodGet && strings.ToUpper(strings.TrimSpace(m.ExternalPolicy.Method)) != http.MethodPost {
				return fmt.Errorf("model group %s external_policy method must be GET or POST", name)
			}
			if m.ExternalPolicy.TimeoutMS < 0 {
				return fmt.Errorf("model group %s has negative external_policy timeout_ms", name)
			}
			if m.ExternalPolicy.TimeoutMS > 5000 {
				return fmt.Errorf("model group %s external_policy timeout_ms must be <= 5000", name)
			}
			if m.ExternalPolicy.MaxResponseBytes < 0 {
				return fmt.Errorf("model group %s has negative external_policy max_response_bytes", name)
			}
			switch strings.ToLower(strings.TrimSpace(m.ExternalPolicy.OnError)) {
			case "", "fail_closed", "fallback":
			default:
				return fmt.Errorf("model group %s external_policy on_error must be fail_closed or fallback", name)
			}
			for header := range m.ExternalPolicy.Headers {
				if !scriptConfigHeaderAllowed(header) {
					return fmt.Errorf("model group %s external_policy header %s is not allowed", name, header)
				}
			}
		} else if !externalPolicyEmpty(m.ExternalPolicy) {
			return fmt.Errorf("model group %s configures external_policy but does not use external strategy", name)
		}
		if m.AttemptTimeoutMS < 0 {
			return fmt.Errorf("model group %s attempt_timeout_ms cannot be negative", name)
		}
		if strings.EqualFold(m.Strategy, "dynamic_score") {
			if err := validateDynamicScorePolicy(name, m.RoutingPolicy.DynamicScore); err != nil {
				return err
			}
		}
		if err := validatePIIFilter(name, m.PIIFilter); err != nil {
			return err
		}
		if err := validateContentCapture("model group "+name+" content_capture", m.ContentCapture); err != nil {
			return err
		}
		if m.ScriptHTTP.Enabled {
			if !strings.EqualFold(m.Strategy, "script") {
				return fmt.Errorf("model group %s configures script_http but does not use script strategy", name)
			}
			if len(m.ScriptHTTP.AllowHosts) == 0 {
				return fmt.Errorf("model group %s enables script_http but has no allow_hosts", name)
			}
			if m.ScriptHTTP.TimeoutMS < 0 {
				return fmt.Errorf("model group %s has negative script_http timeout_ms", name)
			}
			if m.ScriptHTTP.TimeoutMS > 5000 {
				return fmt.Errorf("model group %s script_http timeout_ms must be <= 5000", name)
			}
			if m.ScriptHTTP.MaxResponseBytes < 0 {
				return fmt.Errorf("model group %s has negative script_http max_response_bytes", name)
			}
			for header := range m.ScriptHTTP.Headers {
				if !scriptConfigHeaderAllowed(header) {
					return fmt.Errorf("model group %s script_http header %s is not allowed", name, header)
				}
			}
		}
		if len(m.Targets) == 0 {
			return fmt.Errorf("model group %s has no targets", name)
		}
		for i, t := range m.Targets {
			resolved, err := c.resolveTarget(name, t)
			if err != nil {
				return err
			}
			m.Targets[i] = resolved
			if _, ok := c.Provider[resolved.Provider]; !ok {
				return fmt.Errorf("model group %s references unknown provider %s", name, t.Provider)
			}
			if resolved.InputPricePerMillionUSD < 0 {
				return fmt.Errorf("model group %s target %s has negative input_price_per_million_usd", name, resolved.Model)
			}
			if resolved.OutputPricePerMillionUSD < 0 {
				return fmt.Errorf("model group %s target %s has negative output_price_per_million_usd", name, resolved.Model)
			}
			if resolved.ImageInputPricePerMillionTokensUSD < 0 {
				return fmt.Errorf("model group %s target %s has negative image_input_price_per_million_tokens_usd", name, resolved.Model)
			}
			if resolved.ImageInputPricePerImageUSD < 0 {
				return fmt.Errorf("model group %s target %s has negative image_input_price_per_image_usd", name, resolved.Model)
			}
			if resolved.TimeoutMS < 0 {
				return fmt.Errorf("model group %s target %s timeout_ms cannot be negative", name, resolved.Model)
			}
			if resolved.ContextTokens < 0 {
				return fmt.Errorf("model group %s target %s context_tokens cannot be negative", name, resolved.Model)
			}
			if err := validateToolSupport(resolved.ToolSupport); err != nil {
				return fmt.Errorf("model group %s target %s has invalid tool_support: %w", name, resolved.Model, err)
			}
			if err := validateModalities(resolved.InputModalities); err != nil {
				return fmt.Errorf("model group %s target %s has invalid input_modalities: %w", name, resolved.Model, err)
			}
			if err := validateModalities(resolved.OutputModalities); err != nil {
				return fmt.Errorf("model group %s target %s has invalid output_modalities: %w", name, resolved.Model, err)
			}
			resolved.InputModalities = defaultModalities(resolved.InputModalities)
			resolved.OutputModalities = defaultModalities(resolved.OutputModalities)
			if err := validateTargetValidation(name, resolved); err != nil {
				return err
			}
			m.Targets[i] = resolved
		}
		if err := c.validateModelGroupContract(name, m); err != nil {
			return err
		}
		c.Models[name] = m
	}
	callerIDs := map[string]string{}
	callerTokenHashes := map[string]string{}
	callerTokenIDs := map[string]string{}
	for _, caller := range c.Callers {
		if caller.ID == "" {
			return fmt.Errorf("caller missing id")
		}
		ownerUser, err := callerOwnerUser(caller)
		if err != nil {
			return err
		}
		if previousCallerID, ok := callerIDs[caller.ID]; ok {
			return fmt.Errorf("duplicate caller id %q for callers %s and %s", caller.ID, previousCallerID, caller.ID)
		}
		callerIDs[caller.ID] = caller.ID
		if ownerUser == "" && (caller.Project != "" || caller.User != "" || caller.OwnerUser != "") {
			return fmt.Errorf("caller %s has invalid owner_user", caller.ID)
		}
		if caller.Project != "" && slugify(caller.Project) == "" {
			return fmt.Errorf("caller %s has invalid project", caller.ID)
		}
		if caller.Environment != "" && slugify(caller.Environment) == "" {
			return fmt.Errorf("caller %s has invalid environment", caller.ID)
		}
		status := normalizeStatusDefault(caller.Status)
		if !validCallerKeyStatus(status) {
			return fmt.Errorf("caller %s has invalid status %q", caller.ID, caller.Status)
		}
		if caller.TokenSHA256 == "" || len(caller.TokenSHA256) != sha256.Size*2 {
			return fmt.Errorf("caller %s has invalid token_sha256", caller.ID)
		}
		if _, err := hex.DecodeString(caller.TokenSHA256); err != nil {
			return fmt.Errorf("caller %s has invalid token_sha256: %w", caller.ID, err)
		}
		tokenHashKey := strings.ToLower(caller.TokenSHA256)
		if previousCallerID, ok := callerTokenHashes[tokenHashKey]; ok {
			return fmt.Errorf("duplicate caller token_sha256 for callers %s and %s", previousCallerID, caller.ID)
		}
		callerTokenHashes[tokenHashKey] = caller.ID
		if caller.TokenID != "" {
			if previousCallerID, ok := callerTokenIDs[caller.TokenID]; ok {
				return fmt.Errorf("duplicate caller token_id for callers %s and %s", previousCallerID, caller.ID)
			}
			callerTokenIDs[caller.TokenID] = caller.ID
		}
		for _, group := range caller.Allow {
			if _, ok := c.Models[group]; !ok {
				return fmt.Errorf("caller %s allows unknown model group %s", caller.ID, group)
			}
		}
		if err := validateContentCapture("caller "+caller.ID+" content_capture", caller.ContentCapture); err != nil {
			return err
		}
	}
	return nil
}

func validateAdminAuth(cfg AdminAuthConfig, usage UsageDBConfig) error {
	if err := validateAdminAuthorization(cfg.Authorization, usage); err != nil {
		return err
	}
	if err := validateAdminBasicAuth(cfg.Basic); err != nil {
		return err
	}
	if err := validateAdminOIDC(cfg.OIDC); err != nil {
		return err
	}
	if err := validateAdminSessions(cfg.Sessions, cfg.OIDC.Enabled); err != nil {
		return err
	}
	return nil
}

func validateLicenseConfig(cfg LicenseConfig) error {
	if !cfg.Enabled {
		if cfg.FailOpenForDev {
			return fmt.Errorf("server license fail_open_for_dev requires license enabled")
		}
		return nil
	}
	if strings.TrimSpace(cfg.Path) == "" && !cfg.FailOpenForDev {
		return fmt.Errorf("server license path is required when license is enabled")
	}
	if cfg.RecheckInterval <= 0 || cfg.RecheckInterval > 24*time.Hour {
		return fmt.Errorf("server license recheck_interval must be greater than 0 and at most 24h")
	}
	if cfg.GracePeriodOnValidationError < 0 || cfg.GracePeriodOnValidationError > 7*24*time.Hour {
		return fmt.Errorf("server license grace_period_on_validation_error must be between 0 and 168h")
	}
	if cfg.FailOpenForDev && strings.TrimSpace(cfg.Path) != "" {
		return fmt.Errorf("server license fail_open_for_dev cannot be combined with a license path")
	}
	return nil
}

func validateAdminBasicAuth(basic AdminBasicAuthConfig) error {
	if !basic.Enabled {
		if len(basic.Users) > 0 {
			return fmt.Errorf("server admin_auth.basic users configured but basic auth is disabled")
		}
		return nil
	}
	realm := strings.TrimSpace(basic.Realm)
	if realm == "" {
		realm = "GenAI Smart Router Admin"
	}
	if strings.ContainsAny(realm, "\"\\\r\n") {
		return fmt.Errorf("server admin_auth.basic realm contains unsupported characters")
	}
	if len(basic.Users) == 0 {
		return fmt.Errorf("server admin_auth.basic requires at least one user when enabled")
	}
	for _, cidr := range basic.TrustedProxyCIDRs {
		trimmed := strings.TrimSpace(cidr)
		if trimmed == "" {
			return fmt.Errorf("server admin_auth.basic trusted_proxy_cidrs contains an empty value")
		}
		if _, _, err := net.ParseCIDR(trimmed); err != nil {
			return fmt.Errorf("server admin_auth.basic trusted_proxy_cidrs contains invalid CIDR %q: %w", trimmed, err)
		}
	}
	usernames := map[string]bool{}
	subjects := map[string]bool{}
	for i, user := range basic.Users {
		label := fmt.Sprintf("server admin_auth.basic users[%d]", i)
		username := strings.TrimSpace(user.Username)
		if username == "" {
			return fmt.Errorf("%s username is required", label)
		}
		if strings.ContainsAny(username, ":\r\n") {
			return fmt.Errorf("%s username contains unsupported characters", label)
		}
		if usernames[username] {
			return fmt.Errorf("server admin_auth.basic duplicate username %q", username)
		}
		usernames[username] = true
		envName := strings.TrimSpace(user.PasswordHashEnv)
		if envName == "" {
			return fmt.Errorf("%s requires password_hash_env", label)
		}
		if !validEnvName(envName) {
			return fmt.Errorf("%s password_hash_env is invalid", label)
		}
		hash, ok := os.LookupEnv(envName)
		hash = strings.TrimSpace(hash)
		if !ok || hash == "" {
			return fmt.Errorf("%s password_hash_env %s is not set", label, envName)
		}
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return fmt.Errorf("%s password hash must be bcrypt: %w", label, err)
		}
		subject := strings.TrimSpace(user.Subject)
		if subject == "" {
			subject = "basic:" + username
		}
		if !strings.HasPrefix(subject, "basic:") {
			return fmt.Errorf("%s subject must start with basic:", label)
		}
		if strings.ContainsAny(subject, "\r\n") {
			return fmt.Errorf("%s subject contains unsupported characters", label)
		}
		if subjects[subject] {
			return fmt.Errorf("server admin_auth.basic duplicate subject %q", subject)
		}
		subjects[subject] = true
		if strings.TrimSpace(user.Domain) == "" {
			return fmt.Errorf("%s domain is required", label)
		}
		for _, permission := range user.Permissions {
			if strings.TrimSpace(permission) == "" {
				return fmt.Errorf("%s permissions contains an empty value", label)
			}
			if strings.ContainsAny(permission, " \t\r\n") {
				return fmt.Errorf("%s permission %q contains whitespace", label, permission)
			}
		}
	}
	return nil
}

func validateAdminOIDC(oidc AdminOIDCConfig) error {
	if !oidc.Enabled {
		return nil
	}
	if strings.TrimSpace(oidc.GroupsClaim) == "" {
		oidc.GroupsClaim = "groups"
	}
	if strings.TrimSpace(oidc.EmailClaim) == "" {
		oidc.EmailClaim = "email"
	}
	if strings.TrimSpace(oidc.SubjectClaim) == "" {
		oidc.SubjectClaim = "email"
	}
	if strings.TrimSpace(oidc.Domain) == "" {
		oidc.Domain = "default"
	}
	issuer, err := url.Parse(strings.TrimSpace(oidc.IssuerURL))
	if err != nil || issuer.Host == "" || issuer.RawQuery != "" || issuer.Fragment != "" {
		return fmt.Errorf("server admin_auth.oidc issuer_url must be an absolute URL without query or fragment")
	}
	if issuer.Scheme != "https" && !(issuer.Scheme == "http" && isLocalhostHost(issuer.Hostname())) {
		return fmt.Errorf("server admin_auth.oidc issuer_url must use https except for localhost development")
	}
	clientIDEnv := strings.TrimSpace(oidc.ClientIDEnv)
	if clientIDEnv == "" {
		return fmt.Errorf("server admin_auth.oidc requires client_id_env when enabled")
	}
	if !validEnvName(clientIDEnv) {
		return fmt.Errorf("server admin_auth.oidc client_id_env is invalid")
	}
	if strings.TrimSpace(os.Getenv(clientIDEnv)) == "" {
		return fmt.Errorf("server admin_auth.oidc client_id_env %s is not set", clientIDEnv)
	}
	clientSecretEnv := strings.TrimSpace(oidc.ClientSecretEnv)
	if clientSecretEnv == "" {
		return fmt.Errorf("server admin_auth.oidc requires client_secret_env when enabled")
	}
	if !validEnvName(clientSecretEnv) {
		return fmt.Errorf("server admin_auth.oidc client_secret_env is invalid")
	}
	if strings.TrimSpace(os.Getenv(clientSecretEnv)) == "" {
		return fmt.Errorf("server admin_auth.oidc client_secret_env %s is not set", clientSecretEnv)
	}
	redirect, err := url.Parse(strings.TrimSpace(oidc.RedirectURL))
	if err != nil || redirect.Host == "" || redirect.RawQuery != "" || redirect.Fragment != "" {
		return fmt.Errorf("server admin_auth.oidc redirect_url must be an absolute URL without query or fragment")
	}
	if redirect.Scheme != "https" && !(redirect.Scheme == "http" && isLocalhostHost(redirect.Hostname())) {
		return fmt.Errorf("server admin_auth.oidc redirect_url must use https except for localhost development")
	}
	if strings.TrimSpace(oidc.Domain) == "" {
		return fmt.Errorf("server admin_auth.oidc domain is required")
	}
	for _, claim := range []struct {
		name  string
		value string
	}{
		{name: "groups_claim", value: oidc.GroupsClaim},
		{name: "email_claim", value: oidc.EmailClaim},
		{name: "subject_claim", value: oidc.SubjectClaim},
	} {
		if strings.TrimSpace(claim.value) == "" {
			return fmt.Errorf("server admin_auth.oidc %s is required", claim.name)
		}
		if !validOIDCClaimName(claim.value) {
			return fmt.Errorf("server admin_auth.oidc %s contains unsupported characters", claim.name)
		}
	}
	seenDomains := map[string]bool{}
	for _, domain := range oidc.AllowedDomains {
		trimmed := strings.ToLower(strings.TrimSpace(domain))
		if trimmed == "" {
			return fmt.Errorf("server admin_auth.oidc allowed_domains contains an empty value")
		}
		if strings.ContainsAny(trimmed, "@:/\\ \t\r\n") {
			return fmt.Errorf("server admin_auth.oidc allowed domain %q is invalid", domain)
		}
		if seenDomains[trimmed] {
			return fmt.Errorf("server admin_auth.oidc duplicate allowed domain %q", trimmed)
		}
		seenDomains[trimmed] = true
	}
	for _, scope := range oidc.Scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			return fmt.Errorf("server admin_auth.oidc scopes contains an empty value")
		}
		if strings.ContainsAny(trimmed, "\r\n") {
			return fmt.Errorf("server admin_auth.oidc scope %q contains unsupported characters", scope)
		}
	}
	return nil
}

func validateAdminSessions(cfg AdminSessionConfig, oidcEnabled bool) error {
	configured := strings.TrimSpace(cfg.CookieName) != "" || cfg.TTL != 0 || strings.TrimSpace(cfg.SameSite) != "" || cfg.SecureCookies != nil
	if !oidcEnabled && !configured {
		return nil
	}
	name := strings.TrimSpace(cfg.CookieName)
	if name == "" {
		name = "smart_router_admin_session"
	}
	if !validCookieName(name) {
		return fmt.Errorf("server admin_auth.sessions cookie_name is invalid")
	}
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 8 * time.Hour
	}
	if ttl < time.Minute || ttl > 24*time.Hour {
		return fmt.Errorf("server admin_auth.sessions ttl must be between 1m and 24h")
	}
	sameSite := strings.ToLower(strings.TrimSpace(cfg.SameSite))
	if sameSite == "" {
		sameSite = "strict"
	}
	switch sameSite {
	case "strict", "lax":
	case "none":
		if cfg.SecureCookies != nil && !*cfg.SecureCookies {
			return fmt.Errorf("server admin_auth.sessions same_site none requires secure_cookies")
		}
	default:
		return fmt.Errorf("server admin_auth.sessions same_site must be strict, lax, or none")
	}
	return nil
}

func isLocalhostHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func validOIDCClaimName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if r == '_' || r == '-' || r == '.' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func validCookieName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r <= 0x20 || r >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", r) {
			return false
		}
	}
	return true
}

func validateAdminAuthorization(cfg AdminAuthorizationConfig, usage UsageDBConfig) error {
	source := strings.ToLower(strings.TrimSpace(cfg.Source))
	if source == "" {
		source = "static"
	}
	switch source {
	case "static", "db":
	default:
		return fmt.Errorf("server admin_auth.authorization source must be static or db")
	}
	if !cfg.Enabled {
		if len(cfg.Policy) > 0 || strings.TrimSpace(cfg.PolicyFile) != "" || source == "db" {
			return fmt.Errorf("server admin_auth.authorization policy configured but authorization is disabled")
		}
		return nil
	}
	if strings.ContainsAny(cfg.PolicyFile, "\r\n") {
		return fmt.Errorf("server admin_auth.authorization policy_file contains unsupported characters")
	}
	if source == "db" {
		if len(cfg.Policy) > 0 || strings.TrimSpace(cfg.PolicyFile) != "" {
			return fmt.Errorf("server admin_auth.authorization source db cannot combine inline policy or policy_file")
		}
		if usage.Enable != nil && !*usage.Enable {
			return fmt.Errorf("server admin_auth.authorization source db requires usage_db enabled")
		}
		return nil
	}
	if len(cfg.Policy) == 0 && strings.TrimSpace(cfg.PolicyFile) == "" {
		return fmt.Errorf("server admin_auth.authorization requires policy or policy_file when enabled")
	}
	for i, line := range cfg.Policy {
		fields, err := parseCasbinPolicyLine(line)
		if err != nil {
			return fmt.Errorf("server admin_auth.authorization policy[%d] is invalid: %w", i, err)
		}
		if err := validateCasbinPolicyFields(fields, fmt.Sprintf("server admin_auth.authorization policy[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateAdminReports(cfg AdminReportsConfig, auth AdminAuthConfig, usage UsageDBConfig) error {
	if strings.TrimSpace(cfg.PathPrefix) == "" {
		cfg.PathPrefix = "/admin/reports"
	}
	if strings.TrimSpace(cfg.DefaultSince) == "" {
		cfg.DefaultSince = "24h"
	}
	if strings.TrimSpace(cfg.MaxRange) == "" {
		cfg.MaxRange = "31d"
	}
	if cfg.MaxRows == 0 {
		cfg.MaxRows = 500
	}
	if strings.TrimSpace(cfg.PathPrefix) == "" {
		return fmt.Errorf("server admin_reports path_prefix is required")
	}
	prefix := cleanAdminReportsPrefix(cfg.PathPrefix)
	if prefix != "/admin/reports" && !strings.HasPrefix(prefix, "/admin/reports/") {
		return fmt.Errorf("server admin_reports path_prefix must be /admin/reports or a child path")
	}
	defaultSince, err := parseReportDuration(cfg.DefaultSince)
	if err != nil || defaultSince <= 0 {
		return fmt.Errorf("server admin_reports default_since is invalid")
	}
	maxRange, err := parseReportDuration(cfg.MaxRange)
	if err != nil || maxRange <= 0 {
		return fmt.Errorf("server admin_reports max_range is invalid")
	}
	if defaultSince > maxRange {
		return fmt.Errorf("server admin_reports default_since cannot exceed max_range")
	}
	if maxRange > 366*24*time.Hour {
		return fmt.Errorf("server admin_reports max_range must be <= 366d")
	}
	if cfg.MaxRows <= 0 || cfg.MaxRows > 10000 {
		return fmt.Errorf("server admin_reports max_rows must be between 1 and 10000")
	}
	if cfg.Security.RetentionDays == 0 {
		cfg.Security.RetentionDays = 90
	}
	if cfg.Security.RetentionDays < 1 || cfg.Security.RetentionDays > 3660 {
		return fmt.Errorf("server admin_reports security retention_days must be between 1 and 3660")
	}
	if len(cfg.Baselines) == 0 {
		cfg.Baselines = defaultAdminReportBaselines()
	}
	seenBaselines := map[string]bool{}
	for i, baseline := range cfg.Baselines {
		id := strings.TrimSpace(baseline.ID)
		if id == "" {
			return fmt.Errorf("server admin_reports baselines[%d] id is required", i)
		}
		if seenBaselines[id] {
			return fmt.Errorf("server admin_reports baselines[%d] id %q is duplicated", i, id)
		}
		seenBaselines[id] = true
		if strings.TrimSpace(baseline.Name) == "" {
			return fmt.Errorf("server admin_reports baselines[%d] name is required", i)
		}
		if strings.TrimSpace(baseline.PricingSource) == "" || strings.TrimSpace(baseline.PricingUpdatedAt) == "" {
			return fmt.Errorf("server admin_reports baselines[%d] pricing_source and pricing_updated_at are required", i)
		}
		if baseline.InputPricePerMillionUSD < 0 || baseline.OutputPricePerMillionUSD < 0 {
			return fmt.Errorf("server admin_reports baselines[%d] prices must be nonnegative", i)
		}
		if baseline.InputPricePerMillionUSD > 100000 || baseline.OutputPricePerMillionUSD > 100000 {
			return fmt.Errorf("server admin_reports baselines[%d] prices are unreasonably high", i)
		}
	}
	if !cfg.Enabled {
		return nil
	}
	if usage.Enable != nil && !*usage.Enable {
		return fmt.Errorf("server admin_reports requires usage_db enabled")
	}
	if !auth.Basic.Enabled && !auth.OIDC.Enabled {
		return fmt.Errorf("server admin_reports requires server.admin_auth.basic or server.admin_auth.oidc enabled")
	}
	if !auth.Authorization.Enabled {
		return fmt.Errorf("server admin_reports requires server.admin_auth.authorization enabled")
	}
	return nil
}

func validateClientIP(cfg ClientIPConfig) error {
	for i, cidr := range cfg.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			return fmt.Errorf("server client_ip trusted_proxy_cidrs[%d] is invalid", i)
		}
	}
	for i, header := range cfg.HeaderOrder {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "x-forwarded-for", "x-real-ip":
		case "":
			return fmt.Errorf("server client_ip header_order[%d] is empty", i)
		default:
			return fmt.Errorf("server client_ip header_order[%d] must be X-Forwarded-For or X-Real-IP", i)
		}
	}
	return nil
}

func defaultAdminReportBaselines() []AdminReportBaselineConfig {
	return []AdminReportBaselineConfig{
		{
			ID:                       "gpt-5.5",
			Name:                     "GPT-5.5",
			PricingSource:            "https://developers.openai.com/api/docs/pricing",
			PricingUpdatedAt:         "2026-06-25",
			InputPricePerMillionUSD:  5.00,
			OutputPricePerMillionUSD: 30.00,
			Notes:                    "OpenAI API standard text token pricing checked on 2026-06-25.",
		},
		{
			ID:                       "claude-opus-4.8",
			Name:                     "Claude Opus 4.8",
			PricingSource:            "https://docs.anthropic.com/en/docs/about-claude/pricing",
			PricingUpdatedAt:         "2026-06-25",
			InputPricePerMillionUSD:  5.00,
			OutputPricePerMillionUSD: 25.00,
			Notes:                    "Anthropic Claude API standard token pricing checked on 2026-06-25; fast mode and batch pricing are separate.",
		},
	}
}

func cleanAdminReportsPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/admin/reports"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = path.Clean(prefix)
	if prefix == "." {
		return "/admin/reports"
	}
	return prefix
}

func parseReportDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

func defaultContentCaptureConfig(cfg *ContentCaptureConfig) {
	if cfg == nil {
		return
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = 30
	}
	if cfg.MaxCaptureBytes == 0 {
		cfg.MaxCaptureBytes = 64 << 10
	}
	if cfg.RedactBeforeStorage == nil {
		v := true
		cfg.RedactBeforeStorage = &v
	}
}

const (
	retentionDataClassUsageDiagnostics  = "usage_diagnostics"
	retentionDataClassSecurityAccess    = "security_access_events"
	retentionDataClassContentCapture    = "content_capture"
	retentionDataClassDecisionTelemetry = "decision_telemetry"
	retentionDataClassUsageDetail       = "usage_detail"
)

func defaultRetentionConfig(cfg *RetentionConfig) {
	if cfg == nil {
		return
	}
	if cfg.DryRun == nil {
		v := true
		cfg.DryRun = &v
	}
	if cfg.DefaultBatchSize == 0 {
		cfg.DefaultBatchSize = 500
	}
	if len(cfg.Classes) == 0 {
		cfg.Classes = []RetentionClassConfig{
			{DataClass: retentionDataClassUsageDiagnostics, RetentionDays: 30},
			{DataClass: retentionDataClassDecisionTelemetry, RetentionDays: 30},
			{DataClass: retentionDataClassSecurityAccess, RetentionDays: 90},
			{DataClass: retentionDataClassContentCapture, RetentionDays: 30},
			{DataClass: retentionDataClassUsageDetail, Enabled: boolPtr(false), RetentionDays: 365, RequireFinalizedRollup: true},
		}
	}
	for i := range cfg.Classes {
		cfg.Classes[i].DataClass = normalizeRetentionDataClass(cfg.Classes[i].DataClass)
		if cfg.Classes[i].BatchSize == 0 {
			cfg.Classes[i].BatchSize = cfg.DefaultBatchSize
		}
		if cfg.Classes[i].DataClass == retentionDataClassUsageDetail {
			cfg.Classes[i].RequireFinalizedRollup = true
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func (c *Config) contentCaptureEnabled() bool {
	if c == nil {
		return false
	}
	if c.Server.ContentCapture.Enabled {
		return true
	}
	for _, caller := range c.Callers {
		if caller.ContentCapture.Enabled {
			return true
		}
	}
	for _, group := range c.Models {
		if group.ContentCapture.Enabled {
			return true
		}
	}
	return false
}

func validateContentCapture(label string, cfg ContentCaptureConfig) error {
	if cfg.RetentionDays < 0 {
		return fmt.Errorf("%s retention_days cannot be negative", label)
	}
	if cfg.MaxCaptureBytes < 0 {
		return fmt.Errorf("%s max_capture_bytes cannot be negative", label)
	}
	if cfg.RedactBeforeStorage != nil && !*cfg.RedactBeforeStorage {
		return fmt.Errorf("%s redact_before_storage must remain true", label)
	}
	if cfg.Encryption.Enabled {
		return fmt.Errorf("%s encryption.enabled is not supported yet", label)
	}
	if cfg.Enabled && !cfg.CaptureRequest && !cfg.CaptureResponse && !cfg.CaptureUpstreamErrors {
		return fmt.Errorf("%s enables content capture but no capture scope is enabled", label)
	}
	for _, header := range cfg.CaptureHeadersAllowlist {
		if !contentCaptureHeaderAllowed(header) {
			return fmt.Errorf("%s capture_headers_allowlist contains forbidden header %q", label, header)
		}
	}
	for _, rule := range cfg.RedactionPatterns {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("%s redaction_patterns contains a rule without name", label)
		}
		if strings.TrimSpace(rule.Expression) == "" {
			return fmt.Errorf("%s redaction pattern %s missing expression", label, rule.Name)
		}
		if _, err := regexp.Compile(rule.Expression); err != nil {
			return fmt.Errorf("%s redaction pattern %s is invalid: %w", label, rule.Name, err)
		}
	}
	return nil
}

func validateRetentionConfig(cfg RetentionConfig) error {
	if !cfg.Enabled && cfg.DryRun == nil && cfg.DefaultBatchSize == 0 && len(cfg.Classes) == 0 {
		return nil
	}
	if cfg.DefaultBatchSize <= 0 {
		return fmt.Errorf("server retention default_batch_size must be positive")
	}
	if cfg.Enabled && cfg.DryRun != nil && !*cfg.DryRun {
		return fmt.Errorf("server retention dry_run=false is not supported in this foundation")
	}
	seen := map[string]bool{}
	for i, class := range cfg.Classes {
		label := fmt.Sprintf("server retention classes[%d]", i)
		dataClass := normalizeRetentionDataClass(class.DataClass)
		if !knownRetentionDataClass(dataClass) {
			return fmt.Errorf("%s has unknown data_class %q", label, class.DataClass)
		}
		if seen[dataClass] {
			return fmt.Errorf("%s duplicates data_class %q", label, dataClass)
		}
		seen[dataClass] = true
		if class.RetentionDays <= 0 {
			return fmt.Errorf("%s retention_days must be positive", label)
		}
		if class.BatchSize <= 0 {
			return fmt.Errorf("%s batch_size must be positive", label)
		}
		if dataClass == retentionDataClassUsageDetail && !class.RequireFinalizedRollup {
			return fmt.Errorf("%s usage_detail requires finalized rollup before future delete", label)
		}
	}
	if cfg.Enabled && len(cfg.Classes) == 0 {
		return fmt.Errorf("server retention requires at least one class")
	}
	return nil
}

func normalizeRetentionDataClass(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func knownRetentionDataClass(v string) bool {
	switch v {
	case retentionDataClassUsageDiagnostics, retentionDataClassDecisionTelemetry, retentionDataClassSecurityAccess, retentionDataClassContentCapture, retentionDataClassUsageDetail:
		return true
	default:
		return false
	}
}

func contentCaptureHeaderAllowed(header string) bool {
	name := strings.ToLower(strings.TrimSpace(header))
	if name == "" {
		return false
	}
	switch name {
	case "authorization", "proxy-authorization", "x-api-key", "api-key", "openai-api-key", "anthropic-api-key", "cookie", "set-cookie":
		return false
	}
	return !strings.Contains(name, "token") && !strings.Contains(name, "secret") && !strings.Contains(name, "key")
}

func validatePIIFilter(group string, cfg PIIFilterConfig) error {
	if !cfg.Enabled {
		if cfg.Mode != "" || cfg.FailOnMatch || cfg.RestoreResponse != nil || cfg.MaxReplacementsPerRequest != 0 || len(cfg.Rules) > 0 {
			return fmt.Errorf("model group %s configures pii_filter but pii_filter.enabled is false", group)
		}
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "redact_only", "redact_and_restore", "fail_on_match":
	default:
		return fmt.Errorf("model group %s pii_filter mode must be redact_only, redact_and_restore, or fail_on_match", group)
	}
	if cfg.MaxReplacementsPerRequest < 0 {
		return fmt.Errorf("model group %s pii_filter max_replacements_per_request cannot be negative", group)
	}
	if len(cfg.Rules) == 0 {
		return fmt.Errorf("model group %s pii_filter requires at least one rule", group)
	}
	seen := map[string]bool{}
	for i, rule := range cfg.Rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return fmt.Errorf("model group %s pii_filter rule %d missing name", group, i)
		}
		if seen[name] {
			return fmt.Errorf("model group %s pii_filter contains duplicate rule %q", group, name)
		}
		seen[name] = true
		if strings.TrimSpace(rule.Expression) == "" {
			return fmt.Errorf("model group %s pii_filter rule %s missing expression", group, name)
		}
		if _, err := regexp.Compile(rule.Expression); err != nil {
			return fmt.Errorf("model group %s pii_filter rule %s has invalid expression: %w", group, name, err)
		}
		if normalizePlaceholderPrefix(rule.PlaceholderPrefix) == "" {
			return fmt.Errorf("model group %s pii_filter rule %s missing placeholder_prefix", group, name)
		}
	}
	return nil
}

func validateDynamicScorePolicy(group string, cfg DynamicScoreConfig) error {
	switch strings.ToLower(strings.TrimSpace(cfg.ColdStartPolicy)) {
	case "", "configured_weight":
	default:
		return fmt.Errorf("model group %s dynamic_score cold_start_policy must be configured_weight", group)
	}
	if cfg.MinObservations < 0 {
		return fmt.Errorf("model group %s dynamic_score min_observations cannot be negative", group)
	}
	if cfg.ObservationWindowSeconds < 0 {
		return fmt.Errorf("model group %s dynamic_score observation_window_seconds cannot be negative", group)
	}
	if cfg.MaxScoreAdjustmentPercent < 0 || cfg.MaxScoreAdjustmentPercent > 100 {
		return fmt.Errorf("model group %s dynamic_score max_score_adjustment_percent must be between 0 and 100", group)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.LowConfidenceFallback)) {
	case "", "configured_weight", "weighted":
	default:
		return fmt.Errorf("model group %s dynamic_score low_confidence_fallback must be configured_weight or weighted", group)
	}
	if cfg.Signals.PromptFeatures.MaxScanBytes < 0 {
		return fmt.Errorf("model group %s dynamic_score prompt_features.max_scan_bytes cannot be negative", group)
	}
	if cfg.Signals.PromptFeatures.MaxScanBytes > 65536 {
		return fmt.Errorf("model group %s dynamic_score prompt_features.max_scan_bytes must be <= 65536", group)
	}
	if cfg.Signals.RecentPenalties.TTLSeconds < 0 {
		return fmt.Errorf("model group %s dynamic_score recent_penalties.ttl_seconds cannot be negative", group)
	}
	if cfg.Signals.EvaluationMetadata.MaxAgeDays < 0 {
		return fmt.Errorf("model group %s dynamic_score evaluation_metadata.max_age_days cannot be negative", group)
	}
	for _, threshold := range []struct {
		name  string
		value *float64
	}{
		{"max_error_rate", cfg.Thresholds.MaxErrorRate},
		{"max_timeout_rate", cfg.Thresholds.MaxTimeoutRate},
	} {
		if threshold.value != nil && (*threshold.value < 0 || *threshold.value > 1) {
			return fmt.Errorf("model group %s dynamic_score threshold %s must be between 0 and 1", group, threshold.name)
		}
	}
	for _, threshold := range []struct {
		name  string
		value *float64
	}{
		{"max_p95_latency_ms", cfg.Thresholds.MaxP95LatencyMS},
		{"max_ttfb_ms", cfg.Thresholds.MaxTTFBMS},
		{"min_output_tokens_per_second", cfg.Thresholds.MinOutputTokensPerSecond},
	} {
		if threshold.value != nil && *threshold.value < 0 {
			return fmt.Errorf("model group %s dynamic_score threshold %s cannot be negative", group, threshold.name)
		}
	}
	for _, term := range cfg.ScoreTerms {
		if strings.TrimSpace(term.Expression) == "" && len(term.Weights) == 0 && len(term.RequireTags) == 0 && len(term.PreferTags) == 0 {
			return fmt.Errorf("model group %s dynamic_score score term %s has no expression, weights, or tag preference", group, term.Name)
		}
	}
	for _, eval := range cfg.EvaluationMetadata {
		if strings.TrimSpace(eval.Provider) == "" || strings.TrimSpace(eval.Model) == "" {
			return fmt.Errorf("model group %s dynamic_score evaluation_metadata entries require provider and model", group)
		}
		if eval.QualityScore < 0 || eval.QualityScore > 1 || eval.PassRate < 0 || eval.PassRate > 1 {
			return fmt.Errorf("model group %s dynamic_score evaluation scores must be between 0 and 1", group)
		}
	}
	return nil
}

func normalizeAuthScheme(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return "default"
	case "bearer", "authorization-bearer":
		return "bearer"
	case "x-api-key", "anthropic":
		return "x-api-key"
	default:
		return ""
	}
}

func (c *Config) resolveTarget(group string, target Target) (Target, error) {
	provider, ok := c.Provider[target.Provider]
	if !ok {
		return target, fmt.Errorf("model group %s references unknown provider %s", group, target.Provider)
	}
	if target.ModelRef == "" {
		if target.Model == "" {
			return target, fmt.Errorf("model group %s target for provider %s missing model or model_ref", group, target.Provider)
		}
		return target, nil
	}
	catalog, ok := provider.Models[target.ModelRef]
	if !ok {
		return target, fmt.Errorf("model group %s target references unknown model_ref %s for provider %s", group, target.ModelRef, target.Provider)
	}
	if target.Model == "" {
		target.Model = catalog.Model
	}
	if target.Dialect == "" {
		target.Dialect = catalog.Dialect
	}
	if target.DisplayName == "" {
		target.DisplayName = catalog.DisplayName
	}
	if target.ContextTokens == 0 {
		target.ContextTokens = catalog.ContextTokens
	}
	if target.RPM == 0 {
		target.RPM = catalog.RPM
	}
	if target.Tier == "" {
		target.Tier = catalog.Tier
	}
	if target.Cost == 0 {
		target.Cost = catalog.Cost
	}
	if target.InputPricePerMillionUSD == 0 {
		target.InputPricePerMillionUSD = catalog.InputPricePerMillionUSD
	}
	if target.OutputPricePerMillionUSD == 0 {
		target.OutputPricePerMillionUSD = catalog.OutputPricePerMillionUSD
	}
	if target.ImageInputPricePerMillionTokensUSD == 0 {
		target.ImageInputPricePerMillionTokensUSD = catalog.ImageInputPricePerMillionTokensUSD
	}
	if target.ImageInputPricePerImageUSD == 0 {
		target.ImageInputPricePerImageUSD = catalog.ImageInputPricePerImageUSD
	}
	if target.PricingSource == "" {
		target.PricingSource = catalog.PricingSource
	}
	if target.PricingUpdatedAt == "" {
		target.PricingUpdatedAt = catalog.PricingUpdatedAt
	}
	if target.PricingNotes == "" {
		target.PricingNotes = catalog.PricingNotes
	}
	if toolSupportEmpty(target.ToolSupport) {
		target.ToolSupport = catalog.ToolSupport
	}
	if len(target.InputModalities) == 0 {
		target.InputModalities = catalog.InputModalities
	}
	if len(target.OutputModalities) == 0 {
		target.OutputModalities = catalog.OutputModalities
	}
	if target.HonorsMaxTokens == nil {
		target.HonorsMaxTokens = catalog.HonorsMaxTokens
	}
	if target.Model == "" {
		return target, fmt.Errorf("model group %s target model_ref %s for provider %s resolved without model", group, target.ModelRef, target.Provider)
	}
	return target, nil
}

func validateToolSupport(ts ToolSupport) error {
	for surface, values := range map[string][]string{
		"openai_chat":        ts.OpenAIChat,
		"openai_responses":   ts.OpenAIResponses,
		"anthropic_messages": ts.AnthropicMessages,
		"provider_hosted":    ts.ProviderHosted,
	} {
		seen := map[string]bool{}
		for _, v := range values {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				return fmt.Errorf("%s contains empty capability", surface)
			}
			if seen[trimmed] {
				return fmt.Errorf("%s contains duplicate capability %q", surface, trimmed)
			}
			seen[trimmed] = true
		}
	}
	return nil
}

func validateTargetValidation(group string, target Target) error {
	if target.Validation == nil {
		return nil
	}
	v := target.Validation
	status := strings.ToLower(strings.TrimSpace(v.Status))
	if status != "" && !validTargetValidationStatus(status) {
		return fmt.Errorf("model group %s target %s validation.status %q is unsupported", group, target.Model, v.Status)
	}
	if v.QualityScore < 0 || v.QualityScore > 1 {
		return fmt.Errorf("model group %s target %s validation.quality_score must be between 0 and 1", group, target.Model)
	}
	if v.PassRate < 0 || v.PassRate > 1 {
		return fmt.Errorf("model group %s target %s validation.pass_rate must be between 0 and 1", group, target.Model)
	}
	if strings.TrimSpace(v.ValidatedAt) != "" {
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(v.ValidatedAt)); err != nil {
			return fmt.Errorf("model group %s target %s validation.validated_at must be YYYY-MM-DD", group, target.Model)
		}
	}
	if strings.TrimSpace(v.Workload) == "" && (strings.TrimSpace(v.Status) != "" || strings.TrimSpace(v.Harness) != "" || strings.TrimSpace(v.ValidatedAt) != "" || v.QualityScore != 0 || v.PassRate != 0) {
		return fmt.Errorf("model group %s target %s validation.workload is required when validation metadata is configured", group, target.Model)
	}
	return nil
}

func validTargetValidationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "passed", "failed", "pending", "stale", "expired", "skipped":
		return true
	default:
		return false
	}
}

func (c *Config) validateModelGroupContract(group string, model ModelGroup) error {
	contract := model.Contract
	if contract == nil {
		return nil
	}
	for _, shape := range contract.SupportedAPIShapes {
		if normalizeContractAPIShape(shape) == "" {
			return fmt.Errorf("model group %s contract.supported_api_shapes contains unsupported API shape %q", group, shape)
		}
		if !modelGroupHasAPIShapeTarget(c, model, normalizeContractAPIShape(shape)) {
			return fmt.Errorf("model group %s contract.supported_api_shapes %q is not served by any target", group, shape)
		}
	}
	if err := validateModalities(contract.RequiredCaps.InputModalities); err != nil {
		return fmt.Errorf("model group %s contract.required_capabilities.input_modalities is invalid: %w", group, err)
	}
	if err := validateModalities(contract.RequiredCaps.OutputModalities); err != nil {
		return fmt.Errorf("model group %s contract.required_capabilities.output_modalities is invalid: %w", group, err)
	}
	if contract.RequiredCaps.MinContextTokens < 0 {
		return fmt.Errorf("model group %s contract.required_capabilities.min_context_tokens cannot be negative", group)
	}
	if err := validateOptionalUnitInterval(group, "contract.quality_floor.min_eval_quality_score", contract.QualityFloor.MinEvalQualityScore); err != nil {
		return err
	}
	if err := validateOptionalUnitInterval(group, "contract.quality_floor.min_eval_pass_rate", contract.QualityFloor.MinEvalPassRate); err != nil {
		return err
	}
	if contract.QualityFloor.MaxEvalAgeDays < 0 {
		return fmt.Errorf("model group %s contract.quality_floor.max_eval_age_days cannot be negative", group)
	}
	for _, status := range contract.QualityFloor.AllowedValidationStatus {
		if !validTargetValidationStatus(status) {
			return fmt.Errorf("model group %s contract.quality_floor.allowed_validation_status contains unsupported status %q", group, status)
		}
	}
	if err := validateOptionalNonNegative(group, "contract.operational_targets.max_p95_latency_ms", contract.OperationalTargets.MaxP95LatencyMS); err != nil {
		return err
	}
	if err := validateOptionalUnitInterval(group, "contract.operational_targets.max_error_rate", contract.OperationalTargets.MaxErrorRate); err != nil {
		return err
	}
	if err := validateOptionalUnitInterval(group, "contract.operational_targets.max_timeout_rate", contract.OperationalTargets.MaxTimeoutRate); err != nil {
		return err
	}
	if err := validateOptionalNonNegative(group, "contract.operational_targets.min_output_tokens_per_second", contract.OperationalTargets.MinOutputTokensPerSecond); err != nil {
		return err
	}
	for _, tag := range contract.QualityFloor.RequireTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("model group %s contract.quality_floor.require_tags contains empty tag", group)
		}
	}
	if !contractCanBeSatisfied(c, model, "openai-chat", nil) {
		return fmt.Errorf("model group %s contract cannot be satisfied by any configured target", group)
	}
	for _, tag := range contract.QualityFloor.RequireTags {
		found := false
		for _, target := range model.Targets {
			if targetHasTag(target, tag) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("model group %s contract.quality_floor.require_tags tag %q is not present on any target", group, tag)
		}
	}
	return nil
}

func validateOptionalUnitInterval(group, field string, value *float64) error {
	if value == nil {
		return nil
	}
	if *value < 0 || *value > 1 {
		return fmt.Errorf("model group %s %s must be between 0 and 1", group, field)
	}
	return nil
}

func validateOptionalNonNegative(group, field string, value *float64) error {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return fmt.Errorf("model group %s %s cannot be negative", group, field)
	}
	return nil
}

func normalizeContractAPIShape(shape string) string {
	switch strings.ToLower(strings.TrimSpace(shape)) {
	case "openai_chat", "openai-chat", "chat":
		return "openai-chat"
	case "openai_responses", "openai-responses", "responses":
		return "openai-responses"
	case "anthropic_messages", "anthropic", "anthropic-messages":
		return "anthropic"
	default:
		return ""
	}
}

func contractCanBeSatisfied(c *Config, model ModelGroup, callerDialect string, req *IRRequest) bool {
	for _, target := range model.Targets {
		provider := c.Provider[target.Provider]
		outDialect := targetDialect(provider, target)
		if model.Contract == nil || targetPassesContract(model.Contract, target, outDialect, callerDialect, req, dynamicStats{}, time.Now().UTC()) == "" {
			return true
		}
	}
	return false
}

func modelGroupHasAPIShapeTarget(c *Config, model ModelGroup, dialect string) bool {
	req := &IRRequest{}
	for _, target := range model.Targets {
		outDialect := targetDialect(c.Provider[target.Provider], target)
		if !target.ToolOnly &&
			targetSupportsInputModalities(target, requestInputModalities(req)) &&
			targetSupportsStructuredOutput(target, dialect, outDialect, requestHasStructuredOutput(req)) &&
			targetHonorsExplicitMaxTokens(target, req) {
			return true
		}
	}
	return false
}

func toolSupportEmpty(ts ToolSupport) bool {
	return len(ts.OpenAIChat) == 0 &&
		len(ts.OpenAIResponses) == 0 &&
		len(ts.AnthropicMessages) == 0 &&
		len(ts.ProviderHosted) == 0
}

func externalPolicyEmpty(cfg ExternalPolicyConfig) bool {
	return strings.TrimSpace(cfg.URL) == "" &&
		strings.TrimSpace(cfg.Method) == "" &&
		len(cfg.AllowHosts) == 0 &&
		!cfg.AllowHTTP &&
		cfg.TimeoutMS == 0 &&
		cfg.MaxResponseBytes == 0 &&
		len(cfg.Headers) == 0 &&
		strings.TrimSpace(cfg.OnError) == "" &&
		!cfg.IncludeRequest
}

func validateModalities(values []string) error {
	allowed := map[string]bool{
		"text":       true,
		"image":      true,
		"video":      true,
		"audio":      true,
		"pdf":        true,
		"file":       true,
		"embeddings": true,
	}
	seen := map[string]bool{}
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return fmt.Errorf("contains empty modality")
		}
		if !allowed[trimmed] {
			return fmt.Errorf("contains unsupported modality %q", trimmed)
		}
		if seen[trimmed] {
			return fmt.Errorf("contains duplicate modality %q", trimmed)
		}
		seen[trimmed] = true
	}
	return nil
}

func defaultModalities(values []string) []string {
	if len(values) == 0 {
		return []string{"text"}
	}
	return values
}

func normalizeDialect(d string) string {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case "anthropic":
		return "anthropic"
	case "openai", "openai-chat", "chat":
		return "openai-chat"
	case "openai-responses", "responses":
		return "openai-responses"
	case "replicate":
		return "replicate"
	default:
		return ""
	}
}
