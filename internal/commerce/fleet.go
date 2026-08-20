package commerce

import (
	"context"
	"encoding/json"
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
// mount router config secrets into logs.
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
	RewritePaths     string
	ExtraArgs        []string
	// StatusBinary optionally overrides Binary for a follow-up customer status call.
	StatusBinary string
}

func (s *ShellFleetRunner) validateRefs() error {
	missing := []string{}
	if strings.TrimSpace(s.ProfileRef) == "" {
		missing = append(missing, "profile-ref")
	}
	if strings.TrimSpace(s.LicenseRef) == "" {
		missing = append(missing, "license-ref")
	}
	if strings.TrimSpace(s.ConfigFile) == "" {
		missing = append(missing, "config-file")
	}
	if strings.TrimSpace(s.EnvFile) == "" {
		missing = append(missing, "env-file")
	}
	if strings.TrimSpace(s.SignWithKey) == "" {
		missing = append(missing, "sign-with-key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("shell fleet refs incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (s *ShellFleetRunner) Bootstrap(ctx context.Context, req FleetBootstrapRequest) (FleetBootstrapResult, error) {
	if err := s.validateRefs(); err != nil {
		return FleetBootstrapResult{}, err
	}
	bin := strings.TrimSpace(s.Binary)
	if bin == "" {
		bin = "metrum-genai-smartrouter-fleetctl"
	}
	tokenOut := s.TokenOutTemplate
	if tokenOut == "" {
		tokenOut = "/tmp/commerce-token-%s.txt"
	}
	tokenOut = fmt.Sprintf(tokenOut, req.CustomerID)
	rewrite := strings.TrimSpace(s.RewritePaths)
	if rewrite == "" {
		rewrite = "fleet-eks"
	}
	args := []string{
		"customer", "bootstrap",
		"--customer-id", req.CustomerID,
		"--profile-ref", s.ProfileRef,
		"--license-ref", s.LicenseRef,
		"--config-file", s.ConfigFile,
		"--env-file", s.EnvFile,
		"--sign-with-key", s.SignWithKey,
		"--owner-user", firstNonEmptyEnv(s.OwnerUser, "commerce"),
		"--project", firstNonEmptyEnv(s.Project, "sandbox"),
		"--token-out", tokenOut,
		"--rewrite-paths", rewrite,
	}
	args = append(args, s.ExtraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Do not include command output: may contain paths near token files.
		return FleetBootstrapResult{}, fmt.Errorf("fleet bootstrap failed for customer %s: %w", req.CustomerID, err)
	}
	res := FleetBootstrapResult{
		CustomerID: req.CustomerID,
		JobID:      fmt.Sprintf("bootstrap-%s", req.CustomerID),
		State:      "submitted",
	}
	if parsed, ok := parseFleetBootstrapJSON(out); ok {
		if parsed.CustomerID != "" {
			res.CustomerID = parsed.CustomerID
		}
		if parsed.JobID != "" {
			res.JobID = parsed.JobID
		}
		if parsed.Hostname != "" {
			res.Hostname = parsed.Hostname
		}
		if parsed.State != "" {
			res.State = parsed.State
		}
	}
	// Optional status follow-up for hostname when bootstrap JSON lacked it.
	if res.Hostname == "" {
		if host, jobID, state := s.tryCustomerStatus(ctx, bin, req.CustomerID); host != "" || jobID != "" {
			if host != "" {
				res.Hostname = host
			}
			if jobID != "" {
				res.JobID = jobID
			}
			if state != "" {
				res.State = state
			}
		}
	}
	_ = out // intentionally unused beyond parse; never log
	return res, nil
}

func (s *ShellFleetRunner) tryCustomerStatus(ctx context.Context, bin, customerID string) (hostname, jobID, state string) {
	statusBin := strings.TrimSpace(s.StatusBinary)
	if statusBin == "" {
		statusBin = bin
	}
	args := []string{
		"customer", "status",
		"--customer-id", customerID,
		"--profile-ref", s.ProfileRef,
	}
	cmd := exec.CommandContext(ctx, statusBin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", ""
	}
	if parsed, ok := parseFleetBootstrapJSON(out); ok {
		return parsed.Hostname, parsed.JobID, parsed.State
	}
	return "", "", ""
}

func parseFleetBootstrapJSON(out []byte) (FleetBootstrapResult, bool) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return FleetBootstrapResult{}, false
	}
	// Prefer last JSON object line (CLIs often print progress then JSON).
	lines := strings.Split(trimmed, "\n")
	var candidate string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			candidate = line
			break
		}
	}
	if candidate == "" {
		candidate = trimmed
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return FleetBootstrapResult{}, false
	}
	res := FleetBootstrapResult{}
	if v, ok := raw["customer_id"].(string); ok {
		res.CustomerID = v
	}
	if v, ok := raw["hostname"].(string); ok {
		res.Hostname = v
	}
	if v, ok := raw["state"].(string); ok {
		res.State = v
	}
	if v, ok := raw["job_id"].(string); ok {
		res.JobID = v
	}
	if res.JobID == "" {
		if v, ok := raw["deployment_job_id"].(string); ok {
			res.JobID = v
		}
	}
	return res, res.CustomerID != "" || res.Hostname != "" || res.JobID != "" || res.State != ""
}
