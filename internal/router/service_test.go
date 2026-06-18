package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	if _, tokenID, err := svc.authenticate("", ""); err == nil || tokenID != "missing-token" {
		t.Fatalf("missing token id=%q err=%v", tokenID, err)
	}
	if _, tokenID, err := svc.authenticate("Bearer bogus-secret-token", ""); err == nil || tokenID != "invalid-token" {
		t.Fatalf("invalid token id=%q err=%v", tokenID, err)
	}
}

func TestAuthAcceptsXAPIKeyForAnthropicStyleClients(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_x_api_key",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "x-api-key ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-API-Key", testToken)
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "x-api-key ok") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestModelsEndpointIncludesCodexModelsField(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["data"].([]any); !ok {
		t.Fatalf("missing OpenAI data field: %#v", body)
	}
	for _, forbidden := range []string{"version", "commit", "build_date", "go_version", "goos", "goarch"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("/v1/models leaked router build field %q: %#v", forbidden, body)
		}
	}
	if models, ok := body["models"].([]any); !ok || len(models) == 0 {
		t.Fatalf("missing Codex models field: %#v", body)
	} else if first, ok := models[0].(map[string]any); !ok || first["slug"] == "" || first["display_name"] == "" || first["base_instructions"] == "" || first["context_window"] == nil || first["max_context_window"] == nil || first["supported_reasoning_levels"] == nil || first["shell_type"] == "" || first["supported_in_api"] != true {
		t.Fatalf("missing Codex model compatibility fields: %#v", body)
	}
}

func TestVersionAndHealthEndpointsExposeBuildInfo(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	for _, path := range []string{"/version", "/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s json: %v", path, err)
		}
		for _, key := range []string{"version", "commit", "build_date"} {
			if body[key] == "" || body[key] == nil {
				t.Fatalf("%s missing %s: %#v", path, key, body)
			}
		}
		if path == "/version" {
			for _, key := range []string{"go_version", "goos", "goarch"} {
				if body[key] == "" || body[key] == nil {
					t.Fatalf("%s missing %s: %#v", path, key, body)
				}
			}
		} else if body["ok"] != true {
			t.Fatalf("%s ok field=%#v body=%#v", path, body["ok"], body)
		}
	}
}

func TestEmbeddedDocsRootRedirectsToDocs(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/docs/" {
		t.Fatalf("location=%q", got)
	}
}

func TestEmbeddedDocsAreServedUnderDocs(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Metrum Smart LLM Router") {
		t.Fatalf("root did not serve docs HTML: %s", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	for _, header := range []string{"X-Smart-LLMRouter-Version", "X-Smart-LLMRouter-Commit", "X-Smart-LLMRouter-Build-Date"} {
		if rr.Header().Get(header) == "" {
			t.Fatalf("missing docs version header %s", header)
		}
	}
}

func TestEmbeddedDocsServeExtensionlessDocusaurusPages(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/docs/solution-brief", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Solution Brief") {
		t.Fatalf("extensionless doc page did not serve page HTML: %s", rr.Body.String())
	}
}

func TestEmbeddedDocsFallbackDoesNotMaskAPIRoutes(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	for _, path := range []string{"/v1/unknown", "/v1", "/metrics/extra"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()

		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "Metrum Smart LLM Router Docs") {
			t.Fatalf("%s unexpectedly served docs fallback", path)
		}
	}
}

func TestModelsEndpointMarksAgentToolsSmokeAsToolCapable(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "tool-model", Dialect: "openai-responses"}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	models := body["models"].([]any)
	first := models[0].(map[string]any)
	if first["supports_parallel_tool_calls"] != true {
		t.Fatalf("agent-tools-smoke not marked tool capable: %#v", first)
	}
	tools := first["experimental_supported_tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("agent-tools-smoke missing supported tools: %#v", first)
	}
}

func TestModelsEndpointReportsVisionInputModalities(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "text-model", InputModalities: []string{"text"}},
		{Provider: "mock", Model: "vision-model", InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"}},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	models := body["models"].([]any)
	first := models[0].(map[string]any)
	if first["supports_image_detail_original"] != true {
		t.Fatalf("vision support not advertised: %#v", first)
	}
	modalities := first["input_modalities"].([]any)
	hasImage := false
	for _, modality := range modalities {
		hasImage = hasImage || modality == "image"
	}
	if !hasImage {
		t.Fatalf("input_modalities missing image: %#v", first)
	}
	for _, modality := range modalities {
		if modality != "text" && modality != "image" {
			t.Fatalf("public input_modalities exposed client-incompatible modality %q: %#v", modality, first)
		}
	}
}

func TestImageRequestsFilterToVisionTargetsAndBypassCache(t *testing.T) {
	var gotModel string
	var gotImageURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = stringValue(body["model"])
		msgs := body["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		img := content[1].(map[string]any)["image_url"].(map[string]any)
		gotImageURL = stringValue(img["url"])
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_vision",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Rite Aid"},
			}},
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 4,
				"total_tokens":      104,
				"prompt_tokens_details": map[string]any{
					"image_tokens": 64,
				},
				"cost": 0.00456,
				"cost_details": map[string]any{
					"upstream_inference_prompt_cost":      0.003,
					"upstream_inference_completions_cost": 0.00156,
				},
			},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "text-model", InputModalities: []string{"text"}},
		{
			Provider:                           "mock",
			Model:                              "vision-model",
			InputModalities:                    []string{"text", "image"},
			InputPricePerMillionUSD:            2,
			OutputPricePerMillionUSD:           8,
			ImageInputPricePerMillionTokensUSD: 10,
			ImageInputPricePerImageUSD:         0.001,
		},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "default",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Read the receipt."},
				{"type": "image_url", "image_url": {"url": "`+receiptImageURL+`"}}
			]
		}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	svc.Close()

	if gotModel != "vision-model" || gotImageURL != receiptImageURL {
		t.Fatalf("upstream model/image=%q/%q", gotModel, gotImageURL)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(raw)
	for _, want := range []string{
		`"target_model":"vision-model"`,
		`"input_has_image":true`,
		`"input_image_count":1`,
		`"input_image_tokens":64`,
		`"image_input_price_per_million_tokens_usd":10`,
		`"image_input_price_per_image_usd":0.001`,
		`"image_cost_usd":0.00164`,
		`"upstream_reported_total_cost_usd":0.00456`,
		`"cache":"bypass"`,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("vision log missing %s: %s", want, raw)
		}
	}
	if !strings.Contains(logText, `"input_cost_usd":0.000072`) || !strings.Contains(logText, `"output_cost_usd":0.000032`) || !strings.Contains(logText, `"total_cost_usd":0.001744`) {
		t.Fatalf("vision log missing target/image/cache fields: %s", raw)
	}
}

func TestOpenAIResponsesToolPassthroughPreservesToolsAndRawOutput(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "resp_upstream_tool",
			"object": "response",
			"status": "requires_action",
			"model":  "gpt-tool",
			"output": []map[string]any{{
				"id":        "call_1",
				"type":      "function_call",
				"name":      "shell",
				"call_id":   "call_1",
				"arguments": `{"cmd":"cat > /app/solver.py"}`,
				"status":    "completed",
			}},
			"usage": map[string]any{"input_tokens": 11, "output_tokens": 7, "total_tokens": 18},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["openai"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "openai", Model: "gpt-tool"}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"agent-tools-smoke",
		"input":"write solver",
		"stream":true,
		"tools":[{"type":"function","name":"shell","description":"run shell","parameters":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "codex-test")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "gpt-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q, want event stream; body=%s", ct, rr.Body.String())
	}
	events := parseSSEEvents(t, rr.Body.String())
	itemDone, ok := events["response.output_item.done"]
	if !ok {
		t.Fatalf("tool output SSE missing: %#v", events)
	}
	item := itemDone["item"].(map[string]any)
	if item["type"] != "function_call" || item["call_id"] != "call_1" {
		t.Fatalf("function call not preserved in SSE: %#v", item)
	}
	completed, ok := events["response.completed"]
	if !ok {
		t.Fatalf("completed SSE missing: %#v", events)
	}
	response := completed["response"].(map[string]any)
	if response["id"] != "resp_upstream_tool" || response["status"] != "requires_action" {
		t.Fatalf("raw response not preserved in completed event: %#v", response)
	}
}

func TestOpenAIChatToolPassthroughPreservesToolsAndStreamsToolCalls(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_tool",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "chat-tool",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]any{{
						"id":   "call_weather",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"location":"San Francisco"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 17, "completion_tokens": 5, "total_tokens": 22},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["openai_chat"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["warp-agent-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "openai_chat",
		Model:       "chat-tool",
		ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
	}}}
	cfg.Callers[0].Allow = []string{"warp-agent-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"warp-agent-smoke",
		"stream":true,
		"messages":[{"role":"user","content":"weather"}],
		"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object","properties":{"location":{"type":"string"}}}}}],
		"tool_choice":"auto",
		"parallel_tool_calls":true
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "OpenAI/Go 3.15.0")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "chat-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	if upstreamBody["tool_choice"] != "auto" || upstreamBody["parallel_tool_calls"] != true {
		t.Fatalf("tool fields not preserved upstream: %#v", upstreamBody)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q, want event stream; body=%s", ct, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"tool_calls"`) ||
		!strings.Contains(body, `"get_weather"`) ||
		!strings.Contains(body, `"finish_reason":"tool_calls"`) ||
		!strings.Contains(body, "data: [DONE]") {
		t.Fatalf("tool call stream not preserved:\n%s", body)
	}
}

func TestOpenAIChatToolRequestsRequireExplicitToolSupport(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "chat-maybe-tools"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := &IRRequest{Tools: []map[string]any{{"type": "function"}}, Messages: []IRMessage{{Role: "user", Content: "hi"}}}
	if got := svc.targetsForRequest(cfg.Models["default"].Targets, req, "openai-chat"); len(got) != 0 {
		t.Fatalf("openai-chat target without explicit tool metadata was eligible: %#v", got)
	}
}

func TestAnthropicToolPassthroughPreservesToolsAndStreamsToolUse(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            "msg_tool_1",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-tool",
			"stop_reason":   "tool_use",
			"stop_sequence": nil,
			"content": []map[string]any{{
				"type":  "tool_use",
				"id":    "toolu_1",
				"name":  "Bash",
				"input": map[string]any{"command": "cat > /app/solver.py"},
			}},
			"usage": map[string]any{"input_tokens": 13, "output_tokens": 9},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["anthropic_passthrough"] = ProviderConfig{BaseURL: upstream.URL, Dialect: "anthropic", APIKey: "provider-key"}
	cfg.Models["claude-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "anthropic_passthrough", Model: "claude-tool"}}}
	cfg.Callers[0].Allow = []string{"claude-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-tools-smoke",
		"max_tokens":256,
		"stream":true,
		"messages":[{"role":"user","content":"write solver"}],
		"tools":[{"name":"Bash","description":"run shell","input_schema":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "claude-code-test")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "claude-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	events := parseSSEEvents(t, rr.Body.String())
	start, ok := events["content_block_start"]
	if !ok {
		t.Fatalf("content block start missing: %#v", events)
	}
	block := start["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["name"] != "Bash" || block["id"] != "toolu_1" {
		t.Fatalf("tool_use block not preserved: %#v", block)
	}
	delta, ok := events["content_block_delta"]
	if !ok {
		t.Fatalf("content block delta missing: %#v", events)
	}
	inputDelta := delta["delta"].(map[string]any)
	if inputDelta["type"] != "input_json_delta" {
		t.Fatalf("tool input delta not emitted: %#v", inputDelta)
	}
	messageDelta := events["message_delta"]
	stop := messageDelta["delta"].(map[string]any)
	if stop["stop_reason"] != "tool_use" {
		t.Fatalf("stop reason not preserved: %#v", stop)
	}
}

func TestToolRequestsBypassCache(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_tool_cache",
			"object":      "response",
			"status":      "completed",
			"model":       "gpt-tool",
			"output_text": "ok",
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": "ok",
				}},
			}},
			"usage": map[string]any{"input_tokens": 3, "output_tokens": 1, "total_tokens": 4},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.Cache.Enabled = true
	cfg.Provider["openai"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "openai", Model: "gpt-tool"}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke","input":"write file","tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}]}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("tool requests should bypass cache; upstream calls=%d", calls.Load())
	}
}

func parseSSEEvents(t *testing.T, body string) map[string]map[string]any {
	t.Helper()
	events := map[string]map[string]any{}
	for _, frame := range strings.Split(body, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		var event string
		var dataLines []string
		for _, line := range strings.Split(frame, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					continue
				}
				dataLines = append(dataLines, data)
			}
		}
		if event == "" || len(dataLines) == 0 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &payload); err != nil {
			t.Fatalf("invalid JSON payload for SSE event %q: %v\nframe:\n%s", event, err, frame)
		}
		events[event] = payload
	}
	return events
}

func TestCallerAllowListRestrictsModelGroups(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_allow",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "allowed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["big-coder"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "big-model"}}}
	cfg.Models["high"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "high-model"}}}
	cfg.Callers[0].Allow = []string{"default"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	allowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	allowed.Header.Set("Authorization", "Bearer "+testToken)
	allowedRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(allowedRR, allowed)
	if allowedRR.Code != http.StatusOK {
		t.Fatalf("allowed status=%d body=%s", allowedRR.Code, allowedRR.Body.String())
	}

	for _, model := range []string{"big-coder", "high"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", model, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "model-not-allowed") {
			t.Fatalf("%s missing model-not-allowed: %s", model, rr.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("disallowed model groups should not call upstream, calls=%d", calls.Load())
	}
}

func TestModelsEndpointOnlyListsAllowedModelGroups(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["big-coder"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "big-model"}}}
	cfg.Models["high"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "high-model"}}}
	cfg.Callers[0].Allow = []string{"default"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, model := range body.Data {
		got = append(got, model.ID)
	}
	if strings.Join(got, ",") != "default" {
		t.Fatalf("models=%v, want only default", got)
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

func TestCacheHitSanitizesProviderIDAndRawPayload(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":              "provider-secret-response-id",
			"provider_secret": "raw-provider-metadata",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "cached sanitized"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": int(n), "total_tokens": int(n) + 3},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"id":"caller-specific-id","model":"default","messages":[{"role":"user","content":"same cache prompt"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		raw := rr.Body.String()
		if strings.Contains(raw, "provider-secret-response-id") || strings.Contains(raw, "raw-provider-metadata") {
			t.Fatalf("provider raw data leaked: %s", raw)
		}
		return body
	}

	first := post()
	second := post()
	if calls.Load() != 1 {
		t.Fatalf("expected cache hit, upstream calls=%d", calls.Load())
	}
	if first["id"] == "" || second["id"] == "" || first["id"] == second["id"] {
		t.Fatalf("expected fresh router IDs, first=%q second=%q", first["id"], second["id"])
	}
	if !strings.HasPrefix(first["id"].(string), "resp_") || !strings.HasPrefix(second["id"].(string), "resp_") {
		t.Fatalf("expected router response IDs, first=%q second=%q", first["id"], second["id"])
	}
}

func TestCacheKeyIgnoresRawRequestID(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "provider-id",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "same"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	for _, id := range []string{"caller-id-1", "caller-id-2"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"id":"`+id+`","model":"default","messages":[{"role":"user","content":"raw id ignored"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected second request to hit cache despite different raw id, calls=%d", calls.Load())
	}
}

func TestCacheTTLAndLRUEviction(t *testing.T) {
	resp := &IRResponse{ID: "provider-id", Model: "m", Text: "cached", Usage: Usage{TotalTokens: 1}, Raw: map[string]any{"id": "provider-id"}}
	short := newCache(CacheConfig{Enabled: true, MaxBytes: 1024, DefaultTTL: time.Nanosecond})
	short.Put("a", resp)
	time.Sleep(time.Millisecond)
	if _, ok := short.Get("a"); ok {
		t.Fatal("expected expired cache entry to miss")
	}

	lru := newCache(CacheConfig{Enabled: true, MaxBytes: 150, DefaultTTL: time.Minute})
	lru.Put("a", &IRResponse{Model: "m", Text: strings.Repeat("a", 40)})
	lru.Put("b", &IRResponse{Model: "m", Text: strings.Repeat("b", 40)})
	if _, ok := lru.Get("a"); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
	if _, ok := lru.Get("b"); !ok {
		t.Fatal("expected newest entry to remain")
	}
}

func TestCacheHitDoesNotConsumeLifetimeQuota(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "quota-cache",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "quota cache"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Callers[0].Key.LifetimeTokens = 6
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	post := func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"quota cache prompt"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	post()
	post()
	if calls.Load() != 1 {
		t.Fatalf("expected second request from cache, calls=%d", calls.Load())
	}
	usage := svc.quota.Usage(svc.quota.callers["alice"])
	keyUsage := usage["key"].(map[string]any)
	if keyUsage["lifetime_tokens"] != int64(5) {
		t.Fatalf("expected only upstream request to count against lifetime quota: %#v", keyUsage)
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
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
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
	group := cfg.Models["default"]
	group.Targets[0].InputPricePerMillionUSD = 2.5
	group.Targets[0].OutputPricePerMillionUSD = 7.5
	group.Targets[0].PricingSource = "https://example.test/pricing"
	group.Targets[0].PricingUpdatedAt = "2026-06-17"
	cfg.Models["default"] = group
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
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
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var rec logRecord
	for _, line := range lines {
		var candidate logRecord
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatal(err)
		}
		if candidate.TargetModel == "mock-model" {
			rec = candidate
			break
		}
	}
	if rec.RequestID == "" {
		t.Fatalf("chat completion record not found: %s", raw)
	}
	if rec.UpstreamMS == nil || *rec.UpstreamMS <= 0 {
		t.Fatalf("upstream duration missing: %#v", rec.UpstreamMS)
	}
	if rec.DownstreamMS == nil || *rec.DownstreamMS <= 0 {
		t.Fatalf("downstream duration missing: %#v", rec.DownstreamMS)
	}
	if rec.UpstreamOutputTPS == nil || rec.DownstreamOutputTPS == nil {
		t.Fatalf("throughput missing: upstream=%#v downstream=%#v", rec.UpstreamOutputTPS, rec.DownstreamOutputTPS)
	}
	if !rec.CacheEnabled || rec.CacheMaxBytes <= 0 {
		t.Fatalf("cache snapshot missing: enabled=%v max=%d", rec.CacheEnabled, rec.CacheMaxBytes)
	}
	if rec.CallerIP != "203.0.113.10" {
		t.Fatalf("caller ip = %q", rec.CallerIP)
	}
	if rec.InputPricePerMillionUSD != 2.5 || rec.OutputPricePerMillionUSD != 7.5 ||
		rec.InputCostUSD != 0.000005 || rec.OutputCostUSD != 0.0000225 || rec.TotalCostUSD != 0.0000275 ||
		rec.PricingSource != "https://example.test/pricing" || rec.PricingUpdatedAt != "2026-06-17" {
		t.Fatalf("cost metadata missing from log record: %#v", rec)
	}
}

func TestMetricsEndpointRequiresMetricsAdminAndExportsGlobalLabels(t *testing.T) {
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

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	bobToken := "rtr_bob_test_token"
	bobSum := sha256.Sum256([]byte(bobToken))
	adminToken := "rtr_metrics_admin_test_token"
	adminSum := sha256.Sum256([]byte(adminToken))
	cfg.Callers = append(cfg.Callers,
		CallerConfig{
			ID:          "bob",
			User:        "bob",
			Project:     "openfang-daily-reports",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(bobSum[:]),
			TokenID:     "rtr_bob_test",
			Allow:       []string{"default"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		},
		CallerConfig{
			ID:           "metrics-admin",
			User:         "ops",
			Project:      "observability",
			Environment:  "test",
			TokenSHA256:  hex.EncodeToString(adminSum[:]),
			TokenID:      "rtr_metrics_admin_test",
			Allow:        []string{"default"},
			MetricsAdmin: true,
			Rate:         RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		},
	)
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	unauth := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	unauthRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauth metrics status=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}

	for _, token := range []string{testToken, bobToken} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	for _, token := range []string{testToken, bobToken} {
		metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		metricsReq.Header.Set("Authorization", "Bearer "+token)
		metricsRR := httptest.NewRecorder()
		svc.Handler().ServeHTTP(metricsRR, metricsReq)
		if metricsRR.Code != http.StatusForbidden {
			t.Fatalf("non-admin metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
		}
		body := metricsRR.Body.String()
		if !strings.Contains(body, "metrics-forbidden") {
			t.Fatalf("non-admin metrics missing metrics-forbidden: %s", body)
		}
		if strings.Contains(body, "caller_user") || strings.Contains(body, "rtr_") || strings.Contains(body, "smart_llmrouter_") {
			t.Fatalf("non-admin metrics leaked metrics data: %s", body)
		}
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+adminToken)
	metricsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusOK {
		t.Fatalf("admin metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
	}
	body := metricsRR.Body.String()
	for _, want := range []string{
		`smart_llmrouter_requests_total`,
		`caller_id="alice"`,
		`caller_user="alice"`,
		`caller_project="metrum-insights"`,
		`caller_environment="test"`,
		`token_id="rtr_alice_test"`,
		`caller_id="bob"`,
		`caller_user="bob"`,
		`caller_project="openfang-daily-reports"`,
		`token_id="rtr_bob_test"`,
		`model_group="default"`,
		`target_provider="mock"`,
		`target_model="mock-model"`,
		`smart_llmrouter_tokens_total`,
		`smart_llmrouter_cache_bypass_total`,
		`smart_llmrouter_cache_entries`,
		`smart_llmrouter_upstream_output_tokens_per_second_sum`,
		`smart_llmrouter_downstream_output_tokens_per_second_sum`,
		`smart_llmrouter_build_info`,
		`version="`,
		`build_date="`,
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
type Target = {
  provider: string;
  model: string;
  tier?: string;
  weight: number;
  keyConfigured: boolean;
};

type RouteContext = {
  text: string;
  targets: Target[];
};

export function route(ctx: RouteContext) {
  const eligible = ctx.targets
    .map((target, index) => ({ target, index }))
    .filter((entry) => entry.target.keyConfigured && entry.target.weight > 0);

  if (eligible.length === 0) {
    return { targetIndex: 0, classLabel: "prompt-size:no-eligible-targets" };
  }

  const preferredTier = ctx.text.length > 8000 ? "heavy" : "cheap";
  const preferred = eligible.find((entry) => entry.target.tier === preferredTier) || eligible[0];

  return {
    targetIndex: preferred.index,
    fallbackIndexes: eligible
      .filter((entry) => entry.index !== preferred.index)
      .map((entry) => entry.index),
    classLabel: "prompt-size:" + preferredTier,
  };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["scripted"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 70},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 30},
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
		Messages: []IRMessage{{Role: "user", Content: "short question"}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("short script pick: %v", err)
	}
	if dec.Target.Model != "cheap-model" {
		t.Fatalf("short script pick selected %q", dec.Target.Model)
	}
	if dec.ClassLabel == nil || *dec.ClassLabel != "prompt-size:cheap" {
		t.Fatalf("short script class label=%v", dec.ClassLabel)
	}
	if len(dec.Fallbacks) == 0 || dec.Fallbacks[0].Model != "heavy-model" {
		t.Fatalf("short script fallbacks=%#v", dec.Fallbacks)
	}

	dec, err = svc.pick("scripted", cfg.Models["scripted"], &IRRequest{
		Model:    "scripted",
		Messages: []IRMessage{{Role: "user", Content: strings.Repeat("large prompt ", 900)}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("long script pick: %v", err)
	}
	if dec.Target.Model != "heavy-model" {
		t.Fatalf("long script pick selected %q", dec.Target.Model)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"scripted","messages":[{"role":"user","content":"`+strings.Repeat("large prompt ", 900)+`"}]}`))
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

func TestTypeScriptRoutingCanUseCallerTokenRegex(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_key_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script key routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
type Ctx = { caller?: { tokenId: string; user: string; project: string }; targets: Array<{ model: string }> };
export function route(ctx: Ctx) {
  if (/^rtr_alice_/.test(ctx.caller?.tokenId || "") && /^metrum-/.test(ctx.caller?.project || "")) {
    return { targetIndex: 1, classLabel: "key-regex:" + ctx.caller?.user };
  }
  return { targetIndex: 0, classLabel: "key-regex:fallback" };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["keyed"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "default-key-model"},
			{Provider: "mock", Model: "alice-key-model"},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "keyed")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"keyed","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "alice-key-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestTypeScriptRoutingCanUseBundledImportsAndAllowedHTTP(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_http_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script http routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer policy-secret" {
			t.Fatalf("missing policy auth header %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tier": "heavy"})
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "policy.ts"), []byte(`
export function chooseTier(text: string) {
  return text.includes("force") ? "heavy" : "cheap";
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
import { chooseTier } from "./policy";

type RouteContext = {
  text: string;
  targets: Array<{ tier?: string; keyConfigured: boolean; weight: number }>;
};

export function route(ctx: RouteContext) {
  const response = router.fetchJSON("`+policy.URL+`/route", {
    method: "POST",
    body: { hint: chooseTier(ctx.text) },
  });
  const tier = response.ok ? response.body.tier : chooseTier(ctx.text);
  const targetIndex = ctx.targets.findIndex((target) =>
    target.keyConfigured && target.weight > 0 && target.tier === tier
  );
  return { targetIndex: targetIndex >= 0 ? targetIndex : 0, classLabel: "external-policy:" + tier };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["script-http"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:          true,
			AllowHosts:       []string{policyURL.Hostname()},
			TimeoutMS:        500,
			MaxResponseBytes: 4096,
			Headers:          map[string]string{"Authorization": "Bearer policy-secret"},
		},
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 50},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 50},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http","messages":[{"role":"user","content":"force external policy"}]}`))
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

func TestTypeScriptRoutingRejectsHTTPHostOutsideAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when script policy fails")
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tier": "heavy"})
	}))
	defer policy.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
export function route() {
  router.fetchJSON("`+policy.URL+`/route");
  return { targetIndex: 0 };
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["script-http-blocked"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:    true,
			AllowHosts: []string{"policy.internal.example"},
			TimeoutMS:  500,
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http-blocked")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http-blocked","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "routing-failed") {
		t.Fatalf("unexpected body=%s", rr.Body.String())
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

func TestScriptCallerMetadataExcludesSecrets(t *testing.T) {
	sum := sha256.Sum256([]byte(testToken))
	caller := &callerRuntime{cfg: CallerConfig{
		ID:          "alice",
		User:        "Alice",
		Project:     "Metrum Insights",
		Environment: "Prod",
		TokenSHA256: hex.EncodeToString(sum[:]),
		TokenID:     "rtr_metrum_alice_metrum-insights_prod_key1",
		Allow:       []string{"default"},
	}}

	scriptCaller := buildScriptCaller(caller, caller.cfg.TokenID)
	if scriptCaller == nil || scriptCaller.TokenID != caller.cfg.TokenID || scriptCaller.User != "alice" || scriptCaller.Project != "metrum-insights" || scriptCaller.Environment != "prod" {
		t.Fatalf("caller metadata not populated: %#v", scriptCaller)
	}
	raw, err := json.Marshal(scriptCaller)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testToken) || strings.Contains(string(raw), caller.cfg.TokenSHA256) {
		t.Fatalf("caller secret leaked into script context: %s", raw)
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

func TestToolRequestsRequireMatchingToolSupportMetadata(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{
		BaseURL: "http://127.0.0.1:1/v1",
		Dialect: "openai-responses",
		APIKey:  "provider-key",
		Models: map[string]ProviderModel{
			"chat-tools-only": {
				Model: "chat-tools-only",
				ToolSupport: ToolSupport{
					OpenAIChat: []string{"tools", "tool_choice"},
				},
			},
			"responses-tools": {
				Model: "responses-tools",
				ToolSupport: ToolSupport{
					OpenAIResponses: []string{"function"},
				},
			},
		},
	}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "chat-tools-only", ToolOnly: true}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.supportedToolsForGroup("default"); len(got) != 0 {
		t.Fatalf("advertised tools for incompatible metadata: %#v", got)
	}
	svc.Close()

	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "responses-tools", ToolOnly: true}}}
	svc, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if got := svc.supportedToolsForGroup("default"); len(got) == 0 {
		t.Fatal("expected tools for matching responses metadata")
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
			"qwen3-coder-30b-nitro": {
				Model:            "qwen/qwen3-coder-30b-a3b-instruct:nitro",
				Weight:           1,
				InputModalities:  []string{"text", "image"},
				OutputModalities: []string{"text"},
				ToolSupport:      ToolSupport{AnthropicMessages: []string{"client_tools"}},
			},
		},
	}
	cfg.Models["openrouter-anthropic"] = ModelGroup{
		Strategy: "static",
		Targets:  []Target{{Provider: "openrouter_anthropic", ModelRef: "qwen3-coder-30b-nitro"}},
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
	if gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("upstream model=%q", gotModel)
	}
}

func TestAnthropicToolPassthroughAppliesDefaultThinking(t *testing.T) {
	var gotThinking map[string]any
	var gotToolChoice any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotThinking, _ = body["thinking"].(map[string]any)
		gotToolChoice = body["tool_choice"]
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "msg_kimi",
			"type":        "message",
			"role":        "assistant",
			"model":       body["model"],
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["kimi_anthropic"] = ProviderConfig{BaseURL: upstream.URL + "/anthropic", Dialect: "anthropic", AuthScheme: "bearer", APIKey: "kimi-key"}
	cfg.Models["kimi-tools"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:        "kimi_anthropic",
		Model:           "kimi-k2.7-code",
		ToolOnly:        true,
		DefaultThinking: map[string]any{"type": "enabled", "budget_tokens": 512},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "kimi-tools")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"kimi-tools","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"echo"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotThinking["type"] != "enabled" || gotThinking["budget_tokens"].(float64) != 512 {
		t.Fatalf("thinking=%#v, want enabled budget 512", gotThinking)
	}
	if gotToolChoice != nil {
		t.Fatalf("tool_choice=%#v, want omitted for Kimi thinking compatibility", gotToolChoice)
	}
}

func TestResponsesToolPassthroughCanUseMiniMaxTarget(t *testing.T) {
	var gotPath, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_minimax",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": "ok",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["minimax"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "minimax-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "minimax", Model: "MiniMax-M3", Dialect: "openai-responses"}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "agent-tools-smoke")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke","input":"hi","tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/responses" || gotModel != "MiniMax-M3" {
		t.Fatalf("path/model=%s/%s, want /v1/responses MiniMax-M3", gotPath, gotModel)
	}
}

func TestResponsesToolPassthroughCanUseOpenRouterResponsesTarget(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_openrouter",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": "ok",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["openrouter_responses"] = ProviderConfig{
		BaseURL: upstream.URL + "/api/v1",
		Dialect: "openai-responses",
		APIKey:  "openrouter-key",
		Models: map[string]ProviderModel{
			"qwen3-coder-30b-nitro": {Model: "qwen/qwen3-coder-30b-a3b-instruct:nitro", Weight: 1},
		},
	}
	cfg.Models["agent-tools-smoke-openrouter"] = ModelGroup{
		Strategy: "static",
		Targets:  []Target{{Provider: "openrouter_responses", ModelRef: "qwen3-coder-30b-nitro", Dialect: "openai-responses"}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "agent-tools-smoke-openrouter")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke-openrouter","input":"hi","tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/responses" || gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("path/model=%s/%s, want /api/v1/responses qwen/qwen3-coder-30b-a3b-instruct:nitro", gotPath, gotModel)
	}
	if gotAuth != "Bearer openrouter-key" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
}

func TestAnthropicToolPassthroughCanUseOpenRouterAnthropicTarget(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey, gotVersion, gotModel string
	var gotContent []any
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
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if msg, ok := messages[0].(map[string]any); ok {
				gotContent, _ = msg["content"].([]any)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "msg_openrouter_tool",
			"type":        "message",
			"role":        "assistant",
			"model":       gotModel,
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
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
			"qwen3-coder-30b-nitro": {Model: "qwen/qwen3-coder-30b-a3b-instruct:nitro", Weight: 1},
		},
	}
	cfg.Models["claude-tools-smoke-openrouter"] = ModelGroup{
		Strategy: "static",
		Targets: []Target{{
			Provider:         "openrouter_anthropic",
			ModelRef:         "qwen3-coder-30b-nitro",
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
			ToolSupport:      ToolSupport{AnthropicMessages: []string{"client_tools"}},
		}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "claude-tools-smoke-openrouter")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"claude-tools-smoke-openrouter","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"` + receiptImageURL + `"}}]}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/messages" || gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("path/model=%s/%s, want /api/v1/messages qwen/qwen3-coder-30b-a3b-instruct:nitro", gotPath, gotModel)
	}
	if gotAuth != "Bearer openrouter-key" || gotAPIKey != "" {
		t.Fatalf("unexpected auth headers Authorization=%q X-API-Key=%q", gotAuth, gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("missing Anthropic-Version, got %q", gotVersion)
	}
	if len(gotContent) != 2 {
		t.Fatalf("forwarded content=%#v, want text and image blocks", gotContent)
	}
	imageBlock, _ := gotContent[1].(map[string]any)
	source, _ := imageBlock["source"].(map[string]any)
	if imageBlock["type"] != "image" || source["type"] != "url" || source["url"] != receiptImageURL {
		t.Fatalf("forwarded image block=%#v", imageBlock)
	}
}

func TestNoEligibleTargetReturnsActionableError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when no target supports the request shape")
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:        "mock",
		Model:           "vision-chat-only",
		InputModalities: []string{"text", "image"},
		ToolSupport:     ToolSupport{OpenAIChat: []string{"tools"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")

	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
		"model":"vision",
		"max_tokens":512,
		"tools":[{"name":"noop","description":"No-op","input_schema":{"type":"object","properties":{}}}],
		"messages":[{"role":"user","content":[
			{"type":"text","text":"Read the image."},
			{"type":"image","source":{"type":"url","url":"` + receiptImageURL + `"}}
		]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) ||
		!strings.Contains(rr.Body.String(), `anthropic_tool_passthrough`) ||
		!strings.Contains(rr.Body.String(), `image`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestUpstreamFailureReturnsActionableError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary outage", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"upstream-failed"`) ||
		!strings.Contains(rr.Body.String(), `"attempts":1`) ||
		!strings.Contains(rr.Body.String(), `"provider":"mock"`) ||
		!strings.Contains(rr.Body.String(), `upstream status 503`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestUpstreamAttemptTimeoutReturnsGatewayTimeoutAndDiagnostics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "late",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "late"},
			}},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	group := cfg.Models["default"]
	group.AttemptTimeoutMS = 10
	cfg.Models["default"] = group
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"upstream-timeout"`) ||
		!strings.Contains(rr.Body.String(), `"request_id"`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
	var attempts []requestAttemptRecord
	if err := svc.usage.db.Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt rows=%d", len(attempts))
	}
	if !attempts[0].TimedOut || attempts[0].ErrorClass != "upstream_timeout" || attempts[0].AttemptTimeoutMS != 10 {
		t.Fatalf("unexpected attempt row: %#v", attempts[0])
	}
	var errors []requestErrorRecord
	if err := svc.usage.db.Find(&errors).Error; err != nil {
		t.Fatal(err)
	}
	if len(errors) != 1 || errors[0].ErrorType != "upstream-timeout" || !errors[0].Retryable {
		t.Fatalf("unexpected error rows: %#v", errors)
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
