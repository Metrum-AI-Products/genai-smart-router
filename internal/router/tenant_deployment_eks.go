package router

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	tenantDeploymentOwnerLabel                  = "metrum.ai/smartrouter-instance"
	tenantDeploymentRuntimeBundleHashAnnotation = "metrum.ai/runtime-bundle-sha256"
)

// tenantDeploymentActivationWait bounds live readiness polling before hostname
// publish. Tests set it to zero so fake clients fail closed immediately.
var tenantDeploymentActivationWait = 3 * time.Minute
var tenantDeploymentActivationPoll = 5 * time.Second

// EKS tenant deployment adapters use typed AWS and Kubernetes clients. No
// command runner exists in this implementation.
type EKSTenantDeploymentAdapters struct {
	profile          TenantDeploymentProfile
	kube             kubernetes.Interface
	ssm              *ssm.Client
	secrets          *secretsmanager.Client
	database         TenantDatabaseAdapter
	resolveReference func(context.Context, string) ([]byte, error)
}

// ObserveTenantDeployment obtains a bounded live readback for status. It does
// not mutate the registry or the cluster and returns only a safe state class.
func ObserveTenantDeployment(ctx context.Context, profile TenantDeploymentProfile, status TenantDeploymentStatus) (string, error) {
	adapter, _, err := NewEKSTenantDeploymentAdapters(ctx, profile)
	if err != nil {
		return "", err
	}
	plan := TenantDeploymentPlan{InstanceID: status.InstanceID, Namespace: status.Namespace}
	deployment, err := adapter.kube.AppsV1().Deployments(plan.Namespace).Get(ctx, "router", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	if !owned(deployment.Labels, plan) || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		return "ownership_or_replica_mismatch", nil
	}
	pvc, err := adapter.kube.CoreV1().PersistentVolumeClaims(plan.Namespace).Get(ctx, "router-state", metav1.GetOptions{})
	if err != nil {
		return "not_ready", nil
	}
	if pvc.Status.Phase != corev1.ClaimBound || deployment.Status.AvailableReplicas != 1 {
		return "not_ready", nil
	}
	if status.State == TenantDeploymentReady {
		ingress, err := adapter.kube.NetworkingV1().Ingresses(plan.Namespace).Get(ctx, "router", metav1.GetOptions{})
		if err != nil || !owned(ingress.Labels, plan) {
			return "activation_or_ingress_mismatch", nil
		}
	}
	return "ready", nil
}

func NewEKSTenantDeploymentAdapters(ctx context.Context, profile TenantDeploymentProfile) (*EKSTenantDeploymentAdapters, TenantDeploymentAdapters, error) {
	return newEKSTenantDeploymentAdapters(ctx, profile, nil)
}

// NewApprovedEKSTenantDeploymentAdapters is intentionally separate from the
// default constructor. Only a validated, time-bounded non-production admission
// can attach the typed RDS adapter. The Fleet CLI reaches this constructor only
// after it consumes an externally issued, protected admission document; it
// never constructs that admission.
func NewApprovedEKSTenantDeploymentAdapters(ctx context.Context, profile TenantDeploymentProfile, admission TenantDeploymentRDSAdmission) (*EKSTenantDeploymentAdapters, TenantDeploymentAdapters, error) {
	if err := validateTenantDeploymentRDSAdmission(profile, admission, time.Now().UTC()); err != nil {
		return nil, TenantDeploymentAdapters{}, err
	}
	return newEKSTenantDeploymentAdapters(ctx, profile, &admission)
}

func newEKSTenantDeploymentAdapters(ctx context.Context, profile TenantDeploymentProfile, admission *TenantDeploymentRDSAdmission) (*EKSTenantDeploymentAdapters, TenantDeploymentAdapters, error) {
	cfg, err := tenantDeploymentAWSConfig(ctx, profile)
	if err != nil {
		return nil, TenantDeploymentAdapters{}, err
	}
	cluster, err := eks.NewFromConfig(cfg).DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(profile.ClusterAlias)})
	if err != nil || cluster.Cluster == nil || cluster.Cluster.Endpoint == nil || cluster.Cluster.CertificateAuthority == nil || cluster.Cluster.CertificateAuthority.Data == nil {
		return nil, TenantDeploymentAdapters{}, errors.New("describe approved EKS cluster")
	}
	if cluster.Cluster.Status == "" || string(cluster.Cluster.Status) != "ACTIVE" {
		return nil, TenantDeploymentAdapters{}, errors.New("approved EKS cluster is not active")
	}
	token, err := eksAuthenticationToken(ctx, cfg, profile.ClusterAlias)
	if err != nil {
		return nil, TenantDeploymentAdapters{}, err
	}
	ca, err := base64.StdEncoding.DecodeString(*cluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, TenantDeploymentAdapters{}, errors.New("decode EKS certificate authority")
	}
	kube, err := kubernetes.NewForConfig(&rest.Config{Host: *cluster.Cluster.Endpoint, BearerToken: token, TLSClientConfig: rest.TLSClientConfig{CAData: ca}, Timeout: 30 * time.Second})
	if err != nil {
		return nil, TenantDeploymentAdapters{}, errors.New("construct typed Kubernetes client")
	}
	a := &EKSTenantDeploymentAdapters{profile: profile, kube: kube, ssm: ssm.NewFromConfig(cfg), secrets: secretsmanager.NewFromConfig(cfg)}
	a.resolveReference = a.referenceValue
	a.database = a
	if admission != nil {
		database, err := NewTenantDeploymentRDSAdapter(profile, *admission, rds.NewFromConfig(cfg))
		if err != nil {
			return nil, TenantDeploymentAdapters{}, err
		}
		a.database = database
	}
	return a, TenantDeploymentAdapters{Namespace: a, NetworkPolicy: a, SecretBinding: a, LicenseBinding: a, State: a, Database: a.database, Router: a, Activation: a, Hostname: a}, nil
}

func eksAuthenticationToken(ctx context.Context, cfg aws.Config, cluster string) (string, error) {
	credentials, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", errors.New("retrieve AWS operator credentials")
	}
	// EKS bearer tokens are a base64url-wrapped STS GetCallerIdentity presign.
	// X-Amz-Expires must be present before signing; an empty-body SHA-256 is required.
	const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	u, err := url.Parse("https://sts." + cfg.Region + ".amazonaws.com/?Action=GetCallerIdentity&Version=2011-06-15&X-Amz-Expires=60")
	if err != nil {
		return "", errors.New("build EKS authentication URL")
	}
	req := &http.Request{Method: http.MethodGet, URL: u, Header: http.Header{"x-k8s-aws-id": []string{cluster}}}
	presigned, _, err := v4.NewSigner().PresignHTTP(ctx, credentials, req, emptyPayloadHash, "sts", cfg.Region, time.Now().UTC())
	if err != nil {
		return "", errors.New("presign EKS authentication token")
	}
	return "k8s-aws-v1." + base64.RawURLEncoding.EncodeToString([]byte(presigned)), nil
}

func (a *EKSTenantDeploymentAdapters) labels(plan TenantDeploymentPlan) map[string]string {
	return map[string]string{tenantDeploymentOwnerLabel: plan.InstanceID, "app.kubernetes.io/name": "smart-llmrouter", "app.kubernetes.io/managed-by": "metrum-fleetctl"}
}
func owned(labels map[string]string, plan TenantDeploymentPlan) bool {
	return labels != nil && labels[tenantDeploymentOwnerLabel] == plan.InstanceID
}
func ownershipError() error {
	return &TenantDeploymentAdapterError{Class: "ownership_conflict", UnknownOutcome: true, Err: errors.New("Kubernetes object is not owned by this deployment")}
}

func (a *EKSTenantDeploymentAdapters) EnsureNamespace(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	ns, err := a.kube.CoreV1().Namespaces().Get(ctx, p.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = a.kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: p.Namespace, Labels: a.labels(p)}}, metav1.CreateOptions{})
		return "namespace/" + p.Namespace, err
	}
	if err != nil {
		return "", err
	}
	if !owned(ns.Labels, p) {
		return "", ownershipError()
	}
	return "namespace/" + p.Namespace, nil
}
func (a *EKSTenantDeploymentAdapters) DeleteNamespace(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	ns, err := a.kube.CoreV1().Namespaces().Get(ctx, p.Namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !owned(ns.Labels, p) {
		return ownershipError()
	}
	return a.kube.CoreV1().Namespaces().Delete(ctx, p.Namespace, metav1.DeleteOptions{})
}

func (a *EKSTenantDeploymentAdapters) EnsureNetworkPolicy(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	name := "router-ingress"
	client := a.kube.NetworkingV1().NetworkPolicies(p.Namespace)
	current, err := client.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if !owned(current.Labels, p) {
			return "", ownershipError()
		}
		return "networkpolicy/" + name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: a.labels(p)}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{tenantDeploymentOwnerLabel: p.InstanceID}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": a.profile.IngressNamespace}}}}}}}}
	_, err = client.Create(ctx, policy, metav1.CreateOptions{})
	return "networkpolicy/" + name, err
}
func (a *EKSTenantDeploymentAdapters) DeleteNetworkPolicy(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	return a.deleteNetworkPolicy(ctx, p, "router-ingress")
}
func (a *EKSTenantDeploymentAdapters) deleteNetworkPolicy(ctx context.Context, p TenantDeploymentPlan, name string) error {
	c := a.kube.NetworkingV1().NetworkPolicies(p.Namespace)
	o, e := c.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(o.Labels, p) {
		return ownershipError()
	}
	return c.Delete(ctx, name, metav1.DeleteOptions{})
}

func (a *EKSTenantDeploymentAdapters) EnsureSecretBinding(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	return a.ensureRuntimeBundleSecret(ctx, p, pRuntimeBundleRef(p))
}
func (a *EKSTenantDeploymentAdapters) DeleteSecretBinding(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	return a.deleteSecret(ctx, p, "router-runtime")
}
func (a *EKSTenantDeploymentAdapters) EnsureLicenseBinding(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	return a.ensureReferenceSecret(ctx, p, "router-license", "license.json", pLicenseRef(p))
}
func (a *EKSTenantDeploymentAdapters) DeleteLicenseBinding(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	return a.deleteSecret(ctx, p, "router-license")
}

// References are retained only as private plan fields and resolved by the
// typed adapter in memory. This prevents protected references from entering
// status or resource rows.
func pRuntimeBundleRef(p TenantDeploymentPlan) string { return p.runtimeBundleRef }
func pLicenseRef(p TenantDeploymentPlan) string       { return p.licenseRequestRef }

func (a *EKSTenantDeploymentAdapters) ensureRuntimeBundleSecret(ctx context.Context, p TenantDeploymentPlan, ref string) (string, error) {
	value, err := a.resolveProtectedReference(ctx, ref)
	if err != nil {
		return "", errors.New("read protected runtime bundle")
	}
	bundle, err := parseTenantDeploymentRuntimeBundle(value)
	if err != nil {
		return "", err
	}
	configYAML := bundle.ConfigYAML
	envJSON := bundle.EnvJSON
	if p.DatabaseID != "" {
		rdsAdapter, ok := a.database.(*TenantDeploymentRDSAdapter)
		if !ok || rdsAdapter == nil {
			return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("dedicated RDS adapter is required for usage DSN binding")}
		}
		dsn, err := rdsAdapter.UsageDSN(ctx, p, a.secrets)
		if err != nil {
			return "", err
		}
		envJSON, err = injectRuntimeEnvJSONValue(envJSON, tenantDeploymentUsageDSNEnvKey, dsn)
		if err != nil {
			return "", &TenantDeploymentAdapterError{Class: "rds_credential_binding_failed", Err: errors.New("bind dedicated RDS usage DSN")}
		}
	} else {
		// Default Fleet path: SQLite on the owned PVC. Rewrite postgres /
		// deployment-job bundles so deploy is self-contained and repeatable.
		rewritten, err := applySQLiteUsageDBConfig(configYAML)
		if err != nil {
			return "", err
		}
		configYAML = rewritten
		envJSON, err = stripRuntimeEnvJSONKey(envJSON, tenantDeploymentUsageDSNEnvKey)
		if err != nil {
			return "", err
		}
	}
	data := map[string][]byte{"config.yaml": []byte(configYAML), "env.json": []byte(envJSON)}
	c := a.kube.CoreV1().Secrets(p.Namespace)
	existing, err := c.Get(ctx, "router-runtime", metav1.GetOptions{})
	if err == nil {
		if !owned(existing.Labels, p) {
			return "", ownershipError()
		}
		existing.Data = data
		if _, err := c.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return "", errors.New("write protected runtime bundle")
		}
		return "secret/router-runtime", nil
	}
	if !apierrors.IsNotFound(err) {
		return "", errors.New("read protected runtime secret")
	}
	if _, err := c.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "router-runtime", Namespace: p.Namespace, Labels: a.labels(p)}, Type: corev1.SecretTypeOpaque, Data: data}, metav1.CreateOptions{}); err != nil {
		return "", errors.New("write protected runtime bundle")
	}
	return "secret/router-runtime", nil
}

func (a *EKSTenantDeploymentAdapters) ensureReferenceSecret(ctx context.Context, p TenantDeploymentPlan, name, key, ref string) (string, error) {
	c := a.kube.CoreV1().Secrets(p.Namespace)
	existing, err := c.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if !owned(existing.Labels, p) {
			return "", ownershipError()
		}
		return "secret/" + name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	value, err := a.resolveProtectedReference(ctx, ref)
	if err != nil {
		return "", err
	}
	_, err = c.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: a.labels(p)}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{key: value}}, metav1.CreateOptions{})
	return "secret/" + name, err
}
func (a *EKSTenantDeploymentAdapters) deleteSecret(ctx context.Context, p TenantDeploymentPlan, name string) error {
	c := a.kube.CoreV1().Secrets(p.Namespace)
	o, e := c.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(o.Labels, p) {
		return ownershipError()
	}
	return c.Delete(ctx, name, metav1.DeleteOptions{})
}
func (a *EKSTenantDeploymentAdapters) resolveProtectedReference(ctx context.Context, ref string) ([]byte, error) {
	if a.resolveReference != nil {
		return a.resolveReference(ctx, ref)
	}
	return a.referenceValue(ctx, ref)
}

func (a *EKSTenantDeploymentAdapters) referenceValue(ctx context.Context, ref string) ([]byte, error) {
	if strings.HasPrefix(ref, "aws-ssm:///") {
		name := strings.TrimPrefix(ref, "aws-ssm:///")
		if name != "" && !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		r, e := a.ssm.GetParameter(ctx, &ssm.GetParameterInput{Name: &name, WithDecryption: aws.Bool(true)})
		if e != nil || r.Parameter == nil || r.Parameter.Value == nil {
			return nil, errors.New("read protected deployment reference")
		}
		return []byte(*r.Parameter.Value), nil
	}
	if strings.HasPrefix(ref, "aws-secretsmanager:///") {
		id := strings.TrimPrefix(ref, "aws-secretsmanager:///")
		r, e := a.secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &id})
		if e != nil || r.SecretString == nil {
			return nil, errors.New("read protected deployment reference")
		}
		return []byte(*r.SecretString), nil
	}
	return nil, errors.New("unsupported protected deployment reference")
}

func (a *EKSTenantDeploymentAdapters) EnsureStatePVC(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	name := "router-state"
	c := a.kube.CoreV1().PersistentVolumeClaims(p.Namespace)
	o, e := c.Get(ctx, name, metav1.GetOptions{})
	if e == nil {
		if !owned(o.Labels, p) || len(o.Spec.AccessModes) != 1 || o.Spec.AccessModes[0] != corev1.ReadWriteOnce {
			return "", ownershipError()
		}
		return "pvc/" + name, nil
	}
	if !apierrors.IsNotFound(e) {
		return "", e
	}
	q := resource.MustParse(fmt.Sprintf("%dGi", a.profile.StateStorageGiB))
	sc := a.profile.StorageClass
	_, e = c.Create(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: a.labels(p)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, StorageClassName: &sc, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: q}}}}, metav1.CreateOptions{})
	return "pvc/" + name, e
}
func (a *EKSTenantDeploymentAdapters) DeleteStatePVC(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	c := a.kube.CoreV1().PersistentVolumeClaims(p.Namespace)
	o, e := c.Get(ctx, "router-state", metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(o.Labels, p) {
		return ownershipError()
	}
	return c.Delete(ctx, "router-state", metav1.DeleteOptions{})
}

// EnsureDedicatedRDS is deliberately fail-closed. The typed EKS lifecycle can
// carry a dedicated-RDS plan and its fake adapter covers retry/retention
// semantics, but live RDS mutation remains disabled until the #555 recorded
// non-production RDS adapter, credential-binding, and activation review gates
// are accepted. It never resolves or emits a DSN.
func (a *EKSTenantDeploymentAdapters) EnsureDedicatedRDS(_ context.Context, plan TenantDeploymentPlan) (string, error) {
	if a.profile.DatabaseMode != "dedicated-rds" || plan.DatabaseID == "" || plan.DatabaseProfile != a.profile.ApprovedDatabaseProfile {
		return "", &TenantDeploymentAdapterError{Class: "rds_profile_invalid", Err: errors.New("dedicated RDS database profile is required")}
	}
	return "", &TenantDeploymentAdapterError{Class: "rds_live_admission_required", Err: errors.New("dedicated RDS live adapter is not approved")}
}

func (a *EKSTenantDeploymentAdapters) DeleteDedicatedRDS(_ context.Context, _ TenantDeploymentPlan, _ string) error {
	return &TenantDeploymentAdapterError{Class: "rds_live_admission_required", Err: errors.New("dedicated RDS live adapter is not approved")}
}

func (a *EKSTenantDeploymentAdapters) EnsureRouter(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	if err := a.rejectHPA(ctx, p); err != nil {
		return "", err
	}
	const name = "router"
	client := a.kube.AppsV1().Deployments(p.Namespace)
	bundleHash, err := a.runtimeBundleSHA256(ctx, p)
	if err != nil {
		return "", err
	}
	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if !owned(existing.Labels, p) || existing.Spec.Replicas == nil || *existing.Spec.Replicas != 1 {
			return "", ownershipError()
		}
		if err := a.applyRouterPodTemplate(existing, p, bundleHash); err != nil {
			return "", err
		}
		if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return "", errors.New("update router deployment")
		}
		return "deployment/" + name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	one := int32(1)
	labels := a.labels(p)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{tenantDeploymentOwnerLabel: p.InstanceID}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{}}},
	}
	if err := a.applyRouterPodTemplate(deployment, p, bundleHash); err != nil {
		return "", err
	}
	_, err = client.Create(ctx, deployment, metav1.CreateOptions{})
	return "deployment/" + name, err
}

func (a *EKSTenantDeploymentAdapters) runtimeBundleSHA256(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	secret, err := a.kube.CoreV1().Secrets(p.Namespace).Get(ctx, "router-runtime", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("read router-runtime secret for rollout hash")
	}
	sum := sha256.New()
	_, _ = sum.Write(secret.Data["config.yaml"])
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write(secret.Data["env.json"])
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (a *EKSTenantDeploymentAdapters) applyRouterPodTemplate(deployment *appsv1.Deployment, p TenantDeploymentPlan, bundleHash string) error {
	nonRoot := int64(65532)
	labels := a.labels(p)
	if deployment.Spec.Template.ObjectMeta.Labels == nil {
		deployment.Spec.Template.ObjectMeta.Labels = map[string]string{}
	}
	for k, v := range labels {
		deployment.Spec.Template.ObjectMeta.Labels[k] = v
	}
	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	if bundleHash != "" {
		deployment.Spec.Template.ObjectMeta.Annotations[tenantDeploymentRuntimeBundleHashAnnotation] = bundleHash
	}
	deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: aws.Bool(true), RunAsUser: &nonRoot, RunAsGroup: &nonRoot, FSGroup: &nonRoot}
	container := corev1.Container{
		Name:  "router",
		Image: p.ReleaseDigest,
		Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             aws.Bool(true),
			RunAsUser:                &nonRoot,
			RunAsGroup:               &nonRoot,
			AllowPrivilegeEscalation: aws.Bool(false),
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "state", MountPath: "/var/lib/smart-llmrouter"},
			{Name: "runtime", MountPath: "/app/config", ReadOnly: true},
			{Name: "license", MountPath: "/etc/smart-llmrouter-license", ReadOnly: true},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/readyz", Port: intstr.FromInt(8080)}},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
	deployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{Name: "state", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "router-state"}}},
		{Name: "runtime", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "router-runtime"}}},
		{Name: "license", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "router-license"}}},
	}
	return nil
}
func (a *EKSTenantDeploymentAdapters) DeleteRouter(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	c := a.kube.AppsV1().Deployments(p.Namespace)
	o, e := c.Get(ctx, "router", metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(o.Labels, p) {
		return ownershipError()
	}
	return c.Delete(ctx, "router", metav1.DeleteOptions{})
}
func (a *EKSTenantDeploymentAdapters) rejectHPA(ctx context.Context, p TenantDeploymentPlan) error {
	items, e := a.kube.AutoscalingV2().HorizontalPodAutoscalers(p.Namespace).List(ctx, metav1.ListOptions{LabelSelector: tenantDeploymentOwnerLabel + "=" + p.InstanceID})
	if e != nil {
		return e
	}
	for _, h := range items.Items {
		if h.Spec.ScaleTargetRef.Kind == "Deployment" && h.Spec.ScaleTargetRef.Name == "router" {
			return &TenantDeploymentAdapterError{Class: "hpa_forbidden", Err: errors.New("SQLite state requires one replica")}
		}
	}
	return nil
}

func (a *EKSTenantDeploymentAdapters) ValidateActivation(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	if err := a.rejectHPA(ctx, p); err != nil {
		return "", err
	}
	deadline := time.Now().UTC().Add(tenantDeploymentActivationWait)
	var last error
	for {
		pvc, e := a.kube.CoreV1().PersistentVolumeClaims(p.Namespace).Get(ctx, "router-state", metav1.GetOptions{})
		if e != nil || pvc.Status.Phase != corev1.ClaimBound {
			last = &TenantDeploymentAdapterError{Class: "state_not_bound", Err: errors.New("state PVC is not bound")}
		} else {
			d, e := a.kube.AppsV1().Deployments(p.Namespace).Get(ctx, "router", metav1.GetOptions{})
			if e != nil || !owned(d.Labels, p) || d.Status.AvailableReplicas != 1 {
				last = &TenantDeploymentAdapterError{Class: "router_not_ready", Err: errors.New("router deployment is not ready")}
			} else if d.Status.UpdatedReplicas != 1 || d.Status.ReadyReplicas != 1 || d.Status.ObservedGeneration < d.Generation {
				last = &TenantDeploymentAdapterError{Class: "router_not_ready", Err: errors.New("router deployment rollout is not complete")}
			} else {
				return "activation/router-ready", nil
			}
		}
		if !deadline.After(time.Now().UTC()) {
			return "", last
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(tenantDeploymentActivationPoll):
		}
	}
}
func (a *EKSTenantDeploymentAdapters) DeleteActivation(context.Context, TenantDeploymentPlan, string) error {
	return nil
}

func (a *EKSTenantDeploymentAdapters) EnableHostname(ctx context.Context, p TenantDeploymentPlan) (string, error) {
	const name = "router"
	services := a.kube.CoreV1().Services(p.Namespace)
	service, err := services.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = services.Create(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: a.labels(p)}, Spec: corev1.ServiceSpec{Selector: map[string]string{tenantDeploymentOwnerLabel: p.InstanceID}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(8080)}}}}, metav1.CreateOptions{})
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if !owned(service.Labels, p) {
		return "", ownershipError()
	}
	class := a.profile.IngressClassName
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.Namespace, Labels: a.labels(p)},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &class,
			TLS:              []networkingv1.IngressTLS{{Hosts: []string{p.Hostname}, SecretName: a.profile.TLSSecretName}},
			Rules: []networkingv1.IngressRule{{
				Host: p.Hostname,
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: name, Port: networkingv1.ServiceBackendPort{Number: 80}}},
				}}}},
			}},
		},
	}
	ingresses := a.kube.NetworkingV1().Ingresses(p.Namespace)
	existing, err := ingresses.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if !owned(existing.Labels, p) {
			return "", ownershipError()
		}
		return "ingress/" + name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", err
	}
	_, err = ingresses.Create(ctx, ingress, metav1.CreateOptions{})
	return "ingress/" + name, err
}
func (a *EKSTenantDeploymentAdapters) DisableHostname(ctx context.Context, p TenantDeploymentPlan, _ string) error {
	c := a.kube.NetworkingV1().Ingresses(p.Namespace)
	o, e := c.Get(ctx, "router", metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !owned(o.Labels, p) {
		return ownershipError()
	}
	return c.Delete(ctx, "router", metav1.DeleteOptions{})
}

// Compile-time guard: typed HPA API remains part of the adapter contract.
var _ = autoscalingv2.HorizontalPodAutoscaler{}
