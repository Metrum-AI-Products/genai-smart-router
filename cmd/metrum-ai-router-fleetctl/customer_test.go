// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeCustomerID(t *testing.T) {
	got, err := normalizeCustomerID("Acme4")
	if err != nil || got != "acme4" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := normalizeCustomerID("1bad"); err == nil {
		t.Fatal("expected invalid leading digit")
	}
	if _, err := normalizeCustomerID("Bad_ID"); err == nil {
		t.Fatal("expected invalid underscore")
	}
	if _, err := normalizeCustomerID("a"); err == nil {
		t.Fatal("expected too-short id")
	}
}

func TestStableCustomerNaming(t *testing.T) {
	if got := customerHostname("acme4"); got != "acme4.apps.example.test" {
		t.Fatalf("hostname=%q", got)
	}
	want := "aws-secretsmanager:///smartrouter/fleet/customers/acme4/runtime-bundle"
	if got := customerBundleRef("acme4"); got != want {
		t.Fatalf("bundle ref=%q want %q", got, want)
	}
}

func TestDeepMergeAndMergeCallers(t *testing.T) {
	base := map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 1}},
		"callers": []any{
			map[string]any{"id": "a", "allow": []any{"high"}, "project": "p1"},
		},
	}
	patch := map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 2}, "fast": map[string]any{"weight": 1}},
		"callers": []any{
			map[string]any{"id": "a", "allow": []any{"high", "fast"}},
			map[string]any{"id": "b", "allow": []any{"default"}, "project": "p2"},
		},
	}
	mergedAny := deepMerge(base, patch)
	merged := asStringMapMust(mergedAny)
	models := asStringMapMust(merged["models"])
	high := asStringMapMust(models["high"])
	if high["weight"] != 2 {
		t.Fatalf("high weight=%v", high["weight"])
	}
	if _, ok := models["fast"]; !ok {
		t.Fatal("expected fast model")
	}
	callers, ok := asAnySlice(merged["callers"])
	if !ok || len(callers) != 2 {
		t.Fatalf("callers=%v", merged["callers"])
	}
	first := asStringMapMust(callers[0])
	if first["id"] != "a" {
		t.Fatalf("first caller id=%v", first["id"])
	}
	allow, ok := asAnySlice(first["allow"])
	if !ok || len(allow) != 2 {
		t.Fatalf("merged allow=%v", first["allow"])
	}
	second := asStringMapMust(callers[1])
	if second["id"] != "b" || second["project"] != "p2" {
		t.Fatalf("second caller=%v", second)
	}

	patched, err := applyConfigPatch("models:\n  high:\n    weight: 1\n", map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if patched == "" || !strings.Contains(patched, "weight: 3") {
		t.Fatalf("patched=%q", patched)
	}
}

func TestPlanSelectsDedicatedRDS(t *testing.T) {
	if planSelectsDedicatedRDS(planPayload{}) {
		t.Fatal("empty plan should be SQLite-safe")
	}
	if !planSelectsDedicatedRDS(planPayload{DatabaseID: "db-1"}) {
		t.Fatal("database_id should refuse")
	}
	if !planSelectsDedicatedRDS(planPayload{DatabaseProfile: "postgres-dedicated-small"}) {
		t.Fatal("database_profile should refuse")
	}
	if !planSelectsDedicatedRDS(planPayload{Actions: []string{"namespace", "dedicated_rds", "router"}}) {
		t.Fatal("dedicated_rds action should refuse")
	}
}

func TestRejectForeignDefaultRefs(t *testing.T) {
	if err := foreignDefaultRefError("acme-rehearsal", "aws-ssm:///x/acme-rehearsal/license-request"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := foreignDefaultRefError("aditya-test1", "aws-ssm:///x/acme-rehearsal/license-request"); err == nil {
		t.Fatal("expected mismatch rejection")
	}
	if err := foreignDefaultRefError("aditya-test1", "aws-ssm:///tenants/aditya-test1/license-request"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateIntentEnvelopeRejectsDedicatedRDS(t *testing.T) {
	env := intentEnvelope{
		ProfileRef: "aws-ssm:///approved/nonproduction/profile",
	}
	env.Manifest.CustomerID = "aditya-test1"
	env.Manifest.Stage = "nonproduction"
	env.Manifest.RuntimeBundleRef = "aws-secretsmanager:///tenants/aditya-test1/runtime"
	env.Manifest.License.RequestRef = "aws-ssm:///tenants/aditya-test1/license-request"
	if err := validateIntentEnvelope(env); err != nil {
		t.Fatalf("sqlite intent: %v", err)
	}
	env.Manifest.DatabaseProfile = "postgres-dedicated-small"
	if err := validateIntentEnvelope(env); err == nil || !strings.Contains(err.Error(), "dedicated RDS") {
		t.Fatalf("expected dedicated RDS refusal, got %v", err)
	}
}

func TestWriteManifestSQLiteOnlyNoDonorCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	donorDir := filepath.Join(home, ".local", "share", "metrum-fleet", "acme-rehearsal")
	if err := os.MkdirAll(donorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	donorPriv := filepath.Join(donorDir, "lifecycle_approval_private_key.b64")
	donorPub := filepath.Join(donorDir, "lifecycle_approval_public_key.b64")
	if err := os.WriteFile(donorPriv, []byte("donor-private-seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(donorPub, []byte("donor-public\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws := prepareCustomerWorkspace("aditya-test1")
	path := writeManifest(
		ws,
		"aws-ssm:///approved/nonproduction/aditya-test1-profile",
		"aws-secretsmanager:///smartrouter/fleet/customers/aditya-test1/runtime-bundle",
		"aws-ssm:///tenants/aditya-test1/license-request",
		"sqlite",
		"",
	)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest["database_profile"]; ok {
		t.Fatalf("customer write-manifest must omit database_profile: %s", raw)
	}
	if manifest["stage"] != "nonproduction" {
		t.Fatalf("stage=%v", manifest["stage"])
	}
	if manifest["state_profile"] != "sqlite-rwo-small" {
		t.Fatalf("state_profile=%v", manifest["state_profile"])
	}
	if manifest["customer_id"] != "aditya-test1" {
		t.Fatalf("customer_id=%v", manifest["customer_id"])
	}

	copiedPriv := filepath.Join(ws.Home, "lifecycle_approval_private_key.b64")
	copiedPub := filepath.Join(ws.Home, "lifecycle_approval_public_key.b64")
	if fileExists(copiedPriv) || fileExists(copiedPub) {
		t.Fatal("customer path must not copy donor lifecycle keys")
	}
}

var (
	customerCLIBinOnce sync.Once
	customerCLIBinPath string
	customerCLIBinErr  error
)

func customerCLIBinary(t *testing.T) string {
	t.Helper()
	customerCLIBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fleetctl-customer-cli-")
		if err != nil {
			customerCLIBinErr = err
			return
		}
		customerCLIBinPath = filepath.Join(dir, "metrum-ai-router-fleetctl")
		cmd := exec.Command("go", "build", "-o", customerCLIBinPath, ".")
		out, err := cmd.CombinedOutput()
		if err != nil {
			customerCLIBinErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			_ = os.RemoveAll(dir)
			customerCLIBinPath = ""
			return
		}
	})
	if customerCLIBinErr != nil {
		t.Fatalf("build customer CLI once: %v", customerCLIBinErr)
	}
	return customerCLIBinPath
}

func runCustomerCLI(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	bin := customerCLIBinary(t)
	cmd := exec.Command(bin, append([]string{"customer"}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCustomerCLICreateRequiresIntent(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "create", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "requires --intent") {
		t.Fatalf("expected signed-intent requirement, got: %s", out)
	}
}

func TestCustomerCLIDeleteRequiresConfirmFile(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "delete", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "confirm-file") && !strings.Contains(out, "sign-with-key") {
		t.Fatalf("expected confirm-file or sign-with-key requirement, got: %s", out)
	}
}

func TestCustomerCLIDeleteRejectsBothConfirmAndSign(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "delete",
		"--customer-id", "aditya-test1",
		"--confirm-file", "/tmp/confirm.json",
		"--sign-with-key", "/tmp/key.b64",
	)
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "not both") {
		t.Fatalf("expected mutual exclusion error, got: %s", out)
	}
}

func TestGenerateDeleteNonceIsDNSSafe(t *testing.T) {
	nonce := generateDeleteNonce()
	if !deleteNonceRE.MatchString(nonce) {
		t.Fatalf("nonce=%q does not match delete nonce pattern", nonce)
	}
	if !strings.HasPrefix(nonce, "del-") {
		t.Fatalf("nonce=%q missing del- prefix", nonce)
	}
	if strings.Contains(nonce, "T") {
		t.Fatalf("nonce=%q must not contain uppercase T", nonce)
	}
}

func TestCustomerListFromWorkspacePlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wsHome := filepath.Join(home, ".local", "share", "metrum-fleet", "gamma3")
	if err := os.MkdirAll(wsHome, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := planPayload{JobID: "job-workspace-gamma3", Hostname: "gamma3.apps.example.test", State: "ready"}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsHome, "plan.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCustomerCLI(t, home, "list")
	if err != nil {
		t.Fatalf("customer list failed: %v\n%s", err, out)
	}
	var payload struct {
		Customers []map[string]any `json:"customers"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("parse list output: %v\n%s", err, out)
	}
	if len(payload.Customers) != 1 {
		t.Fatalf("customers=%v", payload.Customers)
	}
	row := payload.Customers[0]
	if row["customer_id"] != "gamma3" || row["job_id"] != "job-workspace-gamma3" {
		t.Fatalf("row=%v", row)
	}
	if row["hostname"] != "gamma3.apps.example.test" {
		t.Fatalf("hostname=%v", row["hostname"])
	}
}

func TestDefaultRegistryPathPrefersSharedRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := sharedRegistryPath()
	if err := os.WriteFile(shared, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := defaultRegistryPath(); got != shared {
		t.Fatalf("defaultRegistryPath()=%q want %q", got, shared)
	}
	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	if got := defaultRegistryPath(); got != "./tenant-deployments.sqlite" {
		t.Fatalf("fallback=%q", got)
	}
}

func TestCustomerCLIWriteManifestRequiresRefs(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home, "write-manifest", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "no ACME/staging defaults") && !strings.Contains(out, "required") {
		t.Fatalf("expected missing-refs failure, got: %s", out)
	}
}

func TestCustomerCLIWriteManifestRejectsACMERehearsalForOtherAlias(t *testing.T) {
	home := t.TempDir()
	out, err := runCustomerCLI(t, home,
		"write-manifest",
		"--customer-id", "aditya-test1",
		"--profile-ref", "aws-ssm:///approved/nonproduction/profile",
		"--runtime-bundle-ref", "aws-secretsmanager:///tenants/aditya-test1/runtime",
		"--license-ref", "aws-ssm:///metrum/smartrouter/fleet/acme-rehearsal/license-request",
	)
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "acme-rehearsal") {
		t.Fatalf("expected acme-rehearsal refusal, got: %s", out)
	}
}

func TestCustomerCLICreateRefusesDedicatedRDSIntent(t *testing.T) {
	home := t.TempDir()
	intentPath := filepath.Join(home, "rds-intent.json")
	body := `{
  "intent_id": "intent-rds",
  "profile_ref": "aws-ssm:///approved/nonproduction/profile",
  "manifest": {
    "customer_id": "aditya-test1",
    "stage": "nonproduction",
    "runtime_bundle_ref": "aws-secretsmanager:///tenants/aditya-test1/runtime",
    "license": {"request_ref": "aws-ssm:///tenants/aditya-test1/license-request"},
    "database_profile": "postgres-dedicated-small"
  }
}`
	if err := os.WriteFile(intentPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCustomerCLI(t, home, "create", "--intent", intentPath)
	if err == nil {
		t.Fatalf("expected failure, got success: %s", out)
	}
	if !strings.Contains(out, "dedicated RDS") {
		t.Fatalf("expected dedicated RDS refusal before plan, got: %s", out)
	}
}

func TestCustomerCLICreateDoesNotCopyDonorKeys(t *testing.T) {
	home := t.TempDir()
	donorDir := filepath.Join(home, ".local", "share", "metrum-fleet", "acme2")
	if err := os.MkdirAll(donorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(donorDir, "lifecycle_approval_private_key.b64"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(donorDir, "lifecycle_approval_public_key.b64"), []byte("pub\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCustomerCLI(t, home, "create", "--customer-id", "aditya-test1")
	if err == nil {
		t.Fatalf("expected failure without intent, got: %s", out)
	}
	if !strings.Contains(out, "requires --intent") {
		t.Fatalf("expected missing-intent failure, got: %s", out)
	}
	wsHome := filepath.Join(home, ".local", "share", "metrum-fleet", "aditya-test1")
	if fileExists(filepath.Join(wsHome, "lifecycle_approval_private_key.b64")) {
		t.Fatal("create must not copy donor private keys")
	}
}

func TestTransformRuntimeBundleFleetEKSPaths(t *testing.T) {
	configYAML := "server:\n  usage_db:\n    path: /app/state/usage.sqlite\n  log_dir: /app/logs\nmodels:\n  high:\n    targets:\n    - provider: openai\n      model_ref: gpt-test\n"
	envJSON := `{"ROUTER_USAGE_DB_DSN":"postgres:///app/state/db"}`
	bundle, stats, err := transformRuntimeBundle(configYAML, envJSON, bundleTransformOptions{
		StripCallers: true,
		RewritePaths: "fleet-eks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bundle.ConfigYAML, "/var/lib/smart-llmrouter") {
		t.Fatalf("config paths not rewritten: %s", bundle.ConfigYAML)
	}
	if strings.Contains(bundle.ConfigYAML, "/app/state") || strings.Contains(bundle.ConfigYAML, "/app/logs") {
		t.Fatalf("compose paths remain: %s", bundle.ConfigYAML)
	}
	if !strings.Contains(bundle.EnvJSON, "/var/lib/smart-llmrouter") {
		t.Fatalf("env paths not rewritten: %s", bundle.EnvJSON)
	}
	if stats.ModelGroupCount != 1 {
		t.Fatalf("model_group_count=%d", stats.ModelGroupCount)
	}
}

func TestTransformRuntimeBundleStripCallers(t *testing.T) {
	configYAML := "callers:\n- id: old\nusers:\n  u1: {}\nprojects:\n  p1: {}\nproject_memberships: []\nmodels:\n  high: {}\n"
	bundle, _, err := transformRuntimeBundle(configYAML, "{}", bundleTransformOptions{StripCallers: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bundle.ConfigYAML, "old") || strings.Contains(bundle.ConfigYAML, "users:") {
		t.Fatalf("callers/users not stripped: %s", bundle.ConfigYAML)
	}
}

func TestEnforceSecretsManagerSizeLimitRejectsOversize(t *testing.T) {
	huge := strings.Repeat("x", secretsManagerMaxPayloadBytes)
	err := enforceSecretsManagerSizeLimit(runtimeBundle{
		ConfigYAML: "models:\n  big:\n    note: " + huge,
		EnvJSON:    "{}",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds Secrets Manager limit") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestTrimCatalogOnlyModelsKeepsReferencedTargets(t *testing.T) {
	configYAML := `providers:
  openai:
    models:
      used:
        model: gpt-used
      unused:
        model: gpt-unused
models:
  high:
    targets:
    - provider: openai
      model_ref: used
`
	bundle, _, err := transformRuntimeBundle(configYAML, "{}", bundleTransformOptions{TrimCatalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bundle.ConfigYAML, "used:") {
		t.Fatalf("referenced model removed: %s", bundle.ConfigYAML)
	}
	if strings.Contains(bundle.ConfigYAML, "unused:") {
		t.Fatalf("catalog-only model kept: %s", bundle.ConfigYAML)
	}
}

func TestPublishRuntimeBundleOperatorClearsFleetSession(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAFLEET")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "session")
	t.Setenv("METRUM_FLEET_LIFECYCLE_ROLE_ARN", "arn:aws:iam::123456789012:role/example-fleet-lifecycle")
	roleARN, err := fleetLifecycleRoleARN()
	if err != nil {
		t.Fatal(err)
	}
	if roleARN != "arn:aws:iam::123456789012:role/example-fleet-lifecycle" {
		t.Fatalf("role ARN=%q", roleARN)
	}
	if _, _, ok := existingFleetRoleSession(context.Background()); ok {
		t.Fatal("expected no fleet session without matching caller identity")
	}
	clearFleetSessionCredentials()
	if os.Getenv("AWS_SESSION_TOKEN") != "" {
		t.Fatal("session token not cleared")
	}
}

func TestFleetLifecycleRoleARNRequiresEnv(t *testing.T) {
	t.Setenv("METRUM_FLEET_LIFECYCLE_ROLE_ARN", "")
	if _, err := fleetLifecycleRoleARN(); err == nil || !strings.Contains(err.Error(), "METRUM_FLEET_LIFECYCLE_ROLE_ARN") {
		t.Fatalf("expected required-env error, got %v", err)
	}
}

func TestApplyGrantCallerPatchProvisionsExplicitDirectory(t *testing.T) {
	configYAML := `users:
  - id: bootstrap-owner
    name: Bootstrap Owner
    status: active
projects:
  - id: bootstrap
    name: Bootstrap
    status: active
project_memberships:
  - user_id: bootstrap-owner
    project: bootstrap
    status: active
    role: owner
callers: []
models:
  default: {}
`
	caller := map[string]any{
		"id":           "new-caller",
		"owner_user":   "acme-admin",
		"project":      "acme",
		"environment":  "nonproduction",
		"status":       "active",
		"token_sha256": "abc",
		"allow":        []any{"default"},
	}
	patched, err := applyGrantCallerPatch(configYAML, caller, "acme-admin", "acme")
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseConfigRoot(patched)
	if err != nil {
		t.Fatal(err)
	}
	users := accountIndex(root["users"], "id")
	projects := accountIndex(root["projects"], "id")
	memberships := membershipIndex(root["project_memberships"])
	if _, ok := users["acme-admin"]; !ok {
		t.Fatalf("missing user: %v", users)
	}
	if _, ok := projects["acme"]; !ok {
		t.Fatalf("missing project: %v", projects)
	}
	if _, ok := memberships["acme-admin\x00acme"]; !ok {
		t.Fatalf("missing membership: %v", memberships)
	}
	if _, ok := users["bootstrap-owner"]; !ok {
		t.Fatalf("lost bootstrap user: %v", users)
	}
	callers, _ := asAnySlice(root["callers"])
	if len(callers) != 1 {
		t.Fatalf("callers=%d", len(callers))
	}
}

func TestApplyGrantCallerPatchSkipsSyntheticDirectory(t *testing.T) {
	configYAML := "callers: []\nmodels:\n  default: {}\n"
	caller := map[string]any{
		"id":         "new-caller",
		"owner_user": "acme-admin",
		"project":    "acme",
		"status":     "active",
		"allow":      []any{"default"},
	}
	patched, err := applyGrantCallerPatch(configYAML, caller, "acme-admin", "acme")
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseConfigRoot(patched)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := root["users"]; ok {
		t.Fatalf("users should remain absent for synthetic accounts: %#v", root["users"])
	}
	if _, ok := root["projects"]; ok {
		t.Fatalf("projects should remain absent: %#v", root["projects"])
	}
	callers, _ := asAnySlice(root["callers"])
	if len(callers) != 1 {
		t.Fatalf("callers=%d", len(callers))
	}
}
