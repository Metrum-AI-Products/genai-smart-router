package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCallerGenerateWritesTokenOnceAndNeverPrintsIt(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "caller.token")
	run := func() ([]byte, error) {
		return exec.Command("go", "run", ".", "callers", "generate", "--owner-user", "operator", "--project", "example", "--allow", "default", "--token-out", tokenPath).CombinedOutput()
	}
	output, err := run()
	if err != nil {
		t.Fatalf("generate: %v: %s", err, output)
	}
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode=%o, want 600", info.Mode().Perm())
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil || !strings.HasPrefix(string(token), "rtr_metrum_") {
		t.Fatalf("token not written once: %v %q", err, token)
	}
	if strings.Contains(string(output), strings.TrimSpace(string(token))) || !strings.Contains(string(output), "configuration-controller-required") {
		t.Fatalf("unsafe or incomplete generate output: %s", output)
	}
	if second, secondErr := run(); secondErr == nil || !strings.Contains(string(second), "file exists") {
		t.Fatalf("second generate did not fail closed: %v %s", secondErr, second)
	}
}

func TestCustomerConfigAndModelReadCommands(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"config.example.yaml", "env.example.json"} {
		body, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		target := name
		if name == "config.example.yaml" {
			target = "config.yaml"
			body = []byte(strings.NewReplacer(
				"REPLACE_WITH_SHA256_HEX_OF_STANDARD_ROUTER_TOKEN", strings.Repeat("a", 64),
				"REPLACE_WITH_SHA256_HEX_OF_CODING_ROUTER_TOKEN", strings.Repeat("b", 64),
				"REPLACE_WITH_SHA256_HEX_OF_METRICS_ADMIN_ROUTER_TOKEN", strings.Repeat("c", 64),
				"REPLACE_WITH_SHA256_HEX_OF_CONTENT_ADMIN_ROUTER_TOKEN", strings.Repeat("d", 64),
			).Replace(string(body)))
		} else {
			target = "env.json"
		}
		if err := os.WriteFile(filepath.Join(dir, target), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, "config.yaml")
	for _, command := range [][]string{{"config", "validate"}, {"config", "diff", "--from", configPath, "--to", configPath}, {"status"}, {"models", "list"}} {
		args := append([]string{"run", "."}, command...)
		if !(command[0] == "config" && command[1] == "diff") {
			args = append(args, "--config", configPath)
		}
		output, err := exec.Command("go", args...).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "\"schema\"") {
			t.Fatalf("%v failed: %v %s", command, err, output)
		}
	}
}

func TestCustomerCLIFailsClosedForFleetAuthority(t *testing.T) {
	for _, command := range [][]string{{"deploy"}, {"config", "activate"}, {"license", "sign"}, {"keys", "rotate"}} {
		args := append([]string{"run", "."}, command...)
		output, err := exec.Command("go", args...).CombinedOutput()
		if err == nil {
			t.Fatalf("%v did not fail closed: %v %s", command, err, output)
		}
	}
}
