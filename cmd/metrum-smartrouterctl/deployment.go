package main

import (
	"context"
	"flag"
	"io"
	"os"
	"strings"
	"time"

	"smart-llmrouter/internal/router"
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
	fs.Parse(args)
	requireJSONOutput(*flags.output)
	plan := loadDeploymentPlan(flags, os.Stdin)
	store, err := router.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	_, adapters := router.NewFakeTenantDeploymentAdapters()
	engine, err := router.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure local fake deployment engine: %v", err)
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
	writeJSON(status)
}

func deploymentDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	flags := addDeploymentFlags(fs, true)
	confirmFile := fs.String("confirm-file", "", "mode-0600, job-bound, expiring deletion approval JSON file")
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
	store, err := router.OpenTenantDeploymentStore(*flags.registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	_, adapters := router.NewFakeTenantDeploymentAdapters()
	engine, err := router.NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		die("configure local fake deployment engine: %v", err)
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

func isDeploymentStatus(args []string) bool {
	for _, arg := range args {
		if arg == "--job" || strings.HasPrefix(arg, "--job=") || arg == "--profile-ref" || strings.HasPrefix(arg, "--profile-ref=") {
			return true
		}
	}
	return false
}
