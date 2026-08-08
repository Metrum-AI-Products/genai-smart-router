package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCompatibilityCommandOnlyReportsRename(t *testing.T) {
	output, err := exec.Command("go", "run", ".", "deploy").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "renamed to metrum-fleetctl") || strings.Contains(string(output), "tenant-deployments") {
		t.Fatalf("compatibility command must only report rename: %v %s", err, output)
	}
}
