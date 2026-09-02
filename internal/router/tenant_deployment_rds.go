// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const (
	TenantDeploymentRDSAdmissionAPIVersion          = "metrum.ai/smartrouter-rds-admission/v1"
	TenantDeploymentRDSAdmissionActionDisposableE2E = "disposable-e2e"
)

// TenantDeploymentRDSAdmission is an externally issued, bounded,
// non-production gate for typed RDS mutation. It carries no credentials, DSN,
// endpoint, or secret reference. metrum-fleetctl only validates and consumes a
// protected admission document; it never constructs one.
type TenantDeploymentRDSAdmission struct {
	APIVersion      string    `json:"api_version"`
	Action          string    `json:"action"`
	ApprovalID      string    `json:"approval_id"`
	IssuerRole      string    `json:"issuer_role"`
	Signature       string    `json:"signature"`
	ProfileID       string    `json:"profile_id"`
	Environment     string    `json:"environment"`
	DatabaseProfile string    `json:"database_profile"`
	JobID           string    `json:"job_id"`
	Namespace       string    `json:"namespace"`
	ManifestSHA256  string    `json:"manifest_sha256"`
	ApprovedAt      time.Time `json:"approved_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func rdsAdmissionSigningPayload(admission TenantDeploymentRDSAdmission) ([]byte, error) {
	return json.Marshal(struct {
		APIVersion      string    `json:"api_version"`
		Action          string    `json:"action"`
		ApprovalID      string    `json:"approval_id"`
		IssuerRole      string    `json:"issuer_role"`
		ProfileID       string    `json:"profile_id"`
		Environment     string    `json:"environment"`
		DatabaseProfile string    `json:"database_profile"`
		JobID           string    `json:"job_id"`
		Namespace       string    `json:"namespace"`
		ManifestSHA256  string    `json:"manifest_sha256"`
		ApprovedAt      time.Time `json:"approved_at"`
		ExpiresAt       time.Time `json:"expires_at"`
	}{
		APIVersion: admission.APIVersion, Action: admission.Action, ApprovalID: admission.ApprovalID,
		IssuerRole: admission.IssuerRole, ProfileID: admission.ProfileID, Environment: admission.Environment,
		DatabaseProfile: admission.DatabaseProfile, JobID: admission.JobID, Namespace: admission.Namespace,
		ManifestSHA256: admission.ManifestSHA256, ApprovedAt: admission.ApprovedAt, ExpiresAt: admission.ExpiresAt,
	})
}

type tenantDeploymentRDSClient interface {
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	CreateDBInstance(context.Context, *rds.CreateDBInstanceInput, ...func(*rds.Options)) (*rds.CreateDBInstanceOutput, error)
	DeleteDBInstance(context.Context, *rds.DeleteDBInstanceInput, ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error)
	ListTagsForResource(context.Context, *rds.ListTagsForResourceInput, ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error)
}

// TenantDeploymentRDSAdapter provisions one private, encrypted PostgreSQL RDS
// instance. It intentionally returns only opaque scalar identifiers and never
// reads, persists, or emits connection data.
type TenantDeploymentRDSAdapter struct {
	profile   TenantDeploymentProfile
	admission TenantDeploymentRDSAdmission
	client    tenantDeploymentRDSClient
}

func LoadTenantDeploymentRDSAdmission(path string, profile TenantDeploymentProfile, plan TenantDeploymentPlan, now time.Time) (TenantDeploymentRDSAdmission, string, error) {
	data, err := readDeploymentDocument(path, nil, true)
	if err != nil {
		return TenantDeploymentRDSAdmission{}, "", errors.New("RDS admission cannot be read")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var admission TenantDeploymentRDSAdmission
	if err := decoder.Decode(&admission); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return TenantDeploymentRDSAdmission{}, "", errors.New("RDS admission schema is invalid")
	}
	if err := rejectSecretShapedDeploymentData(data); err != nil {
		return TenantDeploymentRDSAdmission{}, "", err
	}
	if err := validateTenantDeploymentRDSAdmissionForPlan(profile, admission, plan, now); err != nil {
		return TenantDeploymentRDSAdmission{}, "", err
	}
	sum := sha256.Sum256(data)
	return admission, hex.EncodeToString(sum[:]), nil
}

func NewTenantDeploymentRDSAdapter(profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission, client tenantDeploymentRDSClient) (*TenantDeploymentRDSAdapter, error) {
	if client == nil {
		return nil, errors.New("typed RDS client is required")
	}
	if err := validateTenantDeploymentRDSAdmission(profile, admission, time.Now().UTC()); err != nil {
		return nil, err
	}
	return &TenantDeploymentRDSAdapter{profile: profile, admission: admission, client: client}, nil
}

func validateTenantDeploymentRDSAdmission(profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission, now time.Time) error {
	if profile.Environment != "nonproduction" || profile.DatabaseMode != "dedicated-rds" {
		return errors.New("non-production dedicated RDS profile is required")
	}
	if admission.APIVersion != TenantDeploymentRDSAdmissionAPIVersion ||
		admission.Action != TenantDeploymentRDSAdmissionActionDisposableE2E ||
		admission.ProfileID != profile.ProfileID ||
		admission.Environment != profile.Environment ||
		admission.DatabaseProfile != profile.ApprovedDatabaseProfile {
		return errors.New("approved non-production RDS admission is required")
	}
	for name, value := range map[string]string{
		"rds_admission_id":        admission.ApprovalID,
		"rds_admission_issuer":    admission.IssuerRole,
		"rds_admission_job":       admission.JobID,
		"rds_admission_namespace": admission.Namespace,
	} {
		if err := validateDeploymentID(name, value); err != nil {
			return errors.New("approved non-production RDS admission is required")
		}
	}
	if len(admission.ManifestSHA256) != sha256.Size*2 {
		return errors.New("approved non-production RDS admission is required")
	}
	if _, err := hex.DecodeString(admission.ManifestSHA256); err != nil {
		return errors.New("approved non-production RDS admission is required")
	}
	if admission.ApprovedAt.IsZero() ||
		admission.ExpiresAt.IsZero() ||
		admission.ApprovedAt.After(now) ||
		now.Sub(admission.ApprovedAt) > 24*time.Hour ||
		!admission.ExpiresAt.After(now) ||
		admission.ExpiresAt.Sub(admission.ApprovedAt) > 24*time.Hour {
		return errors.New("approved non-production RDS admission is expired or invalid")
	}
	payload, err := rdsAdmissionSigningPayload(admission)
	if err != nil || verifyLifecycleApprovalSignature(profile, admission.IssuerRole, admission.Signature, payload) != nil {
		return errors.New("approved non-production RDS admission is not authenticated")
	}
	return nil
}

func validateTenantDeploymentRDSAdmissionForPlan(profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission, plan TenantDeploymentPlan, now time.Time) error {
	if err := validateTenantDeploymentRDSAdmission(profile, admission, now); err != nil {
		return err
	}
	if plan.DatabaseID == "" ||
		plan.DatabaseProfile != profile.ApprovedDatabaseProfile ||
		admission.JobID != plan.JobID ||
		admission.Namespace != plan.Namespace ||
		admission.ManifestSHA256 != plan.ManifestSHA256 {
		return errors.New("RDS admission is not bound to this deployment plan")
	}
	return nil
}
func (a *TenantDeploymentRDSAdapter) validateAdmission(plan TenantDeploymentPlan) error {
	if err := validateTenantDeploymentRDSAdmissionForPlan(a.profile, a.admission, plan, time.Now().UTC()); err != nil {
		return &TenantDeploymentAdapterError{Class: "rds_admission_invalid", Err: errors.New("dedicated RDS admission is invalid")}
	}
	return nil
}

func (a *TenantDeploymentRDSAdapter) EnsureDedicatedRDS(ctx context.Context, plan TenantDeploymentPlan) (string, error) {
	if plan.DatabaseID == "" || plan.DatabaseProfile != a.profile.ApprovedDatabaseProfile {
		return "", &TenantDeploymentAdapterError{Class: "rds_profile_invalid", Err: errors.New("dedicated RDS database profile is required")}
	}
	if err := a.validateAdmission(plan); err != nil {
		return "", err
	}

	instance, found, err := a.observe(ctx, plan)
	if err != nil {
		return "", err
	}
	if found {
		if err := a.validateInstance(ctx, plan, instance); err != nil {
			return "", err
		}
		if err := a.waitUntilAvailable(ctx, plan); err != nil {
			return "", err
		}
		return rdsResourceReference(plan), nil
	}
	created, err := a.client.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
		AllocatedStorage:         aws.Int32(int32(a.profile.RDSStorageGiB)),
		BackupRetentionPeriod:    aws.Int32(int32(a.profile.RDSBackupRetentionDays)),
		CopyTagsToSnapshot:       aws.Bool(true),
		DBInstanceClass:          aws.String(normalizeRDSInstanceClass(a.profile.RDSInstanceClass)),
		DBInstanceIdentifier:     aws.String(plan.DatabaseID),
		DBSubnetGroupName:        aws.String(a.profile.RDSSubnetGroup),
		DeletionProtection:       aws.Bool(false),
		Engine:                   aws.String("postgres"),
		ManageMasterUserPassword: aws.Bool(true),
		MasterUsername:           aws.String(a.profile.RDSMasterUsername),
		PubliclyAccessible:       aws.Bool(false),
		StorageEncrypted:         aws.Bool(true),
		VpcSecurityGroupIds:      []string{a.profile.RDSVPCSecurityGroup},
		Tags: []rdstypes.Tag{
			{Key: aws.String(tenantDeploymentOwnerLabel), Value: aws.String(plan.InstanceID)},
			{Key: aws.String("app.kubernetes.io/managed-by"), Value: aws.String("metrum-fleetctl")},
		},
	})
	if err != nil || created == nil || created.DBInstance == nil {
		return "", &TenantDeploymentAdapterError{Class: "rds_create_unknown", UnknownOutcome: true, Err: errors.New("create dedicated RDS instance")}
	}
	if err := a.validateInstance(ctx, plan, created.DBInstance); err != nil {
		return "", err
	}
	if err := a.waitUntilAvailable(ctx, plan); err != nil {
		return "", err
	}
	return rdsResourceReference(plan), nil
}

func (a *TenantDeploymentRDSAdapter) waitUntilAvailable(ctx context.Context, plan TenantDeploymentPlan) error {
	deadline := time.Now().Add(30 * time.Minute)
	for {
		instance, found, err := a.observe(ctx, plan)
		if err != nil {
			return err
		}
		if found && instance != nil && aws.ToString(instance.DBInstanceStatus) == "available" &&
			instance.Endpoint != nil && aws.ToString(instance.Endpoint.Address) != "" {
			if err := a.validateInstance(ctx, plan, instance); err != nil {
				return err
			}
			return nil
		}
		if time.Now().After(deadline) {
			return &TenantDeploymentAdapterError{Class: "rds_not_available", Err: errors.New("dedicated RDS instance did not become available")}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

// UsageDSN returns a postgres DSN for the owned available instance. The value
// stays in memory for runtime-secret binding and is never logged or persisted
// into Fleet status/registry rows.
func (a *TenantDeploymentRDSAdapter) UsageDSN(ctx context.Context, plan TenantDeploymentPlan, secrets *secretsmanager.Client) (string, error) {
	if secrets == nil {
		return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("secrets client is required")}
	}
	if err := a.waitUntilAvailable(ctx, plan); err != nil {
		return "", err
	}
	instance, found, err := a.observe(ctx, plan)
	if err != nil {
		return "", err
	}
	if !found || instance == nil || instance.Endpoint == nil || instance.MasterUserSecret == nil ||
		aws.ToString(instance.MasterUserSecret.SecretArn) == "" {
		return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("dedicated RDS endpoint or master secret is unavailable")}
	}
	secretOut, err := secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: instance.MasterUserSecret.SecretArn})
	if err != nil || secretOut == nil || secretOut.SecretString == nil {
		return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("read dedicated RDS master secret")}
	}
	var cred struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(*secretOut.SecretString), &cred); err != nil || cred.Username == "" || cred.Password == "" {
		return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("parse dedicated RDS master secret")}
	}
	host := aws.ToString(instance.Endpoint.Address)
	port := aws.ToInt32(instance.Endpoint.Port)
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=require TimeZone=UTC",
		host, port, cred.Username, cred.Password), nil
}

func (a *TenantDeploymentRDSAdapter) DeleteDedicatedRDS(ctx context.Context, plan TenantDeploymentPlan, _ string) error {
	if plan.DatabaseID == "" || plan.DatabaseProfile != a.profile.ApprovedDatabaseProfile {
		return &TenantDeploymentAdapterError{Class: "rds_profile_invalid", Err: errors.New("dedicated RDS database profile is required")}
	}
	if err := a.validateAdmission(plan); err != nil {
		return err
	}
	_, found, err := a.observe(ctx, plan)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	_, err = a.client.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier:      aws.String(plan.DatabaseID),
		DeleteAutomatedBackups:    aws.Bool(false),
		FinalDBSnapshotIdentifier: aws.String("final-" + plan.DatabaseID),
		SkipFinalSnapshot:         aws.Bool(false),
	})
	if err != nil {
		return &TenantDeploymentAdapterError{Class: "rds_delete_unknown", UnknownOutcome: true, Err: errors.New("delete dedicated RDS instance")}
	}
	return nil
}

func (a *TenantDeploymentRDSAdapter) observe(ctx context.Context, plan TenantDeploymentPlan) (*rdstypes.DBInstance, bool, error) {
	output, err := a.client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(plan.DatabaseID)})
	if err != nil {
		var notFound *rdstypes.DBInstanceNotFoundFault
		if errors.As(err, &notFound) {
			return nil, false, nil
		}
		return nil, false, &TenantDeploymentAdapterError{Class: "rds_observation_unknown", UnknownOutcome: true, Err: errors.New("observe dedicated RDS instance")}
	}
	if output == nil || len(output.DBInstances) != 1 || output.DBInstances[0].DBInstanceIdentifier == nil || *output.DBInstances[0].DBInstanceIdentifier != plan.DatabaseID {
		return nil, false, nil
	}
	return &output.DBInstances[0], true, nil
}

func (a *TenantDeploymentRDSAdapter) validateInstance(ctx context.Context, plan TenantDeploymentPlan, instance *rdstypes.DBInstance) error {
	if instance == nil || instance.DBInstanceArn == nil || instance.DBSubnetGroup == nil || instance.DBSubnetGroup.DBSubnetGroupName == nil ||
		aws.ToString(instance.DBSubnetGroup.DBSubnetGroupName) != a.profile.RDSSubnetGroup ||
		aws.ToBool(instance.PubliclyAccessible) || !aws.ToBool(instance.StorageEncrypted) || aws.ToBool(instance.DeletionProtection) ||
		aws.ToString(instance.DBInstanceClass) != normalizeRDSInstanceClass(a.profile.RDSInstanceClass) ||
		aws.ToString(instance.MasterUsername) != a.profile.RDSMasterUsername ||
		aws.ToInt32(instance.AllocatedStorage) != int32(a.profile.RDSStorageGiB) ||
		aws.ToInt32(instance.BackupRetentionPeriod) != int32(a.profile.RDSBackupRetentionDays) ||
		!containsString(instance.VpcSecurityGroups, a.profile.RDSVPCSecurityGroup) {
		return &TenantDeploymentAdapterError{Class: "rds_policy_violation", Err: errors.New("dedicated RDS policy does not match approved profile")}
	}
	tags, err := a.client.ListTagsForResource(ctx, &rds.ListTagsForResourceInput{ResourceName: instance.DBInstanceArn})
	if err != nil || !hasRDSTag(tags, tenantDeploymentOwnerLabel, plan.InstanceID) || !hasRDSTag(tags, "app.kubernetes.io/managed-by", "metrum-fleetctl") {
		return &TenantDeploymentAdapterError{Class: "rds_ownership_unverified", UnknownOutcome: err != nil, Err: errors.New("dedicated RDS ownership tags are not verified")}
	}
	return nil
}

func rdsResourceReference(plan TenantDeploymentPlan) string { return "rds." + plan.DatabaseID }

func containsString(groups []rdstypes.VpcSecurityGroupMembership, wanted string) bool {
	for _, group := range groups {
		if aws.ToString(group.VpcSecurityGroupId) == wanted {
			return true
		}
	}
	return false
}

func hasRDSTag(output *rds.ListTagsForResourceOutput, key, value string) bool {
	if output == nil {
		return false
	}
	for _, tag := range output.TagList {
		if aws.ToString(tag.Key) == key && aws.ToString(tag.Value) == value {
			return true
		}
	}
	return false
}

// normalizeRDSInstanceClass accepts profile values written with hyphens
// (db-t4g-medium) and returns the AWS API form (db.t4g.medium).
func normalizeRDSInstanceClass(class string) string {
	class = strings.TrimSpace(class)
	if class == "" || strings.Contains(class, ".") {
		return class
	}
	if strings.HasPrefix(class, "db-") {
		return "db." + strings.ReplaceAll(strings.TrimPrefix(class, "db-"), "-", ".")
	}
	return class
}

var _ TenantDatabaseAdapter = (*TenantDeploymentRDSAdapter)(nil)
