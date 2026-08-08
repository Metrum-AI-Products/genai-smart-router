package router

import (
	"bytes"
	"crypto/sha256"
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
	TenantDeploymentProfileAPIVersion  = "metrum.ai/smartrouter-profile/v1"
	TenantDeletionApprovalAPIVersion   = "metrum.ai/smartrouter-delete-approval/v1"
	TenantReleaseLatestApproved        = "latest-approved"
	maxTenantDeploymentDocumentBytes   = 64 << 10
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
	APIVersion        string                  `json:"api_version" yaml:"api_version"`
	CustomerID        string                  `json:"customer_id" yaml:"customer_id"`
	Stage             string                  `json:"stage" yaml:"stage"`
	Release           string                  `json:"release" yaml:"release"`
	ResourceProfile   string                  `json:"resource_profile" yaml:"resource_profile"`
	StateProfile      string                  `json:"state_profile" yaml:"state_profile"`
	UpstreamConfigRef string                  `json:"upstream_config_ref" yaml:"upstream_config_ref"`
	ConfigRevision    string                  `json:"config_revision" yaml:"config_revision"`
	License           TenantDeploymentLicense `json:"license" yaml:"license"`
}

type TenantDeploymentLicense struct {
	RequestRef string `json:"request_ref" yaml:"request_ref"`
	Validity   string `json:"validity" yaml:"validity"`
}

// TenantDeploymentProfile is protected deployment policy, not caller intent.
// The local fake-first implementation resolves only mode-0600 file:// profiles;
// live protected-profile adapters remain a separate, explicit gate.
type TenantDeploymentProfile struct {
	APIVersion              string `json:"api_version" yaml:"api_version"`
	ProfileID               string `json:"profile_id" yaml:"profile_id"`
	Environment             string `json:"environment" yaml:"environment"`
	AccountAlias            string `json:"account_alias" yaml:"account_alias"`
	Region                  string `json:"region" yaml:"region"`
	ClusterAlias            string `json:"cluster_alias" yaml:"cluster_alias"`
	NamespacePrefix         string `json:"namespace_prefix" yaml:"namespace_prefix"`
	HostnameSuffix          string `json:"hostname_suffix" yaml:"hostname_suffix"`
	ApprovedReleaseDigest   string `json:"approved_release_digest" yaml:"approved_release_digest"`
	ApprovedResourceProfile string `json:"approved_resource_profile" yaml:"approved_resource_profile"`
	ApprovedStateProfile    string `json:"approved_state_profile" yaml:"approved_state_profile"`
	AWSProfile              string `json:"aws_profile" yaml:"aws_profile"`
	StorageClass            string `json:"storage_class" yaml:"storage_class"`
	StateStorageGiB         int    `json:"state_storage_gib" yaml:"state_storage_gib"`
	IngressClassName        string `json:"ingress_class_name" yaml:"ingress_class_name"`
	IngressNamespace        string `json:"ingress_namespace" yaml:"ingress_namespace"`
	TLSSecretName           string `json:"tls_secret_name" yaml:"tls_secret_name"`
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
	Actions           []string `json:"actions"`
	upstreamConfigRef string
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
	return TenantDeploymentPlan{
		Schema: "metrum.ai/smartrouter-deployment-plan/v1", Mode: "eks",
		JobID: "job-" + jobSuffix, InstanceID: "instance-" + instanceSuffix,
		ProfileID: profile.ProfileID, Environment: profile.Environment, AccountAlias: profile.AccountAlias,
		Region: profile.Region, ClusterAlias: profile.ClusterAlias, CustomerID: manifest.CustomerID,
		Stage: manifest.Stage, Namespace: namespace, Hostname: hostname,
		ReleaseDigest: profile.ApprovedReleaseDigest, ResourceProfile: manifest.ResourceProfile,
		StateProfile: manifest.StateProfile, ConfigRevision: manifest.ConfigRevision,
		ManifestSHA256:    hex.EncodeToString(manifestSum[:]),
		upstreamConfigRef: manifest.UpstreamConfigRef, licenseRequestRef: manifest.License.RequestRef,
		Actions: []string{"namespace", "network_policy", "runtime_secret_binding", "license_binding", "state_pvc", "router", "activation", "hostname"},
	}, nil
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
	for name, value := range map[string]string{"upstream_config_ref": manifest.UpstreamConfigRef, "license.request_ref": manifest.License.RequestRef} {
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
