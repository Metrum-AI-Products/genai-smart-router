// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WrapStatus shells to fleetctl customer status and prints lifecycle state.
func WrapStatus(ctx context.Context, intent Intent) error {
	fleetBin, err := resolveBinary("metrum-genai-smartrouter-fleetctl", "metrum-fleetctl")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, fleetBin,
		"customer", "status",
		"--customer-id", intent.CustomerID,
		"--profile-ref", intent.ProfileRef,
		"--runtime-bundle-ref", intent.RuntimeBundleRef,
		"--license-ref", intent.LicenseRef,
	)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fleetctl customer status: %w", err)
	}
	st := LoadOnboardState(intent.CustomerID)
	return PrintStateJSON(st)
}

// WrapSmoke shells to fleetctl customer smoke.
func WrapSmoke(ctx context.Context, intent Intent, tokenFile, model string) error {
	fleetBin, err := resolveBinary("metrum-genai-smartrouter-fleetctl", "metrum-fleetctl")
	if err != nil {
		return err
	}
	if strings.TrimSpace(tokenFile) == "" {
		tokenFile = intent.TokenOut
	}
	if strings.TrimSpace(model) == "" {
		model = intent.SmokeModel
	}
	args := []string{
		"customer", "smoke",
		"--customer-id", intent.CustomerID,
		"--token-file", tokenFile,
		"--model", model,
	}
	_, err = runCaptureJSON(ctx, fleetBin, args...)
	return err
}

// WrapUpdateConfig publishes a refreshed runtime bundle (and optional YAML patch),
// then signs and redeploys via fleetctl customer create. Operators without source
// get a real rollout from one verb; publish-and-return alone is not acceptance.
func WrapUpdateConfig(ctx context.Context, intent Intent, patchFile string, refreshBYOK bool) error {
	fleetBin, err := resolveBinary("metrum-genai-smartrouter-fleetctl", "metrum-fleetctl")
	if err != nil {
		return err
	}
	if err := ValidateFleetEKSProvisionPaths(intent.ConfigFile); err != nil {
		return err
	}
	if err := ValidateConfigRetentionForSKU(intent.ConfigFile, intent.Catalog, intent.SKU); err != nil {
		return err
	}
	home := fleetCustomerHome(intent.CustomerID)
	envPath := filepath.Join(home, "instance-env.json")
	if refreshBYOK || !fileExists(envPath) {
		envJSON, err := BuildInstanceEnvJSON(intent.BYOKEnvFile, intent.BYOK, intent.Hostname)
		if err != nil {
			return err
		}
		if err := writeMode0600(envPath, envJSON); err != nil {
			return err
		}
	}
	_, err = runCaptureJSON(ctx, fleetBin,
		"customer", "publish-runtime-bundle",
		"--customer-id", intent.CustomerID,
		"--config-file", intent.ConfigFile,
		"--env-file", envPath,
		"--rewrite-paths", "fleet-eks",
	)
	if err != nil {
		return fmt.Errorf("publish-runtime-bundle: %w", err)
	}
	if strings.TrimSpace(patchFile) != "" {
		_, err = runCaptureJSON(ctx, fleetBin,
			"customer", "update-config",
			"--customer-id", intent.CustomerID,
			"--profile-ref", intent.ProfileRef,
			"--runtime-bundle-ref", intent.RuntimeBundleRef,
			"--license-ref", intent.LicenseRef,
			"--patch-file", expandHome(patchFile),
		)
		if err != nil {
			return fmt.Errorf("fleetctl customer update-config: %w", err)
		}
	} else {
		_, err = runCaptureJSON(ctx, fleetBin,
			"customer", "write-manifest",
			"--customer-id", intent.CustomerID,
			"--profile-ref", intent.ProfileRef,
			"--runtime-bundle-ref", intent.RuntimeBundleRef,
			"--license-ref", intent.LicenseRef,
			"--revision-prefix", "cfg",
		)
		if err != nil {
			return fmt.Errorf("fleetctl customer write-manifest: %w", err)
		}
	}
	_, err = runCaptureJSON(ctx, fleetBin,
		"customer", "create",
		"--customer-id", intent.CustomerID,
		"--sign-with-key", intent.SignKey,
	)
	if err != nil {
		return fmt.Errorf("fleetctl customer create (signed update redeploy): %w", err)
	}
	fmt.Println(string(mustJSONBytes(map[string]any{
		"customer_id":  intent.CustomerID,
		"hostname":     intent.Hostname,
		"activation":   "signed-redeploy",
		"refresh_byok": refreshBYOK,
		"patch_file":   strings.TrimSpace(patchFile) != "",
	})))
	return nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// WrapDelete shells to fleetctl customer delete with signed approval.
func WrapDelete(ctx context.Context, intent Intent, confirmFile string) error {
	fleetBin, err := resolveBinary("metrum-genai-smartrouter-fleetctl", "metrum-fleetctl")
	if err != nil {
		return err
	}
	args := []string{
		"customer", "delete",
		"--customer-id", intent.CustomerID,
		"--profile-ref", intent.ProfileRef,
		"--runtime-bundle-ref", intent.RuntimeBundleRef,
		"--license-ref", intent.LicenseRef,
	}
	if strings.TrimSpace(confirmFile) != "" {
		args = append(args, "--confirm-file", expandHome(confirmFile))
	} else {
		args = append(args, "--sign-with-key", intent.SignKey)
	}
	_, err = runCaptureJSON(ctx, fleetBin, args...)
	return err
}

// ExportUsage pulls authenticated admin usage JSON into a mode-0600 directory.
// It never writes BYOK or provider keys.
func ExportUsage(ctx context.Context, intent Intent, tokenFile, outDir string) error {
	outDir = expandHome(outDir)
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("--out-dir is required")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(outDir, 0o700)
	tokenFile = expandHome(tokenFile)
	if tokenFile == "" {
		tokenFile = intent.TokenOut
	}
	if err := requireMode0600(tokenFile, "token-file"); err != nil {
		return err
	}
	tokenRaw, err := os.ReadFile(tokenFile)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(tokenRaw))
	if token == "" {
		return fmt.Errorf("token-file is empty")
	}
	base := "https://" + intent.Hostname
	client := &http.Client{Timeout: 60 * time.Second}
	for _, path := range []string{"/admin/usage", "/admin/reports/overview"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("export %s: %w", path, err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		name := strings.Trim(strings.ReplaceAll(path, "/", "_"), "_") + ".json"
		outPath := filepath.Join(outDir, name)
		meta := map[string]any{
			"path":        path,
			"http_status": resp.StatusCode,
			"customer_id": intent.CustomerID,
			"hostname":    intent.Hostname,
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"bytes":       len(body),
		}
		// Store response body only when JSON-ish; never invent BYOK fields.
		if resp.StatusCode < 300 && len(body) > 0 && body[0] == '{' {
			if err := writeMode0600(outPath, append(body, '\n')); err != nil {
				return err
			}
		} else {
			raw, _ := jsonMarshalIndent(meta)
			if err := writeMode0600(outPath+".meta.json", raw); err != nil {
				return err
			}
			continue
		}
		raw, _ := jsonMarshalIndent(meta)
		_ = writeMode0600(outPath+".meta.json", raw)
	}
	fmt.Println(string(mustJSONBytes(map[string]any{
		"customer_id": intent.CustomerID,
		"out_dir":     outDir,
		"note":        "usage export contains report JSON only; BYOK keys are never exported",
	})))
	return nil
}

func mustJSONBytes(v any) []byte {
	raw, err := jsonMarshalIndent(v)
	if err != nil {
		return []byte("{}\n")
	}
	return raw
}
