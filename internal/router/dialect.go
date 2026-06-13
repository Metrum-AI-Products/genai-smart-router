package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func decodeRequest(dialect string, body []byte, h http.Header) (*IRRequest, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &IRRequest{Raw: raw}
	if v, _ := raw["model"].(string); v != "" {
		req.Model = v
	} else {
		req.Model = "default"
	}
	if stream, ok := raw["stream"].(bool); ok {
		req.Stream = stream
	}
	if maxTokens, ok := numberAsInt(raw["max_tokens"]); ok {
		req.MaxTokens = maxTokens
	}
	if temp, ok := numberAsFloat(raw["temperature"]); ok {
		req.Temperature = &temp
	}
	if h.Get("Cache-Control") == "no-cache" || strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-store") {
		req.NoCache = true
	}
	switch dialect {
	case "anthropic":
		req.System = contentToText(raw["system"])
		req.Messages = decodeMessages(raw["messages"])
		if tools, ok := raw["tools"].([]any); ok {
			req.Tools = anySliceToMaps(tools)
		}
		if thinking, ok := raw["thinking"].(map[string]any); ok {
			req.Thinking = thinking
		}
	case "openai-chat":
		msgs := decodeMessages(raw["messages"])
		for _, msg := range msgs {
			if msg.Role == "system" || msg.Role == "developer" {
				if req.System != "" {
					req.System += "\n"
				}
				req.System += msg.Content
			} else {
				req.Messages = append(req.Messages, msg)
			}
		}
		if tools, ok := raw["tools"].([]any); ok {
			req.Tools = anySliceToMaps(tools)
		}
	case "openai-responses":
		req.Input = contentToText(raw["input"])
		if req.Input != "" {
			req.Messages = []IRMessage{{Role: "user", Content: req.Input}}
		}
		if inst, _ := raw["instructions"].(string); inst != "" {
			req.System = inst
		}
		if tools, ok := raw["tools"].([]any); ok {
			req.Tools = anySliceToMaps(tools)
		}
		if reasoning, ok := raw["reasoning"].(map[string]any); ok {
			req.Thinking = reasoning
		}
	default:
		return nil, fmt.Errorf("unknown dialect %s", dialect)
	}
	return req, nil
}

func encodeUpstream(dialect, model string, req *IRRequest) ([]byte, error) {
	switch dialect {
	case "anthropic":
		msgs := []map[string]any{}
		for _, m := range req.Messages {
			if m.Role == "system" || m.Role == "developer" {
				continue
			}
			msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
		}
		body := map[string]any{"model": model, "messages": msgs, "max_tokens": max(req.MaxTokens, 1024), "stream": false}
		if req.System != "" {
			body["system"] = req.System
		}
		if req.Temperature != nil {
			body["temperature"] = *req.Temperature
		}
		return json.Marshal(body)
	case "openai-responses":
		body := map[string]any{"model": model, "input": requestText(req), "stream": false}
		if req.System != "" {
			body["instructions"] = req.System
		}
		if req.MaxTokens > 0 {
			body["max_output_tokens"] = req.MaxTokens
		}
		if req.Temperature != nil {
			body["temperature"] = *req.Temperature
		}
		return json.Marshal(body)
	case "replicate":
		prompt := requestText(req)
		if req.System != "" {
			prompt = req.System + "\n\n" + prompt
		}
		body := map[string]any{
			"input": map[string]any{
				"prompt": prompt,
			},
			"stream": false,
		}
		if req.MaxTokens > 0 {
			body["input"].(map[string]any)["max_tokens"] = req.MaxTokens
		}
		if req.Temperature != nil {
			body["input"].(map[string]any)["temperature"] = *req.Temperature
		}
		return json.Marshal(body)
	default:
		msgs := []map[string]any{}
		if req.System != "" {
			msgs = append(msgs, map[string]any{"role": "system", "content": req.System})
		}
		for _, m := range req.Messages {
			msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
		}
		if len(msgs) == 0 && req.Input != "" {
			msgs = append(msgs, map[string]any{"role": "user", "content": req.Input})
		}
		body := map[string]any{"model": model, "messages": msgs, "stream": false}
		if req.MaxTokens > 0 {
			body["max_tokens"] = req.MaxTokens
		}
		if req.Temperature != nil {
			body["temperature"] = *req.Temperature
		}
		return json.Marshal(body)
	}
}

func decodeUpstreamResponse(dialect string, raw []byte, model string) (*IRResponse, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	resp := &IRResponse{ID: stringValue(m["id"]), Model: model, Raw: m}
	if resp.ID == "" {
		resp.ID = "resp_" + time.Now().UTC().Format("20060102150405")
	}
	switch dialect {
	case "anthropic":
		if parts, ok := m["content"].([]any); ok {
			resp.Text = textFromContentParts(parts)
		}
		resp.StopReason = stringValue(m["stop_reason"])
		resp.Usage = usageFromMap(m["usage"])
	case "openai-responses":
		resp.Text = stringValue(m["output_text"])
		if resp.Text == "" {
			resp.Text = textFromResponsesOutput(m["output"])
		}
		resp.Usage = usageFromMap(m["usage"])
	case "replicate":
		status := stringValue(m["status"])
		if status != "" && status != "succeeded" {
			return nil, fmt.Errorf("replicate prediction status %s", status)
		}
		resp.Text = replicateOutputText(m["output"])
		resp.StopReason = "stop"
		resp.Usage = Usage{InputTokens: 0, OutputTokens: 0, TotalTokens: 0}
	default:
		if choices, ok := m["choices"].([]any); ok && len(choices) > 0 {
			if ch, ok := choices[0].(map[string]any); ok {
				if msg, ok := ch["message"].(map[string]any); ok {
					resp.Text = contentToText(msg["content"])
				}
				resp.StopReason = stringValue(ch["finish_reason"])
			}
		}
		resp.Usage = usageFromMap(m["usage"])
	}
	if resp.Usage.TotalTokens == 0 {
		resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}
	return resp, nil
}

func replicateOutputText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := []string{}
		for _, item := range x {
			parts = append(parts, contentToText(item))
		}
		return strings.Join(parts, "")
	case map[string]any:
		if txt := stringValue(x["text"]); txt != "" {
			return txt
		}
		raw, _ := json.Marshal(x)
		return string(raw)
	default:
		return contentToText(v)
	}
}

func encodeAnthropicResponse(resp *IRResponse) map[string]any {
	return map[string]any{
		"id":            resp.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         resp.Model,
		"content":       []map[string]any{{"type": "text", "text": resp.Text}},
		"stop_reason":   defaultString(resp.StopReason, "end_turn"),
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}
}

func encodeChatResponse(resp *IRResponse) map[string]any {
	return map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   resp.Model,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": defaultString(resp.StopReason, "stop"),
			"message":       map[string]any{"role": "assistant", "content": resp.Text},
		}},
		"usage": map[string]any{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		},
	}
}

func encodeResponsesResponse(resp *IRResponse) map[string]any {
	return map[string]any{
		"id":          resp.ID,
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"status":      "completed",
		"model":       resp.Model,
		"output_text": resp.Text,
		"output": []map[string]any{{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{{
				"type": "output_text",
				"text": resp.Text,
			}},
		}},
		"usage": map[string]any{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		},
	}
}

func decodeMessages(v any) []IRMessage {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []IRMessage{}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, IRMessage{Role: defaultString(stringValue(m["role"]), "user"), Content: contentToText(m["content"])})
	}
	return out
}

func contentToText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := []string{}
		for _, p := range x {
			if pm, ok := p.(map[string]any); ok {
				if txt := stringValue(pm["text"]); txt != "" {
					parts = append(parts, txt)
				}
				if txt := contentToText(pm["content"]); txt != "" {
					parts = append(parts, txt)
				}
				if txt := contentToText(pm["input"]); txt != "" {
					parts = append(parts, txt)
				}
				continue
			}
			if txt := contentToText(p); txt != "" {
				parts = append(parts, txt)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if txt := stringValue(x["text"]); txt != "" {
			return txt
		}
		if txt := contentToText(x["content"]); txt != "" {
			return txt
		}
		if txt := contentToText(x["input"]); txt != "" {
			return txt
		}
		if txt := stringValue(x["output_text"]); txt != "" {
			return txt
		}
		if args := stringValue(x["arguments"]); args != "" {
			return args
		}
		if typ := stringValue(x["type"]); typ == "input_text" || typ == "output_text" {
			if txt := stringValue(x["text"]); txt != "" {
				return txt
			}
		}
		return ""
	default:
		return ""
	}
}

func requestText(req *IRRequest) string {
	if req.Input != "" {
		return req.Input
	}
	parts := []string{}
	for _, m := range req.Messages {
		parts = append(parts, m.Role+": "+m.Content)
	}
	return strings.Join(parts, "\n")
}

func textFromContentParts(parts []any) string {
	out := []string{}
	for _, part := range parts {
		if m, ok := part.(map[string]any); ok {
			if stringValue(m["type"]) == "text" || m["text"] != nil {
				out = append(out, stringValue(m["text"]))
			}
		}
	}
	return strings.Join(out, "")
}

func textFromResponsesOutput(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	out := []string{}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if content, ok := m["content"].([]any); ok {
			out = append(out, textFromContentParts(content))
		}
	}
	return strings.Join(out, "")
}

func usageFromMap(v any) Usage {
	m, ok := v.(map[string]any)
	if !ok {
		return Usage{}
	}
	in, _ := numberAsInt(m["input_tokens"])
	if in == 0 {
		in, _ = numberAsInt(m["prompt_tokens"])
	}
	out, _ := numberAsInt(m["output_tokens"])
	if out == 0 {
		out, _ = numberAsInt(m["completion_tokens"])
	}
	total, _ := numberAsInt(m["total_tokens"])
	if total == 0 {
		total = in + out
	}
	return Usage{InputTokens: in, OutputTokens: out, TotalTokens: total}
}

func anySliceToMaps(in []any) []map[string]any {
	out := []map[string]any{}
	for _, v := range in {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func numberAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func numberAsFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
