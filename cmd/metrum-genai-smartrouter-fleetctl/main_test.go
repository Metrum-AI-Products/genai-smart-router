// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smart-llmrouter/internal/router"
)

func writeSignedFleetIntent(t *testing.T, directory string, manifest router.TenantDeploymentManifest, intentID string) string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return writeSignedFleetIntentWithProfile(t, directory, profileBytes, manifest, intentID)
}

func writeSignedFleetIntentWithProfile(t *testing.T, directory string, profileBytes []byte, manifest router.TenantDeploymentManifest, intentID string) string {
	t.Helper()
	profilePath := filepath.Join(directory, "profile.yaml")
	if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	intent := router.TenantDeploymentIntent{
		APIVersion: router.TenantDeploymentIntentAPIVersion,
		IntentID:   intentID,
		IssuerRole: "fleet-lifecycle-admin",
		IssuedAt:   now.Add(-time.Minute),
		ExpiresAt:  now.Add(time.Hour),
		ProfileRef: "file://" + profilePath,
		Manifest:   manifest,
	}
	payload, err := router.TenantDeploymentIntentSigningPayload(intent)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := base64.StdEncoding.DecodeString("nWGxne/9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A=")
	if err != nil {
		t.Fatal(err)
	}
	intent.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), payload))
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "intent.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLifecycleCommandsRejectUnsupportedVerbs(t *testing.T) {
	for _, command := range []string{"promote", "rollback", "quota-reserve"} {
		cmd := exec.Command("go", "run", ".", command, "--manifest", "secret-like-input-must-not-be-echoed")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "unsupported command") {
			t.Fatalf("%s did not fail closed: err=%v output=%s", command, err, out)
		}
		if strings.Contains(string(out), "secret-like-input-must-not-be-echoed") {
			t.Fatalf("%s echoed untrusted lifecycle input: %s", command, out)
		}
	}
	for _, command := range []string{"plan", "deploy", "delete"} {
		cmd := exec.Command("go", "run", ".", command, "--intent", "secret-like-input-must-not-be-echoed")
		out, err := cmd.CombinedOutput()
		expected := "load signed deployment intent"
		if command == "delete" {
			expected = "confirm-file is required"
		}
		if err == nil || !strings.Contains(string(out), expected) {
			t.Fatalf("%s missing contract did not fail closed: err=%v output=%s", command, err, out)
		}
		if strings.Contains(string(out), "secret-like-input-must-not-be-echoed") {
			t.Fatalf("%s echoed untrusted lifecycle input: %s", command, out)
		}
	}
}

func TestFleetPlanRejectsSplitLifecycleInputs(t *testing.T) {
	output, err := exec.Command("go", "run", ".", "plan",
		"--profile-ref", "aws-ssm:///approved/nonproduction/profile",
		"--manifest", "deployment.yaml",
		"--intent-id", "intent-a",
	).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "flag provided but not defined") {
		t.Fatalf("split lifecycle inputs were accepted: %v %s", err, output)
	}
	if strings.Contains(string(output), "aws-ssm:///approved/nonproduction/profile") {
		t.Fatalf("rejected input was echoed: %s", output)
	}
}

func TestTenantDeploymentCLIPlanIsReadOnly(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	manifest, err := router.LoadTenantDeploymentManifest(filepath.Join(root, "manifest.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	intent := writeSignedFleetIntent(t, dir, manifest, "intent-a")
	registry := filepath.Join(dir, "deployments.sqlite")
	common := []string{"--intent", intent, "--registry", registry, "--output", "json"}
	run := func(command string, extra ...string) (string, error) {
		args := append([]string{"run", ".", command}, common...)
		args = append(args, extra...)
		out, runErr := exec.Command("go", args...).CombinedOutput()
		return string(out), runErr
	}
	planOutput, err := run("plan")
	if err != nil || !strings.Contains(planOutput, `"mode": "eks"`) || strings.Contains(planOutput, "aws-secretsmanager") {
		t.Fatalf("plan failed or leaked references: %v %s", err, planOutput)
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatalf("plan mutated registry: %v", err)
	}
}

func TestFleetE2ERequiresRDSAdmissionOnlyForDedicatedRDSManifest(t *testing.T) {
	if requiresDisposableRDSAdmission(router.TenantDeploymentManifest{}) {
		t.Fatal("SQLite Fleet E2E unexpectedly requires an RDS admission")
	}
	if !requiresDisposableRDSAdmission(router.TenantDeploymentManifest{DatabaseProfile: "postgres-dedicated-small"}) {
		t.Fatal("dedicated-RDS Fleet E2E does not require an RDS admission")
	}
}

func TestTenantDeploymentCLIPlansIsolatedAcme2Namespace(t *testing.T) {
	dir := t.TempDir()
	manifest := router.TenantDeploymentManifest{
		APIVersion:       router.TenantDeploymentManifestAPIVersion,
		CustomerID:       "acme2",
		Stage:            "test",
		Release:          router.TenantReleaseLatestApproved,
		ResourceProfile:  "small",
		StateProfile:     "sqlite-rwo-small",
		RuntimeBundleRef: "aws-secretsmanager:///smart-router/test/acme2/runtime-bundle",
		ConfigRevision:   "revision-acme2",
		License: router.TenantDeploymentLicense{
			RequestRef: "aws-ssm:///smart-router/test/acme2/license-request",
			Validity:   "168h",
		},
	}
	intent := writeSignedFleetIntent(t, dir, manifest, "intent-acme2")
	output, err := exec.Command("go", "run", ".", "plan",
		"--intent", intent,
		"--output", "json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("Acme2 Fleet CLI plan: %v: %s", err, output)
	}
	var plan router.TenantDeploymentPlan
	if err := json.Unmarshal(output, &plan); err != nil {
		t.Fatalf("decode Acme2 Fleet CLI plan: %v: %s", err, output)
	}
	if plan.CustomerID != "acme2" || plan.Namespace != "acme2" || plan.Hostname != "acme2.apps.example.test" {
		t.Fatalf("unexpected isolated Acme2 plan: %#v", plan)
	}
	if strings.Contains(string(output), "aws-secretsmanager") || strings.Contains(string(output), "aws-ssm") {
		t.Fatalf("Acme2 Fleet CLI plan leaked protected references: %s", output)
	}
}

func TestDedicatedRDSDeployRequiresProtectedProfileBeforeRegistryOrCloudAccess(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profileBytes = append(profileBytes, []byte(`
database_mode: dedicated-rds
approved_database_profile: postgres-dedicated-small
rds_instance_class: db-t4g-medium
rds_storage_gib: 20
rds_backup_retention_days: 7
rds_subnet_group: router-private
rds_vpc_security_group: sg-router-private
rds_master_username: routeradmin
rds_proxy_disabled: true
`)...)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes = []byte(strings.Replace(string(manifestBytes), "runtime_bundle_ref:", "database_profile: postgres-dedicated-small\nruntime_bundle_ref:", 1))
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	registry := filepath.Join(dir, "lifecycle.sqlite")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := router.LoadTenantDeploymentManifest(manifestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	intent := writeSignedFleetIntentWithProfile(t, dir, profileBytes, manifest, "intent-rds-admission")
	output, err := exec.Command("go", "run", ".", "deploy",
		"--intent", intent,
		"--registry", registry,
		"--output", "json",
	).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "protected aws-ssm profile") {
		t.Fatalf("dedicated RDS deploy accepted a local profile: %v %s", err, output)
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatalf("missing RDS admission created registry: %v", err)
	}
}

func TestDedicatedRDSAdaptersRequireExternalAdmissionBeforeCloudAccess(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profileBytes = append(profileBytes, []byte(`
database_mode: dedicated-rds
approved_database_profile: postgres-dedicated-small
rds_instance_class: db-t4g-medium
rds_storage_gib: 20
rds_backup_retention_days: 7
rds_subnet_group: router-private
rds_vpc_security_group: sg-router-private
rds_master_username: routeradmin
rds_proxy_disabled: true
`)...)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes = []byte(strings.Replace(string(manifestBytes), "runtime_bundle_ref:", "database_profile: postgres-dedicated-small\nruntime_bundle_ref:", 1))
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := router.LoadTenantDeploymentProfile("file://" + profilePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := router.LoadTenantDeploymentManifest(manifestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := router.BuildTenantDeploymentPlan(profile, manifest, "intent-rds-admission")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := deploymentAdaptersForPlan(context.Background(), profile, "aws-ssm:///approved/nonproduction/profile", plan, "", true); err == nil || !strings.Contains(err.Error(), "rds-admission-file is required") {
		t.Fatalf("missing RDS admission reached cloud-adapter setup: %v", err)
	}
}
