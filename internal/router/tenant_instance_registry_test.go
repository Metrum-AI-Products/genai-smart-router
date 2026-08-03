package router

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRDSQuotaAdapter struct {
	snapshot     QuotaSnapshot
	preflightErr error
	reserveErr   error
	preflights   int
	reservations int
}

func (f *fakeRDSQuotaAdapter) Preflight(_ context.Context, region string) (QuotaSnapshot, error) {
	f.preflights++
	if f.preflightErr != nil {
		return QuotaSnapshot{}, f.preflightErr
	}
	if region != f.snapshot.Region {
		return QuotaSnapshot{}, errors.New("unexpected region")
	}
	return f.snapshot, nil
}
func (f *fakeRDSQuotaAdapter) Reserve(_ context.Context, region, _ string, count int) error {
	f.reservations++
	if region != f.snapshot.Region || count != 1 {
		return errors.New("unexpected reservation")
	}
	return f.reserveErr
}

func testTenantInstance(tenant, instance string) TenantInstance {
	return TenantInstance{TenantID: tenant, InstanceID: instance, Stage: "customer-test", Region: "test-region-1", Namespace: "ns-" + instance, ReleaseName: "release-" + instance, RuntimeIdentity: "identity-" + instance, RDSInstanceID: "rds-" + instance, RDSInstanceClass: "t3a.medium", AllocatedStorageGiB: 50, Placement: TenantPlacementDedicatedInstance, RDSProxyMode: RDSProxyDisabled, DesiredReleaseDigest: "sha256:abc", ExpectedSchemaVersion: 4, CurrentSchemaVersion: 4, ObservedAt: time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)}
}

func openTestTenantRegistry(t *testing.T) *TenantInstanceRegistry {
	t.Helper()
	r, err := OpenTenantInstanceRegistry(filepath.Join(t.TempDir(), "tenant-registry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestTenantRegistryUsesNormalizedNonSecretRelations(t *testing.T) {
	r := openTestTenantRegistry(t)
	if err := r.Register(context.Background(), testTenantInstance("tenant-a", "router-a")); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"tenants", "router_instances", "instance_placements", "instance_schema_status", "tenant_instance_quota_reservations"} {
		if !r.db.Migrator().HasTable(table) {
			t.Fatalf("missing relational table %s", table)
		}
	}
	for _, table := range []string{"router_instances", "instance_placements", "instance_schema_status", "tenant_instance_quota_reservations"} {
		var foreignKeys []struct{ Table string }
		if err := r.db.Raw("PRAGMA foreign_key_list(" + table + ")").Scan(&foreignKeys).Error; err != nil {
			t.Fatal(err)
		}
		if len(foreignKeys) == 0 {
			t.Fatalf("%s has no relational foreign key", table)
		}
	}
	var columns []struct{ Name string }
	if err := r.db.Raw("PRAGMA table_info(instance_placements)").Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		lower := strings.ToLower(column.Name)
		if strings.Contains(lower, "dsn") || strings.Contains(lower, "credential") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "endpoint") {
			t.Fatalf("forbidden persisted column %q", column.Name)
		}
	}
	resolved, err := r.ResolveDedicated(context.Background(), "tenant-a", "customer-test")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RDSInstanceID != "rds-router-a" || resolved.Placement != TenantPlacementDedicatedInstance || resolved.RDSProxyMode != RDSProxyDisabled {
		t.Fatalf("unexpected safe resolution: %#v", resolved)
	}
	second := testTenantInstance("tenant-a", "router-a-prod")
	second.Stage, second.Namespace, second.RuntimeIdentity, second.RDSInstanceID = "production", "ns-router-a-prod", "identity-router-a-prod", "rds-router-a-prod"
	if err := r.Register(context.Background(), second); err != nil {
		t.Fatalf("second environment registration: %v", err)
	}
}

func TestTenantRegistryRejectsSharedPlacementAndRDSProxy(t *testing.T) {
	r := openTestTenantRegistry(t)
	shared := testTenantInstance("tenant-shared", "router-shared")
	shared.Placement = "shared_instance"
	if err := r.Register(context.Background(), shared); err == nil {
		t.Fatal("shared placement was accepted")
	}
	proxy := testTenantInstance("tenant-proxy", "router-proxy")
	proxy.RDSProxyMode = "enabled"
	if err := r.Register(context.Background(), proxy); err == nil {
		t.Fatal("RDS Proxy mode was accepted")
	}
	if _, err := r.ResolveDedicated(context.Background(), "missing", "customer-test"); err == nil {
		t.Fatal("missing tenant resolved")
	}
	if _, err := OpenTenantInstanceRegistry("file:unsafe?mode=memory"); err == nil {
		t.Fatal("SQLite URI registry path accepted")
	}
}

func TestTenantRegistryReadOnlyOpenNeverCreatesMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.sqlite")
	if _, err := OpenTenantInstanceRegistryReadOnly(path); err == nil {
		t.Fatal("missing read-only registry opened")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only open created registry: %v", err)
	}
}

func TestTenantRegistryRejectsConflictingReregistration(t *testing.T) {
	r := openTestTenantRegistry(t)
	instance := testTenantInstance("tenant-a", "router-a")
	if err := r.Register(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	instance.ExpectedSchemaVersion++
	if err := r.Register(context.Background(), instance); err == nil {
		t.Fatal("conflicting same tenant/stage/instance registration succeeded")
	}
}

func TestTenantRegistryObserveSchemaVersionIsIndependentBoundedAndLocal(t *testing.T) {
	r := openTestTenantRegistry(t)
	instance := testTenantInstance("tenant-a", "router-a")
	instance.CurrentSchemaVersion = 3
	if err := r.Register(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	status, err := r.DriftStatus(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 || status[0].DriftCode != "schema_version_mismatch" {
		t.Fatalf("independent registration did not expose drift: %#v", status)
	}

	observation, err := r.ObserveSchemaVersion(context.Background(), "tenant-a", "customer-test", 4)
	if err != nil {
		t.Fatal(err)
	}
	if observation.ExpectedSchemaVersion != 4 ||
		observation.CurrentSchemaVersion != 4 ||
		observation.DriftCode != "current" ||
		!observation.ObservedAt.After(instance.ObservedAt) {
		t.Fatalf("unexpected schema observation: %#v", observation)
	}
	resolved, err := r.ResolveDedicated(context.Background(), "tenant-a", "customer-test")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ExpectedSchemaVersion != 4 ||
		resolved.CurrentSchemaVersion != 4 ||
		resolved.DesiredReleaseDigest != instance.DesiredReleaseDigest {
		t.Fatalf("schema observation changed immutable deployment metadata: %#v", resolved)
	}
	status, err = r.DriftStatus(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 || status[0].DriftCode != "current" {
		t.Fatalf("later observation did not clear drift: %#v", status)
	}

	for _, invalidVersion := range []int{-1, maxTenantSchemaVersion + 1} {
		if _, err := r.ObserveSchemaVersion(context.Background(), "tenant-a", "customer-test", invalidVersion); err == nil {
			t.Fatalf("out-of-bounds observed schema version accepted: %d", invalidVersion)
		}
	}
	unchanged, err := r.ResolveDedicated(context.Background(), "tenant-a", "customer-test")
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.CurrentSchemaVersion != 4 || !unchanged.ObservedAt.Equal(observation.ObservedAt) {
		t.Fatalf("rejected observation mutated registry state: %#v", unchanged)
	}
	if _, err := r.ObserveSchemaVersion(context.Background(), "missing", "customer-test", 4); err == nil {
		t.Fatal("missing tenant observation succeeded")
	}
	if _, err := r.ObserveSchemaVersion(context.Background(), "tenant-a", "", 4); err == nil {
		t.Fatal("ambiguous tenant-only observation succeeded")
	}

	future := testTenantInstance("tenant-future", "router-future")
	future.ObservedAt = time.Now().UTC().Add(time.Hour)
	if err := r.Register(context.Background(), future); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ObserveSchemaVersion(context.Background(), "tenant-future", "customer-test", 3); err == nil {
		t.Fatal("stale server-generated observation replaced a future registry observation")
	}
	futureResolved, err := r.ResolveDedicated(context.Background(), "tenant-future", "customer-test")
	if err != nil {
		t.Fatal(err)
	}
	if futureResolved.CurrentSchemaVersion != future.CurrentSchemaVersion ||
		!futureResolved.ObservedAt.Equal(future.ObservedAt) {
		t.Fatalf("rejected stale observation mutated registry state: %#v", futureResolved)
	}

	tooLarge := testTenantInstance("tenant-large", "router-large")
	tooLarge.ExpectedSchemaVersion = maxTenantSchemaVersion + 1
	if err := r.Register(context.Background(), tooLarge); err == nil {
		t.Fatal("out-of-bounds expected schema version registered")
	}
}

func TestTenantRegistryQuotaPreflightReservationIsIdempotentAndFailsClosed(t *testing.T) {
	r := openTestTenantRegistry(t)
	if err := r.Register(context.Background(), testTenantInstance("tenant-a", "router-a")); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeRDSQuotaAdapter{snapshot: QuotaSnapshot{Region: "test-region-1", Limit: 5, Used: 2, Reserved: 1}}
	reservation, err := r.PreflightAndReserve(context.Background(), "tenant-a", "customer-test", "reserve-a", 1, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.State != "admission_reserved" || reservation.Headroom != 1 || adapter.reservations != 1 {
		t.Fatalf("bad reservation %#v calls=%d", reservation, adapter.reservations)
	}
	if _, err := r.PreflightAndReserve(context.Background(), "tenant-a", "customer-test", "reserve-a", 1, adapter); err != nil {
		t.Fatal(err)
	}
	if adapter.reservations != 1 {
		t.Fatalf("idempotency called adapter %d times", adapter.reservations)
	}
	other := testTenantInstance("tenant-b", "router-b")
	other.Namespace, other.RuntimeIdentity, other.RDSInstanceID = "ns-router-b", "identity-router-b", "rds-router-b"
	if err := r.Register(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if _, err := r.PreflightAndReserve(context.Background(), "tenant-b", "customer-test", "reserve-a", 1, adapter); err == nil {
		t.Fatal("reservation ID was reused across tenants")
	}
	regional := &fakeRDSQuotaAdapter{snapshot: QuotaSnapshot{Region: "test-region-1", Limit: 1}}
	if _, err := r.PreflightAndReserve(context.Background(), "tenant-b", "customer-test", "reserve-b", 0, regional); err == nil {
		t.Fatal("active regional local hold did not consume capacity")
	}
	if regional.reservations != 0 {
		t.Fatal("regional local hold rejection called adapter")
	}
	exhausted := &fakeRDSQuotaAdapter{snapshot: QuotaSnapshot{Region: "test-region-1", Limit: 3, Used: 2, Reserved: 0}}
	if _, err := r.PreflightAndReserve(context.Background(), "tenant-a", "customer-test", "reserve-exhausted", 1, exhausted); err == nil {
		t.Fatal("headroom exhaustion was accepted")
	}
	if exhausted.reservations != 0 {
		t.Fatal("quota exhaustion called reserve")
	}
	failing := &fakeRDSQuotaAdapter{snapshot: QuotaSnapshot{Region: "test-region-1", Limit: 10}, reserveErr: errors.New("fake adapter failure")}
	if _, err := r.PreflightAndReserve(context.Background(), "tenant-a", "customer-test", "reserve-fail", 0, failing); err == nil {
		t.Fatal("adapter failure was accepted")
	}
	var rows int64
	if err := r.db.Model(&quotaReservationRecord{}).Where("reservation_id = ?", "reserve-fail").Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("failed reservation persisted %d rows", rows)
	}
}

func TestTenantRegistryDriftStatusIsBoundedDeterministicAndSafe(t *testing.T) {
	r := openTestTenantRegistry(t)
	b := testTenantInstance("tenant-b", "router-b")
	b.CurrentSchemaVersion = 3
	if err := r.Register(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(context.Background(), testTenantInstance("tenant-a", "router-a")); err != nil {
		t.Fatal(err)
	}
	status, err := r.DriftStatus(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 || status[0].TenantID != "tenant-a" || status[1].DriftCode != "schema_version_mismatch" {
		t.Fatalf("unexpected status %#v", status)
	}
	if status[0].EndpointPolicy != "not_evaluated" || status[0].RDSProxyMode != RDSProxyDisabled {
		t.Fatalf("missing gate state %#v", status[0])
	}
	if _, err := r.DriftStatus(context.Background(), 101); err == nil {
		t.Fatal("unbounded status accepted")
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"dsn", "credential", "secret", "token", "config"} {
		if strings.Contains(strings.ToLower(string(payload)), forbidden) {
			t.Fatalf("unsafe status serialization contains %q: %s", forbidden, payload)
		}
	}
}
