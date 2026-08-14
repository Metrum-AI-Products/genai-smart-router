package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type planPayload struct {
	JobID           string   `json:"job_id"`
	Hostname        string   `json:"hostname"`
	Namespace       string   `json:"namespace"`
	ProfileID       string   `json:"profile_id"`
	Environment     string   `json:"environment"`
	DatabaseID      string   `json:"database_id"`
	DatabaseProfile string   `json:"database_profile"`
	ManifestSHA256  string   `json:"manifest_sha256"`
	Actions         []string `json:"actions"`
	State           string   `json:"state"`
	ErrorClass      string   `json:"error_class"`
	ObservedState   any      `json:"observed_state"`
	ConfigRevision  string   `json:"config_revision"`
	CustomerID      string   `json:"customer_id"`
	Stage           string   `json:"stage"`
}

type intentEnvelope struct {
	IntentID   string `json:"intent_id"`
	ProfileRef string `json:"profile_ref"`
	Manifest   struct {
		CustomerID       string `json:"customer_id"`
		Stage            string `json:"stage"`
		RuntimeBundleRef string `json:"runtime_bundle_ref"`
		ConfigRevision   string `json:"config_revision"`
		License          struct {
			RequestRef string `json:"request_ref"`
		} `json:"license"`
		DatabaseProfile string `json:"database_profile"`
	} `json:"manifest"`
}

func planSelectsDedicatedRDS(plan planPayload) bool {
	if strings.TrimSpace(plan.DatabaseID) != "" {
		return true
	}
	if strings.TrimSpace(plan.DatabaseProfile) != "" {
		return true
	}
	for _, action := range plan.Actions {
		if action == "dedicated_rds" {
			return true
		}
	}
	return false
}

func resolvePackagedBinary(name string) (string, error) {
	if envDir := strings.TrimSpace(os.Getenv("METRUM_FLEET_BIN_DIR")); envDir != "" {
		candidate := filepath.Join(envDir, name)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), name)
		if isExecutable(sibling) {
			return sibling, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("missing packaged Fleet binary %s; install from a release binary package, set METRUM_FLEET_BIN_DIR, or place it next to this binary", name)
}

func resolveFleetctlBinary() (string, error) {
	if exe, err := os.Executable(); err == nil && isExecutable(exe) {
		return exe, nil
	}
	return resolvePackagedBinary(fleetBinaryName)
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode()&0o111 != 0
}

func utcStamp() string {
	return time.Now().UTC().Format("20060102t150405z")
}

// rejectForeignDefaultRefs fails closed when a customer alias is paired with
// an ACME-rehearsal license path that cannot authorize that alias. Mutation
// still requires an externally signed intent; these checks only catch obvious
// mismatched rehearsal references before the signing boundary.
func rejectForeignDefaultRefs(customerID, _, _, licenseRef string) {
	if err := foreignDefaultRefError(customerID, licenseRef); err != nil {
		die("%v", err)
	}
}

func foreignDefaultRefError(customerID, licenseRef string) error {
	customerID = strings.ToLower(strings.TrimSpace(customerID))
	licenseRef = strings.ToLower(strings.TrimSpace(licenseRef))
	if strings.Contains(licenseRef, "/acme-rehearsal/") && customerID != "acme-rehearsal" {
		return fmt.Errorf("license-ref binds acme-rehearsal; refusing for customer alias %q", customerID)
	}
	return nil
}

// writeManifest prepares an unsigned SQLite-only deployment manifest for the
// customer convenience path. It never emits database_profile; dedicated RDS
// remains a core Fleet deploy choice with an external admission, not a
// customer-verb input.
func writeManifest(ws customerWorkspace, profileRef, runtimeBundle, licenseRef, revisionPrefix string) string {
	profileRef = strings.TrimSpace(profileRef)
	runtimeBundle = strings.TrimSpace(runtimeBundle)
	licenseRef = strings.TrimSpace(licenseRef)
	if profileRef == "" || runtimeBundle == "" || licenseRef == "" {
		die("write-manifest requires --profile-ref, --runtime-bundle-ref, and --license-ref")
	}
	if !strings.HasPrefix(profileRef, "aws-ssm:///") {
		die("profile-ref must be a protected aws-ssm:/// reference")
	}
	if !strings.HasPrefix(runtimeBundle, "aws-ssm:///") && !strings.HasPrefix(runtimeBundle, "aws-secretsmanager:///") {
		die("runtime-bundle-ref must be aws-ssm:/// or aws-secretsmanager:///")
	}
	if !strings.HasPrefix(licenseRef, "aws-ssm:///") && !strings.HasPrefix(licenseRef, "aws-secretsmanager:///") {
		die("license-ref must be aws-ssm:/// or aws-secretsmanager:///")
	}
	rejectForeignDefaultRefs(ws.CustomerID, profileRef, runtimeBundle, licenseRef)

	stamp := utcStamp()
	if strings.TrimSpace(revisionPrefix) == "" {
		revisionPrefix = "sqlite"
	}
	manifest := map[string]any{
		"api_version":        "metrum.ai/smartrouter-deployment/v1",
		"customer_id":        ws.CustomerID,
		"stage":              "nonproduction",
		"release":            "latest-approved",
		"resource_profile":   "small",
		"state_profile":      "sqlite-rwo-small",
		"runtime_bundle_ref": runtimeBundle,
		"config_revision":    fmt.Sprintf("%s-%s-%s", ws.CustomerID, revisionPrefix, stamp),
		"license": map[string]string{
			"request_ref": licenseRef,
			"validity":    "168h",
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		die("encode manifest: %v", err)
	}
	path := filepath.Join(ws.Home, "manifest.json")
	writeMode0600(path, append(raw, '\n'))
	intentID := fmt.Sprintf("intent-%s-%s", ws.CustomerID, stamp)
	writeMode0600(filepath.Join(ws.Home, "intent-id.txt"), []byte(intentID+"\n"))
	ws.saveState(map[string]any{
		"customer_id":        ws.CustomerID,
		"profile_ref":        profileRef,
		"runtime_bundle_ref": runtimeBundle,
		"license_ref":        licenseRef,
		"intent_id":          intentID,
		"manifest_path":      path,
		"awaiting_signature": true,
	})
	fmt.Println(mustJSON(map[string]any{
		"customer_id":        ws.CustomerID,
		"intent_id":          intentID,
		"manifest_written":   true,
		"awaiting_signature": true,
		"next_step":          "obtain an externally issued signed intent for this manifest, then run customer create --intent <signed-intent>",
		"signing_boundary":   "fleetctl customer never holds, copies, or accepts lifecycle private keys",
		"stage":              "nonproduction",
		"state_profile":      "sqlite-rwo-small",
	}))
	return path
}

func peekIntentEnvelope(intentPath string) intentEnvelope {
	requireMode0600File(intentPath, "intent")
	raw, err := os.ReadFile(intentPath)
	if err != nil {
		die("read intent: %v", err)
	}
	env, err := parseIntentEnvelope(raw)
	if err != nil {
		die("%v", err)
	}
	return env
}

func parseIntentEnvelope(raw []byte) (intentEnvelope, error) {
	var env intentEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return intentEnvelope{}, fmt.Errorf("intent JSON is invalid")
	}
	if err := validateIntentEnvelope(env); err != nil {
		return intentEnvelope{}, err
	}
	return env, nil
}

// validateIntentEnvelope enforces the SQLite-only customer convenience path:
// dedicated RDS intents must use core Fleet deploy with an external admission.
func validateIntentEnvelope(env intentEnvelope) error {
	if strings.TrimSpace(env.Manifest.CustomerID) == "" {
		return fmt.Errorf("intent manifest.customer_id is required")
	}
	if strings.TrimSpace(env.ProfileRef) == "" {
		return fmt.Errorf("intent profile_ref is required")
	}
	if strings.TrimSpace(env.Manifest.RuntimeBundleRef) == "" {
		return fmt.Errorf("intent manifest.runtime_bundle_ref is required")
	}
	if strings.TrimSpace(env.Manifest.License.RequestRef) == "" {
		return fmt.Errorf("intent manifest.license.request_ref is required")
	}
	if strings.TrimSpace(env.Manifest.DatabaseProfile) != "" {
		return fmt.Errorf("refusing customer convenience path: intent selects dedicated RDS; use core Fleet deploy with a validated admission")
	}
	stage := strings.ToLower(strings.TrimSpace(env.Manifest.Stage))
	if stage != "" && stage != "nonproduction" && stage != "test" && stage != "staging" {
		return fmt.Errorf("refusing customer convenience path: intent stage %q is not non-production", env.Manifest.Stage)
	}
	return nil
}

func runLogged(cmd *exec.Cmd) (stdout, stderr string, exitCode int) {
	fmt.Println("+", strings.Join(cmd.Args, " "))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 1
			if errBuf.Len() == 0 {
				errBuf.WriteString(err.Error())
			}
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func requireOK(stdout, stderr string, exitCode int, what string) {
	if exitCode != 0 {
		os.Stderr.WriteString(stdout)
		os.Stderr.WriteString(stderr)
		die("%s failed (exit %d)", what, exitCode)
	}
}

func fleetctlJSON(fleetBin string, args []string, env map[string]string, allowFail bool) (map[string]any, int) {
	cmd := exec.Command(fleetBin, args...)
	if env != nil {
		cmd.Env = envMapToSlice(env)
	}
	stdout, stderr, code := runLogged(cmd)
	text := strings.TrimSpace(stdout)
	if text == "" {
		os.Stderr.WriteString(stderr + "\n")
		if allowFail {
			return map[string]any{"_exit": code, "_error": firstNonEmpty(stderr, "no_json")}, code
		}
		die("fleetctl %s produced no JSON (exit %d)", strings.Join(args, " "), code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		os.Stderr.WriteString(stdout)
		os.Stderr.WriteString(stderr + "\n")
		if allowFail {
			return map[string]any{"_exit": code, "_error": firstNonEmpty(stderr, "bad_json")}, code
		}
		die("fleetctl %s returned non-JSON", strings.Join(args, " "))
	}
	if code != 0 && !allowFail {
		safe := map[string]any{}
		for _, k := range []string{"job_id", "state", "error_class", "observed_state"} {
			if v, ok := payload[k]; ok {
				safe[k] = v
			}
		}
		fmt.Println(mustJSON(safe))
		die("fleetctl %s failed (exit %d)", strings.Join(args, " "), code)
	}
	if code != 0 {
		payload["_exit"] = code
		payload["_error"] = stderr
	}
	return payload, code
}

func decodePlan(payload map[string]any) planPayload {
	raw, err := json.Marshal(payload)
	if err != nil {
		die("encode plan payload: %v", err)
	}
	var plan planPayload
	if err := json.Unmarshal(raw, &plan); err != nil {
		die("decode plan payload: %v", err)
	}
	return plan
}

func currentBundleRef(ws customerWorkspace, defaultRef string) string {
	state := ws.loadState()
	if ref, _ := state["runtime_bundle_ref"].(string); strings.TrimSpace(ref) != "" {
		return ref
	}
	return defaultRef
}

func planAndDeployIntent(ws customerWorkspace, fleetBin, intentPath string, fleetEnv map[string]string) (planPayload, map[string]any) {
	requireMode0600File(intentPath, "intent")
	// Preserve a workspace copy for later signed delete (same bytes, mode 0600).
	workspaceIntent := filepath.Join(ws.Home, "intent.json")
	if absIn, err1 := filepath.Abs(intentPath); err1 == nil {
		if absOut, err2 := filepath.Abs(workspaceIntent); err2 == nil && absIn != absOut {
			raw, err := os.ReadFile(intentPath)
			if err != nil {
				die("read intent: %v", err)
			}
			writeMode0600(workspaceIntent, raw)
		}
	} else {
		raw, err := os.ReadFile(intentPath)
		if err != nil {
			die("read intent: %v", err)
		}
		writeMode0600(workspaceIntent, raw)
	}
	intentPath = workspaceIntent

	planRaw, _ := fleetctlJSON(fleetBin, []string{"plan", "--intent", intentPath, "--output", "json"}, fleetEnv, false)
	plan := decodePlan(planRaw)
	if planSelectsDedicatedRDS(plan) {
		die("refusing deploy: plan selected dedicated RDS; omit database_profile for SQLite")
	}
	if strings.TrimSpace(plan.CustomerID) != "" && plan.CustomerID != ws.CustomerID {
		die("intent customer_id %q does not match workspace %q", plan.CustomerID, ws.CustomerID)
	}
	env := strings.ToLower(strings.TrimSpace(plan.Environment))
	if env != "" && env != "nonproduction" && env != "test" && env != "staging" {
		die("refusing deploy: plan environment %q is not non-production", plan.Environment)
	}
	planBytes, err := json.MarshalIndent(planRaw, "", "  ")
	if err != nil {
		die("encode plan.json: %v", err)
	}
	writeMode0600(filepath.Join(ws.Home, "plan.json"), append(planBytes, '\n'))
	fmt.Println(mustJSON(map[string]any{
		"customer_id": ws.CustomerID,
		"hostname":    plan.Hostname,
		"namespace":   plan.Namespace,
		"job_id":      plan.JobID,
		"actions":     plan.Actions,
		"database":    "sqlite",
		"environment": plan.Environment,
		"binding":     "signed-intent",
	}))

	status, _ := fleetctlJSON(fleetBin, []string{
		"deploy", "--intent", intentPath, "--registry", sharedRegistryPath(), "--output", "json",
	}, fleetEnv, false)
	state, _ := status["state"].(string)
	if state != "ready" {
		die("deploy ended with state=%v error_class=%v", status["state"], status["error_class"])
	}
	jobID, _ := status["job_id"].(string)
	if strings.TrimSpace(jobID) == "" {
		jobID = plan.JobID
	}
	envEnvelope := peekIntentEnvelope(intentPath)
	ws.saveState(map[string]any{
		"customer_id":        ws.CustomerID,
		"hostname":           plan.Hostname,
		"namespace":          plan.Namespace,
		"job_id":             jobID,
		"runtime_bundle_ref": envEnvelope.Manifest.RuntimeBundleRef,
		"profile_ref":        envEnvelope.ProfileRef,
		"license_ref":        envEnvelope.Manifest.License.RequestRef,
		"intent_id":          envEnvelope.IntentID,
		"awaiting_signature": false,
	})
	return plan, status
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
