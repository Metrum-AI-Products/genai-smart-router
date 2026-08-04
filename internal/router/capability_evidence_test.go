package router

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func capabilityTestConfig(provider ProviderConfig, target Target) *Config {
	return &Config{
		Provider: map[string]ProviderConfig{"p": provider},
		Models:   map[string]ModelGroup{"group": {Targets: []Target{target}}},
	}
}

func evidenceForSurface(surface capabilitySurface, capability string) CapabilityEvidence {
	return CapabilityEvidence{
		SchemaVersion:  "capability-smoke/v1",
		Status:         "passed",
		CapabilityCase: capability,
		Identity: CapabilityEvidenceIdentity{
			Provider:             surface.target.Provider,
			AccountIdentityClass: surface.accountIdentity,
			EndpointFingerprint:  surface.endpointFingerprint,
			EndpointPath:         surface.endpointPath,
			APISkin:              surface.apiSkin,
			Model:                surface.model,
			ModelSuffix:          surface.modelSuffix,
			InboundDialect:       surface.inbound,
			BridgeDirection:      surface.bridge,
			RequestShape:         capabilityRequestShape(capability),
			ProfileVersion:       "synthetic/" + surface.bridge + "/" + capability,
		},
	}
}

func completeSurfaceEvidence(t *testing.T, provider ProviderConfig, target Target) ([]CapabilityEvidence, []CapabilityEvidenceIdentity) {
	t.Helper()
	surfaces, err := capabilitySurfaces(provider, target)
	if err != nil {
		t.Fatal(err)
	}
	var evidence []CapabilityEvidence
	var expected []CapabilityEvidenceIdentity
	for _, surface := range surfaces {
		for _, capability := range surface.capabilities {
			row := evidenceForSurface(surface, capability)
			evidence = append(evidence, row)
			expected = append(expected, row.Identity)
		}
	}
	return evidence, expected
}

func TestAdvertisedCapabilitiesUseRuntimeToolVocabulary(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		toolSupport ToolSupport
		unsupported []string
		wantAuto    bool
		wantForced  bool
	}{
		{name: "chat", dialect: "openai-chat", toolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}, wantAuto: true, wantForced: true},
		{name: "responses", dialect: "openai-responses", toolSupport: ToolSupport{OpenAIResponses: []string{"function", "tool_choice"}}, wantAuto: true, wantForced: true},
		{name: "anthropic", dialect: "anthropic", toolSupport: ToolSupport{AnthropicMessages: []string{"client_tools", "tool_choice"}}, wantAuto: true, wantForced: true},
		{name: "auto-only", dialect: "openai-responses", toolSupport: ToolSupport{OpenAIResponses: []string{"function"}}, wantAuto: true},
		{name: "forced-explicitly-unsupported", dialect: "openai-responses", toolSupport: ToolSupport{OpenAIResponses: []string{"function", "tool_choice"}}, unsupported: []string{"forced_tool_choice"}, wantAuto: true},
		{name: "tools-explicitly-unsupported", dialect: "openai-chat", toolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}, unsupported: []string{"tools"}},
		{name: "cross-skin", dialect: "openai-chat", toolSupport: ToolSupport{OpenAIResponses: []string{"function", "tool_choice"}}},
		{name: "provider-hosted-is-not-client-tools", dialect: "openai-responses", toolSupport: ToolSupport{ProviderHosted: []string{"tools", "tool_choice"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := Target{ToolSupport: test.toolSupport, RequestShapeSupport: RequestShapeSupport{UnsupportedRequestFeatures: test.unsupported}}
			capabilities := advertisedCapabilities(target, test.dialect)
			if got := stringSliceContains(capabilities, "tools-auto"); got != test.wantAuto {
				t.Fatalf("tools-auto=%v, want %v: %v", got, test.wantAuto, capabilities)
			}
			if got := stringSliceContains(capabilities, "tools-forced"); got != test.wantForced {
				t.Fatalf("tools-forced=%v, want %v: %v", got, test.wantForced, capabilities)
			}
		})
	}
}

func TestExampleConfigToolEvidenceCasesStayPerSkin(t *testing.T) {
	raw, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		group      string
		dialect    string
		wantForced bool
	}{
		{group: "warp-agent-smoke", dialect: "openai-chat", wantForced: true},
		{group: "agent-tools-smoke", dialect: "openai-responses", wantForced: false},
		{group: "claude-tools-smoke", dialect: "anthropic", wantForced: false},
	} {
		t.Run(test.group, func(t *testing.T) {
			group := cfg.Models[test.group]
			if len(group.Targets) != 1 {
				t.Fatalf("current config group target count=%d, want 1", len(group.Targets))
			}
			target, err := cfg.resolveTarget(test.group, group.Targets[0])
			if err != nil {
				t.Fatal(err)
			}
			capabilities := advertisedCapabilities(target, test.dialect)
			if !stringSliceContains(capabilities, "tools-auto") {
				t.Fatalf("current %s metadata did not map to tools-auto: %#v", test.dialect, target.ToolSupport)
			}
			if got := stringSliceContains(capabilities, "tools-forced"); got != test.wantForced {
				t.Fatalf("current %s tools-forced=%v, want %v: %#v", test.dialect, got, test.wantForced, target.ToolSupport)
			}
			for _, otherDialect := range []string{"openai-chat", "openai-responses", "anthropic"} {
				if otherDialect != test.dialect && targetSupportsClientTools(target, otherDialect, false) {
					t.Fatalf("%s tool metadata leaked into %s: %#v", test.dialect, otherDialect, target.ToolSupport)
				}
			}
		})
	}
}

func TestVerifyCapabilityClaimsUsesResolvedTargetMetadata(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-responses", KeyID: "test", Models: map[string]ProviderModel{
		"m": {Model: "synthetic", Dialect: "openai-responses", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}},
	}}
	cfg := capabilityTestConfig(provider, Target{Provider: "p", ModelRef: "m", Weight: 1})
	resolved, err := cfg.resolveTarget("group", cfg.Models["group"].Targets[0])
	if err != nil {
		t.Fatal(err)
	}
	surfaces, err := capabilitySurfaces(provider, resolved)
	if err != nil {
		t.Fatal(err)
	}
	claim := evidenceForSurface(surfaces[0], "tools-auto")
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{claim}, []CapabilityEvidenceIdentity{claim.Identity}); len(failures) != 0 {
		t.Fatalf("passing inherited capability rejected: %v", failures)
	}
	forced := evidenceForSurface(surfaces[0], "tools-forced")
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{forced}, []CapabilityEvidenceIdentity{forced.Identity}); len(failures) != 1 {
		t.Fatalf("forced tool without metadata must fail: %v", failures)
	}
}

func TestCapabilityEvidenceUsesActualUpstreamEndpointPaths(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		dialect    string
		wantPath   string
		legacyPath string
	}{
		{name: "chat-noncanonical-base", baseURL: "https://provider.example/inference/v1", dialect: "openai-chat", wantPath: "/inference/v1/chat/completions", legacyPath: "/v1/chat/completions"},
		{name: "responses-noncanonical-base", baseURL: "https://provider.example/api/v1", dialect: "openai-responses", wantPath: "/api/v1/responses", legacyPath: "/v1/responses"},
		{name: "anthropic-root-base", baseURL: "https://provider.example", dialect: "anthropic", wantPath: "/v1/messages", legacyPath: "/anthropic/v1/messages"},
		{name: "anthropic-prefixed-base", baseURL: "https://provider.example/anthropic", dialect: "anthropic", wantPath: "/anthropic/v1/messages", legacyPath: "/v1/messages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := ProviderConfig{BaseURL: test.baseURL, Dialect: test.dialect, KeyID: "test"}
			target := Target{Provider: "p", Model: "synthetic", Dialect: test.dialect, Weight: 1}
			cfg := capabilityTestConfig(provider, target)
			evidence, expected := completeSurfaceEvidence(t, provider, target)
			if evidence[0].Identity.EndpointPath != test.wantPath {
				t.Fatalf("endpoint path=%q, want %q", evidence[0].Identity.EndpointPath, test.wantPath)
			}
			if failures := cfg.VerifyAdvertisedCapabilities("group", evidence, expected); len(failures) != 0 {
				t.Fatalf("actual endpoint path rejected: %v", failures)
			}
			wrong := append([]CapabilityEvidence(nil), evidence...)
			wrongExpected := append([]CapabilityEvidenceIdentity(nil), expected...)
			wrong[0].Identity.EndpointPath = test.legacyPath
			wrongExpected[0] = wrong[0].Identity
			if failures := cfg.VerifyAdvertisedCapabilities("group", wrong, wrongExpected); len(failures) == 0 {
				t.Fatal("legacy path for an endpoint that is never called satisfied evidence")
			}
		})
	}

	provider := ProviderConfig{BaseURL: "://invalid", Dialect: "openai-chat", KeyID: "test"}
	target := Target{Provider: "p", Model: "synthetic", Dialect: "openai-chat", Weight: 1}
	if failures := capabilityTestConfig(provider, target).VerifyAdvertisedCapabilities("group", nil, nil); len(failures) != 1 {
		t.Fatalf("invalid endpoint construction did not fail closed: %v", failures)
	}
}

func TestChatToResponsesBridgeRequiresDistinctShapeEvidence(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/api/v1", Dialect: "openai-responses", KeyID: "test"}
	bridgeText := true
	target := Target{
		Provider:        "p",
		Model:           "synthetic",
		Dialect:         "openai-responses",
		Weight:          1,
		ToolSupport:     ToolSupport{OpenAIResponses: []string{"function", "tool_choice"}},
		InputModalities: []string{"text", "image"},
		Bridges:         BridgeSupport{ChatToResponses: DialectBridgeSupport{Enabled: true, Text: &bridgeText, Tools: true, ToolChoice: true, Images: true}},
	}
	cfg := capabilityTestConfig(provider, target)
	evidence, expected := completeSurfaceEvidence(t, provider, target)
	if len(evidence) != 8 {
		t.Fatalf("direct plus bridge requirement count=%d, want 8", len(evidence))
	}
	if failures := cfg.VerifyAdvertisedCapabilities("group", evidence, expected); len(failures) != 0 {
		t.Fatalf("complete bridge evidence rejected: %v", failures)
	}

	var directOnly []CapabilityEvidence
	var directExpected []CapabilityEvidenceIdentity
	for _, row := range evidence {
		if row.Identity.BridgeDirection == "none" {
			directOnly = append(directOnly, row)
			directExpected = append(directExpected, row.Identity)
		}
	}
	if failures := cfg.VerifyAdvertisedCapabilities("group", directOnly, directExpected); len(failures) != 4 {
		t.Fatalf("direct evidence satisfied translated requirements: %v", failures)
	}

	bridgeIndex := -1
	for index, row := range evidence {
		if row.Identity.BridgeDirection == chatToResponsesBridgeDirection && row.CapabilityCase == "tools-auto" {
			bridgeIndex = index
			break
		}
	}
	if bridgeIndex < 0 {
		t.Fatal("missing bridge tools-auto fixture")
	}
	for _, mutation := range []struct {
		name   string
		change func(*CapabilityEvidenceIdentity)
	}{
		{name: "wrong-direction", change: func(identity *CapabilityEvidenceIdentity) { identity.BridgeDirection = responsesToChatBridgeDirection }},
		{name: "wrong-inbound", change: func(identity *CapabilityEvidenceIdentity) { identity.InboundDialect = "anthropic" }},
		{name: "wrong-shape", change: func(identity *CapabilityEvidenceIdentity) { identity.RequestShape = "text" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := append([]CapabilityEvidence(nil), evidence...)
			mutatedExpected := append([]CapabilityEvidenceIdentity(nil), expected...)
			mutation.change(&mutated[bridgeIndex].Identity)
			mutatedExpected[bridgeIndex] = mutated[bridgeIndex].Identity
			if failures := cfg.VerifyAdvertisedCapabilities("group", mutated, mutatedExpected); len(failures) == 0 {
				t.Fatal("wrong translated identity satisfied bridge requirement")
			}
		})
	}
	mutated := append([]CapabilityEvidence(nil), evidence...)
	mutated[bridgeIndex].Identity.ProfileVersion = "unapproved/v1"
	if failures := cfg.VerifyAdvertisedCapabilities("group", mutated, expected); len(failures) == 0 {
		t.Fatal("unapproved bridge profile satisfied bridge requirement")
	}
}

func TestBothRuntimeBridgeDirectionsAndDisabledShapes(t *testing.T) {
	chatProvider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-chat", KeyID: "test"}
	chatTarget := Target{
		Provider:        "p",
		Model:           "synthetic",
		Dialect:         "openai-chat",
		Weight:          1,
		ToolSupport:     ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
		InputModalities: []string{"text", "image"},
		ResponsesToChat: ResponsesToChatBridge{Enabled: true, Text: true, FunctionTools: true, ToolChoice: true, Images: true},
	}
	chatCfg := capabilityTestConfig(chatProvider, chatTarget)
	chatEvidence, chatExpected := completeSurfaceEvidence(t, chatProvider, chatTarget)
	if len(chatEvidence) != 8 {
		t.Fatalf("Responses-to-Chat requirement count=%d, want 8", len(chatEvidence))
	}
	if failures := chatCfg.VerifyAdvertisedCapabilities("group", chatEvidence, chatExpected); len(failures) != 0 {
		t.Fatalf("Responses-to-Chat evidence rejected: %v", failures)
	}

	responsesProvider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-responses", KeyID: "test"}
	bridgeText := true
	responsesTarget := Target{
		Provider:    "p",
		Model:       "synthetic",
		Dialect:     "openai-responses",
		Weight:      1,
		ToolSupport: ToolSupport{OpenAIResponses: []string{"function", "tool_choice"}},
		Bridges:     BridgeSupport{ChatToResponses: DialectBridgeSupport{Enabled: true, Text: &bridgeText}},
	}
	responsesCfg := capabilityTestConfig(responsesProvider, responsesTarget)
	responsesEvidence, responsesExpected := completeSurfaceEvidence(t, responsesProvider, responsesTarget)
	if len(responsesEvidence) != 4 {
		t.Fatalf("disabled bridge shape count=%d, want direct 3 plus bridge text", len(responsesEvidence))
	}
	if failures := responsesCfg.VerifyAdvertisedCapabilities("group", responsesEvidence, responsesExpected); len(failures) != 0 {
		t.Fatalf("disabled bridge shapes incorrectly required evidence: %v", failures)
	}
}

func TestVerifyCapabilityClaimsRequiresExactApprovedIdentity(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-responses", KeyID: "test"}
	target := Target{Provider: "p", Model: "synthetic", Dialect: "openai-responses", Weight: 1}
	cfg := capabilityTestConfig(provider, target)
	evidence, _ := completeSurfaceEvidence(t, provider, target)
	claim := evidence[0]
	expected := claim.Identity
	expected.AccountIdentityClass = "different-account-class"
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{claim}, []CapabilityEvidenceIdentity{expected}); len(failures) != 1 {
		t.Fatalf("unapproved identity tuple must fail closed: %v", failures)
	}
}

func TestStructuredOutputClaimsRequireDirectAndBridgeEvidence(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-responses", KeyID: "test"}
	bridgeText := true
	target := Target{
		Provider:    "p",
		Model:       "synthetic",
		Dialect:     "openai-responses",
		Weight:      1,
		ToolSupport: ToolSupport{OpenAIResponses: []string{"structured_outputs"}},
		Bridges:     BridgeSupport{ChatToResponses: DialectBridgeSupport{Enabled: true, Text: &bridgeText, StructuredOutputs: true}},
	}
	cfg := capabilityTestConfig(provider, target)
	evidence, expected := completeSurfaceEvidence(t, provider, target)
	if len(evidence) != 4 {
		t.Fatalf("structured-output requirement count=%d, want direct and bridge text plus structured output", len(evidence))
	}
	if failures := cfg.VerifyAdvertisedCapabilities("group", evidence, expected); len(failures) != 0 {
		t.Fatalf("complete structured-output evidence rejected: %v", failures)
	}
	for index, row := range evidence {
		if row.Identity.BridgeDirection == chatToResponsesBridgeDirection && row.CapabilityCase == "structured-outputs" {
			evidence = append(evidence[:index], evidence[index+1:]...)
			expected = append(expected[:index], expected[index+1:]...)
			if failures := cfg.VerifyAdvertisedCapabilities("group", evidence, expected); len(failures) != 1 {
				t.Fatalf("missing bridge structured-output evidence did not fail closed: %v", failures)
			}
			return
		}
	}
	t.Fatal("missing bridge structured-output requirement")
}

func TestEvidenceIdentityMustMatchResolvedTarget(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-chat", KeyID: "account-a"}
	target := Target{Provider: "p", Model: "synthetic:nitro", Dialect: "openai-chat", Weight: 1}
	cfg := capabilityTestConfig(provider, target)
	evidence, expected := completeSurfaceEvidence(t, provider, target)
	if got := evidence[0].Identity; got.Model != "synthetic" || got.ModelSuffix != ":nitro" {
		t.Fatalf("model identity not split into base and suffix: %#v", got)
	}
	if failures := cfg.VerifyAdvertisedCapabilities("group", evidence, expected); len(failures) != 0 {
		t.Fatalf("exact target identity rejected: %v", failures)
	}
	for _, mutation := range []struct {
		name   string
		change func(*CapabilityEvidenceIdentity)
	}{
		{name: "account", change: func(identity *CapabilityEvidenceIdentity) { identity.AccountIdentityClass = "account-b" }},
		{name: "endpoint", change: func(identity *CapabilityEvidenceIdentity) {
			identity.EndpointFingerprint = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{name: "model-suffix", change: func(identity *CapabilityEvidenceIdentity) { identity.ModelSuffix = ":free" }},
		{name: "api-skin", change: func(identity *CapabilityEvidenceIdentity) { identity.APISkin = "openai-responses" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			mutatedEvidence := append([]CapabilityEvidence(nil), evidence...)
			mutatedExpected := append([]CapabilityEvidenceIdentity(nil), expected...)
			mutation.change(&mutatedEvidence[0].Identity)
			mutatedExpected[0] = mutatedEvidence[0].Identity
			if failures := cfg.VerifyAdvertisedCapabilities("group", mutatedEvidence, mutatedExpected); len(failures) == 0 {
				t.Fatal("evidence for a different active target identity passed")
			}
		})
	}
}

func TestCapabilityEvidenceRequiresProviderKeyID(t *testing.T) {
	provider := ProviderConfig{BaseURL: "https://provider.example/v1", Dialect: "openai-chat"}
	target := Target{Provider: "p", Model: "synthetic", Dialect: "openai-chat", Weight: 1}
	failures := capabilityTestConfig(provider, target).VerifyAdvertisedCapabilities("group", nil, nil)
	if len(failures) != 1 {
		t.Fatalf("provider without non-secret key identity did not fail closed: %v", failures)
	}
}
