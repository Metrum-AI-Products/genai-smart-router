// Copyright 2026 Metrum AI
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
		{Hostname: "engg.example.com", TLSSecretName: "example-alias-tls"},
		{Hostname: "router.example.com", TLSSecretName: "example-alias-tls"},
	}
	got, err := validateApprovedAliasHostnames(aliases, "apps.example.test")
	if err != nil {
		t.Fatalf("validateApprovedAliasHostnames: %v", err)
	}
	if len(got) != 2 || got[0].Hostname != "engg.example.com" {
		t.Fatalf("normalized aliases = %+v", got)
	}
	if _, err := validateApprovedAliasHostnames([]TenantDeploymentHostnameAlias{
		{Hostname: "llm-api.apps.example.test", TLSSecretName: "wildcard-tls"},
	}, "apps.example.test"); err == nil || !strings.Contains(err.Error(), "hostname suffix") {
		t.Fatalf("suffix alias accepted: %v", err)
	}
}

func TestBuildTenantDeploymentPlanIncludesAliasHostnames(t *testing.T) {
	t.Parallel()
	profile, manifest, _ := tenantDeploymentFixture(t)
	profile.ApprovedAliasHostnames = []TenantDeploymentHostnameAlias{
		{Hostname: "engg.example.com", TLSSecretName: "example-alias-tls"},
	}
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-alias")
	if err != nil {
		t.Fatalf("BuildTenantDeploymentPlan: %v", err)
	}
	if len(plan.AliasHostnames) != 1 || plan.AliasHostnames[0].Hostname != "engg.example.com" {
		t.Fatalf("plan aliases = %+v", plan.AliasHostnames)
	}
}

func TestTenantDeploymentIngressSpecGroupsTLSBySecret(t *testing.T) {
	t.Parallel()
	spec := tenantDeploymentIngressSpec("nginx", []TenantDeploymentHostnameAlias{
		{Hostname: "llm-api.apps.example.test", TLSSecretName: "apps-example-wildcard-tls"},
		{Hostname: "engg.example.com", TLSSecretName: "example-alias-tls"},
		{Hostname: "router.example.com", TLSSecretName: "example-alias-tls"},
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
		case "apps-example-wildcard-tls":
			wildcard = &spec.TLS[i]
		case "example-alias-tls":
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
