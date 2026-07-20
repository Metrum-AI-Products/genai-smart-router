package router

import "testing"

func TestValidateIntelligentSelectionAcceptsOnlyEligibleOpaqueCandidates(t *testing.T) {
	candidates := intelligentCandidates([]Target{{Provider: "provider-a", Model: "model-a"}, {Provider: "provider-b", Model: "model-b"}})
	cfg := IntelligentRoutingConfig{SchemaVersion: "v1", ConfidenceThreshold: 0.7}
	selected, fallbacks, err := validateIntelligentSelection(cfg, candidates, intelligentSelection{
		SchemaVersion: "v1", CandidateID: "candidate-0002", FallbackIDs: []string{"candidate-0001"}, Confidence: 0.7,
		ClassLabel: "workload:tools", ReasonCodes: []string{"tool-heavy", "cost-aware"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Target.Model != "model-b" || len(fallbacks) != 1 || fallbacks[0].Target.Model != "model-a" {
		t.Fatalf("selection = %#v, fallbacks = %#v", selected, fallbacks)
	}
	for _, candidate := range candidates {
		if candidate.ID == candidate.Target.Provider || candidate.ID == candidate.Target.Model {
			t.Fatalf("candidate ID leaks target identity: %#v", candidate)
		}
	}
}

func TestValidateIntelligentSelectionRejectsUnsafeOrInvalidOutput(t *testing.T) {
	candidates := intelligentCandidates([]Target{{Model: "first"}, {Model: "second"}})
	cfg := IntelligentRoutingConfig{SchemaVersion: "v1", ConfidenceThreshold: 0.8}
	base := intelligentSelection{SchemaVersion: "v1", CandidateID: "candidate-0001", Confidence: 0.8, ClassLabel: "workload:normal", ReasonCodes: []string{"normal"}}
	for name, mutate := range map[string]func(*intelligentSelection){
		"wrong schema":       func(s *intelligentSelection) { s.SchemaVersion = "v2" },
		"unknown candidate":  func(s *intelligentSelection) { s.CandidateID = "provider-a:model-a" },
		"duplicate fallback": func(s *intelligentSelection) { s.FallbackIDs = []string{"candidate-0002", "candidate-0002"} },
		"selected fallback":  func(s *intelligentSelection) { s.FallbackIDs = []string{"candidate-0001"} },
		"low confidence":     func(s *intelligentSelection) { s.Confidence = 0.79 },
		"missing class":      func(s *intelligentSelection) { s.ClassLabel = "" },
		"unsafe class":       func(s *intelligentSelection) { s.ClassLabel = "user said secret" },
		"unsafe reason":      func(s *intelligentSelection) { s.ReasonCodes = []string{"prompt contains secret"} },
	} {
		t.Run(name, func(t *testing.T) {
			selection := base
			mutate(&selection)
			if _, _, err := validateIntelligentSelection(cfg, candidates, selection); err == nil {
				t.Fatal("validateIntelligentSelection() error = nil")
			}
		})
	}
}
