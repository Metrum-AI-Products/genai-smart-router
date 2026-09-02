// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifyWebhookSignature validates a Stripe-Signature header against the payload.
// It implements the Stripe webhook signing scheme without requiring network calls.
func VerifyWebhookSignature(payload []byte, header, secret string, tolerance time.Duration) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("webhook secret is required")
	}
	if strings.TrimSpace(header) == "" {
		return fmt.Errorf("Stripe-Signature header is required")
	}
	parts := strings.Split(header, ",")
	var timestamp string
	var signatures []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "t=") {
			timestamp = strings.TrimPrefix(part, "t=")
		}
		if strings.HasPrefix(part, "v1=") {
			signatures = append(signatures, strings.TrimPrefix(part, "v1="))
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("malformed Stripe-Signature header")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid webhook timestamp")
	}
	if tolerance > 0 {
		age := time.Since(time.Unix(ts, 0))
		if age > tolerance || age < -tolerance {
			return fmt.Errorf("webhook timestamp outside tolerance")
		}
	}
	signed := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, sig := range signatures {
		if hmac.Equal([]byte(expected), []byte(sig)) {
			return nil
		}
	}
	return fmt.Errorf("webhook signature mismatch")
}

// SignWebhookPayload builds a Stripe-Signature header for tests.
func SignWebhookPayload(payload []byte, secret string, ts time.Time) string {
	signed := fmt.Sprintf("%d.%s", ts.Unix(), string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

// StripeWebhookEvent is a minimal event envelope.
type StripeWebhookEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// CheckoutSessionObject is the subset of checkout.session fields we need.
type CheckoutSessionObject struct {
	ID                string            `json:"id"`
	Mode              string            `json:"mode"`
	PaymentStatus     string            `json:"payment_status"`
	Customer          string            `json:"customer"`
	CustomerEmail     string            `json:"customer_email"`
	ClientReferenceID string            `json:"client_reference_id"`
	PaymentIntent     string            `json:"payment_intent"`
	Subscription      string            `json:"subscription"`
	Metadata          map[string]string `json:"metadata"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	LineItemsPriceID  string            `json:"-"`
}

// InvoiceObject is the subset of invoice.paid fields we need.
type InvoiceObject struct {
	ID            string            `json:"id"`
	Customer      string            `json:"customer"`
	Subscription  string            `json:"subscription"`
	Paid          bool              `json:"paid"`
	AmountPaid    int64             `json:"amount_paid"`
	Currency      string            `json:"currency"`
	Metadata      map[string]string `json:"metadata"`
	BillingReason string            `json:"billing_reason"`
	LinesPriceIDs []string          `json:"-"`
}

type stripeDataObject struct {
	Object json.RawMessage `json:"object"`
}

// ParseWebhookEvent decodes a Stripe event envelope.
func ParseWebhookEvent(payload []byte) (*StripeWebhookEvent, error) {
	var evt StripeWebhookEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return nil, fmt.Errorf("parse event: %w", err)
	}
	if strings.TrimSpace(evt.ID) == "" || strings.TrimSpace(evt.Type) == "" {
		return nil, fmt.Errorf("event id and type are required")
	}
	return &evt, nil
}

// ParseCheckoutSession extracts checkout.session object from event data.
func ParseCheckoutSession(evt *StripeWebhookEvent) (*CheckoutSessionObject, error) {
	var wrap stripeDataObject
	if err := json.Unmarshal(evt.Data, &wrap); err != nil {
		return nil, err
	}
	var session CheckoutSessionObject
	if err := json.Unmarshal(wrap.Object, &session); err != nil {
		return nil, err
	}
	// Prefer metadata.price_id / line item expansion is not always present; callers may set LineItemsPriceID.
	if session.Metadata == nil {
		session.Metadata = map[string]string{}
	}
	if priceID := session.Metadata["price_id"]; priceID != "" {
		session.LineItemsPriceID = priceID
	}
	return &session, nil
}

// ParseInvoice extracts invoice object from event data.
func ParseInvoice(evt *StripeWebhookEvent) (*InvoiceObject, error) {
	var wrap stripeDataObject
	if err := json.Unmarshal(evt.Data, &wrap); err != nil {
		return nil, err
	}
	var inv InvoiceObject
	if err := json.Unmarshal(wrap.Object, &inv); err != nil {
		return nil, err
	}
	if inv.Metadata == nil {
		inv.Metadata = map[string]string{}
	}
	return &inv, nil
}
