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
	if models, ok := body["models"].([]any); !ok || len(models) == 0 {
		t.Fatalf("missing Codex models field: %#v", body)
	} else if first, ok := models[0].(map[string]any); !ok || first["slug"] == "" || first["display_name"] == "" || first["base_instructions"] == "" || first["context_window"] == nil || first["max_context_window"] == nil || first["supported_reasoning_levels"] == nil || first["shell_type"] == "" || first["supported_in_api"] != true {
		t.Fatalf("missing Codex model compatibility fields: %#v", body)
	}
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
		`smart_llmrouter_cache_bypass_total`,
		`smart_llmrouter_cache_entries`,
		`smart_llmrouter_upstream_output_tokens_per_second_sum`,
		`smart_llmrouter_downstream_output_tokens_per_second_sum`,
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
	}, svc.quota.callers["alice"], "rtr_alice_test")
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
