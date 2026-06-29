package router

import (
	"net/http"
	"net/http/httptest"
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
	if rr.Header().Get("X-Smart-LLMRouter-Version") == "" {
		t.Fatalf("missing docs version header")
	}
}
