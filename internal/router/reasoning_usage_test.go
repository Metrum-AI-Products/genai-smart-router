package router

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestUsageFromMapPreservesReasoningTokenPresence(t *testing.T) {
	reported := usageFromMap(map[string]any{"prompt_tokens": 4, "completion_tokens": 3, "completion_tokens_details": map[string]any{"reasoning_tokens": 0}})
	if reported.ReasoningTokens == nil || *reported.ReasoningTokens != 0 {
		t.Fatalf("reported zero lost: %#v", reported)
	}
	missing := usageFromMap(map[string]any{"prompt_tokens": 4, "completion_tokens": 3})
	if missing.ReasoningTokens != nil {
		t.Fatalf("missing field became a value: %#v", missing)
	}
}

func TestReasoningUsageColumnsPersistAsNullableScalars(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, column := range []string{"reasoning_tokens", "reasoning_attempt_count", "reasoning_successful_attempt_count", "reasoning_reported_attempt_count"} {
		if !store.db.Migrator().HasColumn(&usageRecord{}, column) {
			t.Fatalf("request_usage missing %s", column)
		}
	}
	if !store.db.Migrator().HasColumn(&requestAttemptRecord{}, "reasoning_tokens") {
		t.Fatal("request_attempts missing reasoning_tokens")
	}
	zero := 0
	if err := store.db.Create(&requestAttemptRecord{RequestID: "reasoning-zero", AttemptIndex: 1, TS: "2026-07-23T00:00:00.000Z", Provider: "mock", Model: "mock", Dialect: "openai-chat", EndpointHost: "mock", ReasoningTokens: &zero}).Error; err != nil {
		t.Fatal(err)
	}
	var got requestAttemptRecord
	if err := store.db.First(&got, "request_id = ? AND attempt_index = ?", "reasoning-zero", 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.ReasoningTokens == nil || *got.ReasoningTokens != 0 {
		t.Fatalf("nullable zero did not round trip: %#v", got)
	}
}

func TestReasoningUsageResponseEncodingAndAttemptCoverage(t *testing.T) {
	n := 7
	usage := Usage{InputTokens: 2, OutputTokens: 10, TotalTokens: 12, ReasoningTokens: &n}
	chat := encodeChatResponse(&IRResponse{Usage: usage})["usage"].(map[string]any)
	if got := chat["completion_tokens_details"].(map[string]any)["reasoning_tokens"]; got != 7 {
		t.Fatalf("chat reasoning usage=%#v", chat)
	}
	responses := encodeResponsesResponse(&IRResponse{Usage: usage})["usage"].(map[string]any)
	if got := responses["output_tokens_details"].(map[string]any)["reasoning_tokens"]; got != 7 {
		t.Fatalf("responses reasoning usage=%#v", responses)
	}
	zero := 0
	total, attempts, successful, reported := reasoningUsageCoverage([]attemptLogRecord{{StatusCode: 502}, {StatusCode: 200, ReasoningTokens: &zero}, {StatusCode: 200, ReasoningTokens: &n}})
	if total == nil || *total != 7 || attempts != 3 || successful != 2 || reported != 2 {
		t.Fatalf("coverage total=%v attempts=%d successful=%d reported=%d", total, attempts, successful, reported)
	}
	if absent, _, _, count := reasoningUsageCoverage([]attemptLogRecord{{StatusCode: 200}}); absent != nil || count != 0 {
		t.Fatalf("absence was not preserved: total=%v reported=%d", absent, count)
	}
	stream := httptest.NewRecorder()
	writeChatTextSSE(func(_ string, v any) { _ = json.NewEncoder(stream).Encode(v) }, stream, &IRResponse{ID: "id", Model: "model", Usage: usage})
	if !bytes.Contains(stream.Body.Bytes(), []byte("reasoning_tokens")) {
		t.Fatalf("streaming usage lost reasoning tokens: %s", stream.Body.String())
	}
	bridged, err := decodeResponsesToChatBridge([]byte(`{"id":"chat","usage":{"prompt_tokens":2,"completion_tokens":10,"completion_tokens_details":{"reasoning_tokens":7}},"choices":[{"message":{"content":"ok"}}]}`), "model")
	if err != nil || bridged.Raw["usage"].(map[string]any)["output_tokens_details"].(map[string]any)["reasoning_tokens"] != 7 {
		t.Fatalf("Responses-to-Chat bridge lost reasoning usage: %#v, %v", bridged, err)
	}
}
