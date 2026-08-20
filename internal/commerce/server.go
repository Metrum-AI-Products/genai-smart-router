package commerce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Server is the purchase HTTP API (Checkout + webhooks + entitlement status).
// It must not require router config.yaml / env.json / license.json.
type Server struct {
	Catalog       *Catalog
	Stripe        StripeClient
	Store         *Store
	Fleet         FleetRunner
	WebhookSecret string
	AdminToken    string
	SuccessURL    string
	CancelURL     string
	Tolerance     time.Duration
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /success", s.handleSuccessPage)
	mux.HandleFunc("GET /cancel", s.handleCancelPage)
	mux.HandleFunc("POST /v1/checkout/sessions", s.handleCreateCheckout)
	mux.HandleFunc("POST /webhooks/stripe", s.handleWebhook)
	mux.HandleFunc("GET /v1/entitlements/status", s.handleEntitlementStatus)
	mux.HandleFunc("GET /v1/admin/orders", s.withAdminAuth(s.handleAdminListOrders))
	mux.HandleFunc("GET /v1/admin/entitlements", s.withAdminAuth(s.handleAdminListEntitlements))
	mux.HandleFunc("GET /v1/admin/customers", s.withAdminAuth(s.handleAdminListCustomers))
	mux.HandleFunc("GET /v1/admin/fulfillment-jobs", s.withAdminAuth(s.handleAdminListFulfillmentJobs))
	mux.HandleFunc("POST /v1/admin/fulfillment/{id}/resume", s.withAdminAuth(s.handleAdminResumeFulfillment))
	return mux
}

func (s *Server) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.AdminToken) == "" {
			writeErr(w, http.StatusUnauthorized, "admin-disabled", "commerce admin token is not configured")
			return
		}
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) || strings.TrimSpace(strings.TrimPrefix(auth, prefix)) != s.AdminToken {
			writeErr(w, http.StatusUnauthorized, "admin-unauthorized", "valid commerce admin bearer token required")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleAdminListOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListOrders(adminLimit(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list-failed", "could not list orders")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": rows})
}

func (s *Server) handleAdminListEntitlements(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListEntitlements(adminLimit(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list-failed", "could not list entitlements")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entitlements": rows})
}

func (s *Server) handleAdminListCustomers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListCustomers(adminLimit(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list-failed", "could not list customers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": rows})
}

func (s *Server) handleAdminListFulfillmentJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListFulfillmentJobs(adminLimit(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list-failed", "could not list fulfillment jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fulfillment_jobs": rows})
}

func (s *Server) handleAdminResumeFulfillment(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		writeErr(w, http.StatusBadRequest, "invalid-id", "fulfillment id must be a positive integer")
		return
	}
	job, err := s.Store.ResumeFulfillment(r.Context(), s.Fleet, uint(n))
	if err != nil {
		if job != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":           "resume-failed",
				"message":         "fulfillment resume failed",
				"fulfillment_job": job,
			})
			return
		}
		writeErr(w, http.StatusBadRequest, "resume-failed", "could not resume fulfillment job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fulfillment_job": job})
}

func adminLimit(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 100
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 100
	}
	return n
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSuccessPage(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><title>Payment received</title></head><body>`)
	_, _ = io.WriteString(w, `<h1>Payment received</h1>`)
	_, _ = io.WriteString(w, `<p>Checkout completed. Entitlement is confirmed by the Stripe webhook, not this page.</p>`)
	if sessionID != "" {
		_, _ = io.WriteString(w, `<p>Session: <code>`+htmlEscape(sessionID)+`</code></p>`)
		_, _ = io.WriteString(w, `<p><a href="/v1/entitlements/status?checkout_session_id=`+htmlEscape(sessionID)+`">View entitlement status</a></p>`)
	} else {
		_, _ = io.WriteString(w, `<p><a href="/v1/entitlements/status">Entitlement status API</a> (pass checkout_session_id or customer_alias)</p>`)
	}
	_, _ = io.WriteString(w, `</body></html>`)
}

func (s *Server) handleCancelPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><title>Checkout canceled</title></head><body>`)
	_, _ = io.WriteString(w, `<h1>Checkout canceled</h1><p>No charge was completed. You can start again from the purchase API.</p>`)
	_, _ = io.WriteString(w, `</body></html>`)
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

type createCheckoutRequest struct {
	SKU           string `json:"sku"`
	CustomerEmail string `json:"customer_email"`
	CustomerAlias string `json:"customer_alias"`
	SuccessURL    string `json:"success_url"`
	CancelURL     string `json:"cancel_url"`
}

func (s *Server) handleCreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req createCheckoutRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid-json", "request body must be JSON")
		return
	}
	skuName := strings.TrimSpace(req.SKU)
	if skuName == "" {
		writeErr(w, http.StatusBadRequest, "sku-required", "sku is required")
		return
	}
	priceID, sku, err := ResolvePriceID(r.Context(), s.Stripe, s.Catalog, skuName)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "price-resolve-failed", "sku price could not be resolved against catalog")
		return
	}
	mode, err := sku.CheckoutMode()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "not-self-serve", "sku is not checkout eligible")
		return
	}
	success := withCheckoutSessionID(firstNonEmpty(req.SuccessURL, s.SuccessURL))
	cancel := firstNonEmpty(req.CancelURL, s.CancelURL)
	if success == "" || cancel == "" {
		writeErr(w, http.StatusBadRequest, "urls-required", "success_url and cancel_url are required")
		return
	}
	alias := strings.TrimSpace(req.CustomerAlias)
	clientRef := alias
	if clientRef == "" {
		clientRef = "anon"
	}
	sess, err := s.Stripe.CreateCheckoutSession(r.Context(), CheckoutSessionParams{
		SKU:             sku.SKU,
		PriceID:         priceID,
		Mode:            mode,
		SuccessURL:      success,
		CancelURL:       cancel,
		CustomerEmail:   strings.TrimSpace(req.CustomerEmail),
		CustomerAlias:   alias,
		ClientReference: clientRef,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "checkout-create-failed", "could not create checkout session")
		return
	}
	order := &Order{
		SKU:               sku.SKU,
		LicenseTemplate:   sku.LicenseTemplate,
		StripeMode:        sku.StripeMode,
		CheckoutSessionID: sess.ID,
		StripePriceID:     priceID,
		AmountCents:       sku.Stripe.Price.UnitAmount,
		Currency:          strings.ToLower(sku.Stripe.Price.Currency),
		Status:            OrderStatusCreated,
		CustomerEmail:     strings.TrimSpace(req.CustomerEmail),
		CustomerAlias:     alias,
	}
	if err := s.Store.CreateOrder(order); err != nil {
		writeErr(w, http.StatusInternalServerError, "order-persist-failed", "could not persist order")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"checkout_session_id": sess.ID,
		"url":                 sess.URL,
		"sku":                 sku.SKU,
		"mode":                mode,
	})
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "body-read-failed", "could not read body")
		return
	}
	tol := s.Tolerance
	if tol == 0 {
		tol = 5 * time.Minute
	}
	if err := VerifyWebhookSignature(payload, r.Header.Get("Stripe-Signature"), s.WebhookSecret, tol); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid-signature", "webhook signature verification failed")
		return
	}
	evt, err := ParseWebhookEvent(payload)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid-event", "could not parse event")
		return
	}

	switch evt.Type {
	case "checkout.session.completed":
		session, err := ParseCheckoutSession(evt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid-session", "could not parse checkout session")
			return
		}
		skuName := session.Metadata["sku"]
		if skuName == "" {
			writeErr(w, http.StatusBadRequest, "sku-missing", "checkout metadata.sku is required")
			return
		}
		if err := s.validateSessionAgainstCatalog(r.Context(), skuName, session); err != nil {
			writeErr(w, http.StatusBadRequest, "catalog-mismatch", "price does not match catalog")
			return
		}
		ent, dup, err := s.Store.ProcessPaidCheckout(r.Context(), s.Catalog, s.Fleet, evt.ID, session, skuName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "process-failed", "could not process checkout event")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"duplicate":      dup,
			"entitlement_id": entitlementID(ent),
			"event_id":       evt.ID,
		})
	case "invoice.paid":
		inv, err := ParseInvoice(evt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid-invoice", "could not parse invoice")
			return
		}
		skuName := inv.Metadata["sku"]
		if skuName == "" {
			writeErr(w, http.StatusBadRequest, "sku-missing", "invoice metadata.sku is required")
			return
		}
		if _, _, err := ResolvePriceID(r.Context(), s.Stripe, s.Catalog, skuName); err != nil {
			writeErr(w, http.StatusBadRequest, "catalog-mismatch", "sku price does not match catalog")
			return
		}
		ent, dup, err := s.Store.ProcessPaidInvoice(r.Context(), s.Catalog, s.Fleet, evt.ID, inv, skuName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "process-failed", "could not process invoice event")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"duplicate":      dup,
			"entitlement_id": entitlementID(ent),
			"event_id":       evt.ID,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": evt.Type})
	}
}

func (s *Server) validateSessionAgainstCatalog(ctx context.Context, skuName string, session *CheckoutSessionObject) error {
	sku, ok := s.Catalog.LookupSKU(skuName)
	if !ok || !sku.SelfServeStripe {
		return fmt.Errorf("unknown or non-self-serve sku")
	}
	priceID, _, err := ResolvePriceID(ctx, s.Stripe, s.Catalog, skuName)
	if err != nil {
		return err
	}
	if session.LineItemsPriceID != "" && session.LineItemsPriceID != priceID {
		return fmt.Errorf("session price_id disagrees with catalog")
	}
	if session.AmountTotal > 0 && session.AmountTotal != sku.Stripe.Price.UnitAmount {
		return fmt.Errorf("session amount disagrees with catalog")
	}
	return nil
}

func (s *Server) handleEntitlementStatus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var id uint
	if raw := q.Get("id"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid-id", "id must be an integer")
			return
		}
		id = uint(n)
	}
	status, err := s.Store.GetEntitlementStatus(id, q.Get("checkout_session_id"), q.Get("customer_alias"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not-found", "entitlement not found")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func entitlementID(ent *Entitlement) any {
	if ent == nil {
		return nil
	}
	return ent.ID
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

// withCheckoutSessionID ensures Stripe can append the completed session id on redirect.
func withCheckoutSessionID(successURL string) string {
	u := strings.TrimSpace(successURL)
	if u == "" {
		return u
	}
	if strings.Contains(u, "{CHECKOUT_SESSION_ID}") {
		return u
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "session_id={CHECKOUT_SESSION_ID}"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
