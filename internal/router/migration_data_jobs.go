// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

// Resumable migration data jobs are intentionally small, scalar, and owned by
// router-migrate. They do not run during request handling. A checked-in,
// zero-row-only job may opt into the narrow SQLite auto-safe startup exception;
// that path runs exactly ordinal zero and still requires validation before the
// usage store opens.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// usageDataJobDefinitions is the representative shipped historical conversion.
// It walks existing usage rows in bounded request-ID order and proves each
// row remains addressable by the explicit ledger; it deliberately does not
// rewrite costs or nullable reasoning facts.
var usageDataJobDefinitions = []DataJobDefinition{{
	Key: "historical-usage-validation-v1", Scope: usageMigrationScope,
	MigrationID: usageHistoricalValidationMigrationID, DataVersion: 1,
	ExecutionMode: "batch", ValidationMode: "row-addressability", ThrottlePerMinute: 60,
	RestartSafe: true, AutoSafeZeroRow: true, HandlerKey: "usage.historical-validation.job.v1",
	RunCheckpoint: runUsageHistoricalValidationCheckpoint,
	Validate:      func(db *gorm.DB) error { return verifyUsageReasoningTelemetryMigration(db) },
}}

func applyUsageHistoricalValidationMigration(*gorm.DB) error { return nil }

func runUsageHistoricalValidationCheckpoint(_ context.Context, db *gorm.DB, cp DataJobCheckpoint) (DataJobCheckpointResult, error) {
	var ids []string
	query := db.Table("request_usage").Select("request_id").Order("request_id ASC").Limit(100)
	if cp.Cursor != "" {
		query = query.Where("request_id > ?", cp.Cursor)
	}
	if err := query.Pluck("request_id", &ids).Error; err != nil {
		return DataJobCheckpointResult{}, err
	}
	if len(ids) == 0 {
		return DataJobCheckpointResult{Cursor: cp.Cursor, Complete: true}, nil
	}
	return DataJobCheckpointResult{Cursor: ids[len(ids)-1], RowsScanned: int64(len(ids)), RowsSkipped: int64(len(ids)), Complete: len(ids) < 100}, nil
}

const (
	migrationDataJobPending   = "pending"
	migrationDataJobRunning   = "running"
	migrationDataJobPaused    = "paused"
	migrationDataJobCancelled = "cancelled"
	migrationDataJobFailed    = "failed"
	migrationDataJobValidated = "validated"
)

// DataJobDefinition is immutable checked-in metadata for a bounded,
// checkpointed conversion. Cursor and shard values are opaque bounded scalar
// identifiers; callers must never place request content or credentials in them.
type DataJobDefinition struct {
	Key            string
	Scope          string
	MigrationID    int
	DataVersion    int
	ExecutionMode  string
	ValidationMode string
	// AutoSafeZeroRow permits only the fresh SQLite auto-safe bootstrap to run
	// ordinal zero for this job. It must validate without scanning data; larger
	// historical work remains an explicit non-serving deployment job.
	AutoSafeZeroRow   bool
	ThrottlePerMinute int
	RestartSafe       bool
	HandlerKey        string
	RunCheckpoint     func(context.Context, *gorm.DB, DataJobCheckpoint) (DataJobCheckpointResult, error)
	Validate          func(*gorm.DB) error
}

type DataJobCheckpoint struct {
	Ordinal    int
	Shard      string
	RangeStart int64
	RangeEnd   int64
	Cursor     string
}

type DataJobCheckpointResult struct {
	Cursor      string
	RowsScanned int64
	RowsUpdated int64
	RowsSkipped int64
	RowsFailed  int64
	Complete    bool
}

// MigrationDataJobStatus is safe operator-visible aggregate progress.
type MigrationDataJobStatus struct {
	Key         string
	MigrationID int
	DataVersion int
	// Present distinguishes a durable job record from the synthesized pending
	// status used when an applied schema has not yet created its bound job.
	Present     bool
	State       string
	RowsScanned int64
	RowsUpdated int64
	RowsSkipped int64
	RowsFailed  int64
	Checkpoints int
	ErrorClass  string
}

func validateDataJobDefinitions(scope string, migrations []MigrationDefinition, definitions []DataJobDefinition) ([]DataJobDefinition, error) {
	byID := make(map[int]MigrationDefinition, len(migrations))
	for _, d := range migrations {
		byID[d.ID] = d
	}
	seen := map[string]bool{}
	jobs := append([]DataJobDefinition(nil), definitions...)
	for _, j := range jobs {
		if j.Key == "" || j.Scope != scope || seen[j.Key] || j.MigrationID <= 0 || j.DataVersion <= 0 || j.ExecutionMode == "" || j.ValidationMode == "" || j.HandlerKey == "" || j.RunCheckpoint == nil || j.Validate == nil || !j.RestartSafe {
			return nil, errors.New("invalid immutable migration data-job definition")
		}
		migration, ok := byID[j.MigrationID]
		if !ok || migration.DataJobKey != j.Key || migration.DataVersion != j.DataVersion {
			return nil, errors.New("migration data job is not bound to its immutable migration")
		}
		seen[j.Key] = true
	}
	for _, migration := range migrations {
		if migration.DataJobKey != "" && !seen[migration.DataJobKey] {
			return nil, errors.New("migration data job has no checked-in definition")
		}
	}
	sort.Slice(jobs, func(i, k int) bool { return jobs[i].DataVersion < jobs[k].DataVersion })
	return jobs, nil
}

func dataJobID(scope, key string) string { return safeMigrationText(scope + "-" + key) }

// RunAutoSafeZeroRowDataJob completes the deliberately narrow startup
// exception for a fresh SQLite store. A validated job returns before preflight,
// so usage written after bootstrap cannot alter its checkpoint or counters.
// Every durable non-validated state remains explicit deployment-job work.
func (r *migrationRunner) RunAutoSafeZeroRowDataJob(ctx context.Context, runner string, preflight func(*gorm.DB) error) error {
	status, err := r.Verify()
	if err != nil {
		return err
	}
	if !status.Compatible {
		return errors.New("migration state is incompatible")
	}
	if status.State == "current" {
		for _, job := range status.Jobs {
			if job.State != migrationDataJobValidated {
				return errors.New("auto-safe requires non-serving data-job completion")
			}
		}
		return nil
	}
	if status.State != "pending" || len(status.Jobs) != 1 {
		return errors.New("auto-safe requires one fresh pending data job")
	}
	job := status.Jobs[0]
	if job.Present || job.State != migrationDataJobPending {
		return fmt.Errorf("auto-safe requires non-serving completion of data job %s", safeMigrationText(job.Key))
	}
	var definition *DataJobDefinition
	for i := range r.dataJobs {
		if r.dataJobs[i].Key == job.Key {
			definition = &r.dataJobs[i]
			break
		}
	}
	if definition == nil || !definition.AutoSafeZeroRow {
		return fmt.Errorf("data job %s is not eligible for auto-safe startup", safeMigrationText(job.Key))
	}
	if preflight == nil {
		return errors.New("auto-safe zero-row preflight is required")
	}
	if err := preflight(r.db); err != nil {
		return err
	}
	completed, err := r.runDataJob(ctx, job.Key, runner, DataJobCheckpoint{Ordinal: 0}, requireAutoSafeZeroRowCompletion)
	if err != nil {
		return err
	}
	if completed.State != migrationDataJobValidated || completed.Checkpoints != 1 ||
		completed.RowsScanned != 0 || completed.RowsUpdated != 0 ||
		completed.RowsSkipped != 0 || completed.RowsFailed != 0 {
		return fmt.Errorf("auto-safe zero-row data job %s did not validate empty data", safeMigrationText(job.Key))
	}
	return nil
}

// requireAutoSafeZeroRowCompletion runs inside the same SQLite write
// transaction that inserts the checkpoint and marks the job validated. The
// checkpoint insert acquires the write reservation before this guard reads
// request_usage, so a later writer cannot commit ahead of validation.
func requireAutoSafeZeroRowCompletion(tx *gorm.DB, result DataJobCheckpointResult) error {
	if result.RowsScanned != 0 || result.RowsUpdated != 0 || result.RowsSkipped != 0 || result.RowsFailed != 0 {
		return errors.New("auto-safe checkpoint reported non-zero counters")
	}
	return requireEmptyUsageStore(tx)
}

func (r *migrationRunner) populateDataJobStatus(status *MigrationStatus, applied map[int]bool) error {
	for _, d := range r.dataJobs {
		if !applied[d.MigrationID] {
			continue
		}
		var rec migrationDataJobRecord
		err := r.db.Where("job_id = ?", dataJobID(r.scope, d.Key)).First(&rec).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status.Jobs = append(status.Jobs, MigrationDataJobStatus{Key: d.Key, MigrationID: d.MigrationID, DataVersion: d.DataVersion, State: migrationDataJobPending})
			if status.State == "current" {
				status.State = "pending"
			}
			continue
		}
		if err != nil {
			return err
		}
		var checkpoints int64
		if err := r.db.Model(&migrationDataJobCheckpointRecord{}).Where("job_id = ?", rec.JobID).Count(&checkpoints).Error; err != nil {
			return err
		}
		status.Jobs = append(status.Jobs, MigrationDataJobStatus{Key: d.Key, MigrationID: d.MigrationID, DataVersion: d.DataVersion, Present: true, State: rec.State, RowsScanned: rec.RowsScanned, RowsUpdated: rec.RowsUpdated, RowsSkipped: rec.RowsSkipped, RowsFailed: rec.RowsFailed, Checkpoints: int(checkpoints), ErrorClass: rec.SafeErrorClass})
		switch rec.State {
		case migrationDataJobValidated:
			if d.DataVersion > status.DataVersion {
				status.DataVersion = d.DataVersion
			}
		case migrationDataJobRunning:
			if status.State != "failed" {
				status.State = "in-progress"
			}
		case migrationDataJobFailed:
			status.State = "failed"
		case migrationDataJobPending, migrationDataJobPaused, migrationDataJobCancelled:
			if status.State == "current" {
				status.State = "pending"
			}
		default:
			status.Compatible = false
			status.State = "incompatible"
		}
	}
	if status.DataVersion < r.compatibility.MinData || status.DataVersion > r.compatibility.MaxData {
		status.Compatible = false
		status.State = "incompatible"
	}
	return nil
}

type dataJobCompletionGuard func(*gorm.DB, DataJobCheckpointResult) error

// RunDataJob runs at most one checkpoint. Operators invoke it repeatedly from
// the non-serving CLI, making cancellation and recovery observable between
// batches and avoiding unbounded memory or hot-path work.
func (r *migrationRunner) RunDataJob(ctx context.Context, key, runner string, checkpoint DataJobCheckpoint) (MigrationDataJobStatus, error) {
	return r.runDataJob(ctx, key, runner, checkpoint, nil)
}

func (r *migrationRunner) runDataJob(ctx context.Context, key, runner string, checkpoint DataJobCheckpoint, completionGuard dataJobCompletionGuard) (MigrationDataJobStatus, error) {
	var d *DataJobDefinition
	for i := range r.dataJobs {
		if r.dataJobs[i].Key == key {
			d = &r.dataJobs[i]
			break
		}
	}
	if d == nil {
		return MigrationDataJobStatus{}, errors.New("unknown migration data job")
	}
	if checkpoint.Ordinal < 0 || len(checkpoint.Cursor) > 120 || len(checkpoint.Shard) > 120 {
		return MigrationDataJobStatus{}, errors.New("invalid migration data-job checkpoint")
	}
	status, err := r.Status()
	if err != nil {
		return MigrationDataJobStatus{}, err
	}
	if !status.Compatible {
		return MigrationDataJobStatus{}, errors.New("migration state is incompatible")
	}
	applied := false
	for _, e := range status.Entries {
		if e.MigrationID == d.MigrationID && e.State == "applied" {
			applied = true
		}
	}
	if !applied {
		return MigrationDataJobStatus{}, errors.New("migration data job requires applied schema migration")
	}
	var result MigrationDataJobStatus
	if err := r.withMigrationOwnership(runner, func(owned *migrationRunner) error {
		var runErr error
		result, runErr = owned.runDataJobCheckpoint(ctx, *d, checkpoint, completionGuard)
		return runErr
	}); err != nil {
		return MigrationDataJobStatus{}, err
	}
	return result, nil
}

// runDataJobCheckpoint executes all checkpoint reads, handler work, durable
// mutations, and error/cancellation state changes through an owned runner.
// Its caller holds migration ownership for the whole operation. In PostgreSQL
// that means the advisory lock and every operation below use one physical
// session; in SQLite it preserves the existing single-connection behavior.
// completionGuard is invocation-scoped and runs only for a completing
// checkpoint, inside the transaction that persists validation.
func (r *migrationRunner) runDataJobCheckpoint(ctx context.Context, d DataJobDefinition, checkpoint DataJobCheckpoint, completionGuard dataJobCompletionGuard) (MigrationDataJobStatus, error) {
	id := dataJobID(r.scope, d.Key)
	// An ordinal identifies one durable accounting event.  Check this before
	// cursor restoration, throttling, or handler execution so an operator can
	// safely retry after an uncertain result without overwriting the scalar
	// checkpoint or adding its counters a second time.
	var existing migrationDataJobCheckpointRecord
	err := r.db.Where("job_id = ? AND checkpoint_ordinal = ?", id, checkpoint.Ordinal).First(&existing).Error
	if err == nil {
		var existingJob migrationDataJobRecord
		if err := r.db.Where("job_id = ?", id).First(&existingJob).Error; err != nil {
			return MigrationDataJobStatus{}, err
		}
		return r.dataJobStatus(existingJob)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return MigrationDataJobStatus{}, err
	}
	if checkpoint.Ordinal > 0 && checkpoint.Cursor == "" && checkpoint.Shard == "" && checkpoint.RangeStart == 0 && checkpoint.RangeEnd == 0 {
		var previous migrationDataJobCheckpointRecord
		if err := r.db.Where("job_id = ? AND checkpoint_ordinal = ?", id, checkpoint.Ordinal-1).First(&previous).Error; err != nil {
			return MigrationDataJobStatus{}, errors.New("migration data job checkpoint does not have a resumable predecessor")
		}
		checkpoint.Cursor, checkpoint.Shard, checkpoint.RangeStart, checkpoint.RangeEnd = previous.Cursor, previous.Shard, previous.RangeStart, previous.RangeEnd
	}
	if d.ThrottlePerMinute > 0 {
		var previous migrationDataJobCheckpointRecord
		if err := r.db.Where("job_id = ?", id).Order("checkpoint_ordinal DESC").First(&previous).Error; err == nil {
			if started, parseErr := time.Parse(time.RFC3339Nano, previous.StartedAt); parseErr == nil && time.Since(started) < time.Minute/time.Duration(d.ThrottlePerMinute) {
				return MigrationDataJobStatus{}, errors.New("migration data job throttle active")
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var rec migrationDataJobRecord
	err = r.db.Where("job_id = ?", id).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rec = migrationDataJobRecord{JobID: id, Scope: r.scope, MigrationID: d.MigrationID, DataVersion: d.DataVersion, State: migrationDataJobRunning, ExecutionMode: d.ExecutionMode, ValidationMode: d.ValidationMode, ThrottlePerMinute: d.ThrottlePerMinute, StartedAt: now}
		if err = r.db.Create(&rec).Error; err != nil {
			return MigrationDataJobStatus{}, err
		}
	} else if err != nil {
		return MigrationDataJobStatus{}, err
	} else if rec.State == migrationDataJobValidated {
		return r.dataJobStatus(rec)
	} else if rec.State == migrationDataJobCancelled || rec.State == migrationDataJobFailed {
		return MigrationDataJobStatus{}, errors.New("migration data job requires explicit retry")
	} else if rec.CancelRequestedAt != "" {
		return r.pauseDataJob(rec, migrationDataJobCancelled, "cancelled")
	}
	if err := ctx.Err(); err != nil {
		return r.pauseDataJob(rec, migrationDataJobPaused, "cancelled")
	}
	result, err := d.RunCheckpoint(ctx, r.db, checkpoint)
	if err != nil {
		return r.failDataJob(rec, "checkpoint-failed")
	}
	if len(result.Cursor) > 120 {
		return r.failDataJob(rec, "invalid-cursor")
	}
	cp := migrationDataJobCheckpointRecord{JobID: id, CheckpointOrdinal: checkpoint.Ordinal, Shard: safeMigrationText(checkpoint.Shard), RangeStart: checkpoint.RangeStart, RangeEnd: checkpoint.RangeEnd, Cursor: safeMigrationText(result.Cursor), RowsScanned: result.RowsScanned, RowsUpdated: result.RowsUpdated, RowsSkipped: result.RowsSkipped, RowsFailed: result.RowsFailed, State: migrationDataJobRunning, StartedAt: now}
	if result.Complete {
		cp.State = migrationDataJobValidated
		cp.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		// Insert only: the composite primary key is the PostgreSQL- and
		// SQLite-safe final guard if a stale or failed lease permits a racing
		// contender. Aggregate counters change only with a new checkpoint row.
		if err := tx.Create(&cp).Error; err != nil {
			return err
		}
		updates := map[string]any{"state": migrationDataJobRunning, "rows_scanned": rec.RowsScanned + result.RowsScanned, "rows_updated": rec.RowsUpdated + result.RowsUpdated, "rows_skipped": rec.RowsSkipped + result.RowsSkipped, "rows_failed": rec.RowsFailed + result.RowsFailed}
		if result.Complete {
			if completionGuard != nil {
				if err := completionGuard(tx, result); err != nil {
					return err
				}
			}
			if err := d.Validate(tx); err != nil {
				return err
			}
			updates["state"] = migrationDataJobValidated
			updates["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		return tx.Model(&migrationDataJobRecord{}).Where("job_id = ?", id).Updates(updates).Error
	}); err != nil {
		// A normal duplicate returns above while the scope lease is held. A
		// database-level uniqueness race happens only after the handler has run,
		// so it must fail safely rather than claim execution idempotency for an
		// arbitrary handler with side effects. The next resume observes the
		// durable checkpoint and returns its recorded result.
		var durable migrationDataJobCheckpointRecord
		if lookupErr := r.db.Where("job_id = ? AND checkpoint_ordinal = ?", id, checkpoint.Ordinal).First(&durable).Error; lookupErr == nil {
			return MigrationDataJobStatus{}, errors.New("migration data job checkpoint raced with a durable checkpoint; retry resume")
		}
		return r.failDataJob(rec, "checkpoint-persist-failed")
	}
	if err := r.db.Where("job_id = ?", id).First(&rec).Error; err != nil {
		return MigrationDataJobStatus{}, err
	}
	return r.dataJobStatus(rec)
}

func (r *migrationRunner) RequestDataJobCancellation(key string) error {
	id := dataJobID(r.scope, key)
	return r.db.Model(&migrationDataJobRecord{}).Where("job_id = ? AND state IN (?, ?, ?)", id, migrationDataJobPending, migrationDataJobRunning, migrationDataJobPaused).Update("cancel_requested_at", time.Now().UTC().Format(time.RFC3339Nano)).Error
}

func (r *migrationRunner) RetryDataJob(key, recoveryEvidenceRef string) error {
	if len(recoveryEvidenceRef) == 0 || len(recoveryEvidenceRef) > 120 {
		return errors.New("migration data-job retry requires bounded recovery evidence reference")
	}
	id := dataJobID(r.scope, key)
	return r.db.Model(&migrationDataJobRecord{}).Where("job_id = ? AND state IN (?, ?)", id, migrationDataJobFailed, migrationDataJobCancelled).Updates(map[string]any{"state": migrationDataJobPending, "safe_error_class": "", "cancel_requested_at": ""}).Error
}

func (r *migrationRunner) pauseDataJob(rec migrationDataJobRecord, state, code string) (MigrationDataJobStatus, error) {
	if err := r.db.Model(&migrationDataJobRecord{}).Where("job_id = ?", rec.JobID).Updates(map[string]any{"state": state, "safe_error_class": code, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)}).Error; err != nil {
		return MigrationDataJobStatus{}, err
	}
	rec.State = state
	rec.SafeErrorClass = code
	return r.dataJobStatus(rec)
}
func (r *migrationRunner) failDataJob(rec migrationDataJobRecord, code string) (MigrationDataJobStatus, error) {
	status, err := r.pauseDataJob(rec, migrationDataJobFailed, code)
	if err != nil {
		return status, err
	}
	return status, fmt.Errorf("migration data job %s failed", code)
}
func (r *migrationRunner) dataJobStatus(rec migrationDataJobRecord) (MigrationDataJobStatus, error) {
	var n int64
	if err := r.db.Model(&migrationDataJobCheckpointRecord{}).Where("job_id = ?", rec.JobID).Count(&n).Error; err != nil {
		return MigrationDataJobStatus{}, err
	}
	return MigrationDataJobStatus{Key: strings.TrimPrefix(rec.JobID, safeMigrationText(r.scope+"-")), MigrationID: rec.MigrationID, DataVersion: rec.DataVersion, Present: true, State: rec.State, RowsScanned: rec.RowsScanned, RowsUpdated: rec.RowsUpdated, RowsSkipped: rec.RowsSkipped, RowsFailed: rec.RowsFailed, Checkpoints: int(n), ErrorClass: rec.SafeErrorClass}, nil
}
