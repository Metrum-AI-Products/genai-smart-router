// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	openAIChatEndpointResponsesShapeDisabledError          = "responses-body-on-chat-endpoint-disabled"
	openAIChatEndpointResponsesShapeMixedError             = "responses-body-on-chat-endpoint-mixed-body"
	openAIChatEndpointResponsesShapeUnsupportedFieldError  = "responses-body-on-chat-endpoint-unsupported-field"
	openAIChatEndpointResponsesShapeUnsupportedToolError   = "responses-body-on-chat-endpoint-hosted-tools-unsupported"
	openAIChatEndpointResponsesShapeFormatConflictError    = "responses-body-on-chat-endpoint-format-conflict"
	openAIChatEndpointResponsesShapeFormatUnsupportedError = "responses-body-on-chat-endpoint-format-unsupported"
	openAIChatEndpointResponsesShapeOutputCapConflictError = "responses-body-on-chat-endpoint-output-cap-conflict"
	openAIChatEndpointPreviousResponseUnsupportedError     = "responses-body-on-chat-endpoint-previous-response-id-unsupported"
)

type openAIChatEndpointShape struct {
	Detected bool
	Mixed    bool
	Name     string
}

func detectOpenAIChatEndpointBodyShape(raw map[string]any) openAIChatEndpointShape {
	if len(raw) == 0 {
		return openAIChatEndpointShape{}
	}
	responsesIndicators := []string{
		"input",
		"instructions",
		"reasoning",
		"text",
		"previous_response_id",
		"max_output_tokens",
	}
	detected := false
	for _, key := range responsesIndicators {
		if rawValuePresent(raw, key) {
			detected = true
			break
		}
	}
	if !detected {
		return openAIChatEndpointShape{Name: "openai-chat"}
	}
	return openAIChatEndpointShape{
		Detected: true,
		Mixed:    rawValuePresent(raw, "messages"),
		Name:     "openai-responses",
	}
}

func normalizeResponsesBodyOnChatEndpoint(body []byte) ([]byte, openAIChatEndpointShape, string, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, openAIChatEndpointShape{}, "", err
	}
	shape := detectOpenAIChatEndpointBodyShape(raw)
	if !shape.Detected {
		return body, shape, "", nil
	}
	if shape.Mixed {
		return nil, shape, openAIChatEndpointResponsesShapeMixedError, errors.New(openAIChatEndpointResponsesShapeMixedError)
	}
	if errCode, err := validateResponsesBodyOnChatEndpointSubset(raw); err != nil {
		return nil, shape, errCode, err
	}
	changed := normalizeResponsesBodyOutputCap(raw)
	if format, ok, errCode := responsesTextFormatEquivalent(raw); errCode != "" {
		return nil, shape, errCode, errors.New(errCode)
	} else if ok {
		raw["text"] = map[string]any{"format": format}
		delete(raw, "response_format")
		changed = true
	}
	if changed {
		body, err := json.Marshal(raw)
		if err != nil {
			return nil, shape, "invalid-request", err
		}
		return body, shape, "", nil
	}
	return body, shape, "", nil
}

func validateResponsesBodyOnChatEndpointSubset(raw map[string]any) (string, error) {
	allowed := map[string]bool{
		"model":                 true,
		"input":                 true,
		"instructions":          true,
		"reasoning":             true,
		"tools":                 true,
		"tool_choice":           true,
		"text":                  true,
		"response_format":       true,
		"previous_response_id":  true,
		"max_output_tokens":     true,
		"max_completion_tokens": true,
		"max_tokens":            true,
		"stream":                true,
		"temperature":           true,
		"top_p":                 true,
		"parallel_tool_calls":   true,
	}
	for key := range raw {
		if !allowed[strings.ToLower(strings.TrimSpace(key))] {
			return openAIChatEndpointResponsesShapeUnsupportedFieldError, errors.New(openAIChatEndpointResponsesShapeUnsupportedFieldError)
		}
	}
	for _, tool := range valueAsSlice(raw["tools"]) {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		typ := stringValue(toolMap["type"])
		if forbiddenProviderHostedResponsesToolType(typ) || genericHostedResponsesToolType(typ) {
			return openAIChatEndpointResponsesShapeUnsupportedToolError, errors.New(openAIChatEndpointResponsesShapeUnsupportedToolError)
		}
	}
	capFields := 0
	for _, key := range []string{"max_output_tokens", "max_tokens", "max_completion_tokens"} {
		if rawValuePresent(raw, key) {
			capFields++
		}
	}
	if capFields > 1 {
		return openAIChatEndpointResponsesShapeOutputCapConflictError, errors.New(openAIChatEndpointResponsesShapeOutputCapConflictError)
	}
	return "", nil
}

func normalizeResponsesBodyOutputCap(raw map[string]any) bool {
	hasMaxOutput := rawValuePresent(raw, "max_output_tokens")
	hasMaxTokens := rawValuePresent(raw, "max_tokens")
	hasMaxCompletionTokens := rawValuePresent(raw, "max_completion_tokens")
	if hasMaxOutput {
		delete(raw, "max_tokens")
		delete(raw, "max_completion_tokens")
		return hasMaxTokens || hasMaxCompletionTokens
	}
	switch {
	case hasMaxTokens:
		raw["max_output_tokens"] = raw["max_tokens"]
		delete(raw, "max_tokens")
		delete(raw, "max_completion_tokens")
		return true
	case hasMaxCompletionTokens:
		raw["max_output_tokens"] = raw["max_completion_tokens"]
		delete(raw, "max_completion_tokens")
		return true
	default:
		return false
	}
}

func responsesTextFormatEquivalent(raw map[string]any) (any, bool, string) {
	responseFormat, hasResponseFormat := raw["response_format"]
	if !hasResponseFormat {
		return nil, false, ""
	}
	if nestedValuePresent(raw, "text", "format") {
		return nil, false, openAIChatEndpointResponsesShapeFormatConflictError
	}
	format := chatResponseFormatToResponsesText(responseFormat)
	if format == nil {
		return nil, false, openAIChatEndpointResponsesShapeFormatUnsupportedError
	}
	return format, true, ""
}

func modelGroupHasNativeResponsesTarget(cfg *Config, group ModelGroup) bool {
	if cfg == nil {
		return false
	}
	for _, target := range group.Targets {
		provider := cfg.Provider[target.Provider]
		if normalizeDialect(targetDialect(provider, target)) == "openai-responses" {
			return true
		}
	}
	return false
}
