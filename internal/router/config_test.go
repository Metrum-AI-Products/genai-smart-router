package router

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvJSONSetsMissingValuesOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	if err := os.WriteFile(path, []byte(`{"ROUTER_TEST_ENV":"from_file","ROUTER_KEEP_ENV":"from_file"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTER_TEST_ENV", "")
	t.Setenv("ROUTER_KEEP_ENV", "existing")
	os.Unsetenv("ROUTER_TEST_ENV")
	if err := loadEnvJSON(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ROUTER_TEST_ENV"); got != "from_file" {
		t.Fatalf("ROUTER_TEST_ENV=%q", got)
	}
	if got := os.Getenv("ROUTER_KEEP_ENV"); got != "existing" {
		t.Fatalf("ROUTER_KEEP_ENV overwritten: %q", got)
	}
}

func TestProviderModelRefsResolveAndOverride(t *testing.T) {
	cfg := minimalConfig(t)
	provider := cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"small": {Model: "mock-small", Weight: 99, Tier: "cheap", RPM: 10},
		"large": {Model: "mock-large", Weight: 99, Tier: "heavy"},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", ModelRef: "small"},
		{Provider: "mock", ModelRef: "large", Weight: 9},
		{Provider: "mock", Model: "direct-model", Weight: 1},
	}}
	cfg.Models["fast"] = ModelGroup{Strategy: "weighted", Targets: []Target{
		{Provider: "mock", ModelRef: "small", Weight: 3},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	targets := cfg.Models["default"].Targets
	if targets[0].Model != "mock-small" || targets[0].Weight != 0 || targets[0].Tier != "cheap" || targets[0].RPM != 10 {
		t.Fatalf("small ref not resolved: %#v", targets[0])
	}
	if targets[1].Model != "mock-large" || targets[1].Weight != 9 || targets[1].Tier != "heavy" {
		t.Fatalf("large ref override not resolved: %#v", targets[1])
	}
	if targets[2].Model != "direct-model" {
		t.Fatalf("direct model target changed: %#v", targets[2])
	}
	if got := cfg.Models["fast"].Targets[0].Weight; got != 3 {
		t.Fatalf("group-local target weight = %d, want 3", got)
	}
}

func TestMissingProviderModelRefFailsValidation(t *testing.T) {
	cfg := minimalConfig(t)
	provider := cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"known": {Model: "mock-known"},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "missing"}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown model_ref missing") {
		t.Fatalf("expected missing model_ref error, got %v", err)
	}
}

func TestWeightedOrderTreatsOmittedTargetWeightAsOne(t *testing.T) {
	targets := weightedOrder([]Target{{Provider: "mock", Model: "only"}})
	if len(targets) != 1 || targets[0].Provider != "mock" || targets[0].Model != "only" {
		t.Fatalf("weighted order with omitted weight = %#v", targets)
	}
}

func TestExampleConfigDefaultIncludesLatestCodingTargets(t *testing.T) {
	raw, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	standardSum := sha256.Sum256([]byte("rtr_example_standard_test"))
	codingSum := sha256.Sum256([]byte("rtr_example_coding_test"))
	text := strings.ReplaceAll(string(raw), "REPLACE_WITH_SHA256_HEX_OF_STANDARD_ROUTER_TOKEN", hex.EncodeToString(standardSum[:]))
	text = strings.ReplaceAll(text, "REPLACE_WITH_SHA256_HEX_OF_CODING_ROUTER_TOKEN", hex.EncodeToString(codingSum[:]))
	text = strings.ReplaceAll(text, "script: scripts/router.ts", "script: ../../scripts/router.ts")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultGroupTargets(t, cfg.Models["default"])
	assertAnthropicCompatibleProvider(t, cfg.Provider["minimax_anthropic"], "m3", "MiniMax-M3")
	assertAnthropicCompatibleProvider(t, cfg.Provider["kimi_anthropic"], "kimi-k2.7-code", "kimi-k2.7-code")
	for _, name := range []string{"small", "medium", "high"} {
		group, ok := cfg.Models[name]
		if !ok {
			t.Fatalf("example config missing %s model group", name)
		}
		if len(group.Targets) == 0 {
			t.Fatalf("example config %s model group has no targets", name)
		}
	}
	for name, group := range cfg.Models {
		if name == "agent-tools-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "minimax" || group.Targets[0].Model != "MiniMax-M3" || group.Targets[0].Dialect != "openai-responses" {
				t.Fatalf("example config agent-tools-smoke=%#v, want static minimax MiniMax-M3 responses", group)
			}
			continue
		}
		if name == "claude-tools-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "minimax_anthropic" || group.Targets[0].Model != "MiniMax-M3" {
				t.Fatalf("example config claude-tools-smoke=%#v, want static minimax Anthropic-compatible MiniMax-M3", group)
			}
			continue
		}
		if group.Strategy != "weighted" {
			t.Fatalf("example config group %s strategy=%q want weighted", name, group.Strategy)
		}
		assertActiveGroupPolicy(t, name, group)
	}
	wantAllows := map[string][]string{
		"standard-dev": {"default", "fast", "small"},
		"coding-dev":   {"default", "fast", "big-coder", "small", "medium", "high", "agent-tools-smoke", "claude-tools-smoke"},
	}
	for _, caller := range cfg.Callers {
		want, ok := wantAllows[caller.ID]
		if !ok {
			continue
		}
		if strings.Join(caller.Allow, ",") != strings.Join(want, ",") {
			t.Fatalf("caller %s allow=%v want %v", caller.ID, caller.Allow, want)
		}
		delete(wantAllows, caller.ID)
	}
	if len(wantAllows) != 0 {
		t.Fatalf("example config missing caller profiles: %#v", wantAllows)
	}
}

func assertDefaultGroupTargets(t *testing.T, defaultGroup ModelGroup) {
	t.Helper()
	want := map[string]string{
		"minimax:MiniMax-M3":                          "MiniMax-M3",
		"minimax:MiniMax-M2.7-highspeed":              "MiniMax-M2.7-highspeed",
		"kimi:kimi-k2.7-code":                         "kimi-k2.7-code",
		"openrouter:kwaipilot/kat-coder-pro-v2:nitro": "kwaipilot/kat-coder-pro-v2:nitro",
		"openrouter:nvidia/nemotron-3-nano-30b-a3b":   "nvidia/nemotron-3-nano-30b-a3b",
		"openrouter:inception/mercury-2":              "inception/mercury-2",
		"openrouter:inclusionai/ling-2.6-flash":       "inclusionai/ling-2.6-flash",
	}
	for name, model := range want {
		found := false
		for _, target := range defaultGroup.Targets {
			if target.Provider+":"+target.Model == name && target.Model == model {
				found = true
				if !target.ToolOnly && target.Weight <= 0 {
					t.Fatalf("%s target resolved with non-positive weight: %#v", name, target)
				}
				break
			}
		}
		if !found {
			t.Fatalf("default group missing resolved target %s; targets=%#v", name, defaultGroup.Targets)
		}
	}
}

func assertAnthropicCompatibleProvider(t *testing.T, provider ProviderConfig, ref, model string) {
	t.Helper()
	if provider.Dialect != "anthropic" || normalizeAuthScheme(provider.AuthScheme) != "bearer" || provider.Models[ref].Model != model {
		t.Fatalf("provider not configured for Anthropic-compatible bearer skin: %#v", provider)
	}
}

func assertActiveGroupPolicy(t *testing.T, name string, group ModelGroup) {
	t.Helper()
	totalWeight := 0
	m3Weight := 0
	deepSeekWeight := 0
	kimiWeight := 0
	normalTargets := 0
	codexToolTarget := false
	claudeMiniMaxToolTarget := false
	claudeKimiToolTarget := false
	for _, target := range group.Targets {
		if activeTargetUsesDisallowedModel(target) {
			t.Fatalf("example config group %s has disallowed active target %#v", name, target)
		}
		if target.ToolOnly {
			if target.Provider == "minimax" && target.Model == "MiniMax-M3" && target.Dialect == "openai-responses" {
				codexToolTarget = true
			}
			if target.Provider == "minimax_anthropic" && target.Model == "MiniMax-M3" {
				claudeMiniMaxToolTarget = true
			}
			if target.Provider == "kimi_anthropic" && target.Model == "kimi-k2.7-code" && target.DefaultThinking["type"] == "enabled" {
				claudeKimiToolTarget = true
			}
			continue
		}
		normalTargets++
		totalWeight += target.Weight
		if target.Provider == "minimax" && target.Model == "MiniMax-M3" {
			m3Weight += target.Weight
		}
		if target.Provider == "openrouter" && target.Model == "deepseek/deepseek-v4-flash:nitro" {
			deepSeekWeight += target.Weight
		}
		if target.Provider == "kimi" && target.Model == "kimi-k2.7-code" {
			kimiWeight += target.Weight
		}
	}
	if !codexToolTarget || !claudeMiniMaxToolTarget || !claudeKimiToolTarget {
		t.Fatalf("example config group %s missing tool-only targets codex=%v minimax=%v kimi=%v", name, codexToolTarget, claudeMiniMaxToolTarget, claudeKimiToolTarget)
	}
	if name == "big-coder" {
		if normalTargets != 3 || totalWeight != 100 || m3Weight != 50 || kimiWeight != 30 || deepSeekWeight != 20 {
			t.Fatalf("example config big-coder weights m3=%d kimi=%d deepseek=%d total=%d normal_targets=%d, want 50/30/20 over 3 normal targets", m3Weight, kimiWeight, deepSeekWeight, totalWeight, normalTargets)
		}
		return
	}
	if totalWeight == 0 || m3Weight*10 != totalWeight*3 || deepSeekWeight*10 != totalWeight*6 {
		t.Fatalf("example config group %s weights m3=%d deepseek=%d total=%d, want 30%%/60%%", name, m3Weight, deepSeekWeight, totalWeight)
	}
}

func activeTargetUsesDisallowedModel(target Target) bool {
	if target.Provider == "openai" || target.Provider == "anthropic" {
		return true
	}
	needle := strings.ToLower(target.Model + "/" + target.ModelRef)
	for _, bad := range []string{"anthropic/", "claude", "gpt-5", "gpt54", "gpt55", "openai/gpt"} {
		if strings.Contains(needle, bad) {
			return true
		}
	}
	return false
}

func minimalConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Server: ServerConfig{
			Cache: CacheConfig{Enabled: true},
			Logging: LoggingConfig{
				Path: filepath.Join(t.TempDir(), "requests.jsonl"),
			},
		},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Provider: map[string]ProviderConfig{
			"mock": {BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "secret", APIKeyEnv: "MOCK_API_KEY", KeyID: "mock-key"},
		},
		Models: map[string]ModelGroup{
			"default": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			TokenSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Allow:       []string{"default"},
		}},
	}
}
