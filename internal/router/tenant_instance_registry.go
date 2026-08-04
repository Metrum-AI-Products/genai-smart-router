package router

// ADR-0012 safe contract slice. This registry holds only relational,
// non-secret deployment identity and local quota-admission evidence. It does
// not connect to AWS, Kubernetes, DNS, an RDS endpoint, or router databases.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	TenantPlacementDedicatedInstance = "dedicated_instance"
	RDSProxyDisabled                 = "disabled"
	maxTenantDriftStatusRows         = 100
	quotaReservationIDPrefix         = "rsv-"
	quotaReservationIDLength         = 40
	maxTenantSchemaVersion           = 1<<31 - 1
)

// TenantInstance is the safe input/output contract. Its component fields are
// stored in normalized tables below; it contains no endpoint, DSN, credential,
// token, secret reference/value, or configuration content.
type TenantInstance struct {
	TenantID              string
	InstanceID            string
	Stage                 string
	Region                string
	Namespace             string
	ReleaseName           string
	RuntimeIdentity       string
	RDSInstanceID         string
	RDSInstanceClass      string
	AllocatedStorageGiB   int
	Placement             string
	RDSProxyMode          string
	DesiredReleaseDigest  string
	ExpectedSchemaVersion int
	CurrentSchemaVersion  int
	ObservedAt            time.Time
}

type tenantRecord struct {
	TenantID  string    `gorm:"primaryKey;column:tenant_id;type:text"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (tenantRecord) TableName() string { return "tenants" }

type routerInstanceRecord struct {
	InstanceID      string    `gorm:"primaryKey;column:instance_id;type:text"`
	TenantID        string    `gorm:"uniqueIndex:tenant_stage;column:tenant_id;type:text;not null"`
	Stage           string    `gorm:"uniqueIndex:tenant_stage;column:stage;type:text;not null"`
	Region          string    `gorm:"column:region;type:text;not null"`
	Namespace       string    `gorm:"uniqueIndex;column:namespace;type:text;not null"`
	ReleaseName     string    `gorm:"column:release_name;type:text;not null"`
	RuntimeIdentity string    `gorm:"uniqueIndex;column:runtime_identity;type:text;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (routerInstanceRecord) TableName() string { return "router_instances" }

type instancePlacementRecord struct {
	InstanceID          string `gorm:"primaryKey;column:instance_id;type:text"`
	PlacementKind       string `gorm:"column:placement_kind;type:text;not null"`
	RDSAllocationID     string `gorm:"uniqueIndex;column:rds_allocation_id;type:text;not null"`
	RDSInstanceClass    string `gorm:"column:rds_instance_class;type:text;not null"`
	AllocatedStorageGiB int    `gorm:"column:allocated_storage_gib;not null"`
	RDSProxyMode        string `gorm:"column:rds_proxy_mode;type:text;not null"`
}

func (instancePlacementRecord) TableName() string { return "instance_placements" }

type instanceSchemaStatusRecord struct {
	InstanceID            string    `gorm:"primaryKey;column:instance_id;type:text"`
	DesiredReleaseDigest  string    `gorm:"column:desired_release_digest;type:text;not null"`
	ExpectedSchemaVersion int       `gorm:"column:expected_schema_version;not null"`
	CurrentSchemaVersion  int       `gorm:"column:current_schema_version;not null"`
	ObservedAt            time.Time `gorm:"column:observed_at;not null"`
}

func (instanceSchemaStatusRecord) TableName() string { return "instance_schema_status" }

type quotaReservationRecord struct {
	ReservationID string    `gorm:"primaryKey;column:reservation_id;type:text"`
	InstanceID    string    `gorm:"index;column:instance_id;type:text;not null"`
	Region        string    `gorm:"column:region;type:text;not null"`
	DBInstances   int       `gorm:"column:db_instances;not null"`
	QuotaLimit    int       `gorm:"column:quota_limit;not null"`
	QuotaUsed     int       `gorm:"column:quota_used;not null"`
	QuotaReserved int       `gorm:"column:quota_reserved;not null"`
	Headroom      int       `gorm:"column:headroom;not null"`
	State         string    `gorm:"column:state;type:text;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null"`
}

func (quotaReservationRecord) TableName() string { return "tenant_instance_quota_reservations" }

type QuotaSnapshot struct {
	Region                string
	Limit, Used, Reserved int
}

func (q QuotaSnapshot) Available() int { return q.Limit - q.Used - q.Reserved }

type QuotaReservation struct {
	ReservationID, InstanceID, Region, State                    string
	DBInstances, QuotaLimit, QuotaUsed, QuotaReserved, Headroom int
	CreatedAt                                                   time.Time
}

// RDSQuotaAdapter is a fake-first capacity seam. Its reservation is a local
// admission hold, not an AWS reservation; no production adapter is wired.
type RDSQuotaAdapter interface {
	Preflight(context.Context, string) (QuotaSnapshot, error)
	Reserve(context.Context, string, string, int) error
}

type TenantDriftStatus struct {
	TenantID, InstanceID, Stage, Placement, RDSProxyMode, DriftCode                                   string
	ExpectedSchemaVersion, CurrentSchemaVersion                                                       int
	ObservedAt                                                                                        time.Time
	EndpointPolicy, EncryptionPolicy, TLSPolicy, DurabilityPolicy, HAPolicy, NetworkPolicy, DNSPolicy string
}

// SchemaObservation is the safe scalar result of recording an independently
// supplied current schema version in the local registry.
type SchemaObservation struct {
	TenantID, InstanceID, Stage                 string
	ExpectedSchemaVersion, CurrentSchemaVersion int
	DriftCode                                   string
	ObservedAt                                  time.Time
}

// TenantInstanceRegistry owns a new isolated control-plane SQLite schema. It
// is intentionally separate from router request usage and #507 migrations.
type TenantInstanceRegistry struct{ db *gorm.DB }

func OpenTenantInstanceRegistry(path string) (*TenantInstanceRegistry, error) {
	if err := validateTenantRegistryPath(path); err != nil {
		return nil, err
	}
	if err := ensureSQLitePrivateMode(path); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(sqliteDSNWithForeignKeys(path)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	if err := chmodSQLiteFiles(path); err != nil {
		return nil, err
	}
	r := &TenantInstanceRegistry{db: db}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS tenants (tenant_id TEXT PRIMARY KEY, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS router_instances (instance_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, stage TEXT NOT NULL, region TEXT NOT NULL, namespace TEXT NOT NULL UNIQUE, release_name TEXT NOT NULL, runtime_identity TEXT NOT NULL UNIQUE, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(tenant_id, stage), UNIQUE(region, release_name), FOREIGN KEY(tenant_id) REFERENCES tenants(tenant_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
		`CREATE TABLE IF NOT EXISTS instance_placements (instance_id TEXT PRIMARY KEY, placement_kind TEXT NOT NULL, rds_allocation_id TEXT NOT NULL UNIQUE, rds_instance_class TEXT NOT NULL, allocated_storage_gib INTEGER NOT NULL, rds_proxy_mode TEXT NOT NULL, FOREIGN KEY(instance_id) REFERENCES router_instances(instance_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
		`CREATE TABLE IF NOT EXISTS instance_schema_status (instance_id TEXT PRIMARY KEY, desired_release_digest TEXT NOT NULL, expected_schema_version INTEGER NOT NULL, current_schema_version INTEGER NOT NULL, observed_at DATETIME NOT NULL, FOREIGN KEY(instance_id) REFERENCES router_instances(instance_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
		`CREATE TABLE IF NOT EXISTS tenant_instance_quota_reservations (reservation_id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, region TEXT NOT NULL, db_instances INTEGER NOT NULL, quota_limit INTEGER NOT NULL, quota_used INTEGER NOT NULL, quota_reserved INTEGER NOT NULL, headroom INTEGER NOT NULL, state TEXT NOT NULL, created_at DATETIME NOT NULL, FOREIGN KEY(instance_id) REFERENCES router_instances(instance_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
	} {
		if err := r.db.Exec(stmt).Error; err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("initialize tenant registry: %w", err)
		}
	}
	return r, nil
}

// OpenTenantInstanceRegistryExisting opens an existing registry for local
// updates without creating a file or applying DDL.
func OpenTenantInstanceRegistryExisting(path string) (*TenantInstanceRegistry, error) {
	if err := validateTenantRegistryPath(path); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open existing tenant registry: %w", err)
	}
	db, err := openTenantRegistrySQLite(path, "rw")
	if err != nil {
		return nil, err
	}
	return &TenantInstanceRegistry{db: db}, nil
}

// OpenTenantInstanceRegistryReadOnly never creates a database or applies DDL.
// Status callers use it so an absent registry fails closed rather than being
// initialized as a side effect of a read-only drift query.
func OpenTenantInstanceRegistryReadOnly(path string) (*TenantInstanceRegistry, error) {
	if err := validateTenantRegistryPath(path); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open read-only tenant registry: %w", err)
	}
	db, err := openTenantRegistrySQLite(path, "ro")
	if err != nil {
		return nil, err
	}
	return &TenantInstanceRegistry{db: db}, nil
}

func openTenantRegistrySQLite(path, mode string) (*gorm.DB, error) {
	dsn := (&url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     path,
		RawQuery: "mode=" + mode + "&_pragma=foreign_keys(1)",
	}).String()
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func validateTenantRegistryPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("tenant registry path is required")
	}
	if strings.Contains(path, "?") || strings.HasPrefix(path, "file:") || path == ":memory:" {
		return errors.New("tenant registry must use a local SQLite file path without URI options")
	}
	return nil
}
func (r *TenantInstanceRegistry) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

func (r *TenantInstanceRegistry) Register(ctx context.Context, v TenantInstance) error {
	if err := validateTenantInstance(v); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing routerInstanceRecord
		err := tx.Where("tenant_id = ? AND stage = ?", v.TenantID, v.Stage).First(&existing).Error
		if err == nil {
			if existing.InstanceID == v.InstanceID {
				current, resolveErr := r.ResolveDedicated(ctx, v.TenantID, v.Stage)
				if resolveErr == nil && tenantInstanceImmutableContractEquivalent(current, v) {
					return nil
				}
				return errors.New("tenant stage is already registered with a different instance contract")
			}
			return fmt.Errorf("tenant %q is already registered with a different instance", v.TenantID)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UTC()
		observed := v.ObservedAt.UTC()
		if observed.IsZero() {
			observed = now
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&tenantRecord{TenantID: v.TenantID, CreatedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&routerInstanceRecord{InstanceID: v.InstanceID, TenantID: v.TenantID, Stage: v.Stage, Region: v.Region, Namespace: v.Namespace, ReleaseName: v.ReleaseName, RuntimeIdentity: v.RuntimeIdentity, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&instancePlacementRecord{InstanceID: v.InstanceID, PlacementKind: v.Placement, RDSAllocationID: v.RDSInstanceID, RDSInstanceClass: v.RDSInstanceClass, AllocatedStorageGiB: v.AllocatedStorageGiB, RDSProxyMode: v.RDSProxyMode}).Error; err != nil {
			return err
		}
		return tx.Create(&instanceSchemaStatusRecord{InstanceID: v.InstanceID, DesiredReleaseDigest: v.DesiredReleaseDigest, ExpectedSchemaVersion: v.ExpectedSchemaVersion, CurrentSchemaVersion: v.CurrentSchemaVersion, ObservedAt: observed}).Error
	})
}

// tenantInstanceImmutableContractEquivalent compares only desired-state fields
// supplied by registration. CurrentSchemaVersion and ObservedAt are independent
// observations, so re-registering an unchanged contract must preserve them.
func tenantInstanceImmutableContractEquivalent(a, b TenantInstance) bool {
	return a.TenantID == b.TenantID && a.InstanceID == b.InstanceID && a.Stage == b.Stage && a.Region == b.Region && a.Namespace == b.Namespace && a.ReleaseName == b.ReleaseName && a.RuntimeIdentity == b.RuntimeIdentity && a.RDSInstanceID == b.RDSInstanceID && a.RDSInstanceClass == b.RDSInstanceClass && a.AllocatedStorageGiB == b.AllocatedStorageGiB && a.Placement == b.Placement && a.RDSProxyMode == b.RDSProxyMode && a.DesiredReleaseDigest == b.DesiredReleaseDigest && a.ExpectedSchemaVersion == b.ExpectedSchemaVersion
}
func validateTenantInstance(v TenantInstance) error {
	for _, f := range []struct{ name, value string }{{"tenant id", v.TenantID}, {"instance id", v.InstanceID}, {"stage", v.Stage}, {"region", v.Region}, {"namespace", v.Namespace}, {"release name", v.ReleaseName}, {"runtime identity", v.RuntimeIdentity}, {"RDS allocation id", v.RDSInstanceID}, {"RDS instance class", v.RDSInstanceClass}, {"release digest", v.DesiredReleaseDigest}} {
		if !safeRegistryScalar(f.value) {
			return fmt.Errorf("%s is required", f.name)
		}
	}
	if v.Placement != TenantPlacementDedicatedInstance {
		return errors.New("unsupported placement: dedicated placement is required")
	}
	if v.RDSProxyMode != RDSProxyDisabled {
		return errors.New("unsupported RDS Proxy mode: disabled is required")
	}
	if v.AllocatedStorageGiB <= 0 {
		return errors.New("storage must be positive")
	}
	if err := validateTenantSchemaVersion(v.ExpectedSchemaVersion); err != nil {
		return fmt.Errorf("expected schema version: %w", err)
	}
	if err := validateTenantSchemaVersion(v.CurrentSchemaVersion); err != nil {
		return fmt.Errorf("current schema version: %w", err)
	}
	return nil
}

func validateTenantSchemaVersion(version int) error {
	if version < 0 || version > maxTenantSchemaVersion {
		return fmt.Errorf("must be between 0 and %d", maxTenantSchemaVersion)
	}
	return nil
}

func tenantSchemaDriftCode(expected, current int) string {
	if current != expected {
		return "schema_version_mismatch"
	}
	return "current"
}

func safeRegistryScalar(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 160 || strings.ContainsAny(v, "\n\r{}?@") || strings.Contains(v, "://") {
		return false
	}
	lower := strings.ToLower(v)
	if strings.Contains(lower, "dsn") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:", r)) {
			return false
		}
	}
	return true
}

// ResolveDedicated fetches the explicit allocation. It never creates a name
// from a tenant identifier and has no shared-placement fallback.
func (r *TenantInstanceRegistry) ResolveDedicated(ctx context.Context, tenantID, stage string) (TenantInstance, error) {
	var i routerInstanceRecord
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND stage = ?", tenantID, stage).First(&i).Error; err != nil {
		return TenantInstance{}, err
	}
	var p instancePlacementRecord
	if err := r.db.WithContext(ctx).Where("instance_id = ?", i.InstanceID).First(&p).Error; err != nil {
		return TenantInstance{}, err
	}
	var s instanceSchemaStatusRecord
	if err := r.db.WithContext(ctx).Where("instance_id = ?", i.InstanceID).First(&s).Error; err != nil {
		return TenantInstance{}, err
	}
	if p.PlacementKind != TenantPlacementDedicatedInstance || p.RDSProxyMode != RDSProxyDisabled {
		return TenantInstance{}, errors.New("tenant registry entry violates dedicated RDS policy")
	}
	return TenantInstance{TenantID: i.TenantID, InstanceID: i.InstanceID, Stage: i.Stage, Region: i.Region, Namespace: i.Namespace, ReleaseName: i.ReleaseName, RuntimeIdentity: i.RuntimeIdentity, RDSInstanceID: p.RDSAllocationID, RDSInstanceClass: p.RDSInstanceClass, AllocatedStorageGiB: p.AllocatedStorageGiB, Placement: p.PlacementKind, RDSProxyMode: p.RDSProxyMode, DesiredReleaseDigest: s.DesiredReleaseDigest, ExpectedSchemaVersion: s.ExpectedSchemaVersion, CurrentSchemaVersion: s.CurrentSchemaVersion, ObservedAt: s.ObservedAt}, nil
}

func validQuotaReservationID(reservationID string) bool {
	if len(reservationID) != quotaReservationIDLength ||
		!strings.HasPrefix(reservationID, quotaReservationIDPrefix) {
		return false
	}
	for index := len(quotaReservationIDPrefix); index < len(reservationID); index++ {
		switch index {
		case 12, 17, 22, 27:
			if reservationID[index] != '-' {
				return false
			}
		default:
			character := reservationID[index]
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f')) {
				return false
			}
		}
	}
	return true
}

// validLegacyQuotaReservationID permits a bounded, opaque historic identifier
// only for an exact existing-row retry. It never admits credential, URL, path,
// or unbounded values, even when a legacy database contains one.
func validLegacyQuotaReservationID(reservationID string) bool {
	if len(reservationID) == 0 || len(reservationID) > quotaReservationIDLength {
		return false
	}
	lower := strings.ToLower(reservationID)
	if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "ghp_") {
		return false
	}
	for _, character := range reservationID {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
			return false
		}
	}
	return true
}

// ObserveSchemaVersion records a caller-supplied current version for exactly
// one local tenant/stage registration. It does not contact a router database,
// deployment endpoint, cloud API, Kubernetes, DNS, or a credential store.
func (r *TenantInstanceRegistry) ObserveSchemaVersion(ctx context.Context, tenantID, stage string, currentSchemaVersion int) (SchemaObservation, error) {
	if !safeRegistryScalar(tenantID) || !safeRegistryScalar(stage) {
		return SchemaObservation{}, errors.New("tenant id and stage are required safe registry identifiers")
	}
	if err := validateTenantSchemaVersion(currentSchemaVersion); err != nil {
		return SchemaObservation{}, fmt.Errorf("observed schema version: %w", err)
	}
	var result SchemaObservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instances []routerInstanceRecord
		if err := tx.Where("tenant_id = ? AND stage = ?", tenantID, stage).Limit(2).Find(&instances).Error; err != nil {
			return err
		}
		if len(instances) != 1 {
			return errors.New("tenant stage must resolve to exactly one registered instance")
		}
		instance := instances[0]
		var status instanceSchemaStatusRecord
		if err := tx.Where("instance_id = ?", instance.InstanceID).First(&status).Error; err != nil {
			return err
		}
		observedAt := time.Now().UTC()
		if !observedAt.After(status.ObservedAt) {
			return errors.New("existing schema observation is not older than the new observation")
		}
		update := tx.Model(&instanceSchemaStatusRecord{}).
			Where("instance_id = ?", instance.InstanceID).
			Updates(map[string]any{
				"current_schema_version": currentSchemaVersion,
				"observed_at":            observedAt,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("schema observation did not update exactly one registered instance")
		}
		result = SchemaObservation{
			TenantID:              instance.TenantID,
			InstanceID:            instance.InstanceID,
			Stage:                 instance.Stage,
			ExpectedSchemaVersion: status.ExpectedSchemaVersion,
			CurrentSchemaVersion:  currentSchemaVersion,
			DriftCode:             tenantSchemaDriftCode(status.ExpectedSchemaVersion, currentSchemaVersion),
			ObservedAt:            observedAt,
		}
		return nil
	})
	return result, err
}

func (r *TenantInstanceRegistry) PreflightAndReserve(ctx context.Context, tenantID, stage, reservationID string, headroom int, adapter RDSQuotaAdapter) (QuotaReservation, error) {
	if adapter == nil || headroom < 0 {
		return QuotaReservation{}, errors.New("quota adapter and non-negative headroom are required")
	}
	if !validQuotaReservationID(reservationID) && !validLegacyQuotaReservationID(reservationID) {
		return QuotaReservation{}, errors.New("invalid reservation id: expected rsv- followed by a lowercase canonical UUID")
	}
	var result QuotaReservation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old quotaReservationRecord
		if err := tx.Where("reservation_id = ?", reservationID).First(&old).Error; err == nil {
			instance, resolveErr := r.ResolveDedicated(ctx, tenantID, stage)
			if resolveErr != nil || old.InstanceID != instance.InstanceID || old.Region != instance.Region || old.Headroom != headroom {
				return errors.New("reservation id belongs to a different admission contract")
			}
			result = quotaReservationFromRecord(old)
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !validQuotaReservationID(reservationID) {
			return errors.New("invalid reservation id: expected rsv- followed by a lowercase canonical UUID")
		}
		instance, err := r.ResolveDedicated(ctx, tenantID, stage)
		if err != nil {
			return err
		}
		snapshot, err := adapter.Preflight(ctx, instance.Region)
		if err != nil {
			return fmt.Errorf("quota preflight: %w", err)
		}
		var localHolds, instanceHolds int64
		if err := tx.Model(&quotaReservationRecord{}).Where("region = ? AND state = ?", instance.Region, "admission_reserved").Count(&localHolds).Error; err != nil {
			return err
		}
		if err := tx.Model(&quotaReservationRecord{}).Where("instance_id = ? AND state = ?", instance.InstanceID, "admission_reserved").Count(&instanceHolds).Error; err != nil {
			return err
		}
		if instanceHolds > 0 {
			return errors.New("tenant instance already has an active quota admission reservation")
		}
		if snapshot.Region != instance.Region || snapshot.Limit < 0 || snapshot.Used < 0 || snapshot.Reserved < 0 || snapshot.Available()-int(localHolds) < 1+headroom {
			return errors.New("dedicated RDS quota unavailable")
		}
		if err := adapter.Reserve(ctx, instance.Region, reservationID, 1); err != nil {
			return fmt.Errorf("quota admission reservation: %w", err)
		}
		record := quotaReservationRecord{ReservationID: reservationID, InstanceID: instance.InstanceID, Region: instance.Region, DBInstances: 1, QuotaLimit: snapshot.Limit, QuotaUsed: snapshot.Used, QuotaReserved: snapshot.Reserved, Headroom: headroom, State: "admission_reserved", CreatedAt: time.Now().UTC()}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		result = quotaReservationFromRecord(record)
		return nil
	})
	return result, err
}
func quotaReservationFromRecord(v quotaReservationRecord) QuotaReservation {
	return QuotaReservation{ReservationID: v.ReservationID, InstanceID: v.InstanceID, Region: v.Region, DBInstances: v.DBInstances, QuotaLimit: v.QuotaLimit, QuotaUsed: v.QuotaUsed, QuotaReserved: v.QuotaReserved, Headroom: v.Headroom, State: v.State, CreatedAt: v.CreatedAt}
}

// DriftStatus is registry-only and deterministic. It does not contact a live
// endpoint or persist a new observation; the result is capped at 100 rows.
func (r *TenantInstanceRegistry) DriftStatus(ctx context.Context, limit int) ([]TenantDriftStatus, error) {
	if limit < 1 || limit > maxTenantDriftStatusRows {
		return nil, fmt.Errorf("drift status limit must be between 1 and %d", maxTenantDriftStatusRows)
	}
	var instances []routerInstanceRecord
	if err := r.db.WithContext(ctx).Order("tenant_id ASC, stage ASC, instance_id ASC").Limit(limit).Find(&instances).Error; err != nil {
		return nil, err
	}
	result := make([]TenantDriftStatus, 0, len(instances))
	for _, i := range instances {
		var p instancePlacementRecord
		var s instanceSchemaStatusRecord
		if err := r.db.Where("instance_id = ?", i.InstanceID).First(&p).Error; err != nil {
			return nil, err
		}
		if err := r.db.Where("instance_id = ?", i.InstanceID).First(&s).Error; err != nil {
			return nil, err
		}
		code := tenantSchemaDriftCode(s.ExpectedSchemaVersion, s.CurrentSchemaVersion)
		result = append(result, TenantDriftStatus{TenantID: i.TenantID, InstanceID: i.InstanceID, Stage: i.Stage, Placement: p.PlacementKind, RDSProxyMode: p.RDSProxyMode, DriftCode: code, ExpectedSchemaVersion: s.ExpectedSchemaVersion, CurrentSchemaVersion: s.CurrentSchemaVersion, ObservedAt: s.ObservedAt, EndpointPolicy: "not_evaluated", EncryptionPolicy: "not_authorized", TLSPolicy: "not_authorized", DurabilityPolicy: "not_authorized", HAPolicy: "not_authorized", NetworkPolicy: "not_authorized", DNSPolicy: "not_authorized"})
	}
	return result, nil
}
