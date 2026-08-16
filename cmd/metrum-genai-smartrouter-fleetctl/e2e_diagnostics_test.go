package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSanitizePackageE2ECommandDiagnosticsRetainsActionableText(t *testing.T) {
	raw := []byte("deploy failed: deletion approval has already been used\n" +
		`{"job_id":"job-1","state":"failed","error_class":"approval_expired","next_action":"reissue_delete_approval","profile_ref":"aws-ssm:///approved/nonproduction/profile"}` + "\n")
	got := sanitizePackageE2ECommandDiagnostics(raw)
	for _, want := range []string{"deploy failed", "approval_expired", "job-1", "failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "aws-ssm:///") {
		t.Fatalf("leaked protected ref in %q", got)
	}
}

func TestSanitizePackageE2ECommandDiagnosticsRedactsSecrets(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"plan failed: aws-ssm:///approved/nonproduction/profile",
		"bundle aws-secretsmanager:///tenants/acme/runtime-bundle",
		"token sk-abcdefghijklmnop",
		"Authorization: Bearer rtr_metrum_user_project_env_key_secret",
		"config.yaml:\napi_key: secret",
		"OPENROUTER_API_KEY=sk-leak",
		"secret-like-input-must-not-be-echoed",
		`{"error_class":"intent_expired","job_id":"job-9"}`,
	}, "\n"))
	got := sanitizePackageE2ECommandDiagnostics(raw)
	for _, forbidden := range []string{
		"aws-ssm:///",
		"aws-secretsmanager:///",
		"sk-abcdefghijklmnop",
		"rtr_metrum_user_project_env_key_secret",
		"config.yaml:",
		"OPENROUTER_API_KEY",
		"secret-like-input-must-not-be-echoed",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "intent_expired") || !strings.Contains(got, "job-9") {
		t.Fatalf("lost safe scalars in %q", got)
	}
}

func TestFormatPackageE2ECommandFailureIncludesExitAndBody(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'deploy failed: approval_expired'; exit 2")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	got := formatPackageE2ECommandFailure("deploy", err, out)
	if !strings.Contains(got, "release package deploy failed") {
		t.Fatalf("missing command label: %q", got)
	}
	if !strings.Contains(got, "exit status 2") {
		t.Fatalf("missing exit status in %q", got)
	}
	if !strings.Contains(got, "deploy failed: approval_expired") {
		t.Fatalf("missing sanitized body: %q", got)
	}
}

func TestFormatPackageE2ECommandFailureRedactsProtectedRefs(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'deploy failed: approval_expired'; echo 'aws-ssm:///approved/nonproduction/profile' >&2; exit 3")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	got := formatPackageE2ECommandFailure("deploy", err, out)
	if !strings.Contains(got, "exit status 3") {
		t.Fatalf("missing exit status in %q", got)
	}
	if !strings.Contains(got, "approval_expired") {
		t.Fatalf("missing actionable text in %q", got)
	}
	if strings.Contains(got, "aws-ssm:///") {
		t.Fatalf("leaked protected ref in %q", got)
	}
}
