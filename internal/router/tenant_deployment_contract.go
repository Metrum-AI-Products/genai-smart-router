package router

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	TenantDeploymentManifestAPIVersion = "metrum.ai/smartrouter-deployment/v1"
	TenantDeploymentIntentAPIVersion   = "metrum.ai/smartrouter-deployment-intent/v1"
	TenantDeploymentProfileAPIVersion  = "metrum.ai/smartrouter-profile/v1"
	TenantDeletionApprovalAPIVersion   = "metrum.ai/smartrouter-delete-approval/v1"
	TenantReleaseLatestApproved        = "latest-approved"
	maxTenantDeploymentDocumentBytes   = 64 << 10
	maxTenantDeploymentIntentLifetime  = 24 * time.Hour
)

var (
	deploymentIDPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	awsRegionPattern        = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-[0-9]$`)
	sha256DigestPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]{0,127}@sha256:[0-9a-f]{64}$`)
	configRevisionPattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
	referencePattern        = regexp.MustCompile(`^aws-(ssm|secretsmanager):///[A-Za-z0-9/_.+=@-]{1,512}$`)
	hostnameSuffixPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)
	secretValueLikePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(sk-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{12,}|xox[baprs]-[a-z0-9-]{12,}|akia[0-9a-z]{12,})(?:$|[^a-z0-9])`),
		regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)(authorization|api[_-]?key|password|private[_-]?key|router[_-]?token)\s*[:=]`),
	}
)

// TenantDeploymentManifest is intentionally reference-only. It carries no
// provider credential, Router token, license payload, DSN, kubeconfig, or
// production configuration content.
type TenantDeploymentManifest struct {
	APIVersion       string                  `json:"api_version" yaml:"api_version"`
	CustomerID       string                  `json:"customer_id" yaml:"customer_id"`
	Stage            string                  `json:"stage" yaml:"stage"`
	Release          string                  `json:"release" yaml:"release"`
	ResourceProfile  string                  `json:"resource_profile" yaml:"resource_profile"`
	StateProfile     string                  `json:"state_profile" yaml:"state_profile"`
	DatabaseProfile  string                  `json:"database_profile,omitempty" yaml:"database_profile,omitempty"`
	RuntimeBundleRef string                  `json:"runtime_bundle_ref" yaml:"runtime_bundle_ref"`
	ConfigRevision   string                  `json:"config_revision" yaml:"config_revision"`
	License          TenantDeploymentLicense `json:"license" yaml:"license"`
}

type TenantDeploymentLicense struct {
	RequestRef string `json:"request_ref" yaml:"request_ref"`
	Validity   string `json:"validity" yaml:"validity"`
}

// TenantDeploymentIntent is the one sealed input to Fleet deployment commands.
// It binds protected references to one immutable requested lifecycle operation.
// The document may contain references, but never the values they resolve to.
type TenantDeploymentIntent struct {
	APIVersion string                   `json:"api_version" yaml:"api_version"`
	IntentID   string                   `json:"intent_id" yaml:"intent_id"`
	IssuerRole string                   `json:"issuer_role" yaml:"issuer_role"`
	IssuedAt   time.Time                `json:"issued_at" yaml:"issued_at"`
	ExpiresAt  time.Time                `json:"expires_at" yaml:"expires_at"`
	ProfileRef string                   `json:"profile_ref" yaml:"profile_ref"`
	Manifest   TenantDeploymentManifest `json:"manifest" yaml:"manifest"`
	Signature  string                   `json:"signature" yaml:"signature"`
}

// TenantDeploymentProfile is protected deployment policy, not caller intent.
// The local fake-first implementation resolves only mode-0600 file:// profiles;
// live protected-profile adapters remain a separate, explicit gate.
type TenantDeploymentProfile struct {
	APIVersion                 string `json:"api_version" yaml:"api_version"`
	ProfileID                  string `json:"profile_id" yaml:"profile_id"`
	Environment                string `json:"environment" yaml:"environment"`
	AccountAlias               string `json:"account_alias" yaml:"account_alias"`
	Region                     string `json:"region" yaml:"region"`
	ClusterAlias               string `json:"cluster_alias" yaml:"cluster_alias"`
	NamespacePrefix            string `json:"namespace_prefix" yaml:"namespace_prefix"`
	HostnameSuffix             string `json:"hostname_suffix" yaml:"hostname_suffix"`
	ApprovedReleaseDigest      string `json:"approved_release_digest" yaml:"approved_release_digest"`
	ApprovedResourceProfile    string `json:"approved_resource_profile" yaml:"approved_resource_profile"`
	ApprovedStateProfile       string `json:"approved_state_profile" yaml:"approved_state_profile"`
	AWSProfile                 string `json:"aws_profile" yaml:"aws_profile"`
	LifecycleApprovalIssuer    string `json:"lifecycle_approval_issuer" yaml:"lifecycle_approval_issuer"`
	LifecycleApprovalPublicKey string `json:"lifecycle_approval_public_key" yaml:"lifecycle_approval_public_key"`
	StorageClass               string `json:"storage_class" yaml:"storage_class"`
	StateStorageGiB            int    `json:"state_storage_gib" yaml:"state_storage_gib"`
	IngressClassName           string `json:"ingress_class_name" yaml:"ingress_class_name"`
	IngressNamespace           string `json:"ingress_namespace" yaml:"ingress_namespace"`
	TLSSecretName              string `json:"tls_secret_name" yaml:"tls_secret_name"`
	DatabaseMode               string `json:"database_mode" yaml:"database_mode"`
	ApprovedDatabaseProfile    string `json:"approved_database_profile" yaml:"approved_database_profile"`
	RDSInstanceClass           string `json:"rds_instance_class" yaml:"rds_instance_class"`
	RDSStorageGiB              int    `json:"rds_storage_gib" yaml:"rds_storage_gib"`
	RDSBackupRetentionDays     int    `json:"rds_backup_retention_days" yaml:"rds_backup_retention_days"`
	RDSSubnetGroup             string `json:"rds_subnet_group" yaml:"rds_subnet_group"`
	RDSVPCSecurityGroup        string `json:"rds_vpc_security_group" yaml:"rds_vpc_security_group"`
	RDSMasterUsername          string `json:"rds_master_username" yaml:"rds_master_username"`
	RDSProxyDisabled           bool   `json:"rds_proxy_disabled" yaml:"rds_proxy_disabled"`
}

type TenantDeploymentPlan struct {
	Schema            string   `json:"schema"`
	Mode              string   `json:"mode"`
	JobID             string   `json:"job_id"`
	InstanceID        string   `json:"instance_id"`
	ProfileID         string   `json:"profile_id"`
	Environment       string   `json:"environment"`
	AccountAlias      string   `json:"account_alias"`
	Region            string   `json:"region"`
	ClusterAlias      string   `json:"cluster_alias"`
	CustomerID        string   `json:"customer_id"`
	Stage             string   `json:"stage"`
	Namespace         string   `json:"namespace"`
	Hostname          string   `json:"hostname"`
	ReleaseDigest     string   `json:"release_digest"`
	ResourceProfile   string   `json:"resource_profile"`
	StateProfile      string   `json:"state_profile"`
	ConfigRevision    string   `json:"config_revision"`
	ManifestSHA256    string   `json:"manifest_sha256"`
	DatabaseProfile   string   `json:"database_profile,omitempty"`
	DatabaseID        string   `json:"database_id,omitempty"`
	Actions           []string `json:"actions"`
	runtimeBundleRef  string
	licenseRequestRef string
}

func LoadTenantDeploymentManifest(path string, stdin io.Reader) (TenantDeploymentManifest, error) {
	data, err := readDeploymentDocument(path, stdin, false)
	if err != nil {
		return TenantDeploymentManifest{}, fmt.Errorf("read deployment manifest: %w", err)
	}
	var manifest TenantDeploymentManifest
	if err := decodeStrictDeploymentDocument(data, path, &manifest); err != nil {
		return TenantDeploymentManifest{}, errors.New("deployment manifest schema is invalid")
	}
	if err := validateTenantDeploymentManifest(manifest, data); err != nil {
		return TenantDeploymentManifest{}, err
	}
	return manifest, nil
}

// LoadTenantDeploymentIntent loads and authenticates one sealed Fleet intent.
// Local file:// profiles are accepted only for test fixtures; live callers must
// use the protected aws-ssm profile reference enforced by the Fleet command.
func LoadTenantDeploymentIntent(path string, now time.Time) (TenantDeploymentIntent, TenantDeploymentProfile, error) {
	if path == "-" {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, errors.New("deployment intent must be a protected regular file")
	}
	data, err := readDeploymentDocument(path, nil, true)
	if err != nil {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, errors.New("read deployment intent")
	}
	var intent TenantDeploymentIntent
	if err := decodeStrictDeploymentDocument(data, path, &intent); err != nil {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, errors.New("deployment intent schema is invalid")
	}
	if err := validateTenantDeploymentIntent(intent, data, now); err != nil {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, err
	}
	profile, err := LoadTenantDeploymentProfile(intent.ProfileRef)
	if err != nil {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, errors.New("load deployment profile for intent")
	}
	payload, err := TenantDeploymentIntentSigningPayload(intent)
	if err != nil || verifyLifecycleApprovalSignature(profile, intent.IssuerRole, intent.Signature, payload) != nil {
		return TenantDeploymentIntent{}, TenantDeploymentProfile{}, errors.New("deployment intent is not authenticated")
	}
	return intent, profile, nil
}

// TenantDeploymentIntentSigningPayload returns the exact canonical payload an
// authorized control-plane issuer signs before handing the intent to Fleet.
func TenantDeploymentIntentSigningPayload(intent TenantDeploymentIntent) ([]byte, error) {
	return json.Marshal(struct {
		APIVersion string                   `json:"api_version"`
		IntentID   string                   `json:"intent_id"`
		IssuerRole string                   `json:"issuer_role"`
		IssuedAt   time.Time                `json:"issued_at"`
		ExpiresAt  time.Time                `json:"expires_at"`
		ProfileRef string                   `json:"profile_ref"`
		Manifest   TenantDeploymentManifest `json:"manifest"`
	}{
		APIVersion: intent.APIVersion, IntentID: intent.IntentID, IssuerRole: intent.IssuerRole,
		IssuedAt: intent.IssuedAt, ExpiresAt: intent.ExpiresAt, ProfileRef: intent.ProfileRef,
		Manifest: intent.Manifest,
	})
}

func LoadTenantDeploymentProfile(ref string) (TenantDeploymentProfile, error) {
	parsed, err := url.Parse(ref)
	if err != nil || parsed.Scheme == "" {
		return TenantDeploymentProfile{}, errors.New("profile reference must be an absolute reference")
	}
	if parsed.Scheme == "aws-ssm" {
		if parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || !referencePattern.MatchString(ref) {
			return TenantDeploymentProfile{}, errors.New("SSM profile reference is invalid")
		}
		return loadTenantDeploymentProfileSSM(ref)
	}
	if parsed.Scheme != "file" {
		return TenantDeploymentProfile{}, errors.New("profile reference must be a protected aws-ssm:/// reference or a local file:// test fixture")
	}
	if parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return TenantDeploymentProfile{}, errors.New("file profile reference must contain only an absolute local path")
	}
	data, err := readDeploymentDocument(filepath.Clean(parsed.Path), nil, true)
	if err != nil {
		return TenantDeploymentProfile{}, fmt.Errorf("read deployment profile: %w", err)
	}
	var profile TenantDeploymentProfile
	if err := decodeStrictDeploymentDocument(data, parsed.Path, &profile); err != nil {
		return TenantDeploymentProfile{}, errors.New("deployment profile schema is invalid")
	}
	if err := validateTenantDeploymentProfile(profile, data); err != nil {
		return TenantDeploymentProfile{}, err
	}
	return profile, nil
}
func validateTenantDeploymentIntent(intent TenantDeploymentIntent, raw []byte, now time.Time) error {
	if intent.APIVersion != TenantDeploymentIntentAPIVersion {
		return fmt.Errorf("api_version must be %q", TenantDeploymentIntentAPIVersion)
	}
	for name, value := range map[string]string{"intent_id": intent.IntentID, "issuer_role": intent.IssuerRole} {
		if err := validateDeploymentID(name, value); err != nil {
			return err
		}
	}
	parsed, err := url.Parse(intent.ProfileRef)
	if err != nil || parsed.Scheme == "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("profile_ref must be a protected aws-ssm:/// reference or a local file:// test fixture")
	}
	if parsed.Scheme == "aws-ssm" && !referencePattern.MatchString(intent.ProfileRef) {
		return errors.New("profile_ref must be a protected aws-ssm:/// reference or a local file:// test fixture")
	}
	if parsed.Scheme == "file" && parsed.Path == "" {
		return errors.New("profile_ref must be a protected aws-ssm:/// reference or a local file:// test fixture")
	}
	if parsed.Scheme != "aws-ssm" && parsed.Scheme != "file" {
		return errors.New("profile_ref must be a protected aws-ssm:/// reference or a local file:// test fixture")
	}
	if intent.IssuedAt.IsZero() || intent.ExpiresAt.IsZero() || !intent.ExpiresAt.After(intent.IssuedAt) ||
		intent.ExpiresAt.Sub(intent.IssuedAt) > maxTenantDeploymentIntentLifetime || !intent.ExpiresAt.After(now.UTC()) {
		return errors.New("deployment intent lifetime is invalid or expired")
	}
	manifestRaw, err := json.Marshal(intent.Manifest)
	if err != nil {
		return errors.New("deployment intent manifest cannot be canonicalized")
	}
	if err := validateTenantDeploymentManifest(intent.Manifest, manifestRaw); err != nil {
		return err
	}
	if _, err := base64.StdEncoding.DecodeString(intent.Signature); err != nil {
		return errors.New("deployment intent signature is malformed")
	}
	return rejectSecretShapedDeploymentData(raw)
}

func BuildTenantDeploymentPlan(profile TenantDeploymentProfile, manifest TenantDeploymentManifest, intentID string) (TenantDeploymentPlan, error) {
	if err := validateDeploymentID("intent_id", intentID); err != nil {
		return TenantDeploymentPlan{}, err
	}
	if manifest.ResourceProfile != profile.ApprovedResourceProfile {
		return TenantDeploymentPlan{}, errors.New("resource_profile is not approved by the protected profile")
	}
	if manifest.StateProfile != profile.ApprovedStateProfile {
		return TenantDeploymentPlan{}, errors.New("state_profile is not approved by the protected profile")
	}
	if manifest.Release != TenantReleaseLatestApproved {
		return TenantDeploymentPlan{}, errors.New("release must be latest-approved")
	}
	dedicatedRDS := manifest.DatabaseProfile != ""
	if dedicatedRDS && (profile.DatabaseMode != "dedicated-rds" || manifest.DatabaseProfile != profile.ApprovedDatabaseProfile) {
		return TenantDeploymentPlan{}, errors.New("database_profile is not approved by the protected profile")
	}
	instanceIdentity := strings.Join([]string{profile.ProfileID, manifest.CustomerID, manifest.Stage}, "\x00")
	instanceSum := sha256.Sum256([]byte(instanceIdentity))
	instanceSuffix := hex.EncodeToString(instanceSum[:])[:20]
	jobSum := sha256.Sum256([]byte(instanceIdentity + "\x00" + intentID))
	jobSuffix := hex.EncodeToString(jobSum[:])[:20]
	namespace := truncateDNSLabel(profile.NamespacePrefix+"-"+instanceSuffix, 63)
	hostname := namespace + "." + profile.HostnameSuffix
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return TenantDeploymentPlan{}, fmt.Errorf("canonicalize deployment manifest: %w", err)
	}
	manifestSum := sha256.Sum256(manifestBytes)
	databaseID := ""
	if dedicatedRDS {
		databaseID = "rds-" + instanceSuffix
	}
	return TenantDeploymentPlan{
		Schema: "metrum.ai/smartrouter-deployment-plan/v1", Mode: "eks",
		JobID: "job-" + jobSuffix, InstanceID: "instance-" + instanceSuffix,
		ProfileID: profile.ProfileID, Environment: profile.Environment, AccountAlias: profile.AccountAlias,
		Region: profile.Region, ClusterAlias: profile.ClusterAlias, CustomerID: manifest.CustomerID,
		Stage: manifest.Stage, Namespace: namespace, Hostname: hostname,
		ReleaseDigest: profile.ApprovedReleaseDigest, ResourceProfile: manifest.ResourceProfile,
		StateProfile: manifest.StateProfile, ConfigRevision: manifest.ConfigRevision,
		ManifestSHA256:   hex.EncodeToString(manifestSum[:]),
		runtimeBundleRef: manifest.RuntimeBundleRef, licenseRequestRef: manifest.License.RequestRef,
		DatabaseProfile: manifest.DatabaseProfile,
		DatabaseID:      databaseID,
		Actions:         tenantDeploymentActions(dedicatedRDS),
	}, nil
}

func tenantDeploymentActions(dedicatedRDS bool) []string {
	actions := []string{"namespace", "network_policy"}
	if dedicatedRDS {
		actions = append(actions, "dedicated_rds")
	}
	return append(actions, "runtime_secret_binding", "license_binding", "state_pvc", "router", "activation", "hostname")
}

func readDeploymentDocument(path string, stdin io.Reader, requirePrivate bool) ([]byte, error) {
	var reader io.Reader
	if path == "-" {
		if stdin == nil {
			return nil, errors.New("stdin is unavailable")
		}
		reader = stdin
	} else {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, errors.New("document file is unavailable")
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("document must be a regular file, not a symlink")
		}
		if requirePrivate && info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("protected document permissions must be 0600 or stricter")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.New("document file cannot be opened")
		}
		defer file.Close()
		reader = file
	}
	limited := io.LimitReader(reader, maxTenantDeploymentDocumentBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.New("document cannot be read")
	}
	if len(data) == 0 {
		return nil, errors.New("document is empty")
	}
	if len(data) > maxTenantDeploymentDocumentBytes {
		return nil, fmt.Errorf("document exceeds %d bytes", maxTenantDeploymentDocumentBytes)
	}
	return data, nil
}

func decodeStrictDeploymentDocument(data []byte, name string, destination any) error {
	if strings.HasSuffix(strings.ToLower(name), ".json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(destination); err != nil {
			return err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return errors.New("document contains trailing data")
		}
		return nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return errors.New("document contains multiple YAML documents")
	}
	return nil
}

func validateTenantDeploymentManifest(manifest TenantDeploymentManifest, raw []byte) error {
	if manifest.APIVersion != TenantDeploymentManifestAPIVersion {
		return fmt.Errorf("api_version must be %q", TenantDeploymentManifestAPIVersion)
	}
	for name, value := range map[string]string{"customer_id": manifest.CustomerID, "stage": manifest.Stage, "resource_profile": manifest.ResourceProfile, "state_profile": manifest.StateProfile} {
		if err := validateDeploymentID(name, value); err != nil {
			return err
		}
	}
	if manifest.Release != TenantReleaseLatestApproved {
		return errors.New("release must be latest-approved")
	}
	if !configRevisionPattern.MatchString(manifest.ConfigRevision) {
		return errors.New("config_revision is required and must be an opaque revision identifier")
	}
	if manifest.DatabaseProfile != "" {
		if err := validateDeploymentID("database_profile", manifest.DatabaseProfile); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{"runtime_bundle_ref": manifest.RuntimeBundleRef, "license.request_ref": manifest.License.RequestRef} {
		if !referencePattern.MatchString(value) {
			return fmt.Errorf("%s must be an aws-ssm:/// or aws-secretsmanager:/// reference without query data", name)
		}
	}
	validity, err := time.ParseDuration(manifest.License.Validity)
	if err != nil || validity < time.Hour || validity > 31*24*time.Hour {
		return errors.New("license.validity must be between 1h and 744h")
	}
	return rejectSecretShapedDeploymentData(raw)
}

func validateTenantDeploymentProfile(profile TenantDeploymentProfile, raw []byte) error {
	if profile.APIVersion != TenantDeploymentProfileAPIVersion {
		return fmt.Errorf("api_version must be %q", TenantDeploymentProfileAPIVersion)
	}
	for name, value := range map[string]string{
		"profile_id": profile.ProfileID, "account_alias": profile.AccountAlias, "cluster_alias": profile.ClusterAlias,
		"namespace_prefix": profile.NamespacePrefix, "approved_resource_profile": profile.ApprovedResourceProfile,
		"approved_state_profile": profile.ApprovedStateProfile, "storage_class": profile.StorageClass,
		"ingress_class_name": profile.IngressClassName, "ingress_namespace": profile.IngressNamespace,
		"tls_secret_name": profile.TLSSecretName,
	} {
		if err := validateDeploymentID(name, value); err != nil {
			return err
		}
	}
	if _, err := lifecycleApprovalPublicKey(profile); err != nil {
		return errors.New("lifecycle approval authority is invalid")
	}
	if profile.DatabaseMode != "" && profile.DatabaseMode != "dedicated-rds" {
		return errors.New("database_mode must be dedicated-rds when set")
	}
	if profile.DatabaseMode == "dedicated-rds" {
		for name, value := range map[string]string{
			"approved_database_profile": profile.ApprovedDatabaseProfile,
			"rds_instance_class":        profile.RDSInstanceClass,
			"rds_subnet_group":          profile.RDSSubnetGroup,
			"rds_vpc_security_group":    profile.RDSVPCSecurityGroup,
			"rds_master_username":       profile.RDSMasterUsername,
		} {
			if err := validateDeploymentID(name, value); err != nil {
				return err
			}
		}
		if profile.RDSStorageGiB < 20 || profile.RDSStorageGiB > 65536 {
			return errors.New("rds_storage_gib must be between 20 and 65536")
		}
		if profile.RDSBackupRetentionDays < 1 || profile.RDSBackupRetentionDays > 35 {
			return errors.New("rds_backup_retention_days must be between 1 and 35")
		}
		if !profile.RDSProxyDisabled {
			return errors.New("rds_proxy_disabled must be true")
		}
	}
	if profile.Environment != "nonproduction" {
		return errors.New("production profiles are not accepted by the customer EKS lifecycle")
	}
	if !awsRegionPattern.MatchString(profile.Region) {
		return errors.New("region must be an AWS region identifier")
	}
	if !hostnameSuffixPattern.MatchString(profile.HostnameSuffix) || strings.Contains(profile.HostnameSuffix, "..") {
		return errors.New("hostname_suffix is invalid")
	}
	if !sha256DigestPattern.MatchString(profile.ApprovedReleaseDigest) {
		return errors.New("approved_release_digest must be an immutable repository@sha256 digest")
	}
	if profile.StateStorageGiB < 1 || profile.StateStorageGiB > 1024 {
		return errors.New("state_storage_gib must be between 1 and 1024")
	}
	return rejectSecretShapedDeploymentData(raw)
}

func lifecycleApprovalPublicKey(profile TenantDeploymentProfile) (ed25519.PublicKey, error) {
	if err := validateDeploymentID("lifecycle_approval_issuer", profile.LifecycleApprovalIssuer); err != nil {
		return nil, err
	}
	publicKey, err := base64.StdEncoding.DecodeString(profile.LifecycleApprovalPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("lifecycle approval public key is invalid")
	}
	return ed25519.PublicKey(publicKey), nil
}

func verifyLifecycleApprovalSignature(profile TenantDeploymentProfile, issuer, signature string, payload []byte) error {
	publicKey, err := lifecycleApprovalPublicKey(profile)
	if err != nil || issuer != profile.LifecycleApprovalIssuer {
		return errors.New("lifecycle approval is not authenticated")
	}
	signed, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(signed) != ed25519.SignatureSize || !ed25519.Verify(publicKey, payload, signed) {
		return errors.New("lifecycle approval is not authenticated")
	}
	return nil
}

func validateDeploymentID(name, value string) error {
	if !deploymentIDPattern.MatchString(value) || strings.Contains(strings.ToLower(value), "secret") || strings.Contains(strings.ToLower(value), "token") {
		return fmt.Errorf("%s must be a lowercase DNS-safe non-secret identifier", name)
	}
	return nil
}

func rejectSecretShapedDeploymentData(raw []byte) error {
	for _, pattern := range secretValueLikePatterns {
		if pattern.Match(raw) {
			return errors.New("document contains credential-shaped content; use a protected reference instead")
		}
	}
	return nil
}

func truncateDNSLabel(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return strings.TrimRight(value[:max], "-")
}
