package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type customerCommonFlags struct {
	CustomerID       string
	ProfileRef       string
	RuntimeBundleRef string
	LicenseRef       string
}

func fleetCustomer(args []string) {
	if len(args) == 0 {
		die("usage: metrum-genai-smartrouter-fleetctl customer <write-manifest|create|status|smoke|grant-caller|update-config|delete> [flags]")
	}
	switch args[0] {
	case "write-manifest":
		customerWriteManifest(args[1:])
	case "create":
		customerCreate(args[1:])
	case "status":
		customerStatus(args[1:])
	case "smoke":
		customerSmoke(args[1:])
	case "grant-caller":
		customerGrantCaller(args[1:])
	case "update-config":
		customerUpdateConfig(args[1:])
	case "delete":
		customerDelete(args[1:])
	default:
		die("unsupported customer command %q", args[0])
	}
}

func addCustomerCommonFlags(fs *flag.FlagSet, c *customerCommonFlags) {
	fs.StringVar(&c.CustomerID, "customer-id", "", "customer id (required)")
	fs.StringVar(&c.ProfileRef, "profile-ref", "", "protected Fleet profile reference (required for write-manifest / prepare)")
	fs.StringVar(&c.RuntimeBundleRef, "runtime-bundle-ref", "", "runtime bundle reference (required for write-manifest / prepare)")
	fs.StringVar(&c.LicenseRef, "license-ref", "", "license request reference (required for write-manifest / prepare)")
}

func requireCustomerID(c customerCommonFlags) customerWorkspace {
	if strings.TrimSpace(c.CustomerID) == "" {
		die("--customer-id is required")
	}
	return prepareCustomerWorkspace(c.CustomerID)
}

func requireExplicitRefs(c customerCommonFlags) {
	if strings.TrimSpace(c.ProfileRef) == "" || strings.TrimSpace(c.RuntimeBundleRef) == "" || strings.TrimSpace(c.LicenseRef) == "" {
		die("--profile-ref, --runtime-bundle-ref, and --license-ref are required (no ACME/staging defaults)")
	}
}

func customerWriteManifest(args []string) {
	fs := flag.NewFlagSet("customer write-manifest", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	revisionPrefix := fs.String("revision-prefix", "sqlite", "config_revision prefix segment")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	writeManifest(ws, common.ProfileRef, common.RuntimeBundleRef, common.LicenseRef, *revisionPrefix)
}

func customerCreate(args []string) {
	fs := flag.NewFlagSet("customer create", flag.ExitOnError)
	customerID := fs.String("customer-id", "", "optional; must match signed intent when set")
	intentPath := fs.String("intent", "", "mode-0600 externally signed Fleet intent (required)")
	deleteFirst := fs.Bool("delete-first", false, "run signed delete for an existing workspace job before create")
	confirmFile := fs.String("confirm-file", "", "mode-0600 externally signed delete approval (required with --delete-first)")
	rdsAdmission := fs.String("rds-admission-file", "", "mode-0600 externally issued RDS admission when deleting a dedicated-RDS job")
	_ = fs.Parse(args)
	if strings.TrimSpace(*intentPath) == "" {
		die("customer create requires --intent (externally signed); use write-manifest then the approved signing workflow")
	}
	env := peekIntentEnvelope(*intentPath)
	id := env.Manifest.CustomerID
	if strings.TrimSpace(*customerID) != "" {
		normalized, err := normalizeCustomerID(*customerID)
		if err != nil {
			die("%v", err)
		}
		if normalized != id {
			die("--customer-id %q does not match intent customer_id %q", normalized, id)
		}
	}
	ws := prepareCustomerWorkspace(id)
	if *deleteFirst {
		if strings.TrimSpace(*confirmFile) == "" {
			die("--delete-first requires --confirm-file (externally signed delete approval)")
		}
		if err := customerDeleteBestEffort(ws, false, filepath.Join(ws.Home, "intent.json"), *confirmFile, *rdsAdmission); err != nil {
			fmt.Fprintf(os.Stderr, "delete-first: %v\n", err)
		}
	}
	fleetBin, err := resolveFleetctlBinary()
	if err != nil {
		die("%v", err)
	}
	ctx := context.Background()
	_, fleetEnv, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	plan, _ := planAndDeployIntent(ws, fleetBin, *intentPath, fleetEnv)
	hostname := plan.Hostname
	if hostname == "" {
		hostname = customerHostname(ws.CustomerID)
	}
	fmt.Printf("SQLite create ready: https://%s/readyz\n", hostname)
}

func customerStatus(args []string) {
	fs := flag.NewFlagSet("customer status", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	fleetBin, err := resolveFleetctlBinary()
	if err != nil {
		die("%v", err)
	}
	ctx := context.Background()
	_, fleetEnv, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	state := ws.loadState()
	plan := loadWorkspacePlan(ws)
	jobID, _ := state["job_id"].(string)
	if strings.TrimSpace(jobID) == "" {
		jobID = plan.JobID
	}
	profileRef := strings.TrimSpace(common.ProfileRef)
	if profileRef == "" {
		if fromState, _ := state["profile_ref"].(string); strings.TrimSpace(fromState) != "" {
			profileRef = fromState
		}
	}
	if strings.TrimSpace(profileRef) == "" {
		die("--profile-ref is required (or run create first so lifecycle state records it)")
	}
	if strings.TrimSpace(jobID) == "" {
		die("no job_id in lifecycle state or plan.json; run create first")
	}
	status, _ := fleetctlJSON(fleetBin, []string{
		"status",
		"--profile-ref", profileRef,
		"--job", jobID,
		"--registry", sharedRegistryPath(),
		"--output", "json",
	}, fleetEnv, false)
	safe := map[string]any{}
	for _, k := range []string{"job_id", "state", "hostname", "namespace", "observed_state", "error_class", "config_revision", "environment", "customer_id"} {
		if v, ok := status[k]; ok {
			safe[k] = v
		}
	}
	fmt.Println(mustJSON(safe))
}

func customerSmoke(args []string) {
	fs := flag.NewFlagSet("customer smoke", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	tokenFile := fs.String("token-file", "", "caller token file for authenticated smoke")
	model := fs.String("model", "", "model group for optional chat smoke")
	skipChat := fs.Bool("skip-chat", false, "skip chat completion smoke")
	timeoutSec := fs.Int("timeout-sec", 180, "overall smoke deadline seconds")
	expectModels := fs.Int("expect-models-count", -1, "optional exact /v1/models count")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	if err := runCustomerSmoke(ws, smokeOptions{
		TokenFile:         *tokenFile,
		Model:             *model,
		SkipChat:          *skipChat,
		TimeoutSec:        *timeoutSec,
		ExpectModelsCount: *expectModels,
	}); err != nil {
		die("%v", err)
	}
}

func customerGrantCaller(args []string) {
	fs := flag.NewFlagSet("customer grant-caller", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	ownerUser := fs.String("owner-user", "", "caller owner user id")
	project := fs.String("project", "", "caller project")
	envName := fs.String("env", "nonproduction", "caller environment")
	key := fs.String("key", "", "optional key slug")
	allowRaw := fs.String("allow", "", "comma-separated model groups")
	tokenOut := fs.String("token-out", "", "mode-0600 token output path; refuse overwrite")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	if strings.TrimSpace(*ownerUser) == "" || strings.TrimSpace(*project) == "" || strings.TrimSpace(*allowRaw) == "" || strings.TrimSpace(*tokenOut) == "" {
		die("grant-caller requires --owner-user, --project, --allow, and --token-out")
	}
	allow := splitCSV(*allowRaw)
	if len(allow) == 0 {
		die("--allow requires at least one model group")
	}
	tokenGen, err := resolvePackagedBinary(tokenGenBinaryName)
	if err != nil {
		die("%v", err)
	}
	ctx := context.Background()
	fleetCfg, _, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	sourceRef := currentBundleRef(ws, common.RuntimeBundleRef)
	if strings.TrimSpace(sourceRef) == "" {
		die("runtime bundle reference missing; pass --runtime-bundle-ref")
	}
	bundle, err := fetchRuntimeBundle(ctx, fleetCfg, sourceRef)
	if err != nil {
		die("%v", err)
	}
	keyArgs := []string{
		"generate",
		"--owner-user", *ownerUser,
		"--project", *project,
		"--env", *envName,
		"--allow", strings.Join(allow, ","),
		"--format", "json",
	}
	if strings.TrimSpace(*key) != "" {
		keyArgs = append(keyArgs, "--key", *key)
	}
	cmd := exec.Command(tokenGen, keyArgs...)
	stdout, stderr, code := runLogged(cmd)
	requireOK(stdout, stderr, code, "router-token-gen")
	var generated struct {
		Token       string         `json:"token"`
		TokenID     string         `json:"token_id"`
		TokenSHA256 string         `json:"token_sha256"`
		Caller      map[string]any `json:"caller"`
	}
	if err := json.Unmarshal([]byte(stdout), &generated); err != nil {
		die("parse router-token-gen output: %v", err)
	}
	if strings.TrimSpace(generated.Token) == "" {
		die("router-token-gen returned empty token")
	}
	callerID, _ := generated.Caller["id"].(string)
	owner, _ := generated.Caller["owner_user"].(string)
	if owner == "" {
		owner, _ = generated.Caller["user"].(string)
	}
	projectVal, _ := generated.Caller["project"].(string)
	environment, _ := generated.Caller["environment"].(string)
	tokenID, _ := generated.Caller["token_id"].(string)
	if tokenID == "" {
		tokenID = generated.TokenID
	}
	tokenSHA, _ := generated.Caller["token_sha256"].(string)
	if tokenSHA == "" {
		tokenSHA = generated.TokenSHA256
	}
	callerAllow := allow
	if rawAllow, ok := generated.Caller["allow"].([]any); ok && len(rawAllow) > 0 {
		callerAllow = nil
		for _, item := range rawAllow {
			if s, ok := item.(string); ok && s != "" {
				callerAllow = append(callerAllow, s)
			}
		}
	}
	callerRow := map[string]any{
		"id":            callerID,
		"user":          owner,
		"project":       projectVal,
		"environment":   environment,
		"token_id":      tokenID,
		"token_sha256":  tokenSHA,
		"allow":         callerAllow,
		"metrics_admin": false,
	}
	outPath := expandHome(*tokenOut)
	if fileExists(outPath) {
		die("token-out already exists: %s", outPath)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		die("create token-out directory: %v", err)
	}
	writeMode0600(outPath, []byte(generated.Token+"\n"))
	patched, err := applyConfigPatch(bundle.ConfigYAML, map[string]any{"callers": []any{callerRow}})
	if err != nil {
		die("%v", err)
	}
	newRef, err := publishRuntimeBundle(ctx, ws.CustomerID, runtimeBundle{ConfigYAML: patched, EnvJSON: bundle.EnvJSON})
	if err != nil {
		die("%v", err)
	}
	writeManifest(ws, common.ProfileRef, newRef, common.LicenseRef, "grant")
	fmt.Println(mustJSON(map[string]any{
		"granted_caller_id": callerRow["id"],
		"token_file":        outPath,
		"activation":        "signed-intent-required",
		"next_step":         "sign the workspace manifest externally, then: customer create --intent <signed-intent>",
	}))
}

func customerUpdateConfig(args []string) {
	fs := flag.NewFlagSet("customer update-config", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	patchFile := fs.String("patch-file", "", "YAML mapping merged into runtime config.yaml")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	if strings.TrimSpace(*patchFile) == "" {
		die("--patch-file is required")
	}
	patchPath := expandHome(*patchFile)
	raw, err := os.ReadFile(patchPath)
	if err != nil {
		die("patch file not found: %s", patchPath)
	}
	var patch map[string]any
	if err := yaml.Unmarshal(raw, &patch); err != nil || patch == nil {
		die("patch-file must be a YAML mapping")
	}
	ctx := context.Background()
	fleetCfg, _, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	sourceRef := currentBundleRef(ws, common.RuntimeBundleRef)
	if strings.TrimSpace(sourceRef) == "" {
		die("runtime bundle reference missing; pass --runtime-bundle-ref")
	}
	bundle, err := fetchRuntimeBundle(ctx, fleetCfg, sourceRef)
	if err != nil {
		die("%v", err)
	}
	patched, err := applyConfigPatch(bundle.ConfigYAML, patch)
	if err != nil {
		die("%v", err)
	}
	newRef, err := publishRuntimeBundle(ctx, ws.CustomerID, runtimeBundle{ConfigYAML: patched, EnvJSON: bundle.EnvJSON})
	if err != nil {
		die("%v", err)
	}
	writeManifest(ws, common.ProfileRef, newRef, common.LicenseRef, "cfg")
	fmt.Println(mustJSON(map[string]any{
		"activation": "signed-intent-required",
		"next_step":  "sign the workspace manifest externally, then: customer create --intent <signed-intent>",
	}))
}

func customerDelete(args []string) {
	fs := flag.NewFlagSet("customer delete", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	intentPath := fs.String("intent", "", "mode-0600 signed intent from the job to delete")
	confirmFile := fs.String("confirm-file", "", "mode-0600 externally signed delete approval (required)")
	rdsAdmission := fs.String("rds-admission-file", "", "mode-0600 externally issued RDS admission when deleting dedicated RDS")
	retainPVC := fs.Bool("retain-pvc", false, "retain PVC during signed delete")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	intent := strings.TrimSpace(*intentPath)
	if intent == "" {
		intent = filepath.Join(ws.Home, "intent.json")
	}
	if strings.TrimSpace(*confirmFile) == "" {
		die("customer delete requires --confirm-file (externally signed delete approval); fleetctl never signs locally")
	}
	if err := customerDeleteBestEffort(ws, *retainPVC, intent, *confirmFile, *rdsAdmission); err != nil {
		die("%v", err)
	}
}

func customerDeleteBestEffort(ws customerWorkspace, retainPVC bool, intentPath, confirmFile, rdsAdmissionFile string) error {
	intentPath = strings.TrimSpace(intentPath)
	if intentPath == "" {
		intentPath = filepath.Join(ws.Home, "intent.json")
	}
	planPath := filepath.Join(ws.Home, "plan.json")
	if !fileExists(intentPath) || !fileExists(planPath) {
		return fmt.Errorf("delete requires workspace intent.json and plan.json from a prior create")
	}
	requireMode0600File(intentPath, "intent")
	requireMode0600File(confirmFile, "confirm-file")
	if strings.TrimSpace(rdsAdmissionFile) != "" {
		requireMode0600File(rdsAdmissionFile, "rds-admission-file")
	}
	fleetBin, err := resolveFleetctlBinary()
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, fleetEnv, err := assumeFleetRole(ctx, ws)
	if err != nil {
		return err
	}
	plan := loadWorkspacePlan(ws)
	jobID := plan.JobID
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("plan.json missing job_id")
	}
	if retainPVC {
		fmt.Fprintln(os.Stderr, "note: retain-pvc must already be encoded in the externally signed delete approval")
	}
	delArgs := []string{
		"delete",
		"--intent", intentPath,
		"--registry", sharedRegistryPath(),
		"--confirm-file", confirmFile,
		"--output", "json",
	}
	if strings.TrimSpace(plan.DatabaseID) != "" || strings.TrimSpace(plan.DatabaseProfile) != "" {
		if strings.TrimSpace(rdsAdmissionFile) == "" {
			return fmt.Errorf("dedicated-RDS delete requires --rds-admission-file (externally issued)")
		}
		delArgs = append(delArgs, "--rds-admission-file", rdsAdmissionFile)
	}
	result, code := fleetctlJSON(fleetBin, delArgs, fleetEnv, true)
	errBlob := strings.ToLower(fmt.Sprintf("%v %s", result["_error"], mustJSON(result)))
	if code != 0 && (strings.Contains(errBlob, "record not found") || fmt.Sprint(result["error_class"]) == "record_not_found") {
		fmt.Println("registry missing job; re-binding via deploy then delete")
		fleetctlJSON(fleetBin, []string{
			"deploy", "--intent", intentPath, "--registry", sharedRegistryPath(), "--output", "json",
		}, fleetEnv, false)
		result, code = fleetctlJSON(fleetBin, delArgs, fleetEnv, false)
	}
	if code != 0 {
		return fmt.Errorf("delete failed: state=%v error_class=%v", result["state"], result["error_class"])
	}
	hostname := plan.Hostname
	if hostname == "" {
		hostname = customerHostname(ws.CustomerID)
	}
	readyCode, _, _ := httpJSON("GET", "https://"+hostname+"/readyz", "", nil, 15*time.Second)
	fmt.Println(mustJSON(map[string]any{
		"deleted_job_id": jobID,
		"hostname":       hostname,
		"readyz_http":    readyCode,
		"state":          result["state"],
	}))
	ws.saveState(map[string]any{"deleted": true, "deleted_job_id": jobID})
	return nil
}

func loadWorkspacePlan(ws customerWorkspace) planPayload {
	path := filepath.Join(ws.Home, "plan.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return planPayload{}
		}
		die("read plan.json: %v", err)
	}
	var plan planPayload
	if err := json.Unmarshal(raw, &plan); err != nil {
		die("parse plan.json: %v", err)
	}
	return plan
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
