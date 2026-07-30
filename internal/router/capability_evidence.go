package router

import "strings"

// CapabilityEvidence is an import-safe, persistence-free DTO for the
// deterministic capability-smoke contract. It intentionally has no GORM tags
// or runtime wiring: synthetic evidence is non-promotable test input only.
type CapabilityEvidence struct {
	SchemaVersion  string                     `json:"schema_version"`
	Identity       CapabilityEvidenceIdentity `json:"identity"`
	CapabilityCase string                     `json:"capability_case"`
	Status         string                     `json:"status"`
}

type CapabilityEvidenceIdentity struct {
	Provider             string `json:"provider"`
	AccountIdentityClass string `json:"account_identity_class"`
	EndpointFingerprint  string `json:"endpoint_fingerprint"`
	EndpointPath         string `json:"endpoint_path"`
	APISkin              string `json:"api_skin"`
	Model                string `json:"model"`
	ModelSuffix          string `json:"model_suffix"`
	InboundDialect       string `json:"inbound_dialect"`
	BridgeDirection      string `json:"bridge_direction"`
	RequestShape         string `json:"request_shape"`
	ProfileVersion       string `json:"profile_version"`
}

// VerifyCapabilityClaims checks resolved (catalog plus target override)
// metadata against matching, passing evidence. It does not mutate routing,
// configuration, weights, or provider catalogs.
func (c *Config) VerifyCapabilityClaims(group string, claims []CapabilityEvidence) []string {
	modelGroup, ok := c.Models[group]
	if !ok {
		return []string{"unknown model group " + group}
	}
	var failures []string
	for _, claim := range claims {
		if claim.SchemaVersion != "capability-smoke/v1" || claim.Status != "passed" || !capabilityIdentityComplete(claim.Identity) {
			failures = append(failures, "capability evidence is not a passing v1 result")
			continue
		}
		matched := false
		for _, raw := range modelGroup.Targets {
			target, err := c.resolveTarget(group, raw)
			if err != nil || target.Provider != claim.Identity.Provider || target.Model != claim.Identity.Model {
				continue
			}
			if targetDialect(c.Provider[target.Provider], target) != claim.Identity.APISkin {
				continue
			}
			matched = true
			if !targetAdvertisesCapability(target, claim.CapabilityCase) {
				failures = append(failures, "target metadata does not advertise "+claim.CapabilityCase+" for "+target.Provider+"/"+target.Model)
			}
		}
		if !matched {
			failures = append(failures, "no matching resolved target for "+claim.Identity.Provider+"/"+claim.Identity.Model)
		}
	}
	return failures
}

// VerifyAdvertisedCapabilities fails closed when any resolved target capability
// lacks matching, complete, passing synthetic evidence. It is deliberately a
// test-only contract gate: no evidence is written or trusted for promotion.
func (c *Config) VerifyAdvertisedCapabilities(group string, evidence []CapabilityEvidence, expected []CapabilityEvidenceIdentity) []string {
	modelGroup, ok := c.Models[group]
	if !ok {
		return []string{"unknown model group " + group}
	}
	var failures []string
	for _, raw := range modelGroup.Targets {
		target, err := c.resolveTarget(group, raw)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		dialect := targetDialect(c.Provider[target.Provider], target)
		for _, capability := range advertisedCapabilities(target, dialect) {
			matched := false
			for _, row := range evidence {
				if row.SchemaVersion != "capability-smoke/v1" || row.Status != "passed" || row.CapabilityCase != capability || !capabilityIdentityComplete(row.Identity) {
					continue
				}
				if row.Identity.Provider == target.Provider && row.Identity.Model == target.Model && row.Identity.APISkin == dialect && identityMatchesTargetPath(row.Identity, dialect) && identityIsExpected(row.Identity, expected) {
					matched = true
					break
				}
			}
			if !matched {
				failures = append(failures, "missing passing evidence for "+capability+" on "+target.Provider+"/"+target.Model)
			}
		}
	}
	return failures
}

func advertisedCapabilities(target Target, dialect string) []string {
	cases := []string{"text"}
	if dialect == "openai-responses" {
		cases = append(cases, "openai-responses")
	}
	if targetAdvertisesToolCapability(target, dialect, "auto") {
		cases = append(cases, "tools-auto")
	}
	if targetAdvertisesToolCapability(target, dialect, "forced") {
		cases = append(cases, "tools-forced")
	}
	if targetAdvertisesCapability(target, "image-input") {
		cases = append(cases, "router-selected-ocr")
	}
	return cases
}

func targetAdvertisesToolCapability(target Target, dialect, wanted string) bool {
	var values []string
	switch dialect {
	case "openai-chat":
		values = target.ToolSupport.OpenAIChat
	case "openai-responses":
		values = target.ToolSupport.OpenAIResponses
	case "anthropic":
		values = target.ToolSupport.AnthropicMessages
	}
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func identityMatchesTargetPath(identity CapabilityEvidenceIdentity, dialect string) bool {
	expectedPath := "/v1/chat/completions"
	if dialect == "openai-responses" {
		expectedPath = "/v1/responses"
	}
	if dialect == "anthropic" {
		expectedPath = "/anthropic/v1/messages"
	}
	return identity.EndpointPath == expectedPath && identity.InboundDialect == identity.APISkin && identity.BridgeDirection == "none"
}

func identityIsExpected(identity CapabilityEvidenceIdentity, expected []CapabilityEvidenceIdentity) bool {
	for _, item := range expected {
		if identity == item {
			return true
		}
	}
	return false
}

func capabilityIdentityComplete(identity CapabilityEvidenceIdentity) bool {
	for _, value := range []string{identity.Provider, identity.AccountIdentityClass, identity.EndpointFingerprint, identity.EndpointPath, identity.APISkin, identity.Model, identity.InboundDialect, identity.BridgeDirection, identity.RequestShape, identity.ProfileVersion} {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true // model_suffix may deliberately be empty.
}

func targetAdvertisesCapability(target Target, capability string) bool {
	values := func(values []string, wanted string) bool {
		for _, value := range values {
			if value == wanted {
				return true
			}
		}
		return false
	}
	switch capability {
	case "text":
		return true
	case "image-input", "router-selected-ocr":
		return values(target.InputModalities, "image")
	case "tools-auto":
		return values(target.ToolSupport.OpenAIChat, "auto") || values(target.ToolSupport.OpenAIResponses, "auto") || values(target.ToolSupport.AnthropicMessages, "auto")
	case "tools-forced":
		return values(target.ToolSupport.OpenAIChat, "forced") || values(target.ToolSupport.OpenAIResponses, "forced") || values(target.ToolSupport.AnthropicMessages, "forced")
	case "openai-responses":
		return target.Dialect == "openai-responses"
	default:
		return false
	}
}
