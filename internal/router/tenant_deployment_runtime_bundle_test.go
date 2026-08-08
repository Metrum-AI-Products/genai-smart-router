package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func protectedRuntimeBundle(t *testing.T, configYAML, envJSON string) []byte {
	t.Helper()
	value, err := json.Marshal(map[string]string{"config.yaml": configYAML, "env.json": envJSON})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestParseTenantDeploymentRuntimeBundleAcceptsExactConfigAndEnvironmentFiles(t *testing.T) {
	configYAML := "server:\n  listen: :8080\nproviders: {}\n"
	envJSON := `{"PROVIDER_API_KEY":"synthetic-test-value"}`
	bundle, err := parseTenantDeploymentRuntimeBundle(protectedRuntimeBundle(t, configYAML, envJSON))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ConfigYAML != configYAML || bundle.EnvJSON != envJSON {
		t.Fatal("bundle values changed while parsing")
	}
}

func TestParseTenantDeploymentRuntimeBundleRejectsMalformedOrExtraFieldsWithoutLeakingValues(t *testing.T) {
	const canary = "synthetic-runtime-secret-must-not-escape"
	cases := [][]byte{
		[]byte(`{"config.yaml":"server: {}","env.json":"{}","extra":"` + canary + `"}`),
		[]byte(`{"config.yaml":"server: {}","env.json":"[]"}`),
		[]byte(`{"config.yaml":"not-a-mapping","env.json":"{}"}`),
		[]byte(`{"config.yaml":"","env.json":"{}"}`),
		[]byte(`{"config.yaml":"server: {}","env.json":"{}"} trailing`),
	}
	for _, value := range cases {
		_, err := parseTenantDeploymentRuntimeBundle(value)
		if err != errInvalidTenantDeploymentRuntimeBundle {
			t.Fatalf("parse error = %v, want invalid protected runtime bundle", err)
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("parse error leaked protected bundle data: %v", err)
		}
	}
}
