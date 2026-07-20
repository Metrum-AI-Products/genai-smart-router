package router

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigControlPlanePhase1MigratesRelationalSchema(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	status, err := r.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Compatible || status.State != "current" || status.SchemaVersion != 2 {
		t.Fatalf("unexpected migration status: %+v", status)
	}
	for _, table := range ConfigControlPlaneTableNames() {
		if !r.db.Migrator().HasTable(table) {
			t.Fatalf("missing table %s", table)
		}
	}
	for _, stmt := range configControlPlaneDDL {
		lower := strings.ToLower(stmt)
		for _, forbidden := range []string{" json", "jsonb", "[]", " array"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("control-plane DDL contains forbidden relational type %q: %s", forbidden, stmt)
			}
		}
	}
}

func TestLoadActiveConfigFromDBReadsValidatedCoreProjection(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	db := r.db
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, seed := range []struct {
		sql    string
		values []any
	}{
		{`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "staging", "initial", "active", "valid", now}},
		{`INSERT INTO router_config_server (config_set_id, listen, default_model_group, state_path, license_enabled, license_path, license_state_path) VALUES (?, ?, ?, ?, ?, ?, ?)`, []any{"set-1", ":8080", "default", "router-state.json", true, "license.json", "license-state.json"}},
		{`INSERT INTO router_config_providers (config_set_id, provider_name, base_url, dialect, api_key_env) VALUES (?, ?, ?, ?, ?)`, []any{"set-1", "mock", "https://mock.example/v1", "openai", "MOCK_API_KEY"}},
		{`INSERT INTO router_config_provider_headers (config_set_id, provider_name, header_name, header_value) VALUES (?, ?, ?, ?)`, []any{"set-1", "mock", "X-Title", "test"}},
		{`INSERT INTO router_config_provider_models (config_set_id, provider_name, model_ref, model, context_tokens, input_price_per_million_usd, output_price_per_million_usd) VALUES (?, ?, ?, ?, ?, ?, ?)`, []any{"set-1", "mock", "small", "mock-small", 8192, 0.1, 0.2}},
		{`INSERT INTO router_config_model_groups (config_set_id, group_name, strategy) VALUES (?, ?, ?)`, []any{"set-1", "default", "static"}},
		{`INSERT INTO router_config_model_group_targets (config_set_id, group_name, sequence, provider_name, model_ref, weight) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "default", 1, "mock", "small", 100}},
	} {
		if err := db.Exec(seed.sql, seed.values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadActiveConfigFromDB(db, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DefaultModelGroup != "default" || cfg.Provider["mock"].Headers["X-Title"] != "test" {
		t.Fatalf("server/provider projection mismatch: %+v", cfg)
	}
	if !cfg.Server.License.Enabled || cfg.Server.License.Path != "license.json" || cfg.Server.License.StatePath != "license-state.json" {
		t.Fatalf("license projection mismatch: %+v", cfg.Server.License)
	}
	if target := cfg.Models["default"].Targets[0]; target.Provider != "mock" || target.ModelRef != "small" || target.Weight != 100 {
		t.Fatalf("target projection mismatch: %+v", target)
	}
	if model := cfg.Provider["mock"].Models["small"]; model.Model != "mock-small" || model.ContextTokens != 8192 {
		t.Fatalf("model projection mismatch: %+v", model)
	}
}

func TestLoadActiveConfigFromDBFailsClosedWithoutValidatedActiveSet(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadActiveConfigFromDB(r.db, "staging"); err == nil || !strings.Contains(err.Error(), "no validated active") {
		t.Fatalf("expected closed failure, got %v", err)
	}
}

func TestConfigControlPlaneAllowsOnlyOneActiveSetPerScope(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.db.Exec(`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, "set-1", "staging", "one", "active", "valid", now).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.Exec(`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, "set-2", "staging", "two", "active", "valid", now).Error; err == nil {
		t.Fatal("second active set in one runtime scope must be rejected")
	}
}

func TestConfigControlPlaneEnforcesTargetProviderAndModelForeignKeys(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, seed := range []struct {
		sql    string
		values []any
	}{
		{`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "staging", "one", "draft", "valid", now}},
		{`INSERT INTO router_config_model_groups (config_set_id, group_name) VALUES (?, ?)`, []any{"set-1", "group"}},
	} {
		if err := r.db.Exec(seed.sql, seed.values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := r.db.Exec(`INSERT INTO router_config_model_group_targets (config_set_id, group_name, sequence, provider_name, model_ref) VALUES (?, ?, ?, ?, ?)`, "set-1", "group", 1, "missing", "small").Error; err == nil {
		t.Fatal("target with missing provider/model must be rejected")
	}
}

func TestConfigControlPlaneRejectsCredentialBearingProviderHeadersAtDatabaseBoundary(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, seed := range []struct {
		sql    string
		values []any
	}{
		{`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "staging", "one", "active", "valid", now}},
		{`INSERT INTO router_config_providers (config_set_id, provider_name, base_url, dialect) VALUES (?, ?, ?, ?)`, []any{"set-1", "mock", "https://mock.example/v1", "openai"}},
	} {
		if err := r.db.Exec(seed.sql, seed.values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, headerName := range []string{"Authorization", "Proxy-Authorization", "X-Goog-Api-Key", "Ocp-Apim-Subscription-Key", " X-Title "} {
		if err := r.db.Exec(`INSERT INTO router_config_provider_headers (config_set_id, provider_name, header_name, header_value) VALUES (?, ?, ?, ?)`, "set-1", "mock", headerName, "not-a-real-secret").Error; err == nil {
			t.Fatalf("database accepted disallowed provider header %q", headerName)
		}
	}
	if err := r.db.Exec(`INSERT INTO router_config_provider_headers (config_set_id, provider_name, header_name, header_value) VALUES (?, ?, ?, ?)`, "set-1", "mock", "X-Title", "non-secret-metadata").Error; err != nil {
		t.Fatalf("database rejected approved non-secret header: %v", err)
	}
	if err := r.db.Exec(`UPDATE router_config_provider_headers SET header_name = ? WHERE config_set_id = ? AND provider_name = ? AND header_name = ?`, "Authorization", "set-1", "mock", "X-Title").Error; err == nil {
		t.Fatal("database accepted credential-bearing provider header through update")
	}
}

func TestConfigControlPlanePhase2UpgradesExistingAllowedProviderHeaders(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	phase1Runner, err := NewMigrationRunner(r.db, configControlPlaneScope, MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, configControlPlaneMigrationDefinitions[:1])
	if err != nil {
		t.Fatal(err)
	}
	if err := phase1Runner.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, seed := range []struct {
		sql    string
		values []any
	}{
		{`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "staging", "one", "draft", "valid", now}},
		{`INSERT INTO router_config_providers (config_set_id, provider_name, base_url, dialect) VALUES (?, ?, ?, ?)`, []any{"set-1", "mock", "https://mock.example/v1", "openai"}},
		{`INSERT INTO router_config_provider_headers (config_set_id, provider_name, header_name, header_value) VALUES (?, ?, ?, ?)`, []any{"set-1", "mock", "X-Title", "non-secret-metadata"}},
	} {
		if err := r.db.Exec(seed.sql, seed.values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Verify(); err != nil {
		t.Fatal(err)
	}
	var header providerHeaderRow
	if err := r.db.Where("config_set_id = ? AND provider_name = ? AND header_name = ?", "set-1", "mock", "X-Title").First(&header).Error; err != nil {
		t.Fatalf("approved provider header was not preserved by phase 2: %v", err)
	}
	if header.HeaderValue != "non-secret-metadata" {
		t.Fatalf("provider header value = %q, want preserved non-secret metadata", header.HeaderValue)
	}
	if err := r.db.Exec(`INSERT INTO router_config_provider_headers (config_set_id, provider_name, header_name, header_value) VALUES (?, ?, ?, ?)`, "set-1", "mock", "Authorization", "not-a-real-secret").Error; err == nil {
		t.Fatal("phase 2 migration did not reject credential-bearing provider headers")
	}
}

func TestLoadActiveConfigFromDBAcceptsInlineTargetWithoutModelRef(t *testing.T) {
	r, closeDB, err := ConfigControlPlaneMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "config.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, seed := range []struct {
		sql    string
		values []any
	}{
		{`INSERT INTO router_config_sets (id, runtime_scope, name, status, validation_status, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "staging", "one", "active", "valid", now}},
		{`INSERT INTO router_config_server (config_set_id, default_model_group, license_enabled, license_path) VALUES (?, ?, ?, ?)`, []any{"set-1", "default", true, "license.json"}},
		{`INSERT INTO router_config_providers (config_set_id, provider_name, base_url, dialect) VALUES (?, ?, ?, ?)`, []any{"set-1", "mock", "https://mock.example/v1", "openai"}},
		{`INSERT INTO router_config_model_groups (config_set_id, group_name, strategy) VALUES (?, ?, ?)`, []any{"set-1", "default", "static"}},
		{`INSERT INTO router_config_model_group_targets (config_set_id, group_name, sequence, provider_name, model, weight) VALUES (?, ?, ?, ?, ?, ?)`, []any{"set-1", "default", 1, "mock", "inline-model", 100}},
	} {
		if err := r.db.Exec(seed.sql, seed.values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadActiveConfigFromDB(r.db, "staging")
	if err != nil {
		t.Fatal(err)
	}
	target := cfg.Models["default"].Targets[0]
	if target.ModelRef != "" || target.Model != "inline-model" {
		t.Fatalf("inline target = %+v, want empty model_ref and inline model", target)
	}
}
