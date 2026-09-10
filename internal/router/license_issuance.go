// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"github.com/metrum-ai/router/internal/licensecontract"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type LicenseSKUCatalog struct {
	SchemaVersion int `json:"schema_version"`
	Metadata      struct {
		Product string `json:"product"`
	} `json:"metadata"`
	EntitlementNames struct {
		Features   []string `json:"features"`
		Limits     []string `json:"limits"`
		Deployment []string `json:"deployment"`
	} `json:"entitlement_names"`
	Defaults struct {
		Product string `json:"product"`
		Issuer  string `json:"issuer"`
	} `json:"defaults"`
	SKUs []LicenseSKUTemplate `json:"skus"`
}

type LicenseSKUTemplate struct {
	SKU           string                 `json:"sku"`
	Term          map[string]string      `json:"term"`
	Features      []string               `json:"features"`
	AddOnFeatures []string               `json:"add_on_features"`
	Limits        LicenseLimits          `json:"limits"`
	Deployment    LicenseDeployment      `json:"deployment"`
	SupportTier   string                 `json:"support_tier"`
	Notes         string                 `json:"notes"`
	RawLimits     map[string]any         `json:"-"`
	RawDeployment map[string]any         `json:"-"`
	raw           map[string]interface{} `json:"-"`
}

type LicenseEntitlement struct {
	LicenseID    string              `json:"license_id" yaml:"license_id"`
	CustomerID   string              `json:"customer_id" yaml:"customer_id"`
	CustomerName string              `json:"customer_name" yaml:"customer_name"`
	SKU          string              `json:"sku" yaml:"sku"`
	Deployment   map[string]any      `json:"deployment" yaml:"deployment"`
	Term         LicenseTermInput    `json:"term" yaml:"term"`
	Features     LicenseFeatureInput `json:"features" yaml:"features"`
	Limits       map[string]any      `json:"limits" yaml:"limits"`
	Signing      struct {
		KeyID string `json:"key_id" yaml:"key_id"`
	} `json:"signing" yaml:"signing"`
	Notes string `json:"notes" yaml:"notes"`
}

type LicenseTermInput struct {
	IssuedAt  time.Time `json:"issued_at" yaml:"issued_at"`
	NotBefore time.Time `json:"not_before" yaml:"not_before"`
	ExpiresAt time.Time `json:"expires_at" yaml:"expires_at"`
}

type LicenseFeatureInput struct {
	Include []string `json:"include" yaml:"include"`
	Add     []string `json:"add" yaml:"add"`
	Exclude []string `json:"exclude" yaml:"exclude"`
}

type LicenseRenderOptions struct {
	CatalogPath string
	Now         time.Time
}

type LicenseSafeSummary = licensecontract.SafeSummary

func LoadLicenseSKUCatalog(path string) (LicenseSKUCatalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LicenseSKUCatalog{}, err
	}
	var catalog LicenseSKUCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return LicenseSKUCatalog{}, err
	}
	if catalog.SchemaVersion != 1 {
		return LicenseSKUCatalog{}, fmt.Errorf("license SKU catalog schema_version must be 1")
	}
	for i := range catalog.SKUs {
		var rawSKU map[string]any
		b, _ := json.Marshal(catalog.SKUs[i])
		_ = json.Unmarshal(b, &rawSKU)
		if limits, ok := rawSKU["limits"].(map[string]any); ok {
			catalog.SKUs[i].RawLimits = limits
		}
		if deployment, ok := rawSKU["deployment"].(map[string]any); ok {
			catalog.SKUs[i].RawDeployment = deployment
		}
	}
	return catalog, nil
}

func (c LicenseSKUCatalog) Template(name string) (LicenseSKUTemplate, bool) {
	for _, sku := range c.SKUs {
		if sku.SKU == name {
			return sku, true
		}
	}
	return LicenseSKUTemplate{}, false
}

func LoadLicenseEntitlement(path string) (LicenseEntitlement, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LicenseEntitlement{}, err
	}
	var ent LicenseEntitlement
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".yaml"), strings.HasSuffix(strings.ToLower(path), ".yml"):
		err = yaml.Unmarshal(raw, &ent)
	default:
		err = json.Unmarshal(raw, &ent)
	}
	return ent, err
}

func RenderLicensePayload(ent LicenseEntitlement, catalog LicenseSKUCatalog, opts LicenseRenderOptions) (LicensePayload, error) {
	now := opts.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(ent.SKU) == "" {
		return LicensePayload{}, fmt.Errorf("sku is required")
	}
	tmpl, ok := catalog.Template(ent.SKU)
	if !ok {
		return LicensePayload{}, fmt.Errorf("unknown license SKU %q", ent.SKU)
	}
	limits := tmpl.Limits
	if len(ent.Limits) > 0 {
		if err := mergeStructFromMap(&limits, ent.Limits); err != nil {
			return LicensePayload{}, fmt.Errorf("limits: %w", err)
		}
	}
	deployment := tmpl.Deployment
	if len(ent.Deployment) > 0 {
		if err := mergeStructFromMap(&deployment, ent.Deployment); err != nil {
			return LicensePayload{}, fmt.Errorf("deployment: %w", err)
		}
	}
	issuedAt := ent.Term.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = now
	}
	notBefore := ent.Term.NotBefore.UTC()
	if notBefore.IsZero() {
		notBefore = issuedAt
	}
	expiresAt := ent.Term.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiry, err := templateExpiresAt(notBefore, tmpl.Term["duration"])
		if err != nil {
			return LicensePayload{}, err
		}
		expiresAt = expiry
	}
	graceUntil := time.Time{}
	if grace, err := templateDuration(tmpl.Term["grace"]); err == nil && grace > 0 {
		graceUntil = expiresAt.Add(grace)
	}
	features := renderFeatures(tmpl.Features, ent.Features)
	payload := LicensePayload{
		SchemaVersion: defaultInt(catalog.SchemaVersion, 1),
		LicenseID:     strings.TrimSpace(ent.LicenseID),
		CustomerID:    strings.TrimSpace(ent.CustomerID),
		CustomerName:  strings.TrimSpace(ent.CustomerName),
		Product:       defaultString(catalog.Defaults.Product, licenseProduct),
		SKU:           ent.SKU,
		Features:      features,
		Limits:        limits,
		Deployment:    deployment,
		IssuedAt:      issuedAt,
		NotBefore:     notBefore,
		ExpiresAt:     expiresAt,
		GraceUntil:    graceUntil,
		KeyID:         strings.TrimSpace(ent.Signing.KeyID),
		Issuer:        defaultString(catalog.Defaults.Issuer, licenseIssuer),
		Notes:         strings.TrimSpace(ent.Notes),
	}
	if payload.LicenseID == "" {
		payload.LicenseID = GenerateLicenseID(payload.SKU)
	}
	return payload, nil
}

func GenerateLicenseID(sku string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "lic_" + sanitizeLicenseIDPart(sku) + "_" + time.Now().UTC().Format("20060102150405")
	}
	return "lic_" + sanitizeLicenseIDPart(sku) + "_" + time.Now().UTC().Format("20060102") + "_" + hex.EncodeToString(b[:])
}

func RenewLicensePayload(previous LicensePayload, licenseID string, issuedAt, notBefore, expiresAt time.Time) (LicensePayload, error) {
	out := previous
	out.LicenseID = strings.TrimSpace(licenseID)
	if out.LicenseID == "" || out.LicenseID == previous.LicenseID {
		return LicensePayload{}, fmt.Errorf("renewal requires a new license_id")
	}
	out.IssuedAt = issuedAt.UTC()
	out.NotBefore = notBefore.UTC()
	out.ExpiresAt = expiresAt.UTC()
	return out, nil
}

func TopUpLicensePayload(previous LicensePayload, creditPack LicensePayload, licenseID string) (LicensePayload, error) {
	out := previous
	out.LicenseID = strings.TrimSpace(licenseID)
	if out.LicenseID == "" || out.LicenseID == previous.LicenseID {
		return LicensePayload{}, fmt.Errorf("top-up requires a new license_id")
	}
	out.SKU = creditPack.SKU
	out.IssuedAt = creditPack.IssuedAt
	out.NotBefore = creditPack.NotBefore
	out.ExpiresAt = creditPack.ExpiresAt
	out.GraceUntil = creditPack.GraceUntil
	out.KeyID = creditPack.KeyID
	out.Issuer = creditPack.Issuer
	out.Limits = mergeTopUpLimits(previous.Limits, creditPack.Limits)
	out.Notes = strings.TrimSpace(creditPack.Notes)
	return out, nil
}

func SafeLicenseSummary(payload LicensePayload) LicenseSafeSummary {
	return LicenseSafeSummary{
		SchemaVersion: payload.SchemaVersion,
		LicenseID:     payload.LicenseID,
		CustomerID:    payload.CustomerID,
		CustomerName:  payload.CustomerName,
		Product:       payload.Product,
		SKU:           payload.SKU,
		Features:      append([]string(nil), payload.Features...),
		Limits:        payload.Limits,
		Deployment:    payload.Deployment,
		IssuedAt:      formatLicenseSummaryTime(payload.IssuedAt),
		NotBefore:     formatLicenseSummaryTime(payload.NotBefore),
		ExpiresAt:     formatLicenseSummaryTime(payload.ExpiresAt),
		GraceUntil:    formatLicenseSummaryTime(payload.GraceUntil),
		KeyID:         payload.KeyID,
		Issuer:        payload.Issuer,
	}
}

func mergeTopUpLimits(base, credit LicenseLimits) LicenseLimits {
	out := base
	if credit.MaxTotalTokens > 0 {
		out.MaxTotalTokens = credit.MaxTotalTokens
	}
	if credit.MaxTotalRequests > 0 {
		out.MaxTotalRequests = credit.MaxTotalRequests
	}
	if credit.MaxMonthlyRequests > 0 {
		out.MaxMonthlyRequests = credit.MaxMonthlyRequests
	}
	if credit.WindowTokens > 0 {
		out.WindowTokens = credit.WindowTokens
	}
	if credit.WindowRequests > 0 {
		out.WindowRequests = credit.WindowRequests
	}
	if credit.WindowDurationSeconds > 0 {
		out.WindowDurationSeconds = credit.WindowDurationSeconds
	}
	return out
}

func mergeStructFromMap(dst any, values map[string]any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func renderFeatures(defaults []string, input LicenseFeatureInput) []string {
	var values []string
	if len(input.Include) > 0 {
		values = append(values, input.Include...)
	} else {
		values = append(values, defaults...)
	}
	values = append(values, input.Add...)
	excluded := map[string]bool{}
	for _, feature := range input.Exclude {
		excluded[strings.TrimSpace(feature)] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, feature := range values {
		feature = strings.TrimSpace(feature)
		if feature == "" || excluded[feature] || seen[feature] {
			continue
		}
		seen[feature] = true
		out = append(out, feature)
	}
	sort.Strings(out)
	return out
}

func templateDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "contract" {
		return 0, fmt.Errorf("term.expires_at is required when SKU duration is %q", value)
	}
	if strings.HasSuffix(value, "mo") {
		months := strings.TrimSuffix(value, "mo")
		var n int
		if _, err := fmt.Sscanf(months, "%d", &n); err != nil || n <= 0 {
			return 0, fmt.Errorf("unsupported SKU duration %q", value)
		}
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(value, "d") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSuffix(value, "d"), "%d", &n); err != nil || n < 0 {
			return 0, fmt.Errorf("unsupported SKU duration %q", value)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func templateExpiresAt(notBefore time.Time, value string) (time.Time, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "contract" {
		return time.Time{}, fmt.Errorf("term.expires_at is required when SKU duration is %q", value)
	}
	if strings.HasSuffix(value, "mo") {
		months := strings.TrimSuffix(value, "mo")
		var n int
		if _, err := fmt.Sscanf(months, "%d", &n); err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("unsupported SKU duration %q", value)
		}
		return notBefore.AddDate(0, n, 0), nil
	}
	duration, err := templateDuration(value)
	if err != nil {
		return time.Time{}, err
	}
	return notBefore.Add(duration), nil
}

func sanitizeLicenseIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "license"
	}
	return out
}

func formatLicenseSummaryTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func defaultInt(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}
