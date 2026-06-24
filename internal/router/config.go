package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig              `yaml:"server"`
	Provider  map[string]ProviderConfig `yaml:"providers"`
	Models    map[string]ModelGroup     `yaml:"models"`
	Callers   []CallerConfig            `yaml:"callers"`
	StatePath string                    `yaml:"state_path"`
	baseDir   string
}

type ServerConfig struct {
	Listen            string            `yaml:"listen"`
	DefaultModelGroup string            `yaml:"default_model_group"`
	Cache             CacheConfig       `yaml:"cache"`
	Logging           LoggingConfig     `yaml:"logging"`
	UsageDB           UsageDBConfig     `yaml:"usage_db"`
	Upstream          UpstreamConfig    `yaml:"upstream"`
	Diagnostics       DiagnosticsConfig `yaml:"diagnostics"`
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
	PIIFilter        PIIFilterConfig      `yaml:"pii_filter"`
	AttemptTimeoutMS int                  `yaml:"attempt_timeout_ms"`
	Targets          []Target             `yaml:"targets"`
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
	TimeoutMS        int               `yaml:"timeout_ms" json:"timeoutMs"`
	MaxResponseBytes int64             `yaml:"max_response_bytes" json:"maxResponseBytes"`
	Headers          map[string]string `yaml:"headers" json:"headers"`
}

type ExternalPolicyConfig struct {
	URL              string            `yaml:"url" json:"url"`
	Method           string            `yaml:"method" json:"method"`
	AllowHosts       []string          `yaml:"allow_hosts" json:"allowHosts"`
	TimeoutMS        int               `yaml:"timeout_ms" json:"timeoutMs"`
	MaxResponseBytes int64             `yaml:"max_response_bytes" json:"maxResponseBytes"`
	Headers          map[string]string `yaml:"headers" json:"headers"`
	OnError          string            `yaml:"on_error" json:"onError"`
}

type Target struct {
	Provider                           string         `yaml:"provider" json:"provider"`
	Model                              string         `yaml:"model" json:"model"`
	ModelRef                           string         `yaml:"model_ref" json:"modelRef,omitempty"`
	Dialect                            string         `yaml:"dialect" json:"dialect"`
	DisplayName                        string         `yaml:"display_name" json:"displayName,omitempty"`
	ToolOnly                           bool           `yaml:"tool_only" json:"toolOnly,omitempty"`
	TimeoutMS                          int            `yaml:"timeout_ms" json:"timeoutMs,omitempty"`
	DefaultThinking                    map[string]any `yaml:"default_thinking" json:"defaultThinking,omitempty"`
	Tags                               []string       `yaml:"tags" json:"tags,omitempty"`
	Weight                             int            `yaml:"weight" json:"weight"`
	RPM                                int            `yaml:"rpm" json:"rpm"`
	Tier                               string         `yaml:"tier" json:"tier"`
	Cost                               int            `yaml:"cost" json:"cost"`
	InputPricePerMillionUSD            float64        `yaml:"input_price_per_million_usd" json:"inputPricePerMillionUsd,omitempty"`
	OutputPricePerMillionUSD           float64        `yaml:"output_price_per_million_usd" json:"outputPricePerMillionUsd,omitempty"`
	ImageInputPricePerMillionTokensUSD float64        `yaml:"image_input_price_per_million_tokens_usd" json:"imageInputPricePerMillionTokensUsd,omitempty"`
	ImageInputPricePerImageUSD         float64        `yaml:"image_input_price_per_image_usd" json:"imageInputPricePerImageUsd,omitempty"`
	PricingSource                      string         `yaml:"pricing_source" json:"pricingSource,omitempty"`
	PricingUpdatedAt                   string         `yaml:"pricing_updated_at" json:"pricingUpdatedAt,omitempty"`
	PricingNotes                       string         `yaml:"pricing_notes" json:"pricingNotes,omitempty"`
	ToolSupport                        ToolSupport    `yaml:"tool_support" json:"toolSupport,omitempty"`
	InputModalities                    []string       `yaml:"input_modalities" json:"inputModalities,omitempty"`
	OutputModalities                   []string       `yaml:"output_modalities" json:"outputModalities,omitempty"`
	HonorsMaxTokens                    *bool          `yaml:"honors_max_tokens" json:"honorsMaxTokens,omitempty"`
}

type CallerConfig struct {
	ID           string      `yaml:"id" json:"id"`
	User         string      `yaml:"user" json:"user"`
	Project      string      `yaml:"project" json:"project"`
	Environment  string      `yaml:"environment" json:"environment"`
	TokenSHA256  string      `yaml:"token_sha256" json:"token_sha256"`
	TokenID      string      `yaml:"token_id" json:"token_id"`
	Allow        []string    `yaml:"allow" json:"allow"`
	MetricsAdmin bool        `yaml:"metrics_admin" json:"metrics_admin"`
	Rate         RateConfig  `yaml:"rate" json:"rate"`
	Quota        QuotaConfig `yaml:"quota" json:"quota"`
	Key          KeyConfig   `yaml:"key" json:"key"`
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
			m.Targets[i] = resolved
		}
		c.Models[name] = m
	}
	for _, caller := range c.Callers {
		if caller.ID == "" {
			return fmt.Errorf("caller missing id")
		}
		if caller.User != "" && slugify(caller.User) == "" {
			return fmt.Errorf("caller %s has invalid user", caller.ID)
		}
		if caller.Project != "" && slugify(caller.Project) == "" {
			return fmt.Errorf("caller %s has invalid project", caller.ID)
		}
		if caller.Environment != "" && slugify(caller.Environment) == "" {
			return fmt.Errorf("caller %s has invalid environment", caller.ID)
		}
		if caller.TokenSHA256 == "" || len(caller.TokenSHA256) != sha256.Size*2 {
			return fmt.Errorf("caller %s has invalid token_sha256", caller.ID)
		}
		if _, err := hex.DecodeString(caller.TokenSHA256); err != nil {
			return fmt.Errorf("caller %s has invalid token_sha256: %w", caller.ID, err)
		}
		for _, group := range caller.Allow {
			if _, ok := c.Models[group]; !ok {
				return fmt.Errorf("caller %s allows unknown model group %s", caller.ID, group)
			}
		}
	}
	return nil
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
		cfg.TimeoutMS == 0 &&
		cfg.MaxResponseBytes == 0 &&
		len(cfg.Headers) == 0 &&
		strings.TrimSpace(cfg.OnError) == ""
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
