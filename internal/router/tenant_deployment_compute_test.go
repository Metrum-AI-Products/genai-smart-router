// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"strings"
	"testing"
)

func TestTenantComputeProfileDefaultsAndApproval(t *testing.T) {
	profile, manifest, plan := tenantDeploymentFixture(t)
	if plan.ComputeProfile != DefaultTenantComputeProfile {
		t.Fatalf("default compute_profile=%q want %q", plan.ComputeProfile, DefaultTenantComputeProfile)
	}
	if plan.NodeClassAlias != "t3a.medium" || plan.Architecture != "amd64" || plan.CPURequest == "" || plan.MemoryRequest == "" {
		t.Fatalf("resolved compute scalars incomplete: %+v", plan)
	}
	if plan.computePolicy.AllowSharedWorkerFallback || len(plan.computePolicy.NodeSelector) == 0 {
		t.Fatalf("default compute profile must fail closed without shared fallback: %+v", plan.computePolicy)
	}

	manifest.ComputeProfile = "t3a.large"
	alt, err := BuildTenantDeploymentPlan(profile, manifest, "intent-compute-alt")
	if err != nil {
		t.Fatal(err)
	}
	if alt.ComputeProfile != "t3a.large" || alt.NodeClassAlias != "t3a.large" {
		t.Fatalf("alternate compute profile not selected: %+v", alt)
	}

	manifest.ComputeProfile = "c6a.large"
	if _, err := BuildTenantDeploymentPlan(profile, manifest, "intent-compute-unknown"); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("expected unapproved compute rejection, got %v", err)
	}
}

func TestTenantComputeProfileRejectsIncompleteProtectedPolicy(t *testing.T) {
	profile, _, _ := tenantDeploymentFixture(t)
	bad := profile.ApprovedComputeProfiles[DefaultTenantComputeProfile]
	bad.NodeSelector = nil
	bad.AllowSharedWorkerFallback = false
	profile.ApprovedComputeProfiles[DefaultTenantComputeProfile] = bad
	raw := []byte(`{"api_version":"metrum.ai/smartrouter-profile/v1"}`)
	if err := validateTenantDeploymentProfile(profile, raw); err == nil || !strings.Contains(err.Error(), "node_selector") {
		t.Fatalf("expected incomplete compute profile rejection, got %v", err)
	}
}

func TestTenantComputeProfileRejectsFreeFormInstanceTypeOnManifest(t *testing.T) {
	unknown := `{"api_version":"metrum.ai/smartrouter-deployment/v1","customer_id":"customer-a","stage":"test","release":"latest-approved","resource_profile":"small","state_profile":"sqlite-rwo-small","instance_type":"t3a.medium","runtime_bundle_ref":"aws-secretsmanager:///safe/runtime-bundle","config_revision":"r1","license":{"request_ref":"aws-ssm:///safe/license","validity":"24h"}}`
	if _, err := decodeStrictAndValidateManifest([]byte(unknown)); err == nil {
		t.Fatal("expected free-form instance_type rejection")
	}
}

func decodeStrictAndValidateManifest(raw []byte) (TenantDeploymentManifest, error) {
	var manifest TenantDeploymentManifest
	if err := decodeStrictDeploymentDocument(raw, "manifest.json", &manifest); err != nil {
		return TenantDeploymentManifest{}, err
	}
	if err := validateTenantDeploymentManifest(manifest, raw); err != nil {
		return TenantDeploymentManifest{}, err
	}
	return manifest, nil
}

func TestTenantDeploymentStatusIncludesProfilesWithoutSecrets(t *testing.T) {
	profile, _, plan := tenantDeploymentFixture(t)
	store, err := OpenTenantDeploymentStore(t.TempDir() + "/registry.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, adapters := NewFakeTenantDeploymentAdapters()
	engine, err := NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		t.Fatal(err)
	}
	status, err := engine.Deploy(context.Background(), plan, "intent-status-profiles")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Status(context.Background(), status.JobID, profile.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResourceProfile != plan.ResourceProfile || got.StateProfile != plan.StateProfile || got.ComputeProfile != plan.ComputeProfile {
		t.Fatalf("status missing profile fields: %+v", got)
	}
	blob := string(mustJSON(t, got))
	for _, forbidden := range []string{"secret", "token", "dsn", "password", "runtime_bundle_ref", "aws-ssm", "aws-secretsmanager"} {
		if strings.Contains(strings.ToLower(blob), forbidden) {
			t.Fatalf("status leaked %q: %s", forbidden, blob)
		}
	}
}
