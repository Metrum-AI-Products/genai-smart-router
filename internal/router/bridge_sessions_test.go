package router

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
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

	ctx := context.Background()
	if err := store.Set(ctx, keyA, "resp_a", time.Minute, 10); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, keyB, "resp_b", time.Minute, 10); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Get(ctx, keyA); err != nil || !ok || got.PreviousResponseID != "resp_a" {
		t.Fatalf("keyA lookup=%#v ok=%v", got, ok)
	}
	if got, ok, err := store.Get(ctx, keyB); err != nil || !ok || got.PreviousResponseID != "resp_b" {
		t.Fatalf("keyB lookup=%#v ok=%v", got, ok)
	}

	now = now.Add(2 * time.Minute)
	if _, ok, err := store.Get(ctx, keyA); err != nil || ok {
		t.Fatal("expired session entry was returned")
	}
	if got := store.Len(); got != 0 {
		t.Fatalf("expired entries not pruned, len=%d", got)
	}

	now = time.Date(2026, 6, 30, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if err := store.Set(ctx, fmt.Sprintf("prune-%d", i), fmt.Sprintf("resp_%d", i), time.Hour, 3); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("max-entry pruning len=%d, want 3", got)
	}
	for _, oldKey := range []string{"prune-0", "prune-1"} {
		if _, ok, err := store.Get(ctx, oldKey); err != nil || ok {
			t.Fatalf("oldest key %s was not pruned", oldKey)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-%d", i%8)
			_ = store.Set(ctx, key, fmt.Sprintf("resp_concurrent_%d", i), time.Hour, 100)
			_, _, _ = store.Get(ctx, key)
		}(i)
	}
	wg.Wait()

	if err := store.Delete(ctx, "concurrent-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Get(ctx, "concurrent-1"); err != nil || ok {
		t.Fatal("deleted session entry was returned")
	}
}

func TestRedisBridgeSessionKeyHashesSessionMaterialAndNamespaces(t *testing.T) {
	rawSessionMaterial := "raw-session-alpha"
	target := Target{Provider: "provider-a", Model: "model-a", Dialect: "openai-responses"}
	hashed := bridgeSessionKey(nil, "smoke-a", target, rawSessionMaterial)
	keyA := bridgeRedisSessionKey("prod-router", hashed)
	keyB := bridgeRedisSessionKey("staging-router", hashed)
	if keyA == keyB {
		t.Fatalf("redis keys were not namespace-isolated: %q", keyA)
	}
	for _, forbidden := range []string{rawSessionMaterial, "provider-a", "model-a", "smoke-a"} {
		if strings.Contains(keyA, forbidden) || strings.Contains(keyB, forbidden) {
			t.Fatalf("redis key leaked raw namespace material %q: %q %q", forbidden, keyA, keyB)
		}
	}
	if !strings.Contains(keyA, "prod-router") || !strings.Contains(keyB, "staging-router") {
		t.Fatalf("redis keys do not include configured deployment namespaces: %q %q", keyA, keyB)
	}
}

func TestRedisBridgeSessionStoreIntegrationOptional(t *testing.T) {
	addr := strings.TrimSpace(os.Getenv("SMART_ROUTER_REDIS_TEST_ADDR"))
	if addr == "" {
		t.Skip("set SMART_ROUTER_REDIS_TEST_ADDR to run Redis bridge session integration test")
	}
	cfg := BridgeStatefulSessionsRedisConfig{
		Address:          addr,
		Namespace:        "test-smart-router",
		PasswordEnv:      "SMART_ROUTER_REDIS_TEST_PASSWORD",
		ConnectTimeoutMS: 500,
		ReadTimeoutMS:    500,
		WriteTimeoutMS:   500,
	}
	if os.Getenv("SMART_ROUTER_REDIS_TEST_PASSWORD") == "" {
		cfg.PasswordEnv = ""
	}
	store, err := newRedisBridgeSessionStore(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := store.Set(context.Background(), key, "resp_redis", 50*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Get(context.Background(), key); err != nil || !ok || got.PreviousResponseID != "resp_redis" {
		t.Fatalf("redis get=%#v ok=%v err=%v", got, ok, err)
	}
	time.Sleep(100 * time.Millisecond)
	if got, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("redis ttl get=%#v ok=%v err=%v", got, ok, err)
	}
	if err := store.Set(context.Background(), key, "resp_redis_delete", time.Minute, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("redis delete get=%#v ok=%v err=%v", got, ok, err)
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
