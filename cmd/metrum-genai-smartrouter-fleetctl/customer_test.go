package main

import (
	"strings"
	"testing"
)

func TestNormalizeCustomerID(t *testing.T) {
	got, err := normalizeCustomerID("Acme4")
	if err != nil || got != "acme4" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := normalizeCustomerID("1bad"); err == nil {
		t.Fatal("expected invalid leading digit")
	}
	if _, err := normalizeCustomerID("Bad_ID"); err == nil {
		t.Fatal("expected invalid underscore")
	}
	if _, err := normalizeCustomerID("a"); err == nil {
		t.Fatal("expected too-short id")
	}
}

func TestStableCustomerNaming(t *testing.T) {
	if got := customerHostname("acme4"); got != "acme4.apps.metrum.ai" {
		t.Fatalf("hostname=%q", got)
	}
	want := "aws-secretsmanager:///smartrouter/fleet/customers/acme4/runtime-bundle"
	if got := customerBundleRef("acme4"); got != want {
		t.Fatalf("bundle ref=%q want %q", got, want)
	}
}

func TestDeepMergeAndMergeCallers(t *testing.T) {
	base := map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 1}},
		"callers": []any{
			map[string]any{"id": "a", "allow": []any{"high"}, "project": "p1"},
		},
	}
	patch := map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 2}, "fast": map[string]any{"weight": 1}},
		"callers": []any{
			map[string]any{"id": "a", "allow": []any{"high", "fast"}},
			map[string]any{"id": "b", "allow": []any{"default"}, "project": "p2"},
		},
	}
	mergedAny := deepMerge(base, patch)
	merged := asStringMapMust(mergedAny)
	models := asStringMapMust(merged["models"])
	high := asStringMapMust(models["high"])
	if high["weight"] != 2 {
		t.Fatalf("high weight=%v", high["weight"])
	}
	if _, ok := models["fast"]; !ok {
		t.Fatal("expected fast model")
	}
	callers, ok := asAnySlice(merged["callers"])
	if !ok || len(callers) != 2 {
		t.Fatalf("callers=%v", merged["callers"])
	}
	first := asStringMapMust(callers[0])
	if first["id"] != "a" {
		t.Fatalf("first caller id=%v", first["id"])
	}
	allow, ok := asAnySlice(first["allow"])
	if !ok || len(allow) != 2 {
		t.Fatalf("merged allow=%v", first["allow"])
	}
	second := asStringMapMust(callers[1])
	if second["id"] != "b" || second["project"] != "p2" {
		t.Fatalf("second caller=%v", second)
	}

	patched, err := applyConfigPatch("models:\n  high:\n    weight: 1\n", map[string]any{
		"models": map[string]any{"high": map[string]any{"weight": 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if patched == "" || !strings.Contains(patched, "weight: 3") {
		t.Fatalf("patched=%q", patched)
	}
}

func TestPlanSelectsDedicatedRDS(t *testing.T) {
	if planSelectsDedicatedRDS(planPayload{}) {
		t.Fatal("empty plan should be SQLite-safe")
	}
	if !planSelectsDedicatedRDS(planPayload{DatabaseID: "db-1"}) {
		t.Fatal("database_id should refuse")
	}
	if !planSelectsDedicatedRDS(planPayload{DatabaseProfile: "postgres-dedicated-small"}) {
		t.Fatal("database_profile should refuse")
	}
	if !planSelectsDedicatedRDS(planPayload{Actions: []string{"namespace", "dedicated_rds", "router"}}) {
		t.Fatal("dedicated_rds action should refuse")
	}
}
