// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultDynamicObservationWindow = 10 * time.Minute

var dynamicExpressionTermRE = regexp.MustCompile(`(?i)([0-9]*\.?[0-9]+)\s*\*\s*([a-z_]+_score|[a-z_]+)`)

type dynamicObservationStore struct {
	mu      sync.Mutex
	records map[string][]dynamicObservation
}

type dynamicObservation struct {
	TS                 time.Time
	Status             int
	LatencyMS          int64
	TTFBMS             int64
	UpstreamMS         int64
	OutputTokensPerSec float64
	FallbackUsed       bool
	ErrorClass         string
}

type dynamicStats struct {
	Count              int
	ErrorRate          float64
	TimeoutRate        float64
	FallbackRate       float64
	P50LatencyMS       float64
	P95LatencyMS       float64
	TTFBMS             float64
	DurationMS         float64
	OutputTokensPerSec float64
}

type dynamicCandidate struct {
	Target Target
	Stats  dynamicStats
	Scores map[string]float64
	Final  float64
	Index  int
}

func newDynamicObservationStore() *dynamicObservationStore {
	return &dynamicObservationStore{records: map[string][]dynamicObservation{}}
}

func (s *Service) recordDynamicObservation(rec logRecord) {
	if s == nil || s.observations == nil || rec.ResolvedGroup == "" {
		return
	}
	// Router response-cache hits measure local cache latency, not upstream
	// performance. Exclude them so dynamic_score cannot treat a cheap cache hit
	// as evidence that a target is fast.
	if strings.EqualFold(strings.TrimSpace(rec.Cache), "hit") {
		return
	}
	retention := s.dynamicObservationRetention(rec.ResolvedGroup)
	if len(rec.AttemptsDetail) > 0 {
		for _, attempt := range rec.AttemptsDetail {
			if attempt.Provider == "" || attempt.Model == "" {
				continue
			}
			obs := dynamicObservation{
				TS:           time.Now().UTC(),
				Status:       attempt.StatusCode,
				LatencyMS:    attempt.DurationMS,
				UpstreamMS:   attempt.DurationMS,
				FallbackUsed: attempt.Index > 1 || attempt.FallbackReason != "",
				ErrorClass:   attempt.ErrorClass,
			}
			if rec.TTFBMS != nil && attempt.Selected {
				obs.TTFBMS = *rec.TTFBMS
			}
			if rec.UpstreamOutputTPS != nil && attempt.Selected {
				obs.OutputTokensPerSec = *rec.UpstreamOutputTPS
			}
			key := dynamicObservationKey(rec.ResolvedGroup, attempt.Provider, attempt.Model)
			s.observations.add(key, obs, retention)
		}
		return
	}
	if rec.TargetProvider == "" || rec.TargetModel == "" {
		return
	}
	obs := dynamicObservation{
		TS:           time.Now().UTC(),
		Status:       rec.Status,
		LatencyMS:    rec.LatencyMS,
		FallbackUsed: rec.FallbackUsed,
		ErrorClass:   rec.ErrorClass,
	}
	if rec.TTFBMS != nil {
		obs.TTFBMS = *rec.TTFBMS
	}
	if rec.UpstreamMS != nil {
		obs.UpstreamMS = *rec.UpstreamMS
	}
	if rec.UpstreamOutputTPS != nil {
		obs.OutputTokensPerSec = *rec.UpstreamOutputTPS
	}
	key := dynamicObservationKey(rec.ResolvedGroup, rec.TargetProvider, rec.TargetModel)
	s.observations.add(key, obs, retention)
}

func (s *Service) dynamicObservationRetention(groupName string) time.Duration {
	if s == nil || s.cfg == nil {
		return defaultDynamicObservationWindow
	}
	group, ok := s.cfg.Models[groupName]
	if !ok || !strings.EqualFold(group.Strategy, "dynamic_score") {
		return defaultDynamicObservationWindow
	}
	window := time.Duration(group.RoutingPolicy.DynamicScore.ObservationWindowSeconds) * time.Second
	if window <= 0 {
		return defaultDynamicObservationWindow
	}
	return window
}

func (o *dynamicObservationStore) add(key string, obs dynamicObservation, fallbackWindow time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	window := fallbackWindow
	cutoff := obs.TS.Add(-window)
	current := o.records[key]
	filtered := current[:0]
	for _, rec := range current {
		if rec.TS.After(cutoff) {
			filtered = append(filtered, rec)
		}
	}
	filtered = append(filtered, obs)
	o.records[key] = append([]dynamicObservation(nil), filtered...)
}

func (o *dynamicObservationStore) stats(key string, window time.Duration) dynamicStats {
	o.mu.Lock()
	defer o.mu.Unlock()
	if window <= 0 {
		window = defaultDynamicObservationWindow
	}
	cutoff := time.Now().UTC().Add(-window)
	records := o.records[key]
	filtered := records[:0]
	for _, rec := range records {
		if rec.TS.After(cutoff) {
			filtered = append(filtered, rec)
		}
	}
	o.records[key] = append([]dynamicObservation(nil), filtered...)
	return summarizeDynamicObservations(filtered)
}

func summarizeDynamicObservations(records []dynamicObservation) dynamicStats {
	stats := dynamicStats{Count: len(records)}
	if len(records) == 0 {
		return stats
	}
	latencies := make([]float64, 0, len(records))
	var errors, timeouts, fallbacks int
	var ttfbTotal, ttfbCount, durationTotal, durationCount, tpsTotal, tpsCount float64
	for _, rec := range records {
		if rec.Status >= 500 || rec.ErrorClass != "" {
			errors++
		}
		if strings.Contains(rec.ErrorClass, "timeout") {
			timeouts++
		}
		if rec.FallbackUsed {
			fallbacks++
		}
		if rec.LatencyMS > 0 {
			latencies = append(latencies, float64(rec.LatencyMS))
		}
		if rec.TTFBMS > 0 {
			ttfbTotal += float64(rec.TTFBMS)
			ttfbCount++
		}
		if rec.UpstreamMS > 0 {
			durationTotal += float64(rec.UpstreamMS)
			durationCount++
		}
		if rec.OutputTokensPerSec > 0 {
			tpsTotal += rec.OutputTokensPerSec
			tpsCount++
		}
	}
	stats.ErrorRate = float64(errors) / float64(len(records))
	stats.TimeoutRate = float64(timeouts) / float64(len(records))
	stats.FallbackRate = float64(fallbacks) / float64(len(records))
	sort.Float64s(latencies)
	stats.P50LatencyMS = percentile(latencies, 0.50)
	stats.P95LatencyMS = percentile(latencies, 0.95)
	stats.TTFBMS = avgOrZero(ttfbTotal, ttfbCount)
	stats.DurationMS = avgOrZero(durationTotal, durationCount)
	stats.OutputTokensPerSec = avgOrZero(tpsTotal, tpsCount)
	return stats
}

func (s *Service) pickDynamicScore(groupName string, group ModelGroup, req *IRRequest, callerDialect string, targets []Target) (decision, error) {
	cfg := group.RoutingPolicy.DynamicScore
	window := time.Duration(cfg.ObservationWindowSeconds) * time.Second
	if window <= 0 {
		window = defaultDynamicObservationWindow
	}
	candidates := make([]dynamicCandidate, 0, len(targets))
	for i, target := range targets {
		outDialect := ""
		if s != nil && s.cfg != nil {
			outDialect = targetDialect(s.cfg.Provider[target.Provider], target)
		}
		if !dynamicPassesHardFilters(target, req, callerDialect, outDialect, cfg.HardFilters) {
			continue
		}
		stats := dynamicStats{}
		if s != nil && s.observations != nil {
			stats = s.observations.stats(dynamicObservationKey(groupName, target.Provider, target.Model), window)
		}
		if !dynamicPassesThresholds(stats, cfg.Thresholds) {
			continue
		}
		candidates = append(candidates, dynamicCandidate{Target: target, Stats: stats, Scores: map[string]float64{}, Index: i})
	}
	if len(candidates) == 0 {
		return decision{}, routingEligibilityError{
			Model:        groupName,
			Dialect:      callerDialect,
			Requirements: append(routingRequirements(req, callerDialect), "dynamic_score_thresholds"),
		}
	}
	minObservations := cfg.MinObservations
	if minObservations <= 0 {
		minObservations = 1
	}
	totalObservations := 0
	for _, candidate := range candidates {
		totalObservations += candidate.Stats.Count
	}
	coldStart := totalObservations < minObservations
	if coldStart {
		ordered := configuredWeightOrder(candidates)
		trace := dynamicDecisionTrace(cfg, req, ordered, true, "configured_weight")
		signals := dynamicRoutingSignalTelemetry(cfg, "dynamic_score")
		terms := dynamicColdStartRankingTelemetry(ordered, groupName)
		return decision{Target: ordered[0].Target, Fallbacks: dynamicFallbacks(ordered[1:]), Strategy: "dynamic_score", GroupName: groupName, TargetIndex: ordered[0].Index, DecisionTrace: trace, RoutingSignals: signals, DynamicScoreTerms: terms}, nil
	}
	dynamicNormalizeScores(candidates, req, cfg)
	terms := dynamicTermsForRequest(cfg, req)
	if len(terms) == 0 {
		terms = []DynamicScoreTerm{{Name: "default", Weights: map[string]float64{
			"cost_score":        0.25,
			"latency_score":     0.25,
			"throughput_score":  0.20,
			"reliability_score": 0.30,
		}}}
	}
	weightScores := configuredWeightScores(candidates)
	adjustment := cfg.MaxScoreAdjustmentPercent / 100
	for i := range candidates {
		dynamicScore := 0.0
		for _, term := range terms {
			if !targetHasAllTags(candidates[i].Target, term.RequireTags) {
				dynamicScore = math.Inf(-1)
				continue
			}
			dynamicScore += dynamicEvaluateTerm(candidates[i], term)
			for _, tag := range term.PreferTags {
				if targetHasTag(candidates[i].Target, tag) {
					dynamicScore += 0.05
				}
			}
		}
		if math.IsInf(dynamicScore, -1) {
			candidates[i].Final = dynamicScore
		} else {
			candidates[i].Final = (1-adjustment)*weightScores[i] + adjustment*dynamicScore
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Final != candidates[j].Final {
			return candidates[i].Final > candidates[j].Final
		}
		if candidates[i].Target.Weight != candidates[j].Target.Weight {
			return candidates[i].Target.Weight > candidates[j].Target.Weight
		}
		return candidates[i].Index < candidates[j].Index
	})
	trace := dynamicDecisionTrace(cfg, req, candidates, false, "score")
	signals := dynamicRoutingSignalTelemetry(cfg, "dynamic_score")
	scoreTerms := dynamicScoreTermTelemetry(candidates, terms, groupName)
	return decision{Target: candidates[0].Target, Fallbacks: dynamicFallbacks(candidates[1:]), Strategy: "dynamic_score", GroupName: groupName, TargetIndex: candidates[0].Index, DecisionTrace: trace, RoutingSignals: signals, DynamicScoreTerms: scoreTerms}, nil
}

func dynamicPassesThresholds(stats dynamicStats, thresholds DynamicScoreThresholds) bool {
	if stats.Count == 0 {
		return true
	}
	if thresholds.MaxErrorRate != nil && stats.ErrorRate > *thresholds.MaxErrorRate {
		return false
	}
	if thresholds.MaxTimeoutRate != nil && stats.TimeoutRate > *thresholds.MaxTimeoutRate {
		return false
	}
	if thresholds.MaxP95LatencyMS != nil && stats.P95LatencyMS > *thresholds.MaxP95LatencyMS {
		return false
	}
	if thresholds.MaxTTFBMS != nil && stats.TTFBMS > *thresholds.MaxTTFBMS {
		return false
	}
	if thresholds.MinOutputTokensPerSecond != nil && stats.OutputTokensPerSec > 0 && stats.OutputTokensPerSec < *thresholds.MinOutputTokensPerSecond {
		return false
	}
	return true
}

func dynamicPassesHardFilters(target Target, req *IRRequest, callerDialect, outDialect string, filters DynamicScoreHardFilters) bool {
	if filters.RequireRequestedAPISkin && outDialect != "" && callerDialect != outDialect {
		return false
	}
	if filters.RequireForcedToolChoiceSupport && requestHasForcedToolChoice(req) && !targetSupportsClientTools(target, outDialect, true) {
		return false
	}
	if filters.RequireStructuredOutputSupport && requestHasStructuredOutput(req) && !targetSupportsStructuredOutput(target, callerDialect, outDialect, true) {
		return false
	}
	if filters.RequireReasoningSupportWhenRequested && requestRequiresReasoning(req) && !targetCanSatisfyReasoning(target, outDialect, req) {
		return false
	}
	return true
}

func requestHasForcedToolChoice(req *IRRequest) bool {
	if req == nil || req.Raw == nil {
		return false
	}
	choice, ok := req.Raw["tool_choice"]
	if !ok || choice == nil {
		return false
	}
	switch v := choice.(type) {
	case string:
		return v != "" && v != "auto" && v != "none"
	case map[string]any:
		if t, _ := v["type"].(string); t != "" && t != "auto" && t != "none" {
			return true
		}
		return len(v) > 0
	default:
		return true
	}
}

func requestHasStructuredOutput(req *IRRequest) bool {
	if req == nil || req.Raw == nil {
		return false
	}
	if rf, ok := req.Raw["response_format"]; ok && rf != nil {
		return structuredOutputFormatRequiresSupport(rf)
	}
	if text, ok := req.Raw["text"].(map[string]any); ok {
		if format, ok := text["format"]; ok && format != nil {
			return structuredOutputFormatRequiresSupport(format)
		}
	}
	return false
}

func structuredOutputFormatRequiresSupport(format any) bool {
	var typ string
	switch v := format.(type) {
	case string:
		typ = v
	case map[string]any:
		typ, _ = v["type"].(string)
	default:
		return false
	}
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "json_schema", "json_object":
		return true
	default:
		return false
	}
}

func targetCapabilityValues(target Target, dialect string) []string {
	switch dialect {
	case "openai-responses":
		return append([]string(nil), target.ToolSupport.OpenAIResponses...)
	case "anthropic":
		return append([]string(nil), target.ToolSupport.AnthropicMessages...)
	default:
		return append([]string(nil), target.ToolSupport.OpenAIChat...)
	}
}

func dynamicNormalizeScores(candidates []dynamicCandidate, req *IRRequest, cfg DynamicScoreConfig) {
	costs := make([]float64, len(candidates))
	latencies := make([]float64, len(candidates))
	throughputs := make([]float64, len(candidates))
	for i := range candidates {
		costs[i] = dynamicTargetCost(candidates[i].Target)
		latencies[i] = candidates[i].Stats.P95LatencyMS
		throughputs[i] = candidates[i].Stats.OutputTokensPerSec
	}
	complexity := dynamicComplexityScore(req, cfg)
	for i := range candidates {
		candidates[i].Scores["cost_score"] = inverseNormalize(costs[i], costs)
		candidates[i].Scores["latency_score"] = inverseNormalize(latencies[i], latencies)
		candidates[i].Scores["throughput_score"] = normalizePositive(throughputs[i], throughputs)
		candidates[i].Scores["reliability_score"] = clamp01(1 - candidates[i].Stats.ErrorRate - candidates[i].Stats.TimeoutRate - candidates[i].Stats.FallbackRate*0.5)
		candidates[i].Scores["complexity_score"] = complexity
		candidates[i].Scores["eval_quality_score"] = dynamicEvalScore(candidates[i].Target, cfg.EvaluationMetadata)
		candidates[i].Scores["budget_score"] = 1 - candidates[i].Scores["cost_score"]
	}
}

func dynamicTermsForRequest(cfg DynamicScoreConfig, req *IRRequest) []DynamicScoreTerm {
	if len(cfg.ScoreTerms) == 0 {
		return nil
	}
	features := dynamicPromptFeatures(req, cfg.Signals.PromptFeatures)
	complexity := dynamicComplexityBucket(dynamicComplexityScore(req, cfg))
	out := make([]DynamicScoreTerm, 0, len(cfg.ScoreTerms))
	for _, term := range cfg.ScoreTerms {
		if dynamicWhenMatches(term.When, features, complexity) {
			out = append(out, term)
		}
	}
	return out
}

func dynamicWhenMatches(when map[string]any, features map[string]bool, complexity string) bool {
	if len(when) == 0 {
		return true
	}
	if v, ok := when["prompt_feature"].(string); ok && !features[v] {
		return false
	}
	if v, ok := when["complexity_lte"].(string); ok && complexityRank(complexity) > complexityRank(v) {
		return false
	}
	if v, ok := when["complexity_gte"].(string); ok && complexityRank(complexity) < complexityRank(v) {
		return false
	}
	return true
}

func dynamicEvaluateTerm(candidate dynamicCandidate, term DynamicScoreTerm) float64 {
	weights := term.Weights
	if len(weights) == 0 {
		weights = parseDynamicExpression(term.Expression)
	}
	total := 0.0
	for name, weight := range weights {
		total += weight * candidate.Scores[canonicalScoreName(name)]
	}
	return total
}

func parseDynamicExpression(expression string) map[string]float64 {
	out := map[string]float64{}
	for _, match := range dynamicExpressionTermRE.FindAllStringSubmatch(expression, -1) {
		if len(match) != 3 {
			continue
		}
		weight, err := strconv.ParseFloat(match[1], 64)
		if err == nil {
			out[canonicalScoreName(match[2])] += weight
		}
	}
	return out
}

func dynamicDecisionTrace(cfg DynamicScoreConfig, req *IRRequest, candidates []dynamicCandidate, coldStart bool, mode string) string {
	payload := map[string]any{
		"strategy":        "dynamic_score",
		"mode":            mode,
		"cold_start":      coldStart,
		"signals":         dynamicEnabledSignals(cfg),
		"shape":           dynamicSafeShape(req),
		"candidate_count": len(candidates),
	}
	if len(candidates) > 0 {
		selected := candidates[0]
		payload["selected"] = map[string]any{
			"provider":     selected.Target.Provider,
			"model":        selected.Target.Model,
			"score":        roundScore(selected.Final),
			"observations": selected.Stats.Count,
		}
	}
	if raw, err := json.Marshal(payload); err == nil {
		return string(raw)
	}
	return "strategy=dynamic_score"
}

func dynamicEnabledSignals(cfg DynamicScoreConfig) []string {
	var out []string
	if cfg.Signals.RequestShape.Enabled {
		out = append(out, "request_shape")
	}
	if cfg.Signals.PromptFeatures.Enabled {
		out = append(out, "prompt_features")
	}
	if cfg.Signals.Complexity.Enabled {
		out = append(out, "complexity")
	}
	if cfg.Signals.ObservedPerformance.Enabled {
		out = append(out, "observed_performance")
	}
	if cfg.Signals.Cost.Enabled {
		out = append(out, "cost")
	}
	if cfg.Signals.BudgetPressure.Enabled {
		out = append(out, "budget_pressure")
	}
	if cfg.Signals.EvaluationMetadata.Enabled {
		out = append(out, "evaluation_metadata")
	}
	if cfg.Signals.RecentPenalties.Enabled {
		out = append(out, "recent_penalties")
	}
	return out
}

func dynamicRoutingSignalTelemetry(cfg DynamicScoreConfig, strategy string) []routingSignalLogRecord {
	enabled := dynamicEnabledSignals(cfg)
	out := make([]routingSignalLogRecord, 0, len(enabled)+4)
	for _, signal := range enabled {
		out = append(out, routingSignalLogRecord{
			Seq:        len(out) + 1,
			Strategy:   strategy,
			SignalName: signal,
			Source:     "dynamic_score",
			BoolValue:  true,
		})
	}
	out = append(out,
		routingSignalLogRecord{Seq: len(out) + 1, Strategy: strategy, SignalName: "min_observations", Source: "dynamic_score", IntValue: cfg.MinObservations},
		routingSignalLogRecord{Seq: len(out) + 2, Strategy: strategy, SignalName: "observation_window_seconds", Source: "dynamic_score", IntValue: cfg.ObservationWindowSeconds},
		routingSignalLogRecord{Seq: len(out) + 3, Strategy: strategy, SignalName: "max_score_adjustment_percent", Source: "dynamic_score", FloatValue: cfg.MaxScoreAdjustmentPercent},
		routingSignalLogRecord{Seq: len(out) + 4, Strategy: strategy, SignalName: "cold_start_policy", Source: "dynamic_score", TextValue: cfg.ColdStartPolicy},
	)
	return out
}

func dynamicColdStartRankingTelemetry(candidates []dynamicCandidate, groupName string) []dynamicScoreTermLogRecord {
	out := make([]dynamicScoreTermLogRecord, 0, len(candidates))
	for rank, candidate := range candidates {
		finalScore := float64(candidate.Target.Weight)
		out = append(out, dynamicScoreTermLogRecord{
			Seq:              len(out) + 1,
			CandidateIndex:   candidate.Index,
			Rank:             rank + 1,
			Provider:         candidate.Target.Provider,
			Model:            candidate.Target.Model,
			Dialect:          candidate.Target.Dialect,
			TermName:         "cold_start",
			ScoreName:        "configured_weight",
			Weight:           float64(candidate.Target.Weight),
			Value:            float64(candidate.Target.Weight),
			FinalScore:       finalScore,
			ValueBucket:      scoreValueBucket(finalScore),
			FinalScoreBucket: scoreValueBucket(finalScore),
			ObservationCount: candidate.Stats.Count,
			Selected:         rank == 0,
		})
		_ = groupName
	}
	return out
}

func dynamicScoreTermTelemetry(candidates []dynamicCandidate, terms []DynamicScoreTerm, groupName string) []dynamicScoreTermLogRecord {
	out := []dynamicScoreTermLogRecord{}
	for rank, candidate := range candidates {
		for _, term := range terms {
			weights := term.Weights
			if len(weights) == 0 && term.Expression != "" {
				weights = parseDynamicExpression(term.Expression)
			}
			if len(weights) == 0 && len(term.PreferTags) == 0 && len(term.RequireTags) == 0 {
				out = append(out, dynamicScoreTermLogRecord{
					Seq:              len(out) + 1,
					CandidateIndex:   candidate.Index,
					Rank:             rank + 1,
					Provider:         candidate.Target.Provider,
					Model:            candidate.Target.Model,
					Dialect:          candidate.Target.Dialect,
					TermName:         defaultString(term.Name, "default"),
					FinalScore:       finiteScore(candidate.Final),
					FinalScoreBucket: scoreValueBucket(candidate.Final),
					ObservationCount: candidate.Stats.Count,
					Selected:         rank == 0,
				})
				continue
			}
			for name, weight := range weights {
				scoreName := canonicalScoreName(name)
				value := candidate.Scores[scoreName]
				contribution := weight * value
				out = append(out, dynamicScoreTermLogRecord{
					Seq:                len(out) + 1,
					CandidateIndex:     candidate.Index,
					Rank:               rank + 1,
					Provider:           candidate.Target.Provider,
					Model:              candidate.Target.Model,
					Dialect:            candidate.Target.Dialect,
					TermName:           defaultString(term.Name, "default"),
					ScoreName:          scoreName,
					Weight:             weight,
					Value:              finiteScore(value),
					Contribution:       finiteScore(contribution),
					FinalScore:         finiteScore(candidate.Final),
					ValueBucket:        scoreValueBucket(value),
					ContributionBucket: scoreValueBucket(contribution),
					FinalScoreBucket:   scoreValueBucket(candidate.Final),
					ObservationCount:   candidate.Stats.Count,
					Selected:           rank == 0,
				})
			}
			for _, tag := range term.PreferTags {
				if !targetHasTag(candidate.Target, tag) {
					continue
				}
				out = append(out, dynamicScoreTermLogRecord{
					Seq:                len(out) + 1,
					CandidateIndex:     candidate.Index,
					Rank:               rank + 1,
					Provider:           candidate.Target.Provider,
					Model:              candidate.Target.Model,
					Dialect:            candidate.Target.Dialect,
					TermName:           defaultString(term.Name, "default"),
					ScoreName:          "prefer_tag",
					Weight:             0.05,
					Value:              1,
					Contribution:       0.05,
					FinalScore:         finiteScore(candidate.Final),
					ValueBucket:        scoreValueBucket(1),
					ContributionBucket: scoreValueBucket(0.05),
					FinalScoreBucket:   scoreValueBucket(candidate.Final),
					ObservationCount:   candidate.Stats.Count,
					Selected:           rank == 0,
				})
			}
		}
		_ = groupName
	}
	return out
}

func finiteScore(value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return value
}

func scoreValueBucket(value float64) string {
	value = finiteScore(value)
	switch {
	case value < 0:
		return "negative"
	case value < 0.25:
		return "very_low"
	case value < 0.50:
		return "low"
	case value < 0.75:
		return "medium"
	case value < 1.00:
		return "high"
	default:
		return "top"
	}
}

func dynamicSafeShape(req *IRRequest) map[string]any {
	shape := map[string]any{
		"tools_present":    len(req.Tools) > 0,
		"image_present":    requestHasImages(req),
		"stream":           req.Stream,
		"max_token_bucket": maxTokenBucket(req.MaxTokens),
		"context_bucket":   contextBucket(estimateTokens(req)),
	}
	if summary := buildReasoningSummary(req.Reasoning); summary != nil {
		shape["reasoning"] = summary
	}
	return shape
}

func dynamicPromptFeatures(req *IRRequest, cfg DynamicSignalPromptFeatures) map[string]bool {
	if !cfg.Enabled {
		return map[string]bool{}
	}
	maxBytes := cfg.MaxScanBytes
	if maxBytes <= 0 {
		maxBytes = 16384
	}
	text := boundedRequestText(req, maxBytes)
	features := map[string]bool{}
	for _, feature := range cfg.Features {
		switch feature {
		case "code":
			features[feature] = strings.Contains(text, "```") || strings.Contains(text, "func ") || strings.Contains(text, "class ")
		case "diff":
			features[feature] = strings.Contains(text, "diff --git") || strings.Contains(text, "@@")
		case "stack_trace":
			features[feature] = strings.Contains(text, "stack trace") || strings.Contains(text, "traceback")
		case "summarize":
			features[feature] = strings.Contains(text, "summarize") || strings.Contains(text, "summary")
		case "extract":
			features[feature] = strings.Contains(text, "extract") || strings.Contains(text, "parse")
		case "translate":
			features[feature] = strings.Contains(text, "translate")
		case "security_review":
			features[feature] = strings.Contains(text, "security") || strings.Contains(text, "vulnerability")
		case "tool_agent":
			features[feature] = len(req.Tools) > 0
		}
	}
	return features
}

func boundedRequestText(req *IRRequest, maxBytes int) string {
	var b strings.Builder
	add := func(s string) {
		if b.Len() >= maxBytes {
			return
		}
		remaining := maxBytes - b.Len()
		if len(s) > remaining {
			s = s[:remaining]
		}
		b.WriteString(" ")
		b.WriteString(strings.ToLower(s))
	}
	add(req.System)
	add(req.Input)
	for _, part := range req.InputParts {
		if part.Type == "text" {
			add(part.Text)
		}
	}
	for _, msg := range req.Messages {
		add(msg.Content)
		for _, part := range msg.Parts {
			if part.Type == "text" {
				add(part.Text)
			}
		}
	}
	return b.String()
}

func dynamicComplexityScore(req *IRRequest, cfg DynamicScoreConfig) float64 {
	score := 0.0
	tokens := estimateTokens(req)
	switch {
	case tokens > 12000:
		score += 0.5
	case tokens > 3000:
		score += 0.3
	case tokens > 800:
		score += 0.15
	}
	score += math.Min(float64(len(req.Tools))*0.08, 0.24)
	score += math.Min(float64(requestImageCount(req))*0.10, 0.20)
	if req.MaxTokens > 4096 {
		score += 0.15
	}
	features := dynamicPromptFeatures(req, cfg.Signals.PromptFeatures)
	for _, enabled := range features {
		if enabled {
			score += 0.05
		}
	}
	return clamp01(score)
}

func dynamicComplexityBucket(score float64) string {
	switch {
	case score >= 0.66:
		return "complex"
	case score >= 0.33:
		return "standard"
	default:
		return "simple"
	}
}

func configuredWeightOrder(candidates []dynamicCandidate) []dynamicCandidate {
	out := append([]dynamicCandidate(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Target.Weight != out[j].Target.Weight {
			return out[i].Target.Weight > out[j].Target.Weight
		}
		return out[i].Index < out[j].Index
	})
	return out
}

func dynamicFallbacks(candidates []dynamicCandidate) []Target {
	out := make([]Target, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Target)
	}
	return out
}

func configuredWeightScores(candidates []dynamicCandidate) []float64 {
	weights := make([]float64, len(candidates))
	for i, candidate := range candidates {
		weight := candidate.Target.Weight
		if weight <= 0 {
			weight = 1
		}
		weights[i] = float64(weight)
	}
	return normalizePositiveSet(weights)
}

func dynamicObservationKey(group, provider, model string) string {
	return group + "\x00" + provider + "\x00" + model
}

func dynamicTargetCost(target Target) float64 {
	return target.InputPricePerMillionUSD + target.OutputPricePerMillionUSD
}

func dynamicEvalScore(target Target, evals []DynamicEvaluationTarget) float64 {
	for _, eval := range evals {
		if eval.Provider == target.Provider && eval.Model == target.Model {
			if eval.QualityScore > 0 {
				return clamp01(eval.QualityScore)
			}
			return clamp01(eval.PassRate)
		}
	}
	if target.Validation != nil {
		if target.Validation.QualityScore > 0 {
			return clamp01(target.Validation.QualityScore)
		}
		if target.Validation.PassRate > 0 {
			return clamp01(target.Validation.PassRate)
		}
	}
	return 0.5
}

func targetHasAllTags(target Target, tags []string) bool {
	for _, tag := range tags {
		if !targetHasTag(target, tag) {
			return false
		}
	}
	return true
}

func targetHasTag(target Target, tag string) bool {
	for _, candidate := range target.Tags {
		if candidate == tag {
			return true
		}
	}
	return false
}

func normalizePositive(value float64, values []float64) float64 {
	minV, maxV, ok := minMaxPositive(values)
	if !ok {
		return 0.5
	}
	if value <= 0 {
		return 0
	}
	if maxV == minV {
		return 0.5
	}
	return clamp01((value - minV) / (maxV - minV))
}

func normalizePositiveSet(values []float64) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = normalizePositive(value, values)
	}
	return out
}

func inverseNormalize(value float64, values []float64) float64 {
	minV, maxV, ok := minMaxPositive(values)
	if !ok {
		return 0.5
	}
	if value <= 0 {
		return 0.5
	}
	if maxV == minV {
		return 0.5
	}
	return clamp01(1 - ((value - minV) / (maxV - minV)))
}

func minMaxPositive(values []float64) (float64, float64, bool) {
	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, value := range values {
		if value <= 0 {
			continue
		}
		minV = math.Min(minV, value)
		maxV = math.Max(maxV, value)
	}
	return minV, maxV, !math.IsInf(minV, 1)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(values)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func avgOrZero(total, count float64) float64 {
	if count == 0 {
		return 0
	}
	return total / count
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func canonicalScoreName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if !strings.HasSuffix(name, "_score") {
		name += "_score"
	}
	return name
}

func roundScore(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	return math.Round(v*1000) / 1000
}

func maxTokenBucket(maxTokens int) string {
	switch {
	case maxTokens <= 0:
		return "omitted"
	case maxTokens <= 256:
		return "tiny"
	case maxTokens <= 2048:
		return "standard"
	default:
		return "large"
	}
}

func contextBucket(tokens int) string {
	switch {
	case tokens > 12000:
		return "huge"
	case tokens > 3000:
		return "large"
	case tokens > 800:
		return "medium"
	default:
		return "small"
	}
}

func complexityRank(bucket string) int {
	switch bucket {
	case "simple":
		return 0
	case "standard":
		return 1
	case "complex":
		return 2
	default:
		return 1
	}
}
