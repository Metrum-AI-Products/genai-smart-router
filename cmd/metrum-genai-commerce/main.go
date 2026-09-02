// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// Command metrum-genai-commerce serves Stripe Checkout, webhooks, and entitlement status.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"smart-llmrouter/internal/commerce"
)

func main() {
	addr := flag.String("addr", ":8091", "listen address")
	catalogPath := flag.String("catalog", "docs/enterprise-license-skus.json", "SKU catalog path")
	dbPath := flag.String("db", "tmp/metrum-commerce.sqlite", "SQLite path for commerce tables")
	successURL := flag.String("success-url", "", "default Checkout success URL")
	cancelURL := flag.String("cancel-url", "", "default Checkout cancel URL")
	flag.Parse()

	if err := commerce.LoadCommerceEnvFromCwd(); err != nil {
		die("load commerce.env.json: %v", err)
	}

	secret := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	webhookSecret := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if secret == "" {
		die("STRIPE_SECRET_KEY is required")
	}
	if !strings.HasPrefix(secret, "sk_test_") {
		die("slice 1 requires STRIPE_SECRET_KEY to start with sk_test_")
	}
	if webhookSecret == "" {
		die("STRIPE_WEBHOOK_SECRET is required")
	}

	cat, err := commerce.LoadCatalog(*catalogPath)
	if err != nil {
		die("%v", err)
	}
	stripeClient, err := commerce.NewLiveStripeClient(secret)
	if err != nil {
		die("%v", err)
	}

	if dir := filepath.Dir(*dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			die("mkdir db parent: %v", err)
		}
	}
	gdb, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		die("open db: %v", err)
	}
	store, err := commerce.OpenStore(gdb)
	if err != nil {
		die("%v", err)
	}

	fleet := commerce.FleetRunnerFromEnv()

	srv := &commerce.Server{
		Catalog:       cat,
		Stripe:        stripeClient,
		Store:         store,
		Fleet:         fleet,
		WebhookSecret: webhookSecret,
		AdminToken:    strings.TrimSpace(os.Getenv("COMMERCE_ADMIN_TOKEN")),
		SuccessURL:    *successURL,
		CancelURL:     *cancelURL,
		Tolerance:     5 * time.Minute,
	}

	log.Printf("metrum-genai-commerce listening on %s (catalog=%s db=%s)", *addr, *catalogPath, *dbPath)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		die("serve: %v", err)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
