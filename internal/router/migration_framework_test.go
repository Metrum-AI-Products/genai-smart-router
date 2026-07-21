package router

import (
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func TestMigrationRunnerAppliesAndVerifiesImmutableLedger(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	d := MigrationDefinition{ID: 1, Scope: "test", Name: "add marker", Release: "test", Checksum: MigrationChecksum("test", "1", "add marker"), SchemaVersion: 1, Transactional: true, MaintenanceMode: "online", RollbackClass: "package-only", Apply: func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE migration_marker (id INTEGER PRIMARY KEY)").Error
	}, Verify: func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("migration_marker") {
			return errors.New("marker missing")
		}
		return nil
	}}
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

func TestMigrationRunnerRequiresExplicitMaintenanceOperation(t *testing.T) {
	db, err := openUsageDB(UsageDBConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "migrations.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	online := MigrationDefinition{ID: 1, Scope: "test", Name: "online marker", Release: "test", Checksum: MigrationChecksum("online marker"), SchemaVersion: 1, Transactional: true, MaintenanceMode: "online", RollbackClass: "package-only", Apply: func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE online_marker (id INTEGER PRIMARY KEY)").Error
	}}
	maintenance := MigrationDefinition{ID: 2, Scope: "test", Name: "maintenance marker", Release: "test", Checksum: MigrationChecksum("maintenance marker"), SchemaVersion: 2, Transactional: true, MaintenanceMode: "maintenance", RollbackClass: "restore-required", Apply: func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE maintenance_marker (id INTEGER PRIMARY KEY)").Error
	}}
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
	d := MigrationDefinition{ID: 1, Scope: "test", Name: "marker", Release: "test", Checksum: MigrationChecksum("one"), SchemaVersion: 1, Transactional: true, MaintenanceMode: "online", RollbackClass: "package-only", Apply: func(*gorm.DB) error { return nil }}
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
	if !status.Compatible || status.State != "pending" || len(status.Pending) != 1 {
		t.Fatalf("legacy usage database should be eligible for baseline adoption: %+v", status)
	}
	if err := r.ApplyPending("test-runner"); err != nil {
		t.Fatal(err)
	}
	status, err = r.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Compatible || status.State != "current" || status.SchemaVersion != 1 || status.DataVersion != 0 || len(status.Entries) != 1 {
		t.Fatalf("unexpected adopted usage status: %+v", status)
	}
}

func TestUsageMigrationBaselineRejectsEmptyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sqlite")
	r, closeDB, err := UsageMigrationRunner(UsageDBConfig{Driver: "sqlite", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeDB() }()
	if err := r.ApplyPending("test-runner"); err == nil {
		t.Fatal("baseline adoption must not initialize an empty database")
	}
	status, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 0 || len(status.Entries) != 0 || len(status.Pending) != 1 {
		t.Fatalf("failed baseline adoption must leave no ledger entry: %+v", status)
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

	// A deployment job (or explicitly enabled safe startup migration) may adopt
	// this existing schema. It must not run the legacy AutoMigrate path.
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
	for _, policy := range []string{usageDBMigrationPolicyValidate, usageDBMigrationPolicyDeploymentJob, usageDBMigrationPolicyAutoSafe} {
		if _, err := OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: empty, MigrationPolicy: policy}); err == nil {
			t.Fatalf("%s must reject an empty database without an explicit bootstrap manifest", policy)
		}
	}
}
