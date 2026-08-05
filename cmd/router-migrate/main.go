// router-migrate is intentionally non-serving. It exposes only safe migration
// metadata until persistent scopes have checked-in conversion definitions.
package main

import (
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
	action := flag.String("action", "status", "migration action: status, plan, verify, or apply")
	driver := flag.String("driver", "sqlite", "usage DB driver: sqlite or postgres")
	dbPath := flag.String("db", "usage.sqlite", "path to usage SQLite database")
	dsnEnv := flag.String("dsn-env", "ROUTER_USAGE_DB_DSN", "environment variable containing the Postgres DSN")
	runnerID := flag.String("runner", "router-migrate", "safe runner identifier for the migration ledger")
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
	case "apply":
		err = r.ApplyPending(*runnerID)
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
