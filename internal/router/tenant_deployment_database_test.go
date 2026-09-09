// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTenantDeploymentDedicatedRDSPlanFakeRetryAndRetention(t *testing.T) {
	profile, manifest, _ := tenantDeploymentFixture(t)
	profile.DatabaseMode = "dedicated-rds"
	profile.ApprovedDatabaseProfile = "postgres-dedicated-small"
	profile.RDSInstanceClass = "db-t4g-medium"
	profile.RDSStorageGiB = 20
	profile.RDSBackupRetentionDays = 7
	manifest.DatabaseProfile = profile.ApprovedDatabaseProfile
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-rds")
	if err != nil {
		t.Fatal(err)
	}
	if plan.DatabaseID == "" || plan.DatabaseProfile != "postgres-dedicated-small" || strings.Join(plan.Actions, ",") != "namespace,network_policy,dedicated_rds,runtime_secret_binding,license_binding,state_pvc,router,activation,hostname" {
		t.Fatalf("unexpected dedicated RDS plan: %#v", plan)
	}
	store, fake, engine, _ := openTenantDeploymentTestEngine(t)
	defer store.Close()
	fake.FailAction = "dedicated_rds"
	if status, err := engine.Deploy(context.Background(), plan, "intent-rds"); err == nil || status.State != TenantDeploymentFailed || !status.Retryable || status.NextAction != "dedicated_rds" {
		t.Fatalf("RDS failure status=%#v err=%v", status, err)
	}
	fake.FailAction = ""
	status, err := engine.Deploy(context.Background(), plan, "intent-rds")
	if err != nil || status.State != TenantDeploymentReady {
		t.Fatalf("RDS resume status=%#v err=%v", status, err)
	}
	if got := fake.SnapshotCalls(); strings.Join(got[:3], ",") != "ensure:namespace,ensure:network_policy,ensure:dedicated_rds" {
		t.Fatalf("dedicated RDS did not precede secret binding: %v", got)
	}
	approval := TenantDeletionApproval{APIVersion: TenantDeletionApprovalAPIVersion, JobID: plan.JobID, Action: "delete", ExpiresAt: time.Now().UTC().Add(time.Hour), RetainDatabase: true, Nonce: "retain-rds", authenticated: true}
	status, err = engine.Delete(context.Background(), plan, approval, strings.Repeat("a", 64))
	if err != nil || status.State != TenantDeploymentDeleted {
		t.Fatalf("RDS deletion status=%#v err=%v", status, err)
	}
	resources, err := engine.ListResources(context.Background(), plan.JobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		if resource.ResourceKind == "dedicated_rds" && resource.State != "retained" {
			t.Fatalf("RDS state=%q, want retained", resource.State)
		}
	}
}

func TestTenantDeploymentDedicatedRDSProfileDefaultsToSQLiteWithoutManifestSelection(t *testing.T) {
	profile, manifest, _ := tenantDeploymentFixture(t)
	profile.DatabaseMode = "dedicated-rds"
	profile.ApprovedDatabaseProfile = "postgres-dedicated-small"
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-sqlite-default")
	if err != nil {
		t.Fatal(err)
	}
	if plan.DatabaseID != "" || plan.DatabaseProfile != "" || strings.Contains(strings.Join(plan.Actions, ","), "dedicated_rds") {
		t.Fatalf("unselected dedicated RDS profile changed SQLite default: %#v", plan)
	}
}
