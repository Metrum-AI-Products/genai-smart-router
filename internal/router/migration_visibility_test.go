package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminMigrationStatusProjectsBoundDataJobState(t *testing.T) {
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

	jobDefinition := usageDataJobDefinitions[0]
	jobID := dataJobID(usageMigrationScope, jobDefinition.Key)
	cases := []struct {
		name, jobState, effectiveState, validationState, summaryState string
	}{
		{name: "missing", effectiveState: "pending", validationState: "not-validated", summaryState: "pending"},
		{name: "running", jobState: migrationDataJobRunning, effectiveState: "in-progress", validationState: "in-progress", summaryState: "in-progress"},
		{name: "paused", jobState: migrationDataJobPaused, effectiveState: "pending", validationState: "not-validated", summaryState: "pending"},
		{name: "cancelled", jobState: migrationDataJobCancelled, effectiveState: "pending", validationState: "not-validated", summaryState: "pending"},
		{name: "failed", jobState: migrationDataJobFailed, effectiveState: "failed", validationState: "failed", summaryState: "failed"},
		{name: "validated", jobState: migrationDataJobValidated, effectiveState: "applied", validationState: "verified", summaryState: "current"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.usage.db.Where("job_id = ?", jobID).Delete(&migrationDataJobRecord{}).Error; err != nil {
				t.Fatal(err)
			}
			if tc.jobState != "" {
				record := migrationDataJobRecord{JobID: jobID, Scope: usageMigrationScope, MigrationID: jobDefinition.MigrationID, DataVersion: jobDefinition.DataVersion, State: tc.jobState, ExecutionMode: jobDefinition.ExecutionMode, ValidationMode: jobDefinition.ValidationMode, RowsScanned: 20, RowsUpdated: 10, RowsSkipped: 8, RowsFailed: 2, StartedAt: "2026-08-05T12:00:00Z"}
				if tc.jobState == migrationDataJobFailed {
					record.SafeErrorClass = "data-job-failed"
				}
				if err := svc.usage.db.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
			}

			response := adminMigrationStatusResponseForTest(t, svc, "/admin/reports/api/migrations")
			row := adminMigrationRowForTest(t, response.Rows, usageHistoricalValidationMigrationID)
			if row.State != tc.effectiveState || row.ValidationState != tc.validationState || row.DataJobState != tc.name {
				t.Fatalf("row=%+v, want state=%q validation=%q dataJobState=%q", row, tc.effectiveState, tc.validationState, tc.name)
			}
			if got, _ := response.Summary["state"].(string); got != tc.summaryState {
				t.Fatalf("summary=%v, want state=%q", response.Summary, tc.summaryState)
			}
			if tc.name == "missing" && response.Summary["missingDataJobs"] != float64(1) {
				t.Fatalf("missing data-job summary=%v", response.Summary)
			}
			filtered := adminMigrationStatusResponseForTest(t, svc, "/admin/reports/api/migrations?state="+tc.effectiveState)
			if len(filtered.Rows) == 0 || adminMigrationRowForTest(t, filtered.Rows, usageHistoricalValidationMigrationID).DataJobState != tc.name {
				t.Fatalf("effective-state filter did not retain %q data job: %+v", tc.name, filtered.Rows)
			}
		})
	}
}

func adminMigrationStatusResponseForTest(t *testing.T, svc *Service, target string) adminMigrationStatusResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetBasicAuth("admin", "admin-password")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response adminMigrationStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func adminMigrationRowForTest(t *testing.T, rows []adminMigrationStatusRow, migrationID int) adminMigrationStatusRow {
	t.Helper()
	for _, row := range rows {
		if row.MigrationID == migrationID {
			return row
		}
	}
	t.Fatalf("migration %d not found in %+v", migrationID, rows)
	return adminMigrationStatusRow{}
}

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

func TestMigrationMetricsExposeAllSafeDataJobStates(t *testing.T) {
	metrics := newMetricsStore().Prometheus(nil, newTrafficShapeManager(), MigrationStatus{
		Scope: "usage",
		Jobs: []MigrationDataJobStatus{
			{State: migrationDataJobPending}, // includes a missing durable job.
			{State: migrationDataJobRunning},
			{State: migrationDataJobPaused},
			{State: migrationDataJobCancelled},
			{State: migrationDataJobFailed},
			{State: migrationDataJobValidated},
		},
	})
	for _, state := range []string{"pending", "running", "paused", "cancelled", "failed", "validated"} {
		want := `smart_llmrouter_migration_jobs{scope="usage",state="` + state + `"} 1`
		if !strings.Contains(metrics, want) {
			t.Fatalf("migration metrics missing %q: %s", want, metrics)
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
