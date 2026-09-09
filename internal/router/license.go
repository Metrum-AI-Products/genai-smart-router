// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"smart-llmrouter/internal/licensecontract"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LicenseProduct = licensecontract.Product
	LicenseIssuer  = licensecontract.Issuer

	licenseProduct = LicenseProduct
	licenseIssuer  = LicenseIssuer

	LicenseFeatureRouting              = "routing"
	LicenseFeatureUsageReporting       = "usage_reporting"
	LicenseFeatureAdminReports         = "admin_reports"
	LicenseFeatureAdminSecurityReports = "admin_security_reports"
	LicenseFeatureDynamicScore         = "dynamic_score"
	LicenseFeatureIntelligentRouting   = "intelligent_routing"
	LicenseFeatureExternalPolicy       = "external_policy"
	LicenseFeatureTypeScriptRouting    = "typescript_routing"
	LicenseFeatureModelGroupContracts  = "model_group_contracts"
	LicenseFeatureRetentionRollups     = "retention_rollups"
	LicenseFeatureContentCapture       = "content_capture"
	LicenseFeatureExternalPolicyHTTP   = "external_policy_http"
	LicenseFeaturePrivateUpstreams     = "private_upstreams"
	LicenseFeatureAuditLogExport       = "audit_log_export"
	LicenseFeaturePIIFiltering         = "pii_filtering"
	LicenseFeatureUsageCSVExport       = "usage_csv_export"
	LicenseFeatureUsageBaselineExport  = "usage_baseline_export"
)

type LicenseEnvelope struct {
	Payload   LicensePayload   `json:"payload"`
	Signature LicenseSignature `json:"signature"`
}

type LicenseRevocationEnvelope struct {
	Payload   LicenseRevocationPayload `json:"payload"`
	Signature LicenseSignature         `json:"signature"`
}

type LicenseRevocationPayload struct {
	SchemaVersion   int                      `json:"schema_version"`
	Issuer          string                   `json:"issuer"`
	Product         string                   `json:"product"`
	RevocationSetID string                   `json:"revocation_set_id"`
	RevocationEpoch int64                    `json:"revocation_epoch"`
	IssuedAt        time.Time                `json:"issued_at"`
	NotBefore       time.Time                `json:"not_before"`
	ExpiresAt       time.Time                `json:"expires_at,omitempty"`
	KeyID           string                   `json:"key_id"`
	Entries         []LicenseRevocationEntry `json:"entries"`
}

type LicenseRevocationEntry struct {
	LicenseID    string    `json:"license_id"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	EffectiveAt  time.Time `json:"effective_at"`
	SupersededBy string    `json:"superseded_by,omitempty"`
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

type LicenseLimits = licensecontract.Limits

type LicenseDeployment = licensecontract.Deployment

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
	Enabled               bool
	Valid                 bool
	Ready                 bool
	GraceActive           bool
	Code                  string
	Message               string
	LicenseID             string
	CustomerID            string
	SKU                   string
	KeyID                 string
	ExpiresAt             time.Time
	NotBefore             time.Time
	Features              map[string]bool
	Limits                LicenseLimits
	Deployment            LicenseDeployment
	DaysUntilExpiry       int
	LastCheckedAt         time.Time
	ValidationReason      string
	BindingMatched        bool
	FingerprintHashPrefix string
	RevocationMode        string
	RevocationSetID       string
	RevocationEpoch       int64
	RevocationStatus      string
	RevocationReason      string
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
	inFlight        int
	reservedTokens  int64
	stop            chan struct{}
	stopped         chan struct{}
	failureByReason map[string]int64
}

type licensePersistentState struct {
	LastValidLicenseID          string                    `json:"last_valid_license_id"`
	LastValidLicenseDigest      string                    `json:"last_valid_license_digest,omitempty"`
	LastValidCustomerID         string                    `json:"last_valid_customer_id"`
	LastValidSKU                string                    `json:"last_valid_sku"`
	LastValidKeyID              string                    `json:"last_valid_key_id"`
	LastValidExpiresAt          string                    `json:"last_valid_expires_at"`
	LastValidFeatures           []string                  `json:"last_valid_features"`
	LastValidLimits             LicenseLimits             `json:"last_valid_limits,omitempty"`
	LastValidDeployment         LicenseDeployment         `json:"last_valid_deployment,omitempty"`
	LifetimeTokens              int64                     `json:"lifetime_tokens"`
	LifetimeRequests            int64                     `json:"lifetime_requests"`
	Window                      licenseWindowCounterState `json:"window,omitempty"`
	LastSuccessfulValidationUTC string                    `json:"last_successful_validation_utc"`
	LastObservedWallClockUTC    string                    `json:"last_observed_wall_clock_utc"`
	GraceActive                 bool                      `json:"grace_active"`
	LastRevocationSetID         string                    `json:"last_revocation_set_id,omitempty"`
	LastRevocationEpoch         int64                     `json:"last_revocation_epoch,omitempty"`
	LastRevocationCheckedUTC    string                    `json:"last_revocation_checked_utc,omitempty"`
}

type licenseWindowCounterState struct {
	WindowStart    time.Time `json:"window_start"`
	WindowTokens   int64     `json:"window_tokens"`
	WindowRequests int64     `json:"window_requests"`
}

type licenseReservation struct {
	tokens int64
	active bool
}

func newLicenseManager(cfg LicenseConfig, app *Config, keys []LicensePublicKey) (*licenseManager, error) {
	if !cfg.Enabled && !licenseEnforcementRequired() {
		m := &licenseManager{
			cfg: cfg, app: app, keys: keys, now: time.Now, stop: make(chan struct{}), stopped: make(chan struct{}),
			status:          licenseStatus{Enabled: false, Valid: true, Ready: true, Code: "license-compile-disabled-dev", Features: map[string]bool{}},
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
	raw202601, _ := hex.DecodeString("5f3558f10fd6058fd56c7d03d4ed157fba7c0d87be1717b46fe8fc0b375b1e1f")
	raw202606Prod, _ := hex.DecodeString("c7cfbfb2e3b275528b0f77b4e385849adf379d728d20fbc3f8c42350da9b3a17")
	return []LicensePublicKey{
		{
			KeyID:     "metrum-license-ed25519-2026-01",
			Algorithm: "ed25519",
			PublicKey: ed25519.PublicKey(raw202601),
			NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			KeyID:     "metrum-license-ed25519-2026-06-prod",
			Algorithm: "ed25519",
			PublicKey: ed25519.PublicKey(raw202606Prod),
			NotBefore: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
		},
	}
}

func DefaultLicensePublicKeys() []LicensePublicKey {
	keys := defaultLicensePublicKeys()
	out := make([]LicensePublicKey, len(keys))
	copy(out, keys)
	for i := range out {
		out[i].PublicKey = append(ed25519.PublicKey(nil), out[i].PublicKey...)
	}
	return out
}
func licenseVerificationKeys(cfg LicenseConfig) ([]LicensePublicKey, error) {
	if len(cfg.PublicKeys) == 0 {
		return defaultLicensePublicKeys(), nil
	}
	keys := make([]LicensePublicKey, 0, len(cfg.PublicKeys))
	seen := make(map[string]struct{}, len(cfg.PublicKeys))
	for _, configured := range cfg.PublicKeys {
		keyID := strings.TrimSpace(configured.KeyID)
		path := strings.TrimSpace(configured.Path)
		if keyID == "" || path == "" {
			return nil, fmt.Errorf("license public_keys require key_id and path")
		}
		if _, exists := seen[keyID]; exists {
			return nil, fmt.Errorf("license public_keys duplicate key_id %q", keyID)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read license public key %q: %w", keyID, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("license public key %q must be a base64 Ed25519 public key", keyID)
		}
		seen[keyID] = struct{}{}
		keys = append(keys, LicensePublicKey{
			KeyID: keyID, Algorithm: "ed25519", PublicKey: ed25519.PublicKey(decoded),
		})
	}
	return keys, nil
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
		if status.Code != "" {
			// validateFile may return safe revocation metadata while still
			// failing closed on a signed revocation decision.
		} else if errors.As(err, &lerr) {
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
	if m.cfg.FailOpenForDev && !licenseEnforcementRequired() {
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
	bindingMatched := false
	hashPrefixValue := ""
	if licensePayloadUsesInstanceBinding(env.Payload) {
		bindingMatched, hashPrefixValue, _ = validateLicensedInstance(env.Payload, m.cfg)
	}
	revocationStatus, err := m.validateRevocation(env.Payload, now)
	if err != nil {
		status := licenseStatus{
			Enabled: true, Valid: false, Ready: false, Code: licenseErrorCode(err), Message: licenseErrorCode(err), LastCheckedAt: now,
			LicenseID: env.Payload.LicenseID, CustomerID: env.Payload.CustomerID, SKU: env.Payload.SKU,
			KeyID: env.Payload.KeyID, ExpiresAt: env.Payload.ExpiresAt, NotBefore: env.Payload.NotBefore,
			Features: featureMap(env.Payload.Features), Limits: env.Payload.Limits, Deployment: env.Payload.Deployment,
			BindingMatched: bindingMatched, FingerprintHashPrefix: hashPrefixValue,
			RevocationMode: revocationStatus.Mode, RevocationSetID: revocationStatus.SetID, RevocationEpoch: revocationStatus.Epoch,
			RevocationStatus: revocationStatus.Status, RevocationReason: revocationStatus.Reason,
		}
		return status, err
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
		Features: features, Limits: env.Payload.Limits, Deployment: env.Payload.Deployment, DaysUntilExpiry: days,
		BindingMatched: bindingMatched, FingerprintHashPrefix: hashPrefixValue,
		RevocationMode: revocationStatus.Mode, RevocationSetID: revocationStatus.SetID, RevocationEpoch: revocationStatus.Epoch,
		RevocationStatus: revocationStatus.Status, RevocationReason: revocationStatus.Reason,
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
		digest, digestErr := m.currentLicenseDigest()
		digestMatches := state.LastValidLicenseDigest != "" && (digestErr != nil || state.LastValidLicenseDigest == digest || err.Code == "license-invalid")
		if !last.IsZero() && digestMatches && now.Sub(last) <= m.cfg.GracePeriodOnValidationError && licenseErrorAllowsGrace(err.Code) {
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
			status.Limits = state.LastValidLimits
			status.Deployment = state.LastValidDeployment
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
	raw, err := readVersionedStateFile(m.statePath)
	if err != nil {
		return licensePersistentState{}, err
	}
	key, keyID, err := m.licenseStateIntegrityKey()
	if err != nil {
		return licensePersistentState{}, err
	}
	state, enveloped, err := unmarshalIntegrityState[licensePersistentState](raw, keyID, key)
	if err != nil {
		return licensePersistentState{}, err
	}
	if !enveloped {
		return licensePersistentState{}, errStateIntegrity
	}
	return state, nil
}

func (m *licenseManager) readLegacyUnsignedState() (licensePersistentState, error) {
	if strings.TrimSpace(m.statePath) == "" {
		return licensePersistentState{}, os.ErrNotExist
	}
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return licensePersistentState{}, err
	}
	var probe struct {
		SchemaVersion int             `json:"schema_version"`
		Integrity     json.RawMessage `json:"integrity"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && (probe.SchemaVersion != 0 || len(probe.Integrity) > 0) {
		return licensePersistentState{}, errStateIntegrity
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
	state, stateErr := m.readState()
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		if !errors.Is(stateErr, errStateIntegrity) || !status.Valid || status.GraceActive || status.LicenseID == "" {
			return stateErr
		}
		legacy, legacyErr := m.readLegacyUnsignedState()
		if legacyErr != nil {
			return stateErr
		}
		state = legacy
	}
	now := m.now().UTC()
	state.LastObservedWallClockUTC = now.Format(time.RFC3339)
	state.GraceActive = status.GraceActive
	if status.Valid && !status.GraceActive && status.LicenseID != "" {
		digest, err := m.currentLicenseDigest()
		if err != nil {
			return err
		}
		if state.LastValidLicenseID != "" && state.LastValidLicenseID != status.LicenseID {
			state.LifetimeTokens = 0
			state.LifetimeRequests = 0
			state.Window = licenseWindowCounterState{}
		}
		state.LastValidLicenseID = status.LicenseID
		state.LastValidLicenseDigest = digest
		state.LastValidCustomerID = status.CustomerID
		state.LastValidSKU = status.SKU
		state.LastValidKeyID = status.KeyID
		state.LastValidExpiresAt = status.ExpiresAt.UTC().Format(time.RFC3339)
		state.LastValidFeatures = featureNames(status.Features)
		state.LastValidLimits = status.Limits
		state.LastValidDeployment = status.Deployment
		state.LastSuccessfulValidationUTC = now.Format(time.RFC3339)
	}
	if status.RevocationEpoch > 0 {
		state.LastRevocationSetID = status.RevocationSetID
		state.LastRevocationEpoch = status.RevocationEpoch
		state.LastRevocationCheckedUTC = now.Format(time.RFC3339)
	}
	raw, err := m.marshalLicenseState(state)
	if err != nil {
		return err
	}
	return writeVersionedStateFile(m.statePath, raw)
}

func (m *licenseManager) statusSnapshot() licenseStatus {
	if m == nil {
		return licenseStatus{Valid: true, Ready: true, Code: "license-compile-disabled-dev", Features: map[string]bool{}}
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

func (m *licenseManager) AdmitRequest(dialect string) *licenseValidationError {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := cloneLicenseStatus(m.status)
	if !st.Enabled {
		return nil
	}
	if lerr := licenseStatusError(st); lerr != nil {
		return lerr
	}
	if skin := licenseSkinForDialect(dialect); skin != "" && len(st.Limits.AllowedSkins) > 0 && !licenseSkinAllowed(st.Limits.AllowedSkins, skin) {
		return &licenseValidationError{Code: "license-skin-forbidden", StatusCode: 403, Message: "license-skin-forbidden"}
	}
	if st.Limits.MaxConcurrent > 0 && m.inFlight >= st.Limits.MaxConcurrent {
		return &licenseValidationError{Code: "license-concurrency-exceeded", StatusCode: 429, Message: "license-concurrency-exceeded"}
	}
	state, err := m.readState()
	if err != nil {
		return &licenseValidationError{Code: "license-state-error", StatusCode: 503, Message: "license-state-error"}
	}
	now := m.now().UTC()
	m.resetLicenseUsageStateLocked(&state, st, now)
	if st.Limits.MaxTotalRequests > 0 && state.LifetimeRequests+1 > st.Limits.MaxTotalRequests {
		return &licenseValidationError{Code: "license-volume-exceeded", StatusCode: 429, Message: "license-volume-exceeded"}
	}
	if st.Limits.WindowRequests > 0 && state.Window.WindowRequests+1 > st.Limits.WindowRequests {
		return &licenseValidationError{Code: "license-window-exceeded", StatusCode: 429, Message: "license-window-exceeded"}
	}
	state.LifetimeRequests++
	if st.Limits.WindowRequests > 0 {
		state.Window.WindowRequests++
	}
	m.inFlight++
	if err := m.writeStateLocked(state); err != nil {
		m.inFlight--
		return &licenseValidationError{Code: "license-state-error", StatusCode: 503, Message: "license-state-error"}
	}
	return nil
}

func (m *licenseManager) ReleaseRequest() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inFlight > 0 {
		m.inFlight--
	}
}

func (m *licenseManager) ReserveTokens(estTokens int) (*licenseReservation, *licenseValidationError) {
	if m == nil || estTokens <= 0 {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := cloneLicenseStatus(m.status)
	if !st.Enabled {
		return nil, nil
	}
	if lerr := licenseStatusError(st); lerr != nil {
		return nil, lerr
	}
	state, err := m.readState()
	if err != nil {
		return nil, &licenseValidationError{Code: "license-state-error", StatusCode: 503, Message: "license-state-error"}
	}
	now := m.now().UTC()
	m.resetLicenseUsageStateLocked(&state, st, now)
	est := int64(estTokens)
	if st.Limits.MaxTotalTokens > 0 && state.LifetimeTokens+m.reservedTokens+est > st.Limits.MaxTotalTokens {
		return nil, &licenseValidationError{Code: "license-volume-exceeded", StatusCode: 429, Message: "license-volume-exceeded"}
	}
	if st.Limits.WindowTokens > 0 && state.Window.WindowTokens+m.reservedTokens+est > st.Limits.WindowTokens {
		return nil, &licenseValidationError{Code: "license-window-exceeded", StatusCode: 429, Message: "license-window-exceeded"}
	}
	m.reservedTokens += est
	return &licenseReservation{tokens: est, active: true}, nil
}

func (m *licenseManager) ReleaseReservation(reservation *licenseReservation) {
	if m == nil || reservation == nil || !reservation.active {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseReservationLocked(reservation)
}

func (m *licenseManager) RecordTokens(reservation *licenseReservation, usage Usage) {
	if m == nil {
		return
	}
	total := totalTokens(usage)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseReservationLocked(reservation)
	st := cloneLicenseStatus(m.status)
	if !st.Enabled || total <= 0 {
		return
	}
	state, err := m.readState()
	if err != nil {
		return
	}
	now := m.now().UTC()
	m.resetLicenseUsageStateLocked(&state, st, now)
	state.LifetimeTokens += int64(total)
	if st.Limits.WindowTokens > 0 {
		state.Window.WindowTokens += int64(total)
	}
	_ = m.writeStateLocked(state)
}

func (m *licenseManager) releaseReservationLocked(reservation *licenseReservation) {
	if reservation == nil || !reservation.active {
		return
	}
	m.reservedTokens -= reservation.tokens
	if m.reservedTokens < 0 {
		m.reservedTokens = 0
	}
	reservation.active = false
}

func (m *licenseManager) resetLicenseUsageStateLocked(state *licensePersistentState, status licenseStatus, now time.Time) {
	if state == nil {
		return
	}
	if status.LicenseID != "" && state.LastValidLicenseID != "" && state.LastValidLicenseID != status.LicenseID {
		state.LifetimeTokens = 0
		state.LifetimeRequests = 0
		state.Window = licenseWindowCounterState{}
	}
	if state.LastValidLicenseID == "" && status.LicenseID != "" {
		state.LastValidLicenseID = status.LicenseID
	}
	if status.Limits.WindowDurationSeconds > 0 {
		window := time.Duration(status.Limits.WindowDurationSeconds) * time.Second
		if state.Window.WindowStart.IsZero() || !now.Before(state.Window.WindowStart.Add(window)) {
			state.Window.WindowStart = now
			state.Window.WindowTokens = 0
			state.Window.WindowRequests = 0
		}
	}
}

func (m *licenseManager) writeStateLocked(state licensePersistentState) error {
	if strings.TrimSpace(m.statePath) == "" {
		return nil
	}
	raw, err := m.marshalLicenseState(state)
	if err != nil {
		return err
	}
	return writeVersionedStateFile(m.statePath, raw)
}

func (m *licenseManager) marshalLicenseState(state licensePersistentState) ([]byte, error) {
	key, keyID, err := m.licenseStateIntegrityKey()
	if err != nil {
		return nil, err
	}
	return marshalIntegrityState(state, keyID, key)
}

func (m *licenseManager) licenseStateIntegrityKey() ([]byte, string, error) {
	parts := []string{"smart-llmrouter license state v1", m.statePath, m.cfg.InstanceFingerprint}
	for _, key := range m.keys {
		parts = append(parts, key.KeyID, key.Algorithm, hex.EncodeToString(key.PublicKey))
	}
	key, keyID := stateIntegrityMaterial(parts...)
	return key, keyID, nil
}

func (m *licenseManager) currentLicenseDigest() (string, error) {
	if m == nil || strings.TrimSpace(m.cfg.Path) == "" {
		return "", errStateIntegrity
	}
	raw, err := os.ReadFile(strings.TrimSpace(m.cfg.Path))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func licenseStatusError(st licenseStatus) *licenseValidationError {
	if !st.Ready || !st.Valid {
		code := defaultString(st.Code, "license-invalid")
		status := httpStatusForLicenseCode(code)
		return &licenseValidationError{Code: code, StatusCode: status, Message: code}
	}
	return nil
}

func licenseErrorCode(err error) string {
	var lerr licenseValidationError
	if errors.As(err, &lerr) && strings.TrimSpace(lerr.Code) != "" {
		return lerr.Code
	}
	return "license-invalid"
}

func licenseErrorAllowsGrace(code string) bool {
	switch code {
	case "license-clock-rollback", "license-expired", "license-revoked", "license-suspended", "license-superseded",
		"license-revocation-required", "license-revocation-check-failed":
		return false
	default:
		return true
	}
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

func (m *licenseManager) usageSnapshot() licensePersistentState {
	if m == nil {
		return licensePersistentState{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, _ := m.readState()
	return state
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
	case "license-volume-exceeded", "license-window-exceeded", "license-concurrency-exceeded":
		return 429
	case "license-expired", "license-feature-forbidden", "license-limit-exceeded":
		return 403
	case "license-skin-forbidden", "license-admin-limit-exceeded", "license-retention-limit-exceeded":
		return 403
	case "license-revoked", "license-suspended", "license-superseded":
		return 403
	case "license-instance-limit-exceeded":
		return 503
	default:
		return 503
	}
}

func ParseLicenseEnvelope(raw []byte) (LicenseEnvelope, error) {
	var env LicenseEnvelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&env); err != nil {
		return env, err
	}
	return env, nil
}

func VerifyLicenseEnvelope(env LicenseEnvelope, keys []LicensePublicKey, now time.Time) error {
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
	if err := validateLicenseLimitShape(payload.Limits); err != nil {
		return err
	}
	if err := validateLicenseDeploymentShape(payload.Deployment); err != nil {
		return err
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
		if payload.Limits.MaxAdmins > 0 && countConfiguredAdmins(cfg.Server.AdminAuth) > payload.Limits.MaxAdmins {
			return licenseValidationError{Code: "license-admin-limit-exceeded", StatusCode: 403, Message: "admin limit exceeded"}
		}
		if payload.Limits.MaxRetentionDays > 0 && maxConfiguredRetentionDays(*cfg) > payload.Limits.MaxRetentionDays {
			return licenseValidationError{Code: "license-retention-limit-exceeded", StatusCode: 403, Message: "retention limit exceeded"}
		}
		if licensePayloadUsesInstanceBinding(payload) {
			if _, _, err := validateLicensedInstance(payload, cfg.Server.License); err != nil {
				return err
			}
		}
		if !features[LicenseFeaturePrivateUpstreams] && !features["*"] && configUsesPrivateUpstreams(cfg) {
			return licenseValidationError{Code: "license-feature-forbidden", StatusCode: 403, Message: "private upstreams feature is not licensed"}
		}
		if !features[LicenseFeaturePIIFiltering] && !features["*"] && configUsesPIIFilter(cfg) {
			return licenseValidationError{Code: "license-feature-forbidden", StatusCode: 403, Message: "pii filtering feature is not licensed"}
		}
		if !features[LicenseFeatureExternalPolicyHTTP] && !features["*"] && configUsesScriptHTTP(cfg) {
			return licenseValidationError{Code: "license-feature-forbidden", StatusCode: 403, Message: "script external HTTP feature is not licensed"}
		}
	}
	return nil
}

func ValidateLicensePayload(payload LicensePayload, cfg *Config, keys []LicensePublicKey, now time.Time) error {
	if payload.SchemaVersion != 1 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "unsupported license schema"}
	}
	if payload.Product != licenseProduct {
		return licenseValidationError{Code: "license-product-mismatch", StatusCode: 503, Message: "license product mismatch"}
	}
	if payload.Issuer != licenseIssuer {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license issuer mismatch"}
	}
	if payload.IssuedAt.IsZero() || payload.NotBefore.IsZero() || payload.ExpiresAt.IsZero() {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license date fields are required"}
	}
	if !payload.ExpiresAt.After(payload.NotBefore) {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license expires_at must be after not_before"}
	}
	if payload.IssuedAt.After(payload.NotBefore) {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license issued_at must not be after not_before"}
	}
	if err := validateLicensePayloadForConfig(payload, cfg, now); err != nil {
		return err
	}
	if strings.TrimSpace(payload.KeyID) == "" {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license key id is required"}
	}
	if len(keys) > 0 {
		if _, ok := findLicenseKey(keys, payload.KeyID, payload.NotBefore); !ok {
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "unknown license key id"}
		}
	}
	for _, feature := range payload.Features {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license features cannot contain empty values"}
		}
		if feature == "*" {
			continue
		}
		if !KnownLicenseFeatures()[feature] {
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license features contain unsupported feature"}
		}
	}
	return nil
}

func licensePayloadUsesInstanceBinding(payload LicensePayload) bool {
	return payload.Limits.MaxInstances > 0 || payload.Deployment.InstanceFingerprintRequired ||
		strings.TrimSpace(payload.Deployment.InstanceFingerprint) != "" || len(payload.Deployment.AllowedInstances) > 0 ||
		len(payload.Deployment.AllowedInstanceFingerprints) > 0
}

func KnownLicenseFeatures() map[string]bool {
	return map[string]bool{
		LicenseFeatureRouting:              true,
		LicenseFeatureUsageReporting:       true,
		LicenseFeatureAdminReports:         true,
		LicenseFeatureAdminSecurityReports: true,
		LicenseFeatureDynamicScore:         true,
		LicenseFeatureIntelligentRouting:   true,
		LicenseFeatureExternalPolicy:       true,
		LicenseFeatureTypeScriptRouting:    true,
		LicenseFeatureModelGroupContracts:  true,
		LicenseFeatureRetentionRollups:     true,
		LicenseFeatureContentCapture:       true,
		LicenseFeatureExternalPolicyHTTP:   true,
		LicenseFeaturePrivateUpstreams:     true,
		LicenseFeatureAuditLogExport:       true,
		LicenseFeaturePIIFiltering:         true,
		LicenseFeatureUsageCSVExport:       true,
		LicenseFeatureUsageBaselineExport:  true,
	}
}

func validateLicenseLimitShape(limits LicenseLimits) error {
	if limits.MaxModelGroups < 0 || limits.MaxCallers < 0 || limits.MaxMonthlyRequests < 0 ||
		limits.MaxTotalTokens < 0 || limits.MaxTotalRequests < 0 || limits.WindowTokens < 0 ||
		limits.WindowRequests < 0 || limits.WindowDurationSeconds < 0 || limits.MaxConcurrent < 0 ||
		limits.MaxAdmins < 0 || limits.MaxRetentionDays < 0 || limits.MaxInstances < 0 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license limits cannot be negative"}
	}
	windowCapConfigured := limits.WindowTokens > 0 || limits.WindowRequests > 0
	if windowCapConfigured && limits.WindowDurationSeconds <= 0 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license window duration is required"}
	}
	if !windowCapConfigured && limits.WindowDurationSeconds > 0 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license window cap is required"}
	}
	for _, skin := range limits.AllowedSkins {
		switch strings.ToLower(strings.TrimSpace(skin)) {
		case "openai-chat", "openai-responses", "anthropic-messages":
		default:
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license allowed_skins contains unsupported skin"}
		}
	}
	return nil
}

func validateLicenseDeploymentShape(deployment LicenseDeployment) error {
	if deployment.BindingVersion < 0 || deployment.MaxOfflineInstances < 0 {
		return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license deployment binding fields cannot be negative"}
	}
	if deployment.BindingVersion > 1 {
		return licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license binding version is unsupported"}
	}
	for _, allowed := range deployment.AllowedInstances {
		if strings.TrimSpace(allowed) == "" {
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license allowed_instances contains an empty fingerprint"}
		}
	}
	for _, allowed := range deployment.AllowedInstanceFingerprints {
		if strings.TrimSpace(allowed) == "" {
			return licenseValidationError{Code: "license-invalid", StatusCode: 503, Message: "license allowed_instance_fingerprints contains an empty fingerprint"}
		}
	}
	if deployment.MaxOfflineInstances > 0 && len(deployment.AllowedInstanceFingerprints) > deployment.MaxOfflineInstances {
		return licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license offline instance limit exceeded"}
	}
	return nil
}

func validateLicensedInstance(payload LicensePayload, cfg LicenseConfig) (bool, string, error) {
	fingerprint, err := configuredLicenseFingerprint(cfg)
	if err != nil {
		return false, "", err
	}
	hash := licenseInstanceFingerprintHash(payload, fingerprint)
	if payload.Limits.MaxInstances > 0 && len(payload.Deployment.AllowedInstances)+len(payload.Deployment.AllowedInstanceFingerprints) > payload.Limits.MaxInstances {
		return false, hashPrefix(hash), licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance limit exceeded"}
	}
	allowedInstances := normalizedLicenseStrings(payload.Deployment.AllowedInstances)
	if len(allowedInstances) == 0 && strings.TrimSpace(payload.Deployment.InstanceFingerprint) != "" {
		allowedInstances = []string{strings.TrimSpace(payload.Deployment.InstanceFingerprint)}
	}
	allowedHashes := normalizedLicenseStrings(payload.Deployment.AllowedInstanceFingerprints)
	if payload.Deployment.InstanceFingerprintRequired && fingerprint == "" {
		return false, hashPrefix(hash), licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance fingerprint is required"}
	}
	if len(allowedInstances) == 0 && len(allowedHashes) == 0 {
		return fingerprint != "", hashPrefix(hash), nil
	}
	if fingerprint == "" {
		return false, hashPrefix(hash), licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance fingerprint is required"}
	}
	for _, allowed := range allowedInstances {
		if allowed == fingerprint {
			return true, hashPrefix(hash), nil
		}
	}
	for _, allowed := range allowedHashes {
		if strings.EqualFold(allowed, hash) {
			return true, hashPrefix(hash), nil
		}
	}
	return false, hashPrefix(hash), licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance fingerprint is not allowed"}
}

type revocationCheckStatus struct {
	Mode   string
	SetID  string
	Epoch  int64
	Status string
	Reason string
}

func (m *licenseManager) validateRevocation(payload LicensePayload, now time.Time) (revocationCheckStatus, error) {
	cfg := m.cfg.Revocation
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "off"
	}
	status := revocationCheckStatus{Mode: mode, Status: "clear"}
	if mode == "off" {
		return status, nil
	}
	if mode != "file" {
		return status, licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation mode is unsupported"}
	}
	if strings.TrimSpace(cfg.Path) == "" {
		if cfg.RequireCurrentBundle || cfg.FailClosedOnBundleError {
			return status, licenseValidationError{Code: "license-revocation-required", StatusCode: 503, Message: "license revocation bundle is required"}
		}
		status.Status = "missing"
		return status, nil
	}
	raw, err := os.ReadFile(strings.TrimSpace(cfg.Path))
	if err != nil {
		if cfg.RequireCurrentBundle || cfg.FailClosedOnBundleError {
			return status, licenseValidationError{Code: "license-revocation-required", StatusCode: 503, Message: "license revocation bundle is required"}
		}
		status.Status = "missing"
		return status, nil
	}
	env, err := ParseLicenseRevocationEnvelope(raw)
	if err != nil {
		if cfg.FailClosedOnBundleError {
			return status, licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle is malformed"}
		}
		status.Status = "check_failed"
		return status, nil
	}
	if err := VerifyLicenseRevocationEnvelope(env, m.keys, now); err != nil {
		if cfg.FailClosedOnBundleError || cfg.RequireCurrentBundle {
			return status, err
		}
		status.Status = "check_failed"
		return status, nil
	}
	state, _ := m.readState()
	if state.LastRevocationEpoch > 0 && env.Payload.RevocationEpoch < state.LastRevocationEpoch {
		return status, licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle rollback detected"}
	}
	status.SetID = env.Payload.RevocationSetID
	status.Epoch = env.Payload.RevocationEpoch
	for _, entry := range env.Payload.Entries {
		if strings.TrimSpace(entry.LicenseID) != payload.LicenseID {
			continue
		}
		entryStatus := strings.ToLower(strings.TrimSpace(entry.Status))
		if entry.EffectiveAt.IsZero() || now.Before(entry.EffectiveAt) {
			status.Status = "pending"
			status.Reason = entry.Reason
			return status, nil
		}
		status.Status = entryStatus
		status.Reason = entry.Reason
		switch entryStatus {
		case "revoked":
			return status, licenseValidationError{Code: "license-revoked", StatusCode: 403, Message: "license revoked"}
		case "suspended":
			return status, licenseValidationError{Code: "license-suspended", StatusCode: 403, Message: "license suspended"}
		case "superseded":
			return status, licenseValidationError{Code: "license-superseded", StatusCode: 403, Message: "license superseded"}
		default:
			return status, licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle contains unsupported status"}
		}
	}
	return status, nil
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

func SignLicensePayload(payload LicensePayload, privateKey ed25519.PrivateKey) (LicenseEnvelope, error) {
	payloadBytes, err := CanonicalLicensePayload(payload)
	if err != nil {
		return LicenseEnvelope{}, err
	}
	sig := ed25519.Sign(privateKey, payloadBytes)
	return LicenseEnvelope{
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

func MarshalLicenseEnvelope(env LicenseEnvelope) ([]byte, error) {
	return json.MarshalIndent(env, "", "  ")
}

func ParseLicenseRevocationEnvelope(raw []byte) (LicenseRevocationEnvelope, error) {
	var env LicenseRevocationEnvelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&env); err != nil {
		return env, err
	}
	return env, nil
}

func VerifyLicenseRevocationEnvelope(env LicenseRevocationEnvelope, keys []LicensePublicKey, now time.Time) error {
	if env.Payload.SchemaVersion != 1 || env.Payload.Product != licenseProduct || env.Payload.Issuer != licenseIssuer {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle identity mismatch"}
	}
	if env.Payload.RevocationSetID == "" || env.Payload.RevocationEpoch <= 0 {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle required fields are missing"}
	}
	if now.Before(env.Payload.NotBefore) || (!env.Payload.ExpiresAt.IsZero() && !now.Before(env.Payload.ExpiresAt)) {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation bundle is not current"}
	}
	if env.Signature.Algorithm != "ed25519" || env.Payload.KeyID == "" || env.Payload.KeyID != env.Signature.KeyID {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation signature key mismatch"}
	}
	key, ok := findLicenseKey(keys, env.Payload.KeyID, now)
	if !ok {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "unknown revocation key id"}
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature.ValueBase64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation signature is malformed"}
	}
	payloadBytes, err := CanonicalLicenseRevocationPayload(env.Payload)
	if err != nil {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation canonicalization failed"}
	}
	if !ed25519.Verify(key.PublicKey, payloadBytes, sig) {
		return licenseValidationError{Code: "license-revocation-check-failed", StatusCode: 503, Message: "license revocation signature verification failed"}
	}
	return nil
}

func CanonicalLicenseRevocationPayload(payload LicenseRevocationPayload) ([]byte, error) {
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

func SignLicenseRevocationPayload(payload LicenseRevocationPayload, privateKey ed25519.PrivateKey) (LicenseRevocationEnvelope, error) {
	payloadBytes, err := CanonicalLicenseRevocationPayload(payload)
	if err != nil {
		return LicenseRevocationEnvelope{}, err
	}
	sig := ed25519.Sign(privateKey, payloadBytes)
	return LicenseRevocationEnvelope{Payload: payload, Signature: LicenseSignature{Algorithm: "ed25519", KeyID: payload.KeyID, ValueBase64: base64.StdEncoding.EncodeToString(sig)}}, nil
}

func MarshalLicenseRevocationEnvelope(env LicenseRevocationEnvelope) ([]byte, error) {
	return json.MarshalIndent(env, "", "  ")
}

func configuredLicenseFingerprint(cfg LicenseConfig) (string, error) {
	type source struct{ name, value string }
	var sources []source
	if strings.TrimSpace(cfg.InstanceFingerprint) != "" {
		sources = append(sources, source{name: "instance_fingerprint", value: strings.TrimSpace(cfg.InstanceFingerprint)})
	}
	if strings.TrimSpace(cfg.InstanceFingerprintEnv) != "" {
		if value := strings.TrimSpace(os.Getenv(strings.TrimSpace(cfg.InstanceFingerprintEnv))); value != "" {
			sources = append(sources, source{name: "instance_fingerprint_env", value: value})
		}
	}
	if strings.TrimSpace(cfg.InstanceFingerprintFile) != "" {
		raw, err := os.ReadFile(strings.TrimSpace(cfg.InstanceFingerprintFile))
		if err != nil {
			return "", licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance fingerprint file is unreadable"}
		}
		if value := strings.TrimSpace(string(raw)); value != "" {
			sources = append(sources, source{name: "instance_fingerprint_file", value: value})
		}
	}
	if len(sources) == 0 {
		return "", nil
	}
	if len(sources) > 1 {
		return "", licenseValidationError{Code: "license-instance-limit-exceeded", StatusCode: 503, Message: "license instance fingerprint source is ambiguous"}
	}
	return sources[0].value, nil
}

func licenseInstanceFingerprintHash(payload LicensePayload, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	deploymentID := strings.TrimSpace(payload.Deployment.DeploymentID)
	if deploymentID == "" {
		deploymentID = strings.TrimSpace(payload.Deployment.Mode)
	}
	material := strings.Join([]string{
		licenseProduct,
		strings.TrimSpace(payload.CustomerID),
		strings.TrimSpace(payload.LicenseID),
		deploymentID,
		strings.TrimSpace(raw),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashPrefix(value string) string {
	if len(value) <= len("sha256:")+12 {
		return value
	}
	return value[:len("sha256:")+12]
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

func licenseSkinForDialect(dialect string) string {
	switch normalizeDialect(dialect) {
	case "openai-chat":
		return "openai-chat"
	case "openai-responses":
		return "openai-responses"
	case "anthropic":
		return "anthropic-messages"
	default:
		return ""
	}
}

func licenseSkinAllowed(allowed []string, skin string) bool {
	skin = strings.ToLower(strings.TrimSpace(skin))
	for _, candidate := range allowed {
		if strings.ToLower(strings.TrimSpace(candidate)) == skin {
			return true
		}
	}
	return false
}

func normalizedLicenseStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func licenseFeaturesForGroup(group ModelGroup) []string {
	features := []string{LicenseFeatureRouting}
	if group.Contract != nil {
		features = append(features, LicenseFeatureModelGroupContracts)
	}
	switch strings.ToLower(strings.TrimSpace(group.Strategy)) {
	case "dynamic_score":
		features = append(features, LicenseFeatureDynamicScore)
	case "intelligent":
		features = append(features, LicenseFeatureIntelligentRouting)
	case "script":
		features = append(features, LicenseFeatureTypeScriptRouting)
		if group.ScriptHTTP.Enabled {
			features = append(features, LicenseFeatureExternalPolicyHTTP)
		}
	case "external":
		features = append(features, LicenseFeatureExternalPolicy)
	}
	if group.PIIFilter.Enabled {
		features = append(features, LicenseFeaturePIIFiltering)
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
		"compile_mode":      licenseCompileMode,
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
	if st.Deployment.DeploymentID != "" || st.Deployment.BindingVersion > 0 || st.FingerprintHashPrefix != "" {
		out["deployment_id"] = st.Deployment.DeploymentID
		out["binding_version"] = st.Deployment.BindingVersion
		out["instance_binding_matched"] = st.BindingMatched
		out["instance_fingerprint_hash_prefix"] = st.FingerprintHashPrefix
	}
	if st.RevocationMode != "" && st.RevocationMode != "off" {
		out["revocation_mode"] = st.RevocationMode
		out["revocation_status"] = st.RevocationStatus
		out["revocation_set_id"] = st.RevocationSetID
		out["revocation_epoch"] = st.RevocationEpoch
		if st.RevocationReason != "" {
			out["revocation_reason"] = st.RevocationReason
		}
	}
	return out
}

func safeLicenseStatusResponseWithUsage(m *licenseManager) map[string]any {
	if m == nil {
		return safeLicenseStatusResponse(licenseStatus{Valid: true, Ready: true, Code: "license-disabled", Features: map[string]bool{}})
	}
	st := m.statusSnapshot()
	out := safeLicenseStatusResponse(st)
	usage := m.usageSnapshot()
	out["usage"] = map[string]any{
		"lifetime_tokens":   usage.LifetimeTokens,
		"lifetime_requests": usage.LifetimeRequests,
		"window": map[string]any{
			"window_start":    formatOptionalTime(usage.Window.WindowStart),
			"window_tokens":   usage.Window.WindowTokens,
			"window_requests": usage.Window.WindowRequests,
		},
	}
	out["limits"] = map[string]any{
		"max_total_tokens":        st.Limits.MaxTotalTokens,
		"max_total_requests":      st.Limits.MaxTotalRequests,
		"window_tokens":           st.Limits.WindowTokens,
		"window_requests":         st.Limits.WindowRequests,
		"window_duration_seconds": st.Limits.WindowDurationSeconds,
		"max_concurrent":          st.Limits.MaxConcurrent,
		"max_admins":              st.Limits.MaxAdmins,
		"max_retention_days":      st.Limits.MaxRetentionDays,
		"max_instances":           st.Limits.MaxInstances,
		"allowed_skins":           append([]string(nil), st.Limits.AllowedSkins...),
	}
	return out
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
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

func countConfiguredAdmins(auth AdminAuthConfig) int {
	subjects := map[string]bool{}
	for _, user := range auth.Basic.Users {
		subject := strings.TrimSpace(user.Subject)
		if subject == "" && strings.TrimSpace(user.Username) != "" {
			subject = "basic:" + strings.TrimSpace(user.Username)
		}
		if subject != "" {
			subjects[subject] = true
		}
	}
	return len(subjects)
}

func maxConfiguredRetentionDays(cfg Config) int {
	maxDays := 0
	if cfg.Server.Retention.Enabled {
		for _, class := range cfg.Server.Retention.Classes {
			if class.Enabled != nil && !*class.Enabled {
				continue
			}
			if class.RetentionDays > maxDays {
				maxDays = class.RetentionDays
			}
		}
	}
	if cfg.Server.ContentCapture.Enabled && cfg.Server.ContentCapture.RetentionDays > maxDays {
		maxDays = cfg.Server.ContentCapture.RetentionDays
	}
	if (cfg.Server.Diagnostics.Enabled == nil || *cfg.Server.Diagnostics.Enabled) && cfg.Server.Diagnostics.RetentionDays > maxDays {
		maxDays = cfg.Server.Diagnostics.RetentionDays
	}
	if cfg.Server.AdminReports.Enabled && cfg.Server.AdminReports.Security.Enabled && cfg.Server.AdminReports.Security.RetentionDays > maxDays {
		maxDays = cfg.Server.AdminReports.Security.RetentionDays
	}
	return maxDays
}

func configUsesPIIFilter(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, group := range cfg.Models {
		if group.PIIFilter.Enabled {
			return true
		}
	}
	return false
}

func configUsesScriptHTTP(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, group := range cfg.Models {
		if group.ScriptHTTP.Enabled {
			return true
		}
	}
	return false
}

func configUsesPrivateUpstreams(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, provider := range cfg.Provider {
		if providerBaseURLIsPrivate(provider.BaseURL) {
			return true
		}
	}
	return false
}

func providerBaseURLIsPrivate(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(u.Hostname()), "."))
	if host == "" {
		return false
	}
	if strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast())
}
