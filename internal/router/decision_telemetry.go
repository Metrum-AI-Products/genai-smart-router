package router

import (
	"strings"
	"time"
)

func (s *Service) decisionTelemetryEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Server.DecisionTelemetry.Enabled
}

func (s *Service) recordDecisionShape(rc *requestContext, req *IRRequest, callerDialect string) {
	if !s.decisionTelemetryEnabled() || rc == nil || req == nil {
		return
	}
	addBoolFeature := func(name string, value bool) {
		rc.rec.DecisionShapeFeatures = append(rc.rec.DecisionShapeFeatures, decisionShapeFeatureLogRecord{Seq: len(rc.rec.DecisionShapeFeatures) + 1, Name: name, BoolValue: value})
	}
	addIntFeature := func(name string, value int) {
		rc.rec.DecisionShapeFeatures = append(rc.rec.DecisionShapeFeatures, decisionShapeFeatureLogRecord{Seq: len(rc.rec.DecisionShapeFeatures) + 1, Name: name, IntValue: value})
	}
	addTextFeature := func(name, value string) {
		rc.rec.DecisionShapeFeatures = append(rc.rec.DecisionShapeFeatures, decisionShapeFeatureLogRecord{Seq: len(rc.rec.DecisionShapeFeatures) + 1, Name: name, TextValue: value})
	}
	addTextFeature("caller_dialect", callerDialect)
	addBoolFeature("stream", req.Stream)
	addBoolFeature("has_tools", len(req.Tools) > 0)
	addIntFeature("tools_count", len(req.Tools))
	addBoolFeature("has_images", requestHasImages(req))
	addIntFeature("image_count", requestImageCount(req))
	addBoolFeature("structured_output", requestHasStructuredOutput(req))
	addBoolFeature("max_tokens_set", req.MaxTokens > 0)
	addTextFeature("max_tokens_field", req.MaxTokensField)
	addBoolFeature("cacheable", cacheable(req))
}

func (s *Service) recordEligibilityTelemetry(rc *requestContext, groupName string, group ModelGroup, req *IRRequest, callerDialect string) {
	if !s.decisionTelemetryEnabled() || rc == nil || req == nil {
		return
	}
	cfg := s.cfg.Server.DecisionTelemetry
	maxCandidates := cfg.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 64
	}
	maxReasons := cfg.MaxFilterReasons
	if maxReasons <= 0 {
		maxReasons = 256
	}
	recordCandidates := cfg.RecordCandidates == nil || *cfg.RecordCandidates
	if !recordCandidates {
		return
	}
	for i, target := range group.Targets {
		if len(rc.rec.DecisionCandidates) >= maxCandidates {
			break
		}
		outDialect := targetDialect(s.cfg.Provider[target.Provider], target)
		reasons := s.requestFilterReasons(target, req, callerDialect, outDialect)
		if len(reasons) == 0 {
			reason := s.contractFilterReason(groupName, group, target, req, callerDialect, outDialect)
			if reason != "" {
				reasons = append(reasons, reason)
			}
		}
		candidateIndex := len(rc.rec.DecisionCandidates)
		rc.rec.DecisionCandidates = append(rc.rec.DecisionCandidates, decisionCandidateLogRecord{
			CandidateIndex:   candidateIndex,
			GroupTargetIndex: i,
			Provider:         target.Provider,
			Model:            target.Model,
			ModelRef:         target.ModelRef,
			Dialect:          outDialect,
			Weight:           target.Weight,
			ToolOnly:         target.ToolOnly,
			ContextTokens:    target.ContextTokens,
			InputImage:       targetSupportsInputModalities(target, []string{"image"}),
			OutputImage:      stringSliceContains(defaultModalities(target.OutputModalities), "image"),
			ToolSupport:      targetSupportsTools(target, outDialect),
			ForcedToolChoice: targetSupportsCapability(target, outDialect, "forced_tool_choice", "tool_choice"),
			StructuredOutput: targetSupportsCapability(target, outDialect, "structured_outputs", "json_schema"),
			HonorsMaxTokens:  target.HonorsMaxTokens == nil || *target.HonorsMaxTokens,
			ValidationStatus: decisionCandidateValidationStatus(target.Validation),
			ValidationAge:    validationAgeBucket(target.Validation, time.Now().UTC()),
			Eligible:         len(reasons) == 0,
		})
		for _, reason := range reasons {
			if len(rc.rec.DecisionFilterReasons) >= maxReasons {
				break
			}
			rc.rec.DecisionFilterReasons = append(rc.rec.DecisionFilterReasons, decisionFilterReasonLogRecord{
				Seq:            len(rc.rec.DecisionFilterReasons) + 1,
				CandidateIndex: candidateIndex,
				Stage:          filterReasonStage(reason),
				Reason:         reason,
			})
		}
	}
}

func (s *Service) requestFilterReasons(target Target, req *IRRequest, callerDialect, outDialect string) []string {
	requiredModalities := requestInputModalities(req)
	requiresStructuredOutput := requestHasStructuredOutput(req)
	var reasons []string
	if len(req.Tools) == 0 {
		if target.ToolOnly {
			reasons = append(reasons, "tool-only-target")
		}
	} else {
		if !toolPassthrough(callerDialect, outDialect, req) {
			reasons = append(reasons, "dialect-tool-passthrough")
		}
		if !targetSupportsTools(target, outDialect) {
			reasons = append(reasons, "tool-support")
		}
	}
	for _, modality := range requiredModalities {
		if !targetSupportsInputModalities(target, []string{modality}) {
			reasons = append(reasons, "input-modality-"+safeReasonToken(modality))
		}
	}
	if !targetSupportsStructuredOutput(target, callerDialect, outDialect, requiresStructuredOutput) {
		reasons = append(reasons, "structured-output-support")
	}
	if !targetHonorsExplicitMaxTokens(target, req) {
		reasons = append(reasons, "max-tokens-honored")
	}
	return reasons
}

func (s *Service) contractFilterReason(groupName string, group ModelGroup, target Target, req *IRRequest, callerDialect, outDialect string) string {
	if group.Contract == nil {
		return ""
	}
	stats := dynamicStats{}
	if s != nil && s.observations != nil {
		stats = s.observations.stats(dynamicObservationKey(groupName, target.Provider, target.Model), s.dynamicObservationRetention(groupName))
	}
	return targetPassesContract(group.Contract, target, outDialect, callerDialect, req, stats, time.Now().UTC())
}

func filterReasonStage(reason string) string {
	if strings.HasPrefix(reason, "contract-") {
		return "contract"
	}
	return "request_shape"
}

func (s *Service) recordRoutingDecisionTelemetry(rc *requestContext, dec decision) {
	if !s.decisionTelemetryEnabled() || rc == nil {
		return
	}
	selected := candidateIndexForTarget(rc.rec.DecisionCandidates, dec.Target)
	for i := range rc.rec.DecisionCandidates {
		if rc.rec.DecisionCandidates[i].CandidateIndex == selected {
			rc.rec.DecisionCandidates[i].Selected = true
			break
		}
	}
	rc.rec.RoutingDecisions = append(rc.rec.RoutingDecisions, routingDecisionLogRecord{
		Seq:                    len(rc.rec.RoutingDecisions) + 1,
		Strategy:               dec.Strategy,
		SelectedCandidateIndex: selected,
		Provider:               dec.Target.Provider,
		Model:                  dec.Target.Model,
		Dialect:                targetDialect(s.cfg.Provider[dec.Target.Provider], dec.Target),
		FallbackCount:          len(dec.Fallbacks),
		ClassLabel:             dec.ClassLabel,
	})
	rc.rec.RoutingSignals = append(rc.rec.RoutingSignals, dec.RoutingSignals...)
	rc.rec.DynamicScoreTerms = append(rc.rec.DynamicScoreTerms, dec.DynamicScoreTerms...)
	for i := range dec.PolicyExecutions {
		dec.PolicyExecutions[i].SelectedCandidateIndex = selected
	}
	rc.rec.PolicyExecutions = append(rc.rec.PolicyExecutions, dec.PolicyExecutions...)
}

func (s *Service) recordCacheReasonTelemetry(rc *requestContext, status, reason string, target Target) {
	if !s.decisionTelemetryEnabled() || rc == nil {
		return
	}
	recordCacheReasons := s.cfg.Server.DecisionTelemetry.RecordCacheReasons == nil || *s.cfg.Server.DecisionTelemetry.RecordCacheReasons
	if !recordCacheReasons {
		return
	}
	rc.rec.CacheReasons = append(rc.rec.CacheReasons, cacheReasonLogRecord{
		Seq:            len(rc.rec.CacheReasons) + 1,
		Status:         status,
		Reason:         reason,
		CandidateIndex: candidateIndexForTarget(rc.rec.DecisionCandidates, target),
		Provider:       target.Provider,
		Model:          target.Model,
		Dialect:        targetDialect(s.cfg.Provider[target.Provider], target),
	})
}

func candidateIndexForTarget(candidates []decisionCandidateLogRecord, target Target) int {
	for _, candidate := range candidates {
		if candidate.Provider == target.Provider && candidate.Model == target.Model && candidate.ModelRef == target.ModelRef {
			return candidate.CandidateIndex
		}
	}
	return -1
}

func decisionCandidateValidationStatus(validation *TargetValidation) string {
	if validation == nil {
		return "missing"
	}
	return defaultString(strings.ToLower(strings.TrimSpace(validation.Status)), "missing")
}

func cacheBypassReason(req *IRRequest) string {
	if req == nil {
		return "cache-request-missing"
	}
	switch {
	case req.Stream:
		return "cache-streaming"
	case req.NoCache:
		return "cache-request-no-cache"
	case len(req.Tools) > 0:
		return "cache-tool-request"
	case requestHasImages(req):
		return "cache-image-request"
	case requestHasStructuredOutput(req):
		return "cache-structured-output"
	case req.Temperature != nil && *req.Temperature > 0:
		return "cache-temperature"
	default:
		return "cache-eligible"
	}
}

func safeReasonToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
