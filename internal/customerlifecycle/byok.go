// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateBYOKFile fails closed when BYOK is missing, insecure, empty, or looks like
// a Metrum production-sync env path. Never logs key values.
func ValidateBYOKFile(path string, byok BYOKSpec) error {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("byok_env_file is required")
	}
	if err := rejectMetrumProductionSyncPath(path); err != nil {
		return err
	}
	if strings.TrimSpace(byok.Provider) == "" || strings.TrimSpace(byok.APIKeyEnv) == "" || strings.TrimSpace(byok.Model) == "" {
		return fmt.Errorf("byok.provider, byok.api_key_env, and byok.model are required")
	}
	if err := requireMode0600(path, "byok_env_file"); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read byok_env_file: %w", err)
	}
	var env map[string]string
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("byok_env_file must be a JSON object of string values")
	}
	key := strings.TrimSpace(env[byok.APIKeyEnv])
	if key == "" {
		return fmt.Errorf("byok_env_file missing non-empty %s", byok.APIKeyEnv)
	}
	return nil
}

func rejectMetrumProductionSyncPath(path string) error {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))
	if strings.Contains(lower, "production-sync") {
		return fmt.Errorf("byok_env_file must not use Metrum production-sync env paths")
	}
	if base == "env.production.json" || strings.Contains(lower, "env.production.json") {
		return fmt.Errorf("byok_env_file must not use env.production.json")
	}
	return nil
}

// BuildInstanceEnvJSON builds instance-only env.json from BYOK + ROUTER_HTTP_REFERER.
// It never copies production-sync wholesale; only keys present in the BYOK file are kept.
func BuildInstanceEnvJSON(byokPath string, byok BYOKSpec, hostname string) ([]byte, error) {
	if err := ValidateBYOKFile(byokPath, byok); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(expandHome(byokPath))
	if err != nil {
		return nil, err
	}
	var src map[string]string
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("byok_env_file invalid JSON: %w", err)
	}
	out := map[string]string{}
	for k, v := range src {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		// Refuse commerce/ops secrets if accidentally present in a BYOK file.
		upper := strings.ToUpper(k)
		if strings.Contains(upper, "STRIPE") || strings.Contains(upper, "RESTIC") ||
			strings.Contains(upper, "COMMERCE_ADMIN") || strings.HasPrefix(upper, "COMMERCE_FLEET") {
			return nil, fmt.Errorf("byok_env_file must not contain commerce/ops secret %s", k)
		}
		out[k] = v
	}
	if strings.TrimSpace(out[byok.APIKeyEnv]) == "" {
		return nil, fmt.Errorf("byok_env_file missing %s", byok.APIKeyEnv)
	}
	out["ROUTER_HTTP_REFERER"] = "https://" + strings.TrimSpace(hostname)
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
