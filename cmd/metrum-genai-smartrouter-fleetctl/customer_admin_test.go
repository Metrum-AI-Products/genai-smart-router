// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleCallerConfig = `
users:
- id: coding
  name: Coding Agent
  type: service_account
  status: active
projects:
- id: metrum-insights
  name: Metrum Insights
  status: active
project_memberships:
- user_id: coding
  project: metrum-insights
  role: developer
  status: active
callers:
- id: coding-prod
  owner_user: coding
  project: metrum-insights
  environment: prod
  status: active
  token_sha256: deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
  token_id: rtr_coding_prod
  metrics_admin: false
  content_admin: false
  allow:
  - high
  - big-coder
  rate:
    rpm: 120
    tpm: 200000
    concurrent: 8
  quota:
    day:
      requests: 5000
      tokens: 20000000
    month:
      requests: 0
      tokens: 400000000
    soft_pct: 80
  key:
    lifetime_tokens: 2000000000
    soft_pct: 90
    on_exhaust: disable
`

func TestListCallersFromConfigIncludesProjectAndOmitsHash(t *testing.T) {
	rows, err := listCallersFromConfig(sampleCallerConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if row.ID != "coding-prod" || row.OwnerUser != "coding" || row.Project != "metrum-insights" {
		t.Fatalf("identity=%+v", row)
	}
	if row.ProjectName != "Metrum Insights" || row.UserName != "Coding Agent" {
		t.Fatalf("joined names=%+v", row)
	}
	if row.MembershipRole != "developer" || row.TokenID != "rtr_coding_prod" {
		t.Fatalf("membership/token=%+v", row)
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "deadbeef") || strings.Contains(string(raw), "token_sha256") {
		t.Fatalf("hash leaked: %s", raw)
	}
}

func TestCallersMatchingOwnerUser(t *testing.T) {
	matched, err := callersMatching(sampleCallerConfig, nil, []string{"coding"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || stringField(matched[0], "id") != "coding-prod" {
		t.Fatalf("matched=%v", matched)
	}
	none, err := callersMatching(sampleCallerConfig, []string{"missing"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected empty, got %v", none)
	}
}

func TestQuotaPatchFieldsRequireAtLeastOne(t *testing.T) {
	if got := quotaPatchFields(-1, -1, -1, -1, -1, -1, -1, -1, -1); len(got) != 0 {
		t.Fatalf("empty patch=%v", got)
	}
	got := quotaPatchFields(-1, 500000, -1, 1000, -1, -1, -1, -1, -1)
	rate, _ := asStringMap(got["rate"])
	if rate["tpm"] != 500000 {
		t.Fatalf("rate=%v", got["rate"])
	}
	quota, _ := asStringMap(got["quota"])
	day, _ := asStringMap(quota["day"])
	if day["tokens"] != int64(1000) {
		t.Fatalf("quota=%v", got["quota"])
	}
}

func TestCustomerCLIGetConfigRequiresRefsAndOut(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "get-config", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "required") && !strings.Contains(out, "config-out") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestCustomerCLIGetConfigRefusesOverwrite(t *testing.T) {
	home := t.TempDir()
	configOut := filepath.Join(home, "config.yaml")
	envOut := filepath.Join(home, "env.json")
	if err := os.WriteFile(configOut, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envOut, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCustomerCLI(t, home,
		"get-config",
		"--customer-id", "aditya-test1",
		"--profile-ref", "aws-ssm:///approved/nonproduction/profile",
		"--runtime-bundle-ref", "aws-secretsmanager:///smartrouter/fleet/customers/aditya-test1/runtime-bundle",
		"--license-ref", "aws-ssm:///tenants/aditya-test1/license-request",
		"--config-out", configOut,
		"--env-out", envOut,
	)
	if err == nil {
		t.Fatalf("expected overwrite refusal, got: %s", out)
	}
	if !strings.Contains(out, "already exists") {
		t.Fatalf("expected already exists, got: %s", out)
	}
}

func TestCustomerCLIListCallersRequiresRefs(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "list-callers", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "required") && !strings.Contains(out, "no ACME") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestCustomerCLIRevokeCallerRequiresID(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home,
		"revoke-caller",
		"--customer-id", "aditya-test1",
		"--profile-ref", "aws-ssm:///approved/nonproduction/profile",
		"--runtime-bundle-ref", "aws-secretsmanager:///smartrouter/fleet/customers/aditya-test1/runtime-bundle",
		"--license-ref", "aws-ssm:///tenants/aditya-test1/license-request",
	)
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "caller-id") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestCustomerCLIUpdateQuotaRequiresIdentity(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home,
		"update-quota",
		"--customer-id", "aditya-test1",
		"--profile-ref", "aws-ssm:///approved/nonproduction/profile",
		"--runtime-bundle-ref", "aws-secretsmanager:///smartrouter/fleet/customers/aditya-test1/runtime-bundle",
		"--license-ref", "aws-ssm:///tenants/aditya-test1/license-request",
		"--tpm", "1",
	)
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "caller-id") && !strings.Contains(out, "owner-user") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestCustomerCLIQuotaStatusRequiresAdminFile(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "quota-status", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "admin-basic-file") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestReadBasicAuthFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "basic")
	if err := os.WriteFile(path, []byte("admin:secret-pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	user, pass, err := readBasicAuthFile(path)
	if err != nil || user != "admin" || pass != "secret-pass" {
		t.Fatalf("user=%q pass=%q err=%v", user, pass, err)
	}
}

func TestApplyRevokeStatusPatch(t *testing.T) {
	patched, err := applyConfigPatch(sampleCallerConfig, map[string]any{
		"callers": []any{map[string]any{"id": "coding-prod", "status": "disabled"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patched, "status: disabled") {
		t.Fatalf("patched=%s", patched)
	}
	if !strings.Contains(patched, "coding-prod") {
		t.Fatalf("caller row dropped: %s", patched)
	}
}
