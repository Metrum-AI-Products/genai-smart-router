// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLicenseSKURenderAndValidation(t *testing.T) {
	pub, _, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadLicenseSKUCatalog(filepath.Join("..", "..", "docs", "enterprise-license-skus.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.SKUs) == 0 {
		t.Fatal("expected SKU templates")
	}
	for _, sku := range catalog.SKUs {
		t.Run(sku.SKU, func(t *testing.T) {
			now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
			ent := LicenseEntitlement{
				LicenseID:  "lic_" + strings.ReplaceAll(sku.SKU, "-", "_") + "_test",
				CustomerID: "cust_test",
				SKU:        sku.SKU,
			}
			ent.Signing.KeyID = "test-license-key"
			ent.Term.IssuedAt = now
			ent.Term.NotBefore = now
			if sku.Term["duration"] == "contract" {
				ent.Term.ExpiresAt = now.Add(365 * 24 * time.Hour)
			}
			payload, err := RenderLicensePayload(ent, catalog, LicenseRenderOptions{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			if payload.Product != LicenseProduct || payload.Issuer != LicenseIssuer {
				t.Fatalf("payload identity=%s/%s", payload.Product, payload.Issuer)
			}
			if err := ValidateLicensePayload(payload, nil, []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}, now); err != nil {
				t.Fatalf("rendered payload did not validate: %v", err)
			}
		})
	}
}

func TestValidateLicensePayloadForIssuanceRejectsBadInputs(t *testing.T) {
	pub, _, err := GenerateLicenseKeypair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	keys := []LicensePublicKey{{KeyID: "test-license-key", Algorithm: "ed25519", PublicKey: pub}}
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	if err := ValidateLicensePayload(payload, nil, keys, now); err != nil {
		t.Fatalf("baseline payload rejected: %v", err)
	}

	bad := payload
	bad.Features = []string{"routing", "not_a_feature"}
	if err := ValidateLicensePayload(bad, nil, keys, now); err == nil || !strings.Contains(err.Error(), "unsupported feature") {
		t.Fatalf("bad feature err=%v", err)
	}

	bad = payload
	bad.Limits.AllowedSkins = []string{"openai-chat", "bad-skin"}
	if err := ValidateLicensePayload(bad, nil, keys, now); err == nil || !strings.Contains(err.Error(), "allowed_skins") {
		t.Fatalf("bad skin err=%v", err)
	}

	bad = payload
	bad.Limits.WindowTokens = 100
	bad.Limits.WindowDurationSeconds = 0
	if err := ValidateLicensePayload(bad, nil, keys, now); err == nil || !strings.Contains(err.Error(), "window duration") {
		t.Fatalf("bad window err=%v", err)
	}

	bad = payload
	bad.ExpiresAt = bad.NotBefore
	if err := ValidateLicensePayload(bad, nil, keys, now); err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Fatalf("bad dates err=%v", err)
	}

	if err := ValidateLicensePayload(payload, nil, []LicensePublicKey{{KeyID: "other", Algorithm: "ed25519", PublicKey: pub}}, now); err == nil || !strings.Contains(err.Error(), "unknown license key") {
		t.Fatalf("unknown key err=%v", err)
	}
}

func TestSafeSummaryAndRenewalHelpers(t *testing.T) {
	now := time.Now().UTC()
	payload := testLicensePayload(now, []string{LicenseFeatureRouting})
	summary := SafeLicenseSummary(payload)
	if summary.LicenseID != payload.LicenseID || summary.KeyID != payload.KeyID {
		t.Fatalf("summary mismatch: %#v", summary)
	}
	raw := strings.Join([]string{summary.LicenseID, summary.CustomerID, summary.KeyID}, " ")
	if strings.Contains(raw, "value_base64") || strings.Contains(raw, "private") {
		t.Fatalf("summary leaked unsafe fields: %s", raw)
	}
	renewed, err := RenewLicensePayload(payload, "lic_test_renewed", now, now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if renewed.LicenseID == payload.LicenseID || renewed.Features[0] != payload.Features[0] {
		t.Fatalf("renewal did not preserve expected fields: %#v", renewed)
	}
	if _, err := RenewLicensePayload(payload, payload.LicenseID, now, now, now.Add(48*time.Hour)); err == nil {
		t.Fatal("renewal accepted reused license_id")
	}
}

func TestLicenseSKUMonthDurationUsesCalendarMonths(t *testing.T) {
	catalog, err := LoadLicenseSKUCatalog(filepath.Join("..", "..", "docs", "enterprise-license-skus.json"))
	if err != nil {
		t.Fatal(err)
	}
	notBefore := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	ent := LicenseEntitlement{
		LicenseID:  "lic_enterprise_calendar_months",
		CustomerID: "cust_calendar",
		SKU:        "enterprise-annual",
	}
	ent.Signing.KeyID = "test-license-key"
	ent.Term.IssuedAt = notBefore
	ent.Term.NotBefore = notBefore
	payload, err := RenderLicensePayload(ent, catalog, LicenseRenderOptions{Now: notBefore})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2027, 6, 28, 0, 0, 0, 0, time.UTC)
	if !payload.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at=%s want %s", payload.ExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestTopUpLicensePayloadPreservesExistingEntitlements(t *testing.T) {
	now := time.Now().UTC()
	previous := testLicensePayload(now, []string{LicenseFeatureRouting, LicenseFeaturePrivateUpstreams})
	previous.SKU = "enterprise-annual"
	previous.Limits.MaxCallers = 500
	previous.Limits.MaxModelGroups = 200
	previous.Limits.MaxTotalTokens = 0
	previous.Deployment.Mode = "self_hosted"
	previous.Deployment.AllowedEnvironments = []string{"production", "staging"}

	credit := previous
	credit.SKU = "credit-pack-5m"
	credit.LicenseID = "lic_credit_pack"
	credit.Features = []string{LicenseFeatureRouting, LicenseFeatureUsageReporting}
	credit.Limits = LicenseLimits{MaxTotalTokens: 5_000_000, MaxTotalRequests: 100_000, MaxCallers: 50, MaxModelGroups: 20}

	toppedUp, err := TopUpLicensePayload(previous, credit, "lic_enterprise_topup")
	if err != nil {
		t.Fatal(err)
	}
	if toppedUp.SKU != "credit-pack-5m" || toppedUp.LicenseID != "lic_enterprise_topup" {
		t.Fatalf("top-up identity mismatch: %#v", toppedUp)
	}
	if strings.Join(toppedUp.Features, ",") != strings.Join(previous.Features, ",") {
		t.Fatalf("features not preserved: got %#v want %#v", toppedUp.Features, previous.Features)
	}
	if toppedUp.Limits.MaxCallers != 500 || toppedUp.Limits.MaxModelGroups != 200 {
		t.Fatalf("contract limits not preserved: %#v", toppedUp.Limits)
	}
	if toppedUp.Limits.MaxTotalTokens != 5_000_000 || toppedUp.Limits.MaxTotalRequests != 100_000 {
		t.Fatalf("credit volume limits not applied: %#v", toppedUp.Limits)
	}
	if toppedUp.Deployment.Mode != previous.Deployment.Mode || len(toppedUp.Deployment.AllowedEnvironments) != len(previous.Deployment.AllowedEnvironments) {
		t.Fatalf("deployment not preserved: %#v", toppedUp.Deployment)
	}
}
