package customerlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OnboardStep names resumable onboard phases.
type OnboardStep string

const (
	StepCollect   OnboardStep = "collect"
	StepPay       OnboardStep = "pay"
	StepLicense   OnboardStep = "license"
	StepProvision OnboardStep = "provision"
	StepDone      OnboardStep = "done"
)

// OnboardOptions controls resume and polling.
type OnboardOptions struct {
	IntentPath       string
	FromStep         OnboardStep // empty = auto from workspace state
	PollEvery        time.Duration
	PollWait         time.Duration
	SkipCheckout     bool // resume when checkout_session_id already recorded
	PrintCheckoutURL bool
}

// OnboardState is safe-scalar workspace progress (never includes secrets).
type OnboardState struct {
	CustomerID        string `json:"customer_id"`
	Hostname          string `json:"hostname"`
	SKU               string `json:"sku"`
	Step              string `json:"step"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	CheckoutURL       string `json:"checkout_url,omitempty"`
	EntitlementStatus string `json:"entitlement_status,omitempty"`
	LicenseRef        string `json:"license_ref,omitempty"`
	LicensePublished  bool   `json:"license_published,omitempty"`
	EnvFile           string `json:"env_file,omitempty"`
	ProvisionState    string `json:"provision_state,omitempty"`
	UpdatedAt         string `json:"updated_at"`
}

func statePath(customerID string) string {
	return filepath.Join(fleetCustomerHome(customerID), "customer-lifecycle-state.json")
}

func loadState(customerID string) OnboardState {
	raw, err := os.ReadFile(statePath(customerID))
	if err != nil {
		return OnboardState{CustomerID: customerID}
	}
	var st OnboardState
	_ = json.Unmarshal(raw, &st)
	return st
}

func saveState(st OnboardState) error {
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := jsonMarshalIndent(st)
	if err != nil {
		return err
	}
	return writeMode0600(statePath(st.CustomerID), raw)
}

// RunOnboard executes collect → pay+license → provision.
func RunOnboard(ctx context.Context, opts OnboardOptions) (OnboardState, error) {
	intent, err := LoadIntent(opts.IntentPath)
	if err != nil {
		return OnboardState{}, err
	}
	if err := ValidateConfigRoutesBYOK(intent.ConfigFile, intent.BYOK, intent.SmokeModel); err != nil {
		return OnboardState{}, fmt.Errorf("collect/config: %w", err)
	}
	if err := ValidateFleetEKSProvisionPaths(intent.ConfigFile); err != nil {
		return OnboardState{}, fmt.Errorf("collect/fleet-eks-paths: %w", err)
	}
	if err := ValidateConfigRetentionForSKU(intent.ConfigFile, intent.Catalog, intent.SKU); err != nil {
		return OnboardState{}, fmt.Errorf("collect/retention: %w", err)
	}
	st := loadState(intent.CustomerID)
	st.CustomerID = intent.CustomerID
	st.Hostname = intent.Hostname
	st.SKU = intent.SKU
	st.LicenseRef = intent.LicenseRef
	st.Step = string(StepCollect)
	if err := saveState(st); err != nil {
		return st, err
	}

	start := opts.FromStep
	if start == "" {
		start = resumeStep(st)
	}
	if start == StepDone {
		st.Step = string(StepDone)
		_ = saveState(st)
		return st, nil
	}

	if stepOrder(start) <= stepOrder(StepPay) {
		if err := runPay(ctx, intent, &st, opts); err != nil {
			_ = saveState(st)
			return st, err
		}
	}
	if stepOrder(start) <= stepOrder(StepLicense) {
		if err := runLicense(ctx, intent, &st); err != nil {
			_ = saveState(st)
			return st, err
		}
	}
	if stepOrder(start) <= stepOrder(StepProvision) {
		if err := runProvision(ctx, intent, &st); err != nil {
			_ = saveState(st)
			return st, err
		}
	}
	st.Step = string(StepDone)
	if err := saveState(st); err != nil {
		return st, err
	}
	return st, nil
}

func resumeStep(st OnboardState) OnboardStep {
	switch {
	case st.ProvisionState == "ready" || st.Step == string(StepDone):
		return StepDone
	case st.LicensePublished:
		return StepProvision
	case PaidEntitlementStatuses[st.EntitlementStatus]:
		return StepLicense
	case st.CheckoutSessionID != "":
		return StepPay
	default:
		return StepPay
	}
}

func stepOrder(s OnboardStep) int {
	switch s {
	case StepCollect:
		return 1
	case StepPay:
		return 2
	case StepLicense:
		return 3
	case StepProvision:
		return 4
	case StepDone:
		return 5
	default:
		return 2
	}
}

func runPay(ctx context.Context, intent Intent, st *OnboardState, opts OnboardOptions) error {
	st.Step = string(StepPay)
	client := &CommerceClient{BaseURL: intent.CommerceBaseURL}
	if st.CheckoutSessionID == "" || !opts.SkipCheckout {
		sess, err := client.CreateCheckoutSession(ctx, intent)
		if err != nil {
			return err
		}
		st.CheckoutSessionID = sess.CheckoutSessionID
		st.CheckoutURL = sess.URL
		if err := saveState(*st); err != nil {
			return err
		}
		if opts.PrintCheckoutURL && sess.URL != "" {
			fmt.Fprintf(os.Stderr, "checkout_url=%s\ncomplete Stripe Checkout, then entitlement will advance\n", sess.URL)
		}
	}
	status, err := client.PollUntilPaid(ctx, st.CheckoutSessionID, opts.PollEvery, opts.PollWait)
	if err != nil {
		return err
	}
	st.EntitlementStatus = status.Status
	return saveState(*st)
}

func runLicense(ctx context.Context, intent Intent, st *OnboardState) error {
	st.Step = string(StepLicense)
	workDir := filepath.Join(fleetCustomerHome(intent.CustomerID), "license-issue")
	licensePath, err := IssueLicenseFile(ctx, intent, workDir)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(licensePath)
	if err != nil {
		return err
	}
	if _, err := PublishLicenseSSM(ctx, intent.LicenseRef, raw); err != nil {
		return err
	}
	st.LicensePublished = true
	st.LicenseRef = intent.LicenseRef
	return saveState(*st)
}

func runProvision(ctx context.Context, intent Intent, st *OnboardState) error {
	st.Step = string(StepProvision)
	home := fleetCustomerHome(intent.CustomerID)
	envPath := filepath.Join(home, "instance-env.json")
	envJSON, err := BuildInstanceEnvJSON(intent.BYOKEnvFile, intent.BYOK, intent.Hostname)
	if err != nil {
		return err
	}
	if err := writeMode0600(envPath, envJSON); err != nil {
		return err
	}
	st.EnvFile = envPath

	fleetBin, err := resolveBinary("metrum-genai-smartrouter-fleetctl", "metrum-fleetctl")
	if err != nil {
		return err
	}
	args := []string{
		"customer", "bootstrap",
		"--customer-id", intent.CustomerID,
		"--profile-ref", intent.ProfileRef,
		"--runtime-bundle-ref", intent.RuntimeBundleRef,
		"--license-ref", intent.LicenseRef,
		"--config-file", intent.ConfigFile,
		"--env-file", envPath,
		"--rewrite-paths", "fleet-eks",
		"--sign-with-key", intent.SignKey,
		"--owner-user", intent.OwnerUser,
		"--project", intent.Project,
		"--token-out", intent.TokenOut,
		"--model", intent.SmokeModel,
		"--allow-from-config",
	}
	parsed, err := runCaptureJSON(ctx, fleetBin, args...)
	if err != nil {
		return fmt.Errorf("fleetctl customer bootstrap: %w", err)
	}
	if v, ok := parsed["state"].(string); ok {
		st.ProvisionState = v
	} else {
		st.ProvisionState = "submitted"
	}
	return saveState(*st)
}

// PrintStateJSON prints safe onboard state.
func PrintStateJSON(st OnboardState) error {
	raw, err := jsonMarshalIndent(st)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(raw)
	return err
}

// LoadOnboardState loads workspace state for status.
func LoadOnboardState(customerID string) OnboardState {
	return loadState(strings.ToLower(strings.TrimSpace(customerID)))
}
