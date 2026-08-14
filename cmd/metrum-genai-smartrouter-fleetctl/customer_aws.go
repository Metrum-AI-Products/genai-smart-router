package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"gopkg.in/yaml.v3"
)

const (
	defaultFleetLifecycleRoleARN = "arn:aws:iam::121701826775:role/genai-smart-router-eks-fleet-lifecycle"
	secretsManagerPrefix         = "aws-secretsmanager:///"
)

type runtimeBundle struct {
	ConfigYAML string
	EnvJSON    string
}

func fleetLifecycleRoleARN() string {
	if v := strings.TrimSpace(os.Getenv("METRUM_FLEET_LIFECYCLE_ROLE_ARN")); v != "" {
		return v
	}
	return defaultFleetLifecycleRoleARN
}

func awsRegion() string {
	if v := strings.TrimSpace(os.Getenv("AWS_REGION")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); v != "" {
		return v
	}
	return "us-east-1"
}

func operatorAWSConfig(ctx context.Context) (aws.Config, error) {
	region := awsRegion()
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

func assumeFleetRole(ctx context.Context, ws customerWorkspace) (aws.Config, map[string]string, error) {
	opCfg, err := operatorAWSConfig(ctx)
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("load operator AWS config: %w", err)
	}
	client := sts.NewFromConfig(opCfg)
	roleARN := fleetLifecycleRoleARN()
	sessionName := "metrum-fleetctl-" + ws.CustomerID
	out, err := client.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int32(3600),
	})
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("sts assume-role: %w", err)
	}
	if out.Credentials == nil || out.Credentials.AccessKeyId == nil || out.Credentials.SecretAccessKey == nil || out.Credentials.SessionToken == nil {
		return aws.Config{}, nil, fmt.Errorf("sts assume-role returned empty credentials")
	}
	region := awsRegion()
	sessionPath := filepath.Join(ws.Home, "aws-session.env")
	sessionBody := strings.Join([]string{
		"export AWS_ACCESS_KEY_ID=" + *out.Credentials.AccessKeyId,
		"export AWS_SECRET_ACCESS_KEY=" + *out.Credentials.SecretAccessKey,
		"export AWS_SESSION_TOKEN=" + *out.Credentials.SessionToken,
		"export AWS_REGION=" + region,
		"unset AWS_PROFILE",
		"",
	}, "\n")
	writeMode0600(sessionPath, []byte(sessionBody))

	env := cloneEnvMap(os.Environ())
	env["AWS_ACCESS_KEY_ID"] = *out.Credentials.AccessKeyId
	env["AWS_SECRET_ACCESS_KEY"] = *out.Credentials.SecretAccessKey
	env["AWS_SESSION_TOKEN"] = *out.Credentials.SessionToken
	env["AWS_REGION"] = region
	delete(env, "AWS_PROFILE")

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			*out.Credentials.AccessKeyId,
			*out.Credentials.SecretAccessKey,
			*out.Credentials.SessionToken,
		)),
	)
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("load assumed-role AWS config: %w", err)
	}
	return cfg, env, nil
}

func cloneEnvMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}

func envMapToSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func smNameFromRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, secretsManagerPrefix) {
		return "", fmt.Errorf("runtime bundle ref must be %s...", secretsManagerPrefix)
	}
	name := strings.TrimPrefix(ref, secretsManagerPrefix)
	if name == "" {
		return "", fmt.Errorf("runtime bundle ref must be %s...", secretsManagerPrefix)
	}
	return name, nil
}

func fetchRuntimeBundle(ctx context.Context, cfg aws.Config, ref string) (runtimeBundle, error) {
	name, err := smNameFromRef(ref)
	if err != nil {
		return runtimeBundle{}, err
	}
	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		return runtimeBundle{}, fmt.Errorf("get-secret-value %s: %w", name, err)
	}
	if out.SecretString == nil {
		return runtimeBundle{}, fmt.Errorf("get-secret-value %s: empty secret string", name)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(*out.SecretString), &payload); err != nil {
		return runtimeBundle{}, fmt.Errorf("invalid runtime bundle JSON: %w", err)
	}
	cfgYAML, okCfg := payload["config.yaml"]
	envJSON, okEnv := payload["env.json"]
	if !okCfg || !okEnv || len(payload) != 2 {
		return runtimeBundle{}, fmt.Errorf("runtime bundle must contain exactly config.yaml and env.json")
	}
	return runtimeBundle{ConfigYAML: cfgYAML, EnvJSON: envJSON}, nil
}

func publishRuntimeBundle(ctx context.Context, customerID string, bundle runtimeBundle) (string, error) {
	if err := validateBundleShapes(bundle); err != nil {
		return "", err
	}
	ref := customerBundleRef(customerID)
	name, err := smNameFromRef(ref)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{
		"config.yaml": bundle.ConfigYAML,
		"env.json":    bundle.EnvJSON,
	})
	if err != nil {
		return "", err
	}
	opCfg, err := operatorAWSConfig(ctx)
	if err != nil {
		return "", err
	}
	client := secretsmanager.NewFromConfig(opCfg)
	_, descErr := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if descErr != nil {
		_, createErr := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         aws.String(name),
			SecretString: aws.String(string(payload)),
		})
		if createErr != nil {
			return "", fmt.Errorf("CreateSecret denied for per-customer runtime bundle; operator IAM must allow secretsmanager:CreateSecret/PutSecretValue on %s (Fleet lifecycle role is read-only for deploy): %w", name, createErr)
		}
	} else {
		_, putErr := client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
			SecretId:     aws.String(name),
			SecretString: aws.String(string(payload)),
		})
		if putErr != nil {
			return "", fmt.Errorf("PutSecretValue failed for %s: %w", name, putErr)
		}
	}
	sum := sha256.Sum256([]byte(bundle.ConfigYAML))
	fmt.Println(mustJSON(map[string]any{
		"published_runtime_bundle_ref": ref,
		"config_sha256":                hex.EncodeToString(sum[:])[:16],
	}))
	return ref, nil
}

func validateBundleShapes(bundle runtimeBundle) error {
	var cfg any
	if err := yaml.Unmarshal([]byte(bundle.ConfigYAML), &cfg); err != nil {
		return fmt.Errorf("config.yaml invalid: %w", err)
	}
	var env any
	if err := json.Unmarshal([]byte(bundle.EnvJSON), &env); err != nil {
		return fmt.Errorf("env.json invalid: %w", err)
	}
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		die("encode json: %v", err)
	}
	return string(b)
}
