// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package licensecontract

import "testing"

func TestAcceptedProduct(t *testing.T) {
	if !AcceptedProduct(Product) {
		t.Fatalf("canonical product %q should be accepted", Product)
	}
	if !AcceptedProduct(ProductLegacy) {
		t.Fatalf("legacy product %q should be accepted", ProductLegacy)
	}
	if AcceptedProduct("other-product") {
		t.Fatal("unrelated product must be rejected")
	}
	if Product != "metrum-ai-router" {
		t.Fatalf("Product = %q, want metrum-ai-router", Product)
	}
	if ProductLegacy != "genai-smart-router" {
		t.Fatalf("ProductLegacy = %q, want genai-smart-router", ProductLegacy)
	}
}
