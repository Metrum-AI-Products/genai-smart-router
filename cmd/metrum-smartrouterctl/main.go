// metrum-smartrouterctl exposes the ADR-0012 safe operator contract. Live
// deployment, promotion, rollback, cloud, DNS, and Kubernetes actions remain
// intentionally disabled until the recorded human policy gates are closed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	if len(os.Args) < 2 {
		die("usage: metrum-smartrouterctl <register|quota-reserve|status|deploy|promote|rollback> [flags]")
	}
	switch os.Args[1] {
	case "register":
		register(os.Args[2:])
	case "quota-reserve":
		quotaReserve(os.Args[2:])
	case "status":
		status(os.Args[2:])
	case "deploy", "promote", "rollback":
		die("%s is disabled: ADR-0012 human endpoint, encryption, TLS, durability, HA, network, and DNS gates remain open", os.Args[1])
	default:
		die("unsupported command %q", os.Args[1])
	}
}

func registryFromFlags(fs *flag.FlagSet) (*string, *string) {
	path := fs.String("registry", "tenant-instances.sqlite", "local non-secret tenant registry SQLite path")
	tenant := fs.String("tenant", "", "deployment-defined tenant identifier")
	return path, tenant
}

func register(args []string) {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	path, tenant := registryFromFlags(fs)
	instance := fs.String("instance", "", "router instance identifier")
	stage := fs.String("stage", "", "user-defined stage label")
	region := fs.String("region", "", "deployment-defined region label")
	namespace := fs.String("namespace", "", "isolated Kubernetes namespace label")
	release := fs.String("release", "", "isolated release label")
	runtimeIdentity := fs.String("runtime-identity", "", "isolated runtime identity label")
	rds := fs.String("rds-instance", "", "dedicated RDS instance identifier")
	class := fs.String("rds-class", "t3a.medium", "declared RDS instance class")
	storage := fs.Int("storage-gib", 50, "declared RDS allocated storage in GiB")
	digest := fs.String("release-digest", "", "immutable desired release digest")
	schema := fs.Int("schema-version", 0, "expected schema version")
	proxy := fs.String("rds-proxy", router.RDSProxyDisabled, "must be disabled; RDS Proxy is disabled")
	fs.Parse(args)
	r, err := router.OpenTenantInstanceRegistry(*path)
	if err != nil {
		die("open registry: %v", err)
	}
	defer r.Close()
	err = r.Register(context.Background(), router.TenantInstance{TenantID: *tenant, InstanceID: *instance, Stage: *stage, Region: *region, Namespace: *namespace, ReleaseName: *release, RuntimeIdentity: *runtimeIdentity, RDSInstanceID: *rds, RDSInstanceClass: *class, AllocatedStorageGiB: *storage, Placement: router.TenantPlacementDedicatedInstance, RDSProxyMode: *proxy, DesiredReleaseDigest: *digest, ExpectedSchemaVersion: *schema, CurrentSchemaVersion: *schema})
	if err != nil {
		die("register tenant contract: %v", err)
	}
	fmt.Println("registered safe tenant-instance contract")
}

type staticQuotaAdapter struct{ snapshot router.QuotaSnapshot }

func (a staticQuotaAdapter) Preflight(_ context.Context, region string) (router.QuotaSnapshot, error) {
	if a.snapshot.Region != region {
		return router.QuotaSnapshot{}, fmt.Errorf("mock quota region mismatch")
	}
	return a.snapshot, nil
}
func (a staticQuotaAdapter) Reserve(_ context.Context, region, _ string, count int) error {
	if a.snapshot.Region != region || count != 1 || a.snapshot.Available() < count {
		return fmt.Errorf("mock quota unavailable")
	}
	return nil
}

func quotaReserve(args []string) {
	fs := flag.NewFlagSet("quota-reserve", flag.ExitOnError)
	path, tenant := registryFromFlags(fs)
	stage := fs.String("stage", "", "user-defined stage label")
	reservation := fs.String("reservation", "", "idempotency-safe reservation identifier")
	limit := fs.Int("mock-quota-limit", 0, "fake regional dedicated-RDS quota limit")
	used := fs.Int("mock-quota-used", 0, "fake regional dedicated-RDS quota usage")
	reserved := fs.Int("mock-quota-reserved", 0, "fake regional dedicated-RDS quota reservations")
	headroom := fs.Int("headroom", 0, "fake capacity headroom retained before one local admission reservation")
	fs.Parse(args)
	r, err := router.OpenTenantInstanceRegistry(*path)
	if err != nil {
		die("open registry: %v", err)
	}
	defer r.Close()
	instance, err := r.ResolveDedicated(context.Background(), *tenant, *stage)
	if err != nil {
		die("resolve tenant: %v", err)
	}
	result, err := r.PreflightAndReserve(context.Background(), *tenant, *stage, *reservation, *headroom, staticQuotaAdapter{snapshot: router.QuotaSnapshot{Region: instance.Region, Limit: *limit, Used: *used, Reserved: *reserved}})
	if err != nil {
		die("quota preflight/reservation: %v", err)
	}
	writeJSON(result)
}

func status(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	path := fs.String("registry", "tenant-instances.sqlite", "local non-secret tenant registry SQLite path")
	limit := fs.Int("limit", 25, "bounded number of tenant instances to inspect (1-100)")
	fs.Parse(args)
	r, err := router.OpenTenantInstanceRegistryReadOnly(*path)
	if err != nil {
		die("open registry: %v", err)
	}
	defer r.Close()
	result, err := r.DriftStatus(context.Background(), *limit)
	if err != nil {
		die("read-only drift status: %v", err)
	}
	writeJSON(result)
}

func writeJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		die("encode safe output: %v", err)
	}
	fmt.Println(string(b))
}
func die(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(2) }
