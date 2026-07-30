package router

import "testing"

func syntheticEvidence(capability string) CapabilityEvidence {
	return CapabilityEvidence{SchemaVersion: "capability-smoke/v1", Status: "passed", CapabilityCase: capability, Identity: CapabilityEvidenceIdentity{Provider: "p", AccountIdentityClass: "test", EndpointFingerprint: "sha256:test", EndpointPath: "/v1/responses", APISkin: "openai-responses", Model: "synthetic", InboundDialect: "openai-responses", BridgeDirection: "none", RequestShape: "text", ProfileVersion: "synthetic/v1"}}
}

func TestVerifyCapabilityClaimsUsesResolvedTargetMetadata(t *testing.T) {
	cfg := &Config{Provider: map[string]ProviderConfig{"p": {Dialect: "openai-responses", Models: map[string]ProviderModel{"m": {Model: "synthetic", Dialect: "openai-responses", ToolSupport: ToolSupport{OpenAIResponses: []string{"auto"}}}}}}, Models: map[string]ModelGroup{"group": {Targets: []Target{{Provider: "p", ModelRef: "m", Weight: 1}}}}}
	passing := syntheticEvidence("tools-auto")
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{passing}); len(failures) != 0 {
		t.Fatalf("passing inherited capability rejected: %v", failures)
	}
	forced := passing
	forced.CapabilityCase = "tools-forced"
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{forced}); len(failures) != 1 {
		t.Fatalf("forced tool without metadata must fail: %v", failures)
	}
	responses := passing
	responses.CapabilityCase = "openai-responses"
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{responses}); len(failures) != 0 {
		t.Fatalf("matching Responses skin rejected: %v", failures)
	}
}

func TestVerifyAdvertisedCapabilitiesFailsClosedOnMissingEvidence(t *testing.T) {
	cfg := &Config{Provider: map[string]ProviderConfig{"p": {Dialect: "openai-responses", Models: map[string]ProviderModel{"m": {Model: "synthetic", Dialect: "openai-responses", ToolSupport: ToolSupport{OpenAIResponses: []string{"auto"}}}}}}, Models: map[string]ModelGroup{"group": {Targets: []Target{{Provider: "p", ModelRef: "m", Weight: 1}}}}}
	text := syntheticEvidence("text")
	expected := []CapabilityEvidenceIdentity{text.Identity}
	if failures := cfg.VerifyAdvertisedCapabilities("group", []CapabilityEvidence{text}, expected); len(failures) != 2 {
		t.Fatalf("missing advertised capability evidence must fail closed: %v", failures)
	}
	responses, auto := syntheticEvidence("openai-responses"), syntheticEvidence("tools-auto")
	if failures := cfg.VerifyAdvertisedCapabilities("group", []CapabilityEvidence{text, responses, auto}, []CapabilityEvidenceIdentity{text.Identity, responses.Identity, auto.Identity}); len(failures) != 0 {
		t.Fatalf("complete matching evidence rejected: %v", failures)
	}
}

func TestVerifyAdvertisedCapabilitiesRejectsCrossSkinToolEvidence(t *testing.T) {
	cfg := &Config{Provider: map[string]ProviderConfig{"p": {Dialect: "openai-chat", Models: map[string]ProviderModel{"m": {Model: "synthetic", Dialect: "openai-chat", ToolSupport: ToolSupport{OpenAIResponses: []string{"auto"}}}}}}, Models: map[string]ModelGroup{"group": {Targets: []Target{{Provider: "p", ModelRef: "m", Weight: 1}}}}}
	text := syntheticEvidence("text")
	text.Identity.APISkin, text.Identity.InboundDialect, text.Identity.EndpointPath = "openai-chat", "openai-chat", "/v1/chat/completions"
	if failures := cfg.VerifyAdvertisedCapabilities("group", []CapabilityEvidence{text}, []CapabilityEvidenceIdentity{text.Identity}); len(failures) != 0 {
		t.Fatalf("cross-skin Responses tool metadata must not advertise Chat tools: %v", failures)
	}
}

func TestVerifyCapabilityClaimsRejectsChatOnlyForResponses(t *testing.T) {
	cfg := &Config{Provider: map[string]ProviderConfig{"p": {Dialect: "openai-chat"}}, Models: map[string]ModelGroup{"group": {Targets: []Target{{Provider: "p", Model: "chat-only", Dialect: "openai-chat", Weight: 1}}}}}
	claim := syntheticEvidence("openai-responses")
	claim.Identity.Model, claim.Identity.APISkin, claim.Identity.EndpointPath = "chat-only", "openai-chat", "/v1/chat/completions"
	if failures := cfg.VerifyCapabilityClaims("group", []CapabilityEvidence{claim}); len(failures) != 1 {
		t.Fatalf("Chat-only target must reject Responses claim: %v", failures)
	}
}
