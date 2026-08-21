package customerlifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CheckoutSession is the safe scalar response from POST /v1/checkout/sessions.
type CheckoutSession struct {
	CheckoutSessionID string `json:"checkout_session_id"`
	URL               string `json:"url"`
	SKU               string `json:"sku"`
	Mode              string `json:"mode"`
}

// EntitlementStatus is the safe scalar commerce entitlement payload.
type EntitlementStatus struct {
	ID                uint   `json:"id"`
	SKU               string `json:"sku"`
	LicenseTemplate   string `json:"license_template"`
	Status            string `json:"status"`
	FulfillmentStatus string `json:"fulfillment_status,omitempty"`
	FleetCustomerID   string `json:"fleet_customer_id,omitempty"`
	FleetJobID        string `json:"fleet_job_id,omitempty"`
	FleetHostname     string `json:"fleet_hostname,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// CommerceClient talks to metrum-genai-commerce over HTTP.
type CommerceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *CommerceClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// CreateCheckoutSession creates a Stripe Checkout Session via the commerce API.
func (c *CommerceClient) CreateCheckoutSession(ctx context.Context, intent Intent) (CheckoutSession, error) {
	body := map[string]string{
		"sku":            intent.SKU,
		"customer_email": intent.CustomerEmail,
		"customer_alias": intent.CustomerAlias,
	}
	if strings.TrimSpace(intent.SuccessURL) != "" {
		body["success_url"] = intent.SuccessURL
	}
	if strings.TrimSpace(intent.CancelURL) != "" {
		body["cancel_url"] = intent.CancelURL
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return CheckoutSession{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/checkout/sessions", bytes.NewReader(raw))
	if err != nil {
		return CheckoutSession{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("checkout create: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return CheckoutSession{}, fmt.Errorf("checkout create HTTP %d", resp.StatusCode)
	}
	var out CheckoutSession
	if err := json.Unmarshal(payload, &out); err != nil {
		return CheckoutSession{}, fmt.Errorf("checkout create decode: %w", err)
	}
	if strings.TrimSpace(out.CheckoutSessionID) == "" {
		return CheckoutSession{}, fmt.Errorf("checkout create missing checkout_session_id")
	}
	return out, nil
}

// GetEntitlementStatus fetches entitlement status by checkout session id.
func (c *CommerceClient) GetEntitlementStatus(ctx context.Context, checkoutSessionID string) (EntitlementStatus, error) {
	url := fmt.Sprintf("%s/v1/entitlements/status?checkout_session_id=%s", c.BaseURL, checkoutSessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return EntitlementStatus{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return EntitlementStatus{}, fmt.Errorf("entitlement status: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return EntitlementStatus{}, fmt.Errorf("entitlement not found yet")
	}
	if resp.StatusCode >= 300 {
		return EntitlementStatus{}, fmt.Errorf("entitlement status HTTP %d", resp.StatusCode)
	}
	var out EntitlementStatus
	if err := json.Unmarshal(payload, &out); err != nil {
		return EntitlementStatus{}, fmt.Errorf("entitlement status decode: %w", err)
	}
	return out, nil
}

// PaidEntitlementStatuses are commerce statuses that allow license issuance + provision.
var PaidEntitlementStatuses = map[string]bool{
	"active":           true,
	"provision_queued": true,
	"provisioned":      true,
}

// PollUntilPaid polls entitlement status until paid/provision_* or timeout.
func (c *CommerceClient) PollUntilPaid(ctx context.Context, checkoutSessionID string, every, wait time.Duration) (EntitlementStatus, error) {
	if every <= 0 {
		every = 2 * time.Second
	}
	if wait <= 0 {
		wait = 10 * time.Minute
	}
	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		if ctx.Err() != nil {
			return EntitlementStatus{}, ctx.Err()
		}
		status, err := c.GetEntitlementStatus(ctx, checkoutSessionID)
		if err == nil && PaidEntitlementStatuses[strings.TrimSpace(status.Status)] {
			return status, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("entitlement status %q not paid yet", status.Status)
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return EntitlementStatus{}, fmt.Errorf("timed out waiting for paid entitlement: %w", lastErr)
			}
			return EntitlementStatus{}, fmt.Errorf("timed out waiting for paid entitlement")
		}
		select {
		case <-ctx.Done():
			return EntitlementStatus{}, ctx.Err()
		case <-time.After(every):
		}
	}
}
