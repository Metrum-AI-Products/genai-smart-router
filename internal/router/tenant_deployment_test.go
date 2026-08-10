package router

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func tenantDeploymentFixture(t *testing.T) (TenantDeploymentProfile, TenantDeploymentManifest, TenantDeploymentPlan) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "tenant-deployment")
	profilePath := filepath.Join(t.TempDir(), "profile.yaml")
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := LoadTenantDeploymentProfile("file://" + profilePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadTenantDeploymentManifest(filepath.Join(root, "manifest.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-a")
	if err != nil {
		t.Fatal(err)
	}
	return profile, manifest, plan
}

func testLifecycleApprovalPrivateKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	seed, err := base64.StdEncoding.DecodeString("nWGxne/9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A=")
	if err != nil {
		t.Fatal(err)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func signRDSAdmission(t *testing.T, profile TenantDeploymentProfile, admission *TenantDeploymentRDSAdmission) {
	t.Helper()
	admission.IssuerRole = profile.LifecycleApprovalIssuer
	payload, err := rdsAdmissionSigningPayload(*admission)
	if err != nil {
		t.Fatal(err)
	}
	admission.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testLifecycleApprovalPrivateKey(t), payload))
}

func signDeletionApproval(t *testing.T, profile TenantDeploymentProfile, approval *TenantDeletionApproval) {
	t.Helper()
	approval.IssuerRole = profile.LifecycleApprovalIssuer
	payload, err := deletionApprovalSigningPayload(*approval)
	if err != nil {
		t.Fatal(err)
	}
	approval.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testLifecycleApprovalPrivateKey(t), payload))
}

func openTenantDeploymentTestEngine(t *testing.T) (*TenantDeploymentStore, *FakeTenantDeploymentAdapters, *TenantDeploymentEngine, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deployments.sqlite")
	store, err := OpenTenantDeploymentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	fake, adapters := NewFakeTenantDeploymentAdapters()
	engine, err := NewTenantDeploymentEngine(store, adapters)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, fake, engine, path
}

func TestTenantDeploymentContractDeterministicReferenceOnlyPlan(t *testing.T) {
	profile, manifest, first := tenantDeploymentFixture(t)
	second, err := BuildTenantDeploymentPlan(profile, manifest, "intent-a")
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("plan is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if first.Mode != "eks" || first.Environment != "nonproduction" || first.ReleaseDigest != profile.ApprovedReleaseDigest {
		t.Fatalf("unsafe plan: %+v", first)
	}
	encoded := string(firstJSON)
	for _, forbidden := range []string{manifest.RuntimeBundleRef, manifest.License.RequestRef, "provider_key", "router_token", "license_request"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("safe plan leaked protected input %q: %s", forbidden, encoded)
		}
	}
	wantActions := "namespace,network_policy,runtime_secret_binding,license_binding,state_pvc,router,activation,hostname"
	if strings.Join(first.Actions, ",") != wantActions {
		t.Fatalf("action order = %v", first.Actions)
	}
	if first.DatabaseID != "" || first.DatabaseProfile != "" || strings.Contains(strings.Join(first.Actions, ","), "dedicated_rds") {
		t.Fatalf("SQLite default unexpectedly selected RDS: %+v", first)
	}
}

func TestTenantDeploymentContractRejectsUnknownAndSecretShapedInput(t *testing.T) {
	_, _, _ = tenantDeploymentFixture(t)
	unknown := `{"api_version":"metrum.ai/smartrouter-deployment/v1","customer_id":"customer-a","stage":"test","release":"latest-approved","resource_profile":"small","state_profile":"sqlite-rwo-small","runtime_bundle_ref":"aws-secretsmanager:///safe/runtime-bundle","config_revision":"r1","license":{"request_ref":"aws-ssm:///safe/license","validity":"24h"},"password":"do-not-print-this"}`
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTenantDeploymentManifest(path, nil)
	if err == nil {
		t.Fatal("unknown secret field was accepted")
	}
	if strings.Contains(err.Error(), "do-not-print-this") {
		t.Fatalf("error leaked secret-shaped value: %v", err)
	}
	secretValue := strings.Replace(unknown, `,"password":"do-not-print-this"`, "", 1)
	secretValue = strings.Replace(secretValue, "aws-secretsmanager:///safe/runtime-bundle", "sk-abcdefghijklmnop", 1)
	if err := os.WriteFile(path, []byte(secretValue), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadTenantDeploymentManifest(path, nil)
	if err == nil || strings.Contains(err.Error(), "sk-abcdefghijklmnop") {
		t.Fatalf("credential-shaped value rejection was unsafe: %v", err)
	}
}

func TestTenantDeploymentContractRejectsUnprotectedOrProductionProfile(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "tenant-deployment", "profile.yaml")
	data, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTenantDeploymentProfile("file://" + path); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("unprotected local profile accepted: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTenantDeploymentProfile("file://" + path); err != nil {
		t.Fatalf("protected local test fixture profile failed: %v", err)
	}
	production := strings.Replace(string(data), "environment: nonproduction", "environment: production", 1)
	if err := os.WriteFile(path, []byte(production), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTenantDeploymentProfile("file://" + path); err == nil || !strings.Contains(err.Error(), "production profiles") {
		t.Fatalf("production profile accepted: %v", err)
	}
	if _, err := LoadTenantDeploymentProfile("https://example.test/profile"); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("unsupported profile adapter did not fail closed: %v", err)
	}
}

func TestTenantDeploymentAdaptersIdempotentResumeAndConcurrency(t *testing.T) {
	profile, _, plan := tenantDeploymentFixture(t)
	store, fake, engine, _ := openTenantDeploymentTestEngine(t)
	first, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err != nil || first.State != TenantDeploymentReady || first.Hostname != plan.Hostname {
		t.Fatalf("first deploy = %+v err=%v", first, err)
	}
	second, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err != nil || second.JobID != first.JobID || second.State != TenantDeploymentReady {
		t.Fatalf("idempotent retry = %+v err=%v", second, err)
	}
	if got := len(fake.SnapshotCalls()); got != len(plan.Actions) {
		t.Fatalf("retry repeated adapter calls: %v", fake.SnapshotCalls())
	}
	conflicting := plan
	conflicting.ManifestSHA256 = strings.Repeat("c", 64)
	if _, conflictErr := engine.Deploy(context.Background(), conflicting, "intent-a"); conflictErr == nil || !strings.Contains(conflictErr.Error(), "idempotency conflict") {
		t.Fatalf("changed desired state reused intent: %v", conflictErr)
	}
	resources, err := engine.ListResources(context.Background(), plan.JobID)
	if err != nil || len(resources) != len(plan.Actions) {
		t.Fatalf("resources=%d err=%v", len(resources), err)
	}
	var jobs int64
	if err := store.db.Model(&tenantDeploymentJobRecord{}).Count(&jobs).Error; err != nil || jobs != 1 {
		t.Fatalf("jobs=%d err=%v", jobs, err)
	}

	_, _, otherPlan := tenantDeploymentFixture(t)
	otherPlan.JobID = "job-concurrent-safe"
	otherPlan.InstanceID = "instance-concurrent-safe"
	otherPlan.ManifestSHA256 = strings.Repeat("b", 64)
	var wait sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, deployErr := engine.Deploy(context.Background(), otherPlan, "intent-concurrent")
			errs <- deployErr
		}()
	}
	wait.Wait()
	close(errs)
	for deployErr := range errs {
		if deployErr != nil {
			t.Fatalf("concurrent retry failed: %v", deployErr)
		}
	}
	status, err := store.Status(context.Background(), otherPlan.JobID, profile.ProfileID)
	if err != nil || status.State != TenantDeploymentReady {
		t.Fatalf("concurrent status = %+v err=%v", status, err)
	}
}
func TestTenantDeploymentAdaptersConfigUpdateReusesInstance(t *testing.T) {
	profile, manifest, firstPlan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	if _, err := engine.Deploy(context.Background(), firstPlan, "intent-a"); err != nil {
		t.Fatal(err)
	}
	manifest.ConfigRevision = "revision-18"
	secondPlan, err := BuildTenantDeploymentPlan(profile, manifest, "intent-b")
	if err != nil {
		t.Fatal(err)
	}
	if secondPlan.InstanceID != firstPlan.InstanceID || secondPlan.Namespace != firstPlan.Namespace || secondPlan.Hostname != firstPlan.Hostname || secondPlan.JobID == firstPlan.JobID {
		t.Fatalf("update identity changed incorrectly: first=%+v second=%+v", firstPlan, secondPlan)
	}
	status, err := engine.Deploy(context.Background(), secondPlan, "intent-b")
	if err != nil || status.State != TenantDeploymentReady || status.InstanceID != firstPlan.InstanceID {
		t.Fatalf("update status = %+v err=%v", status, err)
	}
	resources, err := engine.ListResources(context.Background(), secondPlan.JobID)
	if err != nil || len(resources) != len(secondPlan.Actions) {
		t.Fatalf("updated resources=%d err=%v", len(resources), err)
	}
	for _, resource := range resources {
		if resource.InstanceID != firstPlan.InstanceID || resource.JobID != secondPlan.JobID || resource.DesiredRevision != secondPlan.ManifestSHA256 {
			t.Fatalf("resource was not reconciled in place: %+v", resource)
		}
	}
	if got := len(fake.SnapshotCalls()); got != len(firstPlan.Actions)+len(secondPlan.Actions) {
		t.Fatalf("update did not reconcile exact action set: %v", fake.SnapshotCalls())
	}
	approval := TenantDeletionApproval{APIVersion: TenantDeletionApprovalAPIVersion, JobID: firstPlan.JobID, Action: "delete", ExpiresAt: time.Now().UTC().Add(time.Hour), RetainPVC: true, Nonce: "superseded", authenticated: true}
	if _, err := engine.Delete(context.Background(), firstPlan, approval, strings.Repeat("d", 64)); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("superseded job deletion was not rejected: %v", err)
	}
}

func TestTenantDeploymentAdaptersFailureClassificationAndResume(t *testing.T) {
	_, _, plan := tenantDeploymentFixture(t)
	store, fake, engine, _ := openTenantDeploymentTestEngine(t)
	fake.FailAction = "router"
	fake.FailClass = "synthetic_router_failure"
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err == nil || status.State != TenantDeploymentFailed || !status.Retryable || status.ErrorClass != "synthetic_router_failure" || status.Hostname != "" {
		t.Fatalf("failed deploy = %+v err=%v", status, err)
	}
	if strings.Contains(err.Error(), "injected fake adapter failure") {
		t.Fatalf("raw adapter error crossed boundary: %v", err)
	}
	fake.FailAction = ""
	resumed, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err != nil || resumed.State != TenantDeploymentReady {
		t.Fatalf("resume = %+v err=%v", resumed, err)
	}
	var stateAttempts int64
	if err := store.db.Model(&tenantDeploymentAttemptRecord{}).Where("job_id = ? AND action = ?", plan.JobID, "state_pvc").Count(&stateAttempts).Error; err != nil || stateAttempts != 1 {
		t.Fatalf("durable resource repeated: attempts=%d err=%v", stateAttempts, err)
	}
	var routerAttempts int64
	if err := store.db.Model(&tenantDeploymentAttemptRecord{}).Where("job_id = ? AND action = ?", plan.JobID, "router").Count(&routerAttempts).Error; err != nil || routerAttempts != 2 {
		t.Fatalf("failed action was not retried: attempts=%d err=%v", routerAttempts, err)
	}
}
func TestTenantDeploymentAdaptersUnknownDurableOutcomeNeedsOperator(t *testing.T) {
	_, _, plan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	fake.FailAction = "router"
	fake.FailClass = "synthetic_unknown_outcome"
	fake.FailUnknownOutcome = true
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err == nil || status.State != TenantDeploymentOperatorRequired || status.Retryable || status.NextAction != "operator_review" {
		t.Fatalf("unknown outcome = %+v err=%v", status, err)
	}
	fake.FailAction = ""
	if resumed, retryErr := engine.Deploy(context.Background(), plan, "intent-a"); retryErr == nil || resumed.State != TenantDeploymentOperatorRequired {
		t.Fatalf("operator-required job resumed without reconciliation: %+v err=%v", resumed, retryErr)
	}
}
func TestTenantDeploymentAdaptersPredurableUnknownOutcomeIsRetryable(t *testing.T) {
	_, _, plan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	fake.FailAction = "namespace"
	fake.FailClass = "synthetic_unknown_outcome"
	fake.FailUnknownOutcome = true
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err == nil || status.State != TenantDeploymentFailed || !status.Retryable || status.NextAction != "namespace" {
		t.Fatalf("pre-durable unknown outcome = %+v err=%v", status, err)
	}
}
func TestTenantDeploymentAdaptersUnknownStateOutcomeNeedsOperator(t *testing.T) {
	_, _, plan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	fake.FailAction = "state_pvc"
	fake.FailClass = "synthetic_unknown_outcome"
	fake.FailUnknownOutcome = true
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err == nil || status.State != TenantDeploymentOperatorRequired || status.Retryable || status.NextAction != "operator_review" {
		t.Fatalf("unknown state outcome = %+v err=%v", status, err)
	}
}

func TestTenantDeploymentSecurityNormalizedPrivateRegistryAndStatus(t *testing.T) {
	profile, _, plan := tenantDeploymentFixture(t)
	store, _, engine, path := openTenantDeploymentTestEngine(t)
	if _, err := engine.Deploy(context.Background(), plan, "intent-a"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("registry mode=%v err=%v", info.Mode().Perm(), err)
	}
	for _, table := range []string{"tenant_deployment_jobs", "tenant_deployment_attempts", "tenant_deployment_resources", "tenant_deployment_approvals"} {
		var columns []struct{ Name, Type string }
		if err := store.db.Raw("PRAGMA table_info(" + table + ")").Scan(&columns).Error; err != nil {
			t.Fatal(err)
		}
		for _, column := range columns {
			if strings.Contains(strings.ToLower(column.Type), "json") || strings.Contains(strings.ToLower(column.Type), "array") {
				t.Fatalf("%s.%s uses forbidden type %s", table, column.Name, column.Type)
			}
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenTenantDeploymentStoreReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	status, err := readOnly.Status(context.Background(), plan.JobID, profile.ProfileID)
	if err != nil || status.State != TenantDeploymentReady {
		t.Fatalf("read-only status = %+v err=%v", status, err)
	}
	missing := filepath.Join(t.TempDir(), "absent.sqlite")
	if _, err := OpenTenantDeploymentStoreReadOnly(missing); err == nil {
		t.Fatal("read-only status created an absent registry")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("absent status mutated filesystem: %v", err)
	}
}

func TestTenantDeploymentSecurityHostnameRequiresActivation(t *testing.T) {
	_, _, plan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	fake.FailAction = "activation"
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err == nil || status.Hostname != "" {
		t.Fatalf("activation failure published hostname: %+v err=%v", status, err)
	}
	for _, call := range fake.SnapshotCalls() {
		if call == "ensure:hostname" {
			t.Fatalf("hostname adapter ran before activation passed: %v", fake.SnapshotCalls())
		}
	}
}

func TestTenantDeploymentSecurityDeletionApprovalAndRetention(t *testing.T) {
	profile, _, plan := tenantDeploymentFixture(t)
	store, fake, engine, _ := openTenantDeploymentTestEngine(t)
	if _, err := engine.Deploy(context.Background(), plan, "intent-a"); err != nil {
		t.Fatal(err)
	}
	untrusted := TenantDeletionApproval{APIVersion: TenantDeletionApprovalAPIVersion, JobID: plan.JobID, Action: "delete", ExpiresAt: time.Now().UTC().Add(time.Hour), RetainPVC: true, Nonce: "untrusted"}
	if _, err := engine.Delete(context.Background(), plan, untrusted, strings.Repeat("b", 64)); err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("untrusted deletion approval was not rejected: %v", err)
	}
	approval := TenantDeletionApproval{APIVersion: TenantDeletionApprovalAPIVersion, JobID: plan.JobID, Action: "delete", ExpiresAt: time.Now().UTC().Add(time.Hour), RetainPVC: true, Nonce: "approval-a"}
	signDeletionApproval(t, profile, &approval)
	approvalBytes, _ := json.Marshal(approval)
	approvalPath := filepath.Join(t.TempDir(), "approval.json")
	if err := os.WriteFile(approvalPath, approvalBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, sum, err := LoadTenantDeletionApproval(approvalPath, profile, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	forged := approval
	forged.RetainPVC = false
	forgedBytes, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	forgedPath := filepath.Join(t.TempDir(), "forged-approval.json")
	if err := os.WriteFile(forgedPath, forgedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTenantDeletionApproval(forgedPath, profile, time.Now().UTC()); err == nil {
		t.Fatal("forged deletion approval was accepted")
	}
	fake.FailAction = "delete:router"
	if _, err := engine.Delete(context.Background(), plan, loaded, sum); err == nil {
		t.Fatal("partial delete failure was not returned")
	}
	partiallyDeleted, err := engine.ListResources(context.Background(), plan.JobID)
	if err != nil {
		t.Fatal(err)
	}
	partialStates := map[string]string{}
	for _, resource := range partiallyDeleted {
		partialStates[resource.ResourceKind] = resource.State
	}
	if partialStates["hostname"] != "deleted" || partialStates["activation"] != "deleted" || partialStates["router"] != "ready" {
		t.Fatalf("partial delete checkpoint was not durable: %v", partialStates)
	}
	fake.FailAction = ""
	status, err := engine.Delete(context.Background(), plan, loaded, sum)
	if err != nil || status.State != TenantDeploymentDeleted {
		t.Fatalf("delete = %+v err=%v", status, err)
	}
	resources, err := engine.ListResources(context.Background(), plan.JobID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, resource := range resources {
		states[resource.ResourceKind] = resource.State
	}
	if states["state_pvc"] != "retained" || states["hostname"] != "deleted" {
		t.Fatalf("retention states = %v", states)
	}
	calls := fake.SnapshotCalls()
	firstDelete := ""
	for _, call := range calls {
		if strings.HasPrefix(call, "delete:") {
			firstDelete = call
			break
		}
	}
	if firstDelete != "delete:hostname" {
		t.Fatalf("hostname was not disabled first: %v", calls)
	}
	callCount := len(fake.SnapshotCalls())
	retried, err := engine.Delete(context.Background(), plan, loaded, sum)
	if err != nil || retried.State != TenantDeploymentDeleted || len(fake.SnapshotCalls()) != callCount {
		t.Fatalf("exact delete retry was not idempotent: %+v err=%v calls=%v", retried, err, fake.SnapshotCalls())
	}
	if err := os.Chmod(approvalPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTenantDeletionApproval(approvalPath, profile, time.Now().UTC()); err == nil {
		t.Fatal("world-readable approval accepted")
	}
	_ = store
}

type localActivationAdapter struct {
	baseURL string
	client  *http.Client
}

func (a localActivationAdapter) ValidateActivation(ctx context.Context, plan TenantDeploymentPlan) (string, error) {
	checks := []struct {
		method, path, auth, body string
		want                     int
	}{
		{"GET", "/readyz", "", "", http.StatusOK},
		{"GET", "/v1/models", "Bearer local-test-caller", "", http.StatusOK},
		{"POST", "/v1/chat/completions", "Bearer local-test-caller", `{"model":"test","messages":[{"role":"user","content":"health"}],"max_tokens":1}`, http.StatusOK},
		{"GET", "/metrics", "Bearer local-test-caller", "", http.StatusForbidden},
		{"GET", "/metrics", "Basic local-test-admin", "", http.StatusOK},
		{"GET", "/admin/usage", "Basic local-test-admin", "", http.StatusOK},
	}
	for _, check := range checks {
		request, err := http.NewRequestWithContext(ctx, check.method, a.baseURL+check.path, strings.NewReader(check.body))
		if err != nil {
			return "", err
		}
		if check.auth != "" {
			request.Header.Set("Authorization", check.auth)
		}
		if check.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := a.client.Do(request)
		if err != nil {
			return "", err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if response.StatusCode != check.want {
			return "", &TenantDeploymentAdapterError{Class: "activation_check_failed", Err: fmt.Errorf("%s returned %d", check.path, response.StatusCode)}
		}
	}
	return plan.JobID + "-activation", nil
}

func (a localActivationAdapter) DeleteActivation(context.Context, TenantDeploymentPlan, string) error {
	return nil
}

func TestTenantDeploymentActivationLocalMockAPIAndMetricsIsolation(t *testing.T) {
	var mu sync.Mutex
	chatRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch request.URL.Path {
		case "/readyz":
			writer.WriteHeader(http.StatusOK)
		case "/v1/models":
			if request.Header.Get("Authorization") != "Bearer local-test-caller" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writer.WriteHeader(http.StatusOK)
		case "/v1/chat/completions":
			if request.Header.Get("Authorization") != "Bearer local-test-caller" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			chatRequests++
			writer.Header().Set("X-Selected-Target", "fake-provider/fake-model")
			writer.WriteHeader(http.StatusOK)
		case "/metrics":
			if request.Header.Get("Authorization") != "Basic local-test-admin" {
				writer.WriteHeader(http.StatusForbidden)
				return
			}
			writer.WriteHeader(http.StatusOK)
		case "/admin/usage":
			if request.Header.Get("Authorization") != "Basic local-test-admin" || chatRequests != 1 {
				writer.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, _, plan := tenantDeploymentFixture(t)
	_, fake, engine, _ := openTenantDeploymentTestEngine(t)
	engine.adapters.Activation = localActivationAdapter{baseURL: server.URL, client: server.Client()}
	status, err := engine.Deploy(context.Background(), plan, "intent-a")
	if err != nil || status.State != TenantDeploymentReady || status.Hostname == "" {
		t.Fatalf("activation status = %+v err=%v", status, err)
	}
	calls := fake.SnapshotCalls()
	if len(calls) == 0 || calls[len(calls)-1] != "ensure:hostname" {
		t.Fatalf("hostname was not final mutation: %v", calls)
	}
}
