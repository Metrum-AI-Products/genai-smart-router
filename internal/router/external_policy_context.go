// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	externalPolicyConversationKeyPrefix = "ck_"
	externalPolicyConversationKeyHexLen = 32
	externalPolicyVerifierHintMetaKey   = "metrum_verifier_hint"
	externalPolicyDefaultVerifierSpec   = 4096
	externalPolicyDefaultVerifierVerLen = 64
)

var externalPolicyConversationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type externalPolicyVerifierHint struct {
	Kind    string         `json:"kind"`
	Version string         `json:"version,omitempty"`
	Spec    map[string]any `json:"spec,omitempty"`
}

func buildExternalPolicyConversationKey(cfg ExternalPolicyConversationKeyConfig, callerID, group string, req *IRRequest) string {
	scopeIdentity := callerConversationIdentity(req)
	if scopeIdentity == "" {
		scopeIdentity = "anonymous"
	}
	material := strings.Join([]string{
		strings.TrimSpace(cfg.Salt),
		strings.TrimSpace(callerID),
		strings.TrimSpace(group),
		scopeIdentity,
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return externalPolicyConversationKeyPrefix + hex.EncodeToString(sum[:externalPolicyConversationKeyHexLen/2])
}

func conversationKeySource(req *IRRequest) string {
	identity := callerConversationIdentity(req)
	switch {
	case strings.HasPrefix(identity, "id:"):
		return "conversation_id"
	case strings.HasPrefix(identity, "turn0:"):
		return "turn0_hash"
	default:
		return "anonymous"
	}
}

func callerConversationIdentity(req *IRRequest) string {
	if id := sanitizeConversationID(requestMetadataValue(req, "conversation_id", "conversationId", "session_id", "sessionId", "metrum_conversation_id")); id != "" {
		return "id:" + id
	}
	first := firstUserTurnText(req)
	if first == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(first))
	return "turn0:" + hex.EncodeToString(sum[:])
}

func sanitizeConversationID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" || !externalPolicyConversationIDPattern.MatchString(id) {
		return ""
	}
	return id
}

func firstUserTurnText(req *IRRequest) string {
	if req == nil {
		return ""
	}
	for _, msg := range req.Messages {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			return text
		}
		var b strings.Builder
		for _, part := range msg.Parts {
			if strings.TrimSpace(part.Text) != "" {
				b.WriteString(part.Text)
			}
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			return text
		}
	}
	if text := strings.TrimSpace(req.Input); text != "" {
		return text
	}
	if len(req.InputParts) > 0 {
		var b strings.Builder
		for _, part := range req.InputParts {
			if strings.TrimSpace(part.Text) != "" {
				b.WriteString(part.Text)
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

func requestMetadataValue(req *IRRequest, keys ...string) string {
	meta := requestMetadataMap(req)
	for _, key := range keys {
		if value := strings.TrimSpace(meta[key]); value != "" {
			return value
		}
	}
	return ""
}

func requestMetadataMap(req *IRRequest) map[string]string {
	out := map[string]string{}
	if req == nil {
		return out
	}
	for key, value := range req.Metadata {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	if req.Raw == nil {
		return out
	}
	rawMeta, ok := req.Raw["metadata"].(map[string]any)
	if !ok {
		return out
	}
	for key, value := range rawMeta {
		if strings.TrimSpace(key) == "" {
			continue
		}
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case fmt.Stringer:
			out[key] = typed.String()
		default:
			// Non-string metadata values are ignored for scalar key lookup.
		}
	}
	return out
}

func extractExternalPolicyVerifierHint(cfg ExternalPolicyVerifierHintsConfig, req *IRRequest) (*externalPolicyVerifierHint, error) {
	if !cfg.Enabled || req == nil {
		return nil, nil
	}
	rawHint, ok := rawVerifierHintValue(req)
	if !ok || rawHint == nil {
		return nil, nil
	}
	hint, err := normalizeExternalPolicyVerifierHint(cfg, rawHint)
	if err != nil {
		return nil, err
	}
	return hint, nil
}

func rawVerifierHintValue(req *IRRequest) (any, bool) {
	if req == nil {
		return nil, false
	}
	if req.Raw != nil {
		if value, ok := req.Raw["verifier_hint"]; ok && value != nil {
			return value, true
		}
		if value, ok := req.Raw["verifierHint"]; ok && value != nil {
			return value, true
		}
		if meta, ok := req.Raw["metadata"].(map[string]any); ok {
			if value, ok := meta[externalPolicyVerifierHintMetaKey]; ok && value != nil {
				return value, true
			}
			if value, ok := meta["verifier_hint"]; ok && value != nil {
				return value, true
			}
			if value, ok := meta["verifierHint"]; ok && value != nil {
				return value, true
			}
		}
	}
	if value := strings.TrimSpace(requestMetadataValue(req, externalPolicyVerifierHintMetaKey, "verifier_hint", "verifierHint")); value != "" {
		return value, true
	}
	return nil, false
}

func normalizeExternalPolicyVerifierHint(cfg ExternalPolicyVerifierHintsConfig, raw any) (*externalPolicyVerifierHint, error) {
	maxSpec := cfg.MaxSpecBytes
	if maxSpec <= 0 {
		maxSpec = externalPolicyDefaultVerifierSpec
	}
	maxVersion := cfg.MaxVersionLen
	if maxVersion <= 0 {
		maxVersion = externalPolicyDefaultVerifierVerLen
	}
	allowed := map[string]struct{}{}
	if len(cfg.AllowedKinds) == 0 {
		for _, kind := range []string{"none", "exact", "regex", "json_schema"} {
			allowed[kind] = struct{}{}
		}
	} else {
		for _, kind := range cfg.AllowedKinds {
			allowed[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
		}
	}

	var parsed map[string]any
	switch typed := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		if utf8.RuneCountInString(trimmed) > maxSpec+512 {
			return nil, fmt.Errorf("verifier hint exceeds size limit")
		}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("verifier hint must be JSON object metadata")
		}
	case map[string]any:
		parsed = typed
	default:
		return nil, fmt.Errorf("verifier hint must be a JSON object or JSON string")
	}

	kind := "none"
	if rawKind, ok := parsed["kind"]; ok && rawKind != nil {
		kind = strings.ToLower(strings.TrimSpace(fmt.Sprint(rawKind)))
	}
	if kind == "" {
		kind = "none"
	}
	if !externalPolicyVerifierKindAllowed(kind) {
		return nil, fmt.Errorf("verifier hint kind %q is not allowed", kind)
	}
	if _, ok := allowed[kind]; !ok {
		return nil, fmt.Errorf("verifier hint kind %q is not authorized", kind)
	}
	version := ""
	if rawVersion, ok := parsed["version"]; ok && rawVersion != nil {
		version = strings.TrimSpace(fmt.Sprint(rawVersion))
	}
	if version != "" {
		if len(version) > maxVersion || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`).MatchString(version) {
			return nil, fmt.Errorf("verifier hint version is invalid")
		}
	}
	spec := map[string]any{}
	if rawSpec, ok := parsed["spec"]; ok && rawSpec != nil {
		specMap, ok := rawSpec.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("verifier hint spec must be an object")
		}
		encoded, err := json.Marshal(specMap)
		if err != nil {
			return nil, fmt.Errorf("verifier hint spec is invalid")
		}
		if len(encoded) > maxSpec {
			return nil, fmt.Errorf("verifier hint spec exceeds max_spec_bytes")
		}
		if err := validateVerifierHintSpec(kind, specMap); err != nil {
			return nil, err
		}
		spec = specMap
	} else if kind != "none" {
		return nil, fmt.Errorf("verifier hint kind %q requires a spec object", kind)
	}
	hint := &externalPolicyVerifierHint{Kind: kind, Version: version}
	if len(spec) > 0 {
		hint.Spec = spec
	}
	return hint, nil
}

func validateVerifierHintSpec(kind string, spec map[string]any) error {
	required := map[string][]string{
		"none":        {},
		"exact":       {"expected"},
		"regex":       {"pattern"},
		"json_schema": {"schema"},
	}
	keys, ok := required[kind]
	if !ok {
		return fmt.Errorf("verifier hint kind %q is not allowed", kind)
	}
	if len(spec) != len(keys) {
		return fmt.Errorf("verifier hint spec keys are invalid for kind %q", kind)
	}
	for _, key := range keys {
		value, exists := spec[key]
		if !exists {
			return fmt.Errorf("verifier hint spec missing %s", key)
		}
		switch key {
		case "expected", "pattern":
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("verifier hint spec.%s must be a nonempty string", key)
			}
		case "schema":
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("verifier hint spec.schema must be an object")
			}
		}
	}
	return nil
}
