package router

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLicenseEnvelopeVerificationAndTamperFailures(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	env, err := SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	keys := []LicensePublicKey{{KeyID: payload.KeyID, Algorithm: "ed25519", PublicKey: pub}}
	if err := VerifyLicenseEnvelope(env, keys, now); err != nil {
		t.Fatalf("valid license failed: %v", err)
	}
	env.Payload.CustomerID = "cust_tampered"
	if err := VerifyLicenseEnvelope(env, keys, now); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("tampered payload err=%v, want signature failure", err)
	}
	env.Payload.CustomerID = payload.CustomerID
	env.Signature.ValueBase64 = "not-base64"
	if err := VerifyLicenseEnvelope(env, keys, now); err == nil {
		t.Fatal("tampered signature verified")
	}
}

func TestLicenseValidationRejectsExpiredWrongProductAndUnknownKey(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	payload.ExpiresAt = now.Add(-time.Minute)
	env, err := SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	keys := []LicensePublicKey{{KeyID: payload.KeyID, Algorithm: "ed25519", PublicKey: pub}}
	if err := VerifyLicenseEnvelope(env, keys, now); err != nil {
		t.Fatalf("expired signature should verify before time validation: %v", err)
	}
	if err := validateLicensePayloadForConfig(env.Payload, nil, now); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired payload err=%v", err)
	}
	payload = testLicensePayload(now, []string{LicenseFeatureRouting})
	payload.Product = "wrong-product"
	env, err = SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyLicenseEnvelope(env, keys, now); err == nil {
		t.Fatal("wrong product verified")
	}
	payload = testLicensePayload(now, []string{LicenseFeatureRouting})
	env, err = SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyLicenseEnvelope(env, []LicensePublicKey{{KeyID: "other", Algorithm: "ed25519", PublicKey: pub}}, now); err == nil {
		t.Fatal("unknown key verified")
	}
}

func TestLicenseManagerReadinessRequestGateMetricsAndUsage(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "ok",
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "OK"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	now := time.Now().UTC()
	licensePath := writeTestLicense(t, dir, priv, testLicensePayload(now, []string{LicenseFeatureRouting, LicenseFeatureUsageReporting}))
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.UsageDB = UsageDBConfig{Driver: "sqlite", Path: filepath.Join(dir, "usage.sqlite")}
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.license.keys = []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}
	svc.license.now = func() time.Time { return now }
	svc.license.reload()

	ready := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("readyz=%d body=%s", ready.Code, ready.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+testToken)
	metricsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusForbidden {
		t.Fatalf("ordinary metrics status=%d", metricsRR.Code)
	}
	if strings.Contains(metricsRR.Body.String(), "license_id") {
		t.Fatalf("ordinary metrics leaked license detail: %s", metricsRR.Body.String())
	}
}

func TestLicenseVolumeWindowConcurrencyAndReplacement(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	payload.Limits.MaxTotalTokens = 10
	payload.Limits.MaxTotalRequests = 2
	payload.Limits.WindowTokens = 5
	payload.Limits.WindowRequests = 1
	payload.Limits.WindowDurationSeconds = 60
	payload.Limits.MaxConcurrent = 1
	licensePath := writeTestLicense(t, dir, priv, payload)
	cfg := licenseTestConfig(dir)
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	m, err := newLicenseManager(cfg.Server.License, cfg, []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}})
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	m.reload()

	if lerr := m.AdmitRequest("openai-chat"); lerr != nil {
		t.Fatalf("first request rejected: %v", lerr)
	}
	if lerr := m.AdmitRequest("openai-chat"); lerr == nil || lerr.Code != "license-concurrency-exceeded" {
		t.Fatalf("concurrency err=%v, want license-concurrency-exceeded", lerr)
	}
	res, lerr := m.ReserveTokens(4)
	if lerr != nil {
		t.Fatalf("token reservation rejected: %v", lerr)
	}
	if _, lerr := m.ReserveTokens(2); lerr == nil || lerr.Code != "license-window-exceeded" {
		t.Fatalf("window token err=%v, want license-window-exceeded", lerr)
	}
	m.RecordTokens(res, Usage{TotalTokens: 4})
	m.ReleaseRequest()

	if lerr := m.AdmitRequest("openai-chat"); lerr == nil || lerr.Code != "license-window-exceeded" {
		t.Fatalf("window request err=%v, want license-window-exceeded", lerr)
	}
	m.now = func() time.Time { return now.Add(61 * time.Second) }
	if lerr := m.AdmitRequest("openai-chat"); lerr != nil {
		t.Fatalf("second window request rejected: %v", lerr)
	}
	if _, lerr := m.ReserveTokens(7); lerr == nil || lerr.Code != "license-volume-exceeded" {
		t.Fatalf("lifetime token err=%v, want license-volume-exceeded", lerr)
	}
	m.ReleaseRequest()

	replacement := payload
	replacement.LicenseID = "lic_test_002"
	replacement.Limits.MaxTotalTokens = 5
	licensePath = writeTestLicense(t, dir, priv, replacement)
	m.cfg.Path = licensePath
	m.reload()
	state, err := m.readState()
	if err != nil {
		t.Fatal(err)
	}
	if state.LastValidLicenseID != "lic_test_002" || state.LifetimeTokens != 0 || state.LifetimeRequests != 0 || state.Window.WindowTokens != 0 || state.Window.WindowRequests != 0 {
		t.Fatalf("replacement did not reset license-wide counters: %#v", state)
	}
}

func TestLicenseAllowedSkinsBlocksDialect(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	payload.Limits.AllowedSkins = []string{" OpenAI-Chat "}
	licensePath := writeTestLicense(t, dir, priv, payload)
	cfg := licenseTestConfig(dir)
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	m, err := newLicenseManager(cfg.Server.License, cfg, []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}})
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	m.reload()

	if lerr := m.AdmitRequest("openai-chat"); lerr != nil {
		t.Fatalf("openai-chat rejected: %v", lerr)
	}
	m.ReleaseRequest()
	if lerr := m.AdmitRequest("anthropic"); lerr == nil || lerr.Code != "license-skin-forbidden" {
		t.Fatalf("anthropic err=%v, want license-skin-forbidden", lerr)
	}
}

func TestLicenseInstanceBindingUsesRuntimeFingerprint(t *testing.T) {
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	payload.Deployment.InstanceFingerprintRequired = true
	payload.Deployment.AllowedInstances = []string{"host-a"}
	payload.Limits.MaxInstances = 1

	cfg := licenseTestConfig(t.TempDir())
	cfg.Server.License.InstanceFingerprint = "host-a"
	if err := validateLicensePayloadForConfig(payload, cfg, now); err != nil {
		t.Fatalf("matching runtime fingerprint rejected: %v", err)
	}

	cfg.Server.License.InstanceFingerprint = "host-b"
	if err := validateLicensePayloadForConfig(payload, cfg, now); err == nil || !strings.Contains(err.Error(), "instance fingerprint") {
		t.Fatalf("mismatched runtime fingerprint err=%v", err)
	}

	cfg.Server.License.InstanceFingerprint = ""
	if err := validateLicensePayloadForConfig(payload, cfg, now); err == nil || !strings.Contains(err.Error(), "instance fingerprint is required") {
		t.Fatalf("missing runtime fingerprint err=%v", err)
	}

	payload.Deployment.InstanceFingerprint = "host-a"
	payload.Deployment.AllowedInstances = nil
	cfg.Server.License.InstanceFingerprint = "host-a"
	if err := validateLicensePayloadForConfig(payload, cfg, now); err != nil {
		t.Fatalf("legacy single fingerprint binding rejected: %v", err)
	}
	cfg.Server.License.InstanceFingerprint = "host-b"
	if err := validateLicensePayloadForConfig(payload, cfg, now); err == nil || !strings.Contains(err.Error(), "instance fingerprint") {
		t.Fatalf("legacy single fingerprint mismatch err=%v", err)
	}
}

func TestLicenseConfigFeatureAndOperationalGates(t *testing.T) {
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})

	piiCfg := licenseTestConfig(t.TempDir())
	piiCfg.Models["default"] = ModelGroup{Strategy: "static", PIIFilter: testPIIFilterConfig("redact_only"), Targets: []Target{{Provider: "mock", Model: "mock-model"}}}
	if err := validateLicensePayloadForConfig(payload, piiCfg, now); err == nil || !strings.Contains(err.Error(), "pii") {
		t.Fatalf("pii gate err=%v", err)
	}

	privateCfg := licenseTestConfig(t.TempDir())
	privateCfg.Provider["mock"] = ProviderConfig{BaseURL: "http://10.0.0.5:8000/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	privateLimited := payload
	privateLimited.Features = []string{LicenseFeatureRouting}
	if err := validateLicensePayloadForConfig(privateLimited, privateCfg, now); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("private upstream gate err=%v", err)
	}

	scriptHTTPCfg := licenseTestConfig(t.TempDir())
	scriptHTTPCfg.Models["default"] = ModelGroup{
		Strategy: "script",
		Script:   filepath.Join(t.TempDir(), "policy.js"),
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:    true,
			AllowHosts: []string{"policy.example.com"},
		},
		Targets: []Target{{Provider: "mock", Model: "mock-model"}},
	}
	if err := validateLicensePayloadForConfig(payload, scriptHTTPCfg, now); err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("script http gate err=%v", err)
	}

	adminCfg := licenseTestConfig(t.TempDir())
	adminCfg.Server.AdminAuth.Basic.Enabled = true
	adminCfg.Server.AdminAuth.Basic.Users = []AdminBasicAuthUser{{Username: "admin1"}, {Username: "admin2"}}
	adminLimited := payload
	adminLimited.Limits.MaxAdmins = 1
	if err := validateLicensePayloadForConfig(adminLimited, adminCfg, now); err == nil {
		t.Fatal("max_admins gate did not reject")
	}

	retentionCfg := licenseTestConfig(t.TempDir())
	retentionCfg.Server.Retention.Enabled = true
	retentionCfg.Server.Retention.Classes = []RetentionClassConfig{{DataClass: "usage_detail", RetentionDays: 60}}
	retentionLimited := payload
	retentionLimited.Limits.MaxRetentionDays = 30
	if err := validateLicensePayloadForConfig(retentionLimited, retentionCfg, now); err == nil {
		t.Fatal("max_retention_days gate did not reject")
	}

	disabled := false
	retentionCfg.Server.Retention.Classes = []RetentionClassConfig{{DataClass: "usage_detail", Enabled: &disabled, RetentionDays: 365}}
	if err := validateLicensePayloadForConfig(retentionLimited, retentionCfg, now); err != nil {
		t.Fatalf("disabled retention class should not count against cap: %v", err)
	}
}

func TestLicenseSchemaV1IgnoresUnknownFields(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	env, err := SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalLicenseEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"schema_version":`, `"future_field":"ignored","schema_version":`, 1))
	parsed, err := ParseLicenseEnvelope(raw)
	if err != nil {
		t.Fatalf("unknown field rejected: %v", err)
	}
	if err := VerifyLicenseEnvelope(parsed, []LicensePublicKey{{KeyID: payload.KeyID, Algorithm: "ed25519", PublicKey: pub}}, now); err != nil {
		t.Fatalf("license with ignored unknown field did not verify: %v", err)
	}
}

func TestLicenseFeatureGateBlocksDynamicScore(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer upstream.Close()
	dir := t.TempDir()
	now := time.Now().UTC()
	licensePath := writeTestLicense(t, dir, priv, testLicensePayload(now, []string{LicenseFeatureRouting}))
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	cfg.Models["default"] = ModelGroup{Strategy: "dynamic_score", RoutingPolicy: RoutingPolicyConfig{DynamicScore: DynamicScoreConfig{}}, Targets: []Target{{Provider: "mock", Model: "mock-model"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.license.keys = []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}
	svc.license.now = func() time.Time { return now }
	svc.license.reload()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "license-feature-forbidden") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLicenseGracePreservesLastValidFeatures(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	now := time.Now().UTC()
	licensePath := writeTestLicense(t, dir, priv, testLicensePayload(now, []string{LicenseFeatureRouting, LicenseFeatureDynamicScore}))
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Server.License = LicenseConfig{
		Enabled:                      true,
		Path:                         licensePath,
		StatePath:                    filepath.Join(dir, "license-state.json"),
		RecheckInterval:              time.Hour,
		GracePeriodOnValidationError: time.Hour,
	}
	m, err := newLicenseManager(cfg.Server.License, cfg, []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}})
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	m.reload()
	if lerr := m.enforce(LicenseFeatureDynamicScore); lerr != nil {
		t.Fatalf("valid dynamic feature denied: %v", lerr)
	}
	if err := os.WriteFile(licensePath, []byte(`{"payload":`), 0o600); err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now.Add(10 * time.Minute) }
	m.reload()
	if st := m.statusSnapshot(); !st.GraceActive || !st.Features[LicenseFeatureDynamicScore] {
		t.Fatalf("grace status did not preserve dynamic feature: %#v", st)
	}
	if lerr := m.enforce(LicenseFeatureDynamicScore); lerr != nil {
		t.Fatalf("dynamic feature denied during grace: %v", lerr)
	}
}

func TestLicenseMetricsFeatureAfterMetricsAuthorization(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	now := time.Now().UTC()
	licensePath := writeTestLicense(t, dir, priv, testLicensePayload(now, []string{LicenseFeatureRouting}))
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	adminToken := "rtr_license_metrics_admin"
	adminSum := sha256.Sum256([]byte(adminToken))
	cfg.Callers = append(cfg.Callers, CallerConfig{
		ID:           "metrics-admin",
		User:         "ops",
		Project:      "observability",
		Environment:  "test",
		TokenSHA256:  hex.EncodeToString(adminSum[:]),
		TokenID:      "rtr_license_metrics_admin",
		Allow:        []string{"default"},
		MetricsAdmin: true,
		Rate:         RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
	})
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.license.keys = []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}
	svc.license.now = func() time.Time { return now }
	svc.license.reload()

	ordinary := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	ordinary.Header.Set("Authorization", "Bearer "+testToken)
	ordinaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ordinaryRR, ordinary)
	if ordinaryRR.Code != http.StatusForbidden || !strings.Contains(ordinaryRR.Body.String(), "metrics-forbidden") {
		t.Fatalf("ordinary metrics status=%d body=%s", ordinaryRR.Code, ordinaryRR.Body.String())
	}

	admin := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	admin.Header.Set("Authorization", "Bearer "+adminToken)
	adminRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(adminRR, admin)
	if adminRR.Code != http.StatusForbidden || !strings.Contains(adminRR.Body.String(), "license-feature-forbidden") {
		t.Fatalf("admin metrics status=%d body=%s", adminRR.Code, adminRR.Body.String())
	}
}

func TestContractedDynamicScoreRequiresBothLicenseFeatures(t *testing.T) {
	pub, priv, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer upstream.Close()
	dir := t.TempDir()
	now := time.Now().UTC()
	licensePath := writeTestLicense(t, dir, priv, testLicensePayload(now, []string{LicenseFeatureRouting, LicenseFeatureModelGroupContracts}))
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.License = LicenseConfig{Enabled: true, Path: licensePath, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour}
	cfg.Models["default"] = ModelGroup{
		Strategy:      "dynamic_score",
		RoutingPolicy: RoutingPolicyConfig{DynamicScore: DynamicScoreConfig{}},
		Contract:      &ModelGroupContract{DisplayName: "licensed-contract"},
		Targets:       []Target{{Provider: "mock", Model: "mock-model"}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.license.keys = []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}
	svc.license.now = func() time.Time { return now }
	svc.license.reload()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "license-feature-forbidden") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func testLicensePayload(now time.Time, features []string) LicensePayload {
	features = append([]string(nil), features...)
	if !stringSliceContains(features, LicenseFeaturePrivateUpstreams) {
		features = append(features, LicenseFeaturePrivateUpstreams)
	}
	return LicensePayload{
		SchemaVersion: 1,
		LicenseID:     "lic_test_001",
		CustomerID:    "cust_test",
		CustomerName:  "Test Customer",
		Product:       licenseProduct,
		SKU:           "enterprise-test",
		Features:      features,
		Limits:        LicenseLimits{MaxModelGroups: 100, MaxCallers: 100},
		IssuedAt:      now.Add(-time.Hour),
		NotBefore:     now.Add(-time.Hour),
		ExpiresAt:     now.Add(24 * time.Hour),
		KeyID:         "test-license-key",
		Issuer:        licenseIssuer,
	}
}

func licenseTestConfig(dir string) *Config {
	sum := sha256.Sum256([]byte(testToken))
	return &Config{
		Server: ServerConfig{
			Listen:            ":0",
			DefaultModelGroup: "default",
			License:           LicenseConfig{Enabled: true, StatePath: filepath.Join(dir, "license-state.json"), RecheckInterval: time.Hour},
		},
		StatePath: filepath.Join(dir, "state.json"),
		Provider: map[string]ProviderConfig{
			"mock": {BaseURL: "https://api.example.com/v1", Dialect: "openai-chat", APIKey: "provider-key"},
		},
		Models: map[string]ModelGroup{
			"default": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			User:        "alice",
			Project:     "metrum-insights",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(sum[:]),
			TokenID:     "rtr_alice_test",
			Allow:       []string{"default"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		}},
	}
}

func writeTestLicense(t *testing.T, dir string, priv ed25519.PrivateKey, payload LicensePayload) string {
	t.Helper()
	env, err := SignLicensePayload(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalLicenseEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "license.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCanonicalLicensePayloadStableAcrossObjectOrder(t *testing.T) {
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting, LicenseFeatureUsageReporting})
	a, err := CanonicalLicensePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"sku":            payload.SKU,
		"schema_version": payload.SchemaVersion,
		"license_id":     payload.LicenseID,
		"customer_id":    payload.CustomerID,
		"customer_name":  payload.CustomerName,
		"product":        payload.Product,
		"features":       payload.Features,
		"limits":         payload.Limits,
		"deployment":     payload.Deployment,
		"issued_at":      payload.IssuedAt,
		"not_before":     payload.NotBefore,
		"expires_at":     payload.ExpiresAt,
		"key_id":         payload.KeyID,
		"issuer":         payload.Issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded LicensePayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalLicensePayload(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical mismatch:\n%s\n%s", a, b)
	}
}
