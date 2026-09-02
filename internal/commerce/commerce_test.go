package commerce_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"smart-llmrouter/internal/commerce"
)

func testCatalog(t *testing.T) *commerce.Catalog {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "enterprise-license-skus.json")
	cat, err := commerce.LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	return cat
}

func TestCatalogSchemaSelfServe(t *testing.T) {
	cat := testCatalog(t)
	for _, sku := range cat.SelfServeSKUs() {
		if sku.Stripe == nil {
			t.Fatalf("%s missing stripe", sku.SKU)
		}
		if sku.Stripe.Price.LookupKey != sku.SKU {
			t.Fatalf("%s lookup_key != sku", sku.SKU)
		}
		want := int64(sku.PricePlaceholder.AmountUSD*100 + 0.5)
		if sku.Stripe.Price.UnitAmount != want {
			t.Fatalf("%s unit_amount %d != cents from amount_usd (%d)", sku.SKU, sku.Stripe.Price.UnitAmount, want)
		}
		if !sku.SelfServeStripe {
			t.Fatalf("%s expected self_serve", sku.SKU)
		}
	}
	ent, ok := cat.LookupSKU("enterprise-annual")
	if !ok || ent.SelfServeStripe {
		t.Fatal("enterprise-annual must not be self-serve")
	}
	pilot, ok := cat.LookupSKU("hosted-pilot-30d")
	if !ok || pilot.LicenseTemplate != "pilot-30d" {
		t.Fatal("hosted-pilot-30d must map to pilot-30d template")
	}
	market, ok := cat.LookupSKU("marketplace-seat")
	if !ok || market.SelfServeStripe || market.StripeMode != commerce.StripeModeNone {
		t.Fatal("marketplace-seat must be invoice_only without stripe")
	}
}

func TestCatalogValidateSelfManagedNoBilling(t *testing.T) {
	cat := &commerce.Catalog{
		SchemaVersion: 1,
		SKUs: []commerce.SKU{
			{
				SKU:              "oss-self-managed",
				LicenseTemplate:  "oss-self-managed",
				CommercialMotion: "self-managed",
				BillingKind:      commerce.BillingKindNone,
				PricingStatus:    commerce.PricingStatusNotApplicable,
			},
		},
	}

	if err := cat.Validate(); err != nil {
		t.Fatalf("self-managed SKU without billing must be valid: %v", err)
	}
	if got := cat.SelfServeSKUs(); len(got) != 0 {
		t.Fatalf("self-managed SKU must not be offered through Stripe: %+v", got)
	}
}

func TestFakeStripePlanApplyIdempotentAndAmountChange(t *testing.T) {
	cat := testCatalog(t)
	client := commerce.NewFakeStripeClient()
	ctx := context.Background()

	plan, err := commerce.PlanCatalog(ctx, client, cat, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Drift == 0 {
		t.Fatal("expected drift before apply")
	}
	after, err := commerce.ApplyCatalog(ctx, client, cat, false)
	if err != nil {
		t.Fatal(err)
	}
	if after.Drift != 0 {
		t.Fatalf("expected zero drift after apply, got %d: %+v", after.Drift, after.Actions)
	}
	again, err := commerce.ApplyCatalog(ctx, client, cat, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Drift != 0 {
		t.Fatalf("idempotent re-apply should stay clean, drift=%d", again.Drift)
	}

	sku, _ := cat.LookupSKU("eval-72h")
	sku.PricePlaceholder.AmountUSD = 10
	sku.Stripe.Price.UnitAmount = 1000
	for i := range cat.SKUs {
		if cat.SKUs[i].SKU == "eval-72h" {
			cat.SKUs[i] = sku
			break
		}
	}
	changed, err := commerce.PlanCatalog(ctx, client, cat, false)
	if err != nil {
		t.Fatal(err)
	}
	foundReplace := false
	for _, a := range changed.Actions {
		if a.SKU == "eval-72h" && a.Action == commerce.ActionReplacePrice {
			foundReplace = true
		}
	}
	if !foundReplace {
		t.Fatalf("expected replace_price for eval-72h, got %+v", changed.Actions)
	}
	oldPrice := client.ActivePriceBySKU("eval-72h")
	if oldPrice == nil {
		t.Fatal("missing old price")
	}
	oldID := oldPrice.ID
	if _, err := commerce.ApplyCatalog(ctx, client, cat, false); err != nil {
		t.Fatal(err)
	}
	newPrice := client.ActivePriceBySKU("eval-72h")
	if newPrice == nil || newPrice.ID == oldID {
		t.Fatal("expected new price id after amount change")
	}
	if newPrice.UnitAmount != 1000 {
		t.Fatalf("expected unit_amount 1000, got %d", newPrice.UnitAmount)
	}
}

func openTestStore(t *testing.T) *commerce.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:commerce-test-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := commerce.OpenStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestWebhookSignatureAndReplayIdempotency(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_test_1","type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","payment_status":"paid","metadata":{"sku":"eval-72h","price_id":"price_fake_1","customer_alias":"buyer-a"},"amount_total":0,"currency":"usd","client_reference_id":"buyer-a"}}}`)
	header := commerce.SignWebhookPayload(payload, secret, time.Now())
	if err := commerce.VerifyWebhookSignature(payload, header, secret, 5*time.Minute); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := commerce.VerifyWebhookSignature(payload, "t=1,v1=deadbeef", secret, 5*time.Minute); err == nil {
		t.Fatal("expected signature failure")
	}

	cat := testCatalog(t)
	client := commerce.NewFakeStripeClient()
	ctx := context.Background()
	if _, err := commerce.ApplyCatalog(ctx, client, cat, false); err != nil {
		t.Fatal(err)
	}
	// Align metadata price_id with created fake price.
	price := client.ActivePriceBySKU("eval-72h")
	if price == nil {
		t.Fatal("missing eval price")
	}
	payload = []byte(`{"id":"evt_test_1","type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","payment_status":"paid","metadata":{"sku":"eval-72h","price_id":"` + price.ID + `","customer_alias":"buyer-a"},"amount_total":0,"currency":"usd","client_reference_id":"buyer-a"}}}`)
	header = commerce.SignWebhookPayload(payload, secret, time.Now())

	fleet := commerce.NewFakeFleetRunner()
	store := openTestStore(t)
	srv := &commerce.Server{
		Catalog:       cat,
		Stripe:        client,
		Store:         store,
		Fleet:         fleet,
		WebhookSecret: secret,
		Tolerance:     5 * time.Minute,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", header)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first webhook status %d body %s", rr.Code, rr.Body.String())
	}
	if fleet.CallCount() != 1 {
		t.Fatalf("expected one fleet call, got %d", fleet.CallCount())
	}
	status, err := store.GetEntitlementStatus(0, "cs_test_1", "")
	if err != nil {
		t.Fatalf("entitlement status: %v", err)
	}
	if status.Status != commerce.EntitlementStatusProvisioned {
		t.Fatalf("expected provisioned entitlement, got %s", status.Status)
	}
	if status.FulfillmentStatus != commerce.FulfillmentStatusSucceeded {
		t.Fatalf("expected fulfillment succeeded, got %s", status.FulfillmentStatus)
	}
	if status.FleetCustomerID != "buyer-a" {
		t.Fatalf("expected sanitized fleet customer buyer-a, got %q", status.FleetCustomerID)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req2.Header.Set("Stripe-Signature", commerce.SignWebhookPayload(payload, secret, time.Now()))
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("replay status %d body %s", rr2.Code, rr2.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rr2.Body.Bytes(), &body)
	if body["duplicate"] != true {
		t.Fatalf("expected duplicate=true, got %v", body)
	}
	if fleet.CallCount() != 1 {
		t.Fatalf("replay must not call fleet again, got %d", fleet.CallCount())
	}
}

func TestUnknownSKUAndNoAutoProvisionSkipsFleet(t *testing.T) {
	cat := testCatalog(t)
	client := commerce.NewFakeStripeClient()
	ctx := context.Background()
	if _, err := commerce.ApplyCatalog(ctx, client, cat, false); err != nil {
		t.Fatal(err)
	}
	fleet := commerce.NewFakeFleetRunner()
	store := openTestStore(t)

	session := &commerce.CheckoutSessionObject{
		ID: "cs_credit",
		Metadata: map[string]string{
			"sku":            "credit-pack-5m",
			"customer_alias": "topup-user",
		},
		ClientReferenceID: "topup-user",
		PaymentStatus:     "paid",
		AmountTotal:       25000,
		Currency:          "usd",
	}
	ent, _, err := store.ProcessPaidCheckout(ctx, cat, fleet, "evt_credit_1", session, "credit-pack-5m")
	if err != nil {
		t.Fatal(err)
	}
	if ent == nil {
		t.Fatal("expected entitlement")
	}
	if fleet.CallCount() != 0 {
		t.Fatalf("credit pack must not auto-provision, calls=%d", fleet.CallCount())
	}

	_, _, err = store.ProcessPaidCheckout(ctx, cat, fleet, "evt_unknown", &commerce.CheckoutSessionObject{
		ID: "cs_x", Metadata: map[string]string{"sku": "no-such-sku"},
	}, "no-such-sku")
	if err == nil {
		t.Fatal("expected unknown sku error")
	}
	if fleet.CallCount() != 0 {
		t.Fatal("unknown sku must not call fleet")
	}
}

func TestFakeFleetRunner(t *testing.T) {
	fleet := commerce.NewFakeFleetRunner()
	res, err := fleet.Bootstrap(context.Background(), commerce.FleetBootstrapRequest{
		CustomerID: "cust-1", SKU: "eval-72h", LicenseTemplate: "eval-72h",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CustomerID != "cust-1" || res.JobID == "" {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestSanitizeFleetCustomerID(t *testing.T) {
	if got := commerce.SanitizeFleetCustomerID("Buyer_A!", 9); got != "buyer-a" {
		t.Fatalf("got %q", got)
	}
	if got := commerce.SanitizeFleetCustomerID("", 42); got != "c42" {
		t.Fatalf("empty alias got %q", got)
	}
	if got := commerce.SanitizeFleetCustomerID("anon", 7); got != "c7" {
		t.Fatalf("anon got %q", got)
	}
}

func TestShellFleetRunnerRequiresRefs(t *testing.T) {
	runner := &commerce.ShellFleetRunner{}
	_, err := runner.Bootstrap(context.Background(), commerce.FleetBootstrapRequest{CustomerID: "c1"})
	if err == nil {
		t.Fatal("expected missing refs error")
	}
}

func TestAdminAuthListAndResume(t *testing.T) {
	cat := testCatalog(t)
	client := commerce.NewFakeStripeClient()
	ctx := context.Background()
	if _, err := commerce.ApplyCatalog(ctx, client, cat, false); err != nil {
		t.Fatal(err)
	}
	price := client.ActivePriceBySKU("eval-72h")
	if price == nil {
		t.Fatal("missing eval price")
	}
	fleet := commerce.NewFakeFleetRunner()
	store := openTestStore(t)
	adminToken := "test-admin-token"
	srv := &commerce.Server{
		Catalog:       cat,
		Stripe:        client,
		Store:         store,
		Fleet:         fleet,
		WebhookSecret: "whsec_admin_test",
		AdminToken:    adminToken,
		Tolerance:     5 * time.Minute,
	}
	h := srv.Handler()

	deny := httptest.NewRequest(http.MethodGet, "/v1/admin/orders", nil)
	drr := httptest.NewRecorder()
	h.ServeHTTP(drr, deny)
	if drr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", drr.Code)
	}

	payload := []byte(`{"id":"evt_admin_1","type":"checkout.session.completed","data":{"object":{"id":"cs_admin_1","payment_status":"paid","metadata":{"sku":"eval-72h","price_id":"` + price.ID + `","customer_alias":"admin-buyer"},"amount_total":0,"currency":"usd","client_reference_id":"admin-buyer","customer_email":"buyer@example.test"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", commerce.SignWebhookPayload(payload, "whsec_admin_test", time.Now()))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook %d %s", rr.Code, rr.Body.String())
	}

	auth := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{
		"/v1/admin/orders",
		"/v1/admin/entitlements",
		"/v1/admin/customers",
		"/v1/admin/fulfillment-jobs",
	} {
		w := auth(http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status %d body %s", path, w.Code, w.Body.String())
		}
	}

	jobs, err := store.ListFulfillmentJobs(10)
	if err != nil || len(jobs) == 0 {
		t.Fatalf("expected fulfillment job, err=%v len=%d", err, len(jobs))
	}
	// Force failed so resume has work to do.
	_ = store.DB.Model(&jobs[0]).Updates(map[string]any{
		"status": commerce.FulfillmentStatusFailed, "last_error_class": "forced",
	}).Error
	_ = store.DB.Model(&commerce.Entitlement{}).Where("id = ?", jobs[0].EntitlementID).Update("status", commerce.EntitlementStatusProvisionFailed).Error

	resume := auth(http.MethodPost, "/v1/admin/fulfillment/"+itoa(jobs[0].ID)+"/resume")
	if resume.Code != http.StatusOK {
		t.Fatalf("resume status %d body %s", resume.Code, resume.Body.String())
	}
	if fleet.CallCount() < 2 {
		t.Fatalf("expected resume to call fleet again, calls=%d", fleet.CallCount())
	}
	status, err := store.GetEntitlementStatus(jobs[0].EntitlementID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != commerce.EntitlementStatusProvisioned || status.FulfillmentStatus != commerce.FulfillmentStatusSucceeded {
		t.Fatalf("after resume got entitlement=%s fulfillment=%s", status.Status, status.FulfillmentStatus)
	}
}

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}
