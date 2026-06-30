package router

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultBridgeStatefulSessionHeader = "X-Router-Session"

type bridgeSessionStore struct {
	mu      sync.Mutex
	entries map[string]bridgeSessionEntry
	now     func() time.Time
}

type bridgeSessionEntry struct {
	PreviousResponseID string
	ExpiresAt          time.Time
	UpdatedAt          time.Time
}

type bridgeSessionLookup struct {
	Enabled            bool
	Requested          bool
	Header             string
	Key                string
	PreviousResponseID string
	TTL                time.Duration
	MaxEntries         int
}

func newBridgeSessionStore() *bridgeSessionStore {
	return &bridgeSessionStore{
		entries: map[string]bridgeSessionEntry{},
		now:     time.Now,
	}
}

func (s *bridgeSessionStore) Get(key string) (bridgeSessionEntry, bool) {
	if s == nil || strings.TrimSpace(key) == "" {
		return bridgeSessionEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok {
		return bridgeSessionEntry{}, false
	}
	now := s.now()
	if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(now) {
		delete(s.entries, key)
		return bridgeSessionEntry{}, false
	}
	return entry, true
}

func (s *bridgeSessionStore) Set(key, previousResponseID string, ttl time.Duration, maxEntries int) {
	if s == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(previousResponseID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	entry := bridgeSessionEntry{PreviousResponseID: previousResponseID, UpdatedAt: now}
	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}
	s.entries[key] = entry
	s.pruneLocked(now, maxEntries)
}

func (s *bridgeSessionStore) Delete(key string) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

func (s *bridgeSessionStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.pruneLocked(now, 0)
	return len(s.entries)
}

func (s *bridgeSessionStore) pruneLocked(now time.Time, maxEntries int) {
	for key, entry := range s.entries {
		if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(now) {
			delete(s.entries, key)
		}
	}
	if maxEntries <= 0 || len(s.entries) <= maxEntries {
		return
	}
	type candidate struct {
		key       string
		updatedAt time.Time
	}
	candidates := make([]candidate, 0, len(s.entries))
	for key, entry := range s.entries {
		candidates = append(candidates, candidate{key: key, updatedAt: entry.UpdatedAt})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].updatedAt.Before(candidates[j].updatedAt)
	})
	for len(s.entries) > maxEntries && len(candidates) > 0 {
		delete(s.entries, candidates[0].key)
		candidates = candidates[1:]
	}
}

func (s *Service) chatToResponsesBridgeSession(rc *requestContext, groupName string, target Target) bridgeSessionLookup {
	cfg := bridgeStatefulSessionConfig(target)
	if rc == nil || !cfg.Enabled || !isChatToResponsesBridge(rc.dialect, targetDialect(s.cfg.Provider[target.Provider], target), target) {
		return bridgeSessionLookup{}
	}
	header := bridgeStatefulSessionHeader(cfg)
	sessionID := ""
	if rc != nil && rc.bridgeSessionHeaders != nil {
		sessionID = rc.bridgeSessionHeaders[strings.ToLower(header)]
	}
	if strings.TrimSpace(sessionID) == "" {
		return bridgeSessionLookup{Enabled: true, Header: header}
	}
	key := bridgeSessionKey(rc, groupName, target, sessionID)
	lookup := bridgeSessionLookup{
		Enabled:    true,
		Requested:  true,
		Header:     header,
		Key:        key,
		TTL:        bridgeStatefulSessionTTL(cfg),
		MaxEntries: bridgeStatefulSessionMaxEntries(cfg),
	}
	if entry, ok := s.bridgeSessions.Get(key); ok {
		lookup.PreviousResponseID = entry.PreviousResponseID
	}
	return lookup
}

func bridgeStatefulSessionConfig(target Target) BridgeStatefulSessionsConfig {
	cfg := target.Bridges.ChatToResponses.StatefulSessions
	if strings.TrimSpace(cfg.Backend) == "" {
		cfg.Backend = "memory"
	}
	if strings.TrimSpace(cfg.SessionHeader) == "" {
		cfg.SessionHeader = defaultBridgeStatefulSessionHeader
	}
	if cfg.TTLSeconds == 0 {
		cfg.TTLSeconds = 3600
	}
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 10000
	}
	return cfg
}

func bridgeStatefulSessionHeader(cfg BridgeStatefulSessionsConfig) string {
	header := strings.TrimSpace(cfg.SessionHeader)
	if header == "" {
		header = defaultBridgeStatefulSessionHeader
	}
	return http.CanonicalHeaderKey(header)
}

func bridgeStatefulSessionTTL(cfg BridgeStatefulSessionsConfig) time.Duration {
	if cfg.TTLSeconds <= 0 {
		return time.Hour
	}
	return time.Duration(cfg.TTLSeconds) * time.Second
}

func bridgeStatefulSessionMaxEntries(cfg BridgeStatefulSessionsConfig) int {
	if cfg.MaxEntries <= 0 {
		return 10000
	}
	return cfg.MaxEntries
}

func collectBridgeSessionHeaders(headers http.Header, targets []Target) map[string]string {
	values := map[string]string{}
	for _, target := range targets {
		cfg := target.Bridges.ChatToResponses.StatefulSessions
		if !cfg.Enabled {
			continue
		}
		header := bridgeStatefulSessionHeader(cfg)
		if value := strings.TrimSpace(headers.Get(header)); value != "" {
			values[strings.ToLower(header)] = value
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func bridgeSessionKey(rc *requestContext, groupName string, target Target, sessionID string) string {
	parts := []string{
		"chat-to-responses",
		groupName,
		target.Provider,
		target.Model,
		target.Dialect,
		strings.TrimSpace(sessionID),
	}
	if rc != nil {
		if rc.caller != nil {
			parts = append(parts,
				rc.caller.cfg.ID,
				rc.caller.ownerUser,
				rc.caller.project,
				rc.caller.cfg.Environment,
			)
		}
		parts = append(parts, publicTokenID(rc.rec.TokenID))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func chatResponsesBridgeStaleStateRetryAllowed(bridge bool, lookup bridgeSessionLookup, status int, raw []byte, alreadyRetried bool) bool {
	if !bridge || alreadyRetried || !lookup.Requested || strings.TrimSpace(lookup.PreviousResponseID) == "" || strings.TrimSpace(lookup.Key) == "" {
		return false
	}
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
	default:
		return false
	}
	text := strings.ToLower(string(raw))
	if text == "" {
		return false
	}
	for _, needle := range []string{
		"previous_response_id",
		"previous response",
		"previous responses",
		"prior response",
		"response id is stale",
		"response id has expired",
		"response id not found",
		"conversation expired",
		"conversation not found",
		"conversation state",
		"conversation is stale",
		"stale conversation",
		"expired conversation",
		"stale state",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func validHTTPHeaderName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 {
			return false
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", r) {
			return false
		}
	}
	return true
}
