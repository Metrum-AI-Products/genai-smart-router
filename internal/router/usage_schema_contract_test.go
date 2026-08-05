package router

import (
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func openUsageSchemaContractDB(t *testing.T) *gorm.DB {
	t.Helper()
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store.db
}

func TestUsageSchemaContractRejectsRequiredColumnAndIndexDrift(t *testing.T) {
	db := openUsageSchemaContractDB(t)
	if err := db.Exec("DROP INDEX idx_request_usage_ts").Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureUsageRelationalSchema(db); err == nil {
		t.Fatal("schema contract accepted a missing required index")
	}

	db = openUsageSchemaContractDB(t)
	if err := db.Exec("DROP INDEX idx_request_usage_ts").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE request_usage DROP COLUMN ts").Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureUsageRelationalSchema(db); err == nil || !strings.Contains(err.Error(), "request_usage.ts") {
		t.Fatal("schema contract accepted a missing required column")
	}
}

func TestUsageSchemaContractRejectsRequiredForeignKeyDrift(t *testing.T) {
	db := openUsageSchemaContractDB(t)
	// SQLite cannot drop a foreign key in place. Replacing this normalized child
	// table without its FK is the portable drift shape the verifier must reject.
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE request_token_estimates_drift AS SELECT * FROM request_token_estimates").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE request_token_estimates").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE request_token_estimates_drift RENAME TO request_token_estimates").Error; err != nil {
		t.Fatal(err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&usageRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageForeignKeys(db, stmt.Schema); err == nil {
		t.Fatal("schema contract accepted a missing required foreign key")
	}
}

func TestUsageSchemaContractRejectsForeignKeyActionDrift(t *testing.T) {
	db := openUsageSchemaContractDB(t)
	var createSQL string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "request_token_estimates").Scan(&createSQL).Error; err != nil {
		t.Fatal(err)
	}
	if createSQL == "" || !strings.Contains(createSQL, "ON DELETE CASCADE") {
		t.Fatalf("unexpected request_token_estimates DDL: %q", createSQL)
	}
	// Rebuild the child using identical columns/references but a weaker delete
	// action. HasConstraint still sees a FK, so this proves the verifier checks
	// the full physical contract rather than only its existence.
	driftSQL := strings.Replace(createSQL, "ON DELETE CASCADE", "ON DELETE RESTRICT", 1)
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE request_token_estimates").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(driftSQL).Error; err != nil {
		t.Fatal(err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&usageRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageForeignKeys(db, stmt.Schema); err == nil || !strings.Contains(err.Error(), "does not match contract") {
		t.Fatalf("schema contract accepted foreign-key action drift: %v", err)
	}
}
