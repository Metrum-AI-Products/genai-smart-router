// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package customerlifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

const ssmPrefix = "aws-ssm:///"

// PublishLicenseSSM writes a signed license.json envelope to the intent license_ref.
// Fleet Resolve reads this parameter into the router-license Secret as license.json.
//
// PutParameter is an operator-IAM action. Clear any inherited Fleet lifecycle STS
// session (AWS_ACCESS_KEY_ID/SECRET/SESSION_TOKEN) so LoadDefaultConfig uses the
// operator profile/SSO chain, not genai-smart-router-eks-fleet-lifecycle.
func PublishLicenseSSM(ctx context.Context, licenseRef string, licenseJSON []byte) (string, error) {
	name, err := ssmNameFromRef(licenseRef)
	if err != nil {
		return "", err
	}
	if len(bytesTrimSpace(licenseJSON)) == 0 {
		return "", fmt.Errorf("license payload is empty")
	}
	clearInheritedFleetSTSCredentials()
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("load AWS config for license publish: %w", err)
	}
	client := ssm.NewFromConfig(cfg)
	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Type:      types.ParameterTypeSecureString,
		Value:     aws.String(string(licenseJSON)),
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("ssm PutParameter for license_ref failed (operator IAM must allow ssm:PutParameter on %s; do not use the Fleet lifecycle role): %w", name, err)
	}
	return licenseRef, nil
}

func clearInheritedFleetSTSCredentials() {
	// Fleet customer verbs may leave temporary STS keys in the process env.
	// Operator-owned writes (license SSM publish) must not inherit them.
	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		_ = os.Unsetenv(key)
	}
}

func ssmNameFromRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, ssmPrefix) {
		return "", fmt.Errorf("license_ref must be %s...", ssmPrefix)
	}
	name := strings.TrimPrefix(ref, ssmPrefix)
	if name == "" {
		return "", fmt.Errorf("license_ref must be %s...", ssmPrefix)
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return name, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// IssueLicenseFile shells out to metrum-genai-smartrouter-license issue and returns the license path.
func IssueLicenseFile(ctx context.Context, intent Intent, workDir string) (string, error) {
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return "", err
	}
	entitlementPath := filepath.Join(workDir, "entitlement.json")
	licensePath := filepath.Join(workDir, "license.json")
	summaryPath := filepath.Join(workDir, "license-summary.json")
	entitlement := map[string]any{
		"customer_id":   intent.CustomerID,
		"customer_name": intent.CustomerAlias,
		"sku":           intent.SKU,
		"signing": map[string]string{
			"key_id": intent.LicenseKeyID,
		},
		"notes": "issued by metrum-genai-customer-lifecycle after commerce entitlement",
	}
	if intent.LicenseKeyID == "" {
		return "", fmt.Errorf("license_key_id is required for license issue")
	}
	raw, err := jsonMarshalIndent(entitlement)
	if err != nil {
		return "", err
	}
	if err := writeMode0600(entitlementPath, raw); err != nil {
		return "", err
	}
	bin, err := resolveBinary("metrum-genai-smartrouter-license", "router-license")
	if err != nil {
		return "", err
	}
	args := []string{
		"issue",
		"--catalog", intent.Catalog,
		"--entitlement", entitlementPath,
		"--key", intent.LicenseKey,
		"--out", licensePath,
		"--summary-out", summaryPath,
	}
	if intent.LicenseAllowUnknownRuntimeKey {
		args = append(args, "--allow-unknown-runtime-key")
	}
	if err := runQuiet(ctx, bin, args...); err != nil {
		return "", fmt.Errorf("license issue failed: %w", err)
	}
	if err := requireMode0600(licensePath, "license.json"); err != nil {
		return "", err
	}
	return licensePath, nil
}
