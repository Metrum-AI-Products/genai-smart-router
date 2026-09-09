// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smart-llmrouter/internal/fleet"
)

func requiresDisposableRDSAdmission(manifest fleet.TenantDeploymentManifest) bool {
	return manifest.DatabaseProfile != ""
}

// TestDisposableEKSReleasePackageCoreLifecycle is intentionally
// environment-gated. It proves the core plan/deploy/delete lifecycle from an
// extracted release package, not the customer/Codex workflow. The latter
// requires two external signing stages and a real Codex client, so it remains
// an operator-run black-box acceptance sequence.
func TestDisposableEKSReleasePackageCoreLifecycle(t *testing.T) {
	intentPath := strings.TrimSpace(os.Getenv("EKS_E2E_INTENT"))
	deleteApprovalFile := strings.TrimSpace(os.Getenv("EKS_E2E_DELETE_APPROVAL_FILE"))
	packageDir := strings.TrimSpace(os.Getenv("EKS_E2E_PACKAGE_DIR"))
	registry := strings.TrimSpace(os.Getenv("EKS_E2E_REGISTRY"))
	rdsAdmissionFile := strings.TrimSpace(os.Getenv("EKS_E2E_RDS_ADMISSION_FILE"))
	if intentPath == "" || deleteApprovalFile == "" || packageDir == "" || registry == "" {
		t.Skip("set EKS_E2E_INTENT, EKS_E2E_DELETE_APPROVAL_FILE, EKS_E2E_PACKAGE_DIR, and EKS_E2E_REGISTRY for disposable EKS release-package E2E")
	}

	intent, _, err := fleet.LoadTenantDeploymentIntent(intentPath, time.Now().UTC())
	if err != nil {
		t.Fatalf("load disposable EKS intent: %v", err)
	}
	requiresRDSAdmission := requiresDisposableRDSAdmission(intent.Manifest)
	if requiresRDSAdmission && rdsAdmissionFile == "" {
		t.Skip("set EKS_E2E_RDS_ADMISSION_FILE for a dedicated-RDS disposable EKS E2E")
	}
	if !requiresRDSAdmission && rdsAdmissionFile != "" {
		t.Fatal("SQLite disposable EKS E2E must not set EKS_E2E_RDS_ADMISSION_FILE")
	}

	binary := filepath.Join(packageDir, "bin", "metrum-genai-smartrouter-fleetctl")
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("release package Fleet binary unavailable: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("release package Fleet binary is not executable")
	}
	run := func(command string, extra ...string) (output []byte, err error) {
		args := []string{command, "--intent", intentPath, "--registry", registry, "--output", "json"}
		args = append(args, extra...)
		return exec.Command(binary, args...).CombinedOutput()
	}
	if out, err := run("plan"); err != nil {
		t.Fatal(formatPackageE2ECommandFailure("plan", err, out))
	}
	t.Cleanup(func() {
		args := []string{"--confirm-file", deleteApprovalFile}
		if requiresRDSAdmission {
			args = append(args, "--rds-admission-file", rdsAdmissionFile)
		}
		if out, err := run("delete", args...); err != nil {
			t.Error(formatPackageE2ECommandFailure("cleanup", err, out))
		}
	})
	args := []string{}
	if requiresRDSAdmission {
		args = append(args, "--rds-admission-file", rdsAdmissionFile)
	}
	if out, err := run("deploy", args...); err != nil {
		t.Fatal(formatPackageE2ECommandFailure("deploy", err, out))
	}
}
