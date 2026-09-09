// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConversationKeyStableAcrossDialectsWithExplicitID(t *testing.T) {
	cfg := ExternalPolicyConversationKeyConfig{Salt: "demo-salt"}
	callerID := "caller-a"
	group := "adaptive"
	want := ""
	for _, dialect := range []string{"openai-chat", "openai-responses", "anthropic"} {
		req := conversationFixtureRequest(t, dialect, "shared-thread-1", "first turn text")
		got := buildExternalPolicyConversationKey(cfg, callerID, group, req)
		if want == "" {
			want = got
		}
		if got != want {
			t.Fatalf("dialect %s conversation key %q != %q", dialect, got, want)
		}
		if !strings.HasPrefix(got, "ck_") || len(got) != len("ck_")+32 {
			t.Fatalf("unexpected conversation key format %q", got)
		}
		if conversationKeySource(req) != "conversation_id" {
			t.Fatalf("dialect %s source=%q", dialect, conversationKeySource(req))
		}
	}
}

func TestConversationKeyPolicyPayloadOmitsTurnTextForAllDialects(t *testing.T) {
	for _, dialect := range []string{"openai-chat", "openai-responses", "anthropic"} {
		t.Run(dialect, func(t *testing.T) {
			var policyPayload map[string]any
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&policyPayload); err != nil {
					t.Fatal(err)
				}
				writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0, "classLabel": "external-policy:cheap"})
			}))
			defer policy.Close()
			policyURL, err := url.Parse(policy.URL)
			if err != nil {
				t.Fatal(err)
			}
			strat := newExternalPolicyStrategy(ExternalPolicyConfig{
				URL: policy.URL + "/route", AllowHosts: []string{policyURL.Hostname()},
				TimeoutMS: 500, MaxResponseBytes: 4096,
				ConversationKey: ExternalPolicyConversationKeyConfig{Salt: "unit-salt"},
			})
			defer strat.Close()
			req := conversationFixtureRequest(t, dialect, "", "secret first turn prompt")
			targets := []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}}
			dec, err := strat.Pick(context.Background(), "conversation-key", req, nil, targets, targets, map[string]ProviderConfig{
				"mock": {Dialect: "openai-chat"},
			}, &callerRuntime{cfg: CallerConfig{ID: "alice", Allow: []string{"conversation-key"}}}, "rtr_alice_test", dialect)
			if err != nil {
				t.Fatal(err)
			}
			if dec.ConversationKey == "" || !strings.HasPrefix(dec.ConversationKey, "ck_") {
				t.Fatalf("decision conversation key=%q", dec.ConversationKey)
			}
			contextObj, _ := policyPayload["context"].(map[string]any)
			if contextObj["conversationKey"] != dec.ConversationKey {
				t.Fatalf("payload key=%v decision=%q", contextObj["conversationKey"], dec.ConversationKey)
			}
			if contextObj["conversationKeySource"] != "turn0_hash" {
				t.Fatalf("source=%v", contextObj["conversationKeySource"])
			}
			raw, _ := json.Marshal(policyPayload)
			if strings.Contains(string(raw), "secret first turn prompt") {
				t.Fatalf("policy payload leaked first-turn text: %s", raw)
			}
			if _, ok := policyPayload["request"]; ok {
				t.Fatalf("unexpected request mirror")
			}
		})
	}
}

func TestConversationKeyMultiTurnUsesSameExplicitID(t *testing.T) {
	cfg := ExternalPolicyConversationKeyConfig{}
	first := &IRRequest{
		Messages: []IRMessage{{Role: "user", Content: "hello"}},
		Raw:      map[string]any{"metadata": map[string]any{"conversation_id": "thread-9"}},
	}
	second := &IRRequest{
		Messages: []IRMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "follow up"},
		},
		Raw: map[string]any{"metadata": map[string]any{"conversation_id": "thread-9"}},
	}
	a := buildExternalPolicyConversationKey(cfg, "caller", "group", first)
	b := buildExternalPolicyConversationKey(cfg, "caller", "group", second)
	if a != b {
		t.Fatalf("multi-turn keys diverged: %q vs %q", a, b)
	}
	otherCaller := buildExternalPolicyConversationKey(cfg, "other-caller", "group", second)
	if otherCaller == a {
		t.Fatalf("conversation keys collided across callers")
	}
	otherGroup := buildExternalPolicyConversationKey(cfg, "caller", "other-group", second)
	if otherGroup == a {
		t.Fatalf("conversation keys collided across groups")
	}
}

func TestExternalPolicyCompletionFeedbackOptIn(t *testing.T) {
	var (
		mu              sync.Mutex
		feedbackPayload map[string]any
		feedbackHeaders http.Header
		gotFeedback     = make(chan struct{}, 1)
	)
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/route"):
			writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0, "classLabel": "external-policy:feedback"})
		case strings.HasSuffix(r.URL.Path, "/feedback"):
			if r.Header.Get("Authorization") != "Bearer policy-secret" {
				t.Fatalf("missing feedback auth")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			feedbackPayload = payload
			feedbackHeaders = r.Header.Clone()
			mu.Unlock()
			gotFeedback <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "feedback_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["feedback-group"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL: policy.URL + "/route", AllowHosts: []string{policyURL.Hostname()},
			TimeoutMS: 500, MaxResponseBytes: 4096,
			Headers: map[string]string{"Authorization": "Bearer policy-secret"},
			Feedback: ExternalPolicyFeedbackConfig{
				Enabled: true, MaxRetries: 1, TimeoutMS: 500,
				OnDeliveryFailure: "log",
			},
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1, InputPricePerMillionUSD: 0.1, OutputPricePerMillionUSD: 0.2}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "feedback-group")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"feedback-group","messages":[{"role":"user","content":"do not echo this prompt"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-gotFeedback:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for feedback callback")
	}
	mu.Lock()
	defer mu.Unlock()
	if feedbackPayload["schemaVersion"] != externalPolicyFeedbackSchemaVersion {
		t.Fatalf("schema=%v", feedbackPayload["schemaVersion"])
	}
	if feedbackPayload["requestId"] == "" || feedbackPayload["status"].(float64) != 200 {
		t.Fatalf("feedback payload=%#v", feedbackPayload)
	}
	target, _ := feedbackPayload["selectedTarget"].(map[string]any)
	if target["provider"] != "mock" || target["model"] != "cheap-model" {
		t.Fatalf("selectedTarget=%#v", target)
	}
	usage, _ := feedbackPayload["usage"].(map[string]any)
	if usage["inputTokens"].(float64) != 2 || usage["outputTokens"].(float64) != 3 {
		t.Fatalf("usage=%#v", usage)
	}
	raw, _ := json.Marshal(feedbackPayload)
	for _, forbidden := range []string{"do not echo this prompt", testToken, "provider-key"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("feedback leaked %q: %s", forbidden, raw)
		}
	}
	if feedbackHeaders.Get("Authorization") != "Bearer policy-secret" {
		t.Fatalf("feedback auth header missing")
	}
}

func TestExternalPolicyVerifierHintsAuthorizedMetadataOnly(t *testing.T) {
	hint := map[string]any{"kind": "exact", "version": "v1", "spec": map[string]any{"expected": "4"}}
	for _, dialect := range []string{"openai-chat", "openai-responses", "anthropic"} {
		t.Run(dialect, func(t *testing.T) {
			var policyPayload map[string]any
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&policyPayload); err != nil {
					t.Fatal(err)
				}
				writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0})
			}))
			defer policy.Close()
			policyURL, err := url.Parse(policy.URL)
			if err != nil {
				t.Fatal(err)
			}
			strat := newExternalPolicyStrategy(ExternalPolicyConfig{
				URL: policy.URL + "/route", AllowHosts: []string{policyURL.Hostname()},
				TimeoutMS: 500, MaxResponseBytes: 4096,
				VerifierHints: ExternalPolicyVerifierHintsConfig{Enabled: true, MaxSpecBytes: 1024},
			})
			defer strat.Close()
			req := conversationFixtureRequest(t, dialect, "thread-1", "2+2?")
			req.Raw["metadata"].(map[string]any)[externalPolicyVerifierHintMetaKey] = hint
			targets := []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}}
			_, err = strat.Pick(context.Background(), "verifier-hints", req, nil, targets, targets, map[string]ProviderConfig{
				"mock": {Dialect: "openai-chat"},
			}, &callerRuntime{cfg: CallerConfig{ID: "alice"}}, "rtr_alice_test", dialect)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := policyPayload["verifierHint"].(map[string]any)
			if got["kind"] != "exact" || got["version"] != "v1" {
				t.Fatalf("verifierHint=%#v", got)
			}
			spec, _ := got["spec"].(map[string]any)
			if spec["expected"] != "4" {
				t.Fatalf("spec=%#v", spec)
			}
		})
	}
}

func TestExternalPolicyVerifierHintsRejectExecutableKinds(t *testing.T) {
	cfg := ExternalPolicyVerifierHintsConfig{Enabled: true}
	_, err := normalizeExternalPolicyVerifierHint(cfg, map[string]any{
		"kind": "pytest",
		"spec": map[string]any{"tests": "import os; os.system('id')"},
	})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err=%v", err)
	}
	hint, err := extractExternalPolicyVerifierHint(ExternalPolicyVerifierHintsConfig{Enabled: false}, &IRRequest{
		Raw: map[string]any{"metadata": map[string]any{"metrum_verifier_hint": `{"kind":"exact","spec":{"expected":"1"}}`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hint != nil {
		t.Fatalf("disabled verifier hints still forwarded: %#v", hint)
	}
}

func TestValidateExternalPolicyFeedbackAndVerifierHints(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.Models["default"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL: "http://127.0.0.1:18090/route", AllowHosts: []string{"127.0.0.1"},
			Feedback: ExternalPolicyFeedbackConfig{
				Enabled: true, MaxRetries: 2, OnDeliveryFailure: "log",
			},
			VerifierHints: ExternalPolicyVerifierHintsConfig{
				Enabled: true, AllowedKinds: []string{"exact", "regex"}, MaxSpecBytes: 2048,
			},
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Models["default"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL: "http://127.0.0.1:18090/route", AllowHosts: []string{"127.0.0.1"},
			VerifierHints: ExternalPolicyVerifierHintsConfig{Enabled: true, AllowedKinds: []string{"pytest"}},
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "executable kind") {
		t.Fatalf("Validate() err=%v", err)
	}
}

func conversationFixtureRequest(t *testing.T, dialect, conversationID, text string) *IRRequest {
	t.Helper()
	meta := map[string]any{}
	if conversationID != "" {
		if dialect == "openai-responses" {
			meta["conversationId"] = conversationID
		} else {
			meta["conversation_id"] = conversationID
		}
	}
	switch dialect {
	case "openai-chat", "anthropic":
		return &IRRequest{
			Messages: []IRMessage{{Role: "user", Content: text}},
			Raw:      map[string]any{"metadata": meta},
		}
	case "openai-responses":
		return &IRRequest{
			Input:    text,
			Messages: []IRMessage{{Role: "user", Content: text}},
			Raw:      map[string]any{"metadata": meta},
		}
	default:
		t.Fatalf("unknown dialect %s", dialect)
		return nil
	}
}
