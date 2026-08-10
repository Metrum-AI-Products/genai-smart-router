package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDisposableEKSPackagedCLI is intentionally environment-gated. CI can run
// it only with an approved disposable EKS profile, reference-only manifest,
// external scoped RDS admission, and separately authorized deletion approval.
// It exercises the packaged binary and always attempts job-bound cleanup after
// deploy begins, including when deployment itself fails.
func TestDisposableEKSPackagedCLI(t *testing.T) {
	profile := os.Getenv("EKS_E2E_PROFILE_REF")
	manifest := os.Getenv("EKS_E2E_MANIFEST")
	intent := os.Getenv("EKS_E2E_INTENT_ID")
	rdsAdmissionFile := os.Getenv("EKS_E2E_RDS_ADMISSION_FILE")
	deleteApprovalFile := os.Getenv("EKS_E2E_DELETE_APPROVAL_FILE")
	if profile == "" || manifest == "" || intent == "" || rdsAdmissionFile == "" || deleteApprovalFile == "" {
		t.Skip("set EKS_E2E_PROFILE_REF, EKS_E2E_MANIFEST, EKS_E2E_INTENT_ID, EKS_E2E_RDS_ADMISSION_FILE, and EKS_E2E_DELETE_APPROVAL_FILE for disposable EKS/RDS E2E")
	}
	binary := filepath.Join(t.TempDir(), "metrum-fleetctl")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("package CLI: %v: %s", err, output)
	}
	registry := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	run := func(command string, extra ...string) ([]byte, error) {
		args := []string{command, "--profile-ref", profile, "--manifest", manifest, "--intent-id", intent, "--registry", registry, "--output", "json"}
		args = append(args, extra...)
		return exec.Command(binary, args...).CombinedOutput()
	}
	if output, err := run("plan"); err != nil {
		t.Fatalf("packaged CLI plan: %v: %s", err, output)
	}
	t.Cleanup(func() {
		output, err := run("delete", "--confirm-file", deleteApprovalFile, "--rds-admission-file", rdsAdmissionFile)
		if err != nil {
			t.Errorf("packaged CLI cleanup: %v: %s", err, output)
		}
	})
	if output, err := run("deploy", "--rds-admission-file", rdsAdmissionFile); err != nil {
		t.Fatalf("packaged CLI deploy: %v: %s", err, output)
	}
}
