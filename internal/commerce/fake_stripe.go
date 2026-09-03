// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// FakeStripeClient is an in-memory Stripe client for tests.
type FakeStripeClient struct {
	mu        sync.Mutex
	products  map[string]ProductSnapshot // id -> product
	prices    map[string]PriceSnapshot   // id -> price
	byLookup  map[string]string          // lookup_key -> price id
	nextProd  int
	nextPrice int
	nextSess  int
	Sessions  []CheckoutSessionParams
}

// NewFakeStripeClient constructs an empty fake Stripe client.
func NewFakeStripeClient() *FakeStripeClient {
	return &FakeStripeClient{
		products: map[string]ProductSnapshot{},
		prices:   map[string]PriceSnapshot{},
		byLookup: map[string]string{},
	}
}

func (f *FakeStripeClient) ListProducts(ctx context.Context) ([]ProductSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProductSnapshot, 0, len(f.products))
	for _, p := range f.products {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *FakeStripeClient) CreateProduct(ctx context.Context, sku SKU) (ProductSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextProd++
	id := fmt.Sprintf("prod_fake_%d", f.nextProd)
	p := ProductSnapshot{
		ID:          id,
		Name:        sku.Stripe.Product.Name,
		Description: sku.Stripe.Product.Description,
		Active:      true,
		MetadataSKU: sku.SKU,
	}
	f.products[id] = p
	return p, nil
}

func (f *FakeStripeClient) UpdateProduct(ctx context.Context, productID string, sku SKU) (ProductSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.products[productID]
	if !ok {
		return ProductSnapshot{}, fmt.Errorf("product %s not found", productID)
	}
	p.Name = sku.Stripe.Product.Name
	p.Description = sku.Stripe.Product.Description
	p.MetadataSKU = sku.SKU
	p.Active = true
	f.products[productID] = p
	return p, nil
}

func (f *FakeStripeClient) ListPrices(ctx context.Context, productID string) ([]PriceSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PriceSnapshot, 0)
	for _, price := range f.prices {
		if price.ProductID == productID {
			out = append(out, price)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *FakeStripeClient) CreatePrice(ctx context.Context, productID string, sku SKU, transferLookupKey bool) (PriceSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.products[productID]; !ok {
		return PriceSnapshot{}, fmt.Errorf("product %s not found", productID)
	}
	lookup := sku.SKU
	if existingID, ok := f.byLookup[lookup]; ok && !transferLookupKey {
		return PriceSnapshot{}, fmt.Errorf("lookup_key %s already bound to %s", lookup, existingID)
	}
	if transferLookupKey {
		if oldID, ok := f.byLookup[lookup]; ok {
			old := f.prices[oldID]
			old.LookupKey = ""
			old.Active = false
			f.prices[oldID] = old
			delete(f.byLookup, lookup)
		}
	}
	f.nextPrice++
	id := fmt.Sprintf("price_fake_%d", f.nextPrice)
	price := PriceSnapshot{
		ID:          id,
		ProductID:   productID,
		LookupKey:   lookup,
		UnitAmount:  sku.Stripe.Price.UnitAmount,
		Currency:    strings.ToLower(sku.Stripe.Price.Currency),
		Active:      true,
		MetadataSKU: sku.SKU,
	}
	if sku.Stripe.Price.Recurring != nil {
		price.RecurringInterval = sku.Stripe.Price.Recurring.Interval
		count := sku.Stripe.Price.Recurring.IntervalCount
		if count <= 0 {
			count = 1
		}
		price.RecurringCount = count
	}
	f.prices[id] = price
	f.byLookup[lookup] = id
	return price, nil
}

func (f *FakeStripeClient) ArchivePrice(ctx context.Context, priceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	price, ok := f.prices[priceID]
	if !ok {
		return fmt.Errorf("price %s not found", priceID)
	}
	if price.LookupKey != "" {
		delete(f.byLookup, price.LookupKey)
		price.LookupKey = ""
	}
	price.Active = false
	f.prices[priceID] = price
	return nil
}

func (f *FakeStripeClient) FindPriceByLookupKey(ctx context.Context, lookupKey string) (*PriceSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byLookup[lookupKey]
	if !ok {
		return nil, nil
	}
	price := f.prices[id]
	if !price.Active {
		return nil, nil
	}
	cp := price
	return &cp, nil
}

func (f *FakeStripeClient) CreateCheckoutSession(ctx context.Context, params CheckoutSessionParams) (CheckoutSessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sessions = append(f.Sessions, params)
	f.nextSess++
	id := fmt.Sprintf("cs_test_fake_%d", f.nextSess)
	return CheckoutSessionResult{ID: id, URL: "https://checkout.stripe.test/" + id}, nil
}

// ActivePriceBySKU returns the active price for a SKU lookup key (test helper).
func (f *FakeStripeClient) ActivePriceBySKU(sku string) *PriceSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byLookup[sku]
	if !ok {
		return nil
	}
	p := f.prices[id]
	return &p
}
