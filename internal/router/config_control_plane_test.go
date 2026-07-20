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
	if !status.Compatible || status.State != "current" || status.SchemaVersion != 1 {
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
		{`INSERT INTO router_config_server (config_set_id, listen, default_model_group, state_path) VALUES (?, ?, ?, ?)`, []any{"set-1", ":8080", "default", "router-state.json"}},
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
