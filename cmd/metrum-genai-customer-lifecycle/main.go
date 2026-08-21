// Command metrum-genai-customer-lifecycle orchestrates commerce pay + license SSM
// publish + fleetctl customer bootstrap from a JSON intent. It composes existing
// binaries and does not replace metrum-genai-smartrouter-fleetctl (#555).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"smart-llmrouter/internal/customerlifecycle"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "onboard":
		err = cmdOnboard(ctx, os.Args[2:])
	case "status":
		err = cmdWithIntent(ctx, os.Args[2:], func(ctx context.Context, intent customerlifecycle.Intent) error {
			return customerlifecycle.WrapStatus(ctx, intent)
		})
	case "smoke":
		err = cmdSmoke(ctx, os.Args[2:])
	case "update-config":
		err = cmdUpdateConfig(ctx, os.Args[2:])
	case "export-usage":
		err = cmdExportUsage(ctx, os.Args[2:])
	case "delete":
		err = cmdDelete(ctx, os.Args[2:])
	case "validate-intent":
		err = cmdValidateIntent(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: metrum-genai-customer-lifecycle <command> [flags]

commands:
  validate-intent  --intent PATH
  onboard          --intent PATH [--from-step pay|license|provision] [--poll-wait 10m]
  status           --intent PATH
  smoke            --intent PATH [--token-file PATH] [--model GROUP]
  update-config    --intent PATH [--patch-file PATH] [--refresh-byok]
                   (publishes runtime bundle and signed Fleet redeploy)
  export-usage     --intent PATH --out-dir PATH [--token-file PATH]
  delete           --intent PATH [--confirm-file PATH]

Onboard steps: collect/validate → pay+license (commerce + SSM) → provision (fleetctl bootstrap).
BYOK is required. Never put raw API keys in the intent JSON.
Fleet mutation authority remains metrum-genai-smartrouter-fleetctl (#555).`)
}

func cmdValidateIntent(args []string) error {
	fs := flag.NewFlagSet("validate-intent", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	if err := customerlifecycle.ValidateConfigRoutesBYOK(intent.ConfigFile, intent.BYOK, intent.SmokeModel); err != nil {
		return err
	}
	if err := customerlifecycle.ValidateFleetEKSProvisionPaths(intent.ConfigFile); err != nil {
		return err
	}
	if err := customerlifecycle.ValidateConfigRetentionForSKU(intent.ConfigFile, intent.Catalog, intent.SKU); err != nil {
		return err
	}
	fmt.Println(string(mustJSON(map[string]any{
		"ok":          true,
		"customer_id": intent.CustomerID,
		"hostname":    intent.Hostname,
		"sku":         intent.SKU,
		"byok": map[string]string{
			"provider":    intent.BYOK.Provider,
			"api_key_env": intent.BYOK.APIKeyEnv,
			"model":       intent.BYOK.Model,
		},
		"fleet_eks_paths": "ok",
		"retention_gate":  "ok",
	})))
	return nil
}

func cmdOnboard(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("onboard", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	fromStep := fs.String("from-step", "", "resume from pay|license|provision")
	pollWait := fs.Duration("poll-wait", 10*time.Minute, "max wait for paid entitlement")
	pollEvery := fs.Duration("poll-every", 2*time.Second, "entitlement poll interval")
	skipCheckout := fs.Bool("skip-checkout", false, "reuse checkout_session_id from workspace state")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	var step customerlifecycle.OnboardStep
	switch strings.TrimSpace(*fromStep) {
	case "":
	case "pay":
		step = customerlifecycle.StepPay
	case "license":
		step = customerlifecycle.StepLicense
	case "provision":
		step = customerlifecycle.StepProvision
	default:
		return fmt.Errorf("unsupported --from-step %q", *fromStep)
	}
	st, err := customerlifecycle.RunOnboard(ctx, customerlifecycle.OnboardOptions{
		IntentPath:       *intentPath,
		FromStep:         step,
		PollEvery:        *pollEvery,
		PollWait:         *pollWait,
		SkipCheckout:     *skipCheckout,
		PrintCheckoutURL: true,
	})
	if err != nil {
		_ = customerlifecycle.PrintStateJSON(st)
		return err
	}
	return customerlifecycle.PrintStateJSON(st)
}

func cmdWithIntent(ctx context.Context, args []string, fn func(context.Context, customerlifecycle.Intent) error) error {
	fs := flag.NewFlagSet("intent-cmd", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	return fn(ctx, intent)
}

func cmdSmoke(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("smoke", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	tokenFile := fs.String("token-file", "", "caller token file")
	model := fs.String("model", "", "model group")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	return customerlifecycle.WrapSmoke(ctx, intent, *tokenFile, *model)
}

func cmdUpdateConfig(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update-config", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	patchFile := fs.String("patch-file", "", "YAML patch for runtime config")
	refreshBYOK := fs.Bool("refresh-byok", false, "rebuild instance env from byok_env_file and republish bundle")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	if strings.TrimSpace(*patchFile) == "" && !*refreshBYOK {
		return fmt.Errorf("provide --patch-file and/or --refresh-byok")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	return customerlifecycle.WrapUpdateConfig(ctx, intent, *patchFile, *refreshBYOK)
}

func cmdExportUsage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export-usage", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	outDir := fs.String("out-dir", "", "mode-0700 output directory (required)")
	tokenFile := fs.String("token-file", "", "authenticated caller/admin token file")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" || strings.TrimSpace(*outDir) == "" {
		return fmt.Errorf("--intent and --out-dir are required")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	return customerlifecycle.ExportUsage(ctx, intent, *tokenFile, *outDir)
}

func cmdDelete(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	intentPath := fs.String("intent", "", "onboard intent JSON (required)")
	confirmFile := fs.String("confirm-file", "", "externally signed delete approval (optional if sign_key set)")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		return fmt.Errorf("--intent is required")
	}
	intent, err := customerlifecycle.LoadIntent(*intentPath)
	if err != nil {
		return err
	}
	return customerlifecycle.WrapDelete(ctx, intent, *confirmFile)
}

func mustJSON(v any) []byte {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}
	return append(raw, '\n')
}
