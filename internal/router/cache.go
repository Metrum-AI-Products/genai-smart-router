// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

type responseCache struct {
	mu       sync.Mutex
	enabled  bool
	maxBytes int64
	ttl      time.Duration
	bytes    int64
	ll       *list.List
	items    map[string]*list.Element
}

type cacheStats struct {
	Enabled      bool
	Items        int64
	Bytes        int64
	MaxBytes     int64
	OccupancyPct float64
}

type cacheEntry struct {
	key       string
	resp      *IRResponse
	size      int64
	expiresAt time.Time
}

func newCache(cfg CacheConfig) *responseCache {
	return &responseCache{
		enabled:  cfg.Enabled,
		maxBytes: cfg.MaxBytes,
		ttl:      cfg.DefaultTTL,
		ll:       list.New(),
		items:    map[string]*list.Element{},
	}
}

func cacheKey(req *IRRequest, target Target) string {
	type normalized struct {
		Model          string      `json:"model"`
		System         string      `json:"system,omitempty"`
		Messages       []IRMessage `json:"messages,omitempty"`
		Input          string      `json:"input,omitempty"`
		MaxTokens      int         `json:"max_tokens,omitempty"`
		MaxTokensField string      `json:"max_tokens_field,omitempty"`
		Temperature    *float64    `json:"temperature,omitempty"`
		Stop           []string    `json:"stop,omitempty"`
		Provider       string      `json:"provider"`
		TargetModel    string      `json:"target_model"`
	}
	raw, _ := json.Marshal(normalized{
		Model:          req.Model,
		System:         req.System,
		Messages:       req.Messages,
		Input:          req.Input,
		MaxTokens:      req.MaxTokens,
		MaxTokensField: req.MaxTokensField,
		Temperature:    req.Temperature,
		Stop:           req.Stop,
		Provider:       target.Provider,
		TargetModel:    target.Model,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cacheable(req *IRRequest) bool {
	if req.Stream || req.NoCache || len(req.Tools) > 0 || requestHasImages(req) || requestHasStructuredOutput(req) {
		return false
	}
	if req.Temperature != nil && *req.Temperature > 0 {
		return false
	}
	return true
}

func (c *responseCache) Get(key string) (*IRResponse, bool) {
	if c == nil || !c.enabled {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*cacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.remove(el)
		return nil, false
	}
	c.ll.MoveToFront(el)
	return cloneCachedResponse(ent.resp), true
}

func (c *responseCache) Put(key string, resp *IRResponse) {
	if c == nil || !c.enabled || resp == nil {
		return
	}
	cached := sanitizeCachedResponse(resp)
	raw, _ := json.Marshal(cached)
	size := int64(len(raw))
	if size > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.remove(el)
	}
	ent := &cacheEntry{key: key, resp: cached, size: size, expiresAt: time.Now().Add(c.ttl)}
	el := c.ll.PushFront(ent)
	c.items[key] = el
	c.bytes += size
	for c.bytes > c.maxBytes && c.ll.Len() > 0 {
		c.remove(c.ll.Back())
	}
}

func (c *responseCache) Stats() cacheStats {
	if c == nil {
		return cacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.removeExpiredLocked(now)
	stats := cacheStats{
		Enabled:  c.enabled,
		Items:    int64(len(c.items)),
		Bytes:    c.bytes,
		MaxBytes: c.maxBytes,
	}
	if stats.MaxBytes > 0 {
		stats.OccupancyPct = float64(stats.Bytes) * 100 / float64(stats.MaxBytes)
	}
	return stats
}

func (c *responseCache) removeExpiredLocked(now time.Time) {
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		ent := el.Value.(*cacheEntry)
		if now.After(ent.expiresAt) {
			c.remove(el)
		}
		el = prev
	}
}

func sanitizeCachedResponse(resp *IRResponse) *IRResponse {
	if resp == nil {
		return nil
	}
	out := &IRResponse{
		Model:      resp.Model,
		Text:       resp.Text,
		StopReason: resp.StopReason,
		Usage:      resp.Usage,
	}
	if len(resp.Warnings) > 0 {
		out.Warnings = append([]string(nil), resp.Warnings...)
	}
	return out
}

func cloneCachedResponse(resp *IRResponse) *IRResponse {
	if resp == nil {
		return nil
	}
	out := *resp
	if len(resp.Warnings) > 0 {
		out.Warnings = append([]string(nil), resp.Warnings...)
	}
	out.ID = ""
	out.Raw = nil
	out.Headers = nil
	return &out
}

func (c *responseCache) remove(el *list.Element) {
	if el == nil {
		return
	}
	ent := el.Value.(*cacheEntry)
	delete(c.items, ent.key)
	c.bytes -= ent.size
	c.ll.Remove(el)
}
