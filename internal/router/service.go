package router

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type Service struct {
	cfg          *Config
	mux          *http.ServeMux
	httpClient   *http.Client
	callersBySum map[string]*callerRuntime
	quota        *quotaStore
	cache        *responseCache
	logger       *requestLogger
	scripts      map[string]*scriptStrategy
}

type decision struct {
	Target      Target
	Fallbacks   []Target
	ClassLabel  *string
	Strategy    string
	GroupName   string
	TargetIndex int
}

type requestContext struct {
	id      string
	start   time.Time
	caller  *callerRuntime
	dialect string
	client  string
	rec     logRecord
}

func New(cfg *Config) (*Service, error) {
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	quota, err := newQuotaStore(cfg.StatePath, cfg.Callers)
	if err != nil {
		return nil, err
	}
	logger, err := newRequestLogger(cfg.Server.Logging.Path)
	if err != nil {
		return nil, err
	}
	s := &Service{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		httpClient:   &http.Client{Timeout: 10 * time.Minute},
		callersBySum: map[string]*callerRuntime{},
		quota:        quota,
		cache:        newCache(cfg.Server.Cache),
		logger:       logger,
		scripts:      map[string]*scriptStrategy{},
	}
	if err := s.loadScripts(); err != nil {
		_ = quota.Close()
		_ = logger.Close()
		return nil, err
	}
	for _, rt := range quota.callers {
		s.callersBySum[strings.ToLower(rt.cfg.TokenSHA256)] = rt
	}
	s.routes()
	return s, nil
}

func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) Close() {
	_ = s.quota.Close()
	_ = s.logger.Close()
}

func (s *Service) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	s.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.cfg.Validate(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("GET /v1/usage", s.handleUsage)
	s.mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	s.mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "anthropic") })
	s.mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "openai-chat") })
	s.mux.HandleFunc("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "openai-responses") })
}

func (s *Service) handleModels(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "models")
	if !ok {
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	data := []map[string]any{}
	for name := range rc.caller.allow {
		data = append(data, map[string]any{"id": name, "object": "model", "created": 0, "owned_by": "smart-llmrouter"})
	}
	sort.Slice(data, func(i, j int) bool { return data[i]["id"].(string) < data[j]["id"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Service) handleUsage(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "usage")
	if !ok {
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, s.quota.Usage(rc.caller))
}

func (s *Service) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "anthropic")
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
	if err != nil {
		s.writeError(w, rc, http.StatusBadRequest, "invalid-body")
		return
	}
	req, err := decodeRequest("anthropic", body, r.Header)
	if err != nil {
		s.writeError(w, rc, http.StatusBadRequest, "invalid-request")
		return
	}
	ad := s.quota.Admit(rc.caller, estimateTokens(req))
	if !ad.OK {
		s.writeAdmissionError(w, rc, ad)
		return
	}
	defer s.quota.Release(rc.caller)
	rc.rec.RequestedModel = req.Model
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	tokens := estimateTokens(req)
	rc.rec.Usage = Usage{InputTokens: tokens, TotalTokens: tokens}
	s.quota.RecordTokens(rc.caller, rc.rec.Usage)
	defer s.finish(rc, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": tokens})
}

func (s *Service) handleLLM(w http.ResponseWriter, r *http.Request, dialect string) {
	rc, ok := s.begin(w, r, dialect)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		s.writeError(w, rc, http.StatusBadRequest, "invalid-body")
		return
	}
	req, err := decodeRequest(dialect, body, r.Header)
	if err != nil {
		s.writeError(w, rc, http.StatusBadRequest, "invalid-request")
		return
	}
	rc.rec.RequestedModel = req.Model
	rc.rec.Stream = req.Stream

	ad := s.quota.Admit(rc.caller, estimateTokens(req))
	if !ad.OK {
		s.writeAdmissionError(w, rc, ad)
		return
	}
	defer s.quota.Release(rc.caller)
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	if ad.WarningText != "" {
		w.Header().Add("X-Router-Warning", ad.WarningText)
		rc.rec.Warnings = append(rc.rec.Warnings, ad.WarningText)
	}

	if !rc.caller.allow[req.Model] {
		s.writeError(w, rc, http.StatusForbidden, "model-not-allowed")
		return
	}
	group, ok := s.cfg.Models[req.Model]
	if !ok {
		s.writeError(w, rc, http.StatusForbidden, "model-not-found")
		return
	}
	dec, err := s.pick(req.Model, group, req)
	if err != nil {
		s.writeError(w, rc, http.StatusBadGateway, "routing-failed")
		return
	}
	rc.rec.ResolvedGroup = req.Model
	rc.rec.Strategy = dec.Strategy
	rc.rec.ClassLabel = dec.ClassLabel
	rc.rec.TargetProvider = dec.Target.Provider
	rc.rec.TargetModel = dec.Target.Model
	rc.rec.TargetDialect = targetDialect(s.cfg.Provider[dec.Target.Provider], dec.Target)

	key := cacheKey(req, dec.Target)
	if cacheable(req) {
		if cached, ok := s.cache.Get(key); ok {
			rc.rec.Cache = "hit"
			rc.rec.Status = http.StatusOK
			rc.rec.Usage = cached.Usage
			if req.Stream {
				s.writeIRStream(w, dialect, cached, rc)
			} else {
				s.writeIRResponse(w, dialect, cached)
			}
			s.finish(rc, http.StatusOK, nil)
			return
		}
		rc.rec.Cache = "miss"
	} else {
		rc.rec.Cache = "bypass"
	}

	resp, attempts, fallbackUsed, err := s.callUpstreams(r.Context(), w, dialect, req, dec)
	rc.rec.Attempts = attempts
	rc.rec.FallbackUsed = fallbackUsed
	if err != nil {
		s.writeError(w, rc, http.StatusBadGateway, "upstream-failed")
		return
	}
	if resp != nil {
		if cacheable(req) {
			s.cache.Put(key, resp)
		}
		quotaState, keyState := s.quota.RecordTokens(rc.caller, resp.Usage)
		rc.rec.QuotaState = quotaState
		rc.rec.KeyState = keyState
		rc.rec.Usage = resp.Usage
		rc.rec.Warnings = append(rc.rec.Warnings, resp.Warnings...)
		if req.Stream {
			s.writeIRStream(w, dialect, resp, rc)
		} else {
			s.writeIRResponse(w, dialect, resp)
		}
		s.finish(rc, http.StatusOK, nil)
	}
}

func (s *Service) begin(w http.ResponseWriter, r *http.Request, dialect string) (*requestContext, bool) {
	id := requestID()
	w.Header().Set("X-Request-Id", id)
	caller, tokenID, err := s.authenticate(r.Header.Get("Authorization"))
	rc := &requestContext{
		id:      id,
		start:   time.Now(),
		caller:  caller,
		dialect: dialect,
		client:  inferClient(r),
		rec: logRecord{
			RequestID:      id,
			Client:         inferClient(r),
			InboundDialect: dialect,
			Cache:          "bypass",
			QuotaState:     "ok",
			KeyState:       "active",
			Attempts:       0,
			Warnings:       []string{},
		},
	}
	if err != nil {
		rc.rec.TokenID = tokenID
		s.writeError(w, rc, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	rc.rec.CallerID = caller.cfg.ID
	rc.rec.TokenID = tokenID
	return rc, true
}

func (s *Service) finish(rc *requestContext, status int, code *string) {
	if rc == nil {
		return
	}
	if rc.rec.Status == 0 {
		rc.rec.Status = status
	}
	if code != nil {
		rc.rec.Error = code
	}
	rc.rec.LatencyMS = time.Since(rc.start).Milliseconds()
	s.logger.Emit(rc.rec)
}

func (s *Service) authenticate(header string) (*callerRuntime, string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return nil, "", errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	sum := sha256.Sum256([]byte(token))
	sumHex := hex.EncodeToString(sum[:])
	tokenID := tokenPrefix(token, sumHex)
	for configured, caller := range s.callersBySum {
		if subtle.ConstantTimeCompare([]byte(configured), []byte(sumHex)) == 1 {
			if caller.cfg.TokenID != "" {
				tokenID = caller.cfg.TokenID
			}
			return caller, tokenID, nil
		}
	}
	return nil, tokenID, errors.New("unknown token")
}

func (s *Service) pick(groupName string, group ModelGroup, req *IRRequest) (decision, error) {
	targets := append([]Target(nil), group.Targets...)
	if len(targets) == 0 {
		return decision{}, errors.New("no targets")
	}
	strategy := strings.ToLower(group.Strategy)
	var label *string
	switch strategy {
	case "static", "failover":
	case "weighted":
		targets = weightedOrder(targets)
	case "latency":
		sort.SliceStable(targets, func(i, j int) bool { return targets[i].RPM > targets[j].RPM })
	case "cost":
		sort.SliceStable(targets, func(i, j int) bool { return targets[i].Cost < targets[j].Cost })
	case "semantic":
		lbl, tier := classifyStub(req)
		label = &lbl
		if tier != "" {
			sort.SliceStable(targets, func(i, j int) bool {
				if targets[i].Tier == tier && targets[j].Tier != tier {
					return true
				}
				if targets[i].Tier != tier && targets[j].Tier == tier {
					return false
				}
				return i < j
			})
		}
	case "script":
		strat := s.scripts[groupName]
		if strat == nil {
			return decision{}, fmt.Errorf("script strategy %s not loaded", groupName)
		}
		return strat.Pick(groupName, req, targets, s.cfg.Provider)
	default:
		return decision{}, fmt.Errorf("unknown strategy %s", strategy)
	}
	return decision{Target: targets[0], Fallbacks: targets[1:], ClassLabel: label, Strategy: strategy, GroupName: groupName}, nil
}

func (s *Service) loadScripts() error {
	for name, group := range s.cfg.Models {
		if !strings.EqualFold(group.Strategy, "script") {
			continue
		}
		strat, err := loadScriptStrategy(s.cfg.baseDir, group.Script)
		if err != nil {
			return fmt.Errorf("load script for model group %s: %w", name, err)
		}
		s.scripts[name] = strat
	}
	return nil
}

func (s *Service) callUpstreams(ctx context.Context, w http.ResponseWriter, callerDialect string, req *IRRequest, dec decision) (*IRResponse, int, bool, error) {
	targets := append([]Target{dec.Target}, dec.Fallbacks...)
	var lastErr error
	for i, tgt := range targets {
		resp, err := s.callOne(ctx, callerDialect, req, tgt)
		if err == nil {
			return resp, i + 1, i > 0, nil
		}
		lastErr = err
		backoffMS := 100 * (1 << i)
		if backoffMS > 1000 {
			backoffMS = 1000
		}
		time.Sleep(time.Duration(backoffMS) * time.Millisecond)
	}
	return nil, len(targets), len(targets) > 1, lastErr
}

func (s *Service) callOne(ctx context.Context, callerDialect string, req *IRRequest, target Target) (*IRResponse, error) {
	provider := s.cfg.Provider[target.Provider]
	outDialect := targetDialect(provider, target)
	upReqBody, err := encodeUpstream(outDialect, target.Model, req)
	if err != nil {
		return nil, err
	}
	endpoint := upstreamEndpoint(provider.BaseURL, outDialect, target)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(upReqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		switch upstreamAuthScheme(provider, outDialect) {
		case "x-api-key":
			httpReq.Header.Set("X-API-Key", provider.APIKey)
			httpReq.Header.Set("Anthropic-Version", "2023-06-01")
		case "replicate":
			httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
			httpReq.Header.Set("Prefer", "wait=60")
		default:
			httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
			if outDialect == "anthropic" {
				httpReq.Header.Set("Anthropic-Version", "2023-06-01")
			}
		}
	}
	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500 {
		io.Copy(io.Discard, httpResp.Body)
		return nil, fmt.Errorf("retryable upstream status %d", httpResp.StatusCode)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		io.Copy(io.Discard, httpResp.Body)
		return nil, fmt.Errorf("upstream status %d", httpResp.StatusCode)
	}
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	return decodeUpstreamResponse(outDialect, raw, target.Model)
}

func (s *Service) writeIRResponse(w http.ResponseWriter, dialect string, resp *IRResponse) {
	switch dialect {
	case "anthropic":
		writeJSON(w, http.StatusOK, encodeAnthropicResponse(resp))
	case "openai-chat":
		writeJSON(w, http.StatusOK, encodeChatResponse(resp))
	case "openai-responses":
		writeJSON(w, http.StatusOK, encodeResponsesResponse(resp))
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}

func (s *Service) writeIRStream(w http.ResponseWriter, dialect string, resp *IRResponse, rc *requestContext) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	ttfb := time.Since(rc.start).Milliseconds()
	rc.rec.TTFBMS = &ttfb
	flusher, _ := w.(http.Flusher)
	writeSSE := func(event string, data any) {
		raw, _ := json.Marshal(data)
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		if flusher != nil {
			flusher.Flush()
		}
	}
	switch dialect {
	case "anthropic":
		writeSSE("message_start", map[string]any{"type": "message_start", "message": encodeAnthropicResponse(resp)})
		writeSSE("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		writeSSE("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": resp.Text}})
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		writeSSE("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": defaultString(resp.StopReason, "end_turn")}, "usage": map[string]any{"output_tokens": resp.Usage.OutputTokens}})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
	case "openai-responses":
		itemID := "msg_" + strings.TrimPrefix(resp.ID, "resp_")
		writeSSE("response.created", map[string]any{"type": "response.created", "sequence_number": 1, "response": map[string]any{"id": resp.ID, "object": "response", "status": "in_progress", "model": resp.Model, "output": []any{}}})
		writeSSE("response.in_progress", map[string]any{"type": "response.in_progress", "sequence_number": 2, "response": map[string]any{"id": resp.ID, "object": "response", "status": "in_progress", "model": resp.Model, "output": []any{}}})
		writeSSE("response.output_item.added", map[string]any{"type": "response.output_item.added", "sequence_number": 3, "output_index": 0, "item": map[string]any{"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}})
		writeSSE("response.content_part.added", map[string]any{"type": "response.content_part.added", "sequence_number": 4, "item_id": itemID, "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
		writeSSE("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "sequence_number": 5, "item_id": itemID, "output_index": 0, "content_index": 0, "delta": resp.Text})
		writeSSE("response.output_text.done", map[string]any{"type": "response.output_text.done", "sequence_number": 6, "item_id": itemID, "output_index": 0, "content_index": 0, "text": resp.Text})
		writeSSE("response.content_part.done", map[string]any{"type": "response.content_part.done", "sequence_number": 7, "item_id": itemID, "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": resp.Text, "annotations": []any{}}})
		writeSSE("response.output_item.done", map[string]any{"type": "response.output_item.done", "sequence_number": 8, "output_index": 0, "item": map[string]any{"id": itemID, "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": resp.Text, "annotations": []any{}}}}})
		completed := encodeResponsesResponse(resp)
		completed["status"] = "completed"
		writeSSE("response.completed", map[string]any{"type": "response.completed", "sequence_number": 9, "response": completed})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	default:
		writeSSE("", map[string]any{"id": resp.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": resp.Model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": resp.Text}, "finish_reason": nil}}})
		writeSSE("", map[string]any{"id": resp.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": resp.Model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": defaultString(resp.StopReason, "stop")}}, "usage": map[string]any{"prompt_tokens": resp.Usage.InputTokens, "completion_tokens": resp.Usage.OutputTokens, "total_tokens": resp.Usage.TotalTokens}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Service) writeAdmissionError(w http.ResponseWriter, rc *requestContext, ad admission) {
	if ad.RetryAfter != "" {
		w.Header().Set("Retry-After", ad.RetryAfter)
	}
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	s.writeError(w, rc, ad.Status, ad.Reason)
}

func (s *Service) writeError(w http.ResponseWriter, rc *requestContext, status int, code string) {
	if rc != nil {
		rc.rec.Status = status
		c := code
		rc.rec.Error = &c
		s.finish(rc, status, &c)
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"type": code, "message": code}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(b[:])
}

func tokenPrefix(token, sum string) string {
	if len(token) >= 12 {
		return token[:12]
	}
	if len(sum) >= 12 {
		return sum[:12]
	}
	return sum
}

func inferClient(r *http.Request) string {
	if v := r.Header.Get("X-Router-Client"); v != "" {
		return v
	}
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "claude"):
		return "claude-code"
	case strings.Contains(ua, "codex"):
		return "codex"
	case ua != "":
		return r.UserAgent()
	default:
		return "unknown"
	}
}

func targetDialect(provider ProviderConfig, target Target) string {
	if d := normalizeDialect(target.Dialect); d != "" {
		return d
	}
	return normalizeDialect(provider.Dialect)
}

func upstreamAuthScheme(provider ProviderConfig, dialect string) string {
	if scheme := normalizeAuthScheme(provider.AuthScheme); scheme != "" && scheme != "default" {
		return scheme
	}
	switch dialect {
	case "anthropic":
		return "x-api-key"
	case "replicate":
		return "replicate"
	default:
		return "bearer"
	}
}

func upstreamEndpoint(base, dialect string, target Target) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	switch dialect {
	case "anthropic":
		u.Path = joinPath(u.Path, "/v1/messages")
	case "replicate":
		owner, name, ok := strings.Cut(strings.Trim(target.Model, "/"), "/")
		if ok && owner != "" && name != "" {
			u.Path = "/v1/models/" + owner + "/" + name + "/predictions"
		} else {
			u.Path = joinPath(u.Path, "/v1/predictions")
		}
	case "openai-responses":
		u.Path = joinPath(u.Path, "/responses")
	default:
		u.Path = joinPath(u.Path, "/chat/completions")
	}
	return u.String()
}

func joinPath(basePath, suffix string) string {
	if strings.HasSuffix(basePath, suffix) {
		return basePath
	}
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(suffix, "/") {
		return basePath + suffix
	}
	return path.Join(basePath, suffix)
}

func weightedOrder(targets []Target) []Target {
	expanded := []Target{}
	for _, t := range targets {
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		for i := 0; i < w; i++ {
			expanded = append(expanded, t)
		}
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(expanded))))
	pick := expanded[n.Int64()]
	out := []Target{pick}
	for _, t := range targets {
		if t.Provider == pick.Provider && t.Model == pick.Model {
			continue
		}
		out = append(out, t)
	}
	return out
}

func classifyStub(req *IRRequest) (string, string) {
	text := strings.ToLower(req.System + " " + req.Input)
	for _, m := range req.Messages {
		text += " " + strings.ToLower(m.Content)
	}
	if strings.Contains(text, "reason") || strings.Contains(text, "complex") || strings.Contains(text, "architecture") {
		return "reasoning:0.90", "heavy"
	}
	return "simple:0.80", "cheap"
}

func contentTypeIsSSE(h http.Header) bool {
	mt, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	return mt == "text/event-stream"
}
