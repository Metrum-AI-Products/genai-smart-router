package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDisposableEKSPackagedCLI is intentionally environment-gated. CI can run
// it only with an independently approved disposable EKS profile and a
// reference-only manifest; it exercises the packaged binary, never go run.
func TestDisposableEKSPackagedCLI(t *testing.T) {
	profile := os.Getenv("EKS_E2E_PROFILE_REF")
	manifest := os.Getenv("EKS_E2E_MANIFEST")
	intent := os.Getenv("EKS_E2E_INTENT_ID")
	if profile == "" || manifest == "" || intent == "" {
		t.Skip("set EKS_E2E_PROFILE_REF, EKS_E2E_MANIFEST, and EKS_E2E_INTENT_ID for disposable EKS E2E")
	}
	binary := filepath.Join(t.TempDir(), "metrum-fleetctl")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("package CLI: %v: %s", err, output)
	}
	registry := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	for _, command := range []string{"plan", "deploy"} {
		cmd := exec.Command(binary, command, "--profile-ref", profile, "--manifest", manifest, "--intent-id", intent, "--registry", registry, "--output", "json")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("packaged CLI %s: %v: %s", command, err, output)
		}
	}
}
