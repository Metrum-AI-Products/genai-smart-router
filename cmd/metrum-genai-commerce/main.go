// Command metrum-genai-commerce serves Stripe Checkout, webhooks, and entitlement status.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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
	dbPath := flag.String("db", "commerce.sqlite", "SQLite path for commerce tables")
	successURL := flag.String("success-url", "", "default Checkout success URL")
	cancelURL := flag.String("cancel-url", "", "default Checkout cancel URL")
	flag.Parse()

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

	var fleet commerce.FleetRunner
	if strings.EqualFold(os.Getenv("COMMERCE_FLEET_ENABLED"), "1") {
		fleet = &commerce.ShellFleetRunner{
			Binary:           firstNonEmpty(os.Getenv("COMMERCE_FLEET_BINARY"), "metrum-genai-smartrouter-fleetctl"),
			ProfileRef:       os.Getenv("COMMERCE_FLEET_PROFILE_REF"),
			LicenseRef:       os.Getenv("COMMERCE_FLEET_LICENSE_REF"),
			ConfigFile:       os.Getenv("COMMERCE_FLEET_CONFIG_FILE"),
			EnvFile:          os.Getenv("COMMERCE_FLEET_ENV_FILE"),
			SignWithKey:      os.Getenv("COMMERCE_FLEET_SIGN_KEY"),
			OwnerUser:        firstNonEmpty(os.Getenv("COMMERCE_FLEET_OWNER_USER"), "commerce"),
			Project:          firstNonEmpty(os.Getenv("COMMERCE_FLEET_PROJECT"), "sandbox"),
			TokenOutTemplate: firstNonEmpty(os.Getenv("COMMERCE_FLEET_TOKEN_OUT"), "/tmp/commerce-token-%s.txt"),
		}
	}

	srv := &commerce.Server{
		Catalog:       cat,
		Stripe:        stripeClient,
		Store:         store,
		Fleet:         fleet,
		WebhookSecret: webhookSecret,
		SuccessURL:    *successURL,
		CancelURL:     *cancelURL,
		Tolerance:     5 * time.Minute,
	}

	log.Printf("metrum-genai-commerce listening on %s (catalog=%s)", *addr, *catalogPath)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		die("serve: %v", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
