package router

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

type fakeTenantDeploymentRDSClient struct {
	instance    *rdstypes.DBInstance
	tags        []rdstypes.Tag
	createInput *rds.CreateDBInstanceInput
	deleteInput *rds.DeleteDBInstanceInput
	createErr   error
	calls       int
}

func (f *fakeTenantDeploymentRDSClient) DescribeDBInstances(_ context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	f.calls++
	if f.instance == nil {
		return &rds.DescribeDBInstancesOutput{}, nil
	}
	return &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{*f.instance}}, nil
}
func (f *fakeTenantDeploymentRDSClient) CreateDBInstance(_ context.Context, input *rds.CreateDBInstanceInput, _ ...func(*rds.Options)) (*rds.CreateDBInstanceOutput, error) {
	f.calls++
	f.createInput = input
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.instance = dedicatedRDSInstance(input)
	f.tags = append([]rdstypes.Tag(nil), input.Tags...)
	return &rds.CreateDBInstanceOutput{DBInstance: f.instance}, nil
}
func (f *fakeTenantDeploymentRDSClient) DeleteDBInstance(_ context.Context, input *rds.DeleteDBInstanceInput, _ ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error) {
	f.calls++
	f.deleteInput = input
	return &rds.DeleteDBInstanceOutput{}, nil
}
func (f *fakeTenantDeploymentRDSClient) ListTagsForResource(_ context.Context, _ *rds.ListTagsForResourceInput, _ ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error) {
	f.calls++
	return &rds.ListTagsForResourceOutput{TagList: append([]rdstypes.Tag(nil), f.tags...)}, nil
}

func dedicatedRDSInstance(input *rds.CreateDBInstanceInput) *rdstypes.DBInstance {
	return &rdstypes.DBInstance{
		AllocatedStorage:      input.AllocatedStorage,
		BackupRetentionPeriod: input.BackupRetentionPeriod,
		DBInstanceArn:         aws.String("arn:aws:rds:us-west-2:123456789012:db:" + aws.ToString(input.DBInstanceIdentifier)),
		DBInstanceClass:       input.DBInstanceClass,
		DBInstanceIdentifier:  input.DBInstanceIdentifier,
		DBSubnetGroup:         &rdstypes.DBSubnetGroup{DBSubnetGroupName: input.DBSubnetGroupName},
		DeletionProtection:    input.DeletionProtection,
		Endpoint:              &rdstypes.Endpoint{Address: aws.String("router-db.internal")},
		MasterUsername:        input.MasterUsername,
		PubliclyAccessible:    input.PubliclyAccessible,
		DBInstanceStatus:      aws.String("available"),
		StorageEncrypted:      input.StorageEncrypted,
		VpcSecurityGroups:     []rdstypes.VpcSecurityGroupMembership{{VpcSecurityGroupId: aws.String(input.VpcSecurityGroupIds[0])}},
	}
}

func dedicatedRDSFixture(t *testing.T) (TenantDeploymentProfile, TenantDeploymentPlan, TenantDeploymentRDSAdmission) {
	t.Helper()
	profile, manifest, _ := tenantDeploymentFixture(t)
	profile.DatabaseMode = "dedicated-rds"
	profile.ApprovedDatabaseProfile = "postgres-dedicated-small"
	profile.RDSInstanceClass = "db-t4g-medium"
	profile.RDSStorageGiB = 20
	profile.RDSBackupRetentionDays = 7
	profile.RDSSubnetGroup = "router-private"
	profile.RDSVPCSecurityGroup = "sg-router-private"
	profile.RDSMasterUsername = "routeradmin"
	profile.RDSProxyDisabled = true
	manifest.DatabaseProfile = profile.ApprovedDatabaseProfile
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-rds-adapter")
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTenantDeploymentProfile(profile, raw); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	admission := TenantDeploymentRDSAdmission{
		APIVersion: TenantDeploymentRDSAdmissionAPIVersion, Action: TenantDeploymentRDSAdmissionActionDisposableE2E,
		ApprovalID: "approved-rds-admission",
		ProfileID:  profile.ProfileID, Environment: profile.Environment, DatabaseProfile: profile.ApprovedDatabaseProfile,
		JobID: plan.JobID, Namespace: plan.Namespace, ManifestSHA256: plan.ManifestSHA256,
		ApprovedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	signRDSAdmission(t, profile, &admission)
	return profile, plan, admission
}

func TestLoadTenantDeploymentRDSAdmissionRequiresPrivateExactScope(t *testing.T) {
	profile, plan, admission := dedicatedRDSFixture(t)
	data, err := json.Marshal(admission)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/rds-admission.json"
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, digest, err := LoadTenantDeploymentRDSAdmission(path, profile, plan, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if loaded != admission || len(digest) != 64 {
		t.Fatalf("loaded admission=%+v digest=%q", loaded, digest)
	}
	forged := admission
	forged.ApprovalID = "forged-rds-admission"
	forgedData, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	forgedPath := t.TempDir() + "/forged-rds-admission.json"
	if err := os.WriteFile(forgedPath, forgedData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTenantDeploymentRDSAdmission(forgedPath, profile, plan, time.Now().UTC()); err == nil {
		t.Fatal("forged RDS admission was accepted")
	}
	otherPlan := plan
	otherPlan.Namespace = "other-namespace"
	if _, _, err := LoadTenantDeploymentRDSAdmission(path, profile, otherPlan, time.Now().UTC()); err == nil {
		t.Fatal("admission was accepted for another namespace")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTenantDeploymentRDSAdmission(path, profile, plan, time.Now().UTC()); err == nil {
		t.Fatal("world-readable admission was accepted")
	}
}

func TestTenantDeploymentRDSAdapterRejectsAdmissionForAnotherPlanBeforeClientCalls(t *testing.T) {
	profile, plan, admission := dedicatedRDSFixture(t)
	client := &fakeTenantDeploymentRDSClient{}
	adapter, err := NewTenantDeploymentRDSAdapter(profile, admission, client)
	if err != nil {
		t.Fatal(err)
	}
	otherPlan := plan
	otherPlan.ManifestSHA256 = strings.Repeat("b", 64)
	if _, err := adapter.EnsureDedicatedRDS(context.Background(), otherPlan); err == nil {
		t.Fatal("admission was accepted for another plan")
	}
	if client.calls != 0 {
		t.Fatalf("out-of-scope admission invoked RDS client: %d calls", client.calls)
	}
}

func TestTenantDeploymentRDSAdapterRevalidatesAdmissionBeforeClientCalls(t *testing.T) {
	profile, plan, admission := dedicatedRDSFixture(t)
	client := &fakeTenantDeploymentRDSClient{}
	adapter, err := NewTenantDeploymentRDSAdapter(profile, admission, client)
	if err != nil {
		t.Fatal(err)
	}
	adapter.admission.ExpiresAt = time.Now().UTC().Add(-time.Second)
	if _, err := adapter.EnsureDedicatedRDS(context.Background(), plan); err == nil {
		t.Fatal("expired admission reached RDS adapter")
	}
	if client.calls != 0 {
		t.Fatalf("expired admission invoked RDS client: %d calls", client.calls)
	}
}

func TestTenantDeploymentRDSAdapterRejectsAdmissionBeforeClientCalls(t *testing.T) {
	profile, _, admission := dedicatedRDSFixture(t)
	client := &fakeTenantDeploymentRDSClient{}
	admission.ExpiresAt = admission.ApprovedAt
	if _, err := NewTenantDeploymentRDSAdapter(profile, admission, client); err == nil {
		t.Fatal("expired admission accepted")
	}
	if client.calls != 0 {
		t.Fatalf("admission rejection invoked RDS client: %d calls", client.calls)
	}
}

func TestTenantDeploymentRDSAdapterProvisionsPrivateEncryptedOwnedInstanceAndDeletesSafely(t *testing.T) {
	profile, plan, admission := dedicatedRDSFixture(t)
	client := &fakeTenantDeploymentRDSClient{}
	adapter, err := NewTenantDeploymentRDSAdapter(profile, admission, client)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.EnsureDedicatedRDS(context.Background(), plan)
	if err != nil || ref != rdsResourceReference(plan) {
		t.Fatalf("ensure ref=%q err=%v", ref, err)
	}
	created := client.createInput
	if created == nil || aws.ToBool(created.PubliclyAccessible) || !aws.ToBool(created.StorageEncrypted) || aws.ToBool(created.DeletionProtection) || !aws.ToBool(created.ManageMasterUserPassword) || aws.ToString(created.DBSubnetGroupName) != profile.RDSSubnetGroup || len(created.VpcSecurityGroupIds) != 1 || created.VpcSecurityGroupIds[0] != profile.RDSVPCSecurityGroup {
		t.Fatalf("unsafe RDS creation input: %#v", created)
	}
	if encoded := strings.ToLower(ref); strings.Contains(encoded, "dsn") || strings.Contains(encoded, "endpoint") || strings.Contains(encoded, "password") {
		t.Fatalf("unsafe RDS resource reference: %q", ref)
	}
	if err := adapter.DeleteDedicatedRDS(context.Background(), plan, ref); err != nil {
		t.Fatal(err)
	}
	if deleted := client.deleteInput; deleted == nil || aws.ToBool(deleted.SkipFinalSnapshot) || aws.ToBool(deleted.DeleteAutomatedBackups) || aws.ToString(deleted.FinalDBSnapshotIdentifier) != "final-"+plan.DatabaseID {
		t.Fatalf("unsafe RDS delete input: %#v", deleted)
	}
}

func TestTenantDeploymentRDSUnknownCreateOutcomeNeedsOperator(t *testing.T) {
	profile, plan, admission := dedicatedRDSFixture(t)
	client := &fakeTenantDeploymentRDSClient{createErr: errors.New("synthetic RDS transport failure")}
	database, err := NewTenantDeploymentRDSAdapter(profile, admission, client)
	if err != nil {
		t.Fatal(err)
	}
	store, _, _, _ := openTenantDeploymentTestEngine(t)
	activeFake, adapters := NewFakeTenantDeploymentAdapters()
	adapters.Database = database
	engine, err := NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		t.Fatal(err)
	}
	status, err := engine.Deploy(context.Background(), plan, "intent-rds-adapter")
	if err == nil || status.State != TenantDeploymentOperatorRequired || status.NextAction != "operator_review" || status.ErrorClass != "rds_create_unknown" {
		t.Fatalf("unknown RDS outcome status=%+v err=%v", status, err)
	}
	if calls := activeFake.SnapshotCalls(); strings.Join(calls, ",") != "ensure:namespace,ensure:network_policy" {
		t.Fatalf("unexpected fake calls before RDS failure: %v", calls)
	}
}
