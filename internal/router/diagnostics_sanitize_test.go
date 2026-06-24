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
	for _, want := range []string{`"prompt":[REDACTED]`, `"authorization":[REDACTED]`, `"token_hash":[REDACTED]`, `"provider_api_key":[REDACTED]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized diagnostic missing %q in %q", want, got)
		}
	}
}
