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
