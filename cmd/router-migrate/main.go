// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-migrate is intentionally non-serving. It exposes only safe migration
// metadata until persistent scopes have checked-in conversion definitions.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	action := flag.String("action", "status", "migration action: status, plan, verify, verify-serving, apply, maintenance, non-transactional-maintenance, cancel, retry, or resume")
	driver := flag.String("driver", "sqlite", "usage DB driver: sqlite or postgres")
	dbPath := flag.String("db", "usage.sqlite", "path to usage SQLite database")
	dsnEnv := flag.String("dsn-env", "ROUTER_USAGE_DB_DSN", "environment variable containing the Postgres DSN")
	runnerID := flag.String("runner", "router-migrate", "safe runner identifier for the migration ledger")
	jobKey := flag.String("job", "", "checked-in migration data-job key for cancel, retry, or resume")
	checkpointOrdinal := flag.Int("checkpoint-ordinal", 0, "non-negative scalar checkpoint ordinal for resume")
	recoveryEvidenceRef := flag.String("recovery-evidence-ref", "", "bounded safe recovery evidence reference required for retry")
	backupEvidenceRef := flag.String("backup-evidence-ref", "", "bounded safe backup evidence reference required for maintenance actions")
	jsonOut := flag.Bool("json", false, "emit machine-readable safe migration status")
	flag.Parse()
	dsn := ""
	if *driver == "postgres" {
		dsn = os.Getenv(*dsnEnv)
		if dsn == "" {
			die("Postgres migration scope requires non-empty --dsn-env %q", *dsnEnv)
		}
	}
	r, closeDB, err := router.UsageMigrationRunner(router.UsageDBConfig{Driver: *driver, Path: *dbPath, DSN: dsn})
	if err != nil {
		die("open migration scope: %v", err)
	}
	defer closeDB()
	var status router.MigrationStatus
	switch *action {
	case "status", "plan":
		status, err = r.Status()
	case "verify":
		status, err = r.Verify()
	case "verify-serving":
		status, err = r.Verify()
		if err == nil {
			err = requireServingCompatible(status)
		}
	case "apply":
		err = r.ApplyPending(*runnerID)
		if err == nil {
			status, err = r.Status()
		}
	case "maintenance":
		if *backupEvidenceRef == "" {
			die("migration maintenance requires --backup-evidence-ref")
		}
		err = r.ApplyMaintenancePendingWithEvidence(*runnerID, *backupEvidenceRef)
		if err == nil {
			status, err = r.Status()
		}
	case "non-transactional-maintenance":
		if *backupEvidenceRef == "" {
			die("migration non-transactional-maintenance requires --backup-evidence-ref")
		}
		err = r.ApplyNonTransactionalMaintenancePendingWithEvidence(*runnerID, *backupEvidenceRef)
		if err == nil {
			status, err = r.Status()
		}
	case "cancel":
		if *jobKey == "" {
			die("migration cancel requires --job")
		}
		err = r.RequestDataJobCancellation(*jobKey)
		if err == nil {
			status, err = r.Status()
		}
	case "retry":
		if *jobKey == "" {
			die("migration retry requires --job")
		}
		err = r.RetryDataJob(*jobKey, *recoveryEvidenceRef)
		if err == nil {
			status, err = r.Status()
		}
	case "resume":
		if *jobKey == "" || *checkpointOrdinal < 0 {
			die("migration resume requires --job and non-negative --checkpoint-ordinal")
		}
		_, err = r.RunDataJob(context.Background(), *jobKey, *runnerID, router.DataJobCheckpoint{Ordinal: *checkpointOrdinal})
		if err == nil {
			status, err = r.Status()
		}
	default:
		die("unsupported --action %q", *action)
	}
	if err != nil {
		die("migration %s: %v", *action, err)
	}
	if *jsonOut {
		b, _ := json.Marshal(status)
		fmt.Println(string(b))
		return
	}
	fmt.Printf("scope=%s schema_version=%d data_version=%d compatible=%t state=%s pending=%d entries=%d\n", status.Scope, status.SchemaVersion, status.DataVersion, status.Compatible, status.State, len(status.Pending), len(status.Entries))
}

func die(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(1) }

func requireServingCompatible(status router.MigrationStatus) error {
	if !router.UsageMigrationServingCompatible(status) {
		return fmt.Errorf("migration verify-serving requires current compatible ledger and validated data jobs (state %s)", status.State)
	}
	return nil
}
