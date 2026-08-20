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
	SuccessURL    string
	CancelURL     string
	Tolerance     time.Duration
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/checkout/sessions", s.handleCreateCheckout)
	mux.HandleFunc("POST /webhooks/stripe", s.handleWebhook)
	mux.HandleFunc("GET /v1/entitlements/status", s.handleEntitlementStatus)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	success := firstNonEmpty(req.SuccessURL, s.SuccessURL)
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
