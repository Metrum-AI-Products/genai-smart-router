package router

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestUsageExplicitBootstrapPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMART_ROUTER_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("SMART_ROUTER_POSTGRES_TEST_DSN is required for disposable PostgreSQL Stage 2 coverage")
	}
	if os.Getenv("SMART_ROUTER_POSTGRES_TEST_ALLOW") != "issue-507-stage2" || !strings.Contains(strings.ToLower(dsn), "smart_router_issue_507_stage2") {
		t.Fatal("PostgreSQL Stage 2 coverage requires the explicit disposable database guard")
	}
	store, err := OpenUsageStore(UsageDBConfig{Driver: "postgres", DSN: dsn, MigrationPolicy: usageDBMigrationPolicyAutoSafe})
	if err != nil {
		t.Fatalf("explicit PostgreSQL bootstrap: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	validated, err := OpenUsageStore(UsageDBConfig{Driver: "postgres", DSN: dsn, MigrationPolicy: usageDBMigrationPolicyDeploymentJob})
	if err != nil {
		t.Fatalf("PostgreSQL deployment-job validation: %v", err)
	}
	defer validated.Close()
	if err := ensureUsageRelationalSchema(validated.db); err != nil {
		t.Fatalf("PostgreSQL explicit schema contract: %v", err)
	}
}

func applyTestMigrationMarker(tx *gorm.DB) error {
	return tx.Exec("CREATE TABLE IF NOT EXISTS migration_marker (id INTEGER PRIMARY KEY)").Error
}

func verifyTestMigrationMarker(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("migration_marker") {
		return errors.New("marker missing")
	}
	return nil
}

func testMigrationDefinition(id int, scope, name, checksum string, schemaVersion int, maintenanceMode, rollbackClass string) MigrationDefinition {
	return FinalizeMigrationDefinition(MigrationDefinition{
		ID: id, Scope: scope, Name: name, Release: "test", Checksum: checksum,
		SchemaVersion: schemaVersion, Transactional: true, MaintenanceMode: maintenanceMode, RollbackClass: rollbackClass,
		HandlerKey: "test.marker.apply.v1@applyTestMigrationMarker", PostconditionKey: "test.marker.schema.v1@verifyTestMigrationMarker",
		ExecutionMode: "transactional", LockClass: maintenanceMode, TimeoutClass: "bounded",
		Apply: applyTestMigrationMarker, Verify: verifyTestMigrationMarker,
	})
}

func TestMigrationRunnerAppliesAndVerifiesImmutableLedger(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	d := testMigrationDefinition(1, "test", "add marker", MigrationChecksum("test", "1", "add marker"), 1, "online", "package-only")
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, []MigrationDefinition{d})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "current" || !status.Compatible || status.SchemaVersion != 1 || len(status.Entries) != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
}

func TestMigrationRunnerUpgradesLegacyLedgerBeforeStatusAndApply(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "legacy-ledger.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	// This is the exact pre-manifest-digest ledger shape. In particular, it
	// already has an applied row, proving Status upgrades the table before it
	// selects into the current ledger record and Apply preserves legacy rows.
	if err := db.Exec(`CREATE TABLE schema_migration_ledger (scope TEXT NOT NULL, migration_id BIGINT NOT NULL, checksum TEXT NOT NULL, state TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', runner TEXT NOT NULL DEFAULT '', duration_ms BIGINT NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '', error_text TEXT NOT NULL DEFAULT '', PRIMARY KEY (scope, migration_id))`).Error; err != nil {
		t.Fatal(err)
	}
	first := testMigrationDefinition(1, "test", "legacy baseline", MigrationChecksum("legacy baseline"), 1, "online", "package-only")
	second := testMigrationDefinition(2, "test", "new marker", MigrationChecksum("new marker"), 2, "online", "package-only")
	second.Dependencies = []int{first.ID}
	second = FinalizeMigrationDefinition(second)
	if err := db.Exec("INSERT INTO schema_migration_ledger (scope, migration_id, checksum, state, started_at) VALUES (?, ?, ?, ?, ?)", "test", first.ID, first.Checksum, "applied", "2026-07-01T00:00:00Z").Error; err != nil {
		t.Fatal(err)
	}
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 2, MinData: 0, MaxData: 0}, []MigrationDefinition{first, second})
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Status()
	if err != nil {
		t.Fatalf("status must upgrade a legacy ledger before querying it: %v", err)
	}
	if status.SchemaVersion != 1 || len(status.Pending) != 1 || status.Pending[0].ID != second.ID {
		t.Fatalf("legacy ledger status = %+v", status)
	}
	if !db.Migrator().HasColumn(&migrationLedgerRecord{}, "manifest_digest") {
		t.Fatal("legacy ledger upgrade did not add manifest_digest")
	}
	if err := r.ApplyPending("legacy-upgrade-test"); err != nil {
		t.Fatalf("apply after legacy ledger upgrade: %v", err)
	}
	var records []migrationLedgerRecord
	if err := db.Where("scope = ?", "test").Order("migration_id ASC").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ManifestDigest != first.ManifestDigest || records[1].ManifestDigest != second.ManifestDigest {
		t.Fatalf("legacy/new manifest digest values = %+v", records)
	}
}

func TestMigrationRunnerFailsClosedWhenLegacyAppliedLedgerCannotBindManifestDigest(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "legacy-unbound.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	if err := db.Exec(`CREATE TABLE schema_migration_ledger (scope TEXT NOT NULL, migration_id BIGINT NOT NULL, checksum TEXT NOT NULL, state TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', runner TEXT NOT NULL DEFAULT '', duration_ms BIGINT NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '', error_text TEXT NOT NULL DEFAULT '', PRIMARY KEY (scope, migration_id))`).Error; err != nil {
		t.Fatal(err)
	}
	d := testMigrationDefinition(1, "test", "baseline", MigrationChecksum("expected"), 1, "online", "package-only")
	if err := db.Exec("INSERT INTO schema_migration_ledger (scope, migration_id, checksum, state, started_at) VALUES (?, ?, ?, ?, ?)", "test", d.ID, MigrationChecksum("tampered"), "applied", "2026-07-01T00:00:00Z").Error; err != nil {
		t.Fatal(err)
	}
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, []MigrationDefinition{d})
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("unbound legacy applied ledger must fail closed: %+v", status)
	}
	var rec migrationLedgerRecord
	if err := db.Where("scope = ? AND migration_id = ?", "test", d.ID).First(&rec).Error; err != nil {
		t.Fatal(err)
	}
	if rec.ManifestDigest != "" {
		t.Fatalf("unbound legacy row must not be assigned a manifest digest: %+v", rec)
	}
}

func TestMigrationRunnerStatusRejectsAppliedMigrationWithMissingOrUnappliedDependency(t *testing.T) {
	for _, dependencyState := range []string{"missing", "failed"} {
		t.Run(dependencyState, func(t *testing.T) {
			db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "dependency-ledger.sqlite")})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
			first := testMigrationDefinition(1, "test", "first", MigrationChecksum("first"), 1, "online", "package-only")
			second := testMigrationDefinition(2, "test", "second", MigrationChecksum("second"), 2, "online", "package-only")
			second.Dependencies = []int{first.ID}
			second = FinalizeMigrationDefinition(second)
			r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 2, MinData: 0, MaxData: 0}, []MigrationDefinition{first, second})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.ensureLedger(); err != nil {
				t.Fatal(err)
			}
			if dependencyState == "failed" {
				if err := db.Create(&migrationLedgerRecord{Scope: "test", MigrationID: first.ID, Checksum: first.Checksum, ManifestDigest: first.ManifestDigest, State: "failed", StartedAt: "2026-07-01T00:00:00Z"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&migrationLedgerRecord{Scope: "test", MigrationID: second.ID, Checksum: second.Checksum, ManifestDigest: second.ManifestDigest, State: "applied", StartedAt: "2026-07-01T00:00:00Z"}).Error; err != nil {
				t.Fatal(err)
			}
			status, err := r.Status()
			if err != nil {
				t.Fatal(err)
			}
			if status.Compatible || status.State != "incompatible" {
				t.Fatalf("applied migration with %s prerequisite must fail closed: %+v", dependencyState, status)
			}
		})
	}
}

func TestMigrationRunnerRequiresExplicitMaintenanceOperation(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	online := testMigrationDefinition(1, "test", "online marker", MigrationChecksum("online marker"), 1, "online", "package-only")
	maintenance := testMigrationDefinition(2, "test", "maintenance marker", MigrationChecksum("maintenance marker"), 2, "maintenance", "restore-required")
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 2, MinData: 0, MaxData: 0}, []MigrationDefinition{online, maintenance})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyPending("test-online"); err == nil {
		t.Fatal("online runner must stop before a maintenance migration")
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 1 || status.State != "pending" || len(status.Pending) != 1 || status.Pending[0].ID != maintenance.ID {
		t.Fatalf("online runner status = %+v, want only maintenance migration pending", status)
	}
	if err := r.ApplyMaintenancePending("test-maintenance"); err != nil {
		t.Fatalf("explicit maintenance operation: %v", err)
	}
	status, err = r.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 2 || status.State != "current" || !status.Compatible {
		t.Fatalf("maintenance operation status = %+v", status)
	}
}

func TestMigrationRunnerFailsClosedForChangedChecksumAndFutureMigration(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	d := testMigrationDefinition(1, "test", "marker", MigrationChecksum("one"), 1, "online", "package-only")
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, []MigrationDefinition{d})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyPending("runner"); err != nil {
		t.Fatal(err)
	}
	changed := d
	changed.Checksum = MigrationChecksum("changed")
	changedRunner, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, []MigrationDefinition{changed})
	if err != nil {
		t.Fatal(err)
	}
	status, err := changedRunner.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("checksum mismatch must fail closed: %+v", status)
	}
	if err := db.Create(&migrationLedgerRecord{Scope: "test", MigrationID: 99, Checksum: MigrationChecksum("future"), State: "applied", StartedAt: "2026-01-01T00:00:00Z"}).Error; err != nil {
		t.Fatal(err)
	}
	status, err = r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("future migration must fail closed: %+v", status)
	}
}

func TestStrictManifestRejectsMetadataAndHandlerTampering(t *testing.T) {
	d := testMigrationDefinition(1, "usage", "strict marker", MigrationChecksum("strict"), 1, "online", "package-only")
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{d}); err != nil {
		t.Fatalf("strict manifest rejected: %v", err)
	}
	tampered := d
	tampered.HandlerKey = "usage.other.v1"
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{tampered}); err == nil {
		t.Fatal("handler-key tampering must fail closed")
	}
	tampered = d
	tampered.Name = "changed semantics"
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{tampered}); err == nil {
		t.Fatal("canonical metadata tampering must fail closed")
	}
	tampered = d
	tampered.Apply = verifyTestMigrationMarker
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{tampered}); err == nil {
		t.Fatal("apply handler pointer swap must fail closed")
	}
	tampered = d
	tampered.Verify = applyTestMigrationMarker
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{tampered}); err == nil {
		t.Fatal("verify handler pointer swap must fail closed")
	}
	tampered = d
	tampered.ManifestDigest = ""
	if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{tampered}); err == nil {
		t.Fatal("missing manifest digest must fail closed")
	}
}

func TestStrictManifestRejectsUnknownForwardAndDuplicateDependencies(t *testing.T) {
	// IDs need not be consecutive. The absent lower ID proves validation uses
	// the complete declared ID set rather than only numeric ordering.
	first := testMigrationDefinition(2, "usage", "first", MigrationChecksum("first"), 1, "online", "package-only")
	second := testMigrationDefinition(3, "usage", "second", MigrationChecksum("second"), 2, "online", "package-only")
	for name, dependencies := range map[string][]int{
		"unknown":          {99},
		"unknown_prior_id": {1},
		"forward":          {3},
		"duplicate":        {2, 2},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := second
			candidate.Dependencies = dependencies
			candidate = FinalizeMigrationDefinition(candidate)
			if _, err := NewMigrationRunner(&gorm.DB{}, "usage", MigrationCompatibility{}, []MigrationDefinition{first, candidate}); err == nil {
				t.Fatalf("manifest accepted %s dependency contract", name)
			}
		})
	}
}

func TestMigrationRunnerRequiresAppliedDependencies(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "dependencies.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	first := testMigrationDefinition(1, "test", "first", MigrationChecksum("first"), 1, "maintenance", "restore-required")
	second := testMigrationDefinition(2, "test", "second", MigrationChecksum("second"), 2, "online", "package-only")
	second.Dependencies = []int{first.ID}
	second = FinalizeMigrationDefinition(second)
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{MinSchema: 0, MaxSchema: 2, MinData: 0, MaxData: 0}, []MigrationDefinition{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyMaintenancePending("maintenance"); err == nil {
		t.Fatal("maintenance runner must not skip its pending online dependency chain")
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 1 || len(status.Pending) != 1 || status.Pending[0].ID != second.ID {
		t.Fatalf("maintenance run must leave only the dependency-satisfied online migration pending: %+v", status)
	}
	if err := r.ApplyPending("online"); err != nil {
		t.Fatalf("online runner must apply only after its dependency: %v", err)
	}
}

func TestMigrationFrameworkBootstrapsNormalizedScalarContract(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "contract.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ensureLedger(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"schema_migration_ledger", "schema_migration_attempts", "schema_data_jobs", "schema_data_job_checkpoints"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing normalized framework table %s", table)
		}
	}
	if !db.Migrator().HasColumn(&migrationLedgerRecord{}, "manifest_digest") {
		t.Fatal("ledger must bind manifest digest")
	}
	if err := db.Exec("INSERT INTO schema_migration_attempts (scope, migration_id, attempt, action, state, started_at) VALUES ('missing', 1, 1, 'apply', 'failed', 'now')").Error; err == nil {
		t.Fatal("migration attempts must be bound to an immutable ledger row")
	}
	if err := db.Exec("INSERT INTO schema_data_jobs (job_id, scope, migration_id, data_version, state, execution_mode, validation_mode, started_at) VALUES ('missing', 'missing', 1, 1, 'pending', 'batch', 'verify', 'now')").Error; err == nil {
		t.Fatal("data jobs must be bound to an immutable ledger row")
	}
}

func TestMigrationRunnerRejectsAnActiveScopeLease(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	r, err := NewMigrationRunner(db, "test", MigrationCompatibility{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ensureLedger(); err != nil {
		t.Fatal(err)
	}
	if err := r.acquireLock("first"); err != nil {
		t.Fatal(err)
	}
	defer r.releaseLock("first")
	if err := r.acquireLock("second"); err == nil {
		t.Fatal("second runner acquired active scope lease")
	}
}

func TestUsageMigrationAdoptsVerifiedLegacyBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	store, err := OpenUsageStorePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	r, closeDB, err := UsageMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Compatible || status.State != "current" || len(status.Pending) != 0 {
		t.Fatalf("explicit local bootstrap should reach the current ledger: %+v", status)
	}
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	status, err = r.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Compatible || status.State != "current" || status.SchemaVersion != usageMigrationCompatibility.MaxSchema || status.DataVersion != 0 || len(status.Entries) != len(usageMigrationDefinitions) {
		t.Fatalf("unexpected fully migrated usage status: %+v", status)
	}
}

func TestUsageMigrationBaselineBootstrapsEmptyDatabaseExplicitly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sqlite")
	r, closeDB, err := UsageMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatalf("explicit baseline must initialize an empty database: %v", err)
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Compatible || status.State != "current" || status.SchemaVersion != usageMigrationCompatibility.MaxSchema || len(status.Entries) != len(usageMigrationDefinitions) {
		t.Fatalf("explicit baseline bootstrap status: %+v", status)
	}
}

func TestUsageMigrationBaselineKeepsReasoningColumnsForTheirOwnDefinition(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "baseline-version.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	baseline, err := NewMigrationRunner(db, usageMigrationScope, MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, usageMigrationDefinitions[:1])
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.ApplyPending("baseline-version-test"); err != nil {
		t.Fatal(err)
	}
	for table, columns := range usageReasoningTelemetryColumns {
		for column := range columns {
			if db.Migrator().HasColumn(table, column) {
				t.Fatalf("baseline unexpectedly created v2 column %s.%s", table, column)
			}
		}
	}
	current, err := newUsageMigrationRunner(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.ApplyPending("baseline-version-test"); err != nil {
		t.Fatal(err)
	}
	for table, columns := range usageReasoningTelemetryColumns {
		for column := range columns {
			if !db.Migrator().HasColumn(table, column) {
				t.Fatalf("reasoning migration did not create v2 column %s.%s", table, column)
			}
		}
	}
}

func TestUsageStoreStartupMigrationPoliciesFailClosedAndAutoAdopt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	legacy, err := OpenUsageStorePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	// A reviewed small/single-node deployment may explicitly select auto-safe.
	auto, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: path, MigrationPolicy: usageDBMigrationPolicyAutoSafe})
	if err != nil {
		t.Fatalf("auto-safe adoption: %v", err)
	}
	if err := auto.Close(); err != nil {
		t.Fatal(err)
	}

	for _, policy := range []string{usageDBMigrationPolicyValidate, usageDBMigrationPolicyDeploymentJob} {
		store, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: path, MigrationPolicy: policy})
		if err != nil {
			t.Fatalf("%s must accept current verified ledger: %v", policy, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}

	empty := filepath.Join(t.TempDir(), "empty.sqlite")
	for _, policy := range []string{usageDBMigrationPolicyValidate, usageDBMigrationPolicyDeploymentJob} {
		if _, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: empty, MigrationPolicy: policy}); err == nil {
			t.Fatalf("%s must reject an empty database before the deployment job runs", policy)
		}
	}
	bootstrapped, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: empty, MigrationPolicy: usageDBMigrationPolicyAutoSafe})
	if err != nil {
		t.Fatalf("auto-safe explicit bootstrap: %v", err)
	}
	if err := bootstrapped.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUsageReasoningTelemetryMigrationAddsColumnsToAdoptedSchema(t *testing.T) {
	var definition *MigrationDefinition
	for i := range usageMigrationDefinitions {
		if usageMigrationDefinitions[i].ID == usageReasoningTelemetryMigrationID {
			definition = &usageMigrationDefinitions[i]
			break
		}
	}
	if definition == nil || definition.RollbackClass != "restore-required" {
		t.Fatalf("reasoning migration must require database restore for a binary downgrade: %#v", definition)
	}
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	legacy, err := OpenUsageStorePath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE request_usage DROP COLUMN reasoning_tokens",
		"ALTER TABLE request_usage DROP COLUMN reasoning_attempt_count",
		"ALTER TABLE request_usage DROP COLUMN reasoning_successful_attempt_count",
		"ALTER TABLE request_usage DROP COLUMN reasoning_reported_attempt_count",
		"ALTER TABLE request_attempts DROP COLUMN reasoning_tokens",
	} {
		if err := legacy.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Recreate the pre-ledger historical shape. The explicit baseline then
	// adopts it and the next immutable definition owns the missing columns.
	if err := legacy.db.Exec("DELETE FROM schema_migration_ledger WHERE scope = ?", usageMigrationScope).Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: path, MigrationPolicy: usageDBMigrationPolicyAutoSafe})
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if err := verifyUsageReasoningTelemetryMigration(migrated.db); err != nil {
		t.Fatalf("reasoning migration postcondition: %v", err)
	}
	runner, err := newUsageMigrationRunner(migrated.db)
	if err != nil {
		t.Fatal(err)
	}
	status, err := runner.Verify()
	if err != nil || status.SchemaVersion != 2 || status.State != "current" {
		t.Fatalf("reasoning migration ledger status=%+v err=%v", status, err)
	}
	previousBinary, err := NewMigrationRunner(migrated.db, usageMigrationScope, MigrationCompatibility{MinSchema: 0, MaxSchema: 1, MinData: 0, MaxData: 0}, usageMigrationDefinitions[:1])
	if err != nil {
		t.Fatal(err)
	}
	previousStatus, err := previousBinary.Status()
	if err != nil || previousStatus.Compatible || previousStatus.State != "incompatible" {
		t.Fatalf("previous binary must reject the newer ledger and require restore before downgrade: status=%+v err=%v", previousStatus, err)
	}
}
