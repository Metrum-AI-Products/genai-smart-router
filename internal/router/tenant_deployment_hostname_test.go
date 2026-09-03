// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
)

func TestValidateApprovedAliasHostnames(t *testing.T) {
	t.Parallel()
	aliases := []TenantDeploymentHostnameAlias{
		{Hostname: "llm-api-engg.metrum.ai", TLSSecretName: "llm-api-metrum-ai-tls"},
		{Hostname: "llm-api.metrum.ai", TLSSecretName: "llm-api-metrum-ai-tls"},
	}
	got, err := validateApprovedAliasHostnames(aliases, "apps.metrum.ai")
	if err != nil {
		t.Fatalf("validateApprovedAliasHostnames: %v", err)
	}
	if len(got) != 2 || got[0].Hostname != "llm-api-engg.metrum.ai" {
		t.Fatalf("normalized aliases = %+v", got)
	}
	if _, err := validateApprovedAliasHostnames([]TenantDeploymentHostnameAlias{
		{Hostname: "llm-api.apps.metrum.ai", TLSSecretName: "wildcard-tls"},
	}, "apps.metrum.ai"); err == nil || !strings.Contains(err.Error(), "hostname suffix") {
		t.Fatalf("suffix alias accepted: %v", err)
	}
}

func TestBuildTenantDeploymentPlanIncludesAliasHostnames(t *testing.T) {
	t.Parallel()
	profile, manifest, _ := tenantDeploymentFixture(t)
	profile.ApprovedAliasHostnames = []TenantDeploymentHostnameAlias{
		{Hostname: "llm-api-engg.metrum.ai", TLSSecretName: "llm-api-metrum-ai-tls"},
	}
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-alias")
	if err != nil {
		t.Fatalf("BuildTenantDeploymentPlan: %v", err)
	}
	if len(plan.AliasHostnames) != 1 || plan.AliasHostnames[0].Hostname != "llm-api-engg.metrum.ai" {
		t.Fatalf("plan aliases = %+v", plan.AliasHostnames)
	}
}

func TestTenantDeploymentIngressSpecGroupsTLSBySecret(t *testing.T) {
	t.Parallel()
	spec := tenantDeploymentIngressSpec("nginx", []TenantDeploymentHostnameAlias{
		{Hostname: "llm-api.apps.metrum.ai", TLSSecretName: "apps-metrum-ai-wildcard-tls"},
		{Hostname: "llm-api-engg.metrum.ai", TLSSecretName: "llm-api-metrum-ai-tls"},
		{Hostname: "llm-api.metrum.ai", TLSSecretName: "llm-api-metrum-ai-tls"},
	}, "router")
	if len(spec.Rules) != 3 {
		t.Fatalf("rules = %d", len(spec.Rules))
	}
	if len(spec.TLS) != 2 {
		t.Fatalf("tls entries = %+v", spec.TLS)
	}
	var wildcard, exact *networkingv1.IngressTLS
	for i := range spec.TLS {
		switch spec.TLS[i].SecretName {
		case "apps-metrum-ai-wildcard-tls":
			wildcard = &spec.TLS[i]
		case "llm-api-metrum-ai-tls":
			exact = &spec.TLS[i]
		}
	}
	if wildcard == nil || len(wildcard.Hosts) != 1 {
		t.Fatalf("wildcard tls = %+v", wildcard)
	}
	if exact == nil || len(exact.Hosts) != 2 {
		t.Fatalf("exact tls = %+v", exact)
	}
}
