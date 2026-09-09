// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func spendGuardComplexRequest() *IRRequest {
	// Pad enough estimated tokens so dynamicComplexityScore reaches the complex bucket
	// while keeping the required keyword markers for SG-1/SG-2.
	filler := strings.Repeat("architecture analysis context ", 1800)
	return &IRRequest{
		Input:     "complex architecture security review " + filler,
		MaxTokens: 8192,
	}
}

func spendGuardComplexQualityGroup() ModelGroup {
	return ModelGroup{
		Strategy: "dynamic_score",
		RoutingPolicy: RoutingPolicyConfig{DynamicScore: DynamicScoreConfig{
			MinObservations:           1,
			MaxScoreAdjustmentPercent: 100,
			Signals: DynamicScoreSignals{
				PromptFeatures: DynamicSignalPromptFeatures{
					Enabled:      true,
					MaxScanBytes: 16384,
					Features:     []string{"security_review"},
				},
				Complexity:         DynamicSignalComplexity{Enabled: true},
				Cost:               DynamicSignalNamedFields{Enabled: true},
				EvaluationMetadata: DynamicSignalEvaluationMetadata{Enabled: true},
			},
			ScoreTerms: []DynamicScoreTerm{
				{
					Name:       "cheapest_fast_enough",
					When:       map[string]any{"complexity_lte": "standard"},
					Expression: "1.0 * cost_score",
				},
				{
					Name:        "complex_quality_floor",
					When:        map[string]any{"complexity_gte": "complex"},
					RequireTags: []string{"validated"},
					Expression:  "1.0 * eval_quality_score",
				},
			},
			EvaluationMetadata: []DynamicEvaluationTarget{
				{Provider: "mock", Model: "cheap-validated", QualityScore: 0.90},
				{Provider: "mock", Model: "expensive-validated", QualityScore: 0.99},
			},
		}},
		Targets: []Target{
			{
				Provider:                 "mock",
				Model:                    "cheap-validated",
				Tier:                     "cheap",
				Weight:                   50,
				Tags:                     []string{"validated"},
				InputPricePerMillionUSD:  0.1,
				OutputPricePerMillionUSD: 0.2,
				ToolSupport:              ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
			},
			{
				Provider:                 "mock",
				Model:                    "expensive-validated",
				Tier:                     "heavy",
				Weight:                   50,
				Tags:                     []string{"validated"},
				InputPricePerMillionUSD:  5.0,
				OutputPricePerMillionUSD: 10.0,
				ToolSupport:              ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
			},
		},
	}
}

func seedSpendGuardObservations(svc *Service, group string, models ...string) {
	now := time.Now().UTC()
	for _, model := range models {
		svc.observations.add(dynamicObservationKey(group, "mock", model), dynamicObservation{
			TS:                 now,
			Status:             http.StatusOK,
			LatencyMS:          100,
			UpstreamMS:         90,
			OutputTokensPerSec: 40,
		}, defaultDynamicObservationWindow)
	}
}

func TestSpendCeilingSG1DynamicScoreEscalatesWithoutCeiling(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := spendGuardComplexQualityGroup()
	seedSpendGuardObservations(svc, "adaptive-agent", "cheap-validated", "expensive-validated")
	req := spendGuardComplexRequest()
	dec, err := svc.pick(nil, "adaptive-agent", group, req, "openai-chat", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Target.Model != "expensive-validated" {
		t.Fatalf("SG-1: selected %q, want expensive-validated when no spend ceiling", dec.Target.Model)
	}
}

func TestSpendCeilingSG2CallerCheapTierBlocksExpensive(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := spendGuardComplexQualityGroup()
	seedSpendGuardObservations(svc, "adaptive-agent", "cheap-validated", "expensive-validated")
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	req := spendGuardComplexRequest()
	dec, err := svc.pick(nil, "adaptive-agent", group, req, "openai-chat", caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Target.Model != "cheap-validated" || dec.Target.Tier != "cheap" {
		t.Fatalf("SG-2: selected %#v, want cheap-validated under cheap-tier ceiling", dec.Target)
	}
	if strings.Contains(dec.DecisionTrace, "complex architecture") {
		t.Fatalf("SG-2/SG-7: decision trace leaked raw prompt text: %s", dec.DecisionTrace)
	}
	if !strings.Contains(dec.DecisionTrace, spendCeilingReason) {
		t.Fatalf("SG-2/SG-7: decision trace missing %q: %s", spendCeilingReason, dec.DecisionTrace)
	}
}

func TestSpendCeilingSG3ToolsCannotBypassCeiling(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := spendGuardComplexQualityGroup()
	seedSpendGuardObservations(svc, "adaptive-agent", "cheap-validated", "expensive-validated")
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	// Tools raise complexity; ceiling must still block expensive escalation.
	req := &IRRequest{
		Input:     "complex architecture security " + strings.Repeat("tool agent context ", 500),
		MaxTokens: 8192,
		Tools: []map[string]any{
			{"type": "function", "function": map[string]any{"name": "read_file"}},
			{"type": "function", "function": map[string]any{"name": "write_file"}},
			{"type": "function", "function": map[string]any{"name": "run_tests"}},
		},
	}
	dec, err := svc.pick(nil, "adaptive-agent", group, req, "openai-chat", caller, "")
	if err != nil {
		t.Fatalf("SG-3: unexpected error: %v", err)
	}
	if dec.Target.Model != "cheap-validated" {
		t.Fatalf("SG-3: tools complexity bump selected %q, want cheap-validated", dec.Target.Model)
	}
}

func TestSpendCeilingSG4WeightedDefaultUnchanged(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := ModelGroup{
		Strategy: "weighted",
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 1},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 100},
		},
	}
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	seenHeavy := false
	for i := 0; i < 40; i++ {
		dec, err := svc.pick(nil, "default", group, &IRRequest{Input: "hello"}, "openai-chat", caller, "")
		if err != nil {
			t.Fatal(err)
		}
		if dec.Target.Model == "heavy-model" {
			seenHeavy = true
			break
		}
	}
	if !seenHeavy {
		t.Fatal("SG-4: weighted default should still be able to select heavy tier; spend ceiling must not apply")
	}
}

func TestSpendCeilingSG5SemanticClassifyStubCapped(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := ModelGroup{
		Strategy: "semantic",
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 10},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 90},
		},
	}
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	req := &IRRequest{Input: "complex architecture reason carefully"}
	dec, err := svc.pick(nil, "semantic-group", group, req, "openai-chat", caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Target.Model != "cheap-model" {
		t.Fatalf("SG-5: semantic heavy classification selected %q, want cheap-model under ceiling", dec.Target.Model)
	}
	if !strings.Contains(dec.DecisionTrace, spendCeilingReason) {
		t.Fatalf("SG-5/SG-7: expected spend-ceiling trace, got %s", dec.DecisionTrace)
	}
	if strings.Contains(dec.DecisionTrace, req.Input) {
		t.Fatalf("SG-5/SG-7: decision trace leaked raw prompt")
	}
}

func TestSpendCeilingSG6ConfigValidationRejectsNegativesAndUnknownTiers(t *testing.T) {
	neg := -0.01
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Callers[0].SpendCeiling = SpendCeilingConfig{MaxEstimatedCostUSD: &neg}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_estimated_cost_usd cannot be negative") {
		t.Fatalf("SG-6: expected negative ceiling rejection, got %v", err)
	}

	cfg = testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Callers[0].SpendCeiling = SpendCeilingConfig{MaxTier: "platinum"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("SG-6: expected unknown max_tier rejection, got %v", err)
	}

	cfg = testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{
		Strategy:     "static",
		SpendCeiling: SpendCeilingConfig{TierCeilings: map[string]float64{"bogus": 0.1}},
		Targets:      []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("SG-6: expected unknown tier_ceilings rejection, got %v", err)
	}

	cfg = testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Callers[0].SpendCeiling = SpendCeilingConfig{TierCeilings: map[string]float64{"cheap": -1}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("SG-6: expected negative tier ceiling rejection, got %v", err)
	}
}

func TestSpendCeilingSG7TelemetryRecordsSafeReason(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	svc.cfg.Server.DecisionTelemetry.Enabled = true
	group := spendGuardComplexQualityGroup()
	seedSpendGuardObservations(svc, "adaptive-agent", "cheap-validated", "expensive-validated")
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	rc := &requestContext{rec: logRecord{}}
	req := spendGuardComplexRequest()
	dec, err := svc.pick(rc, "adaptive-agent", group, req, "openai-chat", caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Target.Model != "cheap-validated" {
		t.Fatalf("SG-7: selected %q, want cheap-validated", dec.Target.Model)
	}
	found := false
	for _, reason := range rc.rec.DecisionFilterReasons {
		if reason.Reason == spendCeilingReason {
			found = true
			if reason.Stage != "spend_ceiling" {
				t.Fatalf("SG-7: stage=%q, want spend_ceiling", reason.Stage)
			}
		}
		if strings.Contains(reason.Reason, "complex") || strings.Contains(reason.Reason, req.Input) {
			t.Fatalf("SG-7: filter reason leaked prompt material: %#v", reason)
		}
	}
	if !found {
		t.Fatalf("SG-7: DecisionFilterReasons missing %q: %#v", spendCeilingReason, rc.rec.DecisionFilterReasons)
	}
	if !strings.Contains(dec.DecisionTrace, spendCeilingReason) {
		t.Fatalf("SG-7: DecisionTrace missing %q: %s", spendCeilingReason, dec.DecisionTrace)
	}
	if strings.Contains(dec.DecisionTrace, "complex architecture") {
		t.Fatalf("SG-7: DecisionTrace leaked raw prompt")
	}
}

func TestSpendCeilingNoEligibleWhenOnlyExpensiveRemain(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	group := ModelGroup{
		Strategy: "semantic",
		Targets: []Target{
			{Provider: "mock", Model: "heavy-a", Tier: "heavy", Weight: 50},
			{Provider: "mock", Model: "heavy-b", Tier: "heavy", Weight: 50},
		},
	}
	caller := &callerRuntime{cfg: CallerConfig{SpendCeiling: SpendCeilingConfig{MaxTier: "cheap"}}}
	_, err := svc.pick(nil, "semantic-group", group, &IRRequest{Input: "complex architecture reason"}, "openai-chat", caller, "")
	var eligibility routingEligibilityError
	if !errors.As(err, &eligibility) {
		t.Fatalf("expected routingEligibilityError, got %v", err)
	}
	joined := strings.Join(eligibility.Requirements, ",")
	if !strings.Contains(joined, spendCeilingReason) {
		t.Fatalf("expected spend-ceiling requirement, got %v", eligibility.Requirements)
	}
}
