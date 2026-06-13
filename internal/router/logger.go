package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type requestLogger struct {
	mu   sync.Mutex
	path string
	file *os.File
}

type logRecord struct {
	TS                string   `json:"ts"`
	RequestID         string   `json:"request_id"`
	CallerID          string   `json:"caller_id"`
	CallerUser        string   `json:"caller_user"`
	CallerProject     string   `json:"caller_project"`
	CallerEnvironment string   `json:"caller_environment"`
	TokenID           string   `json:"token_id"`
	Client            string   `json:"client"`
	InboundDialect    string   `json:"inbound_dialect"`
	RequestedModel    string   `json:"requested_model"`
	ResolvedGroup     string   `json:"resolved_group"`
	Strategy          string   `json:"strategy"`
	ClassLabel        *string  `json:"class_label"`
	TargetProvider    string   `json:"target_provider"`
	TargetModel       string   `json:"target_model"`
	TargetDialect     string   `json:"target_dialect"`
	Stream            bool     `json:"stream"`
	Cache             string   `json:"cache"`
	Status            int      `json:"status"`
	Attempts          int      `json:"attempts"`
	FallbackUsed      bool     `json:"fallback_used"`
	LatencyMS         int64    `json:"latency_ms"`
	TTFBMS            *int64   `json:"ttfb_ms"`
	Usage             Usage    `json:"usage"`
	QuotaState        string   `json:"quota_state"`
	KeyState          string   `json:"key_state"`
	Warnings          []string `json:"warnings"`
	Error             *string  `json:"error"`
}

func newRequestLogger(path string) (*requestLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &requestLogger{path: path, file: f}, nil
}

func (l *requestLogger) Emit(rec logRecord) {
	if l == nil {
		return
	}
	if rec.TS == "" {
		rec.TS = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(raw, '\n'))
}

func (l *requestLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
