package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	tenantDeploymentSQLiteUsageDBPath = "/var/lib/smart-llmrouter/usage.sqlite"
	tenantDeploymentUsageDSNEnvKey    = "ROUTER_USAGE_DB_DSN"
)

var errInvalidTenantDeploymentRuntimeBundle = errors.New("invalid protected runtime bundle")

// tenantDeploymentRuntimeBundle is the only accepted resolved runtime-secret
// shape. Its values remain in memory until they are written into the owned
// Kubernetes Secret; callers must never persist or return this structure.
type tenantDeploymentRuntimeBundle struct {
	ConfigYAML string `json:"config.yaml"`
	EnvJSON    string `json:"env.json"`
}

func parseTenantDeploymentRuntimeBundle(value []byte) (tenantDeploymentRuntimeBundle, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var bundle tenantDeploymentRuntimeBundle
	if err := decoder.Decode(&bundle); err != nil {
		return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
	}
	if strings.TrimSpace(bundle.ConfigYAML) == "" || strings.TrimSpace(bundle.EnvJSON) == "" {
		return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
	}

	var config yaml.Node
	if err := yaml.Unmarshal([]byte(bundle.ConfigYAML), &config); err != nil || len(config.Content) != 1 || config.Content[0].Kind != yaml.MappingNode {
		return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
	}
	var environment map[string]string
	if err := json.Unmarshal([]byte(bundle.EnvJSON), &environment); err != nil || environment == nil {
		return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
	}
	for name := range environment {
		if strings.TrimSpace(name) == "" {
			return tenantDeploymentRuntimeBundle{}, errInvalidTenantDeploymentRuntimeBundle
		}
	}
	return bundle, nil
}

func injectRuntimeEnvJSONValue(envJSON, key, value string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || value == "" {
		return "", errors.New("runtime env injection requires a key and value")
	}
	var environment map[string]string
	if err := json.Unmarshal([]byte(envJSON), &environment); err != nil || environment == nil {
		return "", errInvalidTenantDeploymentRuntimeBundle
	}
	environment[key] = value
	encoded, err := json.Marshal(environment)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func stripRuntimeEnvJSONKey(envJSON, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("runtime env strip requires a key")
	}
	var environment map[string]string
	if err := json.Unmarshal([]byte(envJSON), &environment); err != nil || environment == nil {
		return "", errInvalidTenantDeploymentRuntimeBundle
	}
	delete(environment, key)
	encoded, err := json.Marshal(environment)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// applySQLiteUsageDBConfig rewrites server.usage_db for the default Fleet path
// (no dedicated RDS). Production-identical bundles ship postgres +
// deployment-job; SQLite customers need sqlite + auto-safe on the PVC path so
// metrum-fleetctl deploy is repeatable without one-off migrate Jobs.
func applySQLiteUsageDBConfig(configYAML string) (string, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(configYAML), &root); err != nil || root == nil {
		return "", errInvalidTenantDeploymentRuntimeBundle
	}
	server, _ := root["server"].(map[string]any)
	if server == nil {
		server = map[string]any{}
		root["server"] = server
	}
	server["usage_db"] = map[string]any{
		"enabled":          true,
		"driver":           "sqlite",
		"path":             tenantDeploymentSQLiteUsageDBPath,
		"migration_policy": usageDBMigrationPolicyAutoSafe,
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
