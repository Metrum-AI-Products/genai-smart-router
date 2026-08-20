package commerce

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// FleetBootstrapRequest is the injectable Fleet fulfillment input (safe scalars).
type FleetBootstrapRequest struct {
	CustomerID        string
	SKU               string
	LicenseTemplate   string
	EntitlementID     uint
	CheckoutSessionID string
}

// FleetBootstrapResult is the safe scalar outcome of a Fleet bootstrap call.
type FleetBootstrapResult struct {
	CustomerID string
	JobID      string
	Hostname   string
	State      string
}

// FleetRunner starts (or resumes) an isolated non-prod SQLite customer instance.
type FleetRunner interface {
	Bootstrap(ctx context.Context, req FleetBootstrapRequest) (FleetBootstrapResult, error)
}

// FakeFleetRunner records bootstrap calls for tests.
type FakeFleetRunner struct {
	mu      sync.Mutex
	Calls   []FleetBootstrapRequest
	Results map[string]FleetBootstrapResult // keyed by customer id
	Err     error
	next    int
}

// NewFakeFleetRunner constructs a fake Fleet runner.
func NewFakeFleetRunner() *FakeFleetRunner {
	return &FakeFleetRunner{Results: map[string]FleetBootstrapResult{}}
}

func (f *FakeFleetRunner) Bootstrap(ctx context.Context, req FleetBootstrapRequest) (FleetBootstrapResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	if f.Err != nil {
		return FleetBootstrapResult{}, f.Err
	}
	if res, ok := f.Results[req.CustomerID]; ok {
		return res, nil
	}
	f.next++
	res := FleetBootstrapResult{
		CustomerID: req.CustomerID,
		JobID:      fmt.Sprintf("job_fake_%d", f.next),
		Hostname:   fmt.Sprintf("%s.example.test", req.CustomerID),
		State:      "ready",
	}
	f.Results[req.CustomerID] = res
	return res, nil
}

// CallCount returns how many Bootstrap calls were made.
func (f *FakeFleetRunner) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

// ShellFleetRunner shells out to metrum-genai-smartrouter-fleetctl customer bootstrap.
// Operators must supply explicit flags via environment; the purchase pod must not
// mount router config secrets.
type ShellFleetRunner struct {
	Binary           string
	ProfileRef       string
	LicenseRef       string
	ConfigFile       string
	EnvFile          string
	SignWithKey      string
	OwnerUser        string
	Project          string
	TokenOutTemplate string // sprintf with customer id
	ExtraArgs        []string
}

func (s *ShellFleetRunner) Bootstrap(ctx context.Context, req FleetBootstrapRequest) (FleetBootstrapResult, error) {
	bin := strings.TrimSpace(s.Binary)
	if bin == "" {
		bin = "metrum-genai-smartrouter-fleetctl"
	}
	tokenOut := s.TokenOutTemplate
	if tokenOut == "" {
		tokenOut = "/tmp/commerce-token-%s.txt"
	}
	tokenOut = fmt.Sprintf(tokenOut, req.CustomerID)
	args := []string{
		"customer", "bootstrap",
		"--customer-id", req.CustomerID,
		"--profile-ref", s.ProfileRef,
		"--license-ref", s.LicenseRef,
		"--config-file", s.ConfigFile,
		"--env-file", s.EnvFile,
		"--sign-with-key", s.SignWithKey,
		"--owner-user", s.OwnerUser,
		"--project", s.Project,
		"--token-out", tokenOut,
	}
	args = append(args, s.ExtraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return FleetBootstrapResult{}, fmt.Errorf("fleet bootstrap failed: %w", err)
	}
	_ = out // output may contain paths; do not log raw tokens
	return FleetBootstrapResult{
		CustomerID: req.CustomerID,
		JobID:      fmt.Sprintf("bootstrap-%s", req.CustomerID),
		State:      "submitted",
	}, nil
}
