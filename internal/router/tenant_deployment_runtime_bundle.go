package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
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
