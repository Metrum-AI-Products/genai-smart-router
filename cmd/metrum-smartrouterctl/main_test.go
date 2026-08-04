package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestLifecycleCommandsFailClosed(t *testing.T) {
	for _, command := range []string{"deploy", "promote", "rollback"} {
		cmd := exec.Command("go", "run", ".", command)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), command+" is disabled") || !strings.Contains(string(out), "DNS gates remain open") {
			t.Fatalf("%s did not fail closed: err=%v output=%s", command, err, out)
		}
	}
}

func TestSafeCLIRegistryWorkflowAndReadOnlyStatus(t *testing.T) {
	registry := filepath.Join(t.TempDir(), "registry.sqlite")
	run := func(args ...string) (string, error) {
		out, err := exec.Command("go", append([]string{"run", "."}, args...)...).CombinedOutput()
		return string(out), err
	}
	if out, err := run("status", "--registry", registry, "--limit", "1"); err == nil || !strings.Contains(out, "open read-only tenant registry") {
		t.Fatalf("missing status did not fail closed: %v %s", err, out)
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatalf("status created registry: %v", err)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "missing", "--stage", "test", "--current-schema-version", "1"); err == nil || !strings.Contains(out, "open existing tenant registry") {
		t.Fatalf("missing observation registry did not fail closed: %v %s", err, out)
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatalf("observation created missing registry: %v", err)
	}
	register := []string{"register", "--registry", registry, "--tenant", "tenant-a", "--instance", "router-a", "--stage", "test", "--region", "test-region-1", "--namespace", "ns-router-a", "--release", "release-router-a", "--runtime-identity", "identity-router-a", "--rds-instance", "rds-router-a", "--release-digest", "sha256:test", "--schema-version", "1"}
	if out, err := run(register...); err == nil || !strings.Contains(out, "current schema version") {
		t.Fatalf("register inferred current schema version: %v %s", err, out)
	}
	register = append(register, "--current-schema-version", "0")
	if out, err := run(register...); err != nil || !strings.Contains(out, "registered safe") {
		t.Fatalf("register failed: %v %s", err, out)
	}
	if out, err := run("status", "--registry", registry, "--limit", "1"); err != nil || !strings.Contains(out, "schema_version_mismatch") || !strings.Contains(out, `"CurrentSchemaVersion": 0`) {
		t.Fatalf("independently observed schema drift was not reported: %v %s", err, out)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--current-schema-version", "1"); err != nil || !strings.Contains(out, `"DriftCode": "current"`) {
		t.Fatalf("schema observation failed: %v %s", err, out)
	}
	if out, err := run(register...); err != nil || !strings.Contains(out, "registered safe") {
		t.Fatalf("unchanged registration after schema observation was not idempotent: %v %s", err, out)
	}
	if out, err := run("status", "--registry", registry, "--limit", "1"); err != nil || !strings.Contains(out, `"DriftCode": "current"`) || !strings.Contains(out, `"CurrentSchemaVersion": 1`) {
		t.Fatalf("updated schema status was not current: %v %s", err, out)
	}
	if out, err := run(append(append([]string{}, register...), "--release-digest", "sha256:changed")...); err == nil || !strings.Contains(out, "different instance contract") {
		t.Fatalf("immutable registration conflict was accepted after observation: %v %s", err, out)
	}
	if out, err := run("status", "--registry", registry, "--limit", "1"); err != nil || !strings.Contains(out, `"CurrentSchemaVersion": 1`) {
		t.Fatalf("rejected immutable registration conflict rewrote schema observation: %v %s", err, out)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "missing", "--stage", "test", "--current-schema-version", "1"); err == nil || !strings.Contains(out, "exactly one registered instance") {
		t.Fatalf("missing tenant schema observation did not fail closed: %v %s", err, out)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--current-schema-version", "2147483648"); err == nil || !strings.Contains(out, "between 0 and 2147483647") {
		t.Fatalf("out-of-bounds schema observation did not fail closed: %v %s", err, out)
	}
	for _, unsafeReservationID := range []string{
		"reserve-a",
		"ghp_exampleCredentialValue",
		"sk-exampleProviderSecret",
		"router-token-example",
		"https://example.test/reservation",
		"/private/reservations/example",
	} {
		out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", unsafeReservationID, "--mock-quota-limit", "4")
		if err == nil || !strings.Contains(out, "invalid reservation id") {
			t.Fatalf("unsafe quota reservation ID did not fail closed: %v %s", err, out)
		}
		if strings.Contains(out, unsafeReservationID) {
			t.Fatalf("unsafe quota reservation ID was echoed: %s", out)
		}
	}
	seed, err := sql.Open("sqlite", registry)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Close()
	if _, err := seed.Exec(`INSERT INTO tenant_instance_quota_reservations (reservation_id, instance_id, region, db_instances, quota_limit, quota_used, quota_reserved, headroom, state, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, "reserve-a", "router-a", "test-region-1", 1, 4, 0, 0, 0, "admission_reserved"); err != nil {
		t.Fatal(err)
	}
	if out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", "reserve-a", "--mock-quota-limit", "4"); err != nil || !strings.Contains(out, "admission_reserved") {
		t.Fatalf("exact legacy reservation retry failed: %v %s", err, out)
	}
	if out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", "rsv-00000000-0000-0000-0000-000000000001", "--mock-quota-limit", "4"); err == nil || !strings.Contains(out, "already has an active quota admission reservation") {
		t.Fatalf("canonical identifier bypassed active legacy hold: %v %s", err, out)
	}
	if _, err := seed.Exec(`INSERT INTO tenant_instance_quota_reservations (reservation_id, instance_id, region, db_instances, quota_limit, quota_used, quota_reserved, headroom, state, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, "sk-exampleProviderSecret", "router-a", "test-region-1", 1, 4, 0, 0, 0, "admission_reserved"); err != nil {
		t.Fatal(err)
	}
	if out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", "sk-exampleProviderSecret", "--mock-quota-limit", "4"); err == nil || !strings.Contains(out, "invalid reservation id") || strings.Contains(out, "sk-exampleProviderSecret") {
		t.Fatalf("unsafe existing reservation row was accepted or echoed: %v %s", err, out)
	}
	if _, err := run("status", "--registry", "file:unsafe?mode=memory", "--limit", "1"); err == nil {
		t.Fatal("SQLite URI was accepted")
	}
}
