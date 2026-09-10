// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"github.com/metrum-ai/router/internal/fleet"
	"strings"
	"time"
)

type deploymentCommandFlags struct {
	intent   *string
	registry *string
	output   *string
}

type deploymentInput struct {
	plan       fleet.TenantDeploymentPlan
	profile    fleet.TenantDeploymentProfile
	profileRef string
	intentID   string
}

func addDeploymentFlags(fs *flag.FlagSet) deploymentCommandFlags {
	return deploymentCommandFlags{
		intent:   fs.String("intent", "", "mode-0600, signed, reference-only Fleet deployment intent JSON file"),
		registry: fs.String("registry", "tenant-deployments.sqlite", "private local deployment lifecycle SQLite path"),
		output:   fs.String("output", "json", "safe output format (json)"),
	}
}

func requireJSONOutput(value string) {
	if value != "json" {
		die("output must be json")
	}
}

func requireProtectedFleetProfileReference(value string) {
	if strings.TrimSpace(value) == "" {
		die("profile-ref is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(value), "aws-ssm:///") {
		die("live Fleet lifecycle requires a protected aws-ssm profile reference")
	}
}

func loadDeploymentInput(flags deploymentCommandFlags) deploymentInput {
	if strings.TrimSpace(*flags.intent) == "" {
		die("intent is required")
	}
	intent, profile, err := fleet.LoadTenantDeploymentIntent(*flags.intent, time.Now().UTC())
	if err != nil {
		die("load signed deployment intent: %v", err)
	}
	plan, err := fleet.BuildTenantDeploymentPlan(profile, intent.Manifest, intent.IntentID)
	if err != nil {
		die("build deployment plan: %v", err)
	}
	return deploymentInput{plan: plan, profile: profile, profileRef: intent.ProfileRef, intentID: intent.IntentID}
}

func deploymentPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	flags := addDeploymentFlags(fs)
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	input := loadDeploymentInput(flags)
	writeJSON(input.plan)
}

func deploymentDeploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	flags := addDeploymentFlags(fs)
	rdsAdmissionFile := fs.String("rds-admission-file", "", "mode-0600, externally issued, expiring dedicated-RDS disposable-E2E admission JSON file")
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	input := loadDeploymentInput(flags)
	requireProtectedFleetProfileReference(input.profileRef)
	_, adapters, err := deploymentAdaptersForPlan(context.Background(), input.profile, input.profileRef, input.plan, *rdsAdmissionFile, input.plan.DatabaseID != "")
	if err != nil {
		die("configure AWS/EKS deployment adapters: %v", err)
	}
	store, err := fleet.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	engine, err := fleet.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure deployment engine: %v", err)
	}
	status, err := engine.Deploy(context.Background(), input.plan, input.intentID)
	if err != nil {
		writeJSON(status)
		die("deploy failed: %v", err)
	}
	writeJSON(status)
}

func deploymentStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	profileRef := fs.String("profile-ref", "", "protected deployment profile reference")
	jobID := fs.String("job", "", "exact deployment job identifier")
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local deployment lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	if *profileRef == "" || *jobID == "" {
		die("profile-ref and job are required for deployment status")
	}
	requireProtectedFleetProfileReference(*profileRef)
	profile, err := fleet.LoadTenantDeploymentProfile(*profileRef)
	if err != nil {
		die("load protected profile: %v", err)
	}
	store, err := fleet.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	status, err := store.Status(context.Background(), *jobID, profile.ProfileID)
	if err != nil {
		die("read deployment status: %v", err)
	}
	status, err = fleet.ObserveTenantDeployment(context.Background(), profile, status)
	if err != nil {
		die("observe EKS deployment status: %v", err)
	}
	writeJSON(status)
}

func deploymentDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	flags := addDeploymentFlags(fs)
	confirmFile := fs.String("confirm-file", "", "mode-0600, job-bound, expiring deletion approval JSON file")
	rdsAdmissionFile := fs.String("rds-admission-file", "", "mode-0600, externally issued, expiring dedicated-RDS disposable-E2E admission JSON file")
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	if *confirmFile == "" {
		die("confirm-file is required")
	}
	input := loadDeploymentInput(flags)
	requireProtectedFleetProfileReference(input.profileRef)
	approval, approvalSHA256, err := fleet.LoadTenantDeletionApproval(*confirmFile, input.profile, time.Now().UTC())
	if err != nil {
		die("load deletion approval: %v", err)
	}
	requireRDSAdmission := input.plan.DatabaseID != "" && !approval.RetainDatabase
	_, adapters, err := deploymentAdaptersForPlan(context.Background(), input.profile, input.profileRef, input.plan, *rdsAdmissionFile, requireRDSAdmission)
	if err != nil {
		die("configure AWS/EKS deployment adapters: %v", err)
	}
	store, err := fleet.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	engine, err := fleet.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure deployment engine: %v", err)
	}
	status, err := engine.Delete(context.Background(), input.plan, approval, approvalSHA256)
	if err != nil {
		if status.JobID != "" {
			writeJSON(status)
		}
		die("delete failed: %v", err)
	}
	writeJSON(status)
}

func deploymentAdaptersForPlan(ctx context.Context, profile fleet.TenantDeploymentProfile, profileRef string, plan fleet.TenantDeploymentPlan, admissionFile string, requireRDSAdmission bool) (*fleet.EKSTenantDeploymentAdapters, fleet.TenantDeploymentAdapters, error) {
	if plan.DatabaseID == "" {
		if strings.TrimSpace(admissionFile) != "" {
			return nil, fleet.TenantDeploymentAdapters{}, errors.New("rds-admission-file is only valid for a dedicated RDS deployment")
		}
		return fleet.NewEKSTenantDeploymentAdapters(ctx, profile)
	}
	if !requireRDSAdmission {
		if strings.TrimSpace(admissionFile) != "" {
			return nil, fleet.TenantDeploymentAdapters{}, errors.New("rds-admission-file is not needed when the dedicated RDS is retained")
		}
		return fleet.NewEKSTenantDeploymentAdapters(ctx, profile)
	}
	if strings.TrimSpace(admissionFile) == "" {
		return nil, fleet.TenantDeploymentAdapters{}, errors.New("rds-admission-file is required for dedicated RDS mutation")
	}
	if !strings.HasPrefix(profileRef, "aws-ssm:///") {
		return nil, fleet.TenantDeploymentAdapters{}, errors.New("dedicated RDS mutation requires a protected aws-ssm profile reference")
	}
	admission, _, err := fleet.LoadTenantDeploymentRDSAdmission(admissionFile, profile, plan, time.Now().UTC())
	if err != nil {
		return nil, fleet.TenantDeploymentAdapters{}, errors.New("load dedicated RDS admission")
	}
	return fleet.NewApprovedEKSTenantDeploymentAdapters(ctx, profile, admission)
}

func fleetDatabases(args []string) {
	if len(args) == 0 || args[0] != "status" {
		die("usage: metrum-genai-smartrouter-fleetctl databases status --profile-ref REF --job ID --registry PATH")
	}
	deploymentStatus(args[1:])
}

func fleetSmoke(args []string) {
	if len(args) < 2 || args[0] != "run" || args[1] != "activation" {
		die("usage: metrum-genai-smartrouter-fleetctl smoke run activation --profile-ref REF --job ID --registry PATH")
	}
	deploymentStatus(args[2:])
}
