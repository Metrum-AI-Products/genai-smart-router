// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const hostnameSuffix = "apps.example.test"

var customerIDRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// BYOKSpec declares the customer-owned upstream key and model (no secret values).
type BYOKSpec struct {
	Provider  string `json:"provider"`
	APIKeyEnv string `json:"api_key_env"`
	Model     string `json:"model"`
}

// Intent is the operator onboard document. Raw API keys must never appear here.
type Intent struct {
	CustomerID       string   `json:"customer_id"`
	Hostname         string   `json:"hostname"`
	SKU              string   `json:"sku"`
	CustomerEmail    string   `json:"customer_email"`
	CustomerAlias    string   `json:"customer_alias"`
	OwnerUser        string   `json:"owner_user"`
	Project          string   `json:"project"`
	ProfileRef       string   `json:"profile_ref"`
	LicenseRef       string   `json:"license_ref"`
	RuntimeBundleRef string   `json:"runtime_bundle_ref"`
	ConfigFile       string   `json:"config_file"`
	SignKey          string   `json:"sign_key"`
	LicenseKey       string   `json:"license_key"`
	LicenseKeyID     string   `json:"license_key_id"`
	CommerceBaseURL  string   `json:"commerce_base_url"`
	BYOKEnvFile      string   `json:"byok_env_file"`
	BYOK             BYOKSpec `json:"byok"`
	Catalog          string   `json:"catalog"`
	SmokeModel       string   `json:"smoke_model"`
	TokenOut         string   `json:"token_out"`
	SuccessURL       string   `json:"success_url"`
	CancelURL        string   `json:"cancel_url"`
	// LicenseAllowUnknownRuntimeKey is for sandbox/test keys absent from embedded runtime pubs.
	LicenseAllowUnknownRuntimeKey bool `json:"license_allow_unknown_runtime_key"`
}

// LoadIntent reads and validates an onboard intent JSON file.
func LoadIntent(path string) (Intent, error) {
	path = expandHome(strings.TrimSpace(path))
	raw, err := os.ReadFile(path)
	if err != nil {
		return Intent{}, fmt.Errorf("read intent: %w", err)
	}
	if err := rejectEmbeddedSecrets(raw); err != nil {
		return Intent{}, err
	}
	var intent Intent
	if err := json.Unmarshal(raw, &intent); err != nil {
		return Intent{}, fmt.Errorf("parse intent: %w", err)
	}
	normalized, err := ValidateIntent(intent)
	if err != nil {
		return Intent{}, err
	}
	return normalized, nil
}

func rejectEmbeddedSecrets(raw []byte) error {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	forbiddenTop := []string{"api_key", "provider_api_key", "openai_api_key", "anthropic_api_key", "stripe_secret_key"}
	for _, key := range forbiddenTop {
		if _, ok := probe[key]; ok {
			return fmt.Errorf("intent must not embed secret field %q; use byok_env_file", key)
		}
	}
	if byok, ok := probe["byok"].(map[string]any); ok {
		for _, key := range []string{"api_key", "key", "secret", "token", "value"} {
			if _, ok := byok[key]; ok {
				return fmt.Errorf("byok must not embed secret field %q; use byok_env_file", key)
			}
		}
	}
	return nil
}

// ValidateIntent normalizes and enforces hostname contract, refs, and BYOK metadata.
func ValidateIntent(intent Intent) (Intent, error) {
	id := strings.ToLower(strings.TrimSpace(intent.CustomerID))
	host := strings.ToLower(strings.TrimSpace(intent.Hostname))
	switch {
	case id == "" && host == "":
		return Intent{}, fmt.Errorf("customer_id or hostname is required")
	case id == "" && host != "":
		derived, err := customerIDFromHostname(host)
		if err != nil {
			return Intent{}, err
		}
		id = derived
	case id != "" && host == "":
		host = id + "." + hostnameSuffix
	}
	if !customerIDRE.MatchString(id) {
		return Intent{}, fmt.Errorf("customer_id must match ^[a-z][a-z0-9-]{1,30}$")
	}
	wantHost := id + "." + hostnameSuffix
	if host != wantHost {
		return Intent{}, fmt.Errorf("hostname must be %s (got %q)", wantHost, host)
	}
	intent.CustomerID = id
	intent.Hostname = wantHost
	intent.SKU = strings.TrimSpace(intent.SKU)
	intent.CustomerEmail = strings.TrimSpace(intent.CustomerEmail)
	intent.CustomerAlias = strings.TrimSpace(intent.CustomerAlias)
	intent.OwnerUser = strings.TrimSpace(intent.OwnerUser)
	intent.Project = strings.TrimSpace(intent.Project)
	intent.ProfileRef = strings.TrimSpace(intent.ProfileRef)
	intent.LicenseRef = strings.TrimSpace(intent.LicenseRef)
	intent.RuntimeBundleRef = strings.TrimSpace(intent.RuntimeBundleRef)
	intent.ConfigFile = expandHome(intent.ConfigFile)
	intent.SignKey = expandHome(intent.SignKey)
	intent.LicenseKey = expandHome(intent.LicenseKey)
	intent.CommerceBaseURL = strings.TrimRight(strings.TrimSpace(intent.CommerceBaseURL), "/")
	intent.BYOKEnvFile = expandHome(intent.BYOKEnvFile)
	intent.BYOK.Provider = strings.TrimSpace(intent.BYOK.Provider)
	intent.BYOK.APIKeyEnv = strings.TrimSpace(intent.BYOK.APIKeyEnv)
	intent.BYOK.Model = strings.TrimSpace(intent.BYOK.Model)
	intent.Catalog = expandHome(strings.TrimSpace(intent.Catalog))
	intent.LicenseKeyID = strings.TrimSpace(intent.LicenseKeyID)
	if intent.Catalog == "" {
		intent.Catalog = "docs/enterprise-license-skus.json"
	}
	if intent.SmokeModel == "" {
		intent.SmokeModel = "default"
	}
	if intent.TokenOut == "" {
		intent.TokenOut = filepath.Join(fleetCustomerHome(id), "CALLER_TOKEN.txt")
	} else {
		intent.TokenOut = expandHome(intent.TokenOut)
	}
	if intent.CustomerAlias == "" {
		intent.CustomerAlias = id
	}

	required := map[string]string{
		"sku":                intent.SKU,
		"customer_email":     intent.CustomerEmail,
		"customer_alias":     intent.CustomerAlias,
		"owner_user":         intent.OwnerUser,
		"project":            intent.Project,
		"profile_ref":        intent.ProfileRef,
		"license_ref":        intent.LicenseRef,
		"runtime_bundle_ref": intent.RuntimeBundleRef,
		"config_file":        intent.ConfigFile,
		"sign_key":           intent.SignKey,
		"license_key":        intent.LicenseKey,
		"license_key_id":     intent.LicenseKeyID,
		"commerce_base_url":  intent.CommerceBaseURL,
		"byok_env_file":      intent.BYOKEnvFile,
		"byok.provider":      intent.BYOK.Provider,
		"byok.api_key_env":   intent.BYOK.APIKeyEnv,
		"byok.model":         intent.BYOK.Model,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return Intent{}, fmt.Errorf("%s is required", name)
		}
	}
	if !strings.HasPrefix(intent.ProfileRef, "aws-ssm:///") {
		return Intent{}, fmt.Errorf("profile_ref must be aws-ssm:///")
	}
	if !strings.HasPrefix(intent.LicenseRef, "aws-ssm:///") {
		return Intent{}, fmt.Errorf("license_ref must be aws-ssm:///")
	}
	if !strings.HasPrefix(intent.RuntimeBundleRef, "aws-ssm:///") &&
		!strings.HasPrefix(intent.RuntimeBundleRef, "aws-secretsmanager:///") {
		return Intent{}, fmt.Errorf("runtime_bundle_ref must be aws-ssm:/// or aws-secretsmanager:///")
	}
	if err := ValidateBYOKFile(intent.BYOKEnvFile, intent.BYOK); err != nil {
		return Intent{}, err
	}
	for _, path := range []string{intent.ConfigFile, intent.SignKey, intent.LicenseKey, intent.BYOKEnvFile} {
		if err := requireMode0600(path, filepath.Base(path)); err != nil {
			return Intent{}, err
		}
	}
	return intent, nil
}

func fleetCustomerHome(customerID string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".metrum-fleet", customerID)
	}
	return filepath.Join(home, ".local", "share", "metrum-fleet", customerID)
}

func customerIDFromHostname(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	suffix := "." + hostnameSuffix
	if !strings.HasSuffix(host, suffix) {
		return "", fmt.Errorf("hostname must end with %s", suffix)
	}
	id := strings.TrimSuffix(host, suffix)
	if id == "" || strings.Contains(id, ".") {
		return "", fmt.Errorf("hostname must be {customer_id}.%s", hostnameSuffix)
	}
	return id, nil
}

// ExpectedHostname returns customer_id.apps.example.test.
func ExpectedHostname(customerID string) string {
	return strings.ToLower(strings.TrimSpace(customerID)) + "." + hostnameSuffix
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func requireMode0600(path, label string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s not found: %s", label, filepath.Base(path))
	}
	if st.IsDir() {
		return fmt.Errorf("%s must be a regular file", label)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must be mode 0600 (got %04o)", label, st.Mode().Perm())
	}
	return nil
}
