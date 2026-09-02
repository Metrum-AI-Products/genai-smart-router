// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeDiagnosticTextRedactsInvalidJSONLikeSensitiveFields(t *testing.T) {
	const (
		rawPrompt      = "summarize confidential acquisition notes"
		rawAuth        = "Bearer provider-token-1234567890"
		rawTokenHash   = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		rawProviderKey = "sk-provider-key-1234567890"
	)
	input := `upstream status 500: {"error":{"message":"provider echoed context","prompt":"` + rawPrompt + `","authorization":"` + rawAuth + `","token_hash":"` + rawTokenHash + `","provider_api_key":"` + rawProviderKey + `","details":"truncated`
	if json.Valid([]byte(strings.TrimPrefix(input, "upstream status 500: "))) {
		t.Fatal("test input must remain invalid JSON")
	}

	got := sanitizeDiagnosticText(input, true, 4096)
	for _, forbidden := range []string{rawPrompt, rawAuth, rawTokenHash, rawProviderKey} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized diagnostic leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "upstream status 500: upstream error body redacted") {
		t.Fatalf("sanitized diagnostic missing redaction marker in %q", got)
	}
}

func TestSanitizeDiagnosticTextRedactsFreeformPromptEcho(t *testing.T) {
	const rawPrompt = "please summarize payroll notes for jane@example.test"
	input := "upstream status 400: invalid request near " + rawPrompt
	got := sanitizeDiagnosticText(input, true, 4096)
	if strings.Contains(got, rawPrompt) || strings.Contains(got, "jane@example.test") {
		t.Fatalf("sanitized diagnostic leaked prompt echo in %q", got)
	}
	if got != "upstream status 400: upstream error body redacted" {
		t.Fatalf("sanitized diagnostic=%q", got)
	}
}
