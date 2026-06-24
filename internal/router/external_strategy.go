package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const externalPolicyDefaultTimeoutMS = 500

type externalPolicyStrategy struct {
	cfg ExternalPolicyConfig
}

type externalPolicyInput struct {
	Group           string         `json:"group"`
	Request         *IRRequest     `json:"request"`
	Requirements    []string       `json:"requirements"`
	Targets         []scriptTarget `json:"targets"`
	AllTargets      []scriptTarget `json:"allTargets,omitempty"`
	Caller          *scriptCaller  `json:"caller,omitempty"`
	Text            string         `json:"text"`
	InputModalities []string       `json:"inputModalities"`
	Now             string         `json:"now"`
}

type routingPolicyError struct {
	Group   string
	Message string
	Err     error
}

func (e routingPolicyError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "routing policy failed"
}

func (e routingPolicyError) Unwrap() error {
	return e.Err
}

func (s externalPolicyStrategy) Pick(group string, req *IRRequest, eligibleTargets, allTargets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID, callerDialect string) (decision, error) {
	dec, err := s.pick(group, req, eligibleTargets, allTargets, providers, caller, tokenID, callerDialect)
	if err == nil {
		return dec, nil
	}
	if strings.EqualFold(strings.TrimSpace(s.cfg.OnError), "fallback") {
		label := "external-policy:fallback"
		return decision{
			Target:     eligibleTargets[0],
			Fallbacks:  eligibleTargets[1:],
			ClassLabel: &label,
			Strategy:   "external",
			GroupName:  group,
		}, nil
	}
	return decision{}, routingPolicyError{Group: group, Message: err.Error(), Err: err}
}

func (s externalPolicyStrategy) pick(group string, req *IRRequest, eligibleTargets, allTargets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID, callerDialect string) (decision, error) {
	input := externalPolicyInput{
		Group:           group,
		Request:         req,
		Requirements:    routingRequirements(req, callerDialect),
		Targets:         buildScriptTargets(eligibleTargets, providers),
		AllTargets:      buildScriptTargets(allTargets, providers),
		Caller:          buildScriptCaller(caller, tokenID),
		Text:            requestText(req),
		InputModalities: requestInputModalities(req),
		Now:             time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(input)
	if err != nil {
		return decision{}, err
	}
	raw, err := s.call(body)
	if err != nil {
		return decision{}, err
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return decision{}, fmt.Errorf("external policy response is not JSON")
	}
	out, err := exportDecisionOutput(payload, "external policy")
	if err != nil {
		return decision{}, err
	}
	primary, err := resolveScriptTarget(out, eligibleTargets)
	if err != nil {
		return decision{}, err
	}
	fallbacks, err := resolveScriptFallbacks(out, primary, eligibleTargets)
	if err != nil {
		return decision{}, err
	}
	var classLabel *string
	if out.ClassLabel != "" {
		classLabel = &out.ClassLabel
	}
	return decision{
		Target:     eligibleTargets[primary],
		Fallbacks:  fallbacks,
		ClassLabel: classLabel,
		Strategy:   "external",
		GroupName:  group,
	}, nil
}

func (s externalPolicyStrategy) call(body []byte) ([]byte, error) {
	u, err := url.Parse(s.cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid external_policy.url")
	}
	if err := validateEgressURL(u, s.cfg.AllowHosts, s.cfg.AllowHTTP, "external policy"); err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(s.cfg.Method))
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, fmt.Errorf("external_policy method %s is not allowed", method)
	}
	httpReq, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build external policy request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	for name, value := range s.cfg.Headers {
		if !scriptConfigHeaderAllowed(name) {
			return nil, fmt.Errorf("configured external_policy header %s is not allowed", name)
		}
		httpReq.Header.Set(name, value)
	}
	timeout := time.Duration(s.cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = externalPolicyDefaultTimeoutMS * time.Millisecond
	}
	client := newEgressHTTPClient(timeout, s.cfg.AllowHosts, s.cfg.AllowHTTP, "external policy")
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, auditSafeHTTPError(err, "external policy request failed")
	}
	defer resp.Body.Close()
	limit := s.cfg.MaxResponseBytes
	if limit <= 0 {
		limit = 64 << 10
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("external policy response exceeds max_response_bytes")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external policy returned HTTP %d", resp.StatusCode)
	}
	return raw, nil
}
