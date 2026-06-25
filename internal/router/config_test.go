package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

func TestLoadEnvJSONSetsMissingValuesOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	if err := os.WriteFile(path, []byte(`{"ROUTER_TEST_ENV":"from_file","ROUTER_KEEP_ENV":"from_file"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTER_TEST_ENV", "")
	t.Setenv("ROUTER_KEEP_ENV", "existing")
	os.Unsetenv("ROUTER_TEST_ENV")
	if err := loadEnvJSON(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ROUTER_TEST_ENV"); got != "from_file" {
		t.Fatalf("ROUTER_TEST_ENV=%q", got)
	}
	if got := os.Getenv("ROUTER_KEEP_ENV"); got != "existing" {
		t.Fatalf("ROUTER_KEEP_ENV overwritten: %q", got)
	}
}

func TestEnvExampleContainsOnlySafePlaceholders(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "env.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("env.example.json must remain valid JSON: %v", err)
	}

	liveSecretRe := regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{16,}|sk-or-v1-[A-Za-z0-9_-]{16,}|xai-[A-Za-z0-9_-]{16,}|ghp_[A-Za-z0-9_]{16,}|[A-Za-z0-9_-]{32,})`)
	for name, value := range values {
		if strings.HasSuffix(name, "_API_KEY") && value != "" {
			t.Fatalf("env.example.json %s must be an empty placeholder", name)
		}
		if liveSecretRe.MatchString(value) {
			t.Fatalf("env.example.json %s contains a live-looking secret value", name)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains("\n"+string(gitignore)+"\n", "\nenv.json\n") {
		t.Fatal(".gitignore must keep real env.json out of source control")
	}
}

func TestProviderModelRefsResolveAndOverride(t *testing.T) {
	cfg := minimalConfig(t)
	provider := cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"small": {
			Model:                              "mock-small",
			Weight:                             99,
			Tier:                               "cheap",
			RPM:                                10,
			InputPricePerMillionUSD:            0.25,
			OutputPricePerMillionUSD:           1.25,
			ImageInputPricePerMillionTokensUSD: 3.5,
			ImageInputPricePerImageUSD:         0.002,
			PricingSource:                      "https://example.test/pricing",
			PricingUpdatedAt:                   "2026-06-17",
			ToolSupport: ToolSupport{
				OpenAIResponses: []string{"function"},
			},
		},
		"large": {Model: "mock-large", Weight: 99, Tier: "heavy"},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", ModelRef: "small"},
		{Provider: "mock", ModelRef: "large", Weight: 9},
		{Provider: "mock", Model: "direct-model", Weight: 1},
	}}
	cfg.Models["fast"] = ModelGroup{Strategy: "weighted", Targets: []Target{
		{Provider: "mock", ModelRef: "small", Weight: 3},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	targets := cfg.Models["default"].Targets
	if targets[0].Model != "mock-small" || targets[0].Weight != 0 || targets[0].Tier != "cheap" || targets[0].RPM != 10 {
		t.Fatalf("small ref not resolved: %#v", targets[0])
	}
	if targets[0].InputPricePerMillionUSD != 0.25 || targets[0].OutputPricePerMillionUSD != 1.25 ||
		targets[0].ImageInputPricePerMillionTokensUSD != 3.5 || targets[0].ImageInputPricePerImageUSD != 0.002 ||
		targets[0].PricingSource != "https://example.test/pricing" || targets[0].PricingUpdatedAt != "2026-06-17" ||
		len(targets[0].ToolSupport.OpenAIResponses) != 1 || targets[0].ToolSupport.OpenAIResponses[0] != "function" {
		t.Fatalf("small ref metadata not resolved: %#v", targets[0])
	}
	if targets[1].Model != "mock-large" || targets[1].Weight != 9 || targets[1].Tier != "heavy" {
		t.Fatalf("large ref override not resolved: %#v", targets[1])
	}
	if targets[2].Model != "direct-model" {
		t.Fatalf("direct model target changed: %#v", targets[2])
	}
	if got := cfg.Models["fast"].Targets[0].Weight; got != 3 {
		t.Fatalf("group-local target weight = %d, want 3", got)
	}
}

func TestContentCaptureConfigValidation(t *testing.T) {
	falseValue := false
	for _, tt := range []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "enabled without scope",
			edit: func(cfg *Config) {
				cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, RetentionDays: 30}
			},
			want: "no capture scope",
		},
		{
			name: "redaction disabled",
			edit: func(cfg *Config) {
				cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true, RedactBeforeStorage: &falseValue}
			},
			want: "redact_before_storage must remain true",
		},
		{
			name: "forbidden header",
			edit: func(cfg *Config) {
				cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true, CaptureHeadersAllowlist: []string{"Authorization"}}
			},
			want: "forbidden header",
		},
		{
			name: "invalid custom regex",
			edit: func(cfg *Config) {
				cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true, RedactionPatterns: []ContentCaptureRedactionRule{{Name: "bad", Expression: "["}}}
			},
			want: "redaction pattern bad is invalid",
		},
		{
			name: "unsupported encryption",
			edit: func(cfg *Config) {
				cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true, Encryption: ContentCaptureEncryptionConfig{Enabled: true, KMSKeyID: "kms-test"}}
			},
			want: "encryption.enabled is not supported yet",
		},
		{
			name: "caller capture requires usage db",
			edit: func(cfg *Config) {
				enabled := false
				cfg.Server.UsageDB.Enable = &enabled
				cfg.Callers[0].ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true}
			},
			want: "content_capture requires usage_db enabled",
		},
		{
			name: "model group capture requires usage db",
			edit: func(cfg *Config) {
				enabled := false
				cfg.Server.UsageDB.Enable = &enabled
				group := cfg.Models["default"]
				group.ContentCapture = ContentCaptureConfig{Enabled: true, CaptureRequest: true}
				cfg.Models["default"] = group
			},
			want: "content_capture requires usage_db enabled",
		},
		{
			name: "caller override validates",
			edit: func(cfg *Config) {
				cfg.Callers[0].ContentCapture = ContentCaptureConfig{Enabled: true}
			},
			want: "no capture scope",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			cfg.setDefaults()
			tt.edit(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestExplicitAccountsValidateCallerOwnership(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Users = []UserConfig{{ID: "Alice", Name: "Alice Example"}}
	cfg.Projects = []ProjectConfig{{ID: "Metrum Insights", Name: "Metrum Insights"}}
	cfg.ProjectMemberships = []ProjectMembershipConfig{{UserID: "Alice", Project: "Metrum Insights", Role: "admin"}}
	cfg.Callers[0].OwnerUser = "Alice"
	cfg.Callers[0].Project = "Metrum Insights"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() explicit account directory: %v", err)
	}
}

func TestExplicitAccountsRejectInactiveMembership(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Users = []UserConfig{{ID: "alice"}}
	cfg.Projects = []ProjectConfig{{ID: "metrum-insights"}}
	cfg.ProjectMemberships = []ProjectMembershipConfig{{UserID: "alice", Project: "metrum-insights", Status: "disabled"}}
	cfg.Callers[0].OwnerUser = "alice"
	cfg.Callers[0].Project = "metrum-insights"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "non-active membership") {
		t.Fatalf("Validate() error=%v, want inactive membership rejection", err)
	}
}

func TestExplicitAccountsRejectConflictingLegacyUserAlias(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Callers[0].OwnerUser = "alice"
	cfg.Callers[0].User = "bob"
	cfg.Callers[0].Project = "metrum-insights"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "conflicting owner_user") {
		t.Fatalf("Validate() error=%v, want owner_user/user conflict", err)
	}
}

func TestExplicitAccountsRejectAccountlessCaller(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Users = []UserConfig{{ID: "alice"}}
	cfg.Projects = []ProjectConfig{{ID: "metrum-insights"}}
	cfg.ProjectMemberships = []ProjectMembershipConfig{{UserID: "alice", Project: "metrum-insights"}}
	cfg.Callers[0].OwnerUser = ""
	cfg.Callers[0].User = ""
	cfg.Callers[0].Project = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "missing owner_user and project") {
		t.Fatalf("Validate() error=%v, want accountless caller rejection", err)
	}
}

func TestExplicitAccountsRejectDuplicateNormalizedIDs(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Users = []UserConfig{{ID: "Alice Smith"}, {ID: "alice/smith"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate user id") {
		t.Fatalf("Validate() error=%v, want duplicate user id rejection", err)
	}
}

func TestMissingProviderModelRefFailsValidation(t *testing.T) {
	cfg := minimalConfig(t)
	provider := cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"known": {Model: "mock-known"},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "missing"}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown model_ref missing") {
		t.Fatalf("expected missing model_ref error, got %v", err)
	}
}

func TestProviderModelPricingAndToolSupportValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		model  ProviderModel
		target Target
		want   string
	}{
		{
			name:  "negative provider input price",
			model: ProviderModel{Model: "mock-known", InputPricePerMillionUSD: -0.01},
			want:  "input_price_per_million_usd",
		},
		{
			name:  "duplicate provider tool support",
			model: ProviderModel{Model: "mock-known", ToolSupport: ToolSupport{OpenAIResponses: []string{"function", "function"}}},
			want:  "duplicate",
		},
		{
			name:   "negative target output price",
			model:  ProviderModel{Model: "mock-known"},
			target: Target{OutputPricePerMillionUSD: -0.01},
			want:   "output_price_per_million_usd",
		},
		{
			name:  "negative provider image token price",
			model: ProviderModel{Model: "mock-known", ImageInputPricePerMillionTokensUSD: -0.01},
			want:  "image_input_price_per_million_tokens_usd",
		},
		{
			name:   "negative target image unit price",
			model:  ProviderModel{Model: "mock-known"},
			target: Target{ImageInputPricePerImageUSD: -0.01},
			want:   "image_input_price_per_image_usd",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			provider := cfg.Provider["mock"]
			provider.Models = map[string]ProviderModel{"known": tt.model}
			cfg.Provider["mock"] = provider
			target := Target{Provider: "mock", ModelRef: "known"}
			if tt.target.OutputPricePerMillionUSD != 0 {
				target.OutputPricePerMillionUSD = tt.target.OutputPricePerMillionUSD
			}
			if tt.target.ImageInputPricePerMillionTokensUSD != 0 {
				target.ImageInputPricePerMillionTokensUSD = tt.target.ImageInputPricePerMillionTokensUSD
			}
			if tt.target.ImageInputPricePerImageUSD != 0 {
				target.ImageInputPricePerImageUSD = tt.target.ImageInputPricePerImageUSD
			}
			cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{target}}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateRejectsDuplicateCallerIdentifiers(t *testing.T) {
	const (
		hashOne = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		hashTwo = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	)
	for _, tt := range []struct {
		name          string
		callers       []CallerConfig
		want          string
		mustNotExpose string
	}{
		{
			name: "duplicate token hash different caller ids",
			callers: []CallerConfig{
				{ID: "alice", TokenSHA256: hashOne, Allow: []string{"default"}},
				{ID: "bob", TokenSHA256: hashOne, Allow: []string{"default"}},
			},
			want:          "duplicate caller token_sha256 for callers alice and bob",
			mustNotExpose: hashOne,
		},
		{
			name: "duplicate caller id different hashes",
			callers: []CallerConfig{
				{ID: "alice", TokenSHA256: hashOne, Allow: []string{"default"}},
				{ID: "alice", TokenSHA256: hashTwo, Allow: []string{"default"}},
			},
			want: "duplicate caller id",
		},
		{
			name: "duplicate public token id",
			callers: []CallerConfig{
				{ID: "alice", TokenSHA256: hashOne, TokenID: "rtr_metrum_alice_test_dev_k1", Allow: []string{"default"}},
				{ID: "bob", TokenSHA256: hashTwo, TokenID: "rtr_metrum_alice_test_dev_k1", Allow: []string{"default"}},
			},
			want: "duplicate caller token_id for callers alice and bob",
		},
		{
			name: "case insensitive duplicate hash",
			callers: []CallerConfig{
				{ID: "alice", TokenSHA256: hashOne, Allow: []string{"default"}},
				{ID: "bob", TokenSHA256: strings.ToUpper(hashOne), Allow: []string{"default"}},
			},
			want:          "duplicate caller token_sha256 for callers alice and bob",
			mustNotExpose: hashOne,
		},
		{
			name: "duplicate involving metrics admin caller",
			callers: []CallerConfig{
				{ID: "metrics-admin", TokenSHA256: hashOne, TokenID: "rtr_metrum_metrics_admin_test_k1", MetricsAdmin: true},
				{ID: "alice", TokenSHA256: hashOne, TokenID: "rtr_metrum_alice_test_dev_k1", Allow: []string{"default"}},
			},
			want:          "duplicate caller token_sha256 for callers metrics-admin and alice",
			mustNotExpose: hashOne,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			cfg.Callers = tt.callers
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tt.want)
			}
			if tt.mustNotExpose != "" && strings.Contains(err.Error(), tt.mustNotExpose) {
				t.Fatalf("Validate() exposed token hash in error: %v", err)
			}
		})
	}
}

func TestPIIFilterValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter PIIFilterConfig
		want   string
	}{
		{
			name:   "disabled but configured",
			filter: PIIFilterConfig{Rules: []PIIFilterRule{{Name: "email", Expression: `@`, PlaceholderPrefix: "EMAIL"}}},
			want:   "enabled is false",
		},
		{
			name:   "missing rules",
			filter: PIIFilterConfig{Enabled: true},
			want:   "requires at least one rule",
		},
		{
			name: "invalid mode",
			filter: PIIFilterConfig{
				Enabled: true,
				Mode:    "restore_everywhere",
				Rules:   []PIIFilterRule{{Name: "email", Expression: `@`, PlaceholderPrefix: "EMAIL"}},
			},
			want: "mode must be",
		},
		{
			name: "invalid regex",
			filter: PIIFilterConfig{
				Enabled: true,
				Rules:   []PIIFilterRule{{Name: "email", Expression: `[`, PlaceholderPrefix: "EMAIL"}},
			},
			want: "invalid expression",
		},
		{
			name: "duplicate rule",
			filter: PIIFilterConfig{
				Enabled: true,
				Rules: []PIIFilterRule{
					{Name: "email", Expression: `@`, PlaceholderPrefix: "EMAIL"},
					{Name: "email", Expression: `phone`, PlaceholderPrefix: "PHONE"},
				},
			},
			want: "duplicate rule",
		},
		{
			name: "missing placeholder prefix",
			filter: PIIFilterConfig{
				Enabled: true,
				Rules:   []PIIFilterRule{{Name: "email", Expression: `@`, PlaceholderPrefix: "!!!"}},
			},
			want: "missing placeholder_prefix",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			cfg.Models["default"] = ModelGroup{
				Strategy:  "static",
				PIIFilter: tt.filter,
				Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
			}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestAdminBasicAuthValidation(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	for _, tt := range []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{
			name: "disabled default",
		},
		{
			name: "disabled but users configured",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/dev"}}
			},
			want: "users configured but basic auth is disabled",
		},
		{
			name: "enabled without users",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Basic.Enabled = true
			},
			want: "requires at least one user",
		},
		{
			name: "duplicate usernames",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{
					{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/dev"},
					{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/dev"},
				}
			},
			want: "duplicate username",
		},
		{
			name: "missing env reference",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", Domain: "local/dev"}}
			},
			want: "requires password_hash_env",
		},
		{
			name: "invalid env hash",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_INVALID", "not-a-hash")
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_INVALID", Domain: "local/dev"}}
			},
			want: "password hash must be bcrypt",
		},
		{
			name: "env hash",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/dev"}}
			},
		},
		{
			name: "missing env hash",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_MISSING", Domain: "local/dev"}}
			},
			want: "password_hash_env SMART_ROUTER_ADMIN_PASSWORD_HASH_MISSING is not set",
		},
		{
			name: "missing domain",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST"}}
			},
			want: "domain is required",
		},
		{
			name: "invalid trusted proxy cidr",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.TrustedProxyCIDRs = []string{"not-a-cidr"}
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/dev"}}
			},
			want: "invalid CIDR",
		},
		{
			name: "valid configured subject and permission",
			configure: func(cfg *Config) {
				t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
				cfg.Server.AdminAuth.Basic.Enabled = true
				cfg.Server.AdminAuth.Basic.TrustedProxyCIDRs = []string{"127.0.0.1/32"}
				cfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{
					Username:        "admin",
					PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
					Subject:         "basic:admin",
					Domain:          "local/dev",
					Permissions:     []string{"admin:auth:read"},
				}}
			},
		},
		{
			name: "oidc missing env",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.OIDC.Enabled = true
				cfg.Server.AdminAuth.OIDC.IssuerURL = "https://accounts.example.com"
				cfg.Server.AdminAuth.OIDC.ClientIDEnv = "TEST_OIDC_CLIENT_ID_MISSING"
				cfg.Server.AdminAuth.OIDC.ClientSecretEnv = "TEST_OIDC_CLIENT_SECRET"
				cfg.Server.AdminAuth.OIDC.RedirectURL = "https://router.example.com/admin/auth/callback"
				cfg.Server.AdminAuth.OIDC.Domain = "example/prod"
				t.Setenv("TEST_OIDC_CLIENT_SECRET", "secret")
			},
			want: "client_id_env TEST_OIDC_CLIENT_ID_MISSING is not set",
		},
		{
			name: "oidc invalid redirect",
			configure: func(cfg *Config) {
				t.Setenv("TEST_OIDC_CLIENT_ID", "client-id")
				t.Setenv("TEST_OIDC_CLIENT_SECRET", "secret")
				cfg.Server.AdminAuth.OIDC.Enabled = true
				cfg.Server.AdminAuth.OIDC.IssuerURL = "https://accounts.example.com"
				cfg.Server.AdminAuth.OIDC.ClientIDEnv = "TEST_OIDC_CLIENT_ID"
				cfg.Server.AdminAuth.OIDC.ClientSecretEnv = "TEST_OIDC_CLIENT_SECRET"
				cfg.Server.AdminAuth.OIDC.RedirectURL = "http://router.example.com/admin/auth/callback"
				cfg.Server.AdminAuth.OIDC.Domain = "example/prod"
			},
			want: "redirect_url must use https",
		},
		{
			name: "oidc invalid allowed domain",
			configure: func(cfg *Config) {
				t.Setenv("TEST_OIDC_CLIENT_ID", "client-id")
				t.Setenv("TEST_OIDC_CLIENT_SECRET", "secret")
				cfg.Server.AdminAuth.OIDC.Enabled = true
				cfg.Server.AdminAuth.OIDC.IssuerURL = "https://accounts.example.com"
				cfg.Server.AdminAuth.OIDC.ClientIDEnv = "TEST_OIDC_CLIENT_ID"
				cfg.Server.AdminAuth.OIDC.ClientSecretEnv = "TEST_OIDC_CLIENT_SECRET"
				cfg.Server.AdminAuth.OIDC.RedirectURL = "https://router.example.com/admin/auth/callback"
				cfg.Server.AdminAuth.OIDC.AllowedDomains = []string{"@example.com"}
				cfg.Server.AdminAuth.OIDC.Domain = "example/prod"
			},
			want: "allowed domain",
		},
		{
			name: "oidc valid localhost development",
			configure: func(cfg *Config) {
				t.Setenv("TEST_OIDC_CLIENT_ID", "client-id")
				t.Setenv("TEST_OIDC_CLIENT_SECRET", "secret")
				cfg.Server.AdminAuth.OIDC.Enabled = true
				cfg.Server.AdminAuth.OIDC.IssuerURL = "http://localhost:8081"
				cfg.Server.AdminAuth.OIDC.ClientIDEnv = "TEST_OIDC_CLIENT_ID"
				cfg.Server.AdminAuth.OIDC.ClientSecretEnv = "TEST_OIDC_CLIENT_SECRET"
				cfg.Server.AdminAuth.OIDC.RedirectURL = "http://localhost:8080/admin/auth/callback"
				cfg.Server.AdminAuth.OIDC.AllowedDomains = []string{"example.com"}
				cfg.Server.AdminAuth.OIDC.Domain = "example/prod"
				cfg.Server.AdminAuth.Sessions = AdminSessionConfig{CookieName: "test_admin_session", TTL: time.Hour, SecureCookies: testBoolPtr(false), SameSite: "lax"}
			},
		},
		{
			name: "session same_site none requires secure cookies",
			configure: func(cfg *Config) {
				t.Setenv("TEST_OIDC_CLIENT_ID", "client-id")
				t.Setenv("TEST_OIDC_CLIENT_SECRET", "secret")
				cfg.Server.AdminAuth.OIDC.Enabled = true
				cfg.Server.AdminAuth.OIDC.IssuerURL = "https://accounts.example.com"
				cfg.Server.AdminAuth.OIDC.ClientIDEnv = "TEST_OIDC_CLIENT_ID"
				cfg.Server.AdminAuth.OIDC.ClientSecretEnv = "TEST_OIDC_CLIENT_SECRET"
				cfg.Server.AdminAuth.OIDC.RedirectURL = "https://router.example.com/admin/auth/callback"
				cfg.Server.AdminAuth.OIDC.Domain = "example/prod"
				cfg.Server.AdminAuth.Sessions = AdminSessionConfig{CookieName: "test_admin_session", TTL: time.Hour, SecureCookies: testBoolPtr(false), SameSite: "none"}
			},
			want: "same_site none requires secure_cookies",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			if tt.configure != nil {
				tt.configure(cfg)
			}
			err := cfg.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() err=%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), hash) {
				t.Fatalf("Validate() exposed password hash: %v", err)
			}
		})
	}
}

func TestAdminAuthorizationSourceValidation(t *testing.T) {
	disabled := false
	for _, tt := range []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{
			name: "db source allows policy-free config",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Source: "db"}
			},
		},
		{
			name: "db source requires usage db enabled",
			configure: func(cfg *Config) {
				cfg.Server.UsageDB.Enable = &disabled
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Source: "db"}
			},
			want: "source db requires usage_db enabled",
		},
		{
			name: "db source rejects inline policy mixing",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Source: "db", Policy: []string{"p, role, local/test, metrics, read"}}
			},
			want: "source db cannot combine inline policy or policy_file",
		},
		{
			name: "unknown source rejected",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Source: "elsewhere"}
			},
			want: "source must be static or db",
		},
		{
			name: "static policy rejects unsafe fields",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, bearer secret, local/test, metrics, read"}}
			},
			want: "contains unsafe field",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			tt.configure(cfg)
			err := cfg.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() err=%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestAdminReportsConfigValidation(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	for _, tt := range []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{
			name: "enabled requires browser auth",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, basic:admin, local/test, admin:reports, read"}}
				cfg.Server.AdminReports.Enabled = true
			},
			want: "requires server.admin_auth.basic or server.admin_auth.oidc enabled",
		},
		{
			name: "enabled requires authorization",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{Enabled: true, AllowInsecureHTTP: true, Users: []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", Domain: "local/test"}}}
				cfg.Server.AdminReports.Enabled = true
			},
			want: "requires server.admin_auth.authorization enabled",
		},
		{
			name: "invalid policy",
			configure: func(cfg *Config) {
				cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, only-two-fields"}}
			},
			want: "p lines require",
		},
		{
			name: "bad range",
			configure: func(cfg *Config) {
				cfg.Server.AdminReports.DefaultSince = "32d"
				cfg.Server.AdminReports.MaxRange = "31d"
			},
			want: "default_since cannot exceed max_range",
		},
		{
			name: "bad rows",
			configure: func(cfg *Config) {
				cfg.Server.AdminReports.MaxRows = 10001
			},
			want: "max_rows",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			cfg.setDefaults()
			tt.configure(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestPIIFilterDocumentedYAMLShapeValidates(t *testing.T) {
	raw := []byte(`
server:
  logging:
    path: requests.jsonl
providers:
  mock:
    base_url: http://127.0.0.1:1/v1
    dialect: openai-chat
    api_key: secret
models:
  sensitive-workloads:
    strategy: weighted
    pii_filter:
      enabled: true
      mode: redact_and_restore
      restore_response: true
      max_replacements_per_request: 200
      apply_to:
        system: true
        messages: true
        responses_input: true
        tool_results: true
        image_urls: false
      rules:
        - name: email
          expression: '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}'
          placeholder_prefix: EMAIL
        - name: us_phone
          expression: '\b(?:\+1[-. ]?)?\(?[2-9]\d{2}\)?[-. ]?[2-9]\d{2}[-. ]?\d{4}\b'
          placeholder_prefix: PHONE
        - name: us_ssn
          expression: '\b\d{3}-\d{2}-\d{4}\b'
          placeholder_prefix: US_SSN
    targets:
      - { provider: mock, model: mock-model, weight: 1 }
callers:
  - id: alice
    token_sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    allow: [sensitive-workloads]
`)
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("documented pii_filter YAML shape did not validate: %v", err)
	}
	filter := cfg.Models["sensitive-workloads"].PIIFilter
	if !filter.Enabled || filter.Mode != "redact_and_restore" || len(filter.Rules) != 3 {
		t.Fatalf("unexpected pii_filter decode: %#v", filter)
	}
	if filter.ApplyTo.System == nil || !*filter.ApplyTo.System || filter.ApplyTo.ImageURLs {
		t.Fatalf("unexpected apply_to decode: %#v", filter.ApplyTo)
	}
}

func TestWeightedOrderTreatsOmittedTargetWeightAsOne(t *testing.T) {
	targets := weightedOrder([]Target{{Provider: "mock", Model: "only"}})
	if len(targets) != 1 || targets[0].Provider != "mock" || targets[0].Model != "only" {
		t.Fatalf("weighted order with omitted weight = %#v", targets)
	}
}

func TestExampleConfigDefaultIncludesLatestCodingTargets(t *testing.T) {
	raw, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	standardSum := sha256.Sum256([]byte("rtr_example_standard_test"))
	codingSum := sha256.Sum256([]byte("rtr_example_coding_test"))
	metricsAdminSum := sha256.Sum256([]byte("rtr_example_metrics_admin_test"))
	contentAdminSum := sha256.Sum256([]byte("rtr_example_content_admin_test"))
	text := strings.ReplaceAll(string(raw), "REPLACE_WITH_SHA256_HEX_OF_STANDARD_ROUTER_TOKEN", hex.EncodeToString(standardSum[:]))
	text = strings.ReplaceAll(text, "REPLACE_WITH_SHA256_HEX_OF_CODING_ROUTER_TOKEN", hex.EncodeToString(codingSum[:]))
	text = strings.ReplaceAll(text, "REPLACE_WITH_SHA256_HEX_OF_METRICS_ADMIN_ROUTER_TOKEN", hex.EncodeToString(metricsAdminSum[:]))
	text = strings.ReplaceAll(text, "REPLACE_WITH_SHA256_HEX_OF_CONTENT_ADMIN_ROUTER_TOKEN", hex.EncodeToString(contentAdminSum[:]))
	text = strings.ReplaceAll(text, "script: scripts/router.ts", "script: ../../scripts/router.ts")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultGroupTargets(t, cfg.Models["default"])
	assertAnthropicCompatibleProvider(t, cfg.Provider["minimax_anthropic"], "m3", "MiniMax-M3")
	assertAnthropicCompatibleProvider(t, cfg.Provider["kimi_anthropic"], "kimi-k2.7-code", "kimi-k2.7-code")
	assertAnthropicCompatibleProvider(t, cfg.Provider["baseten_anthropic"], "gpt-oss-120b", "openai/gpt-oss-120b")
	if cfg.Provider["baseten"].Models["gpt-oss-120b"].Model != "openai/gpt-oss-120b" {
		t.Fatalf("example config missing Baseten GPT OSS 120B catalog entry")
	}
	assertOpenAIChatProvider(t, cfg.Provider["crusoe"], "llama-3-3-70b-instruct", "meta-llama/Llama-3.3-70B-Instruct")
	if got := cfg.Provider["crusoe"].Models["gpt-oss-120b"]; got.Model != "openai/gpt-oss-120b" || got.InputPricePerMillionUSD != 0.05 || got.OutputPricePerMillionUSD != 0.2 {
		t.Fatalf("example config Crusoe GPT OSS catalog entry=%#v", got)
	}
	if got := cfg.Provider["crusoe"].Models["gemma-4-31b-it"]; got.Model != "google/gemma-4-31b-it" || got.InputPricePerMillionUSD != 0.14 || got.OutputPricePerMillionUSD != 0.4 ||
		!stringSliceContains(got.ToolSupport.OpenAIChat, "tools") ||
		!stringSliceContains(got.ToolSupport.OpenAIChat, "tool_choice") ||
		!stringSliceContains(got.ToolSupport.OpenAIChat, "structured_outputs") {
		t.Fatalf("example config Crusoe Gemma catalog entry=%#v", got)
	}
	assertAnthropicCompatibleProvider(t, cfg.Provider["openrouter_anthropic"], "gemma-4-26b-a4b-it-nitro", "google/gemma-4-26b-a4b-it:nitro")
	for _, name := range []string{"small", "medium", "high"} {
		group, ok := cfg.Models[name]
		if !ok {
			t.Fatalf("example config missing %s model group", name)
		}
		if len(group.Targets) == 0 {
			t.Fatalf("example config %s model group has no targets", name)
		}
	}
	for name, group := range cfg.Models {
		if name == "agent-tools-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "minimax" || group.Targets[0].Model != "MiniMax-M3" || group.Targets[0].Dialect != "openai-responses" {
				t.Fatalf("example config agent-tools-smoke=%#v, want static minimax MiniMax-M3 responses", group)
			}
			continue
		}
		if name == "claude-tools-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "minimax_anthropic" || group.Targets[0].Model != "MiniMax-M3" {
				t.Fatalf("example config claude-tools-smoke=%#v, want static minimax Anthropic-compatible MiniMax-M3", group)
			}
			continue
		}
		if name == "agent-tools-smoke-openrouter" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_responses" || group.Targets[0].Model != "anthropic/claude-sonnet-4.6" {
				t.Fatalf("example config agent-tools-smoke-openrouter=%#v, want static OpenRouter Claude Sonnet responses", group)
			}
			continue
		}
		if name == "claude-tools-smoke-openrouter" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_anthropic" || group.Targets[0].Model != "anthropic/claude-sonnet-4.6" {
				t.Fatalf("example config claude-tools-smoke-openrouter=%#v, want static OpenRouter Claude Sonnet Anthropic-compatible target", group)
			}
			continue
		}
		if name == "claude-tools-smoke-openrouter-gemma" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_anthropic" || group.Targets[0].Model != "google/gemma-4-26b-a4b-it:nitro" {
				t.Fatalf("example config claude-tools-smoke-openrouter-gemma=%#v, want static OpenRouter Gemma Anthropic-compatible target", group)
			}
			continue
		}
		if name == "baseten-nemotron-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "nvidia/Nemotron-120B-A12B" {
				t.Fatalf("example config baseten-nemotron-smoke=%#v, want static Baseten Nemotron target", group)
			}
			continue
		}
		if name == "baseten-glm52-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "zai-org/GLM-5.2" {
				t.Fatalf("example config baseten-glm52-smoke=%#v, want static Baseten GLM 5.2 target", group)
			}
			continue
		}
		if name == "baseten-gpt-oss-120b-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "openai/gpt-oss-120b" {
				t.Fatalf("example config baseten-gpt-oss-120b-smoke=%#v, want static Baseten GPT OSS 120B target", group)
			}
			continue
		}
		if name == "baseten-gpt-oss-120b-claude-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten_anthropic" || group.Targets[0].Model != "openai/gpt-oss-120b" {
				t.Fatalf("example config baseten-gpt-oss-120b-claude-smoke=%#v, want static Baseten Anthropic GPT OSS 120B target", group)
			}
			if group.Targets[0].ToolOnly {
				t.Fatalf("example config baseten-gpt-oss-120b-claude-smoke=%#v, want text-and-tool smoke target, not tool_only", group)
			}
			continue
		}
		if name == "crusoe-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "crusoe" || group.Targets[0].Model != "meta-llama/Llama-3.3-70B-Instruct" {
				t.Fatalf("example config crusoe-smoke=%#v, want static Crusoe Llama 3.3 70B Instruct target", group)
			}
			if !stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "tools") ||
				!stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "tool_choice") ||
				!stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "structured_outputs") {
				t.Fatalf("example config crusoe-smoke missing validated OpenAI Chat capability metadata: %#v", group.Targets[0].ToolSupport)
			}
			continue
		}
		if name == "crusoe-gemma-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "crusoe" || group.Targets[0].Model != "google/gemma-4-31b-it" {
				t.Fatalf("example config crusoe-gemma-smoke=%#v, want static Crusoe Gemma 4 31B-it target", group)
			}
			if !stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "tools") ||
				!stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "tool_choice") ||
				!stringSliceContains(group.Targets[0].ToolSupport.OpenAIChat, "structured_outputs") {
				t.Fatalf("example config crusoe-gemma-smoke missing validated OpenAI Chat capability metadata: %#v", group.Targets[0].ToolSupport)
			}
			continue
		}
		if name == "warp-agent-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "nvidia/Nemotron-120B-A12B" {
				t.Fatalf("example config warp-agent-smoke=%#v, want static Baseten Nemotron OpenAI Chat tool target", group)
			}
			continue
		}
		if name == "vision" {
			if group.Strategy != "weighted" || len(group.Targets) < 2 {
				t.Fatalf("example config vision=%#v, want weighted multi-target vision group", group)
			}
			seen := map[string]bool{}
			for _, target := range group.Targets {
				seen[target.Provider+":"+target.Model] = true
				if !stringSliceContains(target.InputModalities, "image") {
					t.Fatalf("example config vision target lacks image modality: %#v", target)
				}
			}
			if !seen["xai:grok-4.3"] || !seen["openai:gpt-5.4-nano"] {
				t.Fatalf("example config vision targets=%#v, want xAI Grok 4.3 and OpenAI GPT-5.4 Nano", group.Targets)
			}
			continue
		}
		if name == "external-policy-demo" {
			if group.Strategy != "external" ||
				group.ExternalPolicy.URL != "http://127.0.0.1:18090/route" ||
				!stringSliceContains(group.ExternalPolicy.AllowHosts, "127.0.0.1") ||
				len(group.Targets) != 2 {
				t.Fatalf("example config external-policy-demo=%#v, want prompt-size external policy demo", group)
			}
			continue
		}
		if name == "adaptive-agent" {
			if group.Strategy != "dynamic_score" || len(group.Targets) != 3 || len(group.RoutingPolicy.DynamicScore.ScoreTerms) != 2 {
				t.Fatalf("example config adaptive-agent=%#v, want dynamic_score reference group with three targets and score terms", group)
			}
			if group.RoutingPolicy.DynamicScore.MinObservations != 20 {
				t.Fatalf("adaptive-agent min_observations=%d, want 20", group.RoutingPolicy.DynamicScore.MinObservations)
			}
			continue
		}
		if group.Strategy != "weighted" {
			t.Fatalf("example config group %s strategy=%q want weighted", name, group.Strategy)
		}
		assertActiveGroupPolicy(t, name, group)
	}
	wantAllows := map[string][]string{
		"standard-dev":      {"default", "fast", "small", "vision", "external-policy-demo"},
		"coding-dev":        {"default", "fast", "big-coder", "small", "medium", "high", "vision", "agent-tools-smoke", "claude-tools-smoke", "agent-tools-smoke-openrouter", "claude-tools-smoke-openrouter", "claude-tools-smoke-openrouter-gemma", "baseten-nemotron-smoke", "warp-agent-smoke", "baseten-glm52-smoke", "baseten-gpt-oss-120b-smoke", "baseten-gpt-oss-120b-claude-smoke", "crusoe-smoke", "crusoe-gemma-smoke"},
		"metrics-admin-dev": {},
		"content-admin-dev": {},
	}
	for _, caller := range cfg.Callers {
		want, ok := wantAllows[caller.ID]
		if !ok {
			continue
		}
		if strings.Join(caller.Allow, ",") != strings.Join(want, ",") {
			t.Fatalf("caller %s allow=%v want %v", caller.ID, caller.Allow, want)
		}
		if caller.ID == "metrics-admin-dev" && !caller.MetricsAdmin {
			t.Fatalf("caller %s metrics_admin=false, want true", caller.ID)
		}
		if caller.ID != "metrics-admin-dev" && caller.MetricsAdmin {
			t.Fatalf("caller %s metrics_admin=true, want false", caller.ID)
		}
		if caller.ID == "content-admin-dev" && !caller.ContentAdmin {
			t.Fatalf("caller %s content_admin=false, want true", caller.ID)
		}
		if caller.ID != "content-admin-dev" && caller.ContentAdmin {
			t.Fatalf("caller %s content_admin=true, want false", caller.ID)
		}
		delete(wantAllows, caller.ID)
	}
	if len(wantAllows) != 0 {
		t.Fatalf("example config missing caller profiles: %#v", wantAllows)
	}
}

func assertDefaultGroupTargets(t *testing.T, defaultGroup ModelGroup) {
	t.Helper()
	want := map[string]string{
		"baseten:nvidia/Nemotron-120B-A12B":          "nvidia/Nemotron-120B-A12B",
		"baseten:openai/gpt-oss-120b":                "openai/gpt-oss-120b",
		"baseten:zai-org/GLM-5.2":                    "zai-org/GLM-5.2",
		"minimax:MiniMax-M3":                         "MiniMax-M3",
		"kimi:kimi-k2.7-code":                        "kimi-k2.7-code",
		"openrouter:google/gemma-4-26b-a4b-it:nitro": "google/gemma-4-26b-a4b-it:nitro",
		"openai:gpt-5.4-nano":                        "gpt-5.4-nano",
	}
	for name, model := range want {
		found := false
		for _, target := range defaultGroup.Targets {
			if target.Provider+":"+target.Model == name && target.Model == model {
				found = true
				if !target.ToolOnly && target.Weight <= 0 {
					t.Fatalf("%s target resolved with non-positive weight: %#v", name, target)
				}
				break
			}
		}
		if !found {
			t.Fatalf("default group missing resolved target %s; targets=%#v", name, defaultGroup.Targets)
		}
	}
}

func assertAnthropicCompatibleProvider(t *testing.T, provider ProviderConfig, ref, model string) {
	t.Helper()
	if provider.Dialect != "anthropic" || normalizeAuthScheme(provider.AuthScheme) != "bearer" || provider.Models[ref].Model != model {
		t.Fatalf("provider not configured for Anthropic-compatible bearer skin: %#v", provider)
	}
}

func assertOpenAIChatProvider(t *testing.T, provider ProviderConfig, ref, model string) {
	t.Helper()
	if provider.Dialect != "openai-chat" || normalizeAuthScheme(provider.AuthScheme) != "bearer" || provider.Models[ref].Model != model {
		t.Fatalf("provider not configured for OpenAI Chat-compatible bearer skin: %#v", provider)
	}
}

func assertResponsesCompatibleProvider(t *testing.T, provider ProviderConfig, ref, model string) {
	t.Helper()
	if provider.Dialect != "openai-responses" || provider.Models[ref].Model != model {
		t.Fatalf("provider not configured for OpenAI Responses-compatible skin: %#v", provider)
	}
}

func assertActiveGroupPolicy(t *testing.T, name string, group ModelGroup) {
	t.Helper()
	totalWeight := 0
	m3Weight := 0
	kimiWeight := 0
	gemmaWeight := 0
	openAIWeight := 0
	basetenNemotronWeight := 0
	basetenGLMWeight := 0
	basetenGPTOSSWeight := 0
	crusoeGemmaWeight := 0
	crusoeGLMWeight := 0
	normalTargets := 0
	codexToolTarget := false
	codexOpenRouterToolTarget := false
	claudeBasetenToolTarget := false
	claudeMiniMaxToolTarget := false
	claudeKimiToolTarget := false
	claudeOpenRouterToolTarget := false
	claudeGemmaToolTarget := false
	openAIChatCrusoeGemmaToolTarget := false
	for _, target := range group.Targets {
		if violatesCurrentRoutingPolicy(target) {
			t.Fatalf("example config group %s has routing-policy violation %#v", name, target)
		}
		if target.ToolOnly {
			if target.Provider == "minimax" && target.Model == "MiniMax-M3" && target.Dialect == "openai-responses" {
				codexToolTarget = true
			}
			if target.Provider == "openrouter_responses" && target.Model == "anthropic/claude-sonnet-4.6" {
				codexOpenRouterToolTarget = true
			}
			if target.Provider == "minimax_anthropic" && target.Model == "MiniMax-M3" {
				claudeMiniMaxToolTarget = true
			}
			if target.Provider == "baseten_anthropic" && target.Model == "openai/gpt-oss-120b" {
				claudeBasetenToolTarget = true
			}
			if target.Provider == "kimi_anthropic" && target.Model == "kimi-k2.7-code" && target.DefaultThinking["type"] == "enabled" {
				claudeKimiToolTarget = true
			}
			if target.Provider == "openrouter_anthropic" && target.Model == "anthropic/claude-sonnet-4.6" {
				claudeOpenRouterToolTarget = true
			}
			if target.Provider == "openrouter_anthropic" && target.Model == "google/gemma-4-26b-a4b-it:nitro" {
				claudeGemmaToolTarget = true
			}
			if target.Provider == "crusoe" && target.Model == "google/gemma-4-31b-it" {
				openAIChatCrusoeGemmaToolTarget = true
			}
			continue
		}
		normalTargets++
		totalWeight += target.Weight
		if target.Provider == "minimax" && target.Model == "MiniMax-M3" {
			m3Weight += target.Weight
		}
		if target.Provider == "kimi" && target.Model == "kimi-k2.7-code" {
			kimiWeight += target.Weight
		}
		if target.Provider == "openrouter" && target.Model == "google/gemma-4-26b-a4b-it:nitro" {
			gemmaWeight += target.Weight
		}
		if target.Provider == "openai" && target.Model == "gpt-5.4-nano" {
			openAIWeight += target.Weight
		}
		if target.Provider == "baseten" && target.Model == "nvidia/Nemotron-120B-A12B" {
			basetenNemotronWeight += target.Weight
		}
		if target.Provider == "baseten" && target.Model == "zai-org/GLM-5.2" {
			basetenGLMWeight += target.Weight
		}
		if target.Provider == "baseten" && target.Model == "openai/gpt-oss-120b" {
			basetenGPTOSSWeight += target.Weight
		}
		if target.Provider == "crusoe" && target.Model == "google/gemma-4-31b-it" {
			crusoeGemmaWeight += target.Weight
		}
		if target.Provider == "crusoe" && target.Model == "zai/GLM-5.2" {
			crusoeGLMWeight += target.Weight
		}
	}
	if !codexToolTarget || !codexOpenRouterToolTarget || !claudeMiniMaxToolTarget || !claudeBasetenToolTarget || !claudeKimiToolTarget || !claudeOpenRouterToolTarget || !claudeGemmaToolTarget {
		t.Fatalf("example config group %s missing tool-only targets codex=%v codex_openrouter=%v minimax=%v baseten=%v kimi=%v claude_openrouter=%v claude_gemma=%v", name, codexToolTarget, codexOpenRouterToolTarget, claudeMiniMaxToolTarget, claudeBasetenToolTarget, claudeKimiToolTarget, claudeOpenRouterToolTarget, claudeGemmaToolTarget)
	}
	if name == "big-coder" && !openAIChatCrusoeGemmaToolTarget {
		t.Fatalf("example config group %s missing OpenAI Chat Crusoe Gemma tool-only target", name)
	}
	want := map[string]struct {
		gptOSS, m3, gemma, kimi, openAI, basetenNemotron, basetenGLM, crusoeGemma, crusoeGLM, targets int
	}{
		"default":   {51, 27, 2, 6, 1, 3, 5, 0, 5, 8},
		"fast":      {56, 26, 2, 5, 1, 3, 5, 0, 2, 8},
		"small":     {58, 28, 2, 4, 1, 3, 2, 0, 2, 8},
		"medium":    {51, 25, 2, 8, 1, 3, 5, 0, 5, 8},
		"high":      {45, 26, 2, 10, 1, 3, 6, 0, 7, 8},
		"big-coder": {18, 30, 0, 23, 1, 2, 6, 20, 0, 7},
	}
	expect, ok := want[name]
	if !ok {
		t.Fatalf("example config group %s has no expected weight policy", name)
	}
	if totalWeight != 100 || normalTargets != expect.targets || basetenGPTOSSWeight != expect.gptOSS || m3Weight != expect.m3 || gemmaWeight != expect.gemma || kimiWeight != expect.kimi || openAIWeight != expect.openAI || basetenNemotronWeight != expect.basetenNemotron || basetenGLMWeight != expect.basetenGLM || crusoeGemmaWeight != expect.crusoeGemma || crusoeGLMWeight != expect.crusoeGLM {
		t.Fatalf("example config group %s weights gpt_oss=%d m3=%d gemma=%d kimi=%d openai=%d baseten_nemotron=%d baseten_glm=%d crusoe_gemma=%d crusoe_glm=%d total=%d normal_targets=%d, want %#v", name, basetenGPTOSSWeight, m3Weight, gemmaWeight, kimiWeight, openAIWeight, basetenNemotronWeight, basetenGLMWeight, crusoeGemmaWeight, crusoeGLMWeight, totalWeight, normalTargets, expect)
	}
}

func violatesCurrentRoutingPolicy(target Target) bool {
	needle := strings.ToLower(target.Provider + "/" + target.Model + "/" + target.ModelRef)
	if target.ToolOnly && stringSliceContains(target.InputModalities, "image") && (target.Provider == "openrouter_responses" || target.Provider == "openrouter_anthropic") {
		switch target.ModelRef {
		case "qwen3-7-plus-nitro", "openrouter-claude-sonnet-4-6", "openrouter-xai-grok-4-3", "openrouter-minimax-m3":
			return false
		}
	}
	if strings.Contains(needle, "moonshotai/kimi") {
		return true
	}
	allowedBasetenNemotron := target.Provider == "baseten" && target.Model == "nvidia/Nemotron-120B-A12B"
	allowedBasetenGLM := target.Provider == "baseten" && target.Model == "zai-org/GLM-5.2"
	allowedBasetenGPTOSS := target.Provider == "baseten" && target.Model == "openai/gpt-oss-120b"
	allowedCrusoeGLM := target.Provider == "crusoe" && target.Model == "zai/GLM-5.2"
	for _, bad := range []string{"qwen", "glm", "hy3", "kat-coder", "nemotron", "mercury", "ling-2.6", "pareto", "m2.7-highspeed"} {
		if strings.Contains(needle, bad) {
			return !allowedBasetenNemotron && !allowedBasetenGLM && !allowedBasetenGPTOSS && !allowedCrusoeGLM
		}
	}
	if target.Provider == "openrouter" && strings.Contains(strings.ToLower(target.Model), "deepseek/") {
		return true
	}
	if target.ToolOnly && (target.Provider == "openai" || target.Provider == "anthropic") {
		return true
	}
	if (target.Provider == "openai" || target.Provider == "anthropic") && target.Weight > 1 {
		return true
	}
	return false
}

func TestValidateDynamicScoreRejectsUnsafeBounds(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Models["default"] = ModelGroup{
		Strategy: "dynamic_score",
		RoutingPolicy: RoutingPolicyConfig{DynamicScore: DynamicScoreConfig{
			MaxScoreAdjustmentPercent: 101,
		}},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_score_adjustment_percent") {
		t.Fatalf("Validate() err=%v, want max_score_adjustment_percent error", err)
	}

	cfg = minimalConfig(t)
	cfg.Models["default"] = ModelGroup{
		Strategy: "dynamic_score",
		RoutingPolicy: RoutingPolicyConfig{DynamicScore: DynamicScoreConfig{
			Signals: DynamicScoreSignals{
				PromptFeatures: DynamicSignalPromptFeatures{Enabled: true, MaxScanBytes: 70000},
			},
		}},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_scan_bytes") {
		t.Fatalf("Validate() err=%v, want max_scan_bytes error", err)
	}
}

func TestValidateExternalPolicyHTTPRequiresOptInForNonLocalHost(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Models["default"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        "http://routing-policy.internal.example/route",
			AllowHosts: []string{"routing-policy.internal.example"},
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "allow_http") {
		t.Fatalf("Validate() err=%v, want allow_http error", err)
	}

	cfg.Models["default"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        "http://routing-policy.internal.example/route",
			AllowHosts: []string{"routing-policy.internal.example"},
			AllowHTTP:  true,
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with allow_http=true: %v", err)
	}

	cfg.Models["default"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        "http://127.0.0.1:18090/route",
			AllowHosts: []string{"127.0.0.1"},
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() for trusted-local http: %v", err)
	}
}

func minimalConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Server: ServerConfig{
			Cache: CacheConfig{Enabled: true},
			Logging: LoggingConfig{
				Path: filepath.Join(t.TempDir(), "requests.jsonl"),
			},
		},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Provider: map[string]ProviderConfig{
			"mock": {BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "secret", APIKeyEnv: "MOCK_API_KEY", KeyID: "mock-key"},
		},
		Models: map[string]ModelGroup{
			"default": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			TokenSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Allow:       []string{"default"},
		}},
	}
}

func mustBcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
