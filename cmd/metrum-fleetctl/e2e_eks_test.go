package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"smart-llmrouter/internal/router"
)

func requiresDisposableRDSAdmission(manifest router.TenantDeploymentManifest) bool {
	return manifest.DatabaseProfile != ""
}

// TestDisposableEKSPackagedCLI is intentionally environment-gated. CI can run
// it only with an approved disposable EKS profile embedded in a signed,
// reference-only intent, external scoped RDS admission, and separately
// authorized deletion approval. It exercises the packaged binary and always
// attempts job-bound cleanup after deploy begins, including when deployment
// itself fails.
func TestDisposableEKSPackagedCLI(t *testing.T) {
	intentPath := os.Getenv("EKS_E2E_INTENT")
	rdsAdmissionFile := os.Getenv("EKS_E2E_RDS_ADMISSION_FILE")
	deleteApprovalFile := os.Getenv("EKS_E2E_DELETE_APPROVAL_FILE")
	if intentPath == "" || deleteApprovalFile == "" {
		t.Skip("set EKS_E2E_INTENT and EKS_E2E_DELETE_APPROVAL_FILE for disposable EKS E2E")
	}
	intent, _, err := router.LoadTenantDeploymentIntent(intentPath, time.Now().UTC())
	if err != nil {
		t.Fatalf("load disposable EKS intent: %v", err)
	}
	if requiresDisposableRDSAdmission(intent.Manifest) && rdsAdmissionFile == "" {
		t.Skip("set EKS_E2E_RDS_ADMISSION_FILE for a dedicated-RDS disposable EKS E2E")
	}
	binary := filepath.Join(t.TempDir(), "metrum-fleetctl")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("package CLI: %v: %s", err, output)
	}
	registry := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	run := func(command string, extra ...string) ([]byte, error) {
		args := []string{command, "--intent", intentPath, "--registry", registry, "--output", "json"}
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
