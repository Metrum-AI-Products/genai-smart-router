// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func jsonMarshalIndent(v any) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func writeMode0600(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func resolveBinary(names ...string) (string, error) {
	if dir := strings.TrimSpace(os.Getenv("METRUM_FLEET_BIN_DIR")); dir != "" {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if isExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if isExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("missing binary %s; install from a release package or set METRUM_FLEET_BIN_DIR", strings.Join(names, "|"))
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode()&0o111 != 0
}

func runQuiet(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Never include full output: may sit near token/license paths.
		_ = out
		return fmt.Errorf("%s failed: %w", filepath.Base(bin), err)
	}
	return nil
}

func runCaptureJSON(ctx context.Context, bin string, args ...string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = out
		return nil, fmt.Errorf("%s failed: %w", filepath.Base(bin), err)
	}
	trimmed := strings.TrimSpace(string(out))
	lines := strings.Split(trimmed, "\n")
	var candidate string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			candidate = line
			break
		}
	}
	if candidate == "" {
		return map[string]any{"ok": true}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
		return map[string]any{"ok": true}, nil
	}
	return parsed, nil
}
