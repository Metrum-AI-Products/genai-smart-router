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
	AttemptTimeoutMS int                  `yaml:"attempt_timeout_ms"`
	Targets          []Target             `yaml:"targets"`
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
