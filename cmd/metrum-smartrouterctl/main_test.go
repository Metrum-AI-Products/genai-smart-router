package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if out, err := run("status", "--registry", registry, "--limit", "1"); err != nil || !strings.Contains(out, `"DriftCode": "current"`) || !strings.Contains(out, `"CurrentSchemaVersion": 1`) {
		t.Fatalf("updated schema status was not current: %v %s", err, out)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "missing", "--stage", "test", "--current-schema-version", "1"); err == nil || !strings.Contains(out, "exactly one registered instance") {
		t.Fatalf("missing tenant schema observation did not fail closed: %v %s", err, out)
	}
	if out, err := run("observe-schema", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--current-schema-version", "2147483648"); err == nil || !strings.Contains(out, "between 0 and 2147483647") {
		t.Fatalf("out-of-bounds schema observation did not fail closed: %v %s", err, out)
	}
	if out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", "reserve-a", "--mock-quota-limit", "4"); err != nil || !strings.Contains(out, "admission_reserved") {
		t.Fatalf("fake quota reservation failed: %v %s", err, out)
	}
	if _, err := run("status", "--registry", "file:unsafe?mode=memory", "--limit", "1"); err == nil {
		t.Fatal("SQLite URI was accepted")
	}
}
