// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"smart-llmrouter/internal/fleet"
)

func customerRepair(args []string) {
	fs := flag.NewFlagSet("customer repair", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	intentPath := fs.String("intent", "", "mode-0600 signed intent for redeploy")
	signWithKey := fs.String("sign-with-key", "", "optional lifecycle approval key to sign workspace manifest before redeploy")
	staleMinutes := fs.Int("stale-minutes", 15, "mark in-flight attempts older than this many minutes as retryable failed")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	if strings.TrimSpace(*intentPath) == "" && strings.TrimSpace(*signWithKey) == "" {
		die("customer repair requires --intent or --sign-with-key")
	}
	state := ws.loadState()
	jobID, _ := state["job_id"].(string)
	if strings.TrimSpace(jobID) == "" {
		jobID = loadWorkspacePlan(ws).JobID
	}
	if strings.TrimSpace(jobID) == "" {
		die("repair requires a prior job_id in workspace state or plan.json")
	}
	reconciled, err := reconcileStaleDeploymentAttempts(sharedRegistryPath(), jobID, time.Duration(*staleMinutes)*time.Minute)
	if err != nil {
		die("reconcile registry: %v", err)
	}
	if strings.TrimSpace(*intentPath) == "" {
		*intentPath = signWorkspaceManifest(ws, *signWithKey)
	}
	fleetBin, err := resolveFleetctlBinary()
	if err != nil {
		die("%v", err)
	}
	ctx := context.Background()
	_, fleetEnv, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	plan, status := planAndDeployIntent(ws, fleetBin, *intentPath, fleetEnv)
	fmt.Println(mustJSON(map[string]any{
		"repaired_job_id":     jobID,
		"reconciled_attempts": reconciled,
		"hostname":            plan.Hostname,
		"state":               status["state"],
	}))
}

func repairCustomerJobIfNeeded(ws customerWorkspace, fleetBin, intentPath string, fleetEnv map[string]string, staleMinutes int) {
	state := ws.loadState()
	jobID, _ := state["job_id"].(string)
	if strings.TrimSpace(jobID) == "" {
		jobID = loadWorkspacePlan(ws).JobID
	}
	if strings.TrimSpace(jobID) == "" {
		return
	}
	reconciled, err := reconcileStaleDeploymentAttempts(sharedRegistryPath(), jobID, time.Duration(staleMinutes)*time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resume repair: reconcile registry: %v\n", err)
		return
	}
	if reconciled > 0 {
		fmt.Fprintf(os.Stderr, "resume repair: reconciled %d stale attempt(s) for job %s\n", reconciled, jobID)
		return
	}
	profileRef, _ := state["profile_ref"].(string)
	if strings.TrimSpace(profileRef) == "" {
		return
	}
	status, code := fleetctlJSON(fleetBin, []string{
		"status", "--profile-ref", profileRef, "--job", jobID,
		"--registry", sharedRegistryPath(), "--output", "json",
	}, fleetEnv, true)
	if code != 0 {
		return
	}
	jobState, _ := status["state"].(string)
	if jobState == "failed" || jobState == "operator_required" {
		if n, err := reconcileStaleDeploymentAttempts(sharedRegistryPath(), jobID, 0); err == nil && n >= 0 {
			fmt.Fprintf(os.Stderr, "resume repair: reset retryable job %s from state %s\n", jobID, jobState)
		}
	}
	_ = intentPath
}

func reconcileStaleDeploymentAttempts(registryPath, jobID string, maxAge time.Duration) (int, error) {
	store, err := fleet.OpenTenantDeploymentStore(registryPath)
	if err != nil {
		return 0, err
	}
	defer store.Close()
	_, repaired, err := repairStuckJob(context.Background(), store, jobID, maxAge)
	if err != nil {
		return 0, err
	}
	if repaired {
		return 1, nil
	}
	return 0, nil
}

func repairStuckJob(ctx context.Context, store *fleet.TenantDeploymentStore, jobID string, maxAge time.Duration) (fleet.TenantDeploymentStatus, bool, error) {
	engine, err := fleet.NewTenantDeploymentEngine(store, noopTenantDeploymentAdapters())
	if err != nil {
		return fleet.TenantDeploymentStatus{}, false, err
	}
	return engine.RepairStuckDeployment(ctx, jobID, maxAge)
}

func noopTenantDeploymentAdapters() fleet.TenantDeploymentAdapters {
	noop := tenantDeploymentNoopAdapter{}
	return fleet.TenantDeploymentAdapters{
		Namespace: noop, NetworkPolicy: noop, SecretBinding: noop, LicenseBinding: noop,
		State: noop, Database: noop, Router: noop, Activation: noop, Hostname: noop,
	}
}

type tenantDeploymentNoopAdapter struct{}

func (tenantDeploymentNoopAdapter) EnsureNamespace(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteNamespace(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureNetworkPolicy(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteNetworkPolicy(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureSecretBinding(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteSecretBinding(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureLicenseBinding(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteLicenseBinding(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureStatePVC(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteStatePVC(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureDedicatedRDS(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteDedicatedRDS(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnsureRouter(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteRouter(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) ValidateActivation(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DeleteActivation(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
func (tenantDeploymentNoopAdapter) EnableHostname(context.Context, fleet.TenantDeploymentPlan) (string, error) {
	return "", nil
}
func (tenantDeploymentNoopAdapter) DisableHostname(context.Context, fleet.TenantDeploymentPlan, string) error {
	return nil
}
