package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// FakeTenantDeploymentAdapters is deterministic and process-local. It mutates
// only the normalized local registry through the engine; it never invokes a
// shell, AWS, Kubernetes, DNS, a provider, or a production endpoint.
type FakeTenantDeploymentAdapters struct {
	mu                 sync.Mutex
	FailAction         string
	FailClass          string
	FailUnknownOutcome bool
	Calls              []string
}

func NewFakeTenantDeploymentAdapters() (*FakeTenantDeploymentAdapters, TenantDeploymentAdapters) {
	fake := &FakeTenantDeploymentAdapters{}
	return fake, TenantDeploymentAdapters{
		Namespace: fake, NetworkPolicy: fake, SecretBinding: fake,
		LicenseBinding: fake, State: fake, Database: fake, Router: fake, Activation: fake, Hostname: fake,
	}
}

func (f *FakeTenantDeploymentAdapters) ensure(action string, plan TenantDeploymentPlan) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "ensure:"+action)
	if f.FailAction == action {
		class := f.FailClass
		if class == "" {
			class = "injected_failure"
		}
		return "", &TenantDeploymentAdapterError{Class: class, UnknownOutcome: f.FailUnknownOutcome, Err: errors.New("injected fake adapter failure")}
	}
	safeKind := strings.ReplaceAll(action, "runtime_secret_binding", "runtime-binding")
	return plan.InstanceID + "-" + safeKind, nil
}

func (f *FakeTenantDeploymentAdapters) remove(action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "delete:"+action)
	if f.FailAction == "delete:"+action {
		class := f.FailClass
		if class == "" {
			class = "injected_delete_failure"
		}
		return &TenantDeploymentAdapterError{Class: class, Err: errors.New("injected fake adapter failure")}
	}
	return nil
}

func (f *FakeTenantDeploymentAdapters) EnsureNamespace(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("namespace", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteNamespace(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("namespace")
}
func (f *FakeTenantDeploymentAdapters) EnsureNetworkPolicy(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("network_policy", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteNetworkPolicy(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("network_policy")
}
func (f *FakeTenantDeploymentAdapters) EnsureSecretBinding(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("runtime_secret_binding", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteSecretBinding(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("runtime_secret_binding")
}
func (f *FakeTenantDeploymentAdapters) EnsureLicenseBinding(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("license_binding", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteLicenseBinding(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("license_binding")
}
func (f *FakeTenantDeploymentAdapters) EnsureStatePVC(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("state_pvc", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteStatePVC(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("state_pvc")
}

func (f *FakeTenantDeploymentAdapters) EnsureDedicatedRDS(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("dedicated_rds", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteDedicatedRDS(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("dedicated_rds")
}
func (f *FakeTenantDeploymentAdapters) EnsureRouter(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("router", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteRouter(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("router")
}
func (f *FakeTenantDeploymentAdapters) ValidateActivation(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("activation", plan)
}
func (f *FakeTenantDeploymentAdapters) DeleteActivation(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("activation")
}
func (f *FakeTenantDeploymentAdapters) EnableHostname(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	return f.ensure("hostname", plan)
}
func (f *FakeTenantDeploymentAdapters) DisableHostname(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return f.remove("hostname")
}

func (f *FakeTenantDeploymentAdapters) SnapshotCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Calls...)
}

type TenantDeletionApproval struct {
	APIVersion     string    `json:"api_version"`
	JobID          string    `json:"job_id"`
	Action         string    `json:"action"`
	ExpiresAt      time.Time `json:"expires_at"`
	RetainPVC      bool      `json:"retain_pvc"`
	RetainDatabase bool      `json:"retain_database"`
	Nonce          string    `json:"nonce"`
}

func LoadTenantDeletionApproval(path string, now time.Time) (TenantDeletionApproval, string, error) {
	data, err := readDeploymentDocument(path, nil, true)
	if err != nil {
		return TenantDeletionApproval{}, "", fmt.Errorf("read deletion approval: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var approval TenantDeletionApproval
	if err := decoder.Decode(&approval); err != nil {
		return TenantDeletionApproval{}, "", errors.New("deletion approval schema is invalid")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return TenantDeletionApproval{}, "", errors.New("deletion approval contains trailing data")
	}
	if err := validateTenantDeletionApproval(approval, now); err != nil {
		return TenantDeletionApproval{}, "", err
	}
	if err := rejectSecretShapedDeploymentData(data); err != nil {
		return TenantDeletionApproval{}, "", err
	}
	sum := sha256.Sum256(data)
	return approval, hex.EncodeToString(sum[:]), nil
}
func validateTenantDeletionApproval(approval TenantDeletionApproval, now time.Time) error {
	if approval.APIVersion != TenantDeletionApprovalAPIVersion || approval.Action != "delete" {
		return errors.New("deletion approval schema or action is invalid")
	}
	if err := validateDeploymentID("job_id", approval.JobID); err != nil {
		return err
	}
	if err := validateDeploymentID("nonce", approval.Nonce); err != nil {
		return err
	}
	if !approval.ExpiresAt.After(now) || approval.ExpiresAt.After(now.Add(24*time.Hour)) {
		return errors.New("deletion approval must expire within the next 24 hours")
	}
	return nil
}

func (e *TenantDeploymentEngine) Delete(ctx context.Context, plan TenantDeploymentPlan, approval TenantDeletionApproval, approvalSHA256 string) (TenantDeploymentStatus, error) {
	tenantDeploymentProcessLock.Lock()
	defer tenantDeploymentProcessLock.Unlock()
	now := time.Now().UTC()
	if err := validateTenantDeletionApproval(approval, now); err != nil {
		return TenantDeploymentStatus{}, err
	}
	if len(approvalSHA256) != sha256.Size*2 {
		return TenantDeploymentStatus{}, errors.New("deletion approval digest is invalid")
	}
	if _, err := hex.DecodeString(approvalSHA256); err != nil {
		return TenantDeploymentStatus{}, errors.New("deletion approval digest is invalid")
	}
	if approval.JobID != plan.JobID {
		return TenantDeploymentStatus{}, errors.New("deletion approval is bound to a different job")
	}
	var job tenantDeploymentJobRecord
	if err := e.store.db.WithContext(ctx).Where("job_id = ? AND profile_id = ?", plan.JobID, plan.ProfileID).First(&job).Error; err != nil {
		return TenantDeploymentStatus{}, err
	}
	if job.ManifestSHA256 != plan.ManifestSHA256 {
		return TenantDeploymentStatus{}, errors.New("deletion desired state does not match the deployed manifest")
	}
	if job.State == TenantDeploymentDeleted {
		return statusFromDeploymentRecord(job), nil
	}
	var newerJobs int64
	if err := e.store.db.WithContext(ctx).Model(&tenantDeploymentJobRecord{}).
		Where("instance_id = ? AND created_at > ? AND state <> ?", job.InstanceID, job.CreatedAt, TenantDeploymentDeleted).
		Count(&newerJobs).Error; err != nil {
		return TenantDeploymentStatus{}, err
	}
	if newerJobs != 0 {
		return TenantDeploymentStatus{}, errors.New("deletion job is superseded by a newer instance lifecycle")
	}
	approvalRecord := tenantDeploymentApprovalRecord{
		ApprovalSHA256: approvalSHA256, JobID: approval.JobID, Action: approval.Action,
		ExpiresAt: approval.ExpiresAt,
		RetainPVC: approval.RetainPVC, CreatedAt: now,
	}
	var priorApproval tenantDeploymentApprovalRecord
	if err := e.store.db.WithContext(ctx).Where("approval_sha256 = ?", approvalSHA256).First(&priorApproval).Error; err == nil {
		if priorApproval.ConsumedAt != nil {
			return TenantDeploymentStatus{}, errors.New("deletion approval has already been used")
		}
		if priorApproval.JobID != approvalRecord.JobID || priorApproval.Action != approvalRecord.Action ||
			!priorApproval.ExpiresAt.Equal(approvalRecord.ExpiresAt) ||
			priorApproval.RetainPVC != approvalRecord.RetainPVC {
			return TenantDeploymentStatus{}, errors.New("stored deletion approval does not match")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return TenantDeploymentStatus{}, err
	} else if err := e.store.db.WithContext(ctx).Create(&approvalRecord).Error; err != nil {
		return TenantDeploymentStatus{}, err
	}
	resources, err := e.ListResources(ctx, plan.JobID)
	if err != nil {
		return TenantDeploymentStatus{}, err
	}
	sort.Slice(resources, func(i, j int) bool {
		return actionIndex(resources[i].ResourceKind) > actionIndex(resources[j].ResourceKind)
	})
	for _, resource := range resources {
		if resource.State == "deleted" || resource.State == "retained" {
			continue
		}
		retain := (resource.ResourceKind == "state_pvc" && approval.RetainPVC) || (resource.ResourceKind == "dedicated_rds" && approval.RetainDatabase)
		if retain {
			if err := e.setResourceState(ctx, resource.ID, "retained"); err != nil {
				return TenantDeploymentStatus{}, err
			}
			continue
		}
		if err := e.deleteResource(ctx, resource.ResourceKind, plan, resource.ResourceRef); err != nil {
			errorClass := classifyTenantDeploymentError(err)
			_ = e.updateJob(ctx, plan.JobID, TenantDeploymentOperatorRequired, job.CompletedAction, "operator_review", errorClass, false, false)
			status, _ := e.store.Status(ctx, plan.JobID, plan.ProfileID)
			return status, fmt.Errorf("delete action %s failed: %s", resource.ResourceKind, errorClass)
		}
		if err := e.setResourceState(ctx, resource.ID, "deleted"); err != nil {
			return TenantDeploymentStatus{}, err
		}
	}
	completedAt := time.Now().UTC()
	if err := e.store.db.WithContext(ctx).Model(&tenantDeploymentApprovalRecord{}).Where("approval_sha256 = ? AND consumed_at IS NULL", approvalSHA256).Update("consumed_at", completedAt).Error; err != nil {
		return TenantDeploymentStatus{}, err
	}
	if err := e.updateJob(ctx, plan.JobID, TenantDeploymentDeleted, "delete", "", "", false, false); err != nil {
		return TenantDeploymentStatus{}, err
	}
	return e.store.Status(ctx, plan.JobID, plan.ProfileID)
}

func (e *TenantDeploymentEngine) setResourceState(ctx context.Context, id uint, state string) error {
	return e.store.db.WithContext(ctx).Model(&tenantDeploymentResourceRecord{}).Where("id = ?", id).Updates(map[string]any{"state": state, "updated_at": time.Now().UTC()}).Error
}

// Ensure private-mode files remain private after SQLite creates sidecars.
func (s *TenantDeploymentStore) EnforcePrivateMode(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return chmodSQLiteFiles(path)
}
