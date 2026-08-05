package router

// This file contains the durable migration primitives shared by persistent
// router scopes.  It deliberately stores only scalar, operator-safe metadata;
// migration definitions themselves remain checked into the binary.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const usageMigrationScope = "usage"

const usageLegacyBaselineMigrationID = 2026071901
const usageReasoningTelemetryMigrationID = 2026072301

var usageMigrationCompatibility = MigrationCompatibility{MinSchema: 0, MaxSchema: 2, MinData: 0, MaxData: 0}

// usageMigrationDefinitions starts the immutable usage manifest by adopting a
// database created by the pre-ledger initializer. It intentionally contains no
// DDL: a fresh database must continue through the legacy initializer until its
// full reviewed replacement is shipped. This migration is therefore a guarded
// one-time ledger adoption, not an implicit upgrade path.
var usageMigrationDefinitions = []MigrationDefinition{{
	ID:               usageLegacyBaselineMigrationID,
	Scope:            usageMigrationScope,
	Name:             "adopt legacy usage schema baseline",
	Release:          "2026.7",
	Checksum:         "1bbefd1d653dd50633bd050badc0dae65e87775cbb88dd27f350604dd46862b6",
	SchemaVersion:    1,
	DataVersion:      0,
	Transactional:    true,
	MaintenanceMode:  "online",
	RollbackClass:    "package-only",
	HandlerKey:       "usage.legacy-baseline.verify.v1@verifyUsageLegacyBaseline",
	PostconditionKey: "usage.legacy-baseline.schema.v1@verifyUsageLegacyBaseline",
	ExecutionMode:    "transactional",
	LockClass:        "online",
	TimeoutClass:     "bounded",
	Apply:            verifyUsageLegacyBaseline,
	Verify:           verifyUsageLegacyBaseline,
}, {
	ID:              usageReasoningTelemetryMigrationID,
	Scope:           usageMigrationScope,
	Name:            "add nullable reasoning-token usage telemetry",
	Release:         "2026.7",
	Checksum:        "e23d1dd0f41292af050f2ee9a6c2bc55a5430d22397c90dd33c145bd350264b8",
	SchemaVersion:   2,
	DataVersion:     0,
	Transactional:   true,
	MaintenanceMode: "online",
	// Older binaries reject this new ledger ID under validate/deployment-job.
	// Downgrade therefore requires restoring the pre-migration database snapshot.
	RollbackClass:    "restore-required",
	HandlerKey:       "usage.reasoning-telemetry.apply.v1@applyUsageReasoningTelemetryMigration",
	PostconditionKey: "usage.reasoning-telemetry.schema.v1@verifyUsageReasoningTelemetryMigration",
	Dependencies:     []int{usageLegacyBaselineMigrationID},
	ExecutionMode:    "transactional",
	LockClass:        "online",
	TimeoutClass:     "bounded",
	Apply:            applyUsageReasoningTelemetryMigration,
	Verify:           verifyUsageReasoningTelemetryMigration,
}}

func init() {
	for i := range usageMigrationDefinitions {
		usageMigrationDefinitions[i] = FinalizeMigrationDefinition(usageMigrationDefinitions[i])
	}
}

// MigrationCompatibility declares the inclusive schema/data versions a binary
// can safely operate with. Versions are monotonically increasing integers.
type MigrationCompatibility struct {
	MinSchema int
	MaxSchema int
	MinData   int
	MaxData   int
}

// MigrationDefinition is immutable once released. Checksum is calculated from
// the reviewed metadata and must be supplied by the migration author.
type MigrationDefinition struct {
	ID              int
	Scope           string
	Name            string
	Release         string
	Checksum        string
	SchemaVersion   int
	DataVersion     int
	Transactional   bool
	RollbackClass   string // package-only, config-only, restore-required, prohibited
	MaintenanceMode string // online, maintenance
	// HandlerKey and PostconditionKey are checked-in stable identities. They
	// make a manifest change observable even when Go function pointers change.
	HandlerKey       string
	PostconditionKey string
	Dependencies     []int
	ExecutionMode    string // transactional, non-transactional
	LockClass        string // online, maintenance
	TimeoutClass     string // bounded, maintenance
	ManifestDigest   string // canonical digest of the immutable metadata
	Apply            func(*gorm.DB) error
	Verify           func(*gorm.DB) error
}

// HandlerKey and PostconditionKey deliberately name the checked-in executable
// functions after an @ separator. This keeps the operator-facing portion of a
// key stable while making a source-level handler swap fail manifest validation.
// Function names are used instead of a program-counter address because PC
// values are build-specific and cannot be durable manifest identities.
func migrationHandlerKeyMatches(key string, fn func(*gorm.DB) error) bool {
	if fn == nil {
		return false
	}
	separator := strings.LastIndex(key, "@")
	if separator <= 0 || separator == len(key)-1 {
		return false
	}
	function := runtime.FuncForPC(reflect.ValueOf(fn).Pointer())
	if function == nil {
		return false
	}
	name := function.Name()
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return key[separator+1:] == name
}

// CanonicalMigrationDigest binds all reviewed, non-executable migration
// metadata. It intentionally omits function pointers and instead includes the
// checked-in HandlerKey/PostconditionKey identities.
func CanonicalMigrationDigest(d MigrationDefinition) string {
	deps := append([]int(nil), d.Dependencies...)
	sort.Ints(deps)
	parts := []string{fmt.Sprint(d.ID), d.Scope, d.Name, d.Release, fmt.Sprint(d.SchemaVersion), fmt.Sprint(d.DataVersion), fmt.Sprint(d.Transactional), d.RollbackClass, d.MaintenanceMode, d.HandlerKey, d.PostconditionKey, d.ExecutionMode, d.LockClass, d.TimeoutClass}
	for _, dep := range deps {
		parts = append(parts, fmt.Sprint(dep))
	}
	return MigrationChecksum(parts...)
}

// FinalizeMigrationDefinition is used only by checked-in manifest literals.
// It prevents a caller from supplying an arbitrary digest.
func FinalizeMigrationDefinition(d MigrationDefinition) MigrationDefinition {
	d.ManifestDigest = CanonicalMigrationDigest(d)
	return d
}

// MigrationLedgerEntry is intentionally safe to return to operators. It never
// includes SQL, DSNs, request data, credentials, or configuration values.
type MigrationLedgerEntry struct {
	Scope       string
	MigrationID int
	Checksum    string
	State       string
	StartedAt   time.Time
	CompletedAt time.Time
	Runner      string
	DurationMS  int64
	ErrorCode   string
	ErrorText   string
}

type migrationLedgerRecord struct {
	Scope          string `gorm:"primaryKey;column:scope;type:text"`
	MigrationID    int    `gorm:"primaryKey;column:migration_id"`
	Checksum       string `gorm:"column:checksum;type:text;not null"`
	ManifestDigest string `gorm:"column:manifest_digest;type:text;not null;default:''"`
	State          string `gorm:"column:state;type:text;not null"`
	StartedAt      string `gorm:"column:started_at;type:text;not null"`
	CompletedAt    string `gorm:"column:completed_at;type:text;not null;default:''"`
	Runner         string `gorm:"column:runner;type:text;not null;default:''"`
	DurationMS     int64  `gorm:"column:duration_ms;not null;default:0"`
	ErrorCode      string `gorm:"column:error_code;type:text;not null;default:''"`
	ErrorText      string `gorm:"column:error_text;type:text;not null;default:''"`
}

// migrationAttemptRecord, migrationDataJobRecord, and
// migrationDataJobCheckpointRecord are the normalized scalar Stage-1 contract
// for future resumable jobs. Stage 1 records no jobs and changes no serving
// behavior; later stages own execution/checkpoint semantics.
type migrationAttemptRecord struct {
	Scope               string `gorm:"primaryKey"`
	MigrationID         int    `gorm:"primaryKey"`
	Attempt             int    `gorm:"primaryKey"`
	Action              string
	State               string
	OwnerGeneration     int64
	BackupEvidenceRef   string
	RecoveryEvidenceRef string
	StartedAt           string
	CompletedAt         string
	SafeErrorClass      string
}

func (migrationAttemptRecord) TableName() string { return "schema_migration_attempts" }

type migrationDataJobRecord struct {
	JobID             string `gorm:"primaryKey"`
	Scope             string
	MigrationID       int
	DataVersion       int
	State             string
	ExecutionMode     string
	ValidationMode    string
	ThrottlePerMinute int
	RowsScanned       int64
	RowsUpdated       int64
	RowsSkipped       int64
	RowsFailed        int64
	StartedAt         string
	CompletedAt       string
	SafeErrorClass    string
}

func (migrationDataJobRecord) TableName() string { return "schema_data_jobs" }

type migrationDataJobCheckpointRecord struct {
	JobID             string `gorm:"primaryKey"`
	CheckpointOrdinal int    `gorm:"primaryKey"`
	Shard             string
	RangeStart        int64
	RangeEnd          int64
	Cursor            string
	RowsScanned       int64
	RowsUpdated       int64
	RowsSkipped       int64
	RowsFailed        int64
	State             string
	StartedAt         string
	CompletedAt       string
}

func (migrationDataJobCheckpointRecord) TableName() string { return "schema_data_job_checkpoints" }

func (migrationLedgerRecord) TableName() string { return "schema_migration_ledger" }

type migrationLockRecord struct {
	Scope     string `gorm:"primaryKey;column:scope;type:text"`
	Runner    string `gorm:"column:runner;type:text;not null"`
	LockedAt  string `gorm:"column:locked_at;type:text;not null"`
	ExpiresAt string `gorm:"column:expires_at;type:text;not null"`
}

func (migrationLockRecord) TableName() string { return "schema_migration_locks" }

// MigrationStatus summarizes one persistent scope without exposing database
// internals. Incompatible includes checksum tampering and future versions.
type MigrationStatus struct {
	Scope         string
	SchemaVersion int
	DataVersion   int
	Compatibility MigrationCompatibility
	Compatible    bool
	State         string // current, pending, in-progress, failed, incompatible
	Pending       []MigrationDefinition
	Entries       []MigrationLedgerEntry
}

type migrationRunner struct {
	db            *gorm.DB
	scope         string
	compatibility MigrationCompatibility
	definitions   []MigrationDefinition
}

// NewMigrationRunner validates a checked-in manifest before it can touch a DB.
func NewMigrationRunner(db *gorm.DB, scope string, compatibility MigrationCompatibility, definitions []MigrationDefinition) (*migrationRunner, error) {
	if db == nil || strings.TrimSpace(scope) == "" {
		return nil, errors.New("migration runner requires database and scope")
	}
	if compatibility.MaxSchema < compatibility.MinSchema || compatibility.MaxData < compatibility.MinData {
		return nil, errors.New("invalid migration compatibility range")
	}
	defs := append([]MigrationDefinition(nil), definitions...)
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	previous := 0
	for _, d := range defs {
		if d.Scope != scope || d.ID <= previous || d.ID <= 0 || !validMigrationChecksum(d.Checksum) ||
			d.HandlerKey == "" || d.PostconditionKey == "" || d.ExecutionMode == "" || d.LockClass == "" || d.TimeoutClass == "" || d.ManifestDigest != CanonicalMigrationDigest(d) ||
			!migrationHandlerKeyMatches(d.HandlerKey, d.Apply) || !migrationHandlerKeyMatches(d.PostconditionKey, d.Verify) {
			return nil, errors.New("invalid immutable migration manifest")
		}
		// Dependencies are an ordered part of the immutable manifest contract:
		// a definition can depend only on a definition already declared in this
		// scope. This prevents a manifest from being valid while describing an
		// unexecutable cycle, forward reference, or duplicate prerequisite.
		seenDependencies := make(map[int]struct{}, len(d.Dependencies))
		for _, dependency := range d.Dependencies {
			if dependency <= 0 || dependency >= d.ID {
				return nil, errors.New("invalid immutable migration manifest dependency")
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return nil, errors.New("invalid immutable migration manifest dependency")
			}
			seenDependencies[dependency] = struct{}{}
			if dependency > previous {
				return nil, errors.New("invalid immutable migration manifest dependency")
			}
		}
		previous = d.ID
	}
	return &migrationRunner{db: db, scope: scope, compatibility: compatibility, definitions: defs}, nil
}

func validMigrationChecksum(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

// MigrationChecksum makes definitions reviewable without serializing function
// bodies. Include this value as a literal in the checked-in manifest.
func MigrationChecksum(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *migrationRunner) ensureLedger() error {
	// Explicit DDL keeps the framework bootstrap independent of AutoMigrate.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS schema_migration_ledger (scope TEXT NOT NULL, migration_id BIGINT NOT NULL, checksum TEXT NOT NULL, manifest_digest TEXT NOT NULL DEFAULT '', state TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', runner TEXT NOT NULL DEFAULT '', duration_ms BIGINT NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '', error_text TEXT NOT NULL DEFAULT '', PRIMARY KEY (scope, migration_id))`,
		`CREATE TABLE IF NOT EXISTS schema_migration_locks (scope TEXT NOT NULL PRIMARY KEY, runner TEXT NOT NULL, locked_at TEXT NOT NULL, expires_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS schema_migration_attempts (scope TEXT NOT NULL, migration_id BIGINT NOT NULL, attempt BIGINT NOT NULL, action TEXT NOT NULL, state TEXT NOT NULL, owner_generation BIGINT NOT NULL DEFAULT 0, backup_evidence_ref TEXT NOT NULL DEFAULT '', recovery_evidence_ref TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', safe_error_class TEXT NOT NULL DEFAULT '', PRIMARY KEY (scope, migration_id, attempt))`,
		`CREATE TABLE IF NOT EXISTS schema_data_jobs (job_id TEXT NOT NULL PRIMARY KEY, scope TEXT NOT NULL, migration_id BIGINT NOT NULL, data_version BIGINT NOT NULL, state TEXT NOT NULL, execution_mode TEXT NOT NULL, validation_mode TEXT NOT NULL, throttle_per_minute BIGINT NOT NULL DEFAULT 0, rows_scanned BIGINT NOT NULL DEFAULT 0, rows_updated BIGINT NOT NULL DEFAULT 0, rows_skipped BIGINT NOT NULL DEFAULT 0, rows_failed BIGINT NOT NULL DEFAULT 0, started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', safe_error_class TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS schema_data_job_checkpoints (job_id TEXT NOT NULL, checkpoint_ordinal BIGINT NOT NULL, shard TEXT NOT NULL DEFAULT '', range_start BIGINT NOT NULL DEFAULT 0, range_end BIGINT NOT NULL DEFAULT 0, cursor TEXT NOT NULL DEFAULT '', rows_scanned BIGINT NOT NULL DEFAULT 0, rows_updated BIGINT NOT NULL DEFAULT 0, rows_skipped BIGINT NOT NULL DEFAULT 0, rows_failed BIGINT NOT NULL DEFAULT 0, state TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT NOT NULL DEFAULT '', PRIMARY KEY (job_id, checkpoint_ordinal))`,
	} {
		if err := r.db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("migration ledger bootstrap: %w", err)
		}
	}
	return nil
}

func (r *migrationRunner) Status() (MigrationStatus, error) {
	if err := r.ensureLedger(); err != nil {
		return MigrationStatus{}, err
	}
	status := MigrationStatus{Scope: r.scope, Compatibility: r.compatibility, Compatible: true, State: "current"}
	var records []migrationLedgerRecord
	if err := r.db.Where("scope = ?", r.scope).Order("migration_id ASC").Find(&records).Error; err != nil {
		return status, err
	}
	known := make(map[int]MigrationDefinition, len(r.definitions))
	for _, d := range r.definitions {
		known[d.ID] = d
	}
	applied := map[int]bool{}
	for _, rec := range records {
		status.Entries = append(status.Entries, ledgerEntry(rec))
		d, ok := known[rec.MigrationID]
		if !ok || d.Checksum != rec.Checksum || rec.ManifestDigest != "" && d.ManifestDigest != rec.ManifestDigest {
			status.Compatible = false
			status.State = "incompatible"
			continue
		}
		switch rec.State {
		case "applied":
			applied[rec.MigrationID] = true
			if d.SchemaVersion > status.SchemaVersion {
				status.SchemaVersion = d.SchemaVersion
			}
			if d.DataVersion > status.DataVersion {
				status.DataVersion = d.DataVersion
			}
		case "running":
			status.State = "in-progress"
		case "failed":
			status.State = "failed"
		default:
			status.Compatible = false
			status.State = "incompatible"
		}
	}
	if status.SchemaVersion < r.compatibility.MinSchema || status.SchemaVersion > r.compatibility.MaxSchema || status.DataVersion < r.compatibility.MinData || status.DataVersion > r.compatibility.MaxData {
		status.Compatible = false
		status.State = "incompatible"
	}
	for _, d := range r.definitions {
		if !applied[d.ID] {
			status.Pending = append(status.Pending, d)
		}
	}
	if status.State == "current" && len(status.Pending) > 0 {
		status.State = "pending"
	}
	return status, nil
}

// Verify rechecks the immutable ledger and postconditions of applied
// definitions. Pending definitions are reported but never executed.
func (r *migrationRunner) Verify() (MigrationStatus, error) {
	status, err := r.Status()
	if err != nil || !status.Compatible {
		return status, err
	}
	applied := make(map[int]bool, len(status.Entries))
	for _, entry := range status.Entries {
		if entry.State == "applied" {
			applied[entry.MigrationID] = true
		}
	}
	for _, d := range r.definitions {
		if applied[d.ID] && d.Verify != nil {
			if err := d.Verify(r.db); err != nil {
				return status, fmt.Errorf("migration %d verification: %w", d.ID, err)
			}
		}
	}
	return status, nil
}

func ledgerEntry(rec migrationLedgerRecord) MigrationLedgerEntry {
	started, _ := time.Parse(time.RFC3339Nano, rec.StartedAt)
	completed, _ := time.Parse(time.RFC3339Nano, rec.CompletedAt)
	return MigrationLedgerEntry{rec.Scope, rec.MigrationID, rec.Checksum, rec.State, started, completed, rec.Runner, rec.DurationMS, rec.ErrorCode, rec.ErrorText}
}

// ApplyPending executes only transactional, online definitions. It is safe for
// a serving-process startup policy because it stops before any definition that
// requires a maintenance window or a non-transactional operation.
func (r *migrationRunner) ApplyPending(runner string) error {
	return r.applyPending(runner, "online")
}

// ApplyMaintenancePending is an explicit non-serving maintenance operation.
// It executes only transactional definitions marked maintenance. Callers must
// first apply the online prefix and schedule the required maintenance window;
// non-transactional definitions (for example, a PostgreSQL concurrent index
// build) still require a dedicated non-transactional runner.
func (r *migrationRunner) ApplyMaintenancePending(runner string) error {
	return r.applyPending(runner, "maintenance")
}

func (r *migrationRunner) applyPending(runner, maintenanceMode string) error {
	status, err := r.Status()
	if err != nil {
		return err
	}
	if !status.Compatible {
		return errors.New("migration state is incompatible")
	}
	if err := r.acquireLock(runner); err != nil {
		return err
	}
	defer r.releaseLock(runner)
	applied := make(map[int]bool, len(status.Entries))
	for _, entry := range status.Entries {
		if entry.State == "applied" {
			applied[entry.MigrationID] = true
		}
	}
	for _, d := range status.Pending {
		if !d.Transactional || d.MaintenanceMode != maintenanceMode {
			if maintenanceMode == "online" {
				return fmt.Errorf("migration %d requires explicit maintenance runner", d.ID)
			}
			return fmt.Errorf("migration %d requires the online runner or a dedicated non-transactional runner", d.ID)
		}
		if d.Apply == nil {
			return fmt.Errorf("migration %d has no apply step", d.ID)
		}
		for _, dependency := range d.Dependencies {
			if !applied[dependency] {
				return fmt.Errorf("migration %d requires applied dependency %d", d.ID, dependency)
			}
		}
		if err := r.applyOne(d, runner); err != nil {
			return err
		}
		applied[d.ID] = true
	}
	return nil
}

// acquireLock is the durable single-runner guard for both SQLite and
// PostgreSQL. A stale lease is reclaimed only after its expiry; operators can
// see the holder and expiry in the relational lock table during recovery.
func (r *migrationRunner) acquireLock(runner string) error {
	now := time.Now().UTC()
	if err := r.db.Where("scope = ? AND expires_at < ?", r.scope, now.Format(time.RFC3339Nano)).Delete(&migrationLockRecord{}).Error; err != nil {
		return errors.New("migration lock cleanup failed")
	}
	lock := migrationLockRecord{Scope: r.scope, Runner: safeMigrationText(runner), LockedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(30 * time.Minute).Format(time.RFC3339Nano)}
	if err := r.db.Create(&lock).Error; err != nil {
		return errors.New("migration runner is already active for this scope")
	}
	return nil
}

func (r *migrationRunner) releaseLock(runner string) {
	_ = r.db.Where("scope = ? AND runner = ?", r.scope, safeMigrationText(runner)).Delete(&migrationLockRecord{}).Error
}

func (r *migrationRunner) applyOne(d MigrationDefinition, runner string) error {
	now := time.Now().UTC()
	started := now.Format(time.RFC3339Nano)
	return r.db.Transaction(func(tx *gorm.DB) error {
		rec := migrationLedgerRecord{Scope: r.scope, MigrationID: d.ID, Checksum: d.Checksum, ManifestDigest: d.ManifestDigest, State: "running", StartedAt: started, Runner: safeMigrationText(runner)}
		if err := tx.Create(&rec).Error; err != nil {
			return errors.New("migration already running or applied")
		}
		if err := d.Apply(tx); err != nil {
			return err
		}
		if d.Verify != nil {
			if err := d.Verify(tx); err != nil {
				return err
			}
		}
		return tx.Model(&migrationLedgerRecord{}).Where("scope = ? AND migration_id = ?", r.scope, d.ID).Updates(map[string]any{"state": "applied", "completed_at": time.Now().UTC().Format(time.RFC3339Nano), "duration_ms": time.Since(now).Milliseconds()}).Error
	})
}

func safeMigrationText(v string) string {
	v = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
			return r
		}
		return -1
	}, v)
	if len(v) > 120 {
		return v[:120]
	}
	return v
}

// UsageMigrationRunner opens the configured persistent usage scope without
// invoking the legacy usage-store initializer, so plan/status are non-serving
// and do not silently alter application tables.
func UsageMigrationRunner(cfg UsageDBConfig) (*migrationRunner, func() error, error) {
	db, err := openUsageDB(cfg)
	if err != nil {
		return nil, nil, err
	}
	r, err := newUsageMigrationRunner(db)
	if err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, nil, err
	}
	closeFn := func() error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return r, closeFn, nil
}

func newUsageMigrationRunner(db *gorm.DB) (*migrationRunner, error) {
	return NewMigrationRunner(db, usageMigrationScope, usageMigrationCompatibility, usageMigrationDefinitions)
}
