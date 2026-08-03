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
	register := []string{"register", "--registry", registry, "--tenant", "tenant-a", "--instance", "router-a", "--stage", "test", "--region", "test-region-1", "--namespace", "ns-router-a", "--release", "release-router-a", "--runtime-identity", "identity-router-a", "--rds-instance", "rds-router-a", "--release-digest", "sha256:test", "--schema-version", "1"}
	if out, err := run(register...); err != nil || !strings.Contains(out, "registered safe") {
		t.Fatalf("register failed: %v %s", err, out)
	}
	if out, err := run("status", "--registry", registry, "--limit", "1"); err != nil || !strings.Contains(out, "tenant-a") {
		t.Fatalf("bounded status failed: %v %s", err, out)
	}
	for _, unsafeReservationID := range []string{
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
	if out, err := run("quota-reserve", "--registry", registry, "--tenant", "tenant-a", "--stage", "test", "--reservation", "rsv-00000000-0000-0000-0000-000000000001", "--mock-quota-limit", "4"); err != nil || !strings.Contains(out, "admission_reserved") {
		t.Fatalf("fake quota reservation failed: %v %s", err, out)
	}
	if _, err := run("status", "--registry", "file:unsafe?mode=memory", "--limit", "1"); err == nil {
		t.Fatal("SQLite URI was accepted")
	}
}
