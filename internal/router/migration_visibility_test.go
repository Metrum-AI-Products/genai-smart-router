package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationMetricsAreAggregateAndSafe(t *testing.T) {
	metrics := newMetricsStore().Prometheus(nil, newTrafficShapeManager(), MigrationStatus{
		Scope: "usage", SchemaVersion: 2, DataVersion: 1, Compatible: true,
		Pending: []MigrationDefinition{{ID: 999}},
		Jobs:    []MigrationDataJobStatus{{State: "running"}, {State: "running"}},
	})
	for _, want := range []string{
		`smart_llmrouter_migration_schema_version{scope="usage"} 2`,
		`smart_llmrouter_migration_data_version{scope="usage"} 1`,
		`smart_llmrouter_migration_compatible{scope="usage"} 1`,
		`smart_llmrouter_migration_pending{scope="usage"} 1`,
		`smart_llmrouter_migration_jobs{scope="usage",state="running"} 2`,
		`smart_llmrouter_migration_failures{scope="usage"} 0`,
		`smart_llmrouter_migration_progress_rows{scope="usage",outcome="scanned"} 0`,
		`smart_llmrouter_migration_in_progress_age_seconds{scope="usage"} 0`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("migration metrics missing %q: %s", want, metrics)
		}
	}
	for _, forbidden := range []string{"caller_id", "token_id", "postgres://", "SELECT ", "secret"} {
		if strings.Contains(metrics, forbidden) {
			t.Fatalf("migration metrics leaked %q: %s", forbidden, metrics)
		}
	}
}

func TestAdminMigrationStatusIsReadOnlyAndRedacted(t *testing.T) {
	hash := mustBcryptHash(t, "admin-password")
	t.Setenv("SMART_ROUTER_MIGRATION_ADMIN_HASH", hash)
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.UsageDB = UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "usage.sqlite"), MigrationPolicy: usageDBMigrationPolicyAutoSafe}
	cfg.Server.AdminReports = AdminReportsConfig{Enabled: true}
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{Enabled: true, AllowInsecureHTTP: true, Users: []AdminBasicAuthUser{{Username: "admin", PasswordHashEnv: "SMART_ROUTER_MIGRATION_ADMIN_HASH", Subject: "basic:admin", Domain: "test/local"}}}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, basic:admin, test/local, admin:reports, read"}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	ordinary := httptest.NewRequest(http.MethodGet, "/admin/reports/api/migrations", nil)
	ordinary.Header.Set("Authorization", "Bearer "+testToken)
	ordinaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ordinaryRR, ordinary)
	if ordinaryRR.Code != http.StatusForbidden || !strings.Contains(ordinaryRR.Body.String(), "reports-forbidden") {
		t.Fatalf("ordinary status=%d body=%s", ordinaryRR.Code, ordinaryRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/reports/api/migrations", nil)
	req.SetBasicAuth("admin", "admin-password")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{"schemaVersion", "rollbackClass", "restore-required"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("missing %q: %s", want, rr.Body.String())
		}
	}
	for _, forbidden := range []string{"postgres://", "provider-key", testToken, "checksum", "handlerKey", "SELECT "} {
		if strings.Contains(rr.Body.String(), forbidden) {
			t.Fatalf("leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}
