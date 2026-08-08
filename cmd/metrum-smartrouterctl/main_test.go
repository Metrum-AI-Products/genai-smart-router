package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
