package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type productionDerivedLargePayloadFixture struct {
	Name                       string `json:"name"`
	ClientName                 string `json:"client_name"`
	ModelGroup                 string `json:"model_group"`
	Stream                     bool   `json:"stream"`
	MessageCount               int    `json:"message_count"`
	ToolCount                  int    `json:"tool_count"`
	ImageCount                 int    `json:"image_count"`
	OutputCapPresent           bool   `json:"output_cap_present"`
	EstimatedInputTokensBucket string `json:"estimated_input_tokens_bucket"`
	RequestBytesBucket         string `json:"request_bytes_bucket"`
	ToolSchemaBytesBucket      string `json:"tool_schema_bytes_bucket"`
	MustNotSelect              []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Dialect  string `json:"dialect"`
	} `json:"must_not_select"`
	AllowedSelectedTargets []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Dialect  string `json:"dialect"`
	} `json:"allowed_selected_targets"`
}

type productionDerivedErrorFixture struct {
	Name      string `json:"name"`
	Scenarios []struct {
		Name                 string   `json:"name"`
		UpstreamStatus       int      `json:"upstream_status"`
		ExpectedCallerStatus int      `json:"expected_caller_status"`
		ExpectedErrorType    string   `json:"expected_error_type"`
		ExpectedErrorClass   string   `json:"expected_error_class"`
		ExpectedRetryable    bool     `json:"expected_retryable"`
		RequiredSafeFields   []string `json:"required_safe_fields"`
	} `json:"scenarios"`
}

type productionDerivedAgentCompatibilityFixture struct {
	Name                 string `json:"name"`
	SourceIncidentIssue  string `json:"source_incident_issue"`
	ModelGroup           string `json:"model_group"`
	ProductionAccessNote string `json:"production_access_note"`
	Scenarios            []struct {
		Name                        string   `json:"name"`
		SourceIncidentIssue         string   `json:"source_incident_issue"`
		ClientName                  string   `json:"client_name"`
		Surface                     string   `json:"surface"`
		ModelGroup                  string   `json:"model_group"`
		Stream                      bool     `json:"stream"`
		MessageCount                int      `json:"message_count"`
		ToolCount                   int      `json:"tool_count"`
		ImageCount                  int      `json:"image_count"`
		ReasoningControl            string   `json:"reasoning_control"`
		OutputCapPresent            bool     `json:"output_cap_present"`
		EstimatedInputTokensBucket  string   `json:"estimated_input_tokens_bucket"`
		RequestBytesBucket          string   `json:"request_bytes_bucket"`
		ToolSchemaBytesBucket       string   `json:"tool_schema_bytes_bucket"`
		RequiredCapabilities        []string `json:"required_capabilities"`
		ExpectedBridgeDirection     string   `json:"expected_bridge_direction"`
		ExpectedTranslatedReasoning string   `json:"expected_translated_reasoning_control"`
		ExpectedErrorClass          string   `json:"expected_error_class"`
		ExpectedRejectionReason     string   `json:"expected_rejection_reason"`
		ProductionSafePayload       string   `json:"production_smoke_safe_payload_template"`
		PreviousResponseIDPresent   bool     `json:"previous_response_id_present"`
		DetectedPayloadDialect      string   `json:"detected_payload_dialect"`
		StructuredOutput            bool     `json:"structured_output"`
	} `json:"scenarios"`
}

func TestProductionDerivedAgentCompatibilityFixtureCoversRequiredScenarios(t *testing.T) {
	fixture := loadProductionDerivedAgentCompatibilityFixture(t, "agent-reasoning-bridge-compatibility.json")
	if fixture.ModelGroup != "reasoning-bridge-smoke" {
		t.Fatalf("fixture model_group=%q, want reasoning-bridge-smoke", fixture.ModelGroup)
	}
	if !strings.Contains(fixture.ProductionAccessNote, "Harbor/Chetan") || strings.Contains(fixture.ProductionAccessNote, "change production big-coder") {
		t.Fatalf("production access note must mention Harbor/Chetan access without directing big-coder edits: %q", fixture.ProductionAccessNote)
	}
	requiredIssues := []string{"#319", "#320", "#321", "#330", "#333", "#335", "#344", "#350", "#352", "#357", "#358", "#359"}
	requiredClients := []string{"Codex CLI", "Cursor/1.0", "Claude Code", "opencode", "aider", "Generic SDK"}
	requiredScenarios := []string{
		"codex-responses-reasoning-tools",
		"cursor-chat-reasoning-tools-chat-to-responses",
		"claude-code-messages-thinking-tools",
		"responses-to-chat-function-tool",
		"cursor-mixed-responses-body-chat-endpoint",
		"previous-response-id-stateless-bridge-negative",
		"chat-to-responses-streaming-negative",
		"reasoning-no-compatible-target-negative",
		"reasoning-models-metadata-advertisement",
		"anthropic-thinking-budget-output-cap-negative",
		"cursor-image-tools-no-eligible",
		"no-eligible-target-diagnostics-proof",
		"upstream-entitlement-401-fallback-proof",
		"provider-skin-mismatch-negative",
		"opencode-chat-large-tool-schema",
		"aider-chat-output-cap-structured-output",
	}
	issues := map[string]bool{}
	clients := map[string]bool{}
	scenarios := map[string]bool{}
	surfaces := map[string]bool{}
	for _, scenario := range fixture.Scenarios {
		if scenario.Name == "" || scenario.SourceIncidentIssue == "" || scenario.ClientName == "" || scenario.Surface == "" || scenario.ModelGroup == "" || scenario.ProductionSafePayload == "" {
			t.Fatalf("scenario missing required safe metadata: %#v", scenario)
		}
		if scenario.ModelGroup == "big-coder" {
			t.Fatalf("agent compatibility fixture must use a dedicated smoke group, got big-coder in %#v", scenario)
		}
		if scenario.RequestBytesBucket == "" || scenario.ToolSchemaBytesBucket == "" || scenario.EstimatedInputTokensBucket == "" {
			t.Fatalf("scenario missing shape buckets: %#v", scenario)
		}
		issues[scenario.SourceIncidentIssue] = true
		clients[scenario.ClientName] = true
		scenarios[scenario.Name] = true
		surfaces[scenario.Surface] = true
	}
	for _, issue := range requiredIssues {
		if !issues[issue] {
			t.Fatalf("fixture missing source issue %s", issue)
		}
	}
	for _, client := range requiredClients {
		if !clients[client] {
			t.Fatalf("fixture missing client %s", client)
		}
	}
	for _, name := range requiredScenarios {
		if !scenarios[name] {
			t.Fatalf("fixture missing scenario %s", name)
		}
	}
	for _, surface := range []string{"openai_chat", "openai_responses", "anthropic_messages"} {
		if !surfaces[surface] {
			t.Fatalf("fixture missing surface %s", surface)
		}
	}
}

func TestProductionDerivedAgentCompatibilityNegativeBridgeReasonsMatchRouter(t *testing.T) {
	fixture := loadProductionDerivedAgentCompatibilityFixture(t, "agent-reasoning-bridge-compatibility.json")
	byName := map[string]string{}
	for _, scenario := range fixture.Scenarios {
		byName[scenario.Name] = scenario.ExpectedRejectionReason
	}

	req, err := decodeRequest("openai-responses", []byte(`{"model":"reasoning-bridge-smoke","input":"hi","previous_response_id":"resp_fixture"}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	responsesTarget := Target{ResponsesToChat: ResponsesToChatBridge{Enabled: true, Text: true}}
	if got := responsesToChatBridgeFilterReason(responsesTarget, req, "openai-responses", "openai-chat"); got != byName["previous-response-id-stateless-bridge-negative"] {
		t.Fatalf("previous_response_id filter=%q, want fixture reason %q", got, byName["previous-response-id-stateless-bridge-negative"])
	}

	req, err = decodeRequest("openai-chat", []byte(`{"model":"reasoning-bridge-smoke","stream":true,"messages":[{"role":"user","content":"hi"}]}`), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	chatTarget := Target{Bridges: BridgeSupport{ChatToResponses: DialectBridgeSupport{Enabled: true}}}
	if got := chatToResponsesBridgeFilterReason(chatTarget, req, "openai-chat", "openai-responses"); got != byName["chat-to-responses-streaming-negative"] {
		t.Fatalf("streaming bridge filter=%q, want fixture reason %q", got, byName["chat-to-responses-streaming-negative"])
	}
}

func TestProductionDerivedLargeOpenAIChatToolPayloadSkipsShapeLimitedTarget(t *testing.T) {
	fixture := loadProductionDerivedLargePayloadFixture(t, "large-openai-chat-tools.json")
	body := syntheticProductionDerivedOpenAIChatPayload(t, fixture)

	var selectedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		selectedModel = stringValue(decoded["model"])
		if selectedModel == fixture.MustNotSelect[0].Model {
			t.Fatalf("unsafe production-derived target was selected: %s", selectedModel)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl_production_derived",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1200, "completion_tokens": 2, "total_tokens": 1202},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := productionDerivedRegressionConfig(t, dir, upstream.URL)
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", fixture.ClientName)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if selectedModel != fixture.AllowedSelectedTargets[0].Model {
		t.Fatalf("selected upstream model=%q, want %q", selectedModel, fixture.AllowedSelectedTargets[0].Model)
	}

	var usage usageRecord
	if err := svc.usage.db.First(&usage).Error; err != nil {
		t.Fatal(err)
	}
	var shape requestShapeRecord
	if err := svc.usage.db.Where("request_id = ?", usage.RequestID).First(&shape).Error; err != nil {
		t.Fatal(err)
	}
	if !shape.Stream || shape.MessageCount != fixture.MessageCount || shape.ToolCount != fixture.ToolCount || shape.ImageCount != fixture.ImageCount {
		t.Fatalf("unexpected request shape counts: %#v", shape)
	}
	if shape.TotalRequestBytesBucket != fixture.RequestBytesBucket || shape.ToolSchemaBytesBucket != fixture.ToolSchemaBytesBucket || shape.EstimatedInputTokensBucket != fixture.EstimatedInputTokensBucket || shape.RequestedOutputCapBucket != "omitted" {
		t.Fatalf("unexpected request shape buckets: %#v", shape)
	}
	var candidates []decisionTargetCandidateRecord
	if err := svc.usage.db.Where("request_id = ?", usage.RequestID).Order("candidate_index ASC").Find(&candidates).Error; err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count=%d", len(candidates))
	}
	if candidates[0].Provider != fixture.MustNotSelect[0].Provider || candidates[0].Model != fixture.MustNotSelect[0].Model || candidates[0].EligibilityReason != "request-shape-max-request-bytes" || candidates[0].Selected {
		t.Fatalf("unsafe target candidate not skipped by request shape: %#v", candidates[0])
	}
	if candidates[1].Provider != fixture.AllowedSelectedTargets[0].Provider || candidates[1].Model != fixture.AllowedSelectedTargets[0].Model || !candidates[1].Selected {
		t.Fatalf("safe target was not selected: %#v", candidates[1])
	}
	var reasons []decisionTargetFilterReasonRecord
	if err := svc.usage.db.Where("request_id = ?", usage.RequestID).Find(&reasons).Error; err != nil {
		t.Fatal(err)
	}
	if !filterReasonsContain(reasons, "request_shape", "request-shape-max-request-bytes") {
		t.Fatalf("missing request-shape filter reason: %#v", reasons)
	}
	var translation requestTranslationShapeRecord
	if err := svc.usage.db.Where("request_id = ?", usage.RequestID).First(&translation).Error; err != nil {
		t.Fatal(err)
	}
	if translation.Provider != fixture.AllowedSelectedTargets[0].Provider || translation.Model != fixture.AllowedSelectedTargets[0].Model || translation.TranslatedToolCount != fixture.ToolCount || translation.TranslatedRequestBytesBucket != fixture.RequestBytesBucket {
		t.Fatalf("unexpected translation shape: %#v", translation)
	}
	assertProductionDerivedArtifactsDoNotContain(t, svc, dir, "production-derived-secret", "provider-key", testToken)
}

func TestProductionDerivedUpstreamErrorClassificationIsCallerVisible(t *testing.T) {
	fixture := loadProductionDerivedErrorFixture(t, "upstream-error-classification.json")
	for _, scenario := range fixture.Scenarios {
		if scenario.UpstreamStatus == 0 {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(scenario.UpstreamStatus)
				_, _ = w.Write([]byte(`{"error":{"message":"unsupported request shape","type":"invalid_request_error","code":"unsupported_value","param":"tools"}}`))
			}))
			defer upstream.Close()

			dir := t.TempDir()
			cfg := testConfig(t, upstream.URL, "provider-key", dir)
			cfg.Server.UsageDB = UsageDBConfig{Driver: "sqlite", Path: filepath.Join(dir, "usage.sqlite")}
			cfg.Server.Diagnostics.StoreSanitizedUpstreamError = true
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hello"}]}`))
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("User-Agent", "Cursor/1.0")
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != scenario.ExpectedCallerStatus {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("X-Router-Error-Class") != scenario.ExpectedErrorClass || rr.Header().Get("X-Upstream-Status") == "" {
				t.Fatalf("missing upstream diagnostic headers: %#v", rr.Header())
			}
			details := decodeProductionDerivedErrorDetails(t, rr.Body.Bytes(), scenario.ExpectedErrorType)
			if details["error_class"] != scenario.ExpectedErrorClass || int(details["upstream_status"].(float64)) != scenario.UpstreamStatus || details["retryable"] != scenario.ExpectedRetryable {
				t.Fatalf("unexpected error details: %#v", details)
			}
			for _, field := range scenario.RequiredSafeFields {
				if _, ok := details[field]; !ok {
					t.Fatalf("safe field %q missing from details %#v", field, details)
				}
			}
			if strings.Contains(rr.Body.String(), "provider-key") || strings.Contains(rr.Body.String(), testToken) {
				t.Fatalf("caller error leaked secret material: %s", rr.Body.String())
			}

			var usage usageRecord
			if err := svc.usage.db.First(&usage).Error; err != nil {
				t.Fatal(err)
			}
			if usage.Error != scenario.ExpectedErrorType || usage.Status != scenario.ExpectedCallerStatus || usage.QuotaState != "ok" || usage.KeyState != "active" {
				t.Fatalf("unexpected usage error state: %#v", usage)
			}
			var requestErr requestErrorRecord
			if err := svc.usage.db.Where("request_id = ?", usage.RequestID).First(&requestErr).Error; err != nil {
				t.Fatal(err)
			}
			if requestErr.ErrorClass != scenario.ExpectedErrorClass || requestErr.ErrorType != scenario.ExpectedErrorType || requestErr.Status != scenario.ExpectedCallerStatus || requestErr.Retryable != scenario.ExpectedRetryable {
				t.Fatalf("unexpected request error: %#v", requestErr)
			}
			var upstreamDetails []requestUpstreamErrorDetailRecord
			if err := svc.usage.db.Where("request_id = ?", usage.RequestID).Find(&upstreamDetails).Error; err != nil {
				t.Fatal(err)
			}
			if !upstreamDetailsContain(upstreamDetails, scenario.UpstreamStatus, scenario.ExpectedErrorClass, "type", "invalid_request_error") ||
				!upstreamDetailsContain(upstreamDetails, scenario.UpstreamStatus, scenario.ExpectedErrorClass, "code", "unsupported_value") ||
				!upstreamDetailsContain(upstreamDetails, scenario.UpstreamStatus, scenario.ExpectedErrorClass, "param", "tools") {
				t.Fatalf("unexpected sanitized upstream error details: %#v", upstreamDetails)
			}
		})
	}
}

func TestProductionDerivedRouterTrafficShapeRejectionSkipsUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeChatTestResponse(w, "ok")
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.UsageDB = UsageDBConfig{Driver: "sqlite", Path: filepath.Join(dir, "usage.sqlite")}
	on := true
	cfg.Callers[0].TrafficShape = TrafficShapeConfig{Enabled: &on, RequestStartPerSec: 0.01, RequestBurst: 1}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	first := performChatRequest(t, svc, `{"model":"default","messages":[{"role":"user","content":"first"}]}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	second := performChatRequest(t, svc, `{"model":"default","messages":[{"role":"user","content":"second"}]}`)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want only admitted request", calls.Load())
	}
	assertTrafficShapeError(t, second.Body.Bytes(), "caller.request_start_per_sec")

	var usage usageRecord
	if err := svc.usage.db.Where("status = ?", http.StatusTooManyRequests).First(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if usage.Error != "traffic-shaped" || usage.Attempts != 0 || usage.TrafficShapeDecision != "rejected" {
		t.Fatalf("unexpected router-side traffic shape telemetry: %#v", usage)
	}
	var requestErr requestErrorRecord
	if err := svc.usage.db.Where("request_id = ?", usage.RequestID).First(&requestErr).Error; err != nil {
		t.Fatal(err)
	}
	if requestErr.ErrorClass != "traffic-shaped" || requestErr.Attempts != 0 {
		t.Fatalf("unexpected router-side request error telemetry: %#v", requestErr)
	}
}

func productionDerivedRegressionConfig(t *testing.T, dir, upstreamURL string) *Config {
	t.Helper()
	sum := sha256.Sum256([]byte(testToken))
	return &Config{
		Server: ServerConfig{
			Listen:            ":0",
			DefaultModelGroup: "large-openai-chat-tools-smoke",
			UsageDB:           UsageDBConfig{Driver: "sqlite", Path: filepath.Join(dir, "usage.sqlite")},
			DecisionTelemetry: DecisionTelemetryConfig{Enabled: true},
			Logging:           LoggingConfig{Path: filepath.Join(dir, "requests.jsonl")},
		},
		StatePath: filepath.Join(dir, "state.json"),
		Provider: map[string]ProviderConfig{
			"limited-large-payload":   {BaseURL: upstreamURL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"},
			"validated-large-payload": {BaseURL: upstreamURL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"},
		},
		Models: map[string]ModelGroup{
			"large-openai-chat-tools-smoke": {
				Strategy: "static",
				Targets: []Target{
					{
						Provider:            "limited-large-payload",
						Model:               "limited-tool-model",
						ContextTokens:       200000,
						ToolSupport:         ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
						RequestShapeSupport: RequestShapeSupport{MaxRequestBytes: 196608},
					},
					{
						Provider:      "validated-large-payload",
						Model:         "validated-tool-model",
						ContextTokens: 200000,
						ToolSupport:   ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
					},
				},
			},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			User:        "alice",
			Project:     "metrum-insights",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(sum[:]),
			TokenID:     "rtr_alice_test",
			Allow:       []string{"large-openai-chat-tools-smoke"},
			Rate:        RateConfig{RPM: 100, TPM: 1000000, Concurrent: 4},
			Quota:       QuotaConfig{Day: BudgetConfig{Requests: 100, Tokens: 1000000}, Month: BudgetConfig{Tokens: 1000000}},
			Key:         KeyConfig{LifetimeTokens: 1000000},
		}},
	}
}

func syntheticProductionDerivedOpenAIChatPayload(t *testing.T, fixture productionDerivedLargePayloadFixture) string {
	t.Helper()
	messages := make([]map[string]any, 0, fixture.MessageCount)
	messages = append(messages, map[string]any{"role": "system", "content": "Synthetic production-derived large coding-agent fixture. Do not persist raw content."})
	filler := strings.Repeat("production-derived-safe-filler ", 62)
	for i := 1; i < fixture.MessageCount; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		messages = append(messages, map[string]any{"role": role, "content": filler + "turn " + string(rune('a'+(i%26)))})
	}
	tools := make([]map[string]any, 0, fixture.ToolCount)
	description := strings.Repeat("safe schema description ", 42)
	for i := 0; i < fixture.ToolCount; i++ {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "fixture_tool_" + string(rune('a'+i)),
				"description": description,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": strings.Repeat("safe path field ", 16)},
						"content": map[string]any{"type": "string", "description": strings.Repeat("safe content field ", 16)},
					},
					"required": []string{"path"},
				},
			},
		})
	}
	payload := map[string]any{
		"model":       fixture.ModelGroup,
		"stream":      fixture.Stream,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": "auto",
		"metadata":    map[string]any{"fixture": fixture.Name},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := byteBucket(len(raw)); got != fixture.RequestBytesBucket {
		t.Fatalf("fixture request bytes=%d bucket=%s want %s", len(raw), got, fixture.RequestBytesBucket)
	}
	if got := byteBucket(jsonValueLen(tools)); got != fixture.ToolSchemaBytesBucket {
		t.Fatalf("fixture tool schema bytes=%d bucket=%s want %s", jsonValueLen(tools), got, fixture.ToolSchemaBytesBucket)
	}
	return string(raw)
}

func loadProductionDerivedLargePayloadFixture(t *testing.T, name string) productionDerivedLargePayloadFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "smokes", "production-derived", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture productionDerivedLargePayloadFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func loadProductionDerivedErrorFixture(t *testing.T, name string) productionDerivedErrorFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "smokes", "production-derived", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture productionDerivedErrorFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func loadProductionDerivedAgentCompatibilityFixture(t *testing.T, name string) productionDerivedAgentCompatibilityFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "smokes", "production-derived", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture productionDerivedAgentCompatibilityFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func decodeProductionDerivedErrorDetails(t *testing.T, raw []byte, wantType string) map[string]any {
	t.Helper()
	var payload struct {
		Error struct {
			Type    string         `json:"type"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Type != wantType {
		t.Fatalf("error type=%q want %q body=%s", payload.Error.Type, wantType, raw)
	}
	return payload.Error.Details
}

func upstreamDetailsContain(details []requestUpstreamErrorDetailRecord, status int, class, field, value string) bool {
	for _, detail := range details {
		if detail.StatusCode == status && detail.ErrorClass == class && detail.FieldName == field && detail.FieldValue == value {
			return true
		}
	}
	return false
}

func assertProductionDerivedArtifactsDoNotContain(t *testing.T, svc *Service, dir string, forbidden ...string) {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(raw), value) {
			t.Fatalf("log artifacts contain forbidden value %q", value)
		}
	}
	var estimates []requestTokenEstimateRecord
	if err := svc.usage.db.Find(&estimates).Error; err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(estimates)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(serialized), value) {
			t.Fatalf("usage artifacts contain forbidden value %q", value)
		}
	}
}
