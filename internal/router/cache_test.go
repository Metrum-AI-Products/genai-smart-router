package router

import (
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
