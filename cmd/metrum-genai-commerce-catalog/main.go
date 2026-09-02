// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// Command metrum-genai-commerce-catalog syncs self-serve SKUs to Stripe Products/Prices.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"smart-llmrouter/internal/commerce"
)

func main() {
	if len(os.Args) < 2 {
		die("usage: metrum-genai-commerce-catalog <plan|apply|status> --mode test|live --catalog <path> [--prune-unmanaged]")
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	mode := fs.String("mode", "", "stripe mode: test|live")
	catalogPath := fs.String("catalog", "docs/enterprise-license-skus.json", "path to enterprise-license-skus.json")
	prune := fs.Bool("prune-unmanaged", false, "archive active prices for products with metadata.sku not in catalog")
	_ = fs.Parse(os.Args[2:])

	if err := commerce.LoadCommerceEnvFromCwd(); err != nil {
		die("load commerce.env.json: %v", err)
	}

	if *mode != "test" && *mode != "live" {
		die("--mode must be test or live")
	}
	if *mode == "live" {
		die("slice 1 refuses --mode live; use --mode test only")
	}

	secret := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if secret == "" {
		die("STRIPE_SECRET_KEY is required")
	}
	if !strings.HasPrefix(secret, "sk_test_") {
		die("--mode test requires STRIPE_SECRET_KEY to start with sk_test_")
	}

	cat, err := commerce.LoadCatalog(*catalogPath)
	if err != nil {
		die("%v", err)
	}
	client, err := commerce.NewLiveStripeClient(secret)
	if err != nil {
		die("%v", err)
	}
	ctx := context.Background()

	switch cmd {
	case "plan", "status":
		result, err := commerce.PlanCatalog(ctx, client, cat, *prune)
		if err != nil {
			die("%v", err)
		}
		result.Mode = *mode
		writeJSON(result)
		if cmd == "status" && result.Drift != 0 {
			os.Exit(2)
		}
	case "apply":
		result, err := commerce.ApplyCatalog(ctx, client, cat, *prune)
		if err != nil {
			die("%v", err)
		}
		result.Mode = *mode
		writeJSON(result)
		if result.Drift != 0 {
			os.Exit(2)
		}
	default:
		die("unsupported command %q", cmd)
	}
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
