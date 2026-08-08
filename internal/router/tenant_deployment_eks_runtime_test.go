package router

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestEKSRuntimeBindingCreatesExactConfigAndEnvironmentSecret(t *testing.T) {
	const configYAML = "server:\n  listen: :8080\nproviders: {}\n"
	const envJSON = `{"PROVIDER_API_KEY":"synthetic-test-value"}`
	const runtimeRef = "aws-secretsmanager:///safe/runtime-bundle"
	plan := TenantDeploymentPlan{InstanceID: "instance-a", Namespace: "tenant-a", runtimeBundleRef: runtimeRef}
	adapter := &EKSTenantDeploymentAdapters{
		kube: k8sfake.NewSimpleClientset(),
		resolveReference: func(_ context.Context, ref string) ([]byte, error) {
			if ref != runtimeRef {
				t.Fatalf("runtime resolver ref = %q", ref)
			}
			return protectedRuntimeBundle(t, configYAML, envJSON), nil
		},
	}
	if ref, err := adapter.EnsureSecretBinding(context.Background(), plan); err != nil || ref != "secret/router-runtime" {
		t.Fatalf("runtime binding ref=%q err=%v", ref, err)
	}
	secret, err := adapter.kube.CoreV1().Secrets(plan.Namespace).Get(context.Background(), "router-runtime", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(secret.Data) != 2 || string(secret.Data["config.yaml"]) != configYAML || string(secret.Data["env.json"]) != envJSON {
		t.Fatal("runtime secret does not contain exactly config.yaml and env.json")
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planJSON), runtimeRef) || strings.Contains(string(planJSON), "synthetic-test-value") {
		t.Fatalf("runtime reference or contents leaked into plan: %s", planJSON)
	}
}

func TestEKSRuntimeBindingRejectsInvalidBundleWithoutCreatingSecret(t *testing.T) {
	const canary = "synthetic-runtime-secret-must-not-escape"
	plan := TenantDeploymentPlan{InstanceID: "instance-a", Namespace: "tenant-a", runtimeBundleRef: "aws-ssm:///safe/runtime-bundle"}
	adapter := &EKSTenantDeploymentAdapters{
		kube: k8sfake.NewSimpleClientset(),
		resolveReference: func(context.Context, string) ([]byte, error) {
			return []byte(`{"config.yaml":"server: {}","env.json":"{}","extra":"` + canary + `"}`), nil
		},
	}
	_, err := adapter.EnsureSecretBinding(context.Background(), plan)
	if err != errInvalidTenantDeploymentRuntimeBundle || strings.Contains(err.Error(), canary) {
		t.Fatalf("invalid bundle error leaked protected data: %v", err)
	}
	secrets, listErr := adapter.kube.CoreV1().Secrets(plan.Namespace).List(context.Background(), metav1.ListOptions{})
	if listErr != nil || len(secrets.Items) != 0 {
		t.Fatalf("invalid bundle created runtime secret: items=%d err=%v", len(secrets.Items), listErr)
	}
}

func TestEKSRuntimeBindingSanitizesResolverErrors(t *testing.T) {
	const canary = "synthetic-runtime-secret-must-not-escape"
	adapter := &EKSTenantDeploymentAdapters{
		kube: k8sfake.NewSimpleClientset(),
		resolveReference: func(context.Context, string) ([]byte, error) {
			return nil, errors.New(canary)
		},
	}
	_, err := adapter.EnsureSecretBinding(context.Background(), TenantDeploymentPlan{InstanceID: "instance-a", Namespace: "tenant-a", runtimeBundleRef: "aws-ssm:///safe/runtime-bundle"})
	if err == nil || strings.Contains(err.Error(), canary) || err.Error() != "read protected runtime bundle" {
		t.Fatalf("unsafe runtime resolver error: %v", err)
	}
}

func TestEKSRouterDeploymentMountsRuntimeBundleAtImageConfigPath(t *testing.T) {
	plan := TenantDeploymentPlan{InstanceID: "instance-a", Namespace: "tenant-a", ReleaseDigest: "example/router@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	adapter := &EKSTenantDeploymentAdapters{kube: k8sfake.NewSimpleClientset()}
	if _, err := adapter.EnsureRouter(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	deployment, err := adapter.kube.AppsV1().Deployments(plan.Namespace).Get(context.Background(), "router", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	mounted := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "runtime" {
			mounted = mount.MountPath == "/app/config" && mount.ReadOnly
		}
	}
	if !mounted {
		t.Fatalf("router runtime mount = %#v, want read-only /app/config", container.VolumeMounts)
	}
	runtimeVolume := false
	licenseVolume := false
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "runtime" && volume.Secret != nil && volume.Secret.SecretName == "router-runtime" {
			runtimeVolume = true
		}
		if volume.Name == "license" && volume.Secret != nil && volume.Secret.SecretName == "router-license" {
			licenseVolume = true
		}
	}
	if !runtimeVolume || !licenseVolume {
		t.Fatalf("router deployment secret volumes runtime=%t license=%t", runtimeVolume, licenseVolume)
	}
}
