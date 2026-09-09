// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDerivedFleetOwnershipTransitionFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "smokes", "production-derived", "fleet-ownership-transition-llm-api.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Name                string   `json:"name"`
		SourceIncidentIssue string   `json:"source_incident_issue"`
		CustomerID          string   `json:"customer_id"`
		SourceProfileID     string   `json:"source_profile_id"`
		SourceStage         string   `json:"source_stage"`
		SourceInstanceID    string   `json:"source_instance_id"`
		TargetProfileID     string   `json:"target_profile_id"`
		TargetStage         string   `json:"target_stage"`
		TargetInstanceID    string   `json:"target_instance_id"`
		RequiredFirstAction string   `json:"required_first_action"`
		RequiredSafeFields  []string `json:"required_safe_fields"`
		ForbiddenEvidence   []string `json:"forbidden_evidence"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Name == "" || fixture.SourceIncidentIssue != "#1052" || fixture.RequiredFirstAction != "ownership_transition" {
		t.Fatalf("invalid ownership transition fixture: %#v", fixture)
	}
	if got := TenantDeploymentInstanceID(fixture.SourceProfileID, fixture.CustomerID, fixture.SourceStage); got != fixture.SourceInstanceID {
		t.Fatalf("source instance = %q want %q", got, fixture.SourceInstanceID)
	}
	if got := TenantDeploymentInstanceID(fixture.TargetProfileID, fixture.CustomerID, fixture.TargetStage); got != fixture.TargetInstanceID {
		t.Fatalf("target instance = %q want %q", got, fixture.TargetInstanceID)
	}
	for _, field := range append(fixture.RequiredSafeFields, fixture.ForbiddenEvidence...) {
		if strings.TrimSpace(field) == "" {
			t.Fatal("fixture lists an empty evidence field")
		}
	}
}

func TestProductionDerivedFleetAdminReportsProxyTrustGuard(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "smokes", "production-derived", "admin-reports-forwarded-https-trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		UntrustedProxyCIDR string `json:"untrusted_proxy_cidr"`
		TrustedProxyCIDR   string `json:"trusted_proxy_cidr"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	bundleConfig := func(trustedCIDR string) string {
		return "server:\n  admin_auth:\n    basic:\n      enabled: true\n      allow_insecure_http: false\n      trusted_proxy_cidrs:\n        - " + trustedCIDR +
			"\n    authorization:\n      enabled: true\n  admin_reports:\n    enabled: true\n    path_prefix: /admin/reports\n"
	}
	approved := []string{fixture.TrustedProxyCIDR}
	if err := validateRequiredAdminReports(bundleConfig(fixture.UntrustedProxyCIDR), approved); err == nil {
		t.Fatal("Fleet guard accepted a bundle that does not trust the deployment reverse proxy")
	}
	if err := validateRequiredAdminReports(bundleConfig(fixture.TrustedProxyCIDR), approved); err != nil {
		t.Fatalf("Fleet guard rejected a correctly trusted bundle: %v", err)
	}
}
