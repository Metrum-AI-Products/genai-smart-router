// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package commerce

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PlanAction describes one catalog sync step.
type PlanAction struct {
	SKU     string `json:"sku"`
	Action  string `json:"action"`
	Detail  string `json:"detail"`
	Managed bool   `json:"managed"`
}

// PlanResult is the output of catalog plan/status.
type PlanResult struct {
	Mode    string       `json:"mode"`
	Actions []PlanAction `json:"actions"`
	Drift   int          `json:"drift"`
}

const (
	ActionCreateProduct    = "create_product"
	ActionUpdateProduct    = "update_product"
	ActionCreatePrice      = "create_price"
	ActionReplacePrice     = "replace_price"
	ActionNoop             = "noop"
	ActionArchiveUnmanaged = "archive_unmanaged_price"
)

// PlanCatalog compares desired self-serve SKUs against Stripe state.
func PlanCatalog(ctx context.Context, client StripeClient, cat *Catalog, pruneUnmanaged bool) (*PlanResult, error) {
	products, err := client.ListProducts(ctx)
	if err != nil {
		return nil, err
	}
	bySKUProduct := map[string]ProductSnapshot{}
	for _, p := range products {
		if p.MetadataSKU == "" {
			continue
		}
		bySKUProduct[p.MetadataSKU] = p
	}

	var actions []PlanAction
	managedSKUs := map[string]struct{}{}
	for _, sku := range cat.SelfServeSKUs() {
		managedSKUs[sku.SKU] = struct{}{}
		prod, hasProd := bySKUProduct[sku.SKU]
		if !hasProd {
			actions = append(actions, PlanAction{
				SKU: sku.SKU, Action: ActionCreateProduct,
				Detail: "create product and price", Managed: true,
			})
			continue
		}
		if prod.Name != sku.Stripe.Product.Name || prod.Description != sku.Stripe.Product.Description || !prod.Active {
			actions = append(actions, PlanAction{
				SKU: sku.SKU, Action: ActionUpdateProduct,
				Detail: "update product name/description/active", Managed: true,
			})
		}
		prices, err := client.ListPrices(ctx, prod.ID)
		if err != nil {
			return nil, err
		}
		var activeMatch *PriceSnapshot
		var activeLookup *PriceSnapshot
		for i := range prices {
			p := prices[i]
			if !p.Active {
				continue
			}
			if p.LookupKey == sku.SKU {
				cp := p
				activeLookup = &cp
			}
			if PriceMatchesSKU(p, sku) {
				cp := p
				activeMatch = &cp
			}
		}
		switch {
		case activeMatch != nil && activeMatch.LookupKey == sku.SKU:
			actions = append(actions, PlanAction{
				SKU: sku.SKU, Action: ActionNoop, Detail: "product and price match", Managed: true,
			})
		case activeLookup != nil && !PriceMatchesSKU(*activeLookup, sku):
			actions = append(actions, PlanAction{
				SKU: sku.SKU, Action: ActionReplacePrice,
				Detail:  fmt.Sprintf("archive %s and create price amount=%d", activeLookup.ID, sku.Stripe.Price.UnitAmount),
				Managed: true,
			})
		default:
			actions = append(actions, PlanAction{
				SKU: sku.SKU, Action: ActionCreatePrice,
				Detail: "create price with lookup_key", Managed: true,
			})
		}
	}

	if pruneUnmanaged {
		for skuName, prod := range bySKUProduct {
			if _, ok := managedSKUs[skuName]; ok {
				continue
			}
			prices, err := client.ListPrices(ctx, prod.ID)
			if err != nil {
				return nil, err
			}
			for _, p := range prices {
				if p.Active {
					actions = append(actions, PlanAction{
						SKU: skuName, Action: ActionArchiveUnmanaged,
						Detail: fmt.Sprintf("archive unmanaged price %s", p.ID), Managed: false,
					})
				}
			}
		}
	}

	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].SKU == actions[j].SKU {
			return actions[i].Action < actions[j].Action
		}
		return actions[i].SKU < actions[j].SKU
	})
	drift := 0
	for _, a := range actions {
		if a.Action != ActionNoop {
			drift++
		}
	}
	return &PlanResult{Actions: actions, Drift: drift}, nil
}

// ApplyCatalog idempotently creates/updates Products and Prices for self-serve SKUs.
func ApplyCatalog(ctx context.Context, client StripeClient, cat *Catalog, pruneUnmanaged bool) (*PlanResult, error) {
	plan, err := PlanCatalog(ctx, client, cat, pruneUnmanaged)
	if err != nil {
		return nil, err
	}
	for _, action := range plan.Actions {
		sku, ok := cat.LookupSKU(action.SKU)
		if !ok && action.Managed {
			return nil, fmt.Errorf("unknown managed sku %s", action.SKU)
		}
		switch action.Action {
		case ActionNoop:
			continue
		case ActionCreateProduct:
			prod, err := client.CreateProduct(ctx, sku)
			if err != nil {
				return nil, err
			}
			if _, err := client.CreatePrice(ctx, prod.ID, sku, true); err != nil {
				return nil, err
			}
		case ActionUpdateProduct:
			prodID, err := findProductID(ctx, client, sku.SKU)
			if err != nil {
				return nil, err
			}
			if _, err := client.UpdateProduct(ctx, prodID, sku); err != nil {
				return nil, err
			}
		case ActionCreatePrice:
			prodID, err := findProductID(ctx, client, sku.SKU)
			if err != nil {
				return nil, err
			}
			if _, err := client.CreatePrice(ctx, prodID, sku, true); err != nil {
				return nil, err
			}
		case ActionReplacePrice:
			prodID, err := findProductID(ctx, client, sku.SKU)
			if err != nil {
				return nil, err
			}
			prices, err := client.ListPrices(ctx, prodID)
			if err != nil {
				return nil, err
			}
			for _, p := range prices {
				if p.Active && (p.LookupKey == sku.SKU || p.MetadataSKU == sku.SKU) {
					if err := client.ArchivePrice(ctx, p.ID); err != nil {
						return nil, err
					}
				}
			}
			if _, err := client.CreatePrice(ctx, prodID, sku, true); err != nil {
				return nil, err
			}
		case ActionArchiveUnmanaged:
			detail := action.Detail
			const prefix = "archive unmanaged price "
			if strings.HasPrefix(detail, prefix) {
				priceID := strings.TrimPrefix(detail, prefix)
				if err := client.ArchivePrice(ctx, priceID); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unsupported action %s", action.Action)
		}
	}
	return PlanCatalog(ctx, client, cat, pruneUnmanaged)
}

func findProductID(ctx context.Context, client StripeClient, sku string) (string, error) {
	products, err := client.ListProducts(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range products {
		if p.MetadataSKU == sku {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("product for sku %s not found", sku)
}
