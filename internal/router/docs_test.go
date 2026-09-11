// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsHandlerServesEmbedded404ForMissingDocsPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/missing-page", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	docsHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rr.Code)
	}
	for _, header := range []string{"X-Smart-LLMRouter-Version", "X-Smart-LLMRouter-Build-Date"} {
		if got := rr.Header().Get(header); got != "" {
			t.Fatalf("public docs exposed %s=%q", header, got)
		}
	}
	csp := rr.Header().Get("Content-Security-Policy")
	for _, required := range []string{"script-src 'self' 'unsafe-inline'", "img-src 'self' data: https://storage.googleapis.com/eleven-public-cdn/", "https://api.us.elevenlabs.io", "wss://api.us.elevenlabs.io"} {
		if !strings.Contains(csp, required) {
			t.Fatalf("docs CSP missing %q: %q", required, csp)
		}
	}
	if strings.Contains(csp, "unpkg") {
		t.Fatalf("unexpected docs CSP %q", csp)
	}
	if policy := rr.Header().Get("Permissions-Policy"); !strings.Contains(policy, "microphone=(self)") {
		t.Fatalf("unexpected permissions policy %q", policy)
	}
}

func TestDocsHandlerServesLLMsTxtWhenEmbedded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/llms.txt", nil)
	rr := httptest.NewRecorder()
	docsHandler().ServeHTTP(rr, req)

	// Without a docs build the file may be missing; when present it must be plain text.
	if rr.Code == http.StatusNotFound {
		t.Skip("docsdist has no llms.txt until make docs-build embeds docs-site/static/llms.txt")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type=%q, want text/plain", ct)
	}
	body := rr.Body.String()
	for _, required := range []string{"# Metrum Smart Router", "Category: LLM router", "Learned Routing Policy"} {
		if !strings.Contains(body, required) {
			t.Fatalf("llms.txt missing %q: %s", required, body)
		}
	}
	if strings.Contains(body, "<html") {
		t.Fatalf("llms.txt served HTML fallback: %s", body)
	}
}

func TestSecurityTextHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	rr := httptest.NewRecorder()

	securityTextHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	body := rr.Body.String()
	for _, required := range []string{
		"Contact: mailto:security@metrum.ai",
		"Expires: 2027-08-19T00:00:00.000Z",
		"Preferred-Languages: en",
		"Policy: https://github.com/metrum-ai/router/blob/main/SECURITY.md",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("security.txt missing %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{"Bearer ", "token", "54.84.", "llm-api-engg"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("security.txt contains forbidden value %q", forbidden)
		}
	}
}

func TestDocsFallbackAllowsPublicPrivacyRoutes(t *testing.T) {
	for _, route := range []string{"privacy", "dpa", "subprocessors", "transfer-schedule"} {
		if !docsFallbackRouteAllowed(route) {
			t.Fatalf("fallback route %q not allowed", route)
		}
	}
}
