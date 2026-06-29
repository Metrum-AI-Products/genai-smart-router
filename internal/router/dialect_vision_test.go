package router

import (
	"encoding/json"
	"net/http"
	"testing"
)

const receiptImageURL = "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"

func TestOpenAIChatImageTranslatesToResponsesAndAnthropic(t *testing.T) {
	req, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Read the receipt."},
				{"type": "image_url", "image_url": {"url": "`+receiptImageURL+`", "detail": "high"}}
			]
		}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !requestHasImages(req) {
		t.Fatalf("expected image request: %#v", req)
	}

	responsesRaw, err := encodeUpstream("openai-responses", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var responses map[string]any
	if err := json.Unmarshal(responsesRaw, &responses); err != nil {
		t.Fatal(err)
	}
	msgs := responses["input"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	if got := content[1].(map[string]any)["image_url"]; got != receiptImageURL {
		t.Fatalf("responses image_url=%v", got)
	}

	anthropicRaw, err := encodeUpstream("anthropic", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var anthropic map[string]any
	if err := json.Unmarshal(anthropicRaw, &anthropic); err != nil {
		t.Fatal(err)
	}
	anthropicMsgs := anthropic["messages"].([]any)
	anthropicContent := anthropicMsgs[0].(map[string]any)["content"].([]any)
	source := anthropicContent[1].(map[string]any)["source"].(map[string]any)
	if source["type"] != "url" || source["url"] != receiptImageURL {
		t.Fatalf("anthropic image source=%#v", source)
	}
}

func TestAnthropicMaxTokensHonorsCallerValue(t *testing.T) {
	req, err := decodeRequest("anthropic", []byte(`{
		"model": "vision",
		"max_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeUpstream("anthropic", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["max_tokens"]; got != float64(1) {
		t.Fatalf("max_tokens=%#v, want 1", got)
	}
}

func TestAnthropicMaxTokensDefaultsOnlyWhenOmitted(t *testing.T) {
	req, err := decodeRequest("anthropic", []byte(`{
		"model": "vision",
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeUpstream("anthropic", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["max_tokens"]; got != float64(1024) {
		t.Fatalf("max_tokens=%#v, want default 1024", got)
	}
}

func TestOpenAIChatMaxCompletionTokensDecodesCanonicalMaxTokens(t *testing.T) {
	req, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"max_completion_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 1 {
		t.Fatalf("MaxTokens=%d, want 1", req.MaxTokens)
	}
	if req.MaxTokensField != "max_completion_tokens" {
		t.Fatalf("MaxTokensField=%q, want max_completion_tokens", req.MaxTokensField)
	}
}

func TestOpenAIChatMaxCompletionTokensForwardsOutputCap(t *testing.T) {
	req, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"max_completion_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeUpstream("openai-chat", "chat-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["max_completion_tokens"]; got != float64(1) {
		t.Fatalf("max_completion_tokens=%#v, want 1; body=%#v", got, body)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("max_tokens also present in body=%#v", body)
	}
}

func TestOpenAIChatMaxTokensTakesPrecedenceOverMaxCompletionTokens(t *testing.T) {
	req, err := decodeRequest("openai-chat", []byte(`{
		"model": "default",
		"max_tokens": 2,
		"max_completion_tokens": 1,
		"messages": [{"role": "user", "content": "write a long essay"}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 2 {
		t.Fatalf("MaxTokens=%d, want 2", req.MaxTokens)
	}
	if req.MaxTokensField != "max_tokens" {
		t.Fatalf("MaxTokensField=%q, want max_tokens", req.MaxTokensField)
	}
	raw, err := encodeUpstream("openai-chat", "chat-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["max_tokens"]; got != float64(2) {
		t.Fatalf("max_tokens=%#v, want 2; body=%#v", got, body)
	}
	if _, ok := body["max_completion_tokens"]; ok {
		t.Fatalf("max_completion_tokens should not be emitted when max_tokens wins; body=%#v", body)
	}
}

func TestResponsesMaxOutputTokensTranslatesToChatMaxTokens(t *testing.T) {
	req, err := decodeRequest("openai-responses", []byte(`{
		"model": "vision",
		"max_output_tokens": 1,
		"input": "write a long essay"
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeUpstream("openai-chat", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["max_tokens"]; got != float64(1) {
		t.Fatalf("max_tokens=%#v, want 1", got)
	}
}

func TestResponsesImageMessageArrayTranslatesToChatAndAnthropicBase64(t *testing.T) {
	dataURL := "data:image/png;base64,aGVsbG8="
	req, err := decodeRequest("openai-responses", []byte(`{
		"model": "default",
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_text", "text": "Read the receipt."},
				{"type": "input_image", "image_url": "`+dataURL+`"}
			]
		}]
	}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !requestHasImages(req) || len(req.Messages) != 1 || !hasImageParts(req.Messages[0].Parts) {
		t.Fatalf("responses image input not decoded: %#v", req)
	}

	chatRaw, err := encodeUpstream("openai-chat", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(chatRaw, &chat); err != nil {
		t.Fatal(err)
	}
	msgs := chat["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	img := content[1].(map[string]any)["image_url"].(map[string]any)
	if img["url"] != dataURL {
		t.Fatalf("chat image url=%#v", img)
	}

	anthropicRaw, err := encodeUpstream("anthropic", "vision-model", req)
	if err != nil {
		t.Fatal(err)
	}
	var anthropic map[string]any
	if err := json.Unmarshal(anthropicRaw, &anthropic); err != nil {
		t.Fatal(err)
	}
	anthropicMsgs := anthropic["messages"].([]any)
	anthropicContent := anthropicMsgs[0].(map[string]any)["content"].([]any)
	source := anthropicContent[1].(map[string]any)["source"].(map[string]any)
	if source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != "aGVsbG8=" {
		t.Fatalf("anthropic base64 source=%#v", source)
	}
}

func TestEncodeChatPassthroughStoreAndMaxTokensByProvider(t *testing.T) {
	req := &IRRequest{
		MaxTokens:      16,
		MaxTokensField: "max_tokens",
		Messages:       []IRMessage{{Role: "user", Content: "hi"}},
	}
	crusoeBody, err := encodeChatPassthrough("zai/GLM-5.2", req, Target{Provider: "crusoe"})
	if err != nil {
		t.Fatal(err)
	}
	var crusoe map[string]any
	if err := json.Unmarshal(crusoeBody, &crusoe); err != nil {
		t.Fatal(err)
	}
	if _, ok := crusoe["store"]; ok {
		t.Fatalf("crusoe body should omit store: %#v", crusoe)
	}
	if crusoe["max_tokens"] != float64(16) {
		t.Fatalf("crusoe max_tokens=%#v", crusoe["max_tokens"])
	}

	openaiBody, err := encodeChatPassthrough("gpt-5.4-nano", req, Target{Provider: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	var openai map[string]any
	if err := json.Unmarshal(openaiBody, &openai); err != nil {
		t.Fatal(err)
	}
	if openai["store"] != false {
		t.Fatalf("openai store=%#v, want false", openai["store"])
	}
	if openai["max_completion_tokens"] != float64(16) {
		t.Fatalf("openai max_completion_tokens=%#v", openai["max_completion_tokens"])
	}
	if _, ok := openai["max_tokens"]; ok {
		t.Fatalf("openai body should translate max_tokens: %#v", openai)
	}
}
