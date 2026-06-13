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
		Model       string      `json:"model"`
		System      string      `json:"system,omitempty"`
		Messages    []IRMessage `json:"messages,omitempty"`
		Input       string      `json:"input,omitempty"`
		MaxTokens   int         `json:"max_tokens,omitempty"`
		Temperature *float64    `json:"temperature,omitempty"`
		Stop        []string    `json:"stop,omitempty"`
		Provider    string      `json:"provider"`
		TargetModel string      `json:"target_model"`
	}
	raw, _ := json.Marshal(normalized{
		Model:       req.Model,
		System:      req.System,
		Messages:    req.Messages,
		Input:       req.Input,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stop:        req.Stop,
		Provider:    target.Provider,
		TargetModel: target.Model,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cacheable(req *IRRequest) bool {
	if req.Stream || req.NoCache || len(req.Tools) > 0 {
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
	cp := *ent.resp
	return &cp, true
}

func (c *responseCache) Put(key string, resp *IRResponse) {
	if c == nil || !c.enabled || resp == nil {
		return
	}
	raw, _ := json.Marshal(resp)
	size := int64(len(raw))
	if size > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.remove(el)
	}
	ent := &cacheEntry{key: key, resp: resp, size: size, expiresAt: time.Now().Add(c.ttl)}
	el := c.ll.PushFront(ent)
	c.items[key] = el
	c.bytes += size
	for c.bytes > c.maxBytes && c.ll.Len() > 0 {
		c.remove(c.ll.Back())
	}
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
