package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smart-llmrouter/internal/router"
)

func TestRouterLicenseCLIKeySignVerifyInspect(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "license.pub")
	priv := filepath.Join(dir, "license.key")
	runCLI(t, "generate-keypair", "--public-key-out", pub, "--private-key-out", priv)
	info, err := os.Stat(priv)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key perms=%o, want 600", info.Mode().Perm())
	}

	payload := router.LicensePayload{
		SchemaVersion: 1,
		LicenseID:     "lic_cli_test",
		CustomerID:    "cust_cli_test",
		Product:       "genai-smart-router",
		SKU:           "enterprise-test",
		Features:      []string{"routing", "usage_reporting"},
		IssuedAt:      time.Now().UTC().Add(-time.Hour),
		NotBefore:     time.Now().UTC().Add(-time.Hour),
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
		Issuer:        router.LicenseIssuer,
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payloadPath, payloadRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	licensePath := filepath.Join(dir, "license.json")
	runCLI(t, "sign", "--payload", payloadPath, "--key", priv, "--key-id", "test-license-key", "--out", licensePath)
	out := runCLI(t, "verify", "--license", licensePath, "--public-key", pub)
	if !strings.Contains(out, "valid") {
		t.Fatalf("verify output=%q", out)
	}
	inspect := runCLI(t, "inspect", "--license", licensePath)
	if strings.Contains(inspect, "value_base64") || strings.Contains(inspect, "private") {
		t.Fatalf("inspect leaked signature/private key material: %s", inspect)
	}
	if !strings.Contains(inspect, "lic_cli_test") {
		t.Fatalf("inspect missing safe summary: %s", inspect)
	}
}

func TestRouterLicenseCLIIssueValidateRenewTopUpAndSafeSummary(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "license.pub")
	priv := filepath.Join(dir, "license.key")
	runCLI(t, "generate-keypair", "--public-key-out", pub, "--private-key-out", priv)

	entitlementPath := filepath.Join(dir, "entitlement.json")
	entitlement := `{
  "license_id": "lic_issue_test_001",
  "customer_id": "cust_issue_test",
  "customer_name": "Example Customer",
  "sku": "enterprise-annual",
  "term": {
    "issued_at": "2026-06-28T00:00:00Z",
    "not_before": "2026-06-28T00:00:00Z",
    "expires_at": "2027-06-28T00:00:00Z"
  },
  "features": {
    "add": ["admin_security_reports"]
  },
  "limits": {
    "allowed_skins": ["openai-chat", "openai-responses", "anthropic-messages"]
  },
  "deployment": {
    "allowed_instances": ["fp:test-instance"]
  },
  "signing": {
    "key_id": "test-license-key"
  }
}`
	if err := os.WriteFile(entitlementPath, []byte(entitlement), 0o600); err != nil {
		t.Fatal(err)
	}
	licensePath := filepath.Join(dir, "license.json")
	payloadPath := filepath.Join(dir, "payload.json")
	summaryPath := filepath.Join(dir, "summary.json")
	checklistPath := filepath.Join(dir, "checklist.md")
	catalog := filepath.Join("..", "..", "docs", "enterprise-license-skus.json")
	out := runCLI(t, "issue",
		"--catalog", catalog,
		"--entitlement", entitlementPath,
		"--key", priv,
		"--public-key", pub,
		"--allow-unknown-runtime-key",
		"--out", licensePath,
		"--payload-out", payloadPath,
		"--summary-out", summaryPath,
		"--checklist-out", checklistPath,
	)
	if !strings.Contains(out, "issued lic_issue_test_001") {
		t.Fatalf("issue output=%q", out)
	}
	assertPerm(t, licensePath, 0o600)
	assertPerm(t, payloadPath, 0o600)
	assertPerm(t, summaryPath, 0o600)
	assertPerm(t, checklistPath, 0o600)

	validateOut := runCLI(t, "validate", "--license", licensePath, "--public-key", pub, "--allow-unknown-runtime-key")
	if !strings.Contains(validateOut, "valid") {
		t.Fatalf("validate output=%q", validateOut)
	}
	summary := runCLI(t, "safe-summary", "--license", licensePath)
	if !strings.Contains(summary, "lic_issue_test_001") || strings.Contains(summary, "value_base64") || strings.Contains(summary, "private key") {
		t.Fatalf("unsafe safe-summary output=%s", summary)
	}

	renewed := filepath.Join(dir, "renewed.json")
	runCLI(t, "renew",
		"--license", licensePath,
		"--key", priv,
		"--public-key", pub,
		"--allow-unknown-runtime-key",
		"--license-id", "lic_issue_test_renewed",
		"--expires-at", "2028-06-28T00:00:00Z",
		"--out", renewed,
	)
	renewedSummary := runCLI(t, "safe-summary", "--license", renewed)
	if !strings.Contains(renewedSummary, "lic_issue_test_renewed") || !strings.Contains(renewedSummary, "2028-06-28T00:00:00Z") {
		t.Fatalf("renewed summary=%s", renewedSummary)
	}

	toppedUp := filepath.Join(dir, "topup.json")
	runCLI(t, "top-up",
		"--catalog", catalog,
		"--license", licensePath,
		"--sku", "credit-pack-5m",
		"--key", priv,
		"--public-key", pub,
		"--allow-unknown-runtime-key",
		"--license-id", "lic_issue_test_topup",
		"--out", toppedUp,
	)
	topupSummary := runCLI(t, "safe-summary", "--license", toppedUp)
	if !strings.Contains(topupSummary, "credit-pack-5m") || !strings.Contains(topupSummary, "lic_issue_test_topup") {
		t.Fatalf("top-up summary=%s", topupSummary)
	}

	revocationPath := filepath.Join(dir, "revocations.json")
	runCLI(t, "revocation", "create",
		"--set-id", "revset-cli",
		"--epoch", "1",
		"--license-id", "lic_issue_test_001",
		"--status", "revoked",
		"--reason", "test-revocation",
		"--key", priv,
		"--key-id", "test-license-key",
		"--out", revocationPath,
		"--public-key", pub,
	)
	runCLI(t, "revocation", "validate", "--bundle", revocationPath, "--public-key", pub)
	revocationSummary := runCLI(t, "revocation", "safe-summary", "--bundle", revocationPath)
	if !strings.Contains(revocationSummary, "revset-cli") || !strings.Contains(revocationSummary, "lic_issue_test_001") || strings.Contains(revocationSummary, "value_base64") {
		t.Fatalf("revocation summary wrong or unsafe: %s", revocationSummary)
	}
}
func TestRouterLicenseCLIIssueAppliesValidity(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "license.pub")
	priv := filepath.Join(dir, "license.key")
	runCLI(t, "generate-keypair", "--public-key-out", pub, "--private-key-out", priv)

	entitlementPath := filepath.Join(dir, "entitlement.json")
	entitlement := `{
  "license_id": "lic_validity_test",
  "customer_id": "cust_validity_test",
  "sku": "eval-72h",
  "signing": {"key_id": "test-license-key"}
}`
	if err := os.WriteFile(entitlementPath, []byte(entitlement), 0o600); err != nil {
		t.Fatal(err)
	}
	licensePath := filepath.Join(dir, "license.json")
	catalog := filepath.Join("..", "..", "docs", "enterprise-license-skus.json")
	before := time.Now().UTC()
	runCLI(t, "issue",
		"--catalog", catalog,
		"--entitlement", entitlementPath,
		"--key", priv,
		"--valid-for", "2h",
		"--public-key", pub,
		"--allow-unknown-runtime-key",
		"--out", licensePath,
	)
	env, err := readEnvelope(licensePath)
	if err != nil {
		t.Fatal(err)
	}
	validity := env.Payload.ExpiresAt.Sub(before)
	if validity < 119*time.Minute || validity > 121*time.Minute {
		t.Fatalf("license validity=%s, want 2h", validity)
	}
}

func TestRouterLicenseCLIRejectsInvalidEntitlementFeature(t *testing.T) {
	dir := t.TempDir()
	entitlementPath := filepath.Join(dir, "bad.json")
	entitlement := `{
  "license_id": "lic_bad_feature",
  "customer_id": "cust_bad_feature",
  "sku": "eval-72h",
  "term": {
    "issued_at": "2026-06-28T00:00:00Z",
    "not_before": "2026-06-28T00:00:00Z",
    "expires_at": "2026-07-01T00:00:00Z"
  },
  "features": {"include": ["routing", "bad_feature"]},
  "signing": {"key_id": "test-license-key"}
}`
	if err := os.WriteFile(entitlementPath, []byte(entitlement), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join("..", "..", "docs", "enterprise-license-skus.json")
	raw := runCLIFail(t, "template", "render", "--catalog", catalog, "--entitlement", entitlementPath)
	if !strings.Contains(raw, "unsupported feature") {
		t.Fatalf("expected unsupported feature error, got %s", raw)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = "."
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("router-license %v failed: %v\n%s", args, err, raw)
	}
	return string(raw)
}

func runCLIFail(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = "."
	raw, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("router-license %v unexpectedly succeeded:\n%s", args, raw)
	}
	return string(raw)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s perms=%o, want %o", path, info.Mode().Perm(), want)
	}
}
