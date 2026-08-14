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

func writeManifest(ws customerWorkspace, runtimeBundle, licenseRef, revisionPrefix string) string {
	stamp := utcStamp()
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
	return path
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

func signIntent(signBin, priv, intentID, profileRef, manifestPath, outPath string) {
	cmd := exec.Command(signBin, "intent", priv, intentID, profileRef, manifestPath, outPath, "12")
	stdout, stderr, code := runLogged(cmd)
	requireOK(stdout, stderr, code, "sign intent")
	_ = os.Chmod(outPath, 0o600)
}

func signDeleteApproval(signBin, priv, jobID, outPath string, retainPVC bool) {
	nonce := fmt.Sprintf("delete-%s-%d", jobID, time.Now().Unix())
	retainPVCArg := "false"
	if retainPVC {
		retainPVCArg = "true"
	}
	cmd := exec.Command(signBin, "delete", priv, jobID, nonce, outPath, "false", retainPVCArg)
	stdout, stderr, code := runLogged(cmd)
	requireOK(stdout, stderr, code, "sign delete approval")
	_ = os.Chmod(outPath, 0o600)
}

func signAdmission(signBin, priv, approvalID, profileID, environment, databaseProfile, jobID, namespace, manifestSHA256, outPath string) {
	cmd := exec.Command(signBin, "admission", priv, approvalID, profileID, environment, databaseProfile, jobID, namespace, manifestSHA256, outPath)
	stdout, stderr, code := runLogged(cmd)
	requireOK(stdout, stderr, code, "sign RDS admission")
	_ = os.Chmod(outPath, 0o600)
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

func planAndDeploy(ws customerWorkspace, fleetBin, signBin, priv, profileRef, runtimeBundle, licenseRef string, fleetEnv map[string]string, revisionPrefix string) (planPayload, map[string]any) {
	manifestPath := writeManifest(ws, runtimeBundle, licenseRef, revisionPrefix)
	intentID := fmt.Sprintf("intent-%s-%s", ws.CustomerID, utcStamp())
	writeMode0600(filepath.Join(ws.Home, "intent-id.txt"), []byte(intentID+"\n"))
	intentPath := filepath.Join(ws.Home, "intent.json")
	signIntent(signBin, priv, intentID, profileRef, manifestPath, intentPath)

	planRaw, _ := fleetctlJSON(fleetBin, []string{"plan", "--intent", intentPath, "--output", "json"}, fleetEnv, false)
	plan := decodePlan(planRaw)
	if planSelectsDedicatedRDS(plan) {
		die("refusing deploy: plan selected dedicated RDS; omit database_profile for SQLite")
	}
	planBytes, err := json.MarshalIndent(planRaw, "", "  ")
	if err != nil {
		die("encode plan.json: %v", err)
	}
	writeMode0600(filepath.Join(ws.Home, "plan.json"), append(planBytes, '\n'))
	fmt.Println(mustJSON(map[string]any{
		"customer_id":        ws.CustomerID,
		"hostname":           plan.Hostname,
		"namespace":          plan.Namespace,
		"job_id":             plan.JobID,
		"actions":            plan.Actions,
		"database":           "sqlite",
		"runtime_bundle_ref": runtimeBundle,
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
	ws.saveState(map[string]any{
		"customer_id":        ws.CustomerID,
		"hostname":           plan.Hostname,
		"namespace":          plan.Namespace,
		"job_id":             jobID,
		"runtime_bundle_ref": runtimeBundle,
		"profile_ref":        profileRef,
		"license_ref":        licenseRef,
		"intent_id":          intentID,
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
