// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// DefaultCommerceEnvFile is the cwd-relative commerce secrets file.
const DefaultCommerceEnvFile = "commerce.env.json"

// LoadCommerceEnvJSON loads key/value pairs from path into the process
// environment when the key is not already set (shell wins). Missing file is OK.
func LoadCommerceEnvJSON(path string) error {
	if strings.TrimSpace(path) == "" {
		path = DefaultCommerceEnvFile
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	env := map[string]string{}
	if err := json.Unmarshal(raw, &env); err != nil {
		// Also accept map[string]any with numeric ports etc.
		var anyMap map[string]any
		if err2 := json.Unmarshal(raw, &anyMap); err2 != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		for k, v := range anyMap {
			env[k] = fmt.Sprint(v)
		}
	}
	for k, v := range env {
		if !validCommerceEnvName(k) {
			return fmt.Errorf("load %s: invalid env key %q", path, k)
		}
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// LoadCommerceEnvFromCwd loads ./commerce.env.json (shell wins).
func LoadCommerceEnvFromCwd() error {
	return LoadCommerceEnvJSON(filepath.Join(".", DefaultCommerceEnvFile))
}

func validCommerceEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

var fleetCustomerSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

// SanitizeFleetCustomerID returns a Fleet-safe customer id from alias, or c{id}.
func SanitizeFleetCustomerID(alias string, entitlementID uint) string {
	cleaned := strings.ToLower(strings.TrimSpace(alias))
	cleaned = strings.ReplaceAll(cleaned, "_", "-")
	cleaned = strings.ReplaceAll(cleaned, " ", "-")
	cleaned = fleetCustomerSanitizer.ReplaceAllString(cleaned, "")
	cleaned = strings.Trim(cleaned, "-")
	for strings.Contains(cleaned, "--") {
		cleaned = strings.ReplaceAll(cleaned, "--", "-")
	}
	if cleaned == "" || cleaned == "anon" {
		return fmt.Sprintf("c%d", entitlementID)
	}
	// Keep Fleet ids bounded.
	if len(cleaned) > 48 {
		cleaned = cleaned[:48]
		cleaned = strings.Trim(cleaned, "-")
	}
	return cleaned
}

// ShellFleetRefsComplete reports whether required shell Fleet bootstrap refs are set.
func ShellFleetRefsComplete(profile, license, configFile, envFile, signKey string) bool {
	return strings.TrimSpace(profile) != "" &&
		strings.TrimSpace(license) != "" &&
		strings.TrimSpace(configFile) != "" &&
		strings.TrimSpace(envFile) != "" &&
		strings.TrimSpace(signKey) != ""
}

// FleetRunnerFromEnv selects FakeFleetRunner or ShellFleetRunner from commerce env.
// When COMMERCE_FLEET_ENABLED is not "1", returns nil (fulfillment stays queued).
// When enabled: fake if mode=fake OR shell refs incomplete; shell only when
// mode=shell (or empty with complete refs) and all refs are set.
func FleetRunnerFromEnv() FleetRunner {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("COMMERCE_FLEET_ENABLED")), "1") {
		return nil
	}
	profile := strings.TrimSpace(os.Getenv("COMMERCE_FLEET_PROFILE_REF"))
	license := strings.TrimSpace(os.Getenv("COMMERCE_FLEET_LICENSE_REF"))
	configFile := strings.TrimSpace(os.Getenv("COMMERCE_FLEET_CONFIG_FILE"))
	envFile := strings.TrimSpace(os.Getenv("COMMERCE_FLEET_ENV_FILE"))
	signKey := strings.TrimSpace(os.Getenv("COMMERCE_FLEET_SIGN_KEY"))
	refsOK := ShellFleetRefsComplete(profile, license, configFile, envFile, signKey)
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("COMMERCE_FLEET_MODE")))

	useShell := false
	switch mode {
	case "fake":
		useShell = false
	case "shell":
		useShell = refsOK
	default:
		// Empty / unknown: prefer shell only when refs are complete.
		useShell = refsOK
	}
	if !useShell {
		return NewFakeFleetRunner()
	}
	return &ShellFleetRunner{
		Binary:           firstNonEmptyEnv(os.Getenv("COMMERCE_FLEET_BINARY"), "metrum-genai-smartrouter-fleetctl"),
		ProfileRef:       profile,
		LicenseRef:       license,
		ConfigFile:       configFile,
		EnvFile:          envFile,
		SignWithKey:      signKey,
		OwnerUser:        firstNonEmptyEnv(os.Getenv("COMMERCE_FLEET_OWNER_USER"), "commerce"),
		Project:          firstNonEmptyEnv(os.Getenv("COMMERCE_FLEET_PROJECT"), "sandbox"),
		TokenOutTemplate: firstNonEmptyEnv(os.Getenv("COMMERCE_FLEET_TOKEN_OUT"), "/tmp/commerce-token-%s.txt"),
		RewritePaths:     "fleet-eks",
	}
}

func firstNonEmptyEnv(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
