package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReasoningIntentDetection(t *testing.T) {
	chat, err := decodeRequest("openai-chat", []byte(`{"model":"default","reasoning_effort":"low","messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !chat.Reasoning.Requested || chat.Reasoning.Kind != "effort" || chat.Reasoning.Effort != "low" {
		t.Fatalf("chat reasoning=%#v", chat.Reasoning)
	}

	xhigh, err := decodeRequest("openai-chat", []byte(`{"model":"default","reasoning_effort":"xhigh","messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !xhigh.Reasoning.Requested || xhigh.Reasoning.Effort != "xhigh" {
		t.Fatalf("xhigh reasoning=%#v", xhigh.Reasoning)
	}

	plain, err := decodeRequest("openai-chat", []byte(`{"model":"default","max_completion_tokens":16,"messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Reasoning.Requested {
		t.Fatalf("max_completion_tokens alone should not request reasoning: %#v", plain.Reasoning)
	}

	responses, err := decodeRequest("openai-responses", []byte(`{"model":"default","reasoning":{"effort":"medium","summary":"auto"},"input":"x"}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !responses.Reasoning.Requested || responses.Reasoning.Effort != "medium" || responses.Reasoning.Summary != "auto" {
		t.Fatalf("responses reasoning=%#v", responses.Reasoning)
	}

	anthropic, err := decodeRequest("anthropic", []byte(`{"model":"default","thinking":{"type":"enabled","budget_tokens":4096},"max_tokens":8192,"messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !anthropic.Reasoning.Requested || anthropic.Reasoning.Kind != "token_budget" || anthropic.Reasoning.BudgetTokens != 4096 {
		t.Fatalf("anthropic reasoning=%#v", anthropic.Reasoning)
	}

	disabled, err := decodeRequest("anthropic", []byte(`{"model":"default","thinking":{"type":"disabled"},"messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !disabled.Reasoning.Disabled || disabled.Reasoning.Requested {
		t.Fatalf("disabled reasoning=%#v", disabled.Reasoning)
	}
}

func TestReasoningMetadataResolvesAndValidates(t *testing.T) {
	cfg := minimalConfig(t)
	provider := cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"reasoning": {
			Model: "mock-reasoning",
			Reasoning: ReasoningSupport{
				Supported:         true,
				Mode:              reasoningModeOptIn,
				Control:           reasoningControlEffortEnum,
				SupportsSummaries: true,
			},
		},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "reasoning"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	target := cfg.Models["default"].Targets[0]
	if !target.Reasoning.Supported || target.Reasoning.Control != reasoningControlEffortEnum || !target.Reasoning.SupportsSummaries {
		t.Fatalf("resolved reasoning=%#v", target.Reasoning)
	}

	cfg = minimalConfig(t)
	provider = cfg.Provider["mock"]
	provider.Models = map[string]ProviderModel{
		"bad": {Model: "bad", Reasoning: ReasoningSupport{Supported: true, Mode: "sometimes", Control: reasoningControlEffortEnum}},
	}
	cfg.Provider["mock"] = provider
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "bad"}}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "invalid reasoning") {
		t.Fatalf("expected invalid reasoning metadata error, got %v", err)
	}
}

func TestReasoningRequestFiltersTargetsAndTranslates(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_reasoning",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "reasoning-chat",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{Strategy: "failover", Targets: []Target{
		{Provider: "mock", Model: "plain-chat"},
		{Provider: "mock", Model: "reasoning-chat", Reasoning: ReasoningSupport{Supported: true, Mode: reasoningModeOptIn, Control: reasoningControlEffortEnum, RejectsMaxTokens: true}},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req, err := decodeRequest("openai-chat", []byte(`{"model":"default","reasoning_effort":"high","max_tokens":32,"messages":[{"role":"user","content":"x"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	got := svc.targetsForRequest(cfg.Models["default"].Targets, req, "openai-chat")
	if len(got) != 1 || got[0].Model != "reasoning-chat" {
		t.Fatalf("eligible targets=%#v", got)
	}

	rr := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","reasoning_effort":"high","max_tokens":32,"messages":[{"role":"user","content":"x"}]}`))
	httpReq.Header.Set("Authorization", "Bearer "+testToken)
	httpReq.Header.Set("Content-Type", "application/json")
	svc.Handler().ServeHTTP(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort=%#v", upstreamBody["reasoning_effort"])
	}
	if _, ok := upstreamBody["max_tokens"]; ok {
		t.Fatalf("max_tokens should have been converted for reasoning target: %#v", upstreamBody)
	}
	if upstreamBody["max_completion_tokens"] != float64(32) {
		t.Fatalf("max_completion_tokens=%#v", upstreamBody["max_completion_tokens"])
	}
}

func TestReasoningNoEligibleTargetAndModelListMetadata(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "plain-chat"}}}
	cfg.Models["reasoning"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "reasoning-chat", Reasoning: ReasoningSupport{Supported: true, Mode: reasoningModeOptIn, Control: reasoningControlEffortEnum, SupportsSummaries: true}}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "reasoning")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","reasoning_effort":"low","messages":[{"role":"user","content":"x"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), "reasoning") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	modelReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelReq.Header.Set("Authorization", "Bearer "+testToken)
	modelRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(modelRR, modelReq)
	if modelRR.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", modelRR.Code, modelRR.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(modelRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	foundReasoning := false
	for _, item := range body["data"].([]any) {
		model := item.(map[string]any)
		if model["id"] == "reasoning" {
			foundReasoning = true
			levels := model["supported_reasoning_levels"].([]any)
			if len(levels) != 3 || model["supports_reasoning_summaries"] != true {
				t.Fatalf("reasoning model metadata=%#v", model)
			}
		}
	}
	if !foundReasoning {
		t.Fatalf("reasoning group missing from /v1/models: %#v", body)
	}
}

func TestReasoningModelListDoesNotAdvertiseToolOnlyThinking(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "plain-chat"},
		{Provider: "mock", Model: "anthropic-tool", Dialect: "anthropic", ToolOnly: true, DefaultThinking: map[string]any{"type": "enabled", "budget_tokens": 512}},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	levels, summaries, defaultLevel := svc.reasoningMetadataForGroup("default")
	if len(levels) != 0 || summaries || defaultLevel != "none" {
		t.Fatalf("default reasoning metadata levels=%#v summaries=%v default=%q", levels, summaries, defaultLevel)
	}
}
