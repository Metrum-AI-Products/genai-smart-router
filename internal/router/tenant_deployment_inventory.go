// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	FleetTenantActive  = "active"
	FleetTenantDeleted = "deleted"

	FleetLicenseStatusRegistered = "registered"
	FleetLicenseStatusBound      = "bound"
	FleetLicenseStatusRetired    = "retired"
	FleetLicenseStatusRevoked    = "revoked"

	FleetLicenseBindingIntended = "intended"
	FleetLicenseBindingBound    = "bound"
	FleetLicenseBindingRetired  = "retired"

	FleetDatabaseModeSQLite       = "sqlite"
	FleetDatabaseModeDedicatedRDS = "dedicated-rds"
)

// FleetTenantRecord is the operator inventory row for one customer_id in a Fleet registry.
type FleetTenantRecord struct {
	CustomerID        string    `gorm:"primaryKey;column:customer_id;type:text" json:"customer_id"`
	ProfileID         string    `gorm:"column:profile_id;type:text;not null" json:"profile_id"`
	Stage             string    `gorm:"column:stage;type:text;not null" json:"stage"`
	Environment       string    `gorm:"column:environment;type:text;not null" json:"environment"`
	Namespace         string    `gorm:"column:namespace;type:text;not null" json:"namespace"`
	Hostname          string    `gorm:"column:hostname;type:text;not null" json:"hostname"`
	PrimaryInstanceID string    `gorm:"column:primary_instance_id;type:text;not null" json:"primary_instance_id"`
	LifecycleState    string    `gorm:"column:lifecycle_state;type:text;not null" json:"lifecycle_state"`
	LatestJobID       string    `gorm:"column:latest_job_id;type:text;not null" json:"latest_job_id"`
	LatestJobState    string    `gorm:"column:latest_job_state;type:text;not null" json:"latest_job_state"`
	CreatedAt         time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (FleetTenantRecord) TableName() string { return "fleet_tenants" }

// FleetTenantInstanceRecord tracks one instance identity under a tenant.
type FleetTenantInstanceRecord struct {
	InstanceID     string    `gorm:"primaryKey;column:instance_id;type:text" json:"instance_id"`
	CustomerID     string    `gorm:"index;column:customer_id;type:text;not null" json:"customer_id"`
	ProfileID      string    `gorm:"column:profile_id;type:text;not null" json:"profile_id"`
	Stage          string    `gorm:"column:stage;type:text;not null" json:"stage"`
	Environment    string    `gorm:"column:environment;type:text;not null" json:"environment"`
	Region         string    `gorm:"column:region;type:text;not null" json:"region"`
	ClusterAlias   string    `gorm:"column:cluster_alias;type:text;not null" json:"cluster_alias"`
	Namespace      string    `gorm:"column:namespace;type:text;not null" json:"namespace"`
	Hostname       string    `gorm:"column:hostname;type:text;not null" json:"hostname"`
	CurrentJobID   string    `gorm:"column:current_job_id;type:text;not null" json:"current_job_id"`
	ReleaseDigest  string    `gorm:"column:release_digest;type:text;not null" json:"release_digest"`
	ConfigRevision string    `gorm:"column:config_revision;type:text;not null" json:"config_revision"`
	ComputeProfile string    `gorm:"column:compute_profile;type:text;not null" json:"compute_profile"`
	DatabaseMode   string    `gorm:"column:database_mode;type:text;not null" json:"database_mode"`
	LifecycleState string    `gorm:"column:lifecycle_state;type:text;not null" json:"lifecycle_state"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (FleetTenantInstanceRecord) TableName() string { return "fleet_tenant_instances" }

// FleetLicenseRecord stores safe license inventory scalars only (never envelopes or refs).
type FleetLicenseRecord struct {
	LicenseID          string    `gorm:"primaryKey;column:license_id;type:text" json:"license_id"`
	CustomerID         string    `gorm:"index;column:customer_id;type:text;not null" json:"customer_id"`
	CustomerName       string    `gorm:"column:customer_name;type:text;not null" json:"customer_name"`
	Product            string    `gorm:"column:product;type:text;not null" json:"product"`
	SKU                string    `gorm:"column:sku;type:text;not null" json:"sku"`
	KeyID              string    `gorm:"column:key_id;type:text;not null" json:"key_id"`
	Issuer             string    `gorm:"column:issuer;type:text;not null" json:"issuer"`
	SchemaVersion      int       `gorm:"column:schema_version;not null" json:"schema_version"`
	Status             string    `gorm:"column:status;type:text;not null" json:"status"`
	IssuedAt           time.Time `gorm:"column:issued_at;not null" json:"issued_at"`
	NotBefore          time.Time `gorm:"column:not_before;not null" json:"not_before"`
	ExpiresAt          time.Time `gorm:"column:expires_at;not null" json:"expires_at"`
	GraceUntil         time.Time `gorm:"column:grace_until" json:"grace_until,omitempty"`
	PayloadSHA256      string    `gorm:"column:payload_sha256;type:text;not null" json:"payload_sha256"`
	MaxModelGroups     int       `gorm:"column:max_model_groups;not null" json:"max_model_groups"`
	MaxCallers         int       `gorm:"column:max_callers;not null" json:"max_callers"`
	MaxMonthlyRequests int       `gorm:"column:max_monthly_requests;not null" json:"max_monthly_requests"`
	MaxTotalTokens     int64     `gorm:"column:max_total_tokens;not null" json:"max_total_tokens"`
	MaxTotalRequests   int64     `gorm:"column:max_total_requests;not null" json:"max_total_requests"`
	MaxConcurrent      int       `gorm:"column:max_concurrent;not null" json:"max_concurrent"`
	MaxAdmins          int       `gorm:"column:max_admins;not null" json:"max_admins"`
	MaxRetentionDays   int       `gorm:"column:max_retention_days;not null" json:"max_retention_days"`
	MaxInstances       int       `gorm:"column:max_instances;not null" json:"max_instances"`
	DeploymentMode     string    `gorm:"column:deployment_mode;type:text;not null" json:"deployment_mode"`
	DeploymentID       string    `gorm:"column:deployment_id;type:text;not null" json:"deployment_id"`
	CreatedAt          time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (FleetLicenseRecord) TableName() string { return "fleet_licenses" }

type fleetLicenseFeatureRecord struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	LicenseID string `gorm:"uniqueIndex:license_feature;column:license_id;type:text;not null"`
	Seq       int    `gorm:"uniqueIndex:license_feature;column:seq;not null"`
	Feature   string `gorm:"column:feature;type:text;not null"`
}

func (fleetLicenseFeatureRecord) TableName() string { return "fleet_license_features" }

type fleetLicenseAllowedSkinRecord struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	LicenseID string `gorm:"uniqueIndex:license_skin;column:license_id;type:text;not null"`
	Seq       int    `gorm:"uniqueIndex:license_skin;column:seq;not null"`
	Skin      string `gorm:"column:skin;type:text;not null"`
}

func (fleetLicenseAllowedSkinRecord) TableName() string { return "fleet_license_allowed_skins" }

// FleetLicenseBindingRecord links a license inventory row (when known) to an instance/job.
type FleetLicenseBindingRecord struct {
	ID                  uint      `gorm:"primaryKey;column:id" json:"id"`
	CustomerID          string    `gorm:"index;column:customer_id;type:text;not null" json:"customer_id"`
	InstanceID          string    `gorm:"uniqueIndex:instance_license_binding;column:instance_id;type:text;not null" json:"instance_id"`
	JobID               string    `gorm:"column:job_id;type:text;not null" json:"job_id"`
	LicenseID           string    `gorm:"index;column:license_id;type:text;not null" json:"license_id"`
	BindingState        string    `gorm:"column:binding_state;type:text;not null" json:"binding_state"`
	DesiredRevision     string    `gorm:"column:desired_revision;type:text;not null" json:"desired_revision"`
	ValidityHours       int       `gorm:"column:validity_hours;not null" json:"validity_hours"`
	RequestRefDigest    string    `gorm:"column:request_ref_digest;type:text;not null" json:"request_ref_digest"`
	ObservedSecretAlias string    `gorm:"column:observed_secret_alias;type:text;not null" json:"observed_secret_alias"`
	CreatedAt           time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (FleetLicenseBindingRecord) TableName() string { return "fleet_license_bindings" }

type fleetLicenseEventRecord struct {
	ID         uint      `gorm:"primaryKey;column:id"`
	LicenseID  string    `gorm:"uniqueIndex:license_event;column:license_id;type:text;not null"`
	Seq        int       `gorm:"uniqueIndex:license_event;column:seq;not null"`
	EventName  string    `gorm:"column:event_name;type:text;not null"`
	ActorRole  string    `gorm:"column:actor_role;type:text;not null"`
	JobID      string    `gorm:"column:job_id;type:text;not null"`
	SafeReason string    `gorm:"column:safe_reason;type:text;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (fleetLicenseEventRecord) TableName() string { return "fleet_license_events" }

// FleetTenantView is the safe CLI/list projection for one tenant.
type FleetTenantView struct {
	Schema            string                    `json:"schema"`
	CustomerID        string                    `json:"customer_id"`
	ProfileID         string                    `json:"profile_id"`
	Stage             string                    `json:"stage"`
	Environment       string                    `json:"environment"`
	Namespace         string                    `json:"namespace"`
	Hostname          string                    `json:"hostname"`
	PrimaryInstanceID string                    `json:"primary_instance_id"`
	LifecycleState    string                    `json:"lifecycle_state"`
	LatestJobID       string                    `json:"latest_job_id"`
	LatestJobState    string                    `json:"latest_job_state"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
	Instances         []FleetTenantInstanceView `json:"instances,omitempty"`
	Licenses          []FleetLicenseView        `json:"licenses,omitempty"`
}

// FleetTenantInstanceView is a safe instance projection.
type FleetTenantInstanceView struct {
	InstanceID     string    `json:"instance_id"`
	CurrentJobID   string    `json:"current_job_id"`
	ReleaseDigest  string    `json:"release_digest"`
	ConfigRevision string    `json:"config_revision"`
	ComputeProfile string    `json:"compute_profile"`
	DatabaseMode   string    `json:"database_mode"`
	LifecycleState string    `json:"lifecycle_state"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FleetLicenseView is a safe license inventory projection.
type FleetLicenseView struct {
	LicenseID     string    `json:"license_id"`
	CustomerID    string    `json:"customer_id"`
	CustomerName  string    `json:"customer_name,omitempty"`
	Product       string    `json:"product"`
	SKU           string    `json:"sku"`
	KeyID         string    `json:"key_id"`
	Issuer        string    `json:"issuer"`
	Status        string    `json:"status"`
	IssuedAt      time.Time `json:"issued_at"`
	NotBefore     time.Time `json:"not_before"`
	ExpiresAt     time.Time `json:"expires_at"`
	GraceUntil    time.Time `json:"grace_until,omitempty"`
	PayloadSHA256 string    `json:"payload_sha256,omitempty"`
	Features      []string  `json:"features,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func fleetInventoryDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS fleet_tenants (
			customer_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			environment TEXT NOT NULL,
			namespace TEXT NOT NULL,
			hostname TEXT NOT NULL,
			primary_instance_id TEXT NOT NULL,
			lifecycle_state TEXT NOT NULL,
			latest_job_id TEXT NOT NULL,
			latest_job_state TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_tenant_instances (
			instance_id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			environment TEXT NOT NULL,
			region TEXT NOT NULL,
			cluster_alias TEXT NOT NULL,
			namespace TEXT NOT NULL,
			hostname TEXT NOT NULL,
			current_job_id TEXT NOT NULL,
			release_digest TEXT NOT NULL,
			config_revision TEXT NOT NULL,
			compute_profile TEXT NOT NULL,
			database_mode TEXT NOT NULL,
			lifecycle_state TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY(customer_id) REFERENCES fleet_tenants(customer_id) ON UPDATE CASCADE ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_tenant_instances_customer_id ON fleet_tenant_instances(customer_id)`,
		`CREATE TABLE IF NOT EXISTS fleet_licenses (
			license_id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			customer_name TEXT NOT NULL,
			product TEXT NOT NULL,
			sku TEXT NOT NULL,
			key_id TEXT NOT NULL,
			issuer TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			status TEXT NOT NULL,
			issued_at DATETIME NOT NULL,
			not_before DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			grace_until DATETIME,
			payload_sha256 TEXT NOT NULL,
			max_model_groups INTEGER NOT NULL,
			max_callers INTEGER NOT NULL,
			max_monthly_requests INTEGER NOT NULL,
			max_total_tokens INTEGER NOT NULL,
			max_total_requests INTEGER NOT NULL,
			max_concurrent INTEGER NOT NULL,
			max_admins INTEGER NOT NULL,
			max_retention_days INTEGER NOT NULL,
			max_instances INTEGER NOT NULL,
			deployment_mode TEXT NOT NULL,
			deployment_id TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_licenses_customer_id ON fleet_licenses(customer_id)`,
		`CREATE TABLE IF NOT EXISTS fleet_license_features (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			license_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			feature TEXT NOT NULL,
			UNIQUE(license_id, seq),
			FOREIGN KEY(license_id) REFERENCES fleet_licenses(license_id) ON UPDATE CASCADE ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_license_allowed_skins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			license_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			skin TEXT NOT NULL,
			UNIQUE(license_id, seq),
			FOREIGN KEY(license_id) REFERENCES fleet_licenses(license_id) ON UPDATE CASCADE ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_license_bindings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_id TEXT NOT NULL,
			instance_id TEXT NOT NULL UNIQUE,
			job_id TEXT NOT NULL,
			license_id TEXT NOT NULL,
			binding_state TEXT NOT NULL,
			desired_revision TEXT NOT NULL,
			validity_hours INTEGER NOT NULL,
			request_ref_digest TEXT NOT NULL,
			observed_secret_alias TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_license_bindings_customer_id ON fleet_license_bindings(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_license_bindings_license_id ON fleet_license_bindings(license_id)`,
		`CREATE TABLE IF NOT EXISTS fleet_license_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			license_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			event_name TEXT NOT NULL,
			actor_role TEXT NOT NULL,
			job_id TEXT NOT NULL,
			safe_reason TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			UNIQUE(license_id, seq),
			FOREIGN KEY(license_id) REFERENCES fleet_licenses(license_id) ON UPDATE CASCADE ON DELETE CASCADE
		)`,
	}
}

func (s *TenantDeploymentStore) upsertFleetInventoryFromPlan(ctx context.Context, plan TenantDeploymentPlan, jobState string) error {
	if s == nil || s.db == nil {
		return errors.New("deployment registry is not open")
	}
	now := time.Now().UTC()
	databaseMode := FleetDatabaseModeSQLite
	if strings.TrimSpace(plan.DatabaseID) != "" {
		databaseMode = FleetDatabaseModeDedicatedRDS
	}
	lifecycle := FleetTenantActive
	if jobState == TenantDeploymentDeleted {
		lifecycle = FleetTenantDeleted
	}
	tenant := FleetTenantRecord{
		CustomerID: plan.CustomerID, ProfileID: plan.ProfileID, Stage: plan.Stage, Environment: plan.Environment,
		Namespace: plan.Namespace, Hostname: plan.Hostname, PrimaryInstanceID: plan.InstanceID,
		LifecycleState: lifecycle, LatestJobID: plan.JobID, LatestJobState: jobState,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "customer_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"profile_id": plan.ProfileID, "stage": plan.Stage, "environment": plan.Environment,
			"namespace": plan.Namespace, "hostname": plan.Hostname, "primary_instance_id": plan.InstanceID,
			"lifecycle_state": lifecycle, "latest_job_id": plan.JobID, "latest_job_state": jobState, "updated_at": now,
		}),
	}).Create(&tenant).Error; err != nil {
		return fmt.Errorf("upsert fleet tenant: %w", err)
	}

	instance := FleetTenantInstanceRecord{
		InstanceID: plan.InstanceID, CustomerID: plan.CustomerID, ProfileID: plan.ProfileID, Stage: plan.Stage,
		Environment: plan.Environment, Region: plan.Region, ClusterAlias: plan.ClusterAlias,
		Namespace: plan.Namespace, Hostname: plan.Hostname, CurrentJobID: plan.JobID,
		ReleaseDigest: plan.ReleaseDigest, ConfigRevision: plan.ConfigRevision, ComputeProfile: plan.ComputeProfile,
		DatabaseMode: databaseMode, LifecycleState: lifecycle, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"customer_id": plan.CustomerID, "profile_id": plan.ProfileID, "stage": plan.Stage, "environment": plan.Environment,
			"region": plan.Region, "cluster_alias": plan.ClusterAlias, "namespace": plan.Namespace, "hostname": plan.Hostname,
			"current_job_id": plan.JobID, "release_digest": plan.ReleaseDigest, "config_revision": plan.ConfigRevision,
			"compute_profile": plan.ComputeProfile, "database_mode": databaseMode, "lifecycle_state": lifecycle, "updated_at": now,
		}),
	}).Create(&instance).Error; err != nil {
		return fmt.Errorf("upsert fleet tenant instance: %w", err)
	}

	validityHours := plan.LicenseValidityHours
	if validityHours < 0 {
		return errors.New("license validity hours must be non-negative")
	}
	bindingState := FleetLicenseBindingIntended
	if jobState == TenantDeploymentReady {
		bindingState = FleetLicenseBindingBound
	}
	if jobState == TenantDeploymentDeleted {
		bindingState = FleetLicenseBindingRetired
	}
	binding := FleetLicenseBindingRecord{
		CustomerID: plan.CustomerID, InstanceID: plan.InstanceID, JobID: plan.JobID,
		LicenseID: "", BindingState: bindingState, DesiredRevision: plan.ManifestSHA256,
		ValidityHours: validityHours, RequestRefDigest: plan.LicenseRefDigest,
		ObservedSecretAlias: "secret/router-license", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"customer_id": plan.CustomerID, "job_id": plan.JobID, "binding_state": bindingState,
			"desired_revision": plan.ManifestSHA256, "validity_hours": validityHours,
			"request_ref_digest": plan.LicenseRefDigest, "observed_secret_alias": "secret/router-license",
			"updated_at": now,
		}),
	}).Create(&binding).Error; err != nil {
		return fmt.Errorf("upsert fleet license binding: %w", err)
	}
	return nil
}

func parseLicenseValidityHours(validity string) (int, error) {
	validity = strings.TrimSpace(validity)
	if validity == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(validity)
	if err != nil {
		return 0, fmt.Errorf("license validity is not a duration")
	}
	hours := int(d / time.Hour)
	if hours < 1 {
		hours = 1
	}
	return hours, nil
}

// ListFleetTenants returns all tenants recorded in this registry (operator inventory, not cloud scan).
func (s *TenantDeploymentStore) ListFleetTenants(ctx context.Context) ([]FleetTenantView, error) {
	var rows []FleetTenantRecord
	if err := s.db.WithContext(ctx).Order("customer_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FleetTenantView, 0, len(rows))
	for _, row := range rows {
		out = append(out, fleetTenantViewFromRecord(row))
	}
	return out, nil
}

// GetFleetTenant returns one tenant with instances and registered licenses.
func (s *TenantDeploymentStore) GetFleetTenant(ctx context.Context, customerID string) (FleetTenantView, error) {
	if err := validateDeploymentID("customer_id", customerID); err != nil {
		return FleetTenantView{}, err
	}
	var row FleetTenantRecord
	if err := s.db.WithContext(ctx).Where("customer_id = ?", customerID).First(&row).Error; err != nil {
		return FleetTenantView{}, err
	}
	view := fleetTenantViewFromRecord(row)
	var instances []FleetTenantInstanceRecord
	if err := s.db.WithContext(ctx).Where("customer_id = ?", customerID).Order("instance_id ASC").Find(&instances).Error; err != nil {
		return FleetTenantView{}, err
	}
	for _, instance := range instances {
		view.Instances = append(view.Instances, FleetTenantInstanceView{
			InstanceID: instance.InstanceID, CurrentJobID: instance.CurrentJobID,
			ReleaseDigest: instance.ReleaseDigest, ConfigRevision: instance.ConfigRevision,
			ComputeProfile: instance.ComputeProfile, DatabaseMode: instance.DatabaseMode,
			LifecycleState: instance.LifecycleState, UpdatedAt: instance.UpdatedAt,
		})
	}
	licenses, err := s.ListFleetLicenses(ctx, customerID)
	if err != nil {
		return FleetTenantView{}, err
	}
	view.Licenses = licenses
	return view, nil
}

func fleetTenantViewFromRecord(row FleetTenantRecord) FleetTenantView {
	return FleetTenantView{
		Schema:     "metrum.ai/smartrouter-fleet-tenant/v1",
		CustomerID: row.CustomerID, ProfileID: row.ProfileID, Stage: row.Stage, Environment: row.Environment,
		Namespace: row.Namespace, Hostname: row.Hostname, PrimaryInstanceID: row.PrimaryInstanceID,
		LifecycleState: row.LifecycleState, LatestJobID: row.LatestJobID, LatestJobState: row.LatestJobState,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// ListFleetLicenses returns registered license inventory rows, optionally filtered by customer.
func (s *TenantDeploymentStore) ListFleetLicenses(ctx context.Context, customerID string) ([]FleetLicenseView, error) {
	var rows []FleetLicenseRecord
	q := s.db.WithContext(ctx).Order("customer_id ASC, license_id ASC")
	if strings.TrimSpace(customerID) != "" {
		if err := validateDeploymentID("customer_id", customerID); err != nil {
			return nil, err
		}
		q = q.Where("customer_id = ?", customerID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FleetLicenseView, 0, len(rows))
	for _, row := range rows {
		view, err := s.fleetLicenseView(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// GetFleetLicense returns one registered license inventory row.
func (s *TenantDeploymentStore) GetFleetLicense(ctx context.Context, licenseID string) (FleetLicenseView, error) {
	if err := validateFleetInventoryID("license_id", licenseID); err != nil {
		return FleetLicenseView{}, err
	}
	var row FleetLicenseRecord
	if err := s.db.WithContext(ctx).Where("license_id = ?", licenseID).First(&row).Error; err != nil {
		return FleetLicenseView{}, err
	}
	return s.fleetLicenseView(ctx, row)
}

func (s *TenantDeploymentStore) fleetLicenseView(ctx context.Context, row FleetLicenseRecord) (FleetLicenseView, error) {
	var features []fleetLicenseFeatureRecord
	if err := s.db.WithContext(ctx).Where("license_id = ?", row.LicenseID).Order("seq ASC").Find(&features).Error; err != nil {
		return FleetLicenseView{}, err
	}
	names := make([]string, 0, len(features))
	for _, feature := range features {
		names = append(names, feature.Feature)
	}
	grace := time.Time{}
	if !row.GraceUntil.IsZero() {
		grace = row.GraceUntil
	}
	return FleetLicenseView{
		LicenseID: row.LicenseID, CustomerID: row.CustomerID, CustomerName: row.CustomerName,
		Product: row.Product, SKU: row.SKU, KeyID: row.KeyID, Issuer: row.Issuer, Status: row.Status,
		IssuedAt: row.IssuedAt, NotBefore: row.NotBefore, ExpiresAt: row.ExpiresAt, GraceUntil: grace,
		PayloadSHA256: row.PayloadSHA256, Features: names, UpdatedAt: row.UpdatedAt,
	}, nil
}

// RegisterFleetLicenseFromSafeSummary imports router-license safe-summary JSON into the Fleet registry.
// It never accepts or stores signed envelopes, private keys, or protected request refs.
func (s *TenantDeploymentStore) RegisterFleetLicenseFromSafeSummary(ctx context.Context, summary LicenseSafeSummary, payloadSHA256, actorRole string) (FleetLicenseView, error) {
	if err := validateFleetLicenseSummary(summary); err != nil {
		return FleetLicenseView{}, err
	}
	issuedAt, err := parseFleetSummaryTime("issued_at", summary.IssuedAt)
	if err != nil {
		return FleetLicenseView{}, err
	}
	notBefore, err := parseFleetSummaryTime("not_before", summary.NotBefore)
	if err != nil {
		return FleetLicenseView{}, err
	}
	expiresAt, err := parseFleetSummaryTime("expires_at", summary.ExpiresAt)
	if err != nil {
		return FleetLicenseView{}, err
	}
	var graceUntil time.Time
	if strings.TrimSpace(summary.GraceUntil) != "" {
		graceUntil, err = parseFleetSummaryTime("grace_until", summary.GraceUntil)
		if err != nil {
			return FleetLicenseView{}, err
		}
	}
	payloadDigest := strings.ToLower(strings.TrimSpace(payloadSHA256))
	if payloadDigest != "" {
		if len(payloadDigest) != 64 {
			return FleetLicenseView{}, errors.New("payload_sha256 must be a 64-hex digest")
		}
		if _, err := hex.DecodeString(payloadDigest); err != nil {
			return FleetLicenseView{}, errors.New("payload_sha256 must be a 64-hex digest")
		}
	}
	if strings.TrimSpace(actorRole) == "" {
		actorRole = "fleet-lifecycle-admin"
	}
	if err := validateFleetInventoryID("actor_role", actorRole); err != nil {
		return FleetLicenseView{}, err
	}
	now := time.Now().UTC()
	record := FleetLicenseRecord{
		LicenseID: summary.LicenseID, CustomerID: summary.CustomerID, CustomerName: strings.TrimSpace(summary.CustomerName),
		Product: summary.Product, SKU: summary.SKU, KeyID: summary.KeyID, Issuer: summary.Issuer,
		SchemaVersion: summary.SchemaVersion, Status: FleetLicenseStatusRegistered,
		IssuedAt: issuedAt, NotBefore: notBefore, ExpiresAt: expiresAt, GraceUntil: graceUntil,
		PayloadSHA256:  payloadDigest,
		MaxModelGroups: summary.Limits.MaxModelGroups, MaxCallers: summary.Limits.MaxCallers,
		MaxMonthlyRequests: summary.Limits.MaxMonthlyRequests, MaxTotalTokens: summary.Limits.MaxTotalTokens,
		MaxTotalRequests: summary.Limits.MaxTotalRequests, MaxConcurrent: summary.Limits.MaxConcurrent,
		MaxAdmins: summary.Limits.MaxAdmins, MaxRetentionDays: summary.Limits.MaxRetentionDays,
		MaxInstances:   summary.Limits.MaxInstances,
		DeploymentMode: strings.TrimSpace(summary.Deployment.Mode), DeploymentID: strings.TrimSpace(summary.Deployment.DeploymentID),
		CreatedAt: now, UpdatedAt: now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "license_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"customer_id": record.CustomerID, "customer_name": record.CustomerName, "product": record.Product,
				"sku": record.SKU, "key_id": record.KeyID, "issuer": record.Issuer, "schema_version": record.SchemaVersion,
				"status": FleetLicenseStatusRegistered, "issued_at": record.IssuedAt, "not_before": record.NotBefore,
				"expires_at": record.ExpiresAt, "grace_until": record.GraceUntil, "payload_sha256": record.PayloadSHA256,
				"max_model_groups": record.MaxModelGroups, "max_callers": record.MaxCallers,
				"max_monthly_requests": record.MaxMonthlyRequests, "max_total_tokens": record.MaxTotalTokens,
				"max_total_requests": record.MaxTotalRequests, "max_concurrent": record.MaxConcurrent,
				"max_admins": record.MaxAdmins, "max_retention_days": record.MaxRetentionDays, "max_instances": record.MaxInstances,
				"deployment_mode": record.DeploymentMode, "deployment_id": record.DeploymentID, "updated_at": now,
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Where("license_id = ?", record.LicenseID).Delete(&fleetLicenseFeatureRecord{}).Error; err != nil {
			return err
		}
		for i, feature := range summary.Features {
			if err := validateFleetInventoryID("feature", feature); err != nil {
				return err
			}
			if err := tx.Create(&fleetLicenseFeatureRecord{LicenseID: record.LicenseID, Seq: i + 1, Feature: feature}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("license_id = ?", record.LicenseID).Delete(&fleetLicenseAllowedSkinRecord{}).Error; err != nil {
			return err
		}
		for i, skin := range summary.Limits.AllowedSkins {
			if err := validateFleetInventoryID("allowed_skin", skin); err != nil {
				return err
			}
			if err := tx.Create(&fleetLicenseAllowedSkinRecord{LicenseID: record.LicenseID, Seq: i + 1, Skin: skin}).Error; err != nil {
				return err
			}
		}
		var count int64
		if err := tx.Model(&fleetLicenseEventRecord{}).Where("license_id = ?", record.LicenseID).Count(&count).Error; err != nil {
			return err
		}
		return tx.Create(&fleetLicenseEventRecord{
			LicenseID: record.LicenseID, Seq: int(count) + 1, EventName: "registered",
			ActorRole: actorRole, JobID: "", SafeReason: "safe-summary-import", CreatedAt: now,
		}).Error
	})
	if err != nil {
		return FleetLicenseView{}, err
	}
	// Bind registered license onto matching intended/bound instance bindings for this customer when empty.
	_ = s.db.WithContext(ctx).Model(&FleetLicenseBindingRecord{}).
		Where("customer_id = ? AND license_id = '' AND binding_state IN ?", summary.CustomerID, []string{FleetLicenseBindingIntended, FleetLicenseBindingBound}).
		Updates(map[string]any{"license_id": summary.LicenseID, "updated_at": now}).Error
	return s.GetFleetLicense(ctx, summary.LicenseID)
}

func validateFleetLicenseSummary(summary LicenseSafeSummary) error {
	for name, value := range map[string]string{
		"license_id": summary.LicenseID, "customer_id": summary.CustomerID, "product": summary.Product,
		"sku": summary.SKU, "key_id": summary.KeyID, "issuer": summary.Issuer,
	} {
		if err := validateFleetInventoryID(name, value); err != nil {
			return err
		}
	}
	if summary.SchemaVersion < 1 {
		return errors.New("schema_version must be >= 1")
	}
	if name := strings.TrimSpace(summary.CustomerName); name != "" {
		if len(name) > 120 || strings.ContainsAny(name, "\n\r{}") || strings.Contains(name, "://") {
			return errors.New("customer_name must be a bounded non-secret display string")
		}
	}
	if mode := strings.TrimSpace(summary.Deployment.Mode); mode != "" {
		if err := validateFleetInventoryID("deployment_mode", mode); err != nil {
			return err
		}
	}
	if id := strings.TrimSpace(summary.Deployment.DeploymentID); id != "" {
		if err := validateFleetInventoryID("deployment_id", id); err != nil {
			return err
		}
	}
	return nil
}

var fleetInventoryIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,90}$`)

func validateFleetInventoryID(name, value string) error {
	value = strings.TrimSpace(value)
	if !fleetInventoryIDPattern.MatchString(value) || strings.Contains(strings.ToLower(value), "secret") || strings.Contains(strings.ToLower(value), "token") {
		return fmt.Errorf("%s must be a lowercase inventory identifier", name)
	}
	return nil
}

func parseFleetSummaryTime(name, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("%s is required", name)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339", name)
	}
	return parsed.UTC(), nil
}

// LoadFleetLicenseSafeSummaryFile loads mode-0600 safe-summary JSON produced by router-license.
func LoadFleetLicenseSafeSummaryFile(path string) (LicenseSafeSummary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return LicenseSafeSummary{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return LicenseSafeSummary{}, errors.New("license summary file must be mode 0600")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return LicenseSafeSummary{}, err
	}
	if err := rejectSecretShapedDeploymentData(raw); err != nil {
		return LicenseSafeSummary{}, err
	}
	var summary LicenseSafeSummary
	if err := decodeStrictDeploymentDocument(raw, path, &summary); err != nil {
		return LicenseSafeSummary{}, errors.New("license summary schema is invalid")
	}
	return summary, nil
}

func digestProtectedRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// SyncFleetInventoryFromJobs rebuilds tenant/instance inventory from existing job rows
// for registries created before inventory tables existed.
func (s *TenantDeploymentStore) SyncFleetInventoryFromJobs(ctx context.Context) (int, error) {
	var jobs []tenantDeploymentJobRecord
	if err := s.db.WithContext(ctx).Order("updated_at ASC").Find(&jobs).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, job := range jobs {
		plan := TenantDeploymentPlan{
			JobID: job.JobID, InstanceID: job.InstanceID, ProfileID: job.ProfileID, CustomerID: job.CustomerID,
			Stage: job.Stage, Environment: job.Environment, Region: job.Region, ClusterAlias: job.ClusterAlias,
			Namespace: job.Namespace, Hostname: job.Hostname, ReleaseDigest: job.ReleaseDigest,
			ConfigRevision: job.ConfigRevision, ComputeProfile: job.ComputeProfile, ManifestSHA256: job.ManifestSHA256,
		}
		if err := s.upsertFleetInventoryFromPlan(ctx, plan, job.State); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
