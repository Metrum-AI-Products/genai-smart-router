// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceServerRequiresTestKey(t *testing.T) {
	bin := buildCommerce(t)
	cmd := exec.Command(bin, "-catalog", filepath.Join("..", "..", "docs", "enterprise-license-skus.json"))
	cmd.Env = append(os.Environ(),
		"STRIPE_SECRET_KEY=sk_live_dummy",
		"STRIPE_WEBHOOK_SECRET=whsec_dummy",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected live key refusal")
	}
	if !strings.Contains(string(out), "sk_test_") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func buildCommerce(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "metrum-genai-commerce")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}
