package router

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	licenseProduct = "genai-smart-router"
	licenseIssuer  = "metrum-ai"

	LicenseFeatureRouting              = "routing"
	LicenseFeatureUsageReporting       = "usage_reporting"
	LicenseFeatureAdminReports         = "admin_reports"
	LicenseFeatureAdminSecurityReports = "admin_security_reports"
	LicenseFeatureDynamicScore         = "dynamic_score"
	LicenseFeatureExternalPolicy       = "external_policy"
	LicenseFeatureTypeScriptRouting    = "typescript_routing"
	LicenseFeatureModelGroupContracts  = "model_group_contracts"
	LicenseFeatureRetentionRollups     = "retention_rollups"
	LicenseFeatureContentCapture       = "content_capture"
)

type licenseEnvelope struct {
	Payload   LicensePayload   `json:"payload"`
	Signature LicenseSignature `json:"signature"`
}

type LicensePayload struct {
	SchemaVersion int               `json:"schema_version"`
	LicenseID     string            `json:"license_id"`
	CustomerID    string            `json:"customer_id"`
	CustomerName  string            `json:"customer_name,omitempty"`
	Product       string            `json:"product"`
	SKU           string            `json:"sku"`
	Features      []string          `json:"features"`
	Limits        LicenseLimits     `json:"limits,omitempty"`
	Deployment    LicenseDeployment `json:"deployment,omitempty"`
	IssuedAt      time.Time         `json:"issued_at"`
	NotBefore     time.Time         `json:"not_before"`
	ExpiresAt     time.Time         `json:"expires_at"`
	GraceUntil    time.Time         `json:"grace_until,omitempty"`
	KeyID         string            `json:"key_id"`
	Issuer        string            `json:"issuer"`
	Notes         string            `json:"notes,omitempty"`
}

type LicenseLimits struct {
	MaxModelGroups     int `json:"max_model_groups,omitempty"`
	MaxCallers         int `json:"max_callers,omitempty"`
	MaxMonthlyRequests int `json:"max_monthly_requests,omitempty"`
}

type LicenseDeployment struct {
	Mode                        string   `json:"mode,omitempty"`
	AllowedEnvironments         []string `json:"allowed_environments,omitempty"`
	InstanceFingerprintRequired bool     `json:"instance_fingerprint_required,omitempty"`
	InstanceFingerprint         string   `json:"instance_fingerprint,omitempty"`
}

type LicenseSignature struct {
	Algorithm   string `json:"algorithm"`
	KeyID       string `json:"key_id"`
	ValueBase64 string `json:"value_base64"`
}

type LicensePublicKey struct {
	KeyID     string
	Algorithm string
	PublicKey ed25519.PublicKey
	NotBefore time.Time
	NotAfter  *time.Time
}

type licenseStatus struct {
	Enabled          bool
	Valid            bool
	Ready            bool
	GraceActive      bool
	Code             string
	Message          string
	LicenseID        string
	CustomerID       string
	SKU              string
	KeyID            string
	ExpiresAt        time.Time
	NotBefore        time.Time
	Features         map[string]bool
	DaysUntilExpiry  int
	LastCheckedAt    time.Time
	ValidationReason string
}

type licenseValidationError struct {
	Code       string
	StatusCode int
	Message    string
}

func (e licenseValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type licenseManager struct {
	cfg       LicenseConfig
	keys      []LicensePublicKey
	app       *Config
	statePath string
	now       func() time.Time

	mu              sync.RWMutex
	status          licenseStatus
	stop            chan struct{}
	stopped         chan struct{}
	failureByReason map[string]int64
}

type licensePersistentState struct {
	LastValidLicenseID          string   `json:"last_valid_license_id"`
	LastValidCustomerID         string   `json:"last_valid_customer_id"`
	LastValidSKU                string   `json:"last_valid_sku"`
	LastValidKeyID              string   `json:"last_valid_key_id"`
	LastValidExpiresAt          string   `json:"last_valid_expires_at"`
	LastValidFeatures           []string `json:"last_valid_features"`
	LastSuccessfulValidationUTC string   `json:"last_successful_validation_utc"`
	LastObservedWallClockUTC    string   `json:"last_observed_wall_clock_utc"`
	GraceActive                 bool     `json:"grace_active"`
}

func newLicenseManager(cfg LicenseConfig, app *Config, keys []LicensePublicKey) (*licenseManager, error) {
	if !cfg.Enabled {
		m := &licenseManager{
			cfg: cfg, app: app, keys: keys, now: time.Now, stop: make(chan struct{}), stopped: make(chan struct{}),
			status:          licenseStatus{Enabled: false, Valid: true, Ready: true, Code: "license-disabled", Features: map[string]bool{}},
			failureByReason: map[string]int64{},
		}
		close(m.stopped)
		return m, nil
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("license enabled but no public verification keys are embedded")
	}
	path := strings.TrimSpace(cfg.StatePath)
	if path == "" && app != nil {
		base := strings.TrimSpace(app.StatePath)
		if base == "" {
			base = "state.json"
		}
		path = strings.TrimSuffix(base, filepath.Ext(base)) + ".license.json"
	}
	m := &licenseManager{
		cfg: cfg, app: app, keys: keys, statePath: path, now: time.Now,
		stop: make(chan struct{}), stopped: make(chan struct{}), failureByReason: map[string]int64{},
	}
	m.reload()
	return m, nil
}

func defaultLicensePublicKeys() []LicensePublicKey {
	// Public verification key only. The matching private signing key is not shipped.
	raw, _ := hex.DecodeString("5f3558f10fd6058fd56c7d03d4ed157fba7c0d87be1717b46fe8fc0b375b1e1f")
	return []LicensePublicKey{{
		KeyID:     "metrum-license-ed25519-2026-01",
		Algorithm: "ed25519",
		PublicKey: ed25519.PublicKey(raw),
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}}
}

func (m *licenseManager) start() {
	if m == nil || !m.cfg.Enabled || m.cfg.RecheckInterval <= 0 {
		return
	}
	go func() {
		defer close(m.stopped)
		ticker := time.NewTicker(m.cfg.RecheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.reload()
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *licenseManager) close() {
	if m == nil {
		return
	}
	select {
	case <-m.stopped:
		return
	default:
	}
	close(m.stop)
	select {
	case <-m.stopped:
	case <-time.After(2 * time.Second):
	}
}

func (m *licenseManager) reload() {
	status, err := m.validateFile()
	if err != nil {
		var lerr licenseValidationError
		if errors.As(err, &lerr) {
			status = m.statusForError(lerr)
		} else {
			status = m.statusForError(licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license validation failed"})
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !status.Valid {
		m.failureByReason[status.Code]++
	}
	m.status = status
	_ = m.writeObservedStateLocked(status)
}

func (m *licenseManager) validateFile() (licenseStatus, error) {
	now := m.now().UTC()
	if m.cfg.FailOpenForDev {
		return licenseStatus{Enabled: true, Valid: true, Ready: true, Code: "license-dev-fail-open", LastCheckedAt: now, Features: allLicenseFeatures()}, nil
	}
	if strings.TrimSpace(m.cfg.Path) == "" {
		return licenseStatus{}, licenseValidationError{Code: "license-missing", StatusCode: 503, Message: "license file path is required"}
	}
	if err := m.checkClock(now); err != nil {
		return licenseStatus{}, err
	}
	raw, err := os.ReadFile(strings.TrimSpace(m.cfg.Path))
	if err != nil {
		return licenseStatus{}, licenseValidationError{Code: "license-missing", StatusCode: 503, Message: "license file is missing"}
	}
	env, err := ParseLicenseEnvelope(raw)
	if err != nil {
		return licenseStatus{}, licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license file is malformed"}
	}
	if err := VerifyLicenseEnvelope(env, m.keys, now); err != nil {
		return licenseStatus{}, err
	}
	if err := validateLicensePayloadForConfig(env.Payload, m.app, now); err != nil {
		return licenseStatus{}, err
	}
	features := featureMap(env.Payload.Features)
	days := int(env.Payload.ExpiresAt.Sub(now).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return licenseStatus{
		Enabled: true, Valid: true, Ready: true, Code: "license-valid", LastCheckedAt: now,
		LicenseID: env.Payload.LicenseID, CustomerID: env.Payload.CustomerID, SKU: env.Payload.SKU,
		KeyID: env.Payload.KeyID, ExpiresAt: env.Payload.ExpiresAt, NotBefore: env.Payload.NotBefore,
		Features: features, DaysUntilExpiry: days,
	}, nil
}

func (m *licenseManager) statusForError(err licenseValidationError) licenseStatus {
	now := m.now().UTC()
	status := licenseStatus{Enabled: true, Valid: false, Ready: false, Code: err.Code, Message: err.Code, LastCheckedAt: now, Features: map[string]bool{}}
	if strings.TrimSpace(status.Code) == "" {
		status.Code = "license-invalid"
	}
	if err.Code == "license-expired" {
		status.Message = "license expired"
	}
	state, stateErr := m.readState()
	if stateErr == nil && m.cfg.GracePeriodOnValidationError > 0 && state.LastSuccessfulValidationUTC != "" {
		last, _ := time.Parse(time.RFC3339, state.LastSuccessfulValidationUTC)
		if !last.IsZero() && now.Sub(last) <= m.cfg.GracePeriodOnValidationError && err.Code != "license-clock-rollback" && err.Code != "license-expired" {
			status.Valid = true
			status.Ready = true
			status.GraceActive = true
			status.Code = "license-grace"
			status.ValidationReason = err.Code
			status.LicenseID = state.LastValidLicenseID
			status.CustomerID = state.LastValidCustomerID
			status.SKU = state.LastValidSKU
			status.KeyID = state.LastValidKeyID
			status.Features = featureMap(state.LastValidFeatures)
			if t, parseErr := time.Parse(time.RFC3339, state.LastValidExpiresAt); parseErr == nil {
				status.ExpiresAt = t
			}
		}
	}
	return status
}

func (m *licenseManager) checkClock(now time.Time) error {
	state, err := m.readState()
	if err != nil || state.LastObservedWallClockUTC == "" {
		return nil
	}
	last, err := time.Parse(time.RFC3339, state.LastObservedWallClockUTC)
	if err != nil {
		return nil
	}
	if now.Add(5 * time.Minute).Before(last) {
		return licenseValidationError{Code: "license-clock-rollback", StatusCode: 503, Message: "license clock rollback detected"}
	}
	return nil
}

func (m *licenseManager) readState() (licensePersistentState, error) {
	if strings.TrimSpace(m.statePath) == "" {
		return licensePersistentState{}, os.ErrNotExist
	}
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return licensePersistentState{}, err
	}
	var state licensePersistentState
	if err := json.Unmarshal(raw, &state); err != nil {
		return licensePersistentState{}, err
	}
	return state, nil
}

func (m *licenseManager) writeObservedStateLocked(status licenseStatus) error {
	if strings.TrimSpace(m.statePath) == "" {
		return nil
	}
	state, _ := m.readState()
	now := m.now().UTC()
	state.LastObservedWallClockUTC = now.Format(time.RFC3339)
	state.GraceActive = status.GraceActive
	if status.Valid && !status.GraceActive && status.LicenseID != "" {
		state.LastValidLicenseID = status.LicenseID
		state.LastValidCustomerID = status.CustomerID
		state.LastValidSKU = status.SKU
		state.LastValidKeyID = status.KeyID
		state.LastValidExpiresAt = status.ExpiresAt.UTC().Format(time.RFC3339)
		state.LastValidFeatures = featureNames(status.Features)
		state.LastSuccessfulValidationUTC = now.Format(time.RFC3339)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return err
	}
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statePath)
}

func (m *licenseManager) statusSnapshot() licenseStatus {
	if m == nil {
		return licenseStatus{Valid: true, Ready: true, Code: "license-disabled", Features: map[string]bool{}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneLicenseStatus(m.status)
}

func (m *licenseManager) enforce(feature string) *licenseValidationError {
	st := m.statusSnapshot()
	if !st.Enabled {
		return nil
	}
	if !st.Ready || !st.Valid {
		code := defaultString(st.Code, "license-invalid")
		status := httpStatusForLicenseCode(code)
		return &licenseValidationError{Code: code, StatusCode: status, Message: code}
	}
	if feature != "" && !st.Features[feature] && !st.Features["*"] {
		return &licenseValidationError{Code: "license-feature-forbidden", StatusCode: 403, Message: "license-feature-forbidden"}
	}
	return nil
}

func (m *licenseManager) metrics() (licenseStatus, map[string]int64) {
	st := m.statusSnapshot()
	m.mu.RLock()
	defer m.mu.RUnlock()
	failures := map[string]int64{}
	for k, v := range m.failureByReason {
		failures[k] = v
	}
	return st, failures
}

func cloneLicenseStatus(in licenseStatus) licenseStatus {
	out := in
	out.Features = map[string]bool{}
	for k, v := range in.Features {
		out.Features[k] = v
	}
	return out
}

func httpStatusForLicenseCode(code string) int {
	switch code {
	case "license-expired", "license-feature-forbidden", "license-limit-exceeded":
		return 403
	default:
		return 503
	}
}

func ParseLicenseEnvelope(raw []byte) (licenseEnvelope, error) {
	var env licenseEnvelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return env, err
	}
	return env, nil
}

func VerifyLicenseEnvelope(env licenseEnvelope, keys []LicensePublicKey, now time.Time) error {
	if env.Payload.SchemaVersion != 1 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "unsupported license schema"}
	}
	if env.Payload.Product != licenseProduct {
		return licenseValidationError{Code: "license-product-mismatch", StatusCode: 503, Message: "license product mismatch"}
	}
	if env.Payload.Issuer != licenseIssuer {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license issuer mismatch"}
	}
	if env.Signature.Algorithm != "ed25519" {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license signature algorithm mismatch"}
	}
	if env.Payload.KeyID == "" || env.Payload.KeyID != env.Signature.KeyID {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license key id mismatch"}
	}
	key, ok := findLicenseKey(keys, env.Payload.KeyID, now)
	if !ok {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "unknown license key id"}
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature.ValueBase64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license signature is malformed"}
	}
	payloadBytes, err := CanonicalLicensePayload(env.Payload)
	if err != nil {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license canonicalization failed"}
	}
	if !ed25519.Verify(key.PublicKey, payloadBytes, sig) {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license signature verification failed"}
	}
	return nil
}

func validateLicensePayloadForConfig(payload LicensePayload, cfg *Config, now time.Time) error {
	if payload.LicenseID == "" || payload.CustomerID == "" || payload.SKU == "" {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license required fields are missing"}
	}
	if now.Before(payload.NotBefore) {
		return licenseValidationError{Code: "license-not-yet-valid", StatusCode: 503, Message: "license is not yet valid"}
	}
	if !payload.ExpiresAt.IsZero() && !now.Before(payload.ExpiresAt) {
		return licenseValidationError{Code: "license-expired", StatusCode: 403, Message: "license expired"}
	}
	features := featureMap(payload.Features)
	if !features[LicenseFeatureRouting] && !features["*"] {
		return licenseValidationError{Code: "license-feature-forbidden", StatusCode: 403, Message: "routing feature is not licensed"}
	}
	if cfg != nil {
		if payload.Limits.MaxModelGroups > 0 && len(cfg.Models) > payload.Limits.MaxModelGroups {
			return licenseValidationError{Code: "license-limit-exceeded", StatusCode: 403, Message: "model group limit exceeded"}
		}
		if payload.Limits.MaxCallers > 0 && len(cfg.Callers) > payload.Limits.MaxCallers {
			return licenseValidationError{Code: "license-limit-exceeded", StatusCode: 403, Message: "caller limit exceeded"}
		}
	}
	return nil
}

func CanonicalLicensePayload(payload LicensePayload) ([]byte, error) {
	var value any
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	writeCanonicalJSON(&b, value)
	return b.Bytes(), nil
}

func writeCanonicalJSON(b *bytes.Buffer, value any) {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		enc, _ := json.Marshal(v)
		b.Write(enc)
	case string:
		enc, _ := json.Marshal(v)
		b.Write(enc)
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCanonicalJSON(b, item)
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			enc, _ := json.Marshal(k)
			b.Write(enc)
			b.WriteByte(':')
			writeCanonicalJSON(b, v[k])
		}
		b.WriteByte('}')
	default:
		enc, _ := json.Marshal(v)
		b.Write(enc)
	}
}

func findLicenseKey(keys []LicensePublicKey, keyID string, now time.Time) (LicensePublicKey, bool) {
	for _, key := range keys {
		if key.KeyID != keyID || key.Algorithm != "ed25519" || len(key.PublicKey) != ed25519.PublicKeySize {
			continue
		}
		if !key.NotBefore.IsZero() && now.Before(key.NotBefore) {
			continue
		}
		if key.NotAfter != nil && !now.Before(*key.NotAfter) {
			continue
		}
		return key, true
	}
	return LicensePublicKey{}, false
}

func SignLicensePayload(payload LicensePayload, privateKey ed25519.PrivateKey) (licenseEnvelope, error) {
	payloadBytes, err := CanonicalLicensePayload(payload)
	if err != nil {
		return licenseEnvelope{}, err
	}
	sig := ed25519.Sign(privateKey, payloadBytes)
	return licenseEnvelope{
		Payload: payload,
		Signature: LicenseSignature{
			Algorithm:   "ed25519",
			KeyID:       payload.KeyID,
			ValueBase64: base64.StdEncoding.EncodeToString(sig),
		},
	}, nil
}

func GenerateLicenseKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func MarshalLicenseEnvelope(env licenseEnvelope) ([]byte, error) {
	return json.MarshalIndent(env, "", "  ")
}

func featureMap(features []string) map[string]bool {
	out := map[string]bool{}
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature != "" {
			out[feature] = true
		}
	}
	return out
}

func allLicenseFeatures() map[string]bool {
	return map[string]bool{
		"*": true,
	}
}

func featureNames(features map[string]bool) []string {
	out := make([]string, 0, len(features))
	for feature, enabled := range features {
		if enabled && strings.TrimSpace(feature) != "" {
			out = append(out, feature)
		}
	}
	sort.Strings(out)
	return out
}

func licenseFeatureForRoute(dialect string) string {
	switch dialect {
	case "usage":
		return LicenseFeatureUsageReporting
	case "metrics":
		return ""
	case "content-admin":
		return LicenseFeatureContentCapture
	default:
		return LicenseFeatureRouting
	}
}

func licenseFeaturesForGroup(group ModelGroup) []string {
	features := []string{LicenseFeatureRouting}
	if group.Contract != nil {
		features = append(features, LicenseFeatureModelGroupContracts)
	}
	switch strings.ToLower(strings.TrimSpace(group.Strategy)) {
	case "dynamic_score":
		features = append(features, LicenseFeatureDynamicScore)
	case "script":
		features = append(features, LicenseFeatureTypeScriptRouting)
	case "external":
		features = append(features, LicenseFeatureExternalPolicy)
	}
	return uniqueLicenseFeatures(features)
}

func uniqueLicenseFeatures(features []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(features))
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature == "" || seen[feature] {
			continue
		}
		seen[feature] = true
		out = append(out, feature)
	}
	return out
}

func safeLicenseStatusResponse(st licenseStatus) map[string]any {
	out := map[string]any{
		"enabled":           st.Enabled,
		"valid":             st.Valid,
		"ready":             st.Ready,
		"grace_active":      st.GraceActive,
		"status":            st.Code,
		"license_id":        st.LicenseID,
		"customer_id":       st.CustomerID,
		"sku":               st.SKU,
		"key_id":            st.KeyID,
		"days_until_expiry": st.DaysUntilExpiry,
	}
	if !st.ExpiresAt.IsZero() {
		out["expires_at"] = st.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if !st.LastCheckedAt.IsZero() {
		out["last_checked_at"] = st.LastCheckedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func contextWithLicenseReload(ctx context.Context, m *licenseManager) context.Context {
	if m == nil {
		return ctx
	}
	go func() {
		<-ctx.Done()
		m.close()
	}()
	return ctx
}
