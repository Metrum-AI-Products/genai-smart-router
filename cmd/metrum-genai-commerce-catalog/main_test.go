package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogCLIRefusesLiveMode(t *testing.T) {
	bin := buildCatalogCLI(t)
	cmd := exec.Command(bin, "plan", "--mode", "live", "--catalog", filepath.Join("..", "..", "docs", "enterprise-license-skus.json"))
	cmd.Env = append(os.Environ(), "STRIPE_SECRET_KEY=sk_test_dummy")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected live mode to fail")
	}
	if !strings.Contains(string(out), "refuses --mode live") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCatalogCLIRequiresTestKey(t *testing.T) {
	bin := buildCatalogCLI(t)
	cmd := exec.Command(bin, "plan", "--mode", "test", "--catalog", filepath.Join("..", "..", "docs", "enterprise-license-skus.json"))
	cmd.Env = append(os.Environ(), "STRIPE_SECRET_KEY=sk_live_dummy")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-test key to fail")
	}
	if !strings.Contains(string(out), "sk_test_") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func buildCatalogCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "metrum-genai-commerce-catalog")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}
