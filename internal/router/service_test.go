package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAnthropicIngressUnaryHappyPath(t *testing.T) {
	var upstreamAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_1",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "hello through router"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
		})
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, "secret-provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "claude-code-test")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-provider-key") {
		t.Fatal("response leaked provider key")
	}
	if upstreamAuth != "Bearer secret-provider-key" {
		t.Fatalf("upstream auth not injected, got %q", upstreamAuth)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "message" {
		t.Fatalf("not an anthropic message response: %#v", body)
	}
}

func TestAuthRejectsUnknownTokenBeforeUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer bogus")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatal("upstream was called for unauthorized request")
	}
}

func TestCacheHitAcrossDialectsAndTargetIsolation(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_cache",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "cached answer"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": int(n), "total_tokens": int(n) + 1},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		return rr
	}

	post("/v1/chat/completions", `{"model":"default","messages":[{"role":"user","content":"same"}]}`)
	post("/v1/messages", `{"model":"default","messages":[{"role":"user","content":"same"}]}`)
	if calls.Load() != 1 {
		t.Fatalf("expected cross-dialect cache hit, upstream calls=%d", calls.Load())
	}
	post("/v1/messages", `{"model":"other","messages":[{"role":"user","content":"same"}]}`)
	if calls.Load() != 2 {
		t.Fatalf("expected separate cache entry for different target, upstream calls=%d", calls.Load())
	}
}

func TestLifetimeKeyExhaustionReturns403AndPersists(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_quota",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "quota"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 9, "total_tokens": 10},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Callers[0].Key.LifetimeTokens = 10
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", rr.Code, rr.Body.String())
	}
	svc.Close()

	svc2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc2.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"again"}]}`))
	req2.Header.Set("Authorization", "Bearer "+testToken)
	rr2 := httptest.NewRecorder()
	svc2.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("second status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if !strings.Contains(rr2.Body.String(), "key-exhausted") {
		t.Fatalf("missing key-exhausted: %s", rr2.Body.String())
	}
}

func TestCountTokensEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"count these tokens"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["input_tokens"] <= 0 {
		t.Fatalf("bad token estimate: %#v", body)
	}
}

func TestUsageAndLogsIncludeCallerMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_meta",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "metadata"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer "+testToken)
	usageRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(usageRR, usageReq)
	if usageRR.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", usageRR.Code, usageRR.Body.String())
	}
	var usage map[string]any
	if err := json.Unmarshal(usageRR.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage["caller_user"] != "alice" || usage["caller_project"] != "metrum-insights" || usage["caller_environment"] != "test" {
		t.Fatalf("usage metadata missing: %#v", usage)
	}
	svc.Close()

	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"caller_user":"alice"`) || !strings.Contains(string(raw), `"caller_project":"metrum-insights"`) || !strings.Contains(string(raw), `"caller_environment":"test"`) {
		t.Fatalf("log metadata missing: %s", raw)
	}
}

func TestMetricsEndpointRequiresAuthAndExportsCallerLabels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_metrics",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "metrics"},
			}},
			"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 6, "total_tokens": 10},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	unauth := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	unauthRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauth metrics status=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+testToken)
	metricsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
	}
	body := metricsRR.Body.String()
	for _, want := range []string{
		`smart_llmrouter_requests_total`,
		`caller_id="alice"`,
		`caller_user="alice"`,
		`caller_project="metrum-insights"`,
		`caller_environment="test"`,
		`token_id="rtr_alice_test"`,
		`model_group="default"`,
		`target_provider="mock"`,
		`target_model="mock-model"`,
		`smart_llmrouter_tokens_total`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestReplicateProviderAdapter(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "pred_1",
			"status": "succeeded",
			"output": []any{"replicate ", "answer"},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["replicate"] = ProviderConfig{BaseURL: upstream.URL, Dialect: "replicate", APIKey: "replicate-key"}
	cfg.Models["replicate"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "replicate", Model: "owner/model-name"}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "replicate")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"replicate","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/models/owner/model-name/predictions" {
		t.Fatalf("unexpected replicate path %s", gotPath)
	}
	if gotAuth != "Bearer replicate-key" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	input := gotBody["input"].(map[string]any)
	if !strings.Contains(input["prompt"].(string), "hi") {
		t.Fatalf("prompt not mapped: %#v", input)
	}
	if !strings.Contains(rr.Body.String(), "replicate answer") {
		t.Fatalf("response not mapped: %s", rr.Body.String())
	}
}

func TestTypeScriptRoutingStrategy(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
type Ctx = { text: string; targets: Array<{ provider: string; model: string; tier?: string }> };
export function route(ctx: Ctx) {
  const targetIndex = ctx.text.includes("architecture") ? 1 : 0;
  return { targetIndex, classLabel: "test:" + targetIndex };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["scripted"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap"},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy"},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "scripted")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	dec, err := svc.pick("scripted", cfg.Models["scripted"], &IRRequest{
		Model:    "scripted",
		Messages: []IRMessage{{Role: "user", Content: "architecture question"}},
	})
	if err != nil {
		t.Fatalf("script pick: %v", err)
	}
	if dec.Target.Model != "heavy-model" {
		t.Fatalf("direct script pick selected %q", dec.Target.Model)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"scripted","messages":[{"role":"user","content":"architecture question"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "heavy-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestScriptTargetsIncludeProviderMetadataWithoutRawKeys(t *testing.T) {
	targets := []Target{{
		Provider:    "mock",
		ModelRef:    "small",
		Model:       "mock-small",
		Weight:      7,
		Tier:        "cheap",
		RPM:         12,
		Cost:        3,
		Dialect:     "openai-responses",
		DisplayName: "Mock Small",
	}}
	providers := map[string]ProviderConfig{
		"mock": {
			BaseURL:   "https://mock.example/v1",
			Dialect:   "openai-chat",
			APIKey:    "raw-secret-key",
			APIKeyEnv: "MOCK_API_KEY",
			KeyID:     "mock-key",
		},
	}
	scriptTargets := buildScriptTargets(targets, providers)
	if len(scriptTargets) != 1 {
		t.Fatalf("script target count=%d", len(scriptTargets))
	}
	got := scriptTargets[0]
	if got.Model != "mock-small" || got.ModelRef != "small" || got.BaseURL != "https://mock.example/v1" || got.Dialect != "openai-responses" {
		t.Fatalf("metadata not populated: %#v", got)
	}
	if got.Weight != 7 || got.KeyID != "mock-key" || got.APIKeyEnv != "MOCK_API_KEY" || !got.KeyConfigured {
		t.Fatalf("key/weight metadata not populated: %#v", got)
	}
	raw, err := json.Marshal(scriptTargets)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "raw-secret-key") {
		t.Fatalf("raw API key leaked into script context: %s", raw)
	}
}

func TestModelRefTargetUsesResolvedExternalModel(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "model_ref_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "resolved"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{
		BaseURL: upstream.URL + "/v1",
		Dialect: "openai",
		APIKey:  "provider-key",
		Models: map[string]ProviderModel{
			"small": {Model: "mock-small"},
		},
	}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "small"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "mock-small" {
		t.Fatalf("upstream model=%q", gotModel)
	}
}

func TestOpenRouterAnthropicSkinUsesBearerAuthAndMessagesPath(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey, gotVersion, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotVersion = r.Header.Get("Anthropic-Version")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            "msg_openrouter",
			"type":          "message",
			"role":          "assistant",
			"model":         gotModel,
			"content":       []map[string]any{{"type": "text", "text": "openrouter anthropic ok"}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["openrouter_anthropic"] = ProviderConfig{
		BaseURL:    upstream.URL + "/api",
		Dialect:    "anthropic",
		AuthScheme: "bearer",
		APIKey:     "openrouter-key",
		Models: map[string]ProviderModel{
			"claude-sonnet-46-nitro": {Model: "anthropic/claude-sonnet-4.6:nitro", Weight: 1},
		},
	}
	cfg.Models["openrouter-anthropic"] = ModelGroup{
		Strategy: "static",
		Targets:  []Target{{Provider: "openrouter_anthropic", ModelRef: "claude-sonnet-46-nitro"}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "openrouter-anthropic")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"openrouter-anthropic","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/messages" {
		t.Fatalf("unexpected path %s", gotPath)
	}
	if gotAuth != "Bearer openrouter-key" || gotAPIKey != "" {
		t.Fatalf("unexpected auth headers Authorization=%q X-API-Key=%q", gotAuth, gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("missing Anthropic-Version, got %q", gotVersion)
	}
	if gotModel != "anthropic/claude-sonnet-4.6:nitro" {
		t.Fatalf("upstream model=%q", gotModel)
	}
}

const testToken = "rtr_test_token"

func newTestService(t *testing.T, upstreamURL, providerKey string) *Service {
	t.Helper()
	svc, err := New(testConfig(t, upstreamURL, providerKey, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func testConfig(t *testing.T, upstreamURL, providerKey, dir string) *Config {
	t.Helper()
	sum := sha256.Sum256([]byte(testToken))
	return &Config{
		Server: ServerConfig{
			Listen: ":0",
			Cache:  CacheConfig{Enabled: true, MaxBytes: 1 << 20, DefaultTTL: 0},
			Logging: LoggingConfig{
				Path: filepath.Join(dir, "requests.jsonl"),
			},
		},
		StatePath: filepath.Join(dir, "state.json"),
		Provider: map[string]ProviderConfig{
			"mock": {BaseURL: upstreamURL + "/v1", Dialect: "openai", APIKey: providerKey},
		},
		Models: map[string]ModelGroup{
			"default": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
			"other":   {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "other-model"}}},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			User:        "alice",
			Project:     "metrum-insights",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(sum[:]),
			TokenID:     "rtr_alice_test",
			Allow:       []string{"default", "other"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
			Quota:       QuotaConfig{Day: BudgetConfig{Requests: 100, Tokens: 100000}, Month: BudgetConfig{Tokens: 1000000}, SoftPct: 80},
			Key:         KeyConfig{LifetimeTokens: 1000000, SoftPct: 90, OnExhaust: "disable"},
		}},
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
