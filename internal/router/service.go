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
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	cfg          *Config
	mux          *http.ServeMux
	httpClient   *http.Client
	callersBySum map[string]*callerRuntime
	adminBasic   map[string]adminBasicRuntime
	adminOIDC    *adminOIDCRuntime
	adminSession *adminSessionStore
	authorizer   *authorizer
	quota        *quotaStore
	cache        *responseCache
	logger       *requestLogger
	usage        *usageStore
	metrics      *metricsStore
	trafficShape *trafficShapeManager
	license      *licenseManager
	scripts      map[string]*scriptStrategy
	observations *dynamicObservationStore
	shaping      *upstreamShapeManager
	reportCursor [32]byte
}

type adminBasicRuntime struct {
	username    string
	hash        []byte
	subject     string
	domain      string
	permissions map[string]bool
}

type decision struct {
	Target            Target
	Fallbacks         []Target
	ClassLabel        *string
	Strategy          string
	GroupName         string
	TargetIndex       int
	DecisionTrace     string
	RoutingSignals    []routingSignalLogRecord
	DynamicScoreTerms []dynamicScoreTermLogRecord
	PolicyExecutions  []policyExecutionLogRecord
}

type routingEligibilityError struct {
	Model           string
	Dialect         string
	Requirements    []string
	ContractPresent bool
	ContractReason  string
}

func (e routingEligibilityError) Error() string {
	return fmt.Sprintf("no eligible targets for model group %q", e.Model)
}

type requestContext struct {
	id                   string
	start                time.Time
	caller               *callerRuntime
	dialect              string
	client               string
	clientIP             clientIPInfo
	method               string
	pathTemplate         string
	rec                  logRecord
	securityRecorded     bool
	traceSeq             int
	sanitizeTraceMessage func(string, string) string
}

type upstreamError struct {
	Class        string
	Message      string
	StatusCode   int
	Retryable    bool
	Fallbackable bool
	TimedOut     bool
	Canceled     bool
	ResponseLen  int64
	RetryAfter   time.Duration
	Details      []upstreamErrorDetailLogRecord
	Err          error
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
	quota, err := newQuotaStore(cfg.StatePath, cfg)
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
		httpClient:   newUpstreamHTTPClient(cfg.Server.Upstream),
		callersBySum: map[string]*callerRuntime{},
		adminBasic:   map[string]adminBasicRuntime{},
		adminSession: newAdminSessionStore(cfg.Server.AdminAuth.Sessions),
		quota:        quota,
		cache:        newCache(cfg.Server.Cache),
		logger:       logger,
		usage:        usage,
		metrics:      newMetricsStore(),
		trafficShape: newTrafficShapeManager(),
		scripts:      map[string]*scriptStrategy{},
		observations: newDynamicObservationStore(),
		shaping:      newUpstreamShapeManager(),
	}
	if _, err := rand.Read(s.reportCursor[:]); err != nil {
		_ = quota.Close()
		_ = logger.Close()
		_ = usage.Close()
		return nil, fmt.Errorf("generate admin report cursor key: %w", err)
	}
	s.license, err = newLicenseManager(cfg.Server.License, cfg, defaultLicensePublicKeys())
	if err != nil {
		_ = quota.Close()
		_ = logger.Close()
		_ = usage.Close()
		return nil, err
	}
	s.authorizer, err = newAuthorizer(cfg.Server.AdminAuth.Authorization, quota.callers, usage)
	if err != nil {
		_ = quota.Close()
		_ = logger.Close()
		_ = usage.Close()
		return nil, err
	}
	if cfg.Server.AdminAuth.OIDC.Enabled {
		s.adminOIDC, err = newAdminOIDCRuntime(context.Background(), cfg.Server.AdminAuth.OIDC, s.httpClient)
		if err != nil {
			_ = quota.Close()
			_ = logger.Close()
			_ = usage.Close()
			return nil, err
		}
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
	for _, user := range cfg.Server.AdminAuth.Basic.Users {
		subject := strings.TrimSpace(user.Subject)
		if subject == "" {
			subject = "basic:" + strings.TrimSpace(user.Username)
		}
		hash := strings.TrimSpace(os.Getenv(strings.TrimSpace(user.PasswordHashEnv)))
		perms := map[string]bool{}
		for _, permission := range user.Permissions {
			perms[strings.TrimSpace(permission)] = true
		}
		s.adminBasic[strings.TrimSpace(user.Username)] = adminBasicRuntime{
			username:    strings.TrimSpace(user.Username),
			hash:        []byte(hash),
			subject:     subject,
			domain:      strings.TrimSpace(user.Domain),
			permissions: perms,
		}
	}
	s.routes()
	s.license.start()
	return s, nil
}

func newUpstreamHTTPClient(cfg UpstreamConfig) *http.Client {
	return &http.Client{
		Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (s *Service) Handler() http.Handler {
	return s.mux
}

func (s *Service) Close() {
	_ = s.quota.Close()
	_ = s.logger.Close()
	_ = s.usage.Close()
	s.license.close()
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
		if lerr := s.license.enforce(LicenseFeatureRouting); lerr != nil {
			writeJSON(w, http.StatusServiceUnavailable, healthPayload(false, lerr.Code))
			return
		}
		writeJSON(w, http.StatusOK, healthPayload(true, ""))
	})
	s.mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		info := buildinfo.Current()
		raw, _ := json.Marshal(info)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		out["license_compile_mode"] = licenseCompileMode
		writeJSON(w, http.StatusOK, out)
	})
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("GET /v1/usage", s.handleUsage)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /admin/auth/login", s.handleAdminOIDCLogin)
	s.mux.HandleFunc("GET /admin/auth/callback", s.handleAdminOIDCCallback)
	s.mux.HandleFunc("POST /admin/auth/logout", s.handleAdminOIDCLogout)
	s.mux.HandleFunc("GET /admin/auth/me", s.handleAdminAuthMe)
	s.mux.HandleFunc("GET /admin/auth/check", s.handleAdminAuthCheck)
	s.mux.HandleFunc("GET /admin/license/status", s.handleAdminLicenseStatus)
	s.mux.HandleFunc("GET /admin/reports", s.handleAdminReports)
	s.mux.HandleFunc("GET /admin/reports/", s.handleAdminReports)
	s.mux.HandleFunc("DELETE /v1/content-captures/{request_id}", s.handleContentCaptureDelete)
	s.mux.HandleFunc("POST /v1/content-captures/purge-expired", s.handleContentCapturePurgeExpired)
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
		reasoningLevels, reasoningSummaries, defaultReasoningLevel := s.reasoningMetadataForGroup(name)
		model := map[string]any{
			"id":                               name,
			"slug":                             name,
			"name":                             name,
			"display_name":                     name,
			"description":                      "Smart LLM Router model group " + name,
			"mode":                             "default",
			"base_instructions":                "Use GenAI Smart Router as the model gateway.",
			"context_window":                   131072,
			"max_context_window":               131072,
			"effective_context_window_percent": 95,
			"default_verbosity":                "low",
			"supports_parallel_tool_calls":     len(s.supportedToolsForGroup(name)) > 0,
			"supports_search_tool":             false,
			"supports_image_detail_original":   hasImage,
			"support_verbosity":                true,
			"apply_patch_tool_type":            "freeform",
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
		}
		if len(reasoningLevels) > 0 {
			model["default_reasoning_summary"] = "none"
			model["supported_reasoning_levels"] = reasoningLevelPresets(reasoningLevels)
			model["supports_reasoning_summaries"] = reasoningSummaries
		}
		if defaultReasoningLevel != "" && defaultReasoningLevel != "none" {
			model["default_reasoning_level"] = defaultReasoningLevel
		}
		data = append(data, model)
	}
	sort.Slice(data, func(i, j int) bool { return data[i]["id"].(string) < data[j]["id"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "mode": "default", "data": data, "models": data})
}

func reasoningLevelPresets(levels []string) []map[string]string {
	presets := make([]map[string]string, 0, len(levels))
	for _, level := range levels {
		switch level {
		case "low":
			presets = append(presets, map[string]string{"effort": "low", "description": "Fast responses with lighter reasoning"})
		case "medium":
			presets = append(presets, map[string]string{"effort": "medium", "description": "Balances speed and reasoning depth for everyday tasks"})
		case "high":
			presets = append(presets, map[string]string{"effort": "high", "description": "Greater reasoning depth for complex problems"})
		case "xhigh":
			presets = append(presets, map[string]string{"effort": "xhigh", "description": "Extra high reasoning depth for complex problems"})
		default:
			if strings.TrimSpace(level) != "" {
				presets = append(presets, map[string]string{"effort": level, "description": "Deployment-defined reasoning depth"})
			}
		}
	}
	return presets
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
	if !s.authorizeCaller(rc.caller, authzObjectMetrics, authzActionRead) {
		code := "metrics-forbidden"
		s.writeError(w, rc, http.StatusForbidden, code)
		return
	}
	if lerr := s.license.enforce(LicenseFeatureUsageReporting); lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, s.metrics.Prometheus(s.license, s.trafficShape))
}

func (s *Service) handleAdminLicenseStatus(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.authenticateAdminSubject(w, r)
	if !ok {
		s.recordAdminSecurityAccess(r, adminAuthSubject{}, http.StatusUnauthorized, "admin-auth-failed", authzObjectAdminReports, authzActionRead)
		return
	}
	if !s.authorizeAdmin(subject, authzObjectAdminReports, authzActionRead) {
		s.recordAdminSecurityAccess(r, subject, http.StatusForbidden, "reports-forbidden", authzObjectAdminReports, authzActionRead)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "reports-forbidden", "message": "reports-forbidden"}})
		return
	}
	s.recordAdminSecurityAccess(r, subject, http.StatusOK, "", authzObjectAdminReports, authzActionRead)
	writeJSON(w, http.StatusOK, safeLicenseStatusResponseWithUsage(s.license))
}

func (s *Service) handleAdminAuthCheck(w http.ResponseWriter, r *http.Request) {
	basic, ok := s.authenticateAdminBasic(w, r)
	if !ok {
		s.recordAdminSecurityAccess(r, adminAuthSubject{}, http.StatusUnauthorized, "admin-auth-failed", "admin:auth", authzActionRead)
		return
	}
	subject := adminAuthSubjectForBasic(basic)
	if !subject.permissions["admin:auth:read"] {
		s.recordAdminSecurityAccess(r, subject, http.StatusForbidden, "admin-forbidden", "admin:auth", authzActionRead)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"type": "admin-forbidden", "message": "admin-forbidden"}})
		return
	}
	s.recordAdminSecurityAccess(r, subject, http.StatusOK, "", "admin:auth", authzActionRead)
	writeJSON(w, http.StatusOK, safeAdminSubjectResponse(subject))
}

func (s *Service) handleContentCaptureDelete(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "content-admin")
	if !ok {
		return
	}
	requestID := strings.TrimSpace(r.PathValue("request_id"))
	if requestID == "" {
		s.writeError(w, rc, http.StatusBadRequest, "missing-request-id")
		return
	}
	if s.usage == nil {
		s.writeError(w, rc, http.StatusServiceUnavailable, "content-store-disabled")
		return
	}
	targetDomains, err := s.contentCaptureTargetDomains(requestID)
	if err != nil {
		s.writeError(w, rc, http.StatusInternalServerError, "content-delete-failed")
		return
	}
	if len(targetDomains) == 0 {
		if !s.authorizeCaller(rc.caller, authzObjectContentCapture, authzActionDelete) {
			s.writeError(w, rc, http.StatusForbidden, "content-forbidden")
			return
		}
	} else {
		for _, domain := range targetDomains {
			if domain == contentCaptureUnknownDomain {
				s.writeError(w, rc, http.StatusForbidden, "content-forbidden")
				return
			}
			if !s.authorizeCallerInDomain(rc.caller, domain, authzObjectContentCapture, authzActionDelete) {
				s.writeError(w, rc, http.StatusForbidden, "content-forbidden")
				return
			}
		}
	}
	rows, err := s.usage.DeleteContentCapturesByRequestID(requestID, rc.rec.CallerID, rc.rec.TokenID, "admin-delete")
	if err != nil {
		s.writeError(w, rc, http.StatusInternalServerError, "content-delete-failed")
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, map[string]any{"request_id": requestID, "deleted": rows})
}

func (s *Service) contentCaptureTargetDomains(requestID string) ([]string, error) {
	if s == nil || s.usage == nil {
		return nil, nil
	}
	return s.usage.ContentCaptureTargetDomainsByRequestID(requestID)
}

func (s *Service) handleContentCapturePurgeExpired(w http.ResponseWriter, r *http.Request) {
	rc, ok := s.begin(w, r, "content-admin")
	if !ok {
		return
	}
	if !s.authorizeCaller(rc.caller, authzObjectContentCapture, authzActionPurge) {
		code := "content-forbidden"
		s.writeError(w, rc, http.StatusForbidden, code)
		return
	}
	if s.usage == nil {
		s.writeError(w, rc, http.StatusServiceUnavailable, "content-store-disabled")
		return
	}
	rows, err := s.usage.PurgeExpiredContentCaptures(time.Now().UTC(), rc.rec.CallerID, rc.rec.TokenID, "admin-retention-purge")
	if err != nil {
		s.writeError(w, rc, http.StatusInternalServerError, "content-purge-failed")
		return
	}
	defer s.finish(rc, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": rows})
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
	if req.Model == "" {
		req.Model = s.cfg.Server.DefaultModelGroup
	}
	if req.Model == "" {
		s.writeError(w, rc, http.StatusBadRequest, "missing-model")
		return
	}
	if !rc.caller.allow[req.Model] {
		s.writeError(w, rc, http.StatusForbidden, "model-not-allowed")
		return
	}
	if _, ok := s.cfg.Models[req.Model]; !ok {
		s.writeError(w, rc, http.StatusForbidden, "model-not-found")
		return
	}
	ad := s.quota.Admit(rc.caller, estimateTokens(req))
	if !ad.OK {
		s.writeAdmissionError(w, rc, ad)
		return
	}
	defer s.quota.ReleaseReservation(rc.caller, ad.Reservation)
	defer s.quota.Release(rc.caller)
	if lerr := s.license.AdmitRequest("anthropic"); lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return
	}
	defer s.license.ReleaseRequest()
	licRes, lerr := s.license.ReserveTokens(estimateTokens(req))
	if lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return
	}
	defer s.license.ReleaseReservation(licRes)
	rc.rec.RequestedModel = req.Model
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	tokens := estimateTokens(req)
	rc.rec.Usage = Usage{InputTokens: tokens, TotalTokens: tokens}
	s.quota.RecordTokens(rc.caller, ad.Reservation, rc.rec.Usage)
	s.license.RecordTokens(licRes, rc.rec.Usage)
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
	if req.Model == "" {
		req.Model = s.cfg.Server.DefaultModelGroup
	}
	if req.Model == "" {
		s.writeError(w, rc, http.StatusBadRequest, "missing-model")
		return
	}
	if dialect == "openai-responses" && requestHasForbiddenProviderHostedTools(req) {
		s.writeError(w, rc, http.StatusBadRequest, "provider-hosted-tools-forbidden")
		return
	}
	rc.rec.Stream = req.Stream

	if !rc.caller.allow[req.Model] {
		s.writeError(w, rc, http.StatusForbidden, "model-not-allowed")
		return
	}
	group, ok := s.cfg.Models[req.Model]
	if !ok {
		s.writeError(w, rc, http.StatusForbidden, "model-not-found")
		return
	}
	rc.rec.RequestedModel = req.Model
	s.recordRoutingReproducibility(rc, req.Model, group)
	if group.Contract != nil {
		rc.rec.ContractPresent = true
		rc.rec.ContractBucket = "pending"
		rc.rec.ContractWorkload = contractWorkloadLabel(group.Contract)
	}
	for _, feature := range licenseFeaturesForGroup(group) {
		if lerr := s.license.enforce(feature); lerr != nil {
			s.writeError(w, rc, lerr.StatusCode, lerr.Code)
			return
		}
	}
	shapeCfg, shapeScope, shapeEnabled := resolveTrafficShapeConfig(s.cfg.Server.TrafficShape, rc.caller.cfg)
	inputTokens := estimateTokens(req)
	outputReservationTokens := trafficShapeOutputReservation(req, dialect)
	totalReservationTokens := inputTokens + outputReservationTokens
	if shapeEnabled {
		shapeRes := s.trafficShape.Admit(r.Context(), trafficShapeRequest{
			CallerID:                 rc.caller.cfg.ID,
			ModelGroup:               req.Model,
			Config:                   shapeCfg,
			Scope:                    shapeScope,
			Phase:                    trafficShapeBucketRequestStart,
			EstimatedInputTokens:     inputTokens,
			ReservedOutputTokens:     outputReservationTokens,
			TotalReservedTokens:      totalReservationTokens,
			IncludeRequestStart:      true,
			IncludeTokenReservations: false,
		})
		recordTrafficShapeResult(rc, shapeRes)
		if shapeRes.Decision == trafficShapeDecisionRejected {
			s.writeTrafficShapeError(w, rc, req.Model, shapeRes)
			return
		}
	}
	ad := s.quota.Admit(rc.caller, 0)
	if !ad.OK {
		s.writeAdmissionError(w, rc, ad)
		return
	}
	defer s.quota.Release(rc.caller)
	if lerr := s.license.AdmitRequest(dialect); lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return
	}
	defer s.license.ReleaseRequest()
	rc.rec.QuotaState = ad.QuotaState
	rc.rec.KeyState = ad.KeyState
	if ad.WarningText != "" {
		w.Header().Add("X-Router-Warning", ad.WarningText)
		rc.rec.Warnings = append(rc.rec.Warnings, ad.WarningText)
	}
	piiResult, err := applyPIIFilter(req, group.PIIFilter)
	if err != nil {
		var blocked piiFilterBlockedError
		if errors.As(err, &blocked) {
			rc.rec.PIIFilterApplied = piiResult.Applied
			rc.rec.PIIFilterMode = piiResult.Mode
			rc.rec.PIIFilterReplacements = piiResult.Replacements
			rc.rec.PIIFilterRuleCount = piiRuleCount(piiResult)
			rc.rec.Warnings = append(rc.rec.Warnings, piiResult.Warnings...)
			rc.trace("pii_filter_blocked", "pii_filter matched request content", Target{}, 0, http.StatusBadRequest, "pii-filter-blocked", false, 0)
			s.writeError(w, rc, http.StatusBadRequest, "pii-filter-blocked")
			return
		}
		s.writeError(w, rc, http.StatusBadGateway, "pii-filter-failed")
		return
	}
	if piiResult.Applied {
		rc.rec.PIIFilterApplied = true
		rc.rec.PIIFilterMode = piiResult.Mode
		rc.rec.PIIFilterReplacements = piiResult.Replacements
		rc.rec.PIIFilterRuleCount = piiRuleCount(piiResult)
		rc.rec.Warnings = appendWarning(rc.rec.Warnings, "pii-filter-applied")
		rc.rec.Warnings = append(rc.rec.Warnings, piiResult.Warnings...)
		rc.trace("pii_filter_applied", fmt.Sprintf("replacements=%d rules=%d", piiResult.Replacements, piiRuleCount(piiResult)), Target{}, 0, 0, "", false, 0)
	}
	captureDecision := s.contentCaptureFor(rc.caller, req.Model, group)
	s.captureRequestContent(rc, req, r.Header, captureDecision)
	s.recordRequestShapeTelemetry(rc, req, dialect, len(body))
	s.recordRequestTokenEstimateTelemetry(rc, req, dialect, len(body))
	s.recordDecisionShape(rc, req, dialect)
	s.recordEligibilityTelemetry(rc, req.Model, group, req, dialect)
	dec, err := s.pick(rc, req.Model, group, req, dialect, rc.caller, rc.rec.TokenID)
	if err != nil {
		var shapeErr upstreamCapacityThrottledError
		if errors.As(err, &shapeErr) {
			s.writeUpstreamCapacityThrottledError(w, rc, req.Model, dialect, shapeErr)
			return
		}
		var eligibilityErr routingEligibilityError
		if errors.As(err, &eligibilityErr) {
			s.writeRoutingEligibilityError(w, rc, eligibilityErr)
			return
		}
		var policyErr routingPolicyError
		if errors.As(err, &policyErr) {
			s.recordPolicyFailureTelemetry(rc, policyErr)
			s.writeRoutingPolicyError(w, rc, policyErr)
			return
		}
		s.writeError(w, rc, http.StatusBadGateway, "routing-failed")
		return
	}
	s.recordRoutingDecisionTelemetry(rc, dec)
	rc.rec.ResolvedGroup = req.Model
	rc.rec.Strategy = dec.Strategy
	rc.rec.ClassLabel = dec.ClassLabel
	rc.rec.InputHasImage = requestHasImages(req)
	rc.rec.InputImageCount = requestImageCount(req)
	rc.rec.TargetProvider = dec.Target.Provider
	rc.rec.TargetModel = dec.Target.Model
	rc.rec.TargetDialect = targetDialect(s.cfg.Provider[dec.Target.Provider], dec.Target)
	if group.Contract != nil {
		rc.rec.ContractBucket = "passed"
	}
	if dec.Target.Validation != nil {
		rc.rec.TargetValidationStatus = strings.ToLower(strings.TrimSpace(dec.Target.Validation.Status))
		rc.rec.TargetValidationWorkload = strings.TrimSpace(dec.Target.Validation.Workload)
		rc.rec.TargetValidationAgeBucket = validationAgeBucket(dec.Target.Validation, time.Now().UTC())
	}
	rc.rec.InputPricePerMillionUSD = dec.Target.InputPricePerMillionUSD
	rc.rec.OutputPricePerMillionUSD = dec.Target.OutputPricePerMillionUSD
	rc.rec.ImageInputPricePerMillionTokensUSD = dec.Target.ImageInputPricePerMillionTokensUSD
	rc.rec.ImageInputPricePerImageUSD = dec.Target.ImageInputPricePerImageUSD
	rc.rec.PricingSource = dec.Target.PricingSource
	rc.rec.PricingUpdatedAt = dec.Target.PricingUpdatedAt
	if dec.DecisionTrace != "" {
		rc.trace("routing_decision", dec.DecisionTrace, dec.Target, 0, 0, "", false, 0)
	}

	key := cacheKey(req, dec.Target)
	if cacheable(req) {
		if cached, ok := s.cache.Get(key); ok {
			ensureResponseID(cached)
			captureCached := *cached
			if len(cached.Warnings) > 0 {
				captureCached.Warnings = append([]string(nil), cached.Warnings...)
			}
			s.captureResponseContent(rc, &captureCached, captureDecision)
			if piiRestoreEnabled(group.PIIFilter) {
				restorePIIPlaceholders(cached, piiResult)
			}
			rc.rec.Cache = "hit"
			s.recordCacheReasonTelemetry(rc, "hit", "cache-hit", dec.Target)
			rc.rec.Status = http.StatusOK
			rc.rec.Usage = cached.Usage
			rc.trace("cache_hit", "", dec.Target, 0, http.StatusOK, "", false, 0)
			s.writeIR(w, dialect, cached, req.Stream, rc)
			s.finish(rc, http.StatusOK, nil)
			return
		}
		rc.rec.Cache = "miss"
		s.recordCacheReasonTelemetry(rc, "miss", "cache-miss", dec.Target)
		rc.trace("cache_miss", "", dec.Target, 0, 0, "", false, 0)
	} else {
		rc.rec.Cache = "bypass"
		s.recordCacheReasonTelemetry(rc, "bypass", cacheBypassReason(req), dec.Target)
		rc.trace("cache_bypass", "", dec.Target, 0, 0, "", false, 0)
	}

	reservationTokens := reservationEstimate(req, dialect)
	resAd := s.quota.ReserveTokens(rc.caller, reservationTokens)
	if !resAd.OK {
		s.writeAdmissionError(w, rc, resAd)
		return
	}
	defer s.quota.ReleaseReservation(rc.caller, resAd.Reservation)
	licRes, lerr := s.license.ReserveTokens(reservationTokens)
	if lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return
	}
	defer s.license.ReleaseReservation(licRes)
	rc.rec.QuotaState = resAd.QuotaState
	rc.rec.KeyState = resAd.KeyState
	if resAd.WarningText != "" {
		w.Header().Add("X-Router-Warning", resAd.WarningText)
		rc.rec.Warnings = append(rc.rec.Warnings, resAd.WarningText)
	}
	if shapeEnabled {
		shapeRes := s.trafficShape.Admit(r.Context(), trafficShapeRequest{
			CallerID:                 rc.caller.cfg.ID,
			ModelGroup:               req.Model,
			Config:                   shapeCfg,
			Scope:                    shapeScope,
			Phase:                    "token_reservation",
			EstimatedInputTokens:     inputTokens,
			ReservedOutputTokens:     outputReservationTokens,
			TotalReservedTokens:      totalReservationTokens,
			IncludeRequestStart:      false,
			IncludeTokenReservations: true,
		})
		recordTrafficShapeResult(rc, shapeRes)
		if shapeRes.Decision == trafficShapeDecisionRejected {
			s.writeTrafficShapeError(w, rc, req.Model, shapeRes)
			return
		}
	}

	upstreamStart := time.Now()
	rc.trace("upstream_start", "", dec.Target, 0, 0, "", false, 0)
	resp, attempts, fallbackUsed, err := s.callUpstreams(r.Context(), rc, dialect, req, dec)
	upstreamMS := durationMillis(time.Since(upstreamStart))
	rc.rec.UpstreamMS = &upstreamMS
	rc.rec.Attempts = attempts
	rc.rec.FallbackUsed = fallbackUsed
	if err != nil {
		s.writeUpstreamFailureError(w, rc, req, dec, attempts, err, captureDecision)
		return
	}
	if resp != nil {
		ensureResponseID(resp)
		if totalTokens(resp.Usage) == 0 && reservationTokens > 0 {
			resp.Usage = estimatedUsageForReservation(req, dialect, reservationTokens)
			resp.Warnings = appendWarning(resp.Warnings, "usage-estimated")
		}
		if cacheable(req) {
			s.cache.Put(key, resp)
		}
		s.captureResponseContent(rc, resp, captureDecision)
		if piiRestoreEnabled(group.PIIFilter) {
			restorePIIPlaceholders(resp, piiResult)
		}
		quotaState, keyState := s.quota.RecordTokens(rc.caller, resAd.Reservation, resp.Usage)
		s.license.RecordTokens(licRes, resp.Usage)
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

func estimatedUsageForReservation(req *IRRequest, dialect string, reservationTokens int) Usage {
	input := estimateTokens(req)
	if input <= 0 {
		input = 1
	}
	output := reservationTokens - input
	if output < 0 {
		output = 0
	}
	if output == 0 && dialect != "anthropic" && req != nil && req.MaxTokens > 0 {
		output = req.MaxTokens
	}
	total := input + output
	if total <= 0 {
		total = reservationTokens
	}
	return Usage{InputTokens: input, OutputTokens: output, TotalTokens: total}
}

func (s *Service) begin(w http.ResponseWriter, r *http.Request, dialect string) (*requestContext, bool) {
	id := requestID()
	w.Header().Set("X-Request-Id", id)
	caller, tokenID, err := s.authenticate(r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))
	ipInfo := resolveClientIP(r, s.cfg.Server.ClientIP)
	rc := &requestContext{
		id:                   id,
		start:                time.Now(),
		caller:               caller,
		dialect:              dialect,
		client:               inferClient(r),
		clientIP:             ipInfo,
		method:               r.Method,
		pathTemplate:         requestPathTemplate(r),
		sanitizeTraceMessage: s.sanitizeDiagnosticTraceMessage,
		rec: logRecord{
			RequestID:            id,
			Client:               inferClient(r),
			CallerIP:             ipInfo.Address,
			InboundDialect:       dialect,
			Cache:                "bypass",
			QuotaState:           "ok",
			KeyState:             "active",
			TrafficShapeDecision: trafficShapeDecisionDisabled,
			Attempts:             0,
			Warnings:             []string{},
		},
	}
	if err != nil {
		rc.rec.TokenID = tokenID
		status := http.StatusUnauthorized
		code := "unauthorized"
		var policyErr authPolicyError
		if errors.As(err, &policyErr) {
			status = policyErr.status
			code = policyErr.code
		}
		rc.trace("auth_rejected", err.Error(), Target{}, 0, status, code, false, 0)
		s.writeError(w, rc, status, code)
		return nil, false
	}
	rc.rec.CallerID = caller.cfg.ID
	rc.rec.CallerUser = callerUser(caller.cfg)
	rc.rec.CallerProject = callerProject(caller.cfg)
	rc.rec.CallerEnvironment = callerEnvironment(caller.cfg)
	rc.rec.TokenID = tokenID
	rc.trace("request_accepted", "", Target{}, 0, 0, "", false, 0)
	if lerr := s.license.enforce(licenseFeatureForRoute(dialect)); lerr != nil {
		s.writeError(w, rc, lerr.StatusCode, lerr.Code)
		return nil, false
	}
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
	s.sanitizeDiagnosticRecord(&rc.rec)
	s.recordCacheStats(rc)
	populateThroughput(&rc.rec)
	populateCosts(&rc.rec)
	s.populateLicenseMetadata(&rc.rec)
	if rc.rec.Status == 0 {
		rc.rec.Status = status
	}
	if code != nil {
		rc.rec.Error = code
	}
	rc.rec.LatencyMS = time.Since(rc.start).Milliseconds()
	s.recordDynamicObservation(rc.rec)
	s.metrics.Observe(rc.rec)
	s.logger.Emit(rc.rec)
	s.usage.Emit(rc.rec)
	if !rc.securityRecorded {
		codeText := ""
		if rc.rec.Error != nil {
			codeText = *rc.rec.Error
		}
		s.recordRequestSecurityAccess(rc, rc.rec.Status, codeText)
		rc.securityRecorded = true
	}
}

func (s *Service) populateLicenseMetadata(rec *logRecord) {
	if s == nil || rec == nil || s.license == nil {
		return
	}
	st := s.license.statusSnapshot()
	rec.LicenseStatus = st.Code
	rec.LicenseReason = st.ValidationReason
	rec.LicenseID = st.LicenseID
	rec.LicenseCustomerID = st.CustomerID
	rec.LicenseSKU = st.SKU
	rec.LicenseKeyID = st.KeyID
	rec.LicenseGraceActive = st.GraceActive
	if !st.ExpiresAt.IsZero() {
		rec.LicenseExpiry = st.ExpiresAt.UTC().Format(time.RFC3339)
	}
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
	if rc.sanitizeTraceMessage != nil {
		message = rc.sanitizeTraceMessage(event, message)
	} else {
		message = sanitizePersistedTraceMessage(event, message)
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
			if err := caller.authPolicyError(); err != nil {
				return nil, tokenID, err
			}
			return caller, tokenID, nil
		}
	}
	return nil, "invalid-token", errors.New("unknown token")
}

type authPolicyError struct {
	status int
	code   string
}

func (e authPolicyError) Error() string {
	return e.code
}

func (c *callerRuntime) authPolicyError() error {
	if c == nil {
		return authPolicyError{status: http.StatusUnauthorized, code: "unauthorized"}
	}
	switch {
	case normalizeStatusDefault(c.keyStatus) != accountStatusActive:
		return authPolicyError{status: http.StatusForbidden, code: statusAuthCode("key", normalizeStatusDefault(c.keyStatus))}
	case normalizeStatusDefault(c.ownerUserStatus) != accountStatusActive:
		return authPolicyError{status: http.StatusForbidden, code: statusAuthCode("user", normalizeStatusDefault(c.ownerUserStatus))}
	case normalizeStatusDefault(c.projectStatus) != accountStatusActive:
		return authPolicyError{status: http.StatusForbidden, code: statusAuthCode("project", normalizeStatusDefault(c.projectStatus))}
	case normalizeStatusDefault(c.membershipStatus) != accountStatusActive:
		return authPolicyError{status: http.StatusForbidden, code: statusAuthCode("membership", normalizeStatusDefault(c.membershipStatus))}
	default:
		return nil
	}
}

func statusAuthCode(entity, status string) string {
	switch normalizeStatusDefault(status) {
	case accountStatusSuspended:
		return entity + "-suspended"
	case accountStatusRemoved:
		return entity + "-removed"
	case accountStatusArchived:
		return entity + "-archived"
	case keyStatusExpired:
		return entity + "-expired"
	case keyStatusRotated:
		return entity + "-rotated"
	default:
		return entity + "-disabled"
	}
}

func (s *Service) authenticateAdminBasic(w http.ResponseWriter, r *http.Request) (adminBasicRuntime, bool) {
	s.setAdminAuthHeaders(w)
	if s == nil || !s.cfg.Server.AdminAuth.Basic.Enabled {
		http.NotFound(w, r)
		return adminBasicRuntime{}, false
	}
	if !s.cfg.Server.AdminAuth.Basic.AllowInsecureHTTP && !requestIsHTTPS(r, s.cfg.Server.AdminAuth.Basic.TrustedProxyCIDRs) {
		s.adminBasicChallenge(w)
		return adminBasicRuntime{}, false
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		s.adminBasicChallenge(w)
		return adminBasicRuntime{}, false
	}
	subject, found := s.findAdminBasicUser(username)
	hash := subject.hash
	if !found {
		hash = adminBasicDummyHash()
	}
	passwordOK := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if !found || !passwordOK {
		s.adminBasicChallenge(w)
		return adminBasicRuntime{}, false
	}
	return subject, true
}

func (s *Service) findAdminBasicUser(username string) (adminBasicRuntime, bool) {
	var matched adminBasicRuntime
	found := 0
	for configured, subject := range s.adminBasic {
		if subtle.ConstantTimeCompare([]byte(configured), []byte(username)) == 1 {
			matched = subject
			found = 1
		}
	}
	return matched, found == 1
}

func (s *Service) setAdminAuthHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Service) adminBasicChallenge(w http.ResponseWriter) {
	realm := "GenAI Smart Router Admin"
	if s != nil && strings.TrimSpace(s.cfg.Server.AdminAuth.Basic.Realm) != "" {
		realm = strings.TrimSpace(s.cfg.Server.AdminAuth.Basic.Realm)
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s", charset="UTF-8"`, realm))
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"type": "unauthorized", "message": "unauthorized"}})
}

func requestIsHTTPS(r *http.Request, trustedProxyCIDRs []string) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return false
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range trustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func adminBasicDummyHash() []byte {
	return []byte("$2a$10$PEWHfUFcvCRVUkh/hggt6eUYdDlrFr0cfzXvIXtLDKpS5uTtYq65i")
}

func (s *Service) pick(rc *requestContext, groupName string, group ModelGroup, req *IRRequest, callerDialect string, caller *callerRuntime, tokenID string) (decision, error) {
	targets := append([]Target(nil), group.Targets...)
	targets = s.targetsForRequest(rc, targets, req, callerDialect)
	if len(targets) == 0 {
		return decision{}, routingEligibilityError{
			Model:        groupName,
			Dialect:      callerDialect,
			Requirements: routingRequirements(req, callerDialect),
		}
	}
	contractResult := s.targetsForContract(groupName, group, targets, req, callerDialect)
	targets = contractResult.targets
	if len(targets) == 0 {
		requirements := append(routingRequirements(req, callerDialect), defaultString(contractResult.reason, "contract-quality-floor"))
		return decision{}, routingEligibilityError{
			Model:           groupName,
			Dialect:         callerDialect,
			Requirements:    requirements,
			ContractPresent: true,
			ContractReason:  defaultString(contractResult.reason, "contract-quality-floor"),
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
	case "dynamic_score":
		return s.pickDynamicScore(groupName, group, req, callerDialect, targets)
	case "script":
		strat := s.scripts[groupName]
		if strat == nil {
			err := fmt.Errorf("script strategy %s not loaded", groupName)
			return decision{}, policyFailure(groupName, "script", "typescript", "load_error", err, 0, len(targets), len(group.Targets))
		}
		start := time.Now()
		dec, err := strat.Pick(groupName, req, group.Contract, targets, s.cfg.Provider, caller, tokenID)
		if err != nil {
			return decision{}, policyFailure(groupName, "script", "typescript", "", err, time.Since(start).Milliseconds(), len(targets), len(group.Targets))
		}
		dec.DynamicScoreTerms = append(dec.DynamicScoreTerms, policyOutputRankingTelemetry(dec.Strategy, dec.Target, dec.Fallbacks, s.cfg.Provider)...)
		return dec, nil
	case "external":
		strat := externalPolicyStrategy{cfg: group.ExternalPolicy}
		dec, err := strat.Pick(groupName, req, group.Contract, targets, group.Targets, s.cfg.Provider, caller, tokenID, callerDialect)
		if err != nil {
			return decision{}, err
		}
		dec.DynamicScoreTerms = append(dec.DynamicScoreTerms, policyOutputRankingTelemetry(dec.Strategy, dec.Target, dec.Fallbacks, s.cfg.Provider)...)
		return dec, nil
	default:
		return decision{}, fmt.Errorf("unknown strategy %s", strategy)
	}
	if label != nil {
		label = safePolicyClassLabel(*label)
	}
	return decision{Target: targets[0], Fallbacks: targets[1:], ClassLabel: label, Strategy: strategy, GroupName: groupName, DynamicScoreTerms: simpleStrategyRankingTelemetry(strategy, targets, s.cfg.Provider)}, nil
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

func shapeInputForRequest(req *IRRequest, callerDialect string) shapeReservationInput {
	input := estimateTokens(req)
	total := reservationEstimate(req, callerDialect)
	if total < input {
		total = input
	}
	return shapeReservationInput{
		EstimatedInputTokens: input,
		ReservedOutputTokens: total - input,
		TotalReservedTokens:  total,
	}
}

func (s *Service) targetsForTrafficShape(rc *requestContext, groupName string, targets []Target, req *IRRequest, callerDialect string) ([]Target, error) {
	if s == nil || s.shaping == nil || len(targets) == 0 {
		return targets, nil
	}
	in := shapeInputForRequest(req, callerDialect)
	out := make([]Target, 0, len(targets))
	var maxRetryAfter time.Duration
	for _, target := range targets {
		result := s.shaping.Check(s.shapeScopes(groupName, target), in)
		if result.OK {
			out = append(out, target)
			continue
		}
		if result.RetryAfter > maxRetryAfter {
			maxRetryAfter = result.RetryAfter
		}
		s.recordShapeEvents(rc, result.Events)
		s.recordShapeFilterReason(rc, target, result.Reason)
		if rc != nil {
			rc.trace("upstream_shape_filtered", result.Reason, target, 0, http.StatusServiceUnavailable, result.Reason, true, durationMillis(result.RetryAfter))
		}
	}
	if len(out) == 0 {
		return nil, upstreamCapacityThrottledError{RetryAfter: maxRetryAfter, TargetCount: len(targets)}
	}
	return out, nil
}

func (s *Service) admitTrafficShape(rc *requestContext, groupName string, target Target, in shapeReservationInput) shapeAdmissionResult {
	if s == nil || s.shaping == nil {
		return shapeAdmissionResult{OK: true}
	}
	result := s.shaping.Admit(s.shapeScopes(groupName, target), in)
	s.recordShapeEvents(rc, result.Events)
	return result
}

func (s *Service) recordAdaptiveTrafficBackoff(rc *requestContext, groupName string, target Target, err upstreamError, in shapeReservationInput) {
	if s == nil || s.shaping == nil {
		return
	}
	events := s.shaping.StartBackoff(s.shapeScopes(groupName, target), err.Class, err.RetryAfter, in)
	s.recordShapeEvents(rc, events)
	if rc != nil {
		for _, event := range events {
			rc.trace("upstream_shape_cooldown_started", event.BackoffReason, target, 0, http.StatusServiceUnavailable, event.BackoffReason, true, event.RetryAfterMS)
		}
	}
}

func (s *Service) shapeScopes(groupName string, target Target) []shapeScope {
	if s == nil || s.cfg == nil {
		return nil
	}
	provider := s.cfg.Provider[target.Provider]
	dialect := targetDialect(provider, target)
	modelRef := target.ModelRef
	providerModel := ProviderModel{}
	if modelRef != "" {
		providerModel = provider.Models[modelRef]
	} else {
		for ref, model := range provider.Models {
			if model.Model == target.Model {
				modelRef = ref
				providerModel = model
				break
			}
		}
	}
	var scopes []shapeScope
	if trafficShapeEnabled(provider.TrafficShape) {
		scopes = append(scopes, shapeScope{
			Scope:    shapeScopeProvider,
			Key:      strings.Join([]string{shapeScopeProvider, target.Provider, dialect}, "|"),
			Config:   provider.TrafficShape,
			Provider: target.Provider,
			ModelRef: modelRef,
			Model:    target.Model,
			Dialect:  dialect,
		})
	}
	if trafficShapeEnabled(providerModel.TrafficShape) {
		scopes = append(scopes, shapeScope{
			Scope:    shapeScopeProviderModel,
			Key:      strings.Join([]string{shapeScopeProviderModel, target.Provider, defaultString(modelRef, target.Model), dialect}, "|"),
			Config:   providerModel.TrafficShape,
			Provider: target.Provider,
			ModelRef: modelRef,
			Model:    target.Model,
			Dialect:  dialect,
		})
	}
	if trafficShapeEnabled(target.TrafficShape) {
		targetKey := strings.Join([]string{shapeScopeTarget, groupName, target.Provider, defaultString(modelRef, target.Model), target.Model, dialect}, "|")
		scopes = append(scopes, shapeScope{
			Scope:    shapeScopeTarget,
			Key:      targetKey,
			Config:   target.TrafficShape,
			Provider: target.Provider,
			ModelRef: modelRef,
			Model:    target.Model,
			Dialect:  dialect,
		})
	}
	return scopes
}

func (s *Service) recordShapeEvents(rc *requestContext, events []upstreamShapeEventLogRecord) {
	if rc == nil || len(events) == 0 {
		return
	}
	for _, event := range events {
		event.Seq = len(rc.rec.UpstreamShapeEvents) + 1
		if event.TS == "" {
			event.TS = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		}
		rc.rec.UpstreamShapeEvents = append(rc.rec.UpstreamShapeEvents, event)
	}
}

func (s *Service) recordShapeFilterReason(rc *requestContext, target Target, reason string) {
	if !s.decisionTelemetryEnabled() || rc == nil || reason == "" {
		return
	}
	maxReasons := s.cfg.Server.DecisionTelemetry.MaxFilterReasons
	if maxReasons <= 0 {
		maxReasons = 256
	}
	if len(rc.rec.DecisionFilterReasons) >= maxReasons {
		return
	}
	rc.rec.DecisionFilterReasons = append(rc.rec.DecisionFilterReasons, decisionFilterReasonLogRecord{
		Seq:            len(rc.rec.DecisionFilterReasons) + 1,
		CandidateIndex: candidateIndexForTarget(rc.rec.DecisionCandidates, target),
		Stage:          "traffic_shape",
		Reason:         reason,
	})
}

func (s *Service) callUpstreams(ctx context.Context, rc *requestContext, callerDialect string, req *IRRequest, dec decision) (*IRResponse, int, bool, error) {
	targets := append([]Target{dec.Target}, dec.Fallbacks...)
	var lastErr error
	skippedByShape := 0
	var maxRetryAfter time.Duration
	for i, tgt := range targets {
		if err := ctx.Err(); err != nil {
			lastErr = classifyContextError(err)
			break
		}
		shapeInput := shapeInputForRequest(req, callerDialect)
		shapeAd := s.admitTrafficShape(rc, dec.GroupName, tgt, shapeInput)
		if !shapeAd.OK {
			skippedByShape++
			if shapeAd.RetryAfter > maxRetryAfter {
				maxRetryAfter = shapeAd.RetryAfter
			}
			rc.trace("upstream_shape_skipped", shapeAd.Reason, tgt, 0, http.StatusServiceUnavailable, shapeAd.Reason, true, durationMillis(shapeAd.RetryAfter))
			lastErr = upstreamCapacityThrottledError{RetryAfter: maxRetryAfter, TargetCount: len(targets)}
			continue
		}
		attemptIndex := len(rc.rec.AttemptsDetail) + 1
		resp, attempt, err := s.callOne(ctx, rc, callerDialect, req, dec.GroupName, tgt, attemptIndex)
		if attempt.ErrorMessage != "" {
			attempt.ErrorMessage = s.sanitizeDiagnosticError(attempt.ErrorMessage)
		}
		if err == nil {
			attempt.Selected = true
			rc.rec.AttemptsDetail = append(rc.rec.AttemptsDetail, attempt)
			if i > 0 {
				markFallbackTransitionSucceeded(rc, attemptIndex)
			}
			rc.trace("upstream_attempt_ok", "", tgt, attemptIndex, attempt.StatusCode, "", false, attempt.DurationMS)
			return resp, attemptIndex, i > 0, nil
		}
		classified := classifyError(err)
		s.recordAdaptiveTrafficBackoff(rc, dec.GroupName, tgt, classified, shapeInput)
		if i < len(targets)-1 {
			attempt.FallbackReason = classified.Class
		}
		rc.rec.AttemptsDetail = append(rc.rec.AttemptsDetail, attempt)
		rc.trace("upstream_attempt_failed", classified.Message, tgt, attemptIndex, attempt.StatusCode, classified.Class, classified.Retryable, attempt.DurationMS)
		lastErr = err
		if classified.Canceled {
			break
		}
		canFallback := classified.Retryable || classified.Fallbackable
		if !canFallback {
			rc.trace("fallback_stopped", classified.Message, tgt, attemptIndex, attempt.StatusCode, classified.Class, false, attempt.DurationMS)
			break
		}
		if classified.Retryable {
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
		if i < len(targets)-1 {
			s.recordFallbackTransitionTelemetry(rc, targets, i, attemptIndex, classified)
		}
	}
	if skippedByShape == len(targets) {
		return nil, len(rc.rec.AttemptsDetail), false, upstreamCapacityThrottledError{RetryAfter: maxRetryAfter, TargetCount: len(targets)}
	}
	return nil, len(rc.rec.AttemptsDetail), len(rc.rec.AttemptsDetail) > 1, lastErr
}

func (s *Service) callOne(ctx context.Context, rc *requestContext, callerDialect string, req *IRRequest, groupName string, target Target, attemptIndex int) (*IRResponse, attemptLogRecord, error) {
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
	passthrough := requestShapePassthrough(callerDialect, outDialect, req)
	bridge := isResponsesToChatBridge(callerDialect, outDialect, target)
	var upReqBody []byte
	var err error
	if err := s.validateImageURLsForUpstream(ctx, req); err != nil {
		attempt.ErrorClass = "image_url_forbidden"
		attempt.ErrorMessage = err.Error()
		return nil, attempt, upstreamError{Class: "image_url_forbidden", Message: err.Error(), Retryable: false, Err: err}
	}
	if bridge {
		upReqBody, err = encodeResponsesToChatBridge(target.Model, req, target)
	} else if passthrough {
		upReqBody, err = encodeToolPassthrough(outDialect, target.Model, req, target)
	} else if isChatToResponsesBridge(callerDialect, outDialect, target) {
		upReqBody, err = encodeChatToResponsesBridge(target.Model, req, target)
	} else {
		upReqBody, err = encodeUpstreamForTarget(outDialect, target.Model, req, target)
	}
	attempt.RequestBytes = int64(len(upReqBody))
	if err != nil {
		attempt.ErrorClass = "encode_error"
		attempt.ErrorMessage = err.Error()
		return nil, attempt, upstreamError{Class: "encode_error", Message: err.Error(), Err: err}
	}
	endpoint := upstreamEndpoint(provider.BaseURL, outDialect, target)
	s.recordTranslationShapeTelemetry(rc, req, target, provider, outDialect, endpoint, attemptIndex, upReqBody)
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
		upErr := classifyUpstreamStatus(httpResp.StatusCode, raw)
		upErr.RetryAfter = parseRetryAfterHeader(httpResp.Header.Get("Retry-After"), time.Now().UTC())
		upErr.Details = s.extractUpstreamErrorDetails(raw, httpResp.StatusCode, upErr.Class, attemptIndex, attempt.TS)
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ResponseBytes = int64(len(raw))
		attempt.ErrorClass = upErr.Class
		attempt.ErrorMessage = upErr.Message
		attempt.Retryable = upErr.Retryable
		attempt.RetryAfterMS = durationMillis(upErr.RetryAfter)
		attempt.ErrorDetails = upErr.Details
		upErr.ResponseLen = attempt.ResponseBytes
		return nil, attempt, upErr
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, int64(s.diagnosticMaxErrorBytes())))
		upErr := classifyUpstreamStatus(httpResp.StatusCode, raw)
		upErr.RetryAfter = parseRetryAfterHeader(httpResp.Header.Get("Retry-After"), time.Now().UTC())
		upErr.Details = s.extractUpstreamErrorDetails(raw, httpResp.StatusCode, upErr.Class, attemptIndex, attempt.TS)
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ResponseBytes = int64(len(raw))
		attempt.ErrorClass = upErr.Class
		attempt.ErrorMessage = upErr.Message
		attempt.Retryable = upErr.Retryable
		attempt.RetryAfterMS = durationMillis(upErr.RetryAfter)
		attempt.ErrorDetails = upErr.Details
		upErr.ResponseLen = attempt.ResponseBytes
		return nil, attempt, upErr
	}
	raw, oversized, err := s.readUpstreamSuccessBody(httpResp.Body)
	if err != nil {
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ErrorClass = "read_error"
		attempt.ErrorMessage = err.Error()
		attempt.Retryable = true
		return nil, attempt, upstreamError{Class: "read_error", Message: err.Error(), Retryable: true, Err: err}
	}
	if oversized {
		attempt.DurationMS = durationMillis(time.Since(start))
		attempt.ResponseBytes = int64(len(raw))
		attempt.ErrorClass = "upstream_response_too_large"
		attempt.ErrorMessage = "upstream response exceeded configured size limit"
		attempt.Retryable = false
		return nil, attempt, upstreamError{Class: "upstream_response_too_large", Message: "upstream response exceeded configured size limit", Retryable: false, ResponseLen: attempt.ResponseBytes}
	}
	attempt.DurationMS = durationMillis(time.Since(start))
	attempt.ResponseBytes = int64(len(raw))
	if bridge {
		resp, err := decodeResponsesToChatBridge(raw, target.Model)
		if err != nil {
			attempt.ErrorClass = "decode_error"
			attempt.ErrorMessage = err.Error()
			attempt.Retryable = true
			return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Retryable: true, Err: err}
		}
		return resp, attempt, nil
	}
	if passthrough {
		resp, err := decodeToolPassthrough(outDialect, raw, target.Model)
		if err != nil {
			attempt.ErrorClass = "decode_error"
			attempt.ErrorMessage = err.Error()
			attempt.Retryable = true
			return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Retryable: true, Err: err}
		}
		return resp, attempt, nil
	}
	if isChatToResponsesBridge(callerDialect, outDialect, target) {
		resp, err := decodeChatToResponsesBridgeResponse(raw, target.Model)
		if err != nil {
			attempt.ErrorClass = "decode_error"
			attempt.ErrorMessage = err.Error()
			attempt.Retryable = true
			return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Retryable: true, Err: err}
		}
		return resp, attempt, nil
	}
	resp, err := decodeUpstreamResponse(outDialect, raw, target.Model)
	if err != nil {
		attempt.ErrorClass = "decode_error"
		attempt.ErrorMessage = err.Error()
		attempt.Retryable = true
		return nil, attempt, upstreamError{Class: "decode_error", Message: err.Error(), Retryable: true, Err: err}
	}
	return resp, attempt, nil
}

func (s *Service) readUpstreamSuccessBody(body io.Reader) ([]byte, bool, error) {
	limit := int64(32 << 20)
	if s != nil && s.cfg != nil && s.cfg.Server.Upstream.MaxResponseBytes > 0 {
		limit = int64(s.cfg.Server.Upstream.MaxResponseBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(raw)) > limit {
		return raw[:limit], true, nil
	}
	return raw, false, nil
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

func (s *Service) validateImageURLsForUpstream(ctx context.Context, req *IRRequest) error {
	if s == nil || s.cfg == nil || s.cfg.Server.Upstream.AllowPrivateImageURLs || req == nil {
		return nil
	}
	for _, imageURL := range requestImageURLs(req) {
		if err := validateImageURL(ctx, imageURL); err != nil {
			return err
		}
	}
	return nil
}

func requestImageURLs(req *IRRequest) []string {
	if req == nil {
		return nil
	}
	var out []string
	collect := func(parts []IRContentPart) {
		for _, part := range parts {
			if part.Type == "image" && strings.TrimSpace(part.ImageURL) != "" {
				out = append(out, strings.TrimSpace(part.ImageURL))
			}
		}
	}
	collect(req.InputParts)
	for _, msg := range req.Messages {
		collect(msg.Parts)
	}
	return out
}

func validateImageURL(ctx context.Context, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("image URL is invalid")
	}
	return validateImageURLAddress(ctx, parsed)
}

func validateImageURLAddress(ctx context.Context, parsed *url.URL) error {
	switch strings.ToLower(parsed.Scheme) {
	case "data":
		return nil
	case "http", "https":
	default:
		return fmt.Errorf("image URL scheme is not allowed")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("image URL host is required")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("image URL host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if privateImageIP(ip) {
			return fmt.Errorf("image URL host is not allowed")
		}
		return nil
	}
	resolveCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		resolveCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
	}
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("image URL host could not be resolved")
	}
	for _, addr := range addrs {
		if privateImageIP(addr.IP) {
			return fmt.Errorf("image URL host resolves to a private or reserved address")
		}
	}
	return nil
}

func privateImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if !ip.IsGlobalUnicast() {
		return true
	}
	for _, network := range reservedImageURLNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

var reservedImageURLNetworks = mustParseCIDRs([]string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"255.255.255.255/32",
	"::/128",
	"::1/128",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"2001::/23",
	"2001:db8::/32",
	"2002::/16",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
})

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		out = append(out, network)
	}
	return out
}

func (s *Service) diagnosticMaxErrorBytes() int {
	if s == nil || s.cfg == nil || s.cfg.Server.Diagnostics.MaxErrorBytes <= 0 {
		return 2048
	}
	return s.cfg.Server.Diagnostics.MaxErrorBytes
}

func classifyUpstreamStatus(status int, raw []byte) upstreamError {
	class := statusErrorClass(status, raw)
	return upstreamError{
		Class:        class,
		Message:      upstreamStatusMessage(status, raw, class),
		StatusCode:   status,
		Retryable:    class == "upstream_quota_exhausted" || class == "upstream_rate_limited" || class == "upstream_timeout" || status >= 500,
		Fallbackable: upstreamAccessFailureFallbackable(class),
	}
}

func statusErrorClass(status int, raw []byte) string {
	switch {
	case upstreamBodyIndicatesQuotaExhausted(status, raw):
		return "upstream_quota_exhausted"
	case status == http.StatusTooManyRequests:
		return "upstream_rate_limited"
	case upstreamBodyIndicatesAuthFailure(status, raw):
		return "upstream_auth_failed"
	case upstreamBodyIndicatesModelAccessDenied(status, raw):
		return "upstream_model_access_denied"
	case upstreamBodyIndicatesEntitlementFailure(status, raw):
		return "upstream_entitlement_failed"
	case status == http.StatusUnauthorized:
		return "upstream_auth_failed"
	case status == http.StatusForbidden:
		return "upstream_access_denied"
	case status == http.StatusNotFound:
		return "upstream_model_access_denied"
	case status == http.StatusRequestTimeout:
		return "upstream_timeout"
	case status == http.StatusRequestEntityTooLarge:
		return "upstream_request_too_large"
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return "upstream_bad_request"
	case status >= 500:
		return "upstream_status_5xx"
	default:
		return "upstream_status"
	}
}

func upstreamAccessFailureFallbackable(class string) bool {
	switch class {
	case "upstream_access_denied", "upstream_entitlement_failed", "upstream_model_access_denied":
		return true
	default:
		return false
	}
}

func upstreamStatusMessage(status int, raw []byte, class string) string {
	if class == "upstream_quota_exhausted" {
		return fmt.Sprintf("upstream status %d upstream provider quota, credits, or billing limit exhausted", status)
	}
	if upstreamAccessFailureFallbackable(class) || class == "upstream_auth_failed" {
		return fmt.Sprintf("upstream status %d upstream provider access, authorization, or entitlement failed", status)
	}
	msg := fmt.Sprintf("upstream status %d", status)
	snippet := strings.TrimSpace(string(raw))
	if snippet == "" {
		return msg
	}
	return msg + ": " + snippet
}

func upstreamBodyIndicatesQuotaExhausted(status int, raw []byte) bool {
	text := strings.ToLower(strings.TrimSpace(string(raw)))
	if status == http.StatusPaymentRequired {
		return true
	}
	if text == "" {
		return false
	}
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(text)
	quotaMarkers := []string{
		"insufficient_quota",
		"quota_exceeded",
		"quota_exhausted",
		"billing_hard_limit_reached",
		"billing_not_active",
		"billing_disabled",
		"billing_limit",
		"insufficient_credit",
		"insufficient_credits",
		"not_enough_credit",
		"not_enough_credits",
		"credit_exhausted",
		"credits_exhausted",
		"credit_balance",
		"exhausted_credit",
		"exhausted_credits",
		"insufficient_balance",
		"balance_exhausted",
		"balance_too_low",
		"payment_required",
		"payment_method",
		"spend_limit",
		"usage_limit",
		"account_balance",
	}
	for _, marker := range quotaMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func upstreamBodyIndicatesAuthFailure(status int, raw []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	normalized := normalizedUpstreamErrorText(raw)
	if normalized == "" {
		return false
	}
	markers := []string{
		"invalid_api_key",
		"invalid_key",
		"api_key_invalid",
		"authentication_failed",
		"invalid_auth",
		"invalid_token",
		"missing_api_key",
		"unauthorized_api_key",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func upstreamBodyIndicatesEntitlementFailure(status int, raw []byte) bool {
	if status != http.StatusForbidden {
		return false
	}
	normalized := normalizedUpstreamErrorText(raw)
	if normalized == "" {
		return false
	}
	markers := []string{
		"entitlement",
		"not_entitled",
		"insufficient_permission",
		"permission_denied",
		"permission_required",
		"account_not_authorized",
		"not_enabled",
		"requires_approval",
		"project_restricted",
		"region_restricted",
		"privacy",
		"policy_block",
		"access_denied",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func upstreamBodyIndicatesModelAccessDenied(status int, raw []byte) bool {
	if status != http.StatusForbidden && status != http.StatusNotFound {
		return false
	}
	normalized := normalizedUpstreamErrorText(raw)
	if normalized == "" {
		return status == http.StatusNotFound
	}
	markers := []string{
		"model_not_found",
		"model_not_available",
		"model_unavailable",
		"model_access",
		"model_access_denied",
		"model_not_enabled",
		"served_model_not_found",
		"not_found_for_model",
		"unknown_model",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return status == http.StatusNotFound
}

func normalizedUpstreamErrorText(raw []byte) string {
	text := strings.ToLower(strings.TrimSpace(string(raw)))
	if text == "" {
		return ""
	}
	return strings.NewReplacer("-", "_", " ", "_").Replace(text)
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
	case "upstream-timeout", "upstream-rate-limited", "upstream-quota-exhausted", "upstream-capacity-throttled", "upstream-failed":
		return true
	case "concurrency-exceeded", "rpm-exceeded", "tpm-exceeded", "quota-exhausted", "traffic-shaped":
		return true
	default:
		return false
	}
}

func toolPassthrough(callerDialect, outDialect string, req *IRRequest) bool {
	return callerDialect == outDialect && len(req.Tools) > 0 && (outDialect == "openai-responses" || outDialect == "anthropic" || outDialect == "openai-chat")
}

func requestShapePassthrough(callerDialect, outDialect string, req *IRRequest) bool {
	if toolPassthrough(callerDialect, outDialect, req) {
		return true
	}
	return callerDialect == outDialect && requestHasStructuredOutput(req) && (outDialect == "openai-responses" || outDialect == "openai-chat")
}

func (s *Service) targetsForRequest(rc *requestContext, targets []Target, req *IRRequest, callerDialect string) []Target {
	requiredModalities := requestInputModalities(req)
	requiresStructuredOutput := requestHasStructuredOutput(req)
	estimate := s.requestTokenEstimateForSelection(rc, req, callerDialect)
	if len(req.Tools) == 0 {
		out := make([]Target, 0, len(targets))
		for _, target := range targets {
			provider := s.cfg.Provider[target.Provider]
			outDialect := targetDialect(provider, target)
			fit := s.targetRequestShapeFit(target, req, callerDialect, outDialect, estimate)
			if !target.ToolOnly &&
				targetSupportsCallerDialect(target, req, callerDialect, outDialect) &&
				targetSupportsInputModalities(target, requiredModalities) &&
				targetSupportsStructuredOutput(target, callerDialect, outDialect, requiresStructuredOutput) &&
				targetCanSatisfyReasoning(target, outDialect, req) &&
				targetHonorsExplicitMaxTokens(target, req) &&
				fit.FilterReason == "" {
				out = append(out, target)
			}
		}
		return out
	}
	out := make([]Target, 0, len(targets))
	for _, target := range targets {
		provider := s.cfg.Provider[target.Provider]
		outDialect := targetDialect(provider, target)
		fit := s.targetRequestShapeFit(target, req, callerDialect, outDialect, estimate)
		if targetSupportsCallerDialect(target, req, callerDialect, outDialect) &&
			targetSupportsToolsForCallerDialect(target, callerDialect, outDialect) &&
			targetSupportsInputModalities(target, requiredModalities) &&
			targetSupportsStructuredOutput(target, callerDialect, outDialect, requiresStructuredOutput) &&
			targetCanSatisfyReasoning(target, outDialect, req) &&
			targetHonorsExplicitMaxTokens(target, req) &&
			fit.FilterReason == "" {
			out = append(out, target)
		}
	}
	return out
}

func isChatToResponsesDialectPair(callerDialect, outDialect string) bool {
	return normalizeDialect(callerDialect) == "openai-chat" && normalizeDialect(outDialect) == "openai-responses"
}

func routingRequirements(req *IRRequest, callerDialect string) []string {
	requirements := requestInputModalities(req)
	if len(req.Tools) > 0 {
		requirements = append(requirements, "tools", callerDialect+"_tool_passthrough")
	}
	if requestHasStructuredOutput(req) {
		requirements = append(requirements, "structured_outputs")
	}
	if requestRequiresReasoning(req) {
		requirements = append(requirements, "reasoning")
	}
	if req.MaxTokens > 0 {
		requirements = append(requirements, "max_tokens")
	}
	return requirements
}

func targetHonorsExplicitMaxTokens(target Target, req *IRRequest) bool {
	if req == nil || req.MaxTokens <= 0 || target.HonorsMaxTokens == nil {
		return true
	}
	return *target.HonorsMaxTokens
}

func targetSupportsCallerDialect(target Target, req *IRRequest, callerDialect, outDialect string) bool {
	if callerDialect == outDialect {
		return true
	}
	if normalizeDialect(callerDialect) == "openai-chat" && normalizeDialect(outDialect) == "openai-responses" {
		return chatToResponsesBridgeFilterReason(target, req, callerDialect, outDialect) == ""
	}
	if normalizeDialect(callerDialect) == "openai-responses" && normalizeDialect(outDialect) == "openai-chat" {
		return responsesToChatBridgeFilterReason(target, req, callerDialect, outDialect) == ""
	}
	return true
}

func targetSupportsToolsForCallerDialect(target Target, callerDialect, outDialect string) bool {
	if isChatToResponsesBridge(callerDialect, outDialect, target) {
		return targetSupportsTools(target, "openai-responses")
	}
	if isResponsesToChatBridge(callerDialect, outDialect, target) {
		return targetSupportsTools(target, "openai-chat")
	}
	if callerDialect != outDialect {
		return false
	}
	return targetSupportsTools(target, outDialect)
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
		return supportsAnyCapability(target.ToolSupport.OpenAIResponses, "function", "functions", "tools")
	case "anthropic":
		return supportsAnyCapability(target.ToolSupport.AnthropicMessages, "client_tools", "tools", "tool_use")
	case "openai", "openai-chat":
		if toolSupportEmpty(target.ToolSupport) {
			return false
		}
		return supportsAnyCapability(target.ToolSupport.OpenAIChat, "tools", "function", "functions", "function_tools", "tool_choice", "forced_tool_choice")
	default:
		return false
	}
}

func requestHasForbiddenProviderHostedTools(req *IRRequest) bool {
	if req == nil {
		return false
	}
	for _, tool := range req.Tools {
		if forbiddenProviderHostedResponsesToolType(stringValue(tool["type"])) {
			return true
		}
	}
	return false
}

func forbiddenProviderHostedResponsesToolType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mcp", "sse", "file_search", "code_interpreter", "computer_use_preview":
		return true
	default:
		return false
	}
}

func filterResponsesToolsForUpstream(tools any) any {
	rawTools, ok := tools.([]any)
	if !ok {
		return tools
	}
	out := make([]any, 0, len(rawTools))
	for _, rawTool := range rawTools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			out = append(out, rawTool)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(stringValue(tool["type"]))) {
		case "web_search", "web_search_preview", "image_generation":
			continue
		default:
			out = append(out, rawTool)
		}
	}
	return out
}

func supportsAnyCapability(values []string, capabilities ...string) bool {
	for _, capability := range capabilities {
		if stringSliceContains(values, capability) {
			return true
		}
	}
	return false
}

func targetSupportsStructuredOutput(target Target, callerDialect, outDialect string, required bool) bool {
	if !required {
		return true
	}
	if isResponsesToChatBridge(callerDialect, outDialect, target) && target.ResponsesToChat.StructuredOutputs {
		return targetSupportsCapability(target, "openai-chat", "structured_outputs", "json_schema")
	}
	if isChatToResponsesBridge(callerDialect, outDialect, target) {
		return target.Bridges.ChatToResponses.StructuredOutputs && targetSupportsCapability(target, outDialect, "structured_outputs", "json_schema")
	}
	if callerDialect != outDialect {
		return false
	}
	return targetSupportsCapability(target, outDialect, "structured_outputs", "json_schema")
}

func encodeToolPassthrough(dialect, model string, req *IRRequest, target Target) ([]byte, error) {
	switch dialect {
	case "anthropic":
		return encodeAnthropicPassthrough(model, req, target)
	case "openai-chat":
		return encodeChatPassthrough(model, req, target)
	default:
		return encodeResponsesPassthrough(model, req, target)
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
	if len(s.targetsForRequest(nil, group.Targets, req, "openai-responses")) == 0 {
		if len(s.targetsForRequest(nil, group.Targets, req, "anthropic")) == 0 {
			if len(s.targetsForRequest(nil, group.Targets, req, "openai-chat")) == 0 {
				return []string{}
			}
		}
	}
	return []string{"local_shell", "apply_patch"}
}

func (s *Service) reasoningMetadataForGroup(name string) ([]string, bool, string) {
	group, ok := s.cfg.Models[name]
	if !ok {
		return []string{}, false, "none"
	}
	levels := map[string]bool{}
	summaries := false
	defaultOn := false
	for _, target := range group.Targets {
		if target.ToolOnly || !target.Reasoning.Supported {
			continue
		}
		if target.Reasoning.Control == reasoningControlEffortEnum || target.Reasoning.Control == reasoningControlTokenBudget {
			levels["low"] = true
			levels["medium"] = true
			levels["high"] = true
		}
		if target.Reasoning.SupportsSummaries {
			summaries = true
		}
		if target.Reasoning.DefaultOn {
			defaultOn = true
		}
	}
	out := make([]string, 0, len(levels))
	for _, level := range []string{"low", "medium", "high"} {
		if levels[level] {
			out = append(out, level)
		}
	}
	defaultLevel := "none"
	if defaultOn && len(out) > 0 {
		defaultLevel = "medium"
	}
	return out, summaries, defaultLevel
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

func recordTrafficShapeResult(rc *requestContext, result trafficShapeResult) {
	if rc == nil || !result.Applied {
		return
	}
	rc.rec.TrafficShapeApplied = true
	previousDecision := rc.rec.TrafficShapeDecision
	rc.rec.TrafficShapeDecision = mergeTrafficShapeDecision(rc.rec.TrafficShapeDecision, result.Decision)
	rc.rec.TrafficShapeScope = result.Scope
	if result.Bucket != "" && shouldReplaceTrafficShapeBucket(previousDecision, result.Decision) {
		rc.rec.TrafficShapeBucket = result.Bucket
	}
	if result.RetryAfterMS > rc.rec.TrafficShapeRetryAfterMS {
		rc.rec.TrafficShapeRetryAfterMS = result.RetryAfterMS
	}
	if result.QueueWaitMS > rc.rec.TrafficShapeQueueWaitMS {
		rc.rec.TrafficShapeQueueWaitMS = result.QueueWaitMS
	}
	rc.rec.TrafficShapeEstimatedInputTokens = result.EstimatedInputTokens
	rc.rec.TrafficShapeReservedOutputTokens = result.ReservedOutputTokens
	rc.rec.TrafficShapeTotalReservedTokens = result.TotalReservedTokens
	if len(result.Events) > 0 {
		offset := len(rc.rec.TrafficShapeEvents)
		for _, event := range result.Events {
			event.Seq += offset
			rc.rec.TrafficShapeEvents = append(rc.rec.TrafficShapeEvents, event)
		}
	}
}

func mergeTrafficShapeDecision(current, next string) string {
	switch current {
	case "":
		return next
	case trafficShapeDecisionRejected:
		return current
	case trafficShapeDecisionQueued:
		if next == trafficShapeDecisionRejected {
			return next
		}
		return current
	default:
		if next == trafficShapeDecisionRejected || next == trafficShapeDecisionQueued {
			return next
		}
		return current
	}
}

func shouldReplaceTrafficShapeBucket(current, next string) bool {
	if current == "" {
		return true
	}
	if next == trafficShapeDecisionRejected {
		return true
	}
	if next == trafficShapeDecisionQueued && current != trafficShapeDecisionRejected {
		return true
	}
	return false
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

func (s *Service) writeTrafficShapeError(w http.ResponseWriter, rc *requestContext, modelGroup string, result trafficShapeResult) {
	code := "traffic-shaped"
	retrySeconds := int64(0)
	if result.RetryAfterMS > 0 {
		retrySeconds = int64(math.Ceil(float64(result.RetryAfterMS) / 1000))
		if retrySeconds < 1 {
			retrySeconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySeconds))
	}
	message := fmt.Sprintf("caller traffic shaping limit exceeded for model group %s; retry later or reduce request burst", modelGroup)
	if rc != nil {
		rc.trace("traffic_shape_rejected", result.Bucket, Target{}, 0, http.StatusTooManyRequests, code, true, 0)
		rc.rec.Status = http.StatusTooManyRequests
		rc.rec.Error = &code
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = message
		s.finish(rc, http.StatusTooManyRequests, &code)
	}
	errBody := map[string]any{
		"type":       code,
		"message":    message,
		"request_id": "",
		"bucket":     result.Bucket,
	}
	if rc != nil {
		errBody["request_id"] = rc.id
	}
	if retrySeconds > 0 {
		errBody["retry_after_seconds"] = retrySeconds
	}
	writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": errBody})
}

func (s *Service) writeRoutingEligibilityError(w http.ResponseWriter, rc *requestContext, err routingEligibilityError) {
	code := "no-eligible-target"
	requestID := ""
	if rc != nil {
		requestID = rc.id
		rc.trace("routing_no_eligible_target", strings.Join(err.Requirements, ","), Target{}, 0, http.StatusBadGateway, code, false, 0)
		rc.rec.Status = http.StatusBadGateway
		rc.rec.Error = &code
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = fmt.Sprintf("no eligible upstream target for %s", err.Model)
		if err.ContractPresent {
			rc.rec.ContractPresent = true
			rc.rec.ContractBucket = "failed"
			rc.rec.ContractFailureReason = err.ContractReason
		}
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
				"request_id":   requestID,
				"requirements": err.Requirements,
				"hint":         "ask the router administrator to add or enable an upstream target for this model group that supports the requested API dialect, tools, and input modalities",
			},
		},
	})
}

func (s *Service) writeRoutingPolicyError(w http.ResponseWriter, rc *requestContext, err routingPolicyError) {
	code := "routing-policy-error"
	message := s.sanitizeDiagnosticError(err.Error())
	if message == "" {
		message = "external routing policy failed"
	}
	if rc != nil {
		rc.trace("routing_policy_error", message, Target{}, 0, http.StatusBadGateway, code, false, 0)
		rc.rec.Status = http.StatusBadGateway
		rc.rec.Error = &code
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = message
		s.finish(rc, http.StatusBadGateway, &code)
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{
		"error": map[string]any{
			"type":    code,
			"message": message,
			"details": map[string]any{
				"model": err.Group,
				"hint":  "the configured external routing policy service did not return a valid target decision",
			},
		},
	})
}

func (s *Service) writeUpstreamFailureError(w http.ResponseWriter, rc *requestContext, req *IRRequest, dec decision, attempts int, err error, captureDecision contentCaptureDecision) {
	var shapeErr upstreamCapacityThrottledError
	if errors.As(err, &shapeErr) {
		s.writeUpstreamCapacityThrottledError(w, rc, req.Model, rc.dialect, shapeErr)
		return
	}
	classified := classifyError(err)
	code, status := upstreamFailureResponse(classified, rc)
	if rc != nil {
		s.captureUpstreamErrorContent(rc, req, dec, attempts, classified, captureDecision, status)
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
	message := callerUpstreamFailureMessage(code, req.Model, attempts)
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

func (s *Service) writeUpstreamCapacityThrottledError(w http.ResponseWriter, rc *requestContext, model, dialect string, err upstreamCapacityThrottledError) {
	code := "upstream-capacity-throttled"
	status := http.StatusServiceUnavailable
	if header := retryAfterHeaderValue(err.RetryAfter); header != "" {
		w.Header().Set("Retry-After", header)
	}
	if rc != nil {
		rc.trace("upstream_capacity_throttled", code, Target{}, 0, status, code, true, durationMillis(err.RetryAfter))
		rc.rec.Status = status
		rc.rec.Error = &code
		rc.rec.ErrorClass = code
		rc.rec.ErrorMessage = code
		s.finish(rc, status, &code)
	}
	details := map[string]any{
		"model":        model,
		"dialect":      dialect,
		"target_count": err.TargetCount,
		"retryable":    true,
		"request_id":   "",
		"fallbackUsed": false,
	}
	if rc != nil {
		details["request_id"] = rc.id
	}
	if retryMS := int64(math.Ceil(float64(err.RetryAfter) / float64(time.Millisecond))); retryMS > 0 {
		details["retry_after_ms"] = retryMS
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    code,
			"message": fmt.Sprintf("all currently eligible upstream targets for model %q are temporarily capacity-throttled; retry later or contact the router operator with the request_id", model),
			"details": details,
		},
	})
}

func upstreamFailureResponse(err upstreamError, rc *requestContext) (string, int) {
	switch {
	case err.Canceled:
		return "client-canceled", 499
	case err.TimedOut || err.Class == "upstream_timeout":
		return "upstream-timeout", http.StatusGatewayTimeout
	case attemptsAllClassOrLast(rc, err, "upstream_quota_exhausted"):
		return "upstream-quota-exhausted", http.StatusServiceUnavailable
	case attemptsAllClassOrLast(rc, err, "upstream_rate_limited"):
		return "upstream-rate-limited", http.StatusServiceUnavailable
	case attemptsAllAccessFailureOrLast(rc, err):
		return "upstream-access-denied", http.StatusServiceUnavailable
	default:
		return "upstream-failed", http.StatusBadGateway
	}
}

func attemptsAllAccessFailureOrLast(rc *requestContext, err upstreamError) bool {
	if rc == nil || len(rc.rec.AttemptsDetail) == 0 {
		return upstreamAccessFailureFallbackable(err.Class) || err.Class == "upstream_auth_failed"
	}
	for _, attempt := range rc.rec.AttemptsDetail {
		if !upstreamAccessFailureFallbackable(attempt.ErrorClass) && attempt.ErrorClass != "upstream_auth_failed" {
			return false
		}
	}
	return true
}

func attemptsAllClassOrLast(rc *requestContext, err upstreamError, class string) bool {
	if rc == nil || len(rc.rec.AttemptsDetail) == 0 {
		return err.Class == class
	}
	return attemptsAllClass(rc, class)
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

func callerUpstreamFailureMessage(code, model string, attempts int) string {
	switch code {
	case "upstream-quota-exhausted":
		return fmt.Sprintf("upstream provider quota, credits, or billing limits were exhausted for model %q after %d attempt(s); retry later or contact the router operator with the request_id", model, attempts)
	case "upstream-rate-limited":
		return fmt.Sprintf("upstream providers were rate limited for model %q after %d attempt(s); retry later or contact the router operator with the request_id", model, attempts)
	case "upstream-timeout":
		return fmt.Sprintf("upstream providers timed out for model %q after %d attempt(s); retry with a smaller request or contact the router operator with the request_id", model, attempts)
	case "upstream-access-denied":
		return fmt.Sprintf("upstream provider access or entitlement failed for model %q after %d attempt(s); contact the router operator with the request_id", model, attempts)
	default:
		return fmt.Sprintf("all eligible upstream targets failed for model %q after %d attempt(s)", model, attempts)
	}
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
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		return r.UserAgent()
	}
	return "unknown"
}

func callerIP(r *http.Request) string {
	return resolveClientIP(r, ClientIPConfig{}).Address
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
