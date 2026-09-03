// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandVersionFlags(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	commands := []string{"router", "router-token-gen", "router-usage-report"}
	for _, name := range commands {
		name := name
		t.Run(name, func(t *testing.T) {
			outPath := filepath.Join(binDir, name)
			build := exec.Command("go", "build",
				"-ldflags", "-X smart-llmrouter/internal/buildinfo.Version=test-version -X smart-llmrouter/internal/buildinfo.Commit=test-commit -X smart-llmrouter/internal/buildinfo.BuildDate=2026-06-17T05:00:00Z",
				"-o", outPath,
				"./cmd/"+name,
			)
			build.Dir = root
			if raw, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build %s: %v\n%s", name, err, raw)
			}
			run := exec.Command(outPath, "--version")
			raw, err := run.CombinedOutput()
			if err != nil {
				t.Fatalf("%s --version: %v\n%s", name, err, raw)
			}
			text := string(raw)
			for _, want := range []string{"test-version", "test-commit", "2026-06-17T05:00:00Z"} {
				if !strings.Contains(text, want) {
					t.Fatalf("%s --version missing %q: %s", name, want, text)
				}
			}
		})
	}
}
