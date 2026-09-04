// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write0600(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0o600)
}

func validFixture(t *testing.T) (dir string, intent Intent) {
	t.Helper()
	dir = t.TempDir()
	byokPath := filepath.Join(dir, "byok.env.json")
	write0600(t, byokPath, `{"OPENAI_API_KEY":"sk-test-customer-key-not-real"}`+"\n")
	configPath := filepath.Join(dir, "config.yaml")
	write0600(t, configPath, `
providers:
  openai:
    dialect: openai-chat
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
    api_key_env: OPENAI_API_KEY
    models:
      gpt-4o-mini:
        model: gpt-4o-mini
models:
  default:
    strategy: weighted
    targets:
      - provider: openai
        model: gpt-4o-mini
        weight: 100
`)
	signKey := filepath.Join(dir, "sign.key")
	licenseKey := filepath.Join(dir, "license.key")
	write0600(t, signKey, "dGVzdA==\n")
	write0600(t, licenseKey, "dGVzdA==\n")
	intent = Intent{
		CustomerID:       "acme",
		Hostname:         "acme.apps.metrum.ai",
		SKU:              "eval-72h",
		CustomerEmail:    "ops@example.com",
		CustomerAlias:    "acme",
		OwnerUser:        "acme-admin",
		Project:          "acme",
		ProfileRef:       "aws-ssm:///metrum/smartrouter/profiles/staging",
		LicenseRef:       "aws-ssm:///metrum/smartrouter/fleet/acme/license-request",
		RuntimeBundleRef: "aws-secretsmanager:///smartrouter/fleet/customers/acme/runtime-bundle",
		ConfigFile:       configPath,
		SignKey:          signKey,
		LicenseKey:       licenseKey,
		LicenseKeyID:     "test-license-key",
		CommerceBaseURL:  "http://127.0.0.1:8091",
		BYOKEnvFile:      byokPath,
		BYOK: BYOKSpec{
			Provider:  "openai",
			APIKeyEnv: "OPENAI_API_KEY",
			Model:     "gpt-4o-mini",
		},
		Catalog: filepath.Join("docs", "enterprise-license-skus.json"),
	}
	return dir, intent
}

func TestValidateIntentHostnameContract(t *testing.T) {
	_, intent := validFixture(t)
	got, err := ValidateIntent(intent)
	if err != nil {
		t.Fatalf("ValidateIntent: %v", err)
	}
	if got.Hostname != "acme.apps.metrum.ai" || got.CustomerID != "acme" {
		t.Fatalf("unexpected normalize: %+v", got)
	}

	intent.Hostname = "wrong.apps.metrum.ai"
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "hostname must be") {
		t.Fatalf("expected hostname mismatch, got %v", err)
	}

	intent.Hostname = ""
	intent.CustomerID = "acme"
	got, err = ValidateIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "acme.apps.metrum.ai" {
		t.Fatalf("expected derived hostname, got %q", got.Hostname)
	}

	intent.CustomerID = ""
	intent.Hostname = "acme.apps.metrum.ai"
	got, err = ValidateIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerID != "acme" {
		t.Fatalf("expected derived customer_id, got %q", got.CustomerID)
	}
}

func TestValidateIntentRejectsEmbeddedSecrets(t *testing.T) {
	dir, intent := validFixture(t)
	path := filepath.Join(dir, "bad-intent.json")
	raw, _ := json.Marshal(map[string]any{
		"customer_id":        intent.CustomerID,
		"hostname":           intent.Hostname,
		"sku":                intent.SKU,
		"customer_email":     intent.CustomerEmail,
		"customer_alias":     intent.CustomerAlias,
		"owner_user":         intent.OwnerUser,
		"project":            intent.Project,
		"profile_ref":        intent.ProfileRef,
		"license_ref":        intent.LicenseRef,
		"runtime_bundle_ref": intent.RuntimeBundleRef,
		"config_file":        intent.ConfigFile,
		"sign_key":           intent.SignKey,
		"license_key":        intent.LicenseKey,
		"license_key_id":     intent.LicenseKeyID,
		"commerce_base_url":  intent.CommerceBaseURL,
		"byok_env_file":      intent.BYOKEnvFile,
		"byok":               intent.BYOK,
		"api_key":            "sk-should-not-be-here",
	})
	write0600(t, path, string(raw))
	if _, err := LoadIntent(path); err == nil || !strings.Contains(err.Error(), "must not embed secret") {
		t.Fatalf("expected embedded secret rejection, got %v", err)
	}
}

func TestBYOKRequiredAndProductionSyncRejected(t *testing.T) {
	dir, intent := validFixture(t)

	intent.BYOK.APIKeyEnv = ""
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "byok.api_key_env") {
		t.Fatalf("expected BYOK required, got %v", err)
	}

	intent.BYOK.APIKeyEnv = "OPENAI_API_KEY"
	emptyBYOK := filepath.Join(dir, "empty-byok.json")
	write0600(t, emptyBYOK, `{"OPENAI_API_KEY":""}`+"\n")
	intent.BYOKEnvFile = emptyBYOK
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("expected empty key rejection, got %v", err)
	}

	badMode := filepath.Join(dir, "world-readable.json")
	if err := os.WriteFile(badMode, []byte(`{"OPENAI_API_KEY":"sk-test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badMode, 0o644); err != nil {
		t.Fatal(err)
	}
	intent.BYOKEnvFile = badMode
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("expected mode rejection, got %v", err)
	}

	syncPath := filepath.Join(dir, "production-sync", "env.json")
	write0600(t, syncPath, `{"OPENAI_API_KEY":"sk-test"}`+"\n")
	intent.BYOKEnvFile = syncPath
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "production-sync") {
		t.Fatalf("expected production-sync rejection, got %v", err)
	}

	prodName := filepath.Join(dir, "env.production.json")
	write0600(t, prodName, `{"OPENAI_API_KEY":"sk-test"}`+"\n")
	intent.BYOKEnvFile = prodName
	if _, err := ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "env.production.json") {
		t.Fatalf("expected env.production.json rejection, got %v", err)
	}
}

func TestValidateConfigRoutesBYOK(t *testing.T) {
	_, intent := validFixture(t)
	if err := ValidateConfigRoutesBYOK(intent.ConfigFile, intent.BYOK, "default"); err != nil {
		t.Fatal(err)
	}
	intent.BYOK.Model = "gpt-other"
	if err := ValidateConfigRoutesBYOK(intent.ConfigFile, intent.BYOK, "default"); err == nil {
		t.Fatal("expected model mismatch")
	}
}

func TestValidateConfigRequiresAPIKeyExpand(t *testing.T) {
	dir := t.TempDir()
	missingExpand := filepath.Join(dir, "no-expand.yaml")
	write0600(t, missingExpand, `
providers:
  openai:
    dialect: openai-chat
    base_url: https://api.openai.com/v1
    api_key_env: OPENAI_API_KEY
    models:
      gpt-4o-mini:
        model: gpt-4o-mini
models:
  default:
    strategy: weighted
    targets:
      - provider: openai
        model: gpt-4o-mini
        weight: 100
`)
	byok := BYOKSpec{Provider: "openai", APIKeyEnv: "OPENAI_API_KEY", Model: "gpt-4o-mini"}
	if err := ValidateConfigRoutesBYOK(missingExpand, byok, "default"); err == nil || !strings.Contains(err.Error(), "api_key must be") {
		t.Fatalf("expected api_key expand requirement, got %v", err)
	}
}

func TestValidateConfigRequiresUserIDMembership(t *testing.T) {
	dir := t.TempDir()
	byokPath := filepath.Join(dir, "byok.env.json")
	write0600(t, byokPath, `{"OPENAI_API_KEY":"sk-test-customer-key-not-real"}`+"\n")
	bad := filepath.Join(dir, "bad-membership.yaml")
	write0600(t, bad, `
providers:
  openai:
    dialect: openai-chat
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
    api_key_env: OPENAI_API_KEY
    models:
      gpt-4o-mini:
        model: gpt-4o-mini
models:
  default:
    strategy: weighted
    targets:
      - provider: openai
        model: gpt-4o-mini
        weight: 100
users:
  - id: bootstrap-owner
    status: active
projects:
  - id: bootstrap
    status: active
project_memberships:
  - user: bootstrap-owner
    project: bootstrap
    status: active
    role: owner
`)
	byok := BYOKSpec{Provider: "openai", APIKeyEnv: "OPENAI_API_KEY", Model: "gpt-4o-mini"}
	if err := ValidateConfigRoutesBYOK(bad, byok, "default"); err == nil || !strings.Contains(err.Error(), "user_id") {
		t.Fatalf("expected user_id membership requirement, got %v", err)
	}
}

func TestValidateFleetEKSProvisionPaths(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	write0600(t, good, `
server:
  listen: :8080
  logging:
    path: /var/lib/smart-llmrouter/requests.jsonl
  usage_db:
    path: /var/lib/smart-llmrouter/usage.sqlite
  license:
    enabled: true
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
    revocation:
      path: /etc/smart-llmrouter-license/revocations.json
state_path: /var/lib/smart-llmrouter/router-state.json
`)
	if err := ValidateFleetEKSProvisionPaths(good); err != nil {
		t.Fatalf("expected good fleet-eks paths: %v", err)
	}

	nestedState := filepath.Join(dir, "nested-state.yaml")
	write0600(t, nestedState, `
server:
  state_path: /var/lib/smart-llmrouter/router-state.json
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
state_path: /var/lib/smart-llmrouter/router-state.json
`)
	if err := ValidateFleetEKSProvisionPaths(nestedState); err == nil || !strings.Contains(err.Error(), "server.state_path is ignored") {
		t.Fatalf("expected nested server.state_path rejection, got %v", err)
	}

	missingState := filepath.Join(dir, "missing-state.yaml")
	write0600(t, missingState, `
server:
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
`)
	if err := ValidateFleetEKSProvisionPaths(missingState); err == nil || !strings.Contains(err.Error(), "state_path") {
		t.Fatalf("expected missing state_path, got %v", err)
	}

	relativeState := filepath.Join(dir, "relative-state.yaml")
	write0600(t, relativeState, `
server:
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
state_path: ./.state-router.json
`)
	if err := ValidateFleetEKSProvisionPaths(relativeState); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected relative state_path rejection, got %v", err)
	}

	badLicense := filepath.Join(dir, "bad-license.yaml")
	write0600(t, badLicense, `
server:
  license:
    path: /app/config/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
state_path: /var/lib/smart-llmrouter/router-state.json
`)
	if err := ValidateFleetEKSProvisionPaths(badLicense); err == nil || !strings.Contains(err.Error(), "server.license.path") {
		t.Fatalf("expected license path rejection, got %v", err)
	}

	appStateOnly := filepath.Join(dir, "app-state.yaml")
	write0600(t, appStateOnly, `
server:
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /app/state/license-state.json
state_path: /app/state/router-state.json
`)
	if err := ValidateFleetEKSProvisionPaths(appStateOnly); err == nil || !strings.Contains(err.Error(), "/var/lib/smart-llmrouter") {
		t.Fatalf("expected /app/state rejection before rewrite, got %v", err)
	}
}

func TestValidateConfigRetentionForSKU(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "skus.json")
	write0600(t, catalog, `{
  "schema_version": 1,
  "skus": [{
    "sku": "eval-72h",
    "limits": {"max_retention_days": 7}
  }]
}`)
	okCfg := filepath.Join(dir, "ok.yaml")
	write0600(t, okCfg, `
server:
  diagnostics:
    enabled: true
    retention_days: 7
  retention:
    enabled: false
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
state_path: /var/lib/smart-llmrouter/router-state.json
`)
	if err := ValidateConfigRetentionForSKU(okCfg, catalog, "eval-72h"); err != nil {
		t.Fatalf("expected retention ok: %v", err)
	}

	defaultDiag := filepath.Join(dir, "default-diag.yaml")
	write0600(t, defaultDiag, `
server:
  license:
    path: /etc/smart-llmrouter-license/license.json
    state_path: /var/lib/smart-llmrouter/license-state.json
state_path: /var/lib/smart-llmrouter/router-state.json
`)
	if err := ValidateConfigRetentionForSKU(defaultDiag, catalog, "eval-72h"); err == nil || !strings.Contains(err.Error(), "max_retention_days") {
		t.Fatalf("expected default diagnostics 30 rejection, got %v", err)
	}
}

func TestBuildInstanceEnvJSON(t *testing.T) {
	_, intent := validFixture(t)
	raw, err := BuildInstanceEnvJSON(intent.BYOKEnvFile, intent.BYOK, intent.Hostname)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env["OPENAI_API_KEY"] == "" || env["ROUTER_HTTP_REFERER"] != "https://acme.apps.metrum.ai" {
		t.Fatalf("unexpected env: keys=%v referer=%q", len(env), env["ROUTER_HTTP_REFERER"])
	}
	if strings.Contains(string(raw), "STRIPE") {
		t.Fatal("env must not include stripe")
	}
}

func TestSSMNameFromRef(t *testing.T) {
	name, err := ssmNameFromRef("aws-ssm:///metrum/smartrouter/fleet/acme/license-request")
	if err != nil {
		t.Fatal(err)
	}
	if name != "/metrum/smartrouter/fleet/acme/license-request" {
		t.Fatalf("got %q", name)
	}
	if _, err := ssmNameFromRef("aws-secretsmanager:///x"); err == nil {
		t.Fatal("expected ssm-only rejection")
	}
}

func TestExpectedHostname(t *testing.T) {
	if got := ExpectedHostname("Acme"); got != "acme.apps.metrum.ai" {
		t.Fatalf("got %q", got)
	}
}
