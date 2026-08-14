package main

import (
	"context"
	"flag"
	"smart-llmrouter/internal/router"
	"strings"
)

func fleetTenants(args []string) {
	if len(args) == 0 {
		die("usage: metrum-genai-smartrouter-fleetctl tenants <list|get|sync> [flags]")
	}
	switch args[0] {
	case "list":
		fleetTenantsList(args[1:])
	case "get":
		fleetTenantsGet(args[1:])
	case "sync":
		fleetTenantsSync(args[1:])
	default:
		die("unsupported tenants command %q", args[0])
	}
}

func fleetLicenses(args []string) {
	if len(args) == 0 {
		die("usage: metrum-genai-smartrouter-fleetctl licenses <list|get|register> [flags]")
	}
	switch args[0] {
	case "list":
		fleetLicensesList(args[1:])
	case "get":
		fleetLicensesGet(args[1:])
	case "register":
		fleetLicensesRegister(args[1:])
	default:
		die("unsupported licenses command %q", args[0])
	}
}

func fleetTenantsList(args []string) {
	fs := flag.NewFlagSet("tenants list", flag.ExitOnError)
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	store, err := router.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	tenants, err := store.ListFleetTenants(context.Background())
	if err != nil {
		die("list fleet tenants: %v", err)
	}
	writeJSON(map[string]any{
		"schema":  "metrum.ai/smartrouter-fleet-tenant-list/v1",
		"tenants": tenants,
	})
}

func fleetTenantsGet(args []string) {
	fs := flag.NewFlagSet("tenants get", flag.ExitOnError)
	customerID := fs.String("customer-id", "", "exact customer_id")
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	if strings.TrimSpace(*customerID) == "" {
		die("customer-id is required")
	}
	store, err := router.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	tenant, err := store.GetFleetTenant(context.Background(), *customerID)
	if err != nil {
		die("get fleet tenant: %v", err)
	}
	writeJSON(tenant)
}

func fleetTenantsSync(args []string) {
	fs := flag.NewFlagSet("tenants sync", flag.ExitOnError)
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	store, err := router.OpenTenantDeploymentStore(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	count, err := store.SyncFleetInventoryFromJobs(context.Background())
	if err != nil {
		die("sync fleet inventory: %v", err)
	}
	tenants, err := store.ListFleetTenants(context.Background())
	if err != nil {
		die("list fleet tenants: %v", err)
	}
	writeJSON(map[string]any{
		"schema":      "metrum.ai/smartrouter-fleet-tenant-sync/v1",
		"jobs_synced": count,
		"tenants":     tenants,
	})
}

func fleetLicensesList(args []string) {
	fs := flag.NewFlagSet("licenses list", flag.ExitOnError)
	customerID := fs.String("customer-id", "", "optional customer_id filter")
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	store, err := router.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	licenses, err := store.ListFleetLicenses(context.Background(), *customerID)
	if err != nil {
		die("list fleet licenses: %v", err)
	}
	writeJSON(map[string]any{
		"schema":   "metrum.ai/smartrouter-fleet-license-list/v1",
		"licenses": licenses,
	})
}

func fleetLicensesGet(args []string) {
	fs := flag.NewFlagSet("licenses get", flag.ExitOnError)
	licenseID := fs.String("license-id", "", "exact license_id")
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	if strings.TrimSpace(*licenseID) == "" {
		die("license-id is required")
	}
	store, err := router.OpenTenantDeploymentStoreReadOnly(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	license, err := store.GetFleetLicense(context.Background(), *licenseID)
	if err != nil {
		die("get fleet license: %v", err)
	}
	writeJSON(license)
}

func fleetLicensesRegister(args []string) {
	fs := flag.NewFlagSet("licenses register", flag.ExitOnError)
	summaryFile := fs.String("summary-file", "", "mode-0600 router-license safe-summary JSON")
	payloadSHA := fs.String("payload-sha256", "", "optional 64-hex digest of the signed license payload")
	actorRole := fs.String("actor-role", "fleet-lifecycle-admin", "safe actor role for the inventory event")
	registry := fs.String("registry", "tenant-deployments.sqlite", "private local Fleet lifecycle SQLite path")
	output := fs.String("output", "json", "safe output format (json)")
	fs.Parse(args)
	requireJSONOutput(*output)
	if strings.TrimSpace(*summaryFile) == "" {
		die("summary-file is required")
	}
	summary, err := router.LoadFleetLicenseSafeSummaryFile(*summaryFile)
	if err != nil {
		die("load license summary: %v", err)
	}
	store, err := router.OpenTenantDeploymentStore(*registry)
	if err != nil {
		die("open deployment registry: %v", err)
	}
	defer store.Close()
	view, err := store.RegisterFleetLicenseFromSafeSummary(context.Background(), summary, *payloadSHA, *actorRole)
	if err != nil {
		die("register fleet license: %v", err)
	}
	writeJSON(view)
}
