package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type usagePostgresDefaultContractFixture struct {
	ID    int    `gorm:"primaryKey"`
	Value string `gorm:"type:text;not null;default:''"`
}

func (usagePostgresDefaultContractFixture) TableName() string {
	return "usage_postgres_default_contract_fixture"
}

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

func TestUsageSchemaContractVersionedReasoningTelemetryVerification(t *testing.T) {
	db := openUsageSchemaContractDB(t)
	for _, statement := range []string{
		"ALTER TABLE request_usage DROP COLUMN reasoning_tokens",
		"ALTER TABLE request_usage DROP COLUMN reasoning_attempt_count",
		"ALTER TABLE request_usage DROP COLUMN reasoning_successful_attempt_count",
		"ALTER TABLE request_usage DROP COLUMN reasoning_reported_attempt_count",
		"ALTER TABLE request_attempts DROP COLUMN reasoning_tokens",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyUsageLegacyBaseline(db); err != nil {
		t.Fatalf("v1 baseline rejected its historical schema: %v", err)
	}
	if err := verifyUsageReasoningTelemetryMigration(db); err == nil || !strings.Contains(err.Error(), "reasoning_tokens") {
		t.Fatalf("v2 reasoning verifier accepted missing reasoning contract: %v", err)
	}
	if err := db.Exec("DROP INDEX idx_request_usage_ts").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE request_usage DROP COLUMN ts").Error; err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageLegacyBaseline(db); err == nil || !strings.Contains(err.Error(), "request_usage.ts") {
		t.Fatalf("v1 baseline accepted unrelated historical-contract drift: %v", err)
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

type usageCheckContractFixture struct {
	ID    int `gorm:"primaryKey"`
	Value int `gorm:"check:chk_usage_fixture_positive,value > 0"`
}

func (usageCheckContractFixture) TableName() string { return "usage_check_contract_fixture" }

func TestUsageSchemaContractRejectsCheckConstraintSemanticDrift(t *testing.T) {
	db := openUsageSchemaContractDB(t)
	if err := db.AutoMigrate(&usageCheckContractFixture{}); err != nil {
		t.Fatal(err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&usageCheckContractFixture{}); err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageCheckConstraints(db, &usageCheckContractFixture{}, stmt.Schema); err != nil {
		t.Fatalf("check fixture contract rejected its schema: %v", err)
	}
	if err := db.Exec("DROP TABLE usage_check_contract_fixture").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE usage_check_contract_fixture (id integer PRIMARY KEY, value integer, CONSTRAINT chk_usage_fixture_positive CHECK (value >= 0))").Error; err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageCheckConstraints(db, &usageCheckContractFixture{}, stmt.Schema); err == nil || !strings.Contains(err.Error(), "does not match contract") {
		t.Fatalf("schema contract accepted check semantic drift: %v", err)
	}
}

func TestUsageDefaultCompatibilityNormalizesOnlyPostgresTextCasts(t *testing.T) {
	tests := []struct {
		name     string
		driver   string
		expected string
		actual   string
		want     bool
	}{
		{name: "postgres text cast", driver: "postgres", expected: "''", actual: "''::text", want: true},
		{name: "postgres parenthesized text cast", driver: "postgres", expected: "''", actual: "(( '' :: pg_catalog.text ))", want: true},
		{name: "postgres character varying cast", driver: "postgresql", expected: "''", actual: "''::character varying", want: true},
		{name: "postgres escaped literal cast", driver: "postgres", expected: "'O''Brien'", actual: "E'O''Brien'::text", want: true},
		{name: "sqlite keeps its literal spelling", driver: "sqlite", expected: "''", actual: "''", want: true},
		{name: "sqlite does not accept postgres cast syntax", driver: "sqlite", expected: "''", actual: "''::text", want: false},
		{name: "different text default", driver: "postgres", expected: "''", actual: "'changed'::text", want: false},
		{name: "non-text cast", driver: "postgres", expected: "''", actual: "''::integer", want: false},
		{name: "concatenated expression", driver: "postgres", expected: "''", actual: "''::text || 'changed'::text", want: false},
		{name: "function expression", driver: "postgres", expected: "''", actual: "coalesce(''::text, 'changed'::text)", want: false},
		{name: "matching function expressions remain rejected", driver: "postgres", expected: "now()", actual: "now()", want: false},
		{name: "sqlite boolean numeric spelling remains compatible", driver: "sqlite", expected: "false", actual: "0", want: true},
		{name: "boolean true does not equal false", driver: "postgres", expected: "false", actual: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usageDefaultCompatible(tt.driver, tt.expected, tt.actual); got != tt.want {
				t.Fatalf("usageDefaultCompatible(%q, %q, %q) = %v, want %v", tt.driver, tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

// TestUsageSchemaContractPostgresTextDefaults verifies the exact metadata path
// against a disposable PostgreSQL database. The unit table above is always run;
// this opt-in test proves the driver's real ColumnTypes DefaultValue spelling.
func TestUsageSchemaContractPostgresTextDefaults(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMART_ROUTER_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("SMART_ROUTER_POSTGRES_TEST_DSN is required for disposable PostgreSQL default coverage")
	}
	if os.Getenv("SMART_ROUTER_POSTGRES_TEST_ALLOW") != "issue-704" || !strings.Contains(strings.ToLower(dsn), "smart_router_issue_704") {
		t.Fatal("PostgreSQL default coverage requires the explicit issue-704 disposable database guard")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&usagePostgresDefaultContractFixture{}); err != nil {
		t.Fatal(err)
	}
	if err := verifyUsageRelationalModel(db, &usagePostgresDefaultContractFixture{}); err != nil {
		t.Fatalf("PostgreSQL text default contract rejected: %v", err)
	}
	columns, err := db.Migrator().ColumnTypes(&usagePostgresDefaultContractFixture{})
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if column.Name() != "value" {
			continue
		}
		actual, known := column.DefaultValue()
		if !known || !strings.Contains(actual, "::") || !usageDefaultCompatible("postgres", "''", actual) {
			t.Fatalf("PostgreSQL text default metadata = %q (known=%v), want compatible cast-bearing literal", actual, known)
		}
		return
	}
	t.Fatal("PostgreSQL fixture value column missing from metadata")
}
