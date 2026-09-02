// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"context"
	"fmt"
	"strings"
)

// ProductSnapshot is a sanitized Stripe Product view for catalog sync.
type ProductSnapshot struct {
	ID          string
	Name        string
	Description string
	Active      bool
	MetadataSKU string
}

// PriceSnapshot is a sanitized Stripe Price view for catalog sync.
type PriceSnapshot struct {
	ID                string
	ProductID         string
	LookupKey         string
	UnitAmount        int64
	Currency          string
	Active            bool
	MetadataSKU       string
	RecurringInterval string
	RecurringCount    int64
}

// CheckoutSessionParams are inputs to create a Checkout Session.
type CheckoutSessionParams struct {
	SKU             string
	PriceID         string
	Mode            string
	SuccessURL      string
	CancelURL       string
	CustomerEmail   string
	CustomerAlias   string
	ClientReference string
}

// CheckoutSessionResult is the created session (safe scalars).
type CheckoutSessionResult struct {
	ID  string
	URL string
}

// StripeClient is the injectable Stripe API surface used by catalog CLI and purchase server.
type StripeClient interface {
	ListProducts(ctx context.Context) ([]ProductSnapshot, error)
	CreateProduct(ctx context.Context, sku SKU) (ProductSnapshot, error)
	UpdateProduct(ctx context.Context, productID string, sku SKU) (ProductSnapshot, error)
	ListPrices(ctx context.Context, productID string) ([]PriceSnapshot, error)
	CreatePrice(ctx context.Context, productID string, sku SKU, transferLookupKey bool) (PriceSnapshot, error)
	ArchivePrice(ctx context.Context, priceID string) error
	FindPriceByLookupKey(ctx context.Context, lookupKey string) (*PriceSnapshot, error)
	CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (CheckoutSessionResult, error)
}

// PriceMatchesSKU reports whether an active Stripe Price matches catalog desired state.
func PriceMatchesSKU(price PriceSnapshot, sku SKU) bool {
	if !price.Active {
		return false
	}
	if price.LookupKey != sku.SKU || price.MetadataSKU != sku.SKU {
		return false
	}
	if price.UnitAmount != sku.Stripe.Price.UnitAmount {
		return false
	}
	if !strings.EqualFold(price.Currency, sku.Stripe.Price.Currency) {
		return false
	}
	wantRecurring := sku.Stripe.Price.Recurring
	if wantRecurring == nil {
		return price.RecurringInterval == ""
	}
	count := wantRecurring.IntervalCount
	if count <= 0 {
		count = 1
	}
	return price.RecurringInterval == wantRecurring.Interval && price.RecurringCount == count
}

// ResolvePriceID looks up an active Price by catalog lookup_key and validates it.
func ResolvePriceID(ctx context.Context, client StripeClient, cat *Catalog, skuName string) (string, SKU, error) {
	sku, ok := cat.LookupSKU(skuName)
	if !ok {
		return "", SKU{}, fmt.Errorf("unknown sku %q", skuName)
	}
	if !sku.SelfServeStripe || sku.Stripe == nil {
		return "", SKU{}, fmt.Errorf("sku %q is not self-serve stripe", skuName)
	}
	price, err := client.FindPriceByLookupKey(ctx, sku.SKU)
	if err != nil {
		return "", SKU{}, err
	}
	if price == nil {
		return "", SKU{}, fmt.Errorf("no active stripe price for lookup_key %q", sku.SKU)
	}
	if price.MetadataSKU == "" || price.MetadataSKU != sku.SKU {
		return "", SKU{}, fmt.Errorf("price metadata.sku missing or unknown for lookup_key %q", sku.SKU)
	}
	if !PriceMatchesSKU(*price, sku) {
		return "", SKU{}, fmt.Errorf("price %s disagrees with catalog for sku %q (unit_amount/interval)", price.ID, sku.SKU)
	}
	return price.ID, sku, nil
}
