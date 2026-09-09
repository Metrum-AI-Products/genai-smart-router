// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRequestShapeExtractionOpenAIChatToolsStructuredAndImage(t *testing.T) {
	body := []byte(`{
		"model":"default",
		"stream":true,
		"messages":[
			{"role":"system","content":"do not persist system prompt"},
			{"role":"developer","content":"do not persist developer prompt"},
			{"role":"user","content":[{"type":"text","text":"do not persist prompt"},{"type":"image_url","image_url":{"url":"https://example.test/secret-image.png"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"do not persist tool output"}
		],
		"tools":[{"type":"function","function":{"name":"secret_tool","parameters":{"type":"object","properties":{"secret_field":{"type":"string","description":"do not persist schema text"}}}}}],
		"tool_choice":{"type":"function","function":{"name":"secret_tool"}},
		"parallel_tool_calls":true,
		"response_format":{"type":"json_schema","json_schema":{"name":"secret_schema","schema":{"type":"object"}}},
		"max_tokens":512
	}`)
	req, err := decodeRequest("openai-chat", body, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	shape := requestShapeFromIR(nil, req, "openai-chat", "codex", len(body))
	if !shape.Stream || shape.MessageCount != 4 || shape.SystemMessageCount != 1 || shape.DeveloperMessageCount != 1 || shape.UserMessageCount != 1 {
		t.Fatalf("unexpected counts: %#v", shape)
	}
	if shape.ToolResultCount != 1 || shape.ToolCount != 1 || shape.ToolChoiceMode != "forced" || !shape.StructuredOutputPresent || !shape.ParallelToolCallsPresent {
		t.Fatalf("unexpected tool/structured shape: %#v", shape)
	}
	if shape.ImageCount != 1 || shape.RequestedOutputCapField != "max_tokens" || shape.RequestedOutputCapBucket != "standard" {
		t.Fatalf("unexpected image/cap shape: %#v", shape)
	}
	assertShapeRecordDoesNotContain(t, shape, "do not persist", "secret_tool", "secret_field", "secret-image", "secret_schema")
}

func TestRequestShapeExtractionResponsesContinuationReasoning(t *testing.T) {
	body := []byte(`{
		"model":"default",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"do not persist prompt"}]},
			{"type":"function_call_output","call_id":"call_1","output":"do not persist tool output"}
		],
		"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"secret":{"type":"string"}}}}],
		"tool_choice":"auto",
		"reasoning":{"effort":"high","summary":"auto"},
		"include":["reasoning.encrypted_content"],
		"truncation":"auto",
		"previous_response_id":"resp_secret",
		"max_output_tokens":2048
	}`)
	req, err := decodeRequest("openai-responses", body, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	shape := requestShapeFromIR(nil, req, "openai-responses", "codex", len(body))
	if shape.InputItemCount != 2 || shape.FunctionCallOutputCount != 1 || shape.ToolChoiceMode != "auto" {
		t.Fatalf("unexpected responses counts: %#v", shape)
	}
	if !shape.ReasoningPresent || shape.ReasoningEffortBucket != "high" || !shape.IncludePresent || !shape.TruncationPresent || !shape.PreviousResponseIDPresent {
		t.Fatalf("unexpected responses fields: %#v", shape)
	}
	if shape.RequestedOutputCapField != "max_output_tokens" || shape.RequestedOutputCapBucket != "standard" {
		t.Fatalf("unexpected output cap: %#v", shape)
	}
	assertShapeRecordDoesNotContain(t, shape, "do not persist", "lookup", "secret", "resp_secret")
}

func TestRequestShapeExtractionAnthropicThinkingAndToolResult(t *testing.T) {
	body := []byte(`{
		"model":"default",
		"system":"do not persist system",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"do not persist prompt"}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"do not persist tool output"}]}
		],
		"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"secret":{"type":"string"}}}}],
		"tool_choice":{"type":"tool","name":"lookup"},
		"thinking":{"type":"enabled","budget_tokens":4096},
		"max_tokens":8192
	}`)
	req, err := decodeRequest("anthropic", body, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	shape := requestShapeFromIR(nil, req, "anthropic", "claude-code", len(body))
	if shape.MessageCount != 2 || shape.ToolResultCount != 1 || shape.ToolChoiceMode != "forced" {
		t.Fatalf("unexpected anthropic shape: %#v", shape)
	}
	if !shape.ReasoningPresent || shape.ReasoningBudgetBucket != "medium" || shape.RequestedOutputCapField != "max_tokens" {
		t.Fatalf("unexpected thinking/cap shape: %#v", shape)
	}
	assertShapeRecordDoesNotContain(t, shape, "do not persist", "lookup", "secret", "toolu_1")
}

func TestTranslationShapeEventsAreBoundedAndSafe(t *testing.T) {
	raw := map[string]any{
		"model":        "default",
		"messages":     []any{map[string]any{"role": "user", "content": "do not persist prompt"}},
		"store":        true,
		"metadata":     map[string]any{"customer": "do not persist metadata"},
		"secret_field": "do not persist field",
	}
	translated := map[string]any{
		"model":    "upstream",
		"messages": []any{map[string]any{"role": "user", "content": "do not persist prompt"}},
		"store":    false,
		"stream":   false,
	}
	events := translationFieldEvents(raw, translated, 1)
	if countFieldEvents(events, "rewritten") != 1 || countFieldEvents(events, "stripped") != 1 || countFieldEvents(events, "unsupported") != 1 {
		t.Fatalf("events=%#v", events)
	}
	var sawOther bool
	for _, event := range events {
		if event.FieldName == "secret_field" {
			t.Fatalf("unsafe field name stored: %#v", event)
		}
		if event.FieldName == "other" && event.Action == "unsupported" {
			sawOther = true
		}
		assertShapeRecordDoesNotContain(t, event, "do not persist", "secret_field")
	}
	if !sawOther {
		t.Fatalf("unsupported raw field was not preserved as safe other bucket: %#v", events)
	}
}

func assertShapeRecordDoesNotContain(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(strings.TrimSpace(string(raw)))
	for _, raw := range forbidden {
		if strings.Contains(text, strings.ToLower(raw)) {
			t.Fatalf("shape telemetry leaked %q in %#v", raw, value)
		}
	}
}
