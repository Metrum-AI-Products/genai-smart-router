package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const externalPolicyDefaultTimeoutMS = 500

type externalPolicyStrategy struct {
	cfg ExternalPolicyConfig
}

type externalPolicyInput struct {
	Group           string          `json:"group"`
	Request         *IRRequest      `json:"request,omitempty"`
	Context         requestSummary  `json:"context"`
	Contract        *scriptContract `json:"contract,omitempty"`
	Requirements    []string        `json:"requirements"`
	Targets         []scriptTarget  `json:"targets"`
	AllTargets      []scriptTarget  `json:"allTargets,omitempty"`
	Caller          *scriptCaller   `json:"caller,omitempty"`
	Text            string          `json:"text,omitempty"`
	InputModalities []string        `json:"inputModalities"`
	Now             string          `json:"now"`
}

type requestSummary struct {
	Model               string   `json:"model,omitempty"`
	Dialect             string   `json:"dialect,omitempty"`
	EstimatedTokens     int      `json:"estimatedTokens"`
	TextChars           int      `json:"textChars"`
	SystemChars         int      `json:"systemChars,omitempty"`
	InputChars          int      `json:"inputChars,omitempty"`
	InputPartCount      int      `json:"inputPartCount,omitempty"`
	MessageCount        int      `json:"messageCount,omitempty"`
	MessageTextChars    int      `json:"messageTextChars,omitempty"`
	MessagePartCount    int      `json:"messagePartCount,omitempty"`
	ImageCount          int      `json:"imageCount,omitempty"`
	ToolCount           int      `json:"toolCount,omitempty"`
	HasTools            bool     `json:"hasTools"`
	HasStructuredOutput bool     `json:"hasStructuredOutput"`
	MaxTokens           int      `json:"maxTokens,omitempty"`
	MaxTokensField      string   `json:"maxTokensField,omitempty"`
	TemperatureSet      bool     `json:"temperatureSet"`
	Stream              bool     `json:"stream"`
	StopCount           int      `json:"stopCount,omitempty"`
	MetadataKeys        []string `json:"metadataKeys,omitempty"`
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

func (s externalPolicyStrategy) Pick(group string, req *IRRequest, contract *ModelGroupContract, eligibleTargets, allTargets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID, callerDialect string) (decision, error) {
	dec, err := s.pick(group, req, contract, eligibleTargets, allTargets, providers, caller, tokenID, callerDialect)
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

func (s externalPolicyStrategy) pick(group string, req *IRRequest, contract *ModelGroupContract, eligibleTargets, allTargets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID, callerDialect string) (decision, error) {
	input := externalPolicyInput{
		Group:           group,
		Context:         buildRequestSummary(req, callerDialect),
		Contract:        buildScriptContract(contract),
		Requirements:    routingRequirements(req, callerDialect),
		Targets:         buildScriptTargets(eligibleTargets, providers),
		AllTargets:      buildScriptTargets(allTargets, providers),
		Caller:          buildScriptCaller(caller, tokenID),
		InputModalities: requestInputModalities(req),
		Now:             time.Now().UTC().Format(time.RFC3339),
	}
	if s.cfg.IncludeRequest {
		input.Request = req
		input.Text = requestText(req)
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

func buildRequestSummary(req *IRRequest, callerDialect string) requestSummary {
	if req == nil {
		return requestSummary{Dialect: callerDialect, EstimatedTokens: 1}
	}
	messageTextChars, messagePartCount := canonicalMessageTextChars(req.Messages)
	inputPartCount := 0
	if len(req.Messages) == 0 {
		inputPartCount = len(req.InputParts)
	}
	metadataKeys := make([]string, 0, len(req.Metadata))
	for key := range req.Metadata {
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)
	inputChars := canonicalInputTextChars(req)
	textChars := len(req.System) + inputChars + messageTextChars
	return requestSummary{
		Model:               req.Model,
		Dialect:             callerDialect,
		EstimatedTokens:     estimateSummaryTokens(req, textChars),
		TextChars:           textChars,
		SystemChars:         len(req.System),
		InputChars:          inputChars,
		InputPartCount:      inputPartCount,
		MessageCount:        len(req.Messages),
		MessageTextChars:    messageTextChars,
		MessagePartCount:    messagePartCount,
		ImageCount:          requestImageCount(req),
		ToolCount:           len(req.Tools),
		HasTools:            len(req.Tools) > 0,
		HasStructuredOutput: requestHasStructuredOutput(req),
		MaxTokens:           req.MaxTokens,
		MaxTokensField:      req.MaxTokensField,
		TemperatureSet:      req.Temperature != nil,
		Stream:              req.Stream,
		StopCount:           len(req.Stop),
		MetadataKeys:        metadataKeys,
	}
}

func canonicalMessageTextChars(messages []IRMessage) (int, int) {
	textChars := 0
	partCount := 0
	for _, msg := range messages {
		if len(msg.Parts) == 0 {
			textChars += len(msg.Content)
			continue
		}
		for _, part := range msg.Parts {
			partCount++
			textChars += len(part.Text)
		}
	}
	return textChars, partCount
}

func canonicalInputTextChars(req *IRRequest) int {
	if req == nil || len(req.Messages) > 0 {
		return 0
	}
	if len(req.InputParts) > 0 {
		return contentPartTextChars(req.InputParts)
	}
	return len(req.Input)
}

func contentPartTextChars(parts []IRContentPart) int {
	textChars := 0
	for _, part := range parts {
		textChars += len(part.Text)
	}
	return textChars
}

func estimateSummaryTokens(req *IRRequest, textChars int) int {
	chars := textChars
	if req != nil {
		chars += requestImageCount(req) * 1024
	}
	if chars == 0 {
		return 1
	}
	return chars/4 + 1
}
