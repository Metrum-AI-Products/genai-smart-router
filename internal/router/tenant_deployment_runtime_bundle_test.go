// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

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

func TestApplySQLiteUsageDBConfigRewritesPostgresDeploymentJobBundle(t *testing.T) {
	in := "server:\n  listen: :8080\n  usage_db:\n    enabled: true\n    driver: postgres\n    dsn: ${ROUTER_USAGE_DB_DSN}\n    migration_policy: deployment-job\nproviders: {}\n"
	out, err := applySQLiteUsageDBConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "postgres") || strings.Contains(out, "deployment-job") || strings.Contains(out, "ROUTER_USAGE_DB_DSN") {
		t.Fatalf("sqlite rewrite left postgres/deployment-job values: %s", out)
	}
	if !strings.Contains(out, "driver: sqlite") || !strings.Contains(out, "migration_policy: auto-safe") {
		t.Fatalf("sqlite rewrite missing expected usage_db: %s", out)
	}
}

func TestStripRuntimeEnvJSONKeyRemovesUsageDSN(t *testing.T) {
	in := `{"PROVIDER_API_KEY":"synthetic","ROUTER_USAGE_DB_DSN":"synthetic-dsn"}`
	out, err := stripRuntimeEnvJSONKey(in, tenantDeploymentUsageDSNEnvKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ROUTER_USAGE_DB_DSN") || strings.Contains(out, "synthetic-dsn") {
		t.Fatalf("strip left usage DSN: %s", out)
	}
	if !strings.Contains(out, "PROVIDER_API_KEY") {
		t.Fatalf("strip removed unrelated keys: %s", out)
	}
}
