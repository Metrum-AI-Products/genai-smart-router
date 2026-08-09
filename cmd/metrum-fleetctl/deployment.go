package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"smart-llmrouter/internal/router"
	"strings"
	"time"
)

type deploymentCommandFlags struct {
	profileRef *string
	manifest   *string
	intentID   *string
	registry   *string
	output     *string
}

func addDeploymentFlags(fs *flag.FlagSet, requireManifest bool) deploymentCommandFlags {
	manifestDefault := ""
	if requireManifest {
		manifestDefault = "-"
	}
	return deploymentCommandFlags{
		profileRef: fs.String("profile-ref", "", "protected deployment profile reference (local fake mode accepts file:// only)"),
		manifest:   fs.String("manifest", manifestDefault, "strict reference-only deployment manifest path or - for stdin"),
		intentID:   fs.String("intent-id", "", "caller-supplied idempotency intent identifier"),
		registry:   fs.String("registry", "tenant-deployments.sqlite", "private local deployment lifecycle SQLite path"),
		output:     fs.String("output", "json", "safe output format (json)"),
	}
}

func requireJSONOutput(value string) {
	if value != "json" {
		die("output must be json")
	}
}

func loadDeploymentPlan(flags deploymentCommandFlags, stdin io.Reader) router.TenantDeploymentPlan {
	if strings.TrimSpace(*flags.profileRef) == "" {
		die("profile-ref is required")
	}
	if strings.TrimSpace(*flags.manifest) == "" {
		die("manifest is required")
	}
	profile, err := router.LoadTenantDeploymentProfile(*flags.profileRef)
	if err != nil {
		die("load protected profile: %v", err)
	}
	manifest, err := router.LoadTenantDeploymentManifest(*flags.manifest, stdin)
	if err != nil {
		die("load deployment manifest: %v", err)
	}
	plan, err := router.BuildTenantDeploymentPlan(profile, manifest, *flags.intentID)
	if err != nil {
		die("build deployment plan: %v", err)
	}
	return plan
}

func deploymentPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	flags := addDeploymentFlags(fs, true)
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	plan := loadDeploymentPlan(flags, os.Stdin)
	writeJSON(plan)
}

func deploymentDeploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	flags := addDeploymentFlags(fs, true)
	rdsAdmissionFile := fs.String("rds-admission-file", "", "mode-0600, externally issued, expiring dedicated-RDS disposable-E2E admission JSON file")
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	plan := loadDeploymentPlan(flags, os.Stdin)
	profile := routerProfileForPlan(flags)
	_, adapters, err := deploymentAdaptersForPlan(context.Background(), profile, plan, *rdsAdmissionFile, plan.DatabaseID != "")
	if err != nil {
		die("configure AWS/EKS deployment adapters: %v", err)
	}
	store, err := router.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	engine, err := router.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure deployment engine: %v", err)
	}
	status, err := engine.Deploy(context.Background(), plan, *flags.intentID)
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
	profile, err := router.LoadTenantDeploymentProfile(*profileRef)
	if err != nil {
		die("load protected profile: %v", err)
	}
	store, err := router.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	status, err := store.Status(context.Background(), *jobID, profile.ProfileID)
	if err != nil {
		die("read deployment status: %v", err)
	}
	observed, err := router.ObserveTenantDeployment(context.Background(), profile, status)
	if err != nil {
		die("observe EKS deployment status: %v", err)
	}
	status.ObservedState = observed
	writeJSON(status)
}

func deploymentDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	flags := addDeploymentFlags(fs, true)
	confirmFile := fs.String("confirm-file", "", "mode-0600, job-bound, expiring deletion approval JSON file")
	rdsAdmissionFile := fs.String("rds-admission-file", "", "mode-0600, externally issued, expiring dedicated-RDS disposable-E2E admission JSON file")
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	if *confirmFile == "" {
		die("confirm-file is required")
	}
	plan := loadDeploymentPlan(flags, os.Stdin)
	approval, approvalSHA256, err := router.LoadTenantDeletionApproval(*confirmFile, time.Now().UTC())
	if err != nil {
		die("load deletion approval: %v", err)
	}
	profile := routerProfileForPlan(flags)
	requireRDSAdmission := plan.DatabaseID != "" && !approval.RetainDatabase
	_, adapters, err := deploymentAdaptersForPlan(context.Background(), profile, plan, *rdsAdmissionFile, requireRDSAdmission)
	if err != nil {
		die("configure AWS/EKS deployment adapters: %v", err)
	}
	store, err := router.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	engine, err := router.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure deployment engine: %v", err)
	}
	status, err := engine.Delete(context.Background(), plan, approval, approvalSHA256)
	if err != nil {
		if status.JobID != "" {
			writeJSON(status)
		}
		die("delete failed: %v", err)
	}
	writeJSON(status)
}

func routerProfileForPlan(flags deploymentCommandFlags) router.TenantDeploymentProfile {
	profile, err := router.LoadTenantDeploymentProfile(*flags.profileRef)
	if err != nil {
		die("load protected profile: %v", err)
	}
	return profile
}

func deploymentAdaptersForPlan(ctx context.Context, profile router.TenantDeploymentProfile, plan router.TenantDeploymentPlan, admissionFile string, requireRDSAdmission bool) (*router.EKSTenantDeploymentAdapters, router.TenantDeploymentAdapters, error) {
	if plan.DatabaseID == "" {
		if strings.TrimSpace(admissionFile) != "" {
			return nil, router.TenantDeploymentAdapters{}, errors.New("rds-admission-file is only valid for a dedicated RDS deployment")
		}
		return router.NewEKSTenantDeploymentAdapters(ctx, profile)
	}
	if !requireRDSAdmission {
		if strings.TrimSpace(admissionFile) != "" {
			return nil, router.TenantDeploymentAdapters{}, errors.New("rds-admission-file is not needed when the dedicated RDS is retained")
		}
		return router.NewEKSTenantDeploymentAdapters(ctx, profile)
	}
	if strings.TrimSpace(admissionFile) == "" {
		return nil, router.TenantDeploymentAdapters{}, errors.New("rds-admission-file is required for dedicated RDS mutation")
	}
	admission, _, err := router.LoadTenantDeploymentRDSAdmission(admissionFile, profile, plan, time.Now().UTC())
	if err != nil {
		return nil, router.TenantDeploymentAdapters{}, errors.New("load dedicated RDS admission")
	}
	return router.NewApprovedEKSTenantDeploymentAdapters(ctx, profile, admission)
}

func fleetDatabases(args []string) {
	if len(args) == 0 || args[0] != "status" {
		die("usage: metrum-fleetctl databases status --profile-ref REF --job ID --registry PATH")
	}
	deploymentStatus(args[1:])
}

func fleetSmoke(args []string) {
	if len(args) < 2 || args[0] != "run" || args[1] != "activation" {
		die("usage: metrum-fleetctl smoke run activation --profile-ref REF --job ID --registry PATH")
	}
	deploymentStatus(args[2:])
}
