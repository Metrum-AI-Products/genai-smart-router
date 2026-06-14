package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	Listen  string        `yaml:"listen"`
	Cache   CacheConfig   `yaml:"cache"`
	Logging LoggingConfig `yaml:"logging"`
	UsageDB UsageDBConfig `yaml:"usage_db"`
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
	Models     map[string]ProviderModel `yaml:"models"`
}

type ProviderModel struct {
	Model       string `yaml:"model" json:"model"`
	Dialect     string `yaml:"dialect" json:"dialect,omitempty"`
	DisplayName string `yaml:"display_name" json:"displayName,omitempty"`
	// Weight is accepted for legacy configs but intentionally ignored.
	// Routing weights are group-local and belong on ModelGroup targets.
	Weight int    `yaml:"weight" json:"weight,omitempty"`
	RPM    int    `yaml:"rpm" json:"rpm,omitempty"`
	Tier   string `yaml:"tier" json:"tier,omitempty"`
	Cost   int    `yaml:"cost" json:"cost,omitempty"`
}

type ModelGroup struct {
	Strategy string   `yaml:"strategy"`
	Script   string   `yaml:"script"`
	Targets  []Target `yaml:"targets"`
}

type Target struct {
	Provider    string `yaml:"provider" json:"provider"`
	Model       string `yaml:"model" json:"model"`
	ModelRef    string `yaml:"model_ref" json:"modelRef,omitempty"`
	Dialect     string `yaml:"dialect" json:"dialect"`
	DisplayName string `yaml:"display_name" json:"displayName,omitempty"`
	ToolOnly    bool   `yaml:"tool_only" json:"toolOnly,omitempty"`
	Weight      int    `yaml:"weight" json:"weight"`
	RPM         int    `yaml:"rpm" json:"rpm"`
	Tier        string `yaml:"tier" json:"tier"`
	Cost        int    `yaml:"cost" json:"cost"`
}

type CallerConfig struct {
	ID          string      `yaml:"id" json:"id"`
	User        string      `yaml:"user" json:"user"`
	Project     string      `yaml:"project" json:"project"`
	Environment string      `yaml:"environment" json:"environment"`
	TokenSHA256 string      `yaml:"token_sha256" json:"token_sha256"`
	TokenID     string      `yaml:"token_id" json:"token_id"`
	Allow       []string    `yaml:"allow" json:"allow"`
	Rate        RateConfig  `yaml:"rate" json:"rate"`
	Quota       QuotaConfig `yaml:"quota" json:"quota"`
	Key         KeyConfig   `yaml:"key" json:"key"`
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
}

func (c *Config) Validate() error {
	if len(c.Provider) == 0 {
		return fmt.Errorf("at least one provider is required")
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
		}
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model group is required")
	}
	for name, m := range c.Models {
		if m.Strategy == "" {
			return fmt.Errorf("model group %s missing strategy", name)
		}
		if strings.EqualFold(m.Strategy, "script") && m.Script == "" {
			return fmt.Errorf("model group %s uses script strategy but has no script path", name)
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
	if target.Model == "" {
		return target, fmt.Errorf("model group %s target model_ref %s for provider %s resolved without model", group, target.ModelRef, target.Provider)
	}
	return target, nil
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
