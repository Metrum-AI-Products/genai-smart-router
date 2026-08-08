package router

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

const TenantDeploymentRDSAdmissionAPIVersion = "metrum.ai/smartrouter-rds-admission/v1"

// TenantDeploymentRDSAdmission is the explicit, bounded non-production gate for
// typed RDS mutation. It carries no credentials, DSN, endpoint, or secret
// reference. The current Fleet CLI does not construct this contract.
type TenantDeploymentRDSAdmission struct {
	APIVersion      string
	ApprovalID      string
	ProfileID       string
	Environment     string
	DatabaseProfile string
	ApprovedAt      time.Time
	ExpiresAt       time.Time
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
	profile TenantDeploymentProfile
	client  tenantDeploymentRDSClient
}

func NewTenantDeploymentRDSAdapter(profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission, client tenantDeploymentRDSClient) (*TenantDeploymentRDSAdapter, error) {
	if client == nil {
		return nil, errors.New("typed RDS client is required")
	}
	if err := validateTenantDeploymentRDSAdmission(profile, admission, time.Now().UTC()); err != nil {
		return nil, err
	}
	return &TenantDeploymentRDSAdapter{profile: profile, client: client}, nil
}

func validateTenantDeploymentRDSAdmission(profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission, now time.Time) error {
	if profile.Environment != "nonproduction" || profile.DatabaseMode != "dedicated-rds" {
		return errors.New("non-production dedicated RDS profile is required")
	}
	if admission.APIVersion != TenantDeploymentRDSAdmissionAPIVersion || admission.ProfileID != profile.ProfileID || admission.Environment != profile.Environment || admission.DatabaseProfile != profile.ApprovedDatabaseProfile {
		return errors.New("approved non-production RDS admission is required")
	}
	if err := validateDeploymentID("rds_admission_id", admission.ApprovalID); err != nil {
		return errors.New("approved non-production RDS admission is required")
	}
	if admission.ApprovedAt.IsZero() || admission.ExpiresAt.IsZero() || !admission.ExpiresAt.After(now) || admission.ExpiresAt.Sub(admission.ApprovedAt) > 24*time.Hour {
		return errors.New("approved non-production RDS admission is expired or invalid")
	}
	return nil
}

func (a *TenantDeploymentRDSAdapter) EnsureDedicatedRDS(ctx context.Context, plan TenantDeploymentPlan) (string, error) {
	instance, found, err := a.observe(ctx, plan)
	if err != nil {
		return "", err
	}
	if found {
		if err := a.validateInstance(ctx, plan, instance); err != nil {
			return "", err
		}
		return rdsResourceReference(plan), nil
	}
	created, err := a.client.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
		AllocatedStorage:         aws.Int32(int32(a.profile.RDSStorageGiB)),
		BackupRetentionPeriod:    aws.Int32(int32(a.profile.RDSBackupRetentionDays)),
		CopyTagsToSnapshot:       aws.Bool(true),
		DBInstanceClass:          aws.String(a.profile.RDSInstanceClass),
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
	return rdsResourceReference(plan), nil
}

func (a *TenantDeploymentRDSAdapter) DeleteDedicatedRDS(ctx context.Context, plan TenantDeploymentPlan, _ string) error {
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
		aws.ToString(instance.DBInstanceClass) != a.profile.RDSInstanceClass ||
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

var _ TenantDatabaseAdapter = (*TenantDeploymentRDSAdapter)(nil)
