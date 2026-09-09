// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"context"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/price"
	"github.com/stripe/stripe-go/v82/product"
)

// LiveStripeClient wraps stripe-go SDK calls. Caller must set stripe.Key before use.
type LiveStripeClient struct{}

// NewLiveStripeClient returns a live Stripe client. Requires stripe.Key to be set.
func NewLiveStripeClient(secretKey string) (*LiveStripeClient, error) {
	key := strings.TrimSpace(secretKey)
	if key == "" {
		return nil, fmt.Errorf("STRIPE_SECRET_KEY is required")
	}
	stripe.Key = key
	return &LiveStripeClient{}, nil
}

func (c *LiveStripeClient) ListProducts(ctx context.Context) ([]ProductSnapshot, error) {
	params := &stripe.ProductListParams{}
	params.Filters.AddFilter("limit", "", "100")
	iter := product.List(params)
	var out []ProductSnapshot
	for iter.Next() {
		p := iter.Product()
		out = append(out, snapshotProduct(p))
	}
	return out, iter.Err()
}

func (c *LiveStripeClient) CreateProduct(ctx context.Context, sku SKU) (ProductSnapshot, error) {
	params := &stripe.ProductParams{
		Name:        stripe.String(sku.Stripe.Product.Name),
		Description: stripe.String(sku.Stripe.Product.Description),
		Metadata: map[string]string{
			"sku": sku.SKU,
		},
	}
	p, err := product.New(params)
	if err != nil {
		return ProductSnapshot{}, err
	}
	return snapshotProduct(p), nil
}

func (c *LiveStripeClient) UpdateProduct(ctx context.Context, productID string, sku SKU) (ProductSnapshot, error) {
	params := &stripe.ProductParams{
		Name:        stripe.String(sku.Stripe.Product.Name),
		Description: stripe.String(sku.Stripe.Product.Description),
		Active:      stripe.Bool(true),
		Metadata: map[string]string{
			"sku": sku.SKU,
		},
	}
	p, err := product.Update(productID, params)
	if err != nil {
		return ProductSnapshot{}, err
	}
	return snapshotProduct(p), nil
}

func (c *LiveStripeClient) ListPrices(ctx context.Context, productID string) ([]PriceSnapshot, error) {
	params := &stripe.PriceListParams{}
	params.Filters.AddFilter("product", "", productID)
	params.Filters.AddFilter("limit", "", "100")
	iter := price.List(params)
	var out []PriceSnapshot
	for iter.Next() {
		p := iter.Price()
		out = append(out, snapshotPrice(p))
	}
	return out, iter.Err()
}

func (c *LiveStripeClient) CreatePrice(ctx context.Context, productID string, sku SKU, transferLookupKey bool) (PriceSnapshot, error) {
	params := &stripe.PriceParams{
		Product:    stripe.String(productID),
		Currency:   stripe.String(strings.ToLower(sku.Stripe.Price.Currency)),
		UnitAmount: stripe.Int64(sku.Stripe.Price.UnitAmount),
		LookupKey:  stripe.String(sku.SKU),
		Metadata: map[string]string{
			"sku": sku.SKU,
		},
	}
	if transferLookupKey {
		params.TransferLookupKey = stripe.Bool(true)
	}
	if sku.Stripe.Price.Recurring != nil {
		count := sku.Stripe.Price.Recurring.IntervalCount
		if count <= 0 {
			count = 1
		}
		params.Recurring = &stripe.PriceRecurringParams{
			Interval:      stripe.String(sku.Stripe.Price.Recurring.Interval),
			IntervalCount: stripe.Int64(count),
		}
	}
	p, err := price.New(params)
	if err != nil {
		return PriceSnapshot{}, err
	}
	return snapshotPrice(p), nil
}

func (c *LiveStripeClient) ArchivePrice(ctx context.Context, priceID string) error {
	_, err := price.Update(priceID, &stripe.PriceParams{Active: stripe.Bool(false)})
	return err
}

func (c *LiveStripeClient) FindPriceByLookupKey(ctx context.Context, lookupKey string) (*PriceSnapshot, error) {
	params := &stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{lookupKey}),
		Active:     stripe.Bool(true),
		ListParams: stripe.ListParams{Limit: stripe.Int64(1)},
	}
	iter := price.List(params)
	if iter.Next() {
		snap := snapshotPrice(iter.Price())
		return &snap, iter.Err()
	}
	return nil, iter.Err()
}

func (c *LiveStripeClient) CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (CheckoutSessionResult, error) {
	sp := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(params.Mode),
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(params.PriceID),
				Quantity: stripe.Int64(1),
			},
		},
		Metadata: map[string]string{
			"sku":            params.SKU,
			"price_id":       params.PriceID,
			"customer_alias": params.CustomerAlias,
		},
		ClientReferenceID: stripe.String(params.ClientReference),
	}
	if params.CustomerEmail != "" {
		sp.CustomerEmail = stripe.String(params.CustomerEmail)
	}
	sess, err := session.New(sp)
	if err != nil {
		return CheckoutSessionResult{}, err
	}
	return CheckoutSessionResult{ID: sess.ID, URL: sess.URL}, nil
}

func snapshotProduct(p *stripe.Product) ProductSnapshot {
	sku := ""
	if p.Metadata != nil {
		sku = p.Metadata["sku"]
	}
	return ProductSnapshot{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Active:      p.Active,
		MetadataSKU: sku,
	}
}

func snapshotPrice(p *stripe.Price) PriceSnapshot {
	sku := ""
	if p.Metadata != nil {
		sku = p.Metadata["sku"]
	}
	snap := PriceSnapshot{
		ID:          p.ID,
		ProductID:   "",
		LookupKey:   p.LookupKey,
		UnitAmount:  p.UnitAmount,
		Currency:    string(p.Currency),
		Active:      p.Active,
		MetadataSKU: sku,
	}
	if p.Product != nil {
		snap.ProductID = p.Product.ID
	}
	if p.Recurring != nil {
		snap.RecurringInterval = string(p.Recurring.Interval)
		snap.RecurringCount = p.Recurring.IntervalCount
	}
	return snap
}
