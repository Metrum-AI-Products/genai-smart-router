// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAddRouterWarningHeadersSetsLegacyAndMetrumAlias(t *testing.T) {
	h := make(http.Header)
	addRouterWarningHeaders(h, "rolling quota soft cap reached")
	if got := h.Get(headerRouterWarning); got != "rolling quota soft cap reached" {
		t.Fatalf("%s = %q", headerRouterWarning, got)
	}
	if got := h.Get(headerMetrumWarning); got != "rolling quota soft cap reached" {
		t.Fatalf("%s = %q", headerMetrumWarning, got)
	}
}

func TestSetRouterErrorClassHeadersSetsLegacyAndMetrumAlias(t *testing.T) {
	h := make(http.Header)
	setRouterErrorClassHeaders(h, "upstream_bad_request")
	if got := h.Get(headerRouterErrorClass); got != "upstream_bad_request" {
		t.Fatalf("%s = %q", headerRouterErrorClass, got)
	}
	if got := h.Get(headerMetrumErrorClass); got != "upstream_bad_request" {
		t.Fatalf("%s = %q", headerMetrumErrorClass, got)
	}
}

func TestQuotaSoftWarningSetsMetrumAlias(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_1",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Quota.Day.Requests = 1
	cfg.Callers[0].Quota.SoftPct = 80
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"default","messages":[{"role":"user","content":"hi"}]}`,
	))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	want := "rolling quota soft cap reached"
	if got := rr.Header().Get("X-Router-Warning"); got != want {
		t.Fatalf("X-Router-Warning = %q, want %q; headers=%#v", got, want, rr.Header())
	}
	if got := rr.Header().Get("X-Metrum-Warning"); got != want {
		t.Fatalf("X-Metrum-Warning = %q, want %q; headers=%#v", got, want, rr.Header())
	}
}
