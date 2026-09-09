// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheStats(t *testing.T) {
	cache := newCache(CacheConfig{Enabled: true, MaxBytes: 4096, DefaultTTL: time.Minute})
	stats := cache.Stats()
	if !stats.Enabled || stats.Items != 0 || stats.Bytes != 0 || stats.MaxBytes != 4096 || stats.OccupancyPct != 0 {
		t.Fatalf("empty stats = %#v", stats)
	}

	cache.Put("key", &IRResponse{Model: "model", Text: "cached", Usage: Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5}})
	stats = cache.Stats()
	if !stats.Enabled || stats.Items != 1 || stats.Bytes <= 0 || stats.MaxBytes != 4096 || stats.OccupancyPct <= 0 {
		t.Fatalf("populated stats = %#v", stats)
	}
}

func TestImageRequestsAreNotCacheable(t *testing.T) {
	req := &IRRequest{Messages: []IRMessage{{
		Role:    "user",
		Content: "Read the receipt.",
		Parts: []IRContentPart{
			{Type: "text", Text: "Read the receipt."},
			{Type: "image", ImageURL: receiptImageURL},
		},
	}}}
	if cacheable(req) {
		t.Fatal("image request should bypass cache")
	}
}

func TestCacheKeyDistinguishesOpenAIChatCapField(t *testing.T) {
	maxTokensReq, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"max_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	maxCompletionTokensReq, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"max_completion_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if maxTokensReq.MaxTokens != maxCompletionTokensReq.MaxTokens {
		t.Fatalf("test setup produced different canonical caps: %d vs %d", maxTokensReq.MaxTokens, maxCompletionTokensReq.MaxTokens)
	}

	target := Target{Provider: "openai_chat", Model: "chat-model"}
	maxTokensKey := cacheKey(maxTokensReq, target)
	maxCompletionTokensKey := cacheKey(maxCompletionTokensReq, target)
	if maxTokensKey == maxCompletionTokensKey {
		t.Fatalf("cache key reused across cap fields: %s", maxTokensKey)
	}
}
