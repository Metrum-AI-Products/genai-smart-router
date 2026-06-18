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
	"math"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"
)

type Service struct {
	cfg          *Config
	mux          *http.ServeMux
	httpClient   *http.Client
	callersBySum map[string]*callerRuntime
	quota        *quotaStore
	cache        *responseCache
	logger       *requestLogger
	usage        *usageStore
	metrics      *metricsStore
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

type routingEligibilityError struct {
	Model        string
	Dialect      string
	Requirements []string
}

func (e routingEligibilityError) Error() string {
	return fmt.Sprintf("no eligible targets for model group %q", e.Model)
}

type requestContext struct {
	id       string
	start    time.Time
	caller   *callerRuntime
	dialect  string
	client   string
	rec      logRecord
	traceSeq int
}

type upstreamError struct {
	Class       string
	Message     string
	StatusCode  int
	Retryable   bool
	TimedOut    bool
	Canceled    bool
	ResponseLen int64
	Err         error
}

func (e upstreamError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Class
}

func (e upstreamError) Unwrap() error {
	return e.Err
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
	usage, err := newUsageStore(cfg.Server.UsageDB)
	if err != nil {
		_ = quota.Close()
		_ = logger.Close()
		return nil, err
	}
	s := &Service{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		httpClient:   &http.Client{Timeout: time.Duration(cfg.Server.Upstream.TimeoutMS) * time.Millisecond},
		callersBySum: map[string]*callerRuntime{},
		quota:        quota,
		cache:        newCache(cfg.Server.Cache),
		logger:       logger,
		usage:        usage,
		metrics:      newMetricsStore(),
		scripts:      map[string]*scriptStrategy{},
	}
	if err := s.loadScripts(); err != nil {
		_ = quota.Close()
		_ = logger.Close()
		_ = usage.Close()
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
	_ = s.usage.Close()
}

func (s *Service) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthPayload(true, ""))
	})
	s.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.cfg.Validate(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, healthPayload(false, err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, healthPayload(true, ""))
	})
	s.mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, buildinfo.Current())
	})
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("GET /v1/usage", s.handleUsage)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	s.mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "anthropic") })
	s.mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "openai-chat") })
	s.mux.HandleFunc("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) { s.handleLLM(w, r, "openai-responses") })
	s.mux.Handle("GET /", docsHandler())
}

func healthPayload(ok bool, errorText string) map[string]any {
	info := buildinfo.Current()
	payload := map[string]any{
		"ok":         ok,
		"version":    info.Version,
		"commit":     info.Commit,
		"build_date": info.BuildDate,
	}
	if errorText != "" {
		payload["error"] = errorText
	}
	return payload
}

func (s *Service) handleModels(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "models")
	if !ok {
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	data := []map[string]any{}
	for name := range rc.caller.allow {
		internalModalities := s.supportedInputModalitiesForGroup(name)
		publicModalities := publicModelInputModalities(internalModalities)
		hasImage := stringSliceContains(internalModalities, "image")
		data = append(data, map[string]any{
			"id":                               name,
			"slug":                             name,
			"name":                             name,
			"display_name":                     name,
			"description":                      "Smart LLM Router model group " + name,
			"mode":                             "default",
			"base_instructions":                "You are Codex, a coding agent using Smart LLM Router.",
			"context_window":                   131072,
			"max_context_window":               131072,
			"effective_context_window_percent": 95,
			"default_reasoning_level":          "none",
			"default_reasoning_summary":        "none",
			"default_verbosity":                "low",
			"supported_reasoning_levels":       []string{},
			"supports_reasoning_summaries":     false,
			"supports_parallel_tool_calls":     len(s.supportedToolsForGroup(name)) > 0,
			"supports_search_tool":             false,
			"supports_image_detail_original":   hasImage,
			"support_verbosity":                true,
			"apply_patch_tool_type":            "freeform",
			"web_search_tool_type":             "text_and_image",
			"additional_speed_tiers":           []string{},
			"service_tiers":                    []map[string]any{{"id": "default", "name": "Default", "description": "Default Smart LLM Router service tier"}},
			"experimental_supported_tools":     s.supportedToolsForGroup(name),
			"input_modalities":                 publicModalities,
			"model_messages":                   map[string]any{"instructions_template": "", "instructions_variables": map[string]any{}},
			"truncation_policy":                map[string]any{"mode": "tokens", "limit": 10000},
			"shell_type":                       "shell_command",
			"visibility":                       "list",
			"minimal_client_version":           "0.0.0",
			"supported_in_api":                 true,
			"availability_nux":                 nil,
			"upgrade":                          nil,
			"priority":                         1000,
			"object":                           "model",
			"created":                          0,
			"owned_by":                         "smart-llmrouter",
		})
	}
	sort.Slice(data, func(i, j int) bool { return data[i]["id"].(string) < data[j]["id"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "mode": "default", "data": data, "models": data})
}

func (s *Service) handleUsage(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "usage")
	if !ok {
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, s.quota.Usage(rc.caller))
}

func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "metrics")
	if !ok {
		return
	}
	if !rc.caller.cfg.MetricsAdmin {
		code := "metrics-forbidden"
		s.writeError(w, rc, http.StatusForbidden, code)
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, s.metrics.Prometheus())
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
	dec, err := s.pick(req.Model, group, req, dialect, rc.caller, rc.rec.TokenID)
	if err != nil {
		var eligibilityErr routingEligibilityError
		if errors.As(err, &eligibilityErr) {
			s.writeRoutingEligibilityError(w, rc, eligibilityErr)
			return
		}
		s.writeError(w, rc, http.StatusBadGateway, "routing-failed")
		return
	}
	rc.rec.ResolvedGroup = req.Model
	rc.rec.Strategy = dec.Strategy
	rc.rec.ClassLabel = dec.ClassLabel
	rc.rec.InputHasImage = requestHasImages(req)
	rc.rec.InputImageCount = requestImageCount(req)
	rc.rec.TargetProvider = dec.Target.Provider
	rc.rec.TargetModel = dec.Target.Model
	rc.rec.TargetDialect = targetDialect(s.cfg.Provider[dec.Target.Provider], dec.Target)
	rc.rec.InputPricePerMillionUSD = dec.Target.InputPricePerMillionUSD
	rc.rec.OutputPricePerMillionUSD = dec.Target.OutputPricePerMillionUSD
	rc.rec.ImageInputPricePerMillionTokensUSD = dec.Target.ImageInputPricePerMillionTokensUSD
	rc.rec.ImageInputPricePerImageUSD = dec.Target.ImageInputPricePerImageUSD
	rc.rec.PricingSource = dec.Target.PricingSource
	rc.rec.PricingUpdatedAt = dec.Target.PricingUpdatedAt

	key := cacheKey(req, dec.Target)
	if cacheable(req) {
		if cached, ok := s.cache.Get(key); ok {
			ensureResponseID(cached)
			rc.rec.Cache = "hit"
			rc.rec.Status = http.StatusOK
			rc.rec.Usage = cached.Usage
			rc.trace("cache_hit", "", dec.Target, 0, http.StatusOK, "", false, 0)
			s.writeIR(w, dialect, cached, req.Stream, rc)
			s.finish(rc, http.StatusOK, nil)
			return
		}
		rc.rec.Cache = "miss"
		rc.trace("cache_miss", "", dec.Target, 0, 0, "", false, 0)
	} else {
		rc.rec.Cache = "bypass"
		rc.trace("cache_bypass", "", dec.Target, 0, 0, "", false, 0)
	}

	upstreamStart := time.Now()
	rc.trace("upstream_start", "", dec.Target, 0, 0, "", false, 0)
	resp, attempts, fallbackUsed, err := s.callUpstreams(r.Context(), rc, dialect, req, dec)
	upstreamMS := durationMillis(time.Since(upstreamStart))
	rc.rec.UpstreamMS = &upstreamMS
	rc.rec.Attempts = attempts
	rc.rec.FallbackUsed = fallbackUsed
	if err != nil {
		s.writeUpstreamFailureError(w, rc, req, dec, attempts, err)
		return
	}
	if resp != nil {
		ensureResponseID(resp)
		if cacheable(req) {
			s.cache.Put(key, resp)
		}
		quotaState, keyState := s.quota.RecordTokens(rc.caller, resp.Usage)
		rc.rec.QuotaState = quotaState
		rc.rec.KeyState = keyState
		rc.rec.Usage = resp.Usage
		rc.rec.InputImageTokens = resp.Usage.InputImageTokens
		rc.rec.UpstreamReportedInputCostUSD = resp.Usage.UpstreamReportedInputCostUSD
		rc.rec.UpstreamReportedOutputCostUSD = resp.Usage.UpstreamReportedOutputCostUSD
		rc.rec.UpstreamReportedTotalCostUSD = resp.Usage.UpstreamReportedTotalCostUSD
		rc.rec.Warnings = append(rc.rec.Warnings, resp.Warnings...)
		s.writeIR(w, dialect, resp, req.Stream, rc)
		s.finish(rc, http.StatusOK, nil)
	}
}

func (s *Service) begin(w http.ResponseWriter, r *http.Request, dialect string) (*requestContext, bool) {
	id := requestID()
	w.Header().Set("X-Request-Id", id)
	caller, tokenID, err := s.authenticate(r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))
	rc := &requestContext{
		id:      id,
		start:   time.Now(),
		caller:  caller,
		dialect: dialect,
		client:  inferClient(r),
		rec: logRecord{
			RequestID:      id,
			Client:         inferClient(r),
			CallerIP:       callerIP(r),
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
		rc.trace("auth_rejected", err.Error(), Target{}, 0, http.StatusUnauthorized, "unauthorized", false, 0)
		s.writeError(w, rc, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	rc.rec.CallerID = caller.cfg.ID
	rc.rec.CallerUser = callerUser(caller.cfg)
	rc.rec.CallerProject = callerProject(caller.cfg)
	rc.rec.CallerEnvironment = callerEnvironment(caller.cfg)
	rc.rec.TokenID = tokenID
	rc.trace("request_accepted", "", Target{}, 0, 0, "", false, 0)
	return rc, true
}

func (s *Service) finish(rc *requestContext, status int, code *string) {
	if rc == nil {
		return
	}
	if !s.diagnosticsEnabled() {
		rc.rec.AttemptsDetail = nil
		rc.rec.TraceEvents = nil
	}
	s.recordCacheStats(rc)
	populateThroughput(&rc.rec)
	populateCosts(&rc.rec)
	if rc.rec.Status == 0 {
		rc.rec.Status = status
	}
	if code != nil {
		rc.rec.Error = code
	}
	rc.rec.LatencyMS = time.Since(rc.start).Milliseconds()
	s.metrics.Observe(rc.rec)
	s.logger.Emit(rc.rec)
	s.usage.Emit(rc.rec)
}

func (s *Service) diagnosticsEnabled() bool {
	if s == nil || s.cfg == nil {
		return true
	}
	enabled := s.cfg.Server.Diagnostics.Enabled
	return enabled == nil || *enabled
}

func (rc *requestContext) trace(event, message string, target Target, attempt, status int, errorClass string, retryable bool, durationMS int64) {
	if rc == nil {
		return
	}
	rc.traceSeq++
	rec := traceLogRecord{
		Seq:        rc.traceSeq,
		TS:         time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Event:      event,
		Message:    message,
		Provider:   target.Provider,
		Model:      target.Model,
		Dialect:    target.Dialect,
		Attempt:    attempt,
		StatusCode: status,
		ErrorClass: errorClass,
		Retryable:  retryable,
		DurationMS: durationMS,
	}
	rc.rec.TraceEvents = append(rc.rec.TraceEvents, rec)
}

func (s *Service) recordCacheStats(rc *requestContext) {
	if s == nil || rc == nil {
		return
	}
	stats := s.cache.Stats()
	rc.rec.CacheEnabled = stats.Enabled
	rc.rec.CacheItems = stats.Items
	rc.rec.CacheBytes = stats.Bytes
	rc.rec.CacheMaxBytes = stats.MaxBytes
	rc.rec.CacheOccupancyPct = stats.OccupancyPct
}

func (s *Service) authenticate(header, apiKey string) (*callerRuntime, string, error) {
	const prefix = "Bearer "
	var token string
	if strings.HasPrefix(header, prefix) {
		token = strings.TrimSpace(strings.TrimPrefix(header, prefix))
	} else if strings.TrimSpace(apiKey) != "" {
		token = strings.TrimSpace(apiKey)
	}
	if token == "" {
		return nil, "missing-token", errors.New("missing bearer token")
	}
	sum := sha256.Sum256([]byte(token))
	sumHex := hex.EncodeToString(sum[:])
	for configured, caller := range s.callersBySum {
		if subtle.ConstantTimeCompare([]byte(configured), []byte(sumHex)) == 1 {
			tokenID := "sha256:" + sumHex[:12]
			if caller.cfg.TokenID != "" {
				tokenID = publicTokenID(caller.cfg.TokenID)
			}
			return caller, tokenID, nil
		}
	}
	return nil, "invalid-token", errors.New("unknown token")
}

func (s *Service) pick(groupName string, group ModelGroup, req *IRRequest, callerDialect string, caller *callerRuntime, tokenID string) (decision, error) {
	targets := append([]Target(nil), group.Targets...)
	targets = s.targetsForRequest(targets, req, callerDialect)
	if len(targets) == 0 {
		return decision{}, routingEligibilityError{
			Model:        groupName,
			Dialect:      callerDialect,
			Requirements: routingRequirements(req, callerDialect),
		}
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
		return strat.Pick(groupName, req, targets, s.cfg.Provider, caller, tokenID)
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
		strat, err := loadScriptStrategy(s.cfg.baseDir, group.Script, group.ScriptHTTP)
		if err != nil {
			return fmt.Errorf("load script for model group %s: %w", name, err)
		}
		s.scripts[name] = strat
	}
	return nil
}

func (s *Service) callUpstreams(ctx context.Context, rc *requestContext, callerDialect string, req *IRRequest, dec decision) (*IRResponse, int, bool, error) {
	targets := append([]Target{dec.Target}, dec.Fallbacks...)
	var lastErr error
	for i, tgt := range targets {
		if err := ctx.Err(); err != nil {
			lastErr = classifyContextError(err)
			break
		}
		attemptIndex := i + 1
		resp, attempt, err := s.callOne(ctx, callerDialect, req, dec.GroupName, tgt, attemptIndex)
		if attempt.ErrorMessage != "" {
			attempt.ErrorMessage = s.sanitizeDiagnosticError(attempt.ErrorMessage)
		}
		if err == nil {
			attempt.Selected = true
			rc.rec.AttemptsDetail = append(rc.rec.AttemptsDetail, attempt)
			rc.trace("upstream_attempt_ok", "", tgt, attemptIndex, attempt.StatusCode, "", false, attempt.DurationMS)
			return resp, attemptIndex, i > 0, nil
		}
		if i < len(targets)-1 {
			attempt.FallbackReason = classifyError(err).Class
		}
		rc.rec.AttemptsDetail = append(rc.rec.AttemptsDetail, attempt)
		classified := classifyError(err)
		rc.trace("upstream_attempt_failed", classified.Message, tgt, attemptIndex, attempt.StatusCode, classified.Class, classified.Retryable, attempt.DurationMS)
		lastErr = err
		if classified.Canceled {
			break
		}
		backoffMS := 100 * (1 << i)
		if backoffMS > 1000 {
			backoffMS = 1000
		}
		timer := time.NewTimer(time.Duration(backoffMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			lastErr = classifyContextError(ctx.Err())
			rc.trace("client_canceled", ctx.Err().Error(), tgt, attemptIndex, 499, "client_canceled", false, 0)
			return nil, attemptIndex, attemptIndex > 1, lastErr
		case <-timer.C:
		}
	}
	return nil, len(targets), len(targets) > 1, lastErr
}

func (s *Service) callOne(ctx context.Context, callerDialect string, req *IRRequest, groupName string, target Target, attemptIndex int) (*IRResponse, attemptLogRecord, error) {
	provider := s.cfg.Provider[target.Provider]
	outDialect := targetDialect(provider, target)
	attempt := attemptLogRecord{
		Index:            attemptIndex,
		TS:               time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Provider:         target.Provider,
		Model:            target.Model,
		Dialect:          outDialect,
		EndpointHost:     endpointHost(provider.BaseURL),
		AttemptTimeoutMS: s.attemptTimeoutMS(groupName, target),
	}
	passthrough := toolPassthrough(callerDialect, outDialect, req)
	var upReqBody []byte
	var err error
	if passthrough {
		upReqBody, err = encodeToolPassthrough(outDialect, target.Model, req, target)
	} else {
		upReqBody, err = encodeUpstream(outDialect, target.Model, req)
	}
	attempt.RequestBytes = int64(len(upReqBody))
	if err != nil {
		attempt.ErrorClass = "encode_error"
		attempt.ErrorMessage = err.Error()
		return nil, attempt, upstreamError{Class: "encode_error", Message: err.Error(), Err: err}
	}
	endpoint := upstreamEndpoint(provider.BaseURL, outDialect, target)
	attemptCtx := ctx
	var cancel context.CancelFunc
	if attempt.AttemptTimeoutMS > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(attempt.AttemptTimeoutMS)*time.Millisecond)
		defer cancel()
	}
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(upReqBody))
	if err != nil {
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ErrorClass = "request_build_error"
		attempt.ErrorMessage = err.Error()
		return nil, attempt, upstreamError{Class: "request_build_error", Message: err.Error(), Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for name, value := range provider.Headers {
		httpReq.Header.Set(name, value)
	}
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
		attempt.DurationMS = durationMillis(time.Since(start))
		upErr := classifyContextOrNetworkError(ctx, attemptCtx, err)
		attempt.ErrorClass = upErr.Class
		attempt.ErrorMessage = upErr.Message
		attempt.Retryable = upErr.Retryable
		attempt.TimedOut = upErr.TimedOut
		attempt.ClientCanceled = upErr.Canceled
		return nil, attempt, upErr
	}
	defer httpResp.Body.Close()
	attempt.StatusCode = httpResp.StatusCode
	if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500 {
		raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, int64(s.diagnosticMaxErrorBytes())))
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ResponseBytes = int64(len(raw))
		attempt.ErrorClass = statusErrorClass(httpResp.StatusCode)
		attempt.ErrorMessage = upstreamStatusMessage(httpResp.StatusCode, raw)
		attempt.Retryable = true
		return nil, attempt, upstreamError{Class: attempt.ErrorClass, Message: attempt.ErrorMessage, StatusCode: httpResp.StatusCode, Retryable: true, ResponseLen: attempt.ResponseBytes}
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, int64(s.diagnosticMaxErrorBytes())))
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ResponseBytes = int64(len(raw))
		attempt.ErrorClass = statusErrorClass(httpResp.StatusCode)
		attempt.ErrorMessage = upstreamStatusMessage(httpResp.StatusCode, raw)
		return nil, attempt, upstreamError{Class: attempt.ErrorClass, Message: attempt.ErrorMessage, StatusCode: httpResp.StatusCode, ResponseLen: attempt.ResponseBytes}
	}
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ErrorClass = "read_error"
		attempt.ErrorMessage = err.Error()
		attempt.Retryable = true
		return nil, attempt, upstreamError{Class: "read_error", Message: err.Error(), Retryable: true, Err: err}
	}
	attempt.DurationMS = durationMillis(time.Since(start))
	attempt.ResponseBytes = int64(len(raw))
	if passthrough {
		resp, err := decodeToolPassthrough(outDialect, raw, target.Model)
		if err != nil {
			attempt.ErrorClass = "decode_error"
			attempt.ErrorMessage = err.Error()
			return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Err: err}
		}
		return resp, attempt, nil
	}
	resp, err := decodeUpstreamResponse(outDialect, raw, target.Model)
	if err != nil {
		attempt.ErrorClass = "decode_error"
		attempt.ErrorMessage = err.Error()
		return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Err: err}
	}
	return resp, attempt, nil
}

func (s *Service) attemptTimeoutMS(groupName string, target Target) int {
	if target.TimeoutMS > 0 {
		return target.TimeoutMS
	}
	if group, ok := s.cfg.Models[groupName]; ok && group.AttemptTimeoutMS > 0 {
		return group.AttemptTimeoutMS
	}
	return s.cfg.Server.Upstream.DefaultAttemptTimeoutMS
}

func endpointHost(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func (s *Service) diagnosticMaxErrorBytes() int {
	if s == nil || s.cfg == nil || s.cfg.Server.Diagnostics.MaxErrorBytes <= 0 {
		return 2048
	}
	return s.cfg.Server.Diagnostics.MaxErrorBytes
}

func (s *Service) sanitizeDiagnosticError(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	if !s.cfg.Server.Diagnostics.StoreSanitizedUpstreamError {
		if i := strings.Index(text, ":"); i > 0 {
			text = strings.TrimSpace(text[:i])
		}
	}
	maxBytes := s.diagnosticMaxErrorBytes()
	if maxBytes > 0 && len(text) > maxBytes {
		text = text[:maxBytes]
	}
	return text
}

func statusErrorClass(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "upstream_rate_limited"
	case status >= 500:
		return "upstream_status_5xx"
	default:
		return "upstream_status"
	}
}

func upstreamStatusMessage(status int, raw []byte) string {
	msg := fmt.Sprintf("upstream status %d", status)
	snippet := strings.TrimSpace(string(raw))
	if snippet == "" {
		return msg
	}
	return msg + ": " + snippet
}

func classifyContextError(err error) upstreamError {
	if errors.Is(err, context.Canceled) {
		return upstreamError{Class: "client_canceled", Message: "client canceled request", Canceled: true}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return upstreamError{Class: "upstream_timeout", Message: "upstream request timed out", TimedOut: true, Retryable: true, Err: err}
	}
	return upstreamError{Class: "upstream_error", Message: err.Error(), Retryable: true, Err: err}
}

func classifyContextOrNetworkError(parentCtx, attemptCtx context.Context, err error) upstreamError {
	if parentCtx.Err() != nil {
		return classifyContextError(parentCtx.Err())
	}
	if attemptCtx.Err() != nil {
		return classifyContextError(attemptCtx.Err())
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return classifyContextError(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return upstreamError{Class: "upstream_timeout", Message: "upstream request timed out", TimedOut: true, Retryable: true, Err: err}
	}
	return upstreamError{Class: "upstream_network_error", Message: err.Error(), Retryable: true, Err: err}
}

func classifyError(err error) upstreamError {
	if err == nil {
		return upstreamError{}
	}
	var upErr upstreamError
	if errors.As(err, &upErr) {
		return upErr
	}
	return upstreamError{Class: "upstream_error", Message: err.Error(), Retryable: true, Err: err}
}

func errorTypeRetryable(errorType string) bool {
	switch errorType {
	case "upstream-timeout", "upstream-rate-limited", "upstream-failed":
		return true
	default:
		return false
	}
}

func toolPassthrough(callerDialect, outDialect string, req *IRRequest) bool {
	return callerDialect == outDialect && len(req.Tools) > 0 && (outDialect == "openai-responses" || outDialect == "anthropic" || outDialect == "openai-chat")
}

func (s *Service) targetsForRequest(targets []Target, req *IRRequest, callerDialect string) []Target {
	requiredModalities := requestInputModalities(req)
	if len(req.Tools) == 0 {
		out := make([]Target, 0, len(targets))
		for _, target := range targets {
			if !target.ToolOnly && targetSupportsInputModalities(target, requiredModalities) {
				out = append(out, target)
			}
		}
		return out
	}
	out := make([]Target, 0, len(targets))
	for _, target := range targets {
		provider := s.cfg.Provider[target.Provider]
		outDialect := targetDialect(provider, target)
		if toolPassthrough(callerDialect, outDialect, req) &&
			targetSupportsTools(target, outDialect) &&
			targetSupportsInputModalities(target, requiredModalities) {
			out = append(out, target)
		}
	}
	return out
}

func routingRequirements(req *IRRequest, callerDialect string) []string {
	requirements := requestInputModalities(req)
	if len(req.Tools) > 0 {
		requirements = append(requirements, "tools", callerDialect+"_tool_passthrough")
	}
	return requirements
}

func targetSupportsInputModalities(target Target, required []string) bool {
	targetModalities := defaultModalities(target.InputModalities)
	for _, req := range required {
		if !stringSliceContains(targetModalities, req) {
			return false
		}
	}
	return true
}

func targetSupportsTools(target Target, dialect string) bool {
	switch dialect {
	case "openai-responses":
		if toolSupportEmpty(target.ToolSupport) {
			return true
		}
		return len(target.ToolSupport.OpenAIResponses) > 0
	case "anthropic":
		if toolSupportEmpty(target.ToolSupport) {
			return true
		}
		return len(target.ToolSupport.AnthropicMessages) > 0
	case "openai", "openai-chat":
		if toolSupportEmpty(target.ToolSupport) {
			return false
		}
		return len(target.ToolSupport.OpenAIChat) > 0
	default:
		return false
	}
}

func encodeToolPassthrough(dialect, model string, req *IRRequest, target Target) ([]byte, error) {
	switch dialect {
	case "anthropic":
		return encodeAnthropicPassthrough(model, req, target.DefaultThinking)
	case "openai-chat":
		return encodeChatPassthrough(model, req)
	default:
		return encodeResponsesPassthrough(model, req)
	}
}

func decodeToolPassthrough(dialect string, raw []byte, model string) (*IRResponse, error) {
	switch dialect {
	case "anthropic":
		return decodeAnthropicPassthrough(raw, model)
	case "openai-chat":
		return decodeChatPassthrough(raw, model)
	default:
		return decodeResponsesPassthrough(raw, model)
	}
}

func (s *Service) supportedToolsForGroup(name string) []string {
	group, ok := s.cfg.Models[name]
	if !ok {
		return []string{}
	}
	req := &IRRequest{Tools: []map[string]any{{"type": "local_shell"}}}
	if len(s.targetsForRequest(group.Targets, req, "openai-responses")) == 0 {
		if len(s.targetsForRequest(group.Targets, req, "anthropic")) == 0 {
			if len(s.targetsForRequest(group.Targets, req, "openai-chat")) == 0 {
				return []string{}
			}
		}
	}
	return []string{"local_shell", "apply_patch"}
}

func (s *Service) supportedInputModalitiesForGroup(name string) []string {
	group, ok := s.cfg.Models[name]
	if !ok {
		return []string{"text"}
	}
	seen := map[string]bool{"text": true}
	for _, target := range group.Targets {
		for _, modality := range defaultModalities(target.InputModalities) {
			seen[modality] = true
		}
	}
	out := []string{"text"}
	for _, modality := range []string{"image", "video", "audio", "pdf", "file", "embeddings"} {
		if seen[modality] {
			out = append(out, modality)
		}
	}
	return out
}

func publicModelInputModalities(modalities []string) []string {
	out := []string{"text"}
	if stringSliceContains(modalities, "image") {
		out = append(out, "image")
	}
	return out
}

func (s *Service) writeIR(w http.ResponseWriter, dialect string, resp *IRResponse, stream bool, rc *requestContext) {
	start := time.Now()
	if stream {
		s.writeIRStream(w, dialect, resp, rc)
	} else {
		s.writeIRResponse(w, dialect, resp)
	}
	downstreamMS := durationMillis(time.Since(start))
	rc.rec.DownstreamMS = &downstreamMS
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
		if resp.RawResponse && resp.Raw != nil {
			writeRawAnthropicSSE(writeSSE, resp)
			break
		}
		writeSSE("message_start", map[string]any{"type": "message_start", "message": encodeAnthropicResponse(resp)})
		writeSSE("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		writeSSE("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": resp.Text}})
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		writeSSE("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": defaultString(resp.StopReason, "end_turn")}, "usage": map[string]any{"output_tokens": resp.Usage.OutputTokens}})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
	case "openai-responses":
		if resp.RawResponse && resp.Raw != nil {
			writeRawResponsesSSE(writeSSE, resp)
			break
		}
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
	case "openai-chat":
		if resp.RawResponse && resp.Raw != nil {
			writeRawChatSSE(writeSSE, w, resp)
			break
		}
		writeChatTextSSE(writeSSE, w, resp)
	default:
		writeChatTextSSE(writeSSE, w, resp)
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func writeChatTextSSE(writeSSE func(string, any), w http.ResponseWriter, resp *IRResponse) {
	writeSSE("", map[string]any{"id": resp.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": resp.Model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": resp.Text}, "finish_reason": nil}}})
	writeSSE("", map[string]any{"id": resp.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": resp.Model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": defaultString(resp.StopReason, "stop")}}, "usage": map[string]any{"prompt_tokens": resp.Usage.InputTokens, "completion_tokens": resp.Usage.OutputTokens, "total_tokens": resp.Usage.TotalTokens}})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func writeRawChatSSE(writeSSE func(string, any), w http.ResponseWriter, resp *IRResponse) {
	raw := resp.Raw
	id := defaultString(stringValue(raw["id"]), resp.ID)
	model := defaultString(stringValue(raw["model"]), resp.Model)
	created := time.Now().Unix()
	if n, ok := numberAsInt(raw["created"]); ok {
		created = int64(n)
	}
	writeSSE("", map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}})
	finishReason := defaultString(resp.StopReason, "stop")
	if choices := valueAsSlice(raw["choices"]); len(choices) > 0 {
		if ch, ok := choices[0].(map[string]any); ok {
			finishReason = defaultString(stringValue(ch["finish_reason"]), finishReason)
			if msg, ok := ch["message"].(map[string]any); ok {
				if content := contentToText(msg["content"]); content != "" {
					writeSSE("", map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": nil}}})
				}
				if calls := valueAsSlice(msg["tool_calls"]); len(calls) > 0 {
					writeSSE("", map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{"tool_calls": chatToolCallDeltas(calls)}, "finish_reason": nil}}})
				}
			}
		}
	}
	writeSSE("", map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": finishReason}}, "usage": raw["usage"]})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func chatToolCallDeltas(calls []any) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for i, call := range calls {
		c, ok := call.(map[string]any)
		if !ok {
			continue
		}
		delta := map[string]any{"index": i}
		for _, key := range []string{"id", "type"} {
			if v, ok := c[key]; ok {
				delta[key] = v
			}
		}
		if fn, ok := c["function"].(map[string]any); ok {
			fnDelta := map[string]any{}
			for _, key := range []string{"name", "arguments"} {
				if v, ok := fn[key]; ok {
					fnDelta[key] = v
				}
			}
			delta["function"] = fnDelta
		}
		out = append(out, delta)
	}
	return out
}

func writeRawAnthropicSSE(writeSSE func(string, any), resp *IRResponse) {
	raw := resp.Raw
	message := cloneMap(raw)
	content := valueAsSlice(raw["content"])
	message["content"] = []any{}
	message["stop_reason"] = nil
	writeSSE("message_start", map[string]any{"type": "message_start", "message": message})
	for idx, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType := stringValue(block["type"])
		startBlock := cloneMap(block)
		switch blockType {
		case "tool_use":
			input := startBlock["input"]
			startBlock["input"] = map[string]any{}
			writeSSE("content_block_start", map[string]any{"type": "content_block_start", "index": idx, "content_block": startBlock})
			if input != nil {
				rawInput, _ := json.Marshal(input)
				writeSSE("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": idx,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": string(rawInput)},
				})
			}
		default:
			text := stringValue(block["text"])
			startBlock["text"] = ""
			writeSSE("content_block_start", map[string]any{"type": "content_block_start", "index": idx, "content_block": startBlock})
			if text != "" {
				writeSSE("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": idx,
					"delta": map[string]any{"type": "text_delta", "text": text},
				})
			}
		}
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
	}
	writeSSE("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": defaultString(resp.StopReason, "end_turn"), "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": resp.Usage.OutputTokens},
	})
	writeSSE("message_stop", map[string]any{"type": "message_stop"})
}

func writeRawResponsesSSE(writeSSE func(string, any), resp *IRResponse) {
	raw := resp.Raw
	if raw["id"] == nil {
		raw["id"] = resp.ID
	}
	if raw["object"] == nil {
		raw["object"] = "response"
	}
	if raw["model"] == nil {
		raw["model"] = resp.Model
	}
	created := cloneMap(raw)
	created["status"] = "in_progress"
	created["output"] = []any{}
	writeSSE("response.created", map[string]any{"type": "response.created", "sequence_number": 1, "response": created})
	writeSSE("response.in_progress", map[string]any{"type": "response.in_progress", "sequence_number": 2, "response": created})
	seq := 3
	if outputs, ok := raw["output"].([]any); ok {
		for idx, item := range outputs {
			itemMap, _ := item.(map[string]any)
			if itemMap == nil {
				continue
			}
			writeSSE("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": seq,
				"output_index":    idx,
				"item":            itemMap,
			})
			seq++
			writeSSE("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": seq,
				"output_index":    idx,
				"item":            itemMap,
			})
			seq++
		}
	}
	completed := cloneMap(raw)
	if completed["status"] == nil {
		completed["status"] = "completed"
	}
	writeSSE("response.completed", map[string]any{"type": "response.completed", "sequence_number": seq, "response": completed})
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func populateThroughput(rec *logRecord) {
	if rec == nil {
		return
	}
	rec.UpstreamOutputTPS = tokensPerSecond(rec.Usage.OutputTokens, rec.UpstreamMS)
	rec.UpstreamTotalTPS = tokensPerSecond(totalTokens(rec.Usage), rec.UpstreamMS)
	rec.DownstreamOutputTPS = tokensPerSecond(rec.Usage.OutputTokens, rec.DownstreamMS)
	rec.DownstreamTotalTPS = tokensPerSecond(totalTokens(rec.Usage), rec.DownstreamMS)
}

func populateCosts(rec *logRecord) {
	if rec == nil {
		return
	}
	if rec.Usage.InputTokens > 0 && rec.InputPricePerMillionUSD == 0 {
		rec.Warnings = appendWarning(rec.Warnings, "missing-input-pricing-metadata")
	}
	if rec.Usage.OutputTokens > 0 && rec.OutputPricePerMillionUSD == 0 {
		rec.Warnings = appendWarning(rec.Warnings, "missing-output-pricing-metadata")
	}
	imageTokenCost := 0.0
	textInputTokens := rec.Usage.InputTokens
	if rec.Usage.InputImageTokens > 0 && rec.ImageInputPricePerMillionTokensUSD > 0 {
		textInputTokens -= rec.Usage.InputImageTokens
		if textInputTokens < 0 {
			textInputTokens = 0
		}
		imageTokenCost = float64(rec.Usage.InputImageTokens) * rec.ImageInputPricePerMillionTokensUSD / 1_000_000
	} else if rec.InputHasImage && rec.Usage.InputImageTokens > 0 && rec.ImageInputPricePerMillionTokensUSD == 0 {
		rec.Warnings = appendWarning(rec.Warnings, "missing-image-token-pricing-metadata")
	}
	imageUnitCost := float64(rec.InputImageCount) * rec.ImageInputPricePerImageUSD
	rec.InputCostUSD = roundUSD(float64(textInputTokens) * rec.InputPricePerMillionUSD / 1_000_000)
	rec.ImageCostUSD = roundUSD(imageTokenCost + imageUnitCost)
	rec.OutputCostUSD = roundUSD(float64(rec.Usage.OutputTokens) * rec.OutputPricePerMillionUSD / 1_000_000)
	rec.TotalCostUSD = roundUSD(rec.InputCostUSD + rec.ImageCostUSD + rec.OutputCostUSD)
	if rec.InputHasImage && rec.ImageInputPricePerImageUSD == 0 && rec.ImageInputPricePerMillionTokensUSD == 0 && rec.Usage.InputImageTokens == 0 && rec.UpstreamReportedTotalCostUSD == 0 {
		rec.Warnings = appendWarning(rec.Warnings, "missing-image-pricing-metadata")
	}
}

func appendWarning(warnings []string, warning string) []string {
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}

func roundUSD(v float64) float64 {
	return math.Round(v*1_000_000_000) / 1_000_000_000
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func totalTokens(usage Usage) int {
	if usage.TotalTokens != 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens
}

func tokensPerSecond(tokens int, durationMS *int64) *float64 {
	if tokens <= 0 || durationMS == nil || *durationMS <= 0 {
		return nil
	}
	v := float64(tokens) * 1000 / float64(*durationMS)
	return &v
}

func durationMillis(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms == 0 && d > 0 {
		return 1
	}
	return ms
}

func (s *Service) writeAdmissionError(w http.ResponseWriter, rc *requestContext, ad admission) {
	if ad.RetryAfter != "" {
		w.Header().Set("Retry-After", ad.RetryAfter)
	}
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	rc.trace("admission_rejected", ad.Reason, Target{}, 0, ad.Status, ad.Reason, true, 0)
	s.writeError(w, rc, ad.Status, ad.Reason)
}

func (s *Service) writeRoutingEligibilityError(w http.ResponseWriter, rc *requestContext, err routingEligibilityError) {
	code := "no-eligible-target"
	if rc != nil {
		rc.trace("routing_no_eligible_target", strings.Join(err.Requirements, ","), Target{}, 0, http.StatusBadGateway, code, false, 0)
		rc.rec.Status = http.StatusBadGateway
		rc.rec.Error = &code
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = fmt.Sprintf("no eligible upstream target for %s", err.Model)
		s.finish(rc, http.StatusBadGateway, &code)
	}
	message := fmt.Sprintf("no eligible upstream target is configured for model %q with %s requests requiring %s", err.Model, err.Dialect, strings.Join(err.Requirements, ", "))
	writeJSON(w, http.StatusBadGateway, map[string]any{
		"error": map[string]any{
			"type":    code,
			"message": message,
			"details": map[string]any{
				"model":        err.Model,
				"dialect":      err.Dialect,
				"requirements": err.Requirements,
				"hint":         "ask the router administrator to add or enable an upstream target for this model group that supports the requested API dialect, tools, and input modalities",
			},
		},
	})
}

func (s *Service) writeUpstreamFailureError(w http.ResponseWriter, rc *requestContext, req *IRRequest, dec decision, attempts int, err error) {
	classified := classifyError(err)
	code, status := upstreamFailureResponse(classified, rc)
	if rc != nil {
		rc.trace("upstream_exhausted", classified.Message, dec.Target, attempts, status, classified.Class, classified.Retryable, 0)
		rc.rec.Status = status
		rc.rec.Error = &code
		rc.rec.ErrorClass = classified.Class
		rc.rec.ErrorMessage = s.sanitizeDiagnosticError(classified.Message)
		s.finish(rc, status, &code)
	}
	targets := append([]Target{dec.Target}, dec.Fallbacks...)
	attempted := make([]map[string]string, 0, min(attempts, len(targets)))
	for i := 0; i < attempts && i < len(targets); i++ {
		attempted = append(attempted, map[string]string{
			"provider": targets[i].Provider,
			"model":    targets[i].Model,
		})
	}
	message := fmt.Sprintf("all eligible upstream targets failed for model %q after %d attempt(s)", req.Model, attempts)
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    code,
			"message": message,
			"details": map[string]any{
				"model":        req.Model,
				"dialect":      rc.dialect,
				"attempts":     attempts,
				"targets":      attempted,
				"last_error":   s.sanitizeDiagnosticError(classified.Message),
				"retryable":    classified.Retryable,
				"request_id":   rc.id,
				"fallbackUsed": attempts > 1,
			},
		},
	})
}

func upstreamFailureResponse(err upstreamError, rc *requestContext) (string, int) {
	switch {
	case err.Canceled:
		return "client-canceled", 499
	case err.TimedOut || err.Class == "upstream_timeout":
		return "upstream-timeout", http.StatusGatewayTimeout
	case err.Class == "upstream_rate_limited" || attemptsAllClass(rc, "upstream_rate_limited"):
		return "upstream-rate-limited", http.StatusServiceUnavailable
	default:
		return "upstream-failed", http.StatusBadGateway
	}
}

func attemptsAllClass(rc *requestContext, class string) bool {
	if rc == nil || len(rc.rec.AttemptsDetail) == 0 {
		return false
	}
	for _, attempt := range rc.rec.AttemptsDetail {
		if attempt.ErrorClass != class {
			return false
		}
	}
	return true
}

func sanitizeUpstreamError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 240 {
		text = text[:240]
	}
	return text
}

func (s *Service) writeError(w http.ResponseWriter, rc *requestContext, status int, code string) {
	if rc != nil {
		rc.trace("request_error", code, Target{}, 0, status, code, false, 0)
		rc.rec.Status = status
		c := code
		rc.rec.Error = &c
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = code
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

func responseID() string {
	return "resp_" + strings.TrimPrefix(requestID(), "req_")
}

func ensureResponseID(resp *IRResponse) {
	if resp != nil && resp.ID == "" {
		resp.ID = responseID()
	}
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

func callerIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		for _, value := range r.Header.Values(header) {
			for _, part := range strings.Split(value, ",") {
				host := strings.TrimSpace(part)
				if host == "" {
					continue
				}
				if ip := net.ParseIP(host); ip != nil {
					return ip.String()
				}
				if h, _, err := net.SplitHostPort(host); err == nil {
					if ip := net.ParseIP(h); ip != nil {
						return ip.String()
					}
				}
			}
		}
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
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
