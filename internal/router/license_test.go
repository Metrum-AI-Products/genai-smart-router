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
