// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package commerce_test

import (
	"os"
	"path/filepath"
	"testing"

	"smart-llmrouter/internal/commerce"
)

func TestLoadCommerceEnvJSONShellWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commerce.env.json")
	if err := os.WriteFile(path, []byte(`{"STRIPE_SECRET_KEY":"sk_test_from_file","COMMERCE_FLEET_MODE":"fake"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_from_shell")
	_ = os.Unsetenv("COMMERCE_FLEET_MODE")

	if err := commerce.LoadCommerceEnvJSON(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("STRIPE_SECRET_KEY"); got != "sk_test_from_shell" {
		t.Fatalf("shell should win, got %q", got)
	}
	if got := os.Getenv("COMMERCE_FLEET_MODE"); got != "fake" {
		t.Fatalf("file value expected for unset key, got %q", got)
	}
}

func TestFleetRunnerFromEnvModes(t *testing.T) {
	t.Setenv("COMMERCE_FLEET_ENABLED", "")
	if commerce.FleetRunnerFromEnv() != nil {
		t.Fatal("disabled should return nil")
	}

	t.Setenv("COMMERCE_FLEET_ENABLED", "1")
	t.Setenv("COMMERCE_FLEET_MODE", "fake")
	t.Setenv("COMMERCE_FLEET_PROFILE_REF", "")
	t.Setenv("COMMERCE_FLEET_LICENSE_REF", "")
	t.Setenv("COMMERCE_FLEET_CONFIG_FILE", "")
	t.Setenv("COMMERCE_FLEET_ENV_FILE", "")
	t.Setenv("COMMERCE_FLEET_SIGN_KEY", "")
	if _, ok := commerce.FleetRunnerFromEnv().(*commerce.FakeFleetRunner); !ok {
		t.Fatal("mode=fake should use FakeFleetRunner")
	}

	t.Setenv("COMMERCE_FLEET_MODE", "shell")
	t.Setenv("COMMERCE_FLEET_PROFILE_REF", "prof")
	t.Setenv("COMMERCE_FLEET_LICENSE_REF", "lic")
	t.Setenv("COMMERCE_FLEET_CONFIG_FILE", "/tmp/cfg.yaml")
	t.Setenv("COMMERCE_FLEET_ENV_FILE", "/tmp/env.json")
	t.Setenv("COMMERCE_FLEET_SIGN_KEY", "/tmp/sign.key")
	if _, ok := commerce.FleetRunnerFromEnv().(*commerce.ShellFleetRunner); !ok {
		t.Fatal("mode=shell with refs should use ShellFleetRunner")
	}

	t.Setenv("COMMERCE_FLEET_SIGN_KEY", "")
	if _, ok := commerce.FleetRunnerFromEnv().(*commerce.FakeFleetRunner); !ok {
		t.Fatal("incomplete shell refs should fall back to FakeFleetRunner")
	}
}
