package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"smart-llmrouter/internal/router"
)

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
		cmd := exec.Command("go", "run", ".", command, "--manifest", "secret-like-input-must-not-be-echoed")
		out, err := cmd.CombinedOutput()
		expected := "profile-ref is required"
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

func TestTenantDeploymentCLIPlanIsReadOnly(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(profile, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(t.TempDir(), "deployments.sqlite")
	common := []string{"--profile-ref", "file://" + profile, "--manifest", filepath.Join(root, "manifest.yaml"), "--intent-id", "intent-a", "--registry", registry, "--output", "json"}
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
	profile := filepath.Join(dir, "profile.yaml")
	manifest := filepath.Join(dir, "manifest.yaml")
	registry := filepath.Join(dir, "lifecycle.sqlite")
	if err := os.WriteFile(profile, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("go", "run", ".", "deploy",
		"--profile-ref", "file://"+profile,
		"--manifest", manifest,
		"--intent-id", "intent-rds-admission",
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
