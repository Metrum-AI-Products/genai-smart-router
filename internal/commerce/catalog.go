// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// Package commerce implements Stripe catalog sync, purchase checkout, webhook
// entitlement recording, and Fleet fulfillment hooks for epic #921.
package commerce

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

const (
	PricingStatusPlaceholder   = "placeholder_assumption"
	PricingStatusNotApplicable = "not-applicable"

	BillingKindNone              = "none"
	BillingKindPayment           = "payment"
	BillingKindTopUp             = "top_up"
	BillingKindSubscription      = "subscription"
	BillingKindSubscriptionAddon = "subscription_addon"
	BillingKindInvoiceOnly       = "invoice_only"

	StripeModePayment      = "payment"
	StripeModeSubscription = "subscription"
	StripeModeNone         = "none"

	AutoProvisionFirstPaidOnly = "first_paid_only"
	AutoProvisionEachPaid      = "each_paid"
)

// Catalog is the checked-in enterprise license / commerce SKU document.
type Catalog struct {
	SchemaVersion int            `json:"schema_version"`
	Metadata      map[string]any `json:"metadata"`
	SKUs          []SKU          `json:"skus"`
}

// SKU is one commercial offer plus license template mapping.
type SKU struct {
	SKU                   string           `json:"sku"`
	LicenseTemplate       string           `json:"license_template"`
	CommercialMotion      string           `json:"commercial_motion"`
	BillingKind           string           `json:"billing_kind"`
	StripeMode            string           `json:"stripe_mode"`
	SelfServeStripe       bool             `json:"self_serve_stripe"`
	AutoProvisionInstance bool             `json:"auto_provision_instance"`
	AutoProvisionOn       string           `json:"auto_provision_on,omitempty"`
	PricingStatus         string           `json:"pricing_status"`
	PricePlaceholder      PricePlaceholder `json:"price_placeholder"`
	Stripe                *StripeSKU       `json:"stripe,omitempty"`
	Notes                 string           `json:"notes,omitempty"`
}

// PricePlaceholder holds engineering-assumption USD pricing (not for docs-site).
type PricePlaceholder struct {
	Currency        string  `json:"currency"`
	AmountUSD       float64 `json:"amount_usd"`
	Unit            string  `json:"unit"`
	Interval        string  `json:"interval"`
	AssumptionNotes string  `json:"assumption_notes"`
}

// StripeSKU maps a self-serve SKU onto Stripe Product + Price desired state.
type StripeSKU struct {
	Product StripeProduct `json:"product"`
	Price   StripePrice   `json:"price"`
}

// StripeProduct is desired Product metadata.
type StripeProduct struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// StripePrice is desired Price metadata. LookupKey must equal the SKU.
type StripePrice struct {
	LookupKey  string           `json:"lookup_key"`
	UnitAmount int64            `json:"unit_amount"`
	Currency   string           `json:"currency"`
	Recurring  *StripeRecurring `json:"recurring,omitempty"`
}

// StripeRecurring describes subscription intervals.
type StripeRecurring struct {
	Interval      string `json:"interval"`
	IntervalCount int64  `json:"interval_count"`
}

// LoadCatalog reads and validates a commerce SKU catalog JSON file.
func LoadCatalog(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var cat Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if err := cat.Validate(); err != nil {
		return nil, err
	}
	return &cat, nil
}

// Validate checks commercial Stripe fields and license template aliases.
func (c *Catalog) Validate() error {
	if c == nil {
		return fmt.Errorf("catalog is nil")
	}
	if c.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if len(c.SKUs) == 0 {
		return fmt.Errorf("skus must be non-empty")
	}
	canonical := map[string]struct{}{}
	bySKU := map[string]SKU{}
	for _, sku := range c.SKUs {
		name := strings.TrimSpace(sku.SKU)
		if name == "" {
			return fmt.Errorf("sku name is required")
		}
		if _, exists := bySKU[name]; exists {
			return fmt.Errorf("duplicate sku %q", name)
		}
		bySKU[name] = sku
		if sku.LicenseTemplate == name {
			canonical[name] = struct{}{}
		}
	}
	for _, sku := range c.SKUs {
		if err := validateSKU(sku, canonical); err != nil {
			return err
		}
	}
	return nil
}

func validateSKU(sku SKU, canonical map[string]struct{}) error {
	name := sku.SKU
	if strings.TrimSpace(sku.LicenseTemplate) == "" {
		return fmt.Errorf("%s: license_template is required", name)
	}
	if _, ok := canonical[sku.LicenseTemplate]; !ok {
		return fmt.Errorf("%s: license_template %q must resolve to a canonical template SKU", name, sku.LicenseTemplate)
	}
	switch sku.BillingKind {
	case BillingKindNone, BillingKindPayment, BillingKindTopUp, BillingKindSubscription, BillingKindSubscriptionAddon, BillingKindInvoiceOnly:
	default:
		return fmt.Errorf("%s: invalid billing_kind %q", name, sku.BillingKind)
	}
	if sku.BillingKind == BillingKindNone {
		if sku.CommercialMotion != "self-managed" {
			return fmt.Errorf("%s: billing_kind none requires commercial_motion self-managed", name)
		}
		if sku.StripeMode != "" || sku.SelfServeStripe || sku.AutoProvisionInstance || sku.Stripe != nil {
			return fmt.Errorf("%s: billing_kind none must not declare commerce fulfillment fields", name)
		}
		if sku.PricingStatus != PricingStatusNotApplicable || sku.PricePlaceholder != (PricePlaceholder{}) {
			return fmt.Errorf("%s: billing_kind none requires pricing_status not-applicable without price_placeholder", name)
		}
		return nil
	}
	switch sku.StripeMode {
	case StripeModePayment, StripeModeSubscription, StripeModeNone:
	default:
		return fmt.Errorf("%s: invalid stripe_mode %q", name, sku.StripeMode)
	}
	if sku.PricingStatus != PricingStatusPlaceholder {
		return fmt.Errorf("%s: pricing_status must be %s", name, PricingStatusPlaceholder)
	}
	if strings.TrimSpace(sku.PricePlaceholder.Currency) == "" {
		return fmt.Errorf("%s: price_placeholder.currency is required", name)
	}
	if sku.PricePlaceholder.AmountUSD < 0 {
		return fmt.Errorf("%s: price_placeholder.amount_usd must be non-negative", name)
	}
	if !sku.SelfServeStripe {
		if sku.Stripe != nil && (sku.Stripe.Product.Name != "" || sku.Stripe.Price.LookupKey != "") {
			return fmt.Errorf("%s: non-self-serve SKUs must not declare stripe product/price", name)
		}
		return nil
	}
	if sku.StripeMode == StripeModeNone {
		return fmt.Errorf("%s: self_serve_stripe requires stripe_mode payment or subscription", name)
	}
	if sku.Stripe == nil {
		return fmt.Errorf("%s: self_serve SKUs require stripe object", name)
	}
	if strings.TrimSpace(sku.Stripe.Product.Name) == "" {
		return fmt.Errorf("%s: stripe.product.name is required", name)
	}
	if sku.Stripe.Price.LookupKey != name {
		return fmt.Errorf("%s: stripe.price.lookup_key must equal sku", name)
	}
	expectedCents := dollarsToCents(sku.PricePlaceholder.AmountUSD)
	if sku.Stripe.Price.UnitAmount != expectedCents {
		return fmt.Errorf("%s: stripe.price.unit_amount %d must equal amount_usd*100=%d", name, sku.Stripe.Price.UnitAmount, expectedCents)
	}
	if !strings.EqualFold(sku.Stripe.Price.Currency, sku.PricePlaceholder.Currency) {
		return fmt.Errorf("%s: stripe.price.currency must match price_placeholder.currency", name)
	}
	if sku.StripeMode == StripeModeSubscription {
		if sku.Stripe.Price.Recurring == nil {
			return fmt.Errorf("%s: subscription SKUs require stripe.price.recurring", name)
		}
		interval := sku.Stripe.Price.Recurring.Interval
		if interval != "month" && interval != "year" {
			return fmt.Errorf("%s: recurring.interval must be month or year", name)
		}
		if sku.PricePlaceholder.Interval != interval {
			return fmt.Errorf("%s: price_placeholder.interval must match recurring.interval", name)
		}
		if sku.Stripe.Price.Recurring.IntervalCount <= 0 {
			sku.Stripe.Price.Recurring.IntervalCount = 1
		}
	} else if sku.Stripe.Price.Recurring != nil {
		return fmt.Errorf("%s: non-subscription SKUs must not set recurring", name)
	}
	return nil
}

func dollarsToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

// SelfServeSKUs returns SKUs that sync to Stripe Products/Prices.
func (c *Catalog) SelfServeSKUs() []SKU {
	out := make([]SKU, 0, len(c.SKUs))
	for _, sku := range c.SKUs {
		if sku.SelfServeStripe {
			out = append(out, sku)
		}
	}
	return out
}

// LookupSKU returns a SKU by name.
func (c *Catalog) LookupSKU(name string) (SKU, bool) {
	for _, sku := range c.SKUs {
		if sku.SKU == name {
			return sku, true
		}
	}
	return SKU{}, false
}

// CheckoutMode returns the Stripe Checkout Session mode for a SKU.
func (s SKU) CheckoutMode() (string, error) {
	switch s.StripeMode {
	case StripeModePayment:
		return "payment", nil
	case StripeModeSubscription:
		return "subscription", nil
	default:
		return "", fmt.Errorf("sku %s is not self-serve checkout eligible", s.SKU)
	}
}
