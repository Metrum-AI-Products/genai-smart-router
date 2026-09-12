// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const secretsManagerMaxPayloadBytes = 65536

func customerPublishRuntimeBundle(args []string) {
	fs := flag.NewFlagSet("customer publish-runtime-bundle", flag.ExitOnError)
	customerID := fs.String("customer-id", "", "customer id (required)")
	configFile := fs.String("config-file", "", "local config.yaml (required)")
	envFile := fs.String("env-file", "", "local env.json (required)")
	stripCallers := fs.Bool("strip-callers", false, "clear callers/users/projects before publish")
	rewritePaths := fs.String("rewrite-paths", "", "path rewrite profile (fleet-eks)")
	_ = fs.Parse(args)
	if strings.TrimSpace(*customerID) == "" || strings.TrimSpace(*configFile) == "" || strings.TrimSpace(*envFile) == "" {
		die("publish-runtime-bundle requires --customer-id, --config-file, and --env-file")
	}
	ws := prepareCustomerWorkspace(*customerID)
	configYAML, envJSON, err := loadRuntimeBundleFiles(*configFile, *envFile)
	if err != nil {
		die("%v", err)
	}
	bundle, stats, err := transformRuntimeBundle(configYAML, envJSON, bundleTransformOptions{
		StripCallers: *stripCallers,
		RewritePaths: strings.TrimSpace(*rewritePaths),
		TrimCatalog:  false,
	})
	if err != nil {
		die("%v", err)
	}
	if err := enforceSecretsManagerSizeLimit(bundle); err != nil {
		die("%v", err)
	}
	ctx := context.Background()
	clearFleetSessionCredentials()
	ref, err := publishRuntimeBundle(ctx, ws.CustomerID, bundle)
	if err != nil {
		die("%v", err)
	}
	ws.saveState(map[string]any{
		"customer_id":        ws.CustomerID,
		"runtime_bundle_ref": ref,
	})
	fmt.Println(mustJSON(map[string]any{
		"published_runtime_bundle_ref": ref,
		"config_sha256":                stats.ConfigSHA256,
		"bundle_bytes":                 stats.BundleBytes,
		"model_group_count":            stats.ModelGroupCount,
	}))
}

func customerPrepareRuntimeBundle(args []string) {
	fs := flag.NewFlagSet("customer prepare-runtime-bundle", flag.ExitOnError)
	configFile := fs.String("config-file", "", "source config.yaml (required)")
	envFile := fs.String("env-file", "", "source env.json (required)")
	configOut := fs.String("config-out", "", "prepared config.yaml output (required)")
	envOut := fs.String("env-out", "", "prepared env.json output (required)")
	stripCallers := fs.Bool("strip-callers", true, "clear callers/users/projects")
	rewritePaths := fs.String("rewrite-paths", "fleet-eks", "path rewrite profile (fleet-eks)")
	trimCatalog := fs.Bool("trim-catalog-only", true, "drop unreferenced provider catalog models")
	_ = fs.Parse(args)
	if strings.TrimSpace(*configFile) == "" || strings.TrimSpace(*envFile) == "" || strings.TrimSpace(*configOut) == "" || strings.TrimSpace(*envOut) == "" {
		die("prepare-runtime-bundle requires --config-file, --env-file, --config-out, and --env-out")
	}
	configYAML, envJSON, err := loadRuntimeBundleFiles(*configFile, *envFile)
	if err != nil {
		die("%v", err)
	}
	bundle, stats, err := transformRuntimeBundle(configYAML, envJSON, bundleTransformOptions{
		StripCallers: *stripCallers,
		RewritePaths: strings.TrimSpace(*rewritePaths),
		TrimCatalog:  *trimCatalog,
	})
	if err != nil {
		die("%v", err)
	}
	if err := enforceSecretsManagerSizeLimit(bundle); err != nil {
		die("%v", err)
	}
	writeMode0600(expandHome(*configOut), []byte(bundle.ConfigYAML))
	writeMode0600(expandHome(*envOut), []byte(bundle.EnvJSON))
	fmt.Println(mustJSON(map[string]any{
		"config_out":        expandHome(*configOut),
		"env_out":           expandHome(*envOut),
		"config_sha256":     stats.ConfigSHA256,
		"bundle_bytes":      stats.BundleBytes,
		"model_group_count": stats.ModelGroupCount,
	}))
}

type bundleTransformOptions struct {
	StripCallers bool
	RewritePaths string
	TrimCatalog  bool
}

type bundleTransformStats struct {
	ConfigSHA256    string
	BundleBytes     int
	ModelGroupCount int
}

func loadRuntimeBundleFiles(configPath, envPath string) (string, string, error) {
	configPath = expandHome(configPath)
	envPath = expandHome(envPath)
	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", fmt.Errorf("read config-file: %w", err)
	}
	envRaw, err := os.ReadFile(envPath)
	if err != nil {
		return "", "", fmt.Errorf("read env-file: %w", err)
	}
	return string(configRaw), string(envRaw), nil
}

func transformRuntimeBundle(configYAML, envJSON string, opts bundleTransformOptions) (runtimeBundle, bundleTransformStats, error) {
	var cfg any
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err != nil {
		return runtimeBundle{}, bundleTransformStats{}, fmt.Errorf("parse config.yaml: %w", err)
	}
	root, ok := asStringMap(cfg)
	if !ok {
		return runtimeBundle{}, bundleTransformStats{}, fmt.Errorf("config.yaml must be a mapping")
	}
	if opts.StripCallers {
		root["callers"] = []any{}
		delete(root, "users")
		delete(root, "projects")
		delete(root, "project_memberships")
	}
	if opts.TrimCatalog {
		trimCatalogOnlyModels(root)
		stripProviderNotes(root)
	}
	if opts.RewritePaths != "" {
		if err := rewriteRuntimePaths(root, opts.RewritePaths); err != nil {
			return runtimeBundle{}, bundleTransformStats{}, err
		}
		envJSON = rewriteRuntimePathStrings(envJSON, opts.RewritePaths)
	}
	outYAML, err := yaml.Marshal(root)
	if err != nil {
		return runtimeBundle{}, bundleTransformStats{}, fmt.Errorf("encode config.yaml: %w", err)
	}
	bundle := runtimeBundle{ConfigYAML: string(outYAML), EnvJSON: strings.TrimSpace(envJSON)}
	if err := validateBundleShapes(bundle); err != nil {
		return runtimeBundle{}, bundleTransformStats{}, err
	}
	payload, err := json.Marshal(map[string]string{
		"config.yaml": bundle.ConfigYAML,
		"env.json":    bundle.EnvJSON,
	})
	if err != nil {
		return runtimeBundle{}, bundleTransformStats{}, err
	}
	sum := sha256.Sum256([]byte(bundle.ConfigYAML))
	return bundle, bundleTransformStats{
		ConfigSHA256:    hex.EncodeToString(sum[:])[:16],
		BundleBytes:     len(payload),
		ModelGroupCount: countModelGroups(root),
	}, nil
}

func enforceSecretsManagerSizeLimit(bundle runtimeBundle) error {
	payload, err := json.Marshal(map[string]string{
		"config.yaml": bundle.ConfigYAML,
		"env.json":    bundle.EnvJSON,
	})
	if err != nil {
		return err
	}
	if len(payload) <= secretsManagerMaxPayloadBytes {
		return nil
	}
	over := len(payload) - secretsManagerMaxPayloadBytes
	return fmt.Errorf("runtime bundle exceeds Secrets Manager limit (%d bytes, over by %d); trim catalog-only models with customer prepare-runtime-bundle --trim-catalog-only or reduce config size", len(payload), over)
}

func rewriteRuntimePaths(root map[string]any, profile string) error {
	switch profile {
	case "fleet-eks":
		rewritePathValues(root, map[string]string{
			"/app/state": "/var/lib/smart-llmrouter",
			"/app/logs":  "/var/lib/smart-llmrouter",
		})
		return nil
	default:
		return fmt.Errorf("unsupported rewrite-paths profile %q", profile)
	}
}

func rewriteRuntimePathStrings(raw, profile string) string {
	switch profile {
	case "fleet-eks":
		raw = strings.ReplaceAll(raw, "/app/state", "/var/lib/smart-llmrouter")
		return strings.ReplaceAll(raw, "/app/logs", "/var/lib/smart-llmrouter")
	default:
		return raw
	}
}

func rewritePathValues(value any, replacements map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if s, ok := child.(string); ok {
				for from, to := range replacements {
					if s == from || strings.HasPrefix(s, from+"/") {
						v[key] = to + strings.TrimPrefix(s, from)
					}
				}
			} else {
				rewritePathValues(child, replacements)
			}
		}
	case map[any]any:
		for key, child := range v {
			rewritePathValues(child, replacements)
			_ = key
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				for from, to := range replacements {
					if s == from || strings.HasPrefix(s, from+"/") {
						v[i] = to + strings.TrimPrefix(s, from)
					}
				}
			} else {
				rewritePathValues(child, replacements)
			}
		}
	}
}

func trimCatalogOnlyModels(root map[string]any) {
	referenced := referencedProviderModels(root)
	providers, ok := asStringMap(root["providers"])
	if !ok {
		return
	}
	for providerName, providerAny := range providers {
		provider, ok := asStringMap(providerAny)
		if !ok {
			continue
		}
		models, ok := asStringMap(provider["models"])
		if !ok {
			continue
		}
		for modelRef := range models {
			key := providerName + ":" + modelRef
			if !referenced[key] {
				delete(models, modelRef)
			}
		}
		if len(models) == 0 {
			delete(provider, "models")
		}
	}
}

func stripProviderNotes(root map[string]any) {
	providers, ok := asStringMap(root["providers"])
	if !ok {
		return
	}
	for _, providerAny := range providers {
		provider, ok := asStringMap(providerAny)
		if !ok {
			continue
		}
		models, ok := asStringMap(provider["models"])
		if !ok {
			continue
		}
		for _, modelAny := range models {
			model, ok := asStringMap(modelAny)
			if !ok {
				continue
			}
			delete(model, "validation_notes")
			delete(model, "pricing_notes")
		}
	}
}

func referencedProviderModels(root map[string]any) map[string]bool {
	out := map[string]bool{}
	modelsRoot, ok := asStringMap(root["models"])
	if !ok {
		return out
	}
	for _, groupAny := range modelsRoot {
		group, ok := asStringMap(groupAny)
		if !ok {
			continue
		}
		targets, ok := asAnySlice(group["targets"])
		if !ok {
			continue
		}
		for _, targetAny := range targets {
			target, ok := asStringMap(targetAny)
			if !ok {
				continue
			}
			provider, _ := target["provider"].(string)
			modelRef, _ := target["model_ref"].(string)
			if provider != "" && modelRef != "" {
				out[provider+":"+modelRef] = true
			}
		}
	}
	return out
}

func countModelGroups(root map[string]any) int {
	modelsRoot, ok := asStringMap(root["models"])
	if !ok {
		return 0
	}
	return len(modelsRoot)
}

func clearFleetSessionCredentials() {
	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		_ = os.Unsetenv(key)
	}
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			die("resolve home directory: %v", err)
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func modelGroupsFromConfig(configYAML string) ([]string, error) {
	var cfg any
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err != nil {
		return nil, err
	}
	root, ok := asStringMap(cfg)
	if !ok {
		return nil, fmt.Errorf("config.yaml must be a mapping")
	}
	modelsRoot, ok := asStringMap(root["models"])
	if !ok {
		return nil, fmt.Errorf("config.yaml has no models mapping")
	}
	out := make([]string, 0, len(modelsRoot))
	for name := range modelsRoot {
		out = append(out, name)
	}
	return out, nil
}
