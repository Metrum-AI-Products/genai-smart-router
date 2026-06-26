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
		Issuer:        "metrum-ai",
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
