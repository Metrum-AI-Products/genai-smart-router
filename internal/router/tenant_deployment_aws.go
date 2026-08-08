package router

// This file contains the only AWS integration used by the customer lifecycle.
// It deliberately uses SDK clients: the packaged operator never shells out to
// aws, kubectl, helm, Terraform, or Make.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func tenantDeploymentAWSConfig(ctx context.Context, profile TenantDeploymentProfile) (aws.Config, error) {
	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(profile.Region)}
	if profile.AWSProfile != "" {
		options = append(options, awsconfig.WithSharedConfigProfile(profile.AWSProfile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS operator identity: %w", err)
	}
	return cfg, nil
}

func loadTenantDeploymentProfileSSM(ref string) (TenantDeploymentProfile, error) {
	// The profile itself is protected policy. Do not include either the
	// parameter path or any decoded text in errors or CLI output.
	path := strings.TrimPrefix(ref, "aws-ssm:///")
	if path == ref || path == "" {
		return TenantDeploymentProfile{}, errors.New("SSM profile reference is invalid")
	}
	// Region and optional shared profile for the policy resolver come from the
	// standard AWS SDK environment/config chain. The decoded policy chooses the
	// target region for all subsequent calls.
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return TenantDeploymentProfile{}, errors.New("load protected profile resolver")
	}
	result, err := ssm.NewFromConfig(cfg).GetParameter(context.Background(), &ssm.GetParameterInput{Name: &path, WithDecryption: aws.Bool(true)})
	if err != nil || result.Parameter == nil || result.Parameter.Value == nil {
		return TenantDeploymentProfile{}, errors.New("read protected deployment profile")
	}
	data := []byte(*result.Parameter.Value)
	var profile TenantDeploymentProfile
	if err := decodeStrictDeploymentDocument(data, "profile.yaml", &profile); err != nil {
		return TenantDeploymentProfile{}, errors.New("deployment profile schema is invalid")
	}
	if err := validateTenantDeploymentProfile(profile, data); err != nil {
		return TenantDeploymentProfile{}, err
	}
	return profile, nil
}
