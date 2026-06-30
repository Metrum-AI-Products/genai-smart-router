package router

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestBridgeSessionStoreExpiryIsolationPruningAndConcurrency(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	store := newBridgeSessionStore()
	store.now = func() time.Time { return now }

	targetA := Target{Provider: "provider-a", Model: "model-a", Dialect: "openai-responses"}
	targetB := Target{Provider: "provider-a", Model: "model-b", Dialect: "openai-responses"}
	keyA := bridgeSessionKey(nil, "smoke-a", targetA, "raw-session-alpha")
	keyB := bridgeSessionKey(nil, "smoke-a", targetB, "raw-session-alpha")
	if keyA == keyB || keyA == "raw-session-alpha" || keyB == "raw-session-alpha" {
		t.Fatalf("session keys not isolated or hashed: keyA=%q keyB=%q", keyA, keyB)
	}

	store.Set(keyA, "resp_a", time.Minute, 10)
	store.Set(keyB, "resp_b", time.Minute, 10)
	if got, ok := store.Get(keyA); !ok || got.PreviousResponseID != "resp_a" {
		t.Fatalf("keyA lookup=%#v ok=%v", got, ok)
	}
	if got, ok := store.Get(keyB); !ok || got.PreviousResponseID != "resp_b" {
		t.Fatalf("keyB lookup=%#v ok=%v", got, ok)
	}

	now = now.Add(2 * time.Minute)
	if _, ok := store.Get(keyA); ok {
		t.Fatal("expired session entry was returned")
	}
	if got := store.Len(); got != 0 {
		t.Fatalf("expired entries not pruned, len=%d", got)
	}

	now = time.Date(2026, 6, 30, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		store.Set(fmt.Sprintf("prune-%d", i), fmt.Sprintf("resp_%d", i), time.Hour, 3)
		now = now.Add(time.Second)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("max-entry pruning len=%d, want 3", got)
	}
	for _, oldKey := range []string{"prune-0", "prune-1"} {
		if _, ok := store.Get(oldKey); ok {
			t.Fatalf("oldest key %s was not pruned", oldKey)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-%d", i%8)
			store.Set(key, fmt.Sprintf("resp_concurrent_%d", i), time.Hour, 100)
			_, _ = store.Get(key)
		}(i)
	}
	wg.Wait()

	store.Delete("concurrent-1")
	if _, ok := store.Get("concurrent-1"); ok {
		t.Fatal("deleted session entry was returned")
	}
}

func TestChatResponsesBridgeStaleStateRetryRequiresSpecificContinuationError(t *testing.T) {
	lookup := bridgeSessionLookup{
		Requested:          true,
		Key:                "hashed-session-key",
		PreviousResponseID: "resp_previous",
	}
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "previous response id field",
			body: `{"error":{"message":"previous_response_id was not found"}}`,
			want: true,
		},
		{
			name: "previous response prose",
			body: `{"error":{"message":"The previous response expired."}}`,
			want: true,
		},
		{
			name: "conversation stale state",
			body: `{"error":{"message":"conversation state is stale"}}`,
			want: true,
		},
		{
			name: "generic invalid tool schema",
			body: `{"error":{"message":"invalid tool schema: missing required field"}}`,
			want: false,
		},
		{
			name: "generic model not found",
			body: `{"error":{"message":"model not found"}}`,
			want: false,
		},
		{
			name: "generic unsupported reasoning",
			body: `{"error":{"message":"invalid reasoning effort"}}`,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chatResponsesBridgeStaleStateRetryAllowed(true, lookup, http.StatusBadRequest, []byte(tc.body), false)
			if got != tc.want {
				t.Fatalf("retryAllowed=%v, want %v for %s", got, tc.want, tc.body)
			}
		})
	}
	if chatResponsesBridgeStaleStateRetryAllowed(true, lookup, http.StatusBadRequest, []byte(`{"error":{"message":"previous_response_id was not found"}}`), true) {
		t.Fatal("already retried request should not retry again")
	}
}
