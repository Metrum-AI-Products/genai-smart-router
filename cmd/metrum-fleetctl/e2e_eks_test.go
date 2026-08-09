package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDisposableEKSPackagedCLI is intentionally environment-gated. CI can run
// it only with an approved disposable EKS profile, reference-only manifest, and
// external scoped RDS admission; it exercises the packaged binary, never go run.
func TestDisposableEKSPackagedCLI(t *testing.T) {
	profile := os.Getenv("EKS_E2E_PROFILE_REF")
	manifest := os.Getenv("EKS_E2E_MANIFEST")
	intent := os.Getenv("EKS_E2E_INTENT_ID")
	rdsAdmissionFile := os.Getenv("EKS_E2E_RDS_ADMISSION_FILE")
	if profile == "" || manifest == "" || intent == "" || rdsAdmissionFile == "" {
		t.Skip("set EKS_E2E_PROFILE_REF, EKS_E2E_MANIFEST, EKS_E2E_INTENT_ID, and EKS_E2E_RDS_ADMISSION_FILE for disposable EKS/RDS E2E")
	}
	binary := filepath.Join(t.TempDir(), "metrum-fleetctl")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("package CLI: %v: %s", err, output)
	}
	registry := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	for _, command := range []string{"plan", "deploy"} {
		args := []string{command, "--profile-ref", profile, "--manifest", manifest, "--intent-id", intent, "--registry", registry, "--output", "json"}
		if command == "deploy" {
			args = append(args, "--rds-admission-file", rdsAdmissionFile)
		}
		cmd := exec.Command(binary, args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("packaged CLI %s: %v: %s", command, err, output)
		}
	}
}
