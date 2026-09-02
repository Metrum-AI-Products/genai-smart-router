// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// intelligentCandidate is the deliberately small view a future decision-model
// call may receive. ID is opaque: provider, model, credentials, and request
// content are intentionally not represented here.
type intelligentCandidate struct {
	ID          string
	Target      Target
	TargetIndex int
}

// intelligentSelection is the only accepted v1 structured-selector result.
// It is kept separate from provider response types so untrusted model output
// cannot be used as a routing target without validation.
type intelligentSelection struct {
	SchemaVersion string   `json:"schema_version"`
	CandidateID   string   `json:"candidate_id"`
	FallbackIDs   []string `json:"fallback_ids"`
	Confidence    float64  `json:"confidence"`
	ClassLabel    string   `json:"class_label"`
	ReasonCodes   []string `json:"reason_codes"`
}

var (
	errIntelligentMalformedSelection = errors.New("intelligent selector returned malformed selection")
	errIntelligentLowConfidence      = errors.New("intelligent selector returned low-confidence selection")
	intelligentReasonCode            = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}$`)
)

// intelligentCandidates assigns stable per-request opaque IDs after all normal
// eligibility filtering has completed. The IDs are only useful inside the
// decision exchange and reveal neither target identity nor configuration.
func intelligentCandidates(targets []Target) []intelligentCandidate {
	candidates := make([]intelligentCandidate, len(targets))
	for i, target := range targets {
		candidates[i] = intelligentCandidate{
			ID:          fmt.Sprintf("candidate-%04d", i+1),
			Target:      target,
			TargetIndex: i,
		}
	}
	return candidates
}

// validateIntelligentSelection makes a recommendation usable only when it is
// complete, schema-compatible, confident enough, and wholly constrained to
// the already eligible opaque candidate set. Callers must use their configured
// deterministic baseline on every returned error.
func validateIntelligentSelection(cfg IntelligentRoutingConfig, candidates []intelligentCandidate, selection intelligentSelection) (intelligentCandidate, []intelligentCandidate, error) {
	if len(candidates) == 0 || selection.SchemaVersion != cfg.SchemaVersion || strings.TrimSpace(selection.CandidateID) == "" ||
		math.IsNaN(selection.Confidence) || math.IsInf(selection.Confidence, 0) || selection.Confidence < 0 || selection.Confidence > 1 {
		return intelligentCandidate{}, nil, errIntelligentMalformedSelection
	}
	if selection.Confidence < cfg.ConfidenceThreshold {
		return intelligentCandidate{}, nil, errIntelligentLowConfidence
	}
	label := safePolicyClassLabel(selection.ClassLabel)
	if label == nil || *label == unsafePolicyClassLabel {
		return intelligentCandidate{}, nil, errIntelligentMalformedSelection
	}
	if len(selection.FallbackIDs) > 32 || len(selection.ReasonCodes) > 8 {
		return intelligentCandidate{}, nil, errIntelligentMalformedSelection
	}
	reasons := make(map[string]struct{}, len(selection.ReasonCodes))
	for _, code := range selection.ReasonCodes {
		if !intelligentReasonCode.MatchString(code) {
			return intelligentCandidate{}, nil, errIntelligentMalformedSelection
		}
		if _, duplicate := reasons[code]; duplicate {
			return intelligentCandidate{}, nil, errIntelligentMalformedSelection
		}
		reasons[code] = struct{}{}
	}

	byID := make(map[string]intelligentCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ID] = candidate
	}
	selected, ok := byID[selection.CandidateID]
	if !ok {
		return intelligentCandidate{}, nil, errIntelligentMalformedSelection
	}
	fallbacks := make([]intelligentCandidate, 0, len(selection.FallbackIDs))
	seen := map[string]struct{}{selection.CandidateID: {}}
	for _, id := range selection.FallbackIDs {
		candidate, ok := byID[id]
		if !ok {
			return intelligentCandidate{}, nil, errIntelligentMalformedSelection
		}
		if _, duplicate := seen[id]; duplicate {
			return intelligentCandidate{}, nil, errIntelligentMalformedSelection
		}
		seen[id] = struct{}{}
		fallbacks = append(fallbacks, candidate)
	}
	return selected, fallbacks, nil
}
