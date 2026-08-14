package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	TenantDeploymentRequested        = "requested"
	TenantDeploymentProvisioning     = "provisioning"
	TenantDeploymentValidating       = "validating"
	TenantDeploymentReady            = "ready"
	TenantDeploymentFailed           = "failed"
	TenantDeploymentDeleted          = "deleted"
	TenantDeploymentOperatorRequired = "operator_required"
)

var tenantDeploymentProcessLock sync.Mutex

var tenantDeploymentActionOrder = []string{
	"namespace", "network_policy", "dedicated_rds", "runtime_secret_binding",
	"license_binding", "state_pvc", "router", "activation", "hostname",
}

// TenantDeploymentStatus is the bounded, non-secret lifecycle view returned by
// deploy and status. It intentionally omits endpoints other than the caller
// hostname, credential references, error text, and provider configuration.
type TenantDeploymentStatus struct {
	Schema           string                          `json:"schema"`
	Mode             string                          `json:"mode"`
	JobID            string                          `json:"job_id"`
	InstanceID       string                          `json:"instance_id"`
	ProfileID        string                          `json:"profile_id"`
	CustomerID       string                          `json:"customer_id"`
	Stage            string                          `json:"stage"`
	Environment      string                          `json:"environment"`
	Region           string                          `json:"region"`
	ClusterAlias     string                          `json:"cluster_alias"`
	Namespace        string                          `json:"namespace"`
	Hostname         string                          `json:"hostname,omitempty"`
	ReleaseDigest    string                          `json:"release_digest"`
	ResourceProfile  string                          `json:"resource_profile,omitempty"`
	StateProfile     string                          `json:"state_profile,omitempty"`
	ComputeProfile   string                          `json:"compute_profile,omitempty"`
	NodeClassAlias   string                          `json:"node_class_alias,omitempty"`
	Architecture     string                          `json:"architecture,omitempty"`
	CPURequest       string                          `json:"cpu_request,omitempty"`
	CPULimit         string                          `json:"cpu_limit,omitempty"`
	MemoryRequest    string                          `json:"memory_request,omitempty"`
	MemoryLimit      string                          `json:"memory_limit,omitempty"`
	ConfigRevision   string                          `json:"config_revision"`
	State            string                          `json:"state"`
	ObservedState    string                          `json:"observed_state,omitempty"`
	CompletedAction  string                          `json:"completed_action,omitempty"`
	NextAction       string                          `json:"next_action,omitempty"`
	ErrorClass       string                          `json:"error_class,omitempty"`
	Retryable        bool                            `json:"retryable"`
	CreatedAt        time.Time                       `json:"created_at"`
	UpdatedAt        time.Time                       `json:"updated_at"`
	ActivationPassed bool                            `json:"activation_passed"`
	DatabaseState    string                          `json:"database_state,omitempty"`
	Workload         *TenantDeploymentWorkloadStatus `json:"workload,omitempty"`
	PVC              *TenantDeploymentResourceStatus `json:"pvc,omitempty"`
	Ingress          *TenantDeploymentResourceStatus `json:"ingress,omitempty"`
	Service          *TenantDeploymentResourceStatus `json:"service,omitempty"`
	Database         *TenantDeploymentDatabaseStatus `json:"database,omitempty"`
}

const (
	TenantOwnershipOwned               = "owned"
	TenantOwnershipExpectedMissing     = "expected_missing"
	TenantOwnershipNotApplicable       = "not_applicable"
	TenantOwnershipForeignOrUnverified = "foreign_or_unverified"
	TenantOwnershipAccessDenied        = "access_denied"
)

// TenantDeploymentWorkloadStatus is safe replica/pod aggregate evidence for one exact job.
type TenantDeploymentWorkloadStatus struct {
	DesiredReplicas   int32          `json:"desired_replicas"`
	ReadyReplicas     int32          `json:"ready_replicas"`
	AvailableReplicas int32          `json:"available_replicas"`
	PodPhaseCounts    map[string]int `json:"pod_phase_counts,omitempty"`
	RestartCount      int32          `json:"restart_count"`
	Ownership         string         `json:"ownership"`
}

// TenantDeploymentResourceStatus is ownership-scoped existence/readiness for one Fleet resource.
type TenantDeploymentResourceStatus struct {
	NameAlias         string `json:"name_alias,omitempty"`
	Phase             string `json:"phase,omitempty"`
	StorageClassAlias string `json:"storage_class_alias,omitempty"`
	CapacityBucket    string `json:"capacity_bucket,omitempty"`
	ReadyClass        string `json:"ready_class,omitempty"`
	Ownership         string `json:"ownership"`
}

// TenantDeploymentDatabaseStatus is dedicated-RDS-only scalar evidence.
type TenantDeploymentDatabaseStatus struct {
	DatabaseIDAlias        string `json:"database_id_alias,omitempty"`
	EngineClass            string `json:"engine_class,omitempty"`
	Status                 string `json:"status,omitempty"`
	OwnershipBindingResult string `json:"ownership_binding_result,omitempty"`
	Ownership              string `json:"ownership"`
}

type tenantDeploymentJobRecord struct {
	JobID            string    `gorm:"primaryKey;column:job_id;type:text"`
	InstanceID       string    `gorm:"index;column:instance_id;type:text;not null"`
	IdempotencyKey   string    `gorm:"uniqueIndex;column:idempotency_key;type:text;not null"`
	ManifestSHA256   string    `gorm:"column:manifest_sha256;type:text;not null"`
	ProfileID        string    `gorm:"column:profile_id;type:text;not null"`
	CustomerID       string    `gorm:"column:customer_id;type:text;not null"`
	Stage            string    `gorm:"column:stage;type:text;not null"`
	Environment      string    `gorm:"column:environment;type:text;not null"`
	Region           string    `gorm:"column:region;type:text;not null"`
	ClusterAlias     string    `gorm:"column:cluster_alias;type:text;not null"`
	Namespace        string    `gorm:"column:namespace;type:text;not null"`
	Hostname         string    `gorm:"column:hostname;type:text;not null"`
	ReleaseDigest    string    `gorm:"column:release_digest;type:text;not null"`
	ResourceProfile  string    `gorm:"column:resource_profile;type:text;not null"`
	StateProfile     string    `gorm:"column:state_profile;type:text;not null"`
	ComputeProfile   string    `gorm:"column:compute_profile;type:text;not null"`
	ConfigRevision   string    `gorm:"column:config_revision;type:text;not null"`
	State            string    `gorm:"column:state;type:text;not null"`
	CompletedAction  string    `gorm:"column:completed_action;type:text;not null"`
	NextAction       string    `gorm:"column:next_action;type:text;not null"`
	ErrorClass       string    `gorm:"column:error_class;type:text;not null"`
	Retryable        bool      `gorm:"column:retryable;not null"`
	ActivationPassed bool      `gorm:"column:activation_passed;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
	UpdatedAt        time.Time `gorm:"column:updated_at;not null"`
}

func (tenantDeploymentJobRecord) TableName() string { return "tenant_deployment_jobs" }

type tenantDeploymentAttemptRecord struct {
	ID          uint      `gorm:"primaryKey;column:id"`
	JobID       string    `gorm:"uniqueIndex:job_action_attempt;column:job_id;type:text;not null"`
	Action      string    `gorm:"uniqueIndex:job_action_attempt;column:action;type:text;not null"`
	Attempt     int       `gorm:"uniqueIndex:job_action_attempt;column:attempt;not null"`
	State       string    `gorm:"column:state;type:text;not null"`
	ErrorClass  string    `gorm:"column:error_class;type:text;not null"`
	StartedAt   time.Time `gorm:"column:started_at;not null"`
	CompletedAt time.Time `gorm:"column:completed_at"`
}

func (tenantDeploymentAttemptRecord) TableName() string { return "tenant_deployment_attempts" }

type tenantDeploymentResourceRecord struct {
	ID              uint      `gorm:"primaryKey;column:id"`
	InstanceID      string    `gorm:"uniqueIndex:instance_resource;column:instance_id;type:text;not null"`
	JobID           string    `gorm:"index;column:job_id;type:text;not null"`
	ResourceKind    string    `gorm:"uniqueIndex:instance_resource;column:resource_kind;type:text;not null"`
	ResourceRef     string    `gorm:"uniqueIndex;column:resource_ref;type:text;not null"`
	OwnershipKey    string    `gorm:"uniqueIndex;column:ownership_key;type:text;not null"`
	DesiredRevision string    `gorm:"column:desired_revision;type:text;not null"`
	State           string    `gorm:"column:state;type:text;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (tenantDeploymentResourceRecord) TableName() string { return "tenant_deployment_resources" }

type tenantDeploymentApprovalRecord struct {
	ApprovalSHA256 string     `gorm:"primaryKey;column:approval_sha256;type:text"`
	JobID          string     `gorm:"column:job_id;type:text;not null"`
	Action         string     `gorm:"column:action;type:text;not null"`
	ExpiresAt      time.Time  `gorm:"column:expires_at;not null"`
	ConsumedAt     *time.Time `gorm:"column:consumed_at"`
	RetainPVC      bool       `gorm:"column:retain_pvc;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
}

func (tenantDeploymentApprovalRecord) TableName() string { return "tenant_deployment_approvals" }

// TenantDeploymentStore is a normalized local lifecycle registry. It shares no
// schema or mutable state with Router request usage.
type TenantDeploymentStore struct{ db *gorm.DB }

func openTenantRegistrySQLite(path, mode string) (*gorm.DB, error) {
	dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: "mode=" + mode + "&_pragma=foreign_keys(1)"}).String()
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func validateTenantRegistryPath(path string) error {
	if strings.TrimSpace(path) == "" || strings.Contains(path, "?") || strings.HasPrefix(path, "file:") || path == ":memory:" {
		return errors.New("deployment registry must use a local SQLite file path without URI options")
	}
	return nil
}

func safeRegistryScalar(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 160 || strings.ContainsAny(v, "\n\r{}?@") || strings.Contains(v, "://") {
		return false
	}
	lower := strings.ToLower(v)
	if strings.Contains(lower, "dsn") || strings.Contains(lower, "token") {
		return false
	}
	// Kubernetes object refs use kind/name (including secret/...). Reject other
	// secret-shaped scalars that are not an owned object reference.
	if strings.Contains(lower, "secret") && !strings.HasPrefix(lower, "secret/") {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:/", r)) {
			return false
		}
	}
	return true
}

func OpenTenantDeploymentStore(path string) (*TenantDeploymentStore, error) {
	if err := validateTenantRegistryPath(path); err != nil {
		return nil, err
	}
	if err := ensureSQLitePrivateMode(path); err != nil {
		return nil, err
	}
	db, err := openTenantRegistrySQLite(path, "rwc")
	if err != nil {
		return nil, err
	}
	if err := chmodSQLiteFiles(path); err != nil {
		closeGormDB(db)
		return nil, err
	}
	store := &TenantDeploymentStore{db: db}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS tenant_deployment_jobs (job_id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, idempotency_key TEXT NOT NULL UNIQUE, manifest_sha256 TEXT NOT NULL, profile_id TEXT NOT NULL, customer_id TEXT NOT NULL, stage TEXT NOT NULL, environment TEXT NOT NULL, region TEXT NOT NULL, cluster_alias TEXT NOT NULL, namespace TEXT NOT NULL, hostname TEXT NOT NULL, release_digest TEXT NOT NULL, resource_profile TEXT NOT NULL, state_profile TEXT NOT NULL, compute_profile TEXT NOT NULL DEFAULT '', config_revision TEXT NOT NULL, state TEXT NOT NULL, completed_action TEXT NOT NULL, next_action TEXT NOT NULL, error_class TEXT NOT NULL, retryable NUMERIC NOT NULL, activation_passed NUMERIC NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_deployment_jobs_instance_id ON tenant_deployment_jobs(instance_id)`,
		`CREATE TABLE IF NOT EXISTS tenant_deployment_attempts (id INTEGER PRIMARY KEY AUTOINCREMENT, job_id TEXT NOT NULL, action TEXT NOT NULL, attempt INTEGER NOT NULL, state TEXT NOT NULL, error_class TEXT NOT NULL, started_at DATETIME NOT NULL, completed_at DATETIME, UNIQUE(job_id, action, attempt), FOREIGN KEY(job_id) REFERENCES tenant_deployment_jobs(job_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
		`CREATE TABLE IF NOT EXISTS tenant_deployment_resources (id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL, job_id TEXT NOT NULL, resource_kind TEXT NOT NULL, resource_ref TEXT NOT NULL UNIQUE, ownership_key TEXT NOT NULL UNIQUE, desired_revision TEXT NOT NULL, state TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(instance_id, resource_kind), FOREIGN KEY(job_id) REFERENCES tenant_deployment_jobs(job_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_deployment_resources_job_id ON tenant_deployment_resources(job_id)`,
		`CREATE TABLE IF NOT EXISTS tenant_deployment_approvals (approval_sha256 TEXT PRIMARY KEY, job_id TEXT NOT NULL, action TEXT NOT NULL, expires_at DATETIME NOT NULL, consumed_at DATETIME, retain_pvc NUMERIC NOT NULL, created_at DATETIME NOT NULL, FOREIGN KEY(job_id) REFERENCES tenant_deployment_jobs(job_id) ON UPDATE CASCADE ON DELETE RESTRICT)`,
	}
	statements = append(statements, fleetInventoryDDL()...)
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("initialize tenant deployment store: %w", err)
		}
	}
	// Backward-compatible column for registries created before compute profiles.
	_ = db.Exec(`ALTER TABLE tenant_deployment_jobs ADD COLUMN compute_profile TEXT NOT NULL DEFAULT ''`).Error
	return store, nil
}

func OpenTenantDeploymentStoreReadOnly(path string) (*TenantDeploymentStore, error) {
	if err := validateTenantRegistryPath(path); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open read-only tenant deployment store: %w", err)
	}
	db, err := openTenantRegistrySQLite(path, "ro")
	if err != nil {
		return nil, err
	}
	return &TenantDeploymentStore{db: db}, nil
}

func closeGormDB(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func (s *TenantDeploymentStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *TenantDeploymentStore) createOrLoad(ctx context.Context, plan TenantDeploymentPlan, intentID string) (tenantDeploymentJobRecord, error) {
	now := time.Now().UTC()
	idempotencySum := sha256.Sum256([]byte(plan.ProfileID + "\x00" + plan.CustomerID + "\x00" + plan.Stage + "\x00" + intentID))
	record := tenantDeploymentJobRecord{
		JobID: plan.JobID, InstanceID: plan.InstanceID, IdempotencyKey: hex.EncodeToString(idempotencySum[:]), ManifestSHA256: plan.ManifestSHA256,
		ProfileID: plan.ProfileID, CustomerID: plan.CustomerID, Stage: plan.Stage, Environment: plan.Environment,
		Region: plan.Region, ClusterAlias: plan.ClusterAlias, Namespace: plan.Namespace, Hostname: plan.Hostname,
		ReleaseDigest: plan.ReleaseDigest, ResourceProfile: plan.ResourceProfile, StateProfile: plan.StateProfile,
		ComputeProfile: plan.ComputeProfile, ConfigRevision: plan.ConfigRevision, State: TenantDeploymentRequested, NextAction: plan.Actions[0],
		CreatedAt: now, UpdatedAt: now,
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return tenantDeploymentJobRecord{}, result.Error
	}
	var existing tenantDeploymentJobRecord
	if err := s.db.WithContext(ctx).Where("job_id = ?", plan.JobID).First(&existing).Error; err != nil {
		return tenantDeploymentJobRecord{}, err
	}
	if existing.ManifestSHA256 != plan.ManifestSHA256 || existing.IdempotencyKey != record.IdempotencyKey {
		return tenantDeploymentJobRecord{}, errors.New("idempotency conflict: intent already exists with different desired state")
	}
	if err := s.upsertFleetInventoryFromPlan(ctx, plan, existing.State); err != nil {
		return tenantDeploymentJobRecord{}, err
	}
	return existing, nil
}

func (s *TenantDeploymentStore) Status(ctx context.Context, jobID, profileID string) (TenantDeploymentStatus, error) {
	if err := validateDeploymentID("job_id", jobID); err != nil {
		return TenantDeploymentStatus{}, err
	}
	if err := validateDeploymentID("profile_id", profileID); err != nil {
		return TenantDeploymentStatus{}, err
	}
	var record tenantDeploymentJobRecord
	if err := s.db.WithContext(ctx).Where("job_id = ? AND profile_id = ?", jobID, profileID).First(&record).Error; err != nil {
		return TenantDeploymentStatus{}, err
	}
	status := statusFromDeploymentRecord(record)
	var database tenantDeploymentResourceRecord
	if err := s.db.WithContext(ctx).Where("instance_id = ? AND resource_kind = ?", record.InstanceID, "dedicated_rds").First(&database).Error; err == nil {
		status.DatabaseState = database.State
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return TenantDeploymentStatus{}, err
	}
	return status, nil
}

func statusFromDeploymentRecord(record tenantDeploymentJobRecord) TenantDeploymentStatus {
	hostname := ""
	if record.ActivationPassed && record.State == TenantDeploymentReady {
		hostname = record.Hostname
	}
	return TenantDeploymentStatus{
		Schema: "metrum.ai/smartrouter-deployment-status/v1", Mode: "eks", JobID: record.JobID,
		InstanceID: record.InstanceID,
		ProfileID:  record.ProfileID, CustomerID: record.CustomerID, Stage: record.Stage, Environment: record.Environment,
		Region: record.Region, ClusterAlias: record.ClusterAlias, Namespace: record.Namespace, Hostname: hostname,
		ReleaseDigest: record.ReleaseDigest, ResourceProfile: record.ResourceProfile, StateProfile: record.StateProfile,
		ComputeProfile: record.ComputeProfile, ConfigRevision: record.ConfigRevision, State: record.State,
		CompletedAction: record.CompletedAction, NextAction: record.NextAction, ErrorClass: record.ErrorClass,
		Retryable: record.Retryable, ActivationPassed: record.ActivationPassed,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// Tenant deployment adapters are split by authority so no implementation can
// accidentally turn a generic command runner into a second provisioner.
type TenantNamespaceAdapter interface {
	EnsureNamespace(context.Context, TenantDeploymentPlan) (string, error)
	DeleteNamespace(context.Context, TenantDeploymentPlan, string) error
}
type TenantNetworkPolicyAdapter interface {
	EnsureNetworkPolicy(context.Context, TenantDeploymentPlan) (string, error)
	DeleteNetworkPolicy(context.Context, TenantDeploymentPlan, string) error
}
type TenantSecretBindingAdapter interface {
	EnsureSecretBinding(context.Context, TenantDeploymentPlan) (string, error)
	DeleteSecretBinding(context.Context, TenantDeploymentPlan, string) error
}
type TenantLicenseBindingAdapter interface {
	EnsureLicenseBinding(context.Context, TenantDeploymentPlan) (string, error)
	DeleteLicenseBinding(context.Context, TenantDeploymentPlan, string) error
}
type TenantStateAdapter interface {
	EnsureStatePVC(context.Context, TenantDeploymentPlan) (string, error)
	DeleteStatePVC(context.Context, TenantDeploymentPlan, string) error
}
type TenantDatabaseAdapter interface {
	EnsureDedicatedRDS(context.Context, TenantDeploymentPlan) (string, error)
	DeleteDedicatedRDS(context.Context, TenantDeploymentPlan, string) error
}
type TenantRouterAdapter interface {
	EnsureRouter(context.Context, TenantDeploymentPlan) (string, error)
	DeleteRouter(context.Context, TenantDeploymentPlan, string) error
}
type TenantActivationAdapter interface {
	ValidateActivation(context.Context, TenantDeploymentPlan) (string, error)
	DeleteActivation(context.Context, TenantDeploymentPlan, string) error
}
type TenantHostnameAdapter interface {
	EnableHostname(context.Context, TenantDeploymentPlan) (string, error)
	DisableHostname(context.Context, TenantDeploymentPlan, string) error
}

type TenantDeploymentAdapters struct {
	Namespace      TenantNamespaceAdapter
	NetworkPolicy  TenantNetworkPolicyAdapter
	SecretBinding  TenantSecretBindingAdapter
	LicenseBinding TenantLicenseBindingAdapter
	State          TenantStateAdapter
	Database       TenantDatabaseAdapter
	Router         TenantRouterAdapter
	Activation     TenantActivationAdapter
	Hostname       TenantHostnameAdapter
}

// TenantDeploymentEngine owns the single local lifecycle. Live implementations
// must satisfy these adapters rather than creating another registry or CLI.
type TenantDeploymentEngine struct {
	store    *TenantDeploymentStore
	adapters TenantDeploymentAdapters
}

func NewTenantDeploymentEngine(store *TenantDeploymentStore, adapters TenantDeploymentAdapters) (*TenantDeploymentEngine, error) {
	if store == nil || adapters.Namespace == nil || adapters.NetworkPolicy == nil || adapters.SecretBinding == nil || adapters.LicenseBinding == nil || adapters.State == nil || adapters.Database == nil || adapters.Router == nil || adapters.Activation == nil || adapters.Hostname == nil {
		return nil, errors.New("complete tenant deployment adapter set is required")
	}
	return &TenantDeploymentEngine{store: store, adapters: adapters}, nil
}

func (e *TenantDeploymentEngine) Deploy(ctx context.Context, plan TenantDeploymentPlan, intentID string) (TenantDeploymentStatus, error) {
	tenantDeploymentProcessLock.Lock()
	defer tenantDeploymentProcessLock.Unlock()
	record, err := e.store.createOrLoad(ctx, plan, intentID)
	if err != nil {
		return TenantDeploymentStatus{}, err
	}
	if record.State == TenantDeploymentReady {
		return statusFromDeploymentRecord(record), nil
	}
	if record.State == TenantDeploymentDeleted {
		return statusFromDeploymentRecord(record), errors.New("deleted deployment cannot be resumed with the same intent")
	}
	if record.State == TenantDeploymentOperatorRequired {
		return statusFromDeploymentRecord(record), errors.New("deployment requires operator reconciliation before resume")
	}
	for index, action := range plan.Actions {
		resource, found, err := e.resource(ctx, record.InstanceID, action)
		if err != nil {
			return TenantDeploymentStatus{}, err
		}
		if found && resource.DesiredRevision == plan.ManifestSHA256 && (resource.State == "ready" || resource.State == "retained") {
			continue
		}
		state := TenantDeploymentProvisioning
		if action == "activation" || action == "hostname" {
			state = TenantDeploymentValidating
		}
		if err := e.updateJob(ctx, record.JobID, state, previousAction(plan.Actions, index), action, "", false, action == "hostname"); err != nil {
			return TenantDeploymentStatus{}, err
		}
		attempt, err := e.startAttempt(ctx, record.JobID, action)
		if err != nil {
			return TenantDeploymentStatus{}, err
		}
		resourceRef, ensureErr := e.ensure(ctx, action, plan)
		if ensureErr != nil {
			errorClass := classifyTenantDeploymentError(ensureErr)
			operatorRequired := tenantDeploymentErrorNeedsOperator(ensureErr)
			if operatorRequired && action != "state_pvc" && action != "dedicated_rds" {
				durable, durableErr := e.hasDurableState(ctx, record.InstanceID)
				if durableErr != nil {
					return TenantDeploymentStatus{}, durableErr
				}
				operatorRequired = durable
			}
			failedState := TenantDeploymentFailed
			nextAction := action
			if operatorRequired {
				failedState = TenantDeploymentOperatorRequired
				nextAction = "operator_review"
			}
			_ = e.finishAttempt(ctx, attempt.ID, "failed", errorClass)
			_ = e.updateJob(ctx, record.JobID, failedState, previousAction(plan.Actions, index), nextAction, errorClass, !operatorRequired, false)
			status, statusErr := e.store.Status(ctx, record.JobID, record.ProfileID)
			if statusErr != nil {
				return TenantDeploymentStatus{}, statusErr
			}
			return status, fmt.Errorf("deployment action %s failed: %s", action, errorClass)
		}
		if err := e.persistResource(ctx, record.JobID, action, resourceRef, plan); err != nil {
			return TenantDeploymentStatus{}, err
		}
		if err := e.finishAttempt(ctx, attempt.ID, "succeeded", ""); err != nil {
			return TenantDeploymentStatus{}, err
		}
		if action == "activation" {
			if err := e.setActivationPassed(ctx, record.JobID); err != nil {
				return TenantDeploymentStatus{}, err
			}
		}
	}
	if err := e.updateJob(ctx, record.JobID, TenantDeploymentReady, "hostname", "", "", false, false); err != nil {
		return TenantDeploymentStatus{}, err
	}
	if err := e.store.upsertFleetInventoryFromPlan(ctx, plan, TenantDeploymentReady); err != nil {
		return TenantDeploymentStatus{}, err
	}
	return e.store.Status(ctx, record.JobID, record.ProfileID)
}

func previousAction(actions []string, index int) string {
	if index == 0 {
		return ""
	}
	return actions[index-1]
}

func (e *TenantDeploymentEngine) resource(ctx context.Context, instanceID, kind string) (tenantDeploymentResourceRecord, bool, error) {
	var record tenantDeploymentResourceRecord
	err := e.store.db.WithContext(ctx).Where("instance_id = ? AND resource_kind = ?", instanceID, kind).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tenantDeploymentResourceRecord{}, false, nil
	}
	return record, err == nil, err
}

func (e *TenantDeploymentEngine) hasDurableState(ctx context.Context, instanceID string) (bool, error) {
	var count int64
	err := e.store.db.WithContext(ctx).Model(&tenantDeploymentResourceRecord{}).
		Where("instance_id = ? AND resource_kind IN ? AND state = ?", instanceID, []string{"state_pvc", "dedicated_rds"}, "ready").
		Count(&count).Error
	return count > 0, err
}

func (e *TenantDeploymentEngine) updateJob(ctx context.Context, jobID, state, completed, next, errorClass string, retryable, activationPassed bool) error {
	updates := map[string]any{"state": state, "completed_action": completed, "next_action": next, "error_class": errorClass, "retryable": retryable, "updated_at": time.Now().UTC()}
	if activationPassed {
		updates["activation_passed"] = true
	}
	return e.store.db.WithContext(ctx).Model(&tenantDeploymentJobRecord{}).Where("job_id = ?", jobID).Updates(updates).Error
}

func (e *TenantDeploymentEngine) setActivationPassed(ctx context.Context, jobID string) error {
	return e.store.db.WithContext(ctx).Model(&tenantDeploymentJobRecord{}).Where("job_id = ?", jobID).Updates(map[string]any{"activation_passed": true, "updated_at": time.Now().UTC()}).Error
}

func (e *TenantDeploymentEngine) startAttempt(ctx context.Context, jobID, action string) (tenantDeploymentAttemptRecord, error) {
	var count int64
	if err := e.store.db.WithContext(ctx).Model(&tenantDeploymentAttemptRecord{}).Where("job_id = ? AND action = ?", jobID, action).Count(&count).Error; err != nil {
		return tenantDeploymentAttemptRecord{}, err
	}
	record := tenantDeploymentAttemptRecord{JobID: jobID, Action: action, Attempt: int(count) + 1, State: "running", StartedAt: time.Now().UTC()}
	return record, e.store.db.WithContext(ctx).Create(&record).Error
}

func (e *TenantDeploymentEngine) finishAttempt(ctx context.Context, id uint, state, errorClass string) error {
	return e.store.db.WithContext(ctx).Model(&tenantDeploymentAttemptRecord{}).Where("id = ?", id).Updates(map[string]any{"state": state, "error_class": errorClass, "completed_at": time.Now().UTC()}).Error
}

func (e *TenantDeploymentEngine) persistResource(ctx context.Context, jobID, kind, resourceRef string, plan TenantDeploymentPlan) error {
	if !safeRegistryScalar(resourceRef) {
		return errors.New("adapter returned an unsafe resource reference")
	}
	now := time.Now().UTC()
	record := tenantDeploymentResourceRecord{InstanceID: plan.InstanceID, JobID: jobID, ResourceKind: kind, ResourceRef: resourceRef, OwnershipKey: plan.InstanceID + ":" + kind, DesiredRevision: plan.ManifestSHA256, State: "ready", CreatedAt: now, UpdatedAt: now}
	return e.store.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "instance_id"}, {Name: "resource_kind"}},
		DoUpdates: clause.Assignments(map[string]any{"job_id": jobID, "resource_ref": resourceRef, "desired_revision": plan.ManifestSHA256, "state": "ready", "updated_at": now}),
	}).Create(&record).Error
}

func classifyTenantDeploymentError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "adapter_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "operation_cancelled"
	}
	var classified *TenantDeploymentAdapterError
	if errors.As(err, &classified) && safeRegistryScalar(classified.Class) {
		return classified.Class
	}
	return "adapter_failed"
}

type TenantDeploymentAdapterError struct {
	Class          string
	UnknownOutcome bool
	Err            error
}

func (e *TenantDeploymentAdapterError) Error() string { return e.Class }
func (e *TenantDeploymentAdapterError) Unwrap() error { return e.Err }

func tenantDeploymentErrorNeedsOperator(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var classified *TenantDeploymentAdapterError
	return errors.As(err, &classified) && classified.UnknownOutcome
}

func (e *TenantDeploymentEngine) ensure(ctx context.Context, action string, plan TenantDeploymentPlan) (string, error) {
	switch action {
	case "namespace":
		return e.adapters.Namespace.EnsureNamespace(ctx, plan)
	case "network_policy":
		return e.adapters.NetworkPolicy.EnsureNetworkPolicy(ctx, plan)
	case "runtime_secret_binding":
		return e.adapters.SecretBinding.EnsureSecretBinding(ctx, plan)
	case "license_binding":
		return e.adapters.LicenseBinding.EnsureLicenseBinding(ctx, plan)
	case "state_pvc":
		return e.adapters.State.EnsureStatePVC(ctx, plan)
	case "dedicated_rds":
		return e.adapters.Database.EnsureDedicatedRDS(ctx, plan)
	case "router":
		return e.adapters.Router.EnsureRouter(ctx, plan)
	case "activation":
		return e.adapters.Activation.ValidateActivation(ctx, plan)
	case "hostname":
		return e.adapters.Hostname.EnableHostname(ctx, plan)
	default:
		return "", errors.New("unsupported deployment action")
	}
}

func (e *TenantDeploymentEngine) deleteResource(ctx context.Context, kind string, plan TenantDeploymentPlan, ref string) error {
	switch kind {
	case "namespace":
		return e.adapters.Namespace.DeleteNamespace(ctx, plan, ref)
	case "network_policy":
		return e.adapters.NetworkPolicy.DeleteNetworkPolicy(ctx, plan, ref)
	case "runtime_secret_binding":
		return e.adapters.SecretBinding.DeleteSecretBinding(ctx, plan, ref)
	case "license_binding":
		return e.adapters.LicenseBinding.DeleteLicenseBinding(ctx, plan, ref)
	case "state_pvc":
		return e.adapters.State.DeleteStatePVC(ctx, plan, ref)
	case "dedicated_rds":
		return e.adapters.Database.DeleteDedicatedRDS(ctx, plan, ref)
	case "router":
		return e.adapters.Router.DeleteRouter(ctx, plan, ref)
	case "activation":
		return e.adapters.Activation.DeleteActivation(ctx, plan, ref)
	case "hostname":
		return e.adapters.Hostname.DisableHostname(ctx, plan, ref)
	default:
		return errors.New("unsupported deployment resource")
	}
}

func (e *TenantDeploymentEngine) ListResources(ctx context.Context, jobID string) ([]tenantDeploymentResourceRecord, error) {
	var job tenantDeploymentJobRecord
	if err := e.store.db.WithContext(ctx).Select("instance_id").Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return nil, err
	}
	var records []tenantDeploymentResourceRecord
	err := e.store.db.WithContext(ctx).Where("instance_id = ?", job.InstanceID).Find(&records).Error
	sort.Slice(records, func(i, j int) bool {
		return actionIndex(records[i].ResourceKind) < actionIndex(records[j].ResourceKind)
	})
	return records, err
}

func actionIndex(action string) int {
	for index, candidate := range tenantDeploymentActionOrder {
		if action == candidate {
			return index
		}
	}
	return len(tenantDeploymentActionOrder)
}
