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
		"small": {
			Model:                              "mock-small",
			Weight:                             99,
			Tier:                               "cheap",
			RPM:                                10,
			InputPricePerMillionUSD:            0.25,
			OutputPricePerMillionUSD:           1.25,
			ImageInputPricePerMillionTokensUSD: 3.5,
			ImageInputPricePerImageUSD:         0.002,
			PricingSource:                      "https://example.test/pricing",
			PricingUpdatedAt:                   "2026-06-17",
			ToolSupport: ToolSupport{
				OpenAIResponses: []string{"function"},
			},
		},
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
	if targets[0].InputPricePerMillionUSD != 0.25 || targets[0].OutputPricePerMillionUSD != 1.25 ||
		targets[0].ImageInputPricePerMillionTokensUSD != 3.5 || targets[0].ImageInputPricePerImageUSD != 0.002 ||
		targets[0].PricingSource != "https://example.test/pricing" || targets[0].PricingUpdatedAt != "2026-06-17" ||
		len(targets[0].ToolSupport.OpenAIResponses) != 1 || targets[0].ToolSupport.OpenAIResponses[0] != "function" {
		t.Fatalf("small ref metadata not resolved: %#v", targets[0])
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

func TestProviderModelPricingAndToolSupportValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		model  ProviderModel
		target Target
		want   string
	}{
		{
			name:  "negative provider input price",
			model: ProviderModel{Model: "mock-known", InputPricePerMillionUSD: -0.01},
			want:  "input_price_per_million_usd",
		},
		{
			name:  "duplicate provider tool support",
			model: ProviderModel{Model: "mock-known", ToolSupport: ToolSupport{OpenAIResponses: []string{"function", "function"}}},
			want:  "duplicate",
		},
		{
			name:   "negative target output price",
			model:  ProviderModel{Model: "mock-known"},
			target: Target{OutputPricePerMillionUSD: -0.01},
			want:   "output_price_per_million_usd",
		},
		{
			name:  "negative provider image token price",
			model: ProviderModel{Model: "mock-known", ImageInputPricePerMillionTokensUSD: -0.01},
			want:  "image_input_price_per_million_tokens_usd",
		},
		{
			name:   "negative target image unit price",
			model:  ProviderModel{Model: "mock-known"},
			target: Target{ImageInputPricePerImageUSD: -0.01},
			want:   "image_input_price_per_image_usd",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t)
			provider := cfg.Provider["mock"]
			provider.Models = map[string]ProviderModel{"known": tt.model}
			cfg.Provider["mock"] = provider
			target := Target{Provider: "mock", ModelRef: "known"}
			if tt.target.OutputPricePerMillionUSD != 0 {
				target.OutputPricePerMillionUSD = tt.target.OutputPricePerMillionUSD
			}
			if tt.target.ImageInputPricePerMillionTokensUSD != 0 {
				target.ImageInputPricePerMillionTokensUSD = tt.target.ImageInputPricePerMillionTokensUSD
			}
			if tt.target.ImageInputPricePerImageUSD != 0 {
				target.ImageInputPricePerImageUSD = tt.target.ImageInputPricePerImageUSD
			}
			cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{target}}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
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
	assertResponsesCompatibleProvider(t, cfg.Provider["openrouter_responses"], "deepseek-v4-flash-nitro", "deepseek/deepseek-v4-flash:nitro")
	assertAnthropicCompatibleProvider(t, cfg.Provider["openrouter_anthropic"], "deepseek-v4-flash-nitro", "deepseek/deepseek-v4-flash:nitro")
	assertAnthropicCompatibleProvider(t, cfg.Provider["openrouter_anthropic"], "gemma-4-26b-a4b-it-nitro", "google/gemma-4-26b-a4b-it:nitro")
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
		if name == "agent-tools-smoke-openrouter" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_responses" || group.Targets[0].Model != "deepseek/deepseek-v4-flash:nitro" {
				t.Fatalf("example config agent-tools-smoke-openrouter=%#v, want static OpenRouter DeepSeek V4 Flash responses", group)
			}
			continue
		}
		if name == "claude-tools-smoke-openrouter" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_anthropic" || group.Targets[0].Model != "deepseek/deepseek-v4-flash:nitro" {
				t.Fatalf("example config claude-tools-smoke-openrouter=%#v, want static OpenRouter DeepSeek V4 Flash Anthropic-compatible target", group)
			}
			continue
		}
		if name == "claude-tools-smoke-openrouter-gemma" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_anthropic" || group.Targets[0].Model != "google/gemma-4-26b-a4b-it:nitro" {
				t.Fatalf("example config claude-tools-smoke-openrouter-gemma=%#v, want static OpenRouter Gemma Anthropic-compatible target", group)
			}
			continue
		}
		if name == "agent-tools-smoke-openrouter-qwen36" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_responses" || group.Targets[0].Model != "qwen/qwen3.6-flash:nitro" {
				t.Fatalf("example config agent-tools-smoke-openrouter-qwen36=%#v, want static OpenRouter Qwen3.6 Flash Responses-compatible target", group)
			}
			continue
		}
		if name == "claude-tools-smoke-openrouter-qwen36" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter_anthropic" || group.Targets[0].Model != "qwen/qwen3.6-flash:nitro" {
				t.Fatalf("example config claude-tools-smoke-openrouter-qwen36=%#v, want static OpenRouter Qwen3.6 Flash Anthropic-compatible target", group)
			}
			continue
		}
		if name == "vision-smoke-openrouter-qwen36" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "openrouter" || group.Targets[0].Model != "qwen/qwen3.6-flash:nitro" {
				t.Fatalf("example config vision-smoke-openrouter-qwen36=%#v, want static OpenRouter Qwen3.6 Flash vision target", group)
			}
			continue
		}
		if name == "baseten-nemotron-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "nvidia/Nemotron-120B-A12B" {
				t.Fatalf("example config baseten-nemotron-smoke=%#v, want static Baseten Nemotron target", group)
			}
			continue
		}
		if name == "warp-agent-smoke" {
			if group.Strategy != "static" || len(group.Targets) != 1 || group.Targets[0].Provider != "baseten" || group.Targets[0].Model != "nvidia/Nemotron-120B-A12B" {
				t.Fatalf("example config warp-agent-smoke=%#v, want static Baseten Nemotron OpenAI Chat tool target", group)
			}
			continue
		}
		if name == "vision" {
			if group.Strategy != "weighted" || len(group.Targets) < 2 {
				t.Fatalf("example config vision=%#v, want weighted multi-target vision group", group)
			}
			seen := map[string]bool{}
			for _, target := range group.Targets {
				seen[target.Provider+":"+target.Model] = true
				if !stringSliceContains(target.InputModalities, "image") {
					t.Fatalf("example config vision target lacks image modality: %#v", target)
				}
			}
			if !seen["xai:grok-4.3"] || !seen["openai:gpt-5.4-nano"] {
				t.Fatalf("example config vision targets=%#v, want xAI Grok 4.3 and OpenAI GPT-5.4 Nano", group.Targets)
			}
			continue
		}
		if group.Strategy != "weighted" {
			t.Fatalf("example config group %s strategy=%q want weighted", name, group.Strategy)
		}
		assertActiveGroupPolicy(t, name, group)
	}
	wantAllows := map[string][]string{
		"standard-dev": {"default", "fast", "small", "vision"},
		"coding-dev":   {"default", "fast", "big-coder", "small", "medium", "high", "vision", "agent-tools-smoke", "claude-tools-smoke", "agent-tools-smoke-openrouter", "agent-tools-smoke-openrouter-qwen36", "claude-tools-smoke-openrouter", "claude-tools-smoke-openrouter-qwen36", "vision-smoke-openrouter-qwen36", "claude-tools-smoke-openrouter-gemma", "baseten-nemotron-smoke", "warp-agent-smoke"},
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
		"baseten:nvidia/Nemotron-120B-A12B":           "nvidia/Nemotron-120B-A12B",
		"minimax:MiniMax-M3":                          "MiniMax-M3",
		"kimi:kimi-k2.7-code":                         "kimi-k2.7-code",
		"openrouter:deepseek/deepseek-v4-flash:nitro": "deepseek/deepseek-v4-flash:nitro",
		"openrouter:google/gemma-4-26b-a4b-it:nitro":  "google/gemma-4-26b-a4b-it:nitro",
		"openai:gpt-5.4-nano":                         "gpt-5.4-nano",
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

func assertResponsesCompatibleProvider(t *testing.T, provider ProviderConfig, ref, model string) {
	t.Helper()
	if provider.Dialect != "openai-responses" || provider.Models[ref].Model != model {
		t.Fatalf("provider not configured for OpenAI Responses-compatible skin: %#v", provider)
	}
}

func assertActiveGroupPolicy(t *testing.T, name string, group ModelGroup) {
	t.Helper()
	totalWeight := 0
	m3Weight := 0
	deepSeekWeight := 0
	kimiWeight := 0
	gemmaWeight := 0
	qwenFlashWeight := 0
	openAIWeight := 0
	basetenWeight := 0
	normalTargets := 0
	codexToolTarget := false
	codexOpenRouterToolTarget := false
	claudeMiniMaxToolTarget := false
	claudeKimiToolTarget := false
	claudeOpenRouterToolTarget := false
	claudeGemmaToolTarget := false
	for _, target := range group.Targets {
		if violatesCurrentRoutingPolicy(target) {
			t.Fatalf("example config group %s has routing-policy violation %#v", name, target)
		}
		if target.ToolOnly {
			if target.Provider == "minimax" && target.Model == "MiniMax-M3" && target.Dialect == "openai-responses" {
				codexToolTarget = true
			}
			if target.Provider == "openrouter_responses" && target.Model == "deepseek/deepseek-v4-flash:nitro" {
				codexOpenRouterToolTarget = true
			}
			if target.Provider == "minimax_anthropic" && target.Model == "MiniMax-M3" {
				claudeMiniMaxToolTarget = true
			}
			if target.Provider == "kimi_anthropic" && target.Model == "kimi-k2.7-code" && target.DefaultThinking["type"] == "enabled" {
				claudeKimiToolTarget = true
			}
			if target.Provider == "openrouter_anthropic" && target.Model == "deepseek/deepseek-v4-flash:nitro" {
				claudeOpenRouterToolTarget = true
			}
			if target.Provider == "openrouter_anthropic" && target.Model == "google/gemma-4-26b-a4b-it:nitro" {
				claudeGemmaToolTarget = true
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
		if target.Provider == "openrouter" && target.Model == "google/gemma-4-26b-a4b-it:nitro" {
			gemmaWeight += target.Weight
		}
		if target.Provider == "openrouter" && target.Model == "qwen/qwen3.6-flash:nitro" {
			qwenFlashWeight += target.Weight
		}
		if target.Provider == "openai" && target.Model == "gpt-5.4-nano" {
			openAIWeight += target.Weight
		}
		if target.Provider == "baseten" && target.Model == "nvidia/Nemotron-120B-A12B" {
			basetenWeight += target.Weight
		}
	}
	if !codexToolTarget || !codexOpenRouterToolTarget || !claudeMiniMaxToolTarget || !claudeKimiToolTarget || !claudeOpenRouterToolTarget || !claudeGemmaToolTarget {
		t.Fatalf("example config group %s missing tool-only targets codex=%v codex_openrouter=%v minimax=%v kimi=%v claude_openrouter=%v claude_gemma=%v", name, codexToolTarget, codexOpenRouterToolTarget, claudeMiniMaxToolTarget, claudeKimiToolTarget, claudeOpenRouterToolTarget, claudeGemmaToolTarget)
	}
	want := map[string]struct {
		deepSeek, m3, gemma, qwenFlash, kimi, openAI, baseten, targets int
	}{
		"default":   {51, 27, 7, 5, 6, 1, 3, 7},
		"fast":      {56, 26, 4, 5, 5, 1, 3, 7},
		"small":     {55, 28, 4, 5, 4, 1, 3, 7},
		"medium":    {51, 25, 7, 5, 8, 1, 3, 7},
		"high":      {46, 26, 9, 5, 10, 1, 3, 7},
		"big-coder": {18, 45, 0, 5, 28, 1, 3, 6},
	}
	expect, ok := want[name]
	if !ok {
		t.Fatalf("example config group %s has no expected weight policy", name)
	}
	if totalWeight != 100 || normalTargets != expect.targets || deepSeekWeight != expect.deepSeek || m3Weight != expect.m3 || gemmaWeight != expect.gemma || qwenFlashWeight != expect.qwenFlash || kimiWeight != expect.kimi || openAIWeight != expect.openAI || basetenWeight != expect.baseten {
		t.Fatalf("example config group %s weights deepseek=%d m3=%d gemma=%d qwen_flash=%d kimi=%d openai=%d baseten=%d total=%d normal_targets=%d, want %#v", name, deepSeekWeight, m3Weight, gemmaWeight, qwenFlashWeight, kimiWeight, openAIWeight, basetenWeight, totalWeight, normalTargets, expect)
	}
}

func violatesCurrentRoutingPolicy(target Target) bool {
	needle := strings.ToLower(target.Provider + "/" + target.Model + "/" + target.ModelRef)
	if target.ToolOnly && stringSliceContains(target.InputModalities, "image") && (target.Provider == "openrouter_responses" || target.Provider == "openrouter_anthropic") {
		switch target.ModelRef {
		case "qwen3-7-plus-nitro", "openrouter-claude-sonnet-4-6", "openrouter-xai-grok-4-3", "openrouter-minimax-m3":
			return false
		}
	}
	if strings.Contains(needle, "moonshotai/kimi") {
		return true
	}
	allowedQwen := target.Model == "qwen/qwen3.6-flash:nitro" || target.Model == "qwen/qwen3.7-plus:nitro"
	allowedBasetenNemotron := target.Provider == "baseten" && target.Model == "nvidia/Nemotron-120B-A12B"
	for _, bad := range []string{"qwen", "glm", "hy3", "kat-coder", "nemotron", "mercury", "ling-2.6", "pareto", "m2.7-highspeed"} {
		if strings.Contains(needle, bad) {
			return !allowedQwen && !allowedBasetenNemotron
		}
	}
	if target.ToolOnly && (target.Provider == "openai" || target.Provider == "anthropic") {
		return true
	}
	if (target.Provider == "openai" || target.Provider == "anthropic") && target.Weight > 1 {
		return true
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
