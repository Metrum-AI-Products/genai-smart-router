// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func customerGetConfig(args []string) {
	fs := flag.NewFlagSet("customer get-config", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	configOut := fs.String("config-out", "", "mode-0600 config.yaml output path")
	envOut := fs.String("env-out", "", "mode-0600 env.json output path")
	force := fs.Bool("force", false, "overwrite existing output files")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	if strings.TrimSpace(*configOut) == "" || strings.TrimSpace(*envOut) == "" {
		die("get-config requires --config-out and --env-out")
	}
	configPath := expandHome(*configOut)
	envPath := expandHome(*envOut)
	if !*force {
		if fileExists(configPath) {
			die("config-out already exists: %s (pass --force to overwrite)", configPath)
		}
		if fileExists(envPath) {
			die("env-out already exists: %s (pass --force to overwrite)", envPath)
		}
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		die("create config-out directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		die("create env-out directory: %v", err)
	}
	writeMode0600(configPath, []byte(bundle.ConfigYAML))
	writeMode0600(envPath, []byte(bundle.EnvJSON))
	groups, err := modelGroupsFromConfig(bundle.ConfigYAML)
	if err != nil {
		die("%v", err)
	}
	fmt.Println(mustJSON(map[string]any{
		"config_sha256":     sha256Hex(bundle.ConfigYAML),
		"env_sha256":        sha256Hex(bundle.EnvJSON),
		"model_group_count": len(groups),
		"config_out":        configPath,
		"env_out":           envPath,
	}))
}

func customerListCallers(args []string) {
	fs := flag.NewFlagSet("customer list-callers", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
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
	rows, err := listCallersFromConfig(bundle.ConfigYAML)
	if err != nil {
		die("%v", err)
	}
	fmt.Println(mustJSON(map[string]any{
		"callers": rows,
		"count":   len(rows),
	}))
}

func customerRevokeCaller(args []string) {
	fs := flag.NewFlagSet("customer revoke-caller", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	callerID := fs.String("caller-id", "", "caller id to revoke")
	status := fs.String("status", "disabled", "caller status: disabled, rotated, expired, or suspended")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	id := strings.TrimSpace(*callerID)
	if id == "" {
		die("revoke-caller requires --caller-id")
	}
	st := strings.ToLower(strings.TrimSpace(*status))
	if !callerRevokeStatuses[st] {
		die("revoke-caller --status must be disabled, rotated, expired, or suspended")
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
	matched, err := callersMatching(bundle.ConfigYAML, []string{id}, nil)
	if err != nil {
		die("%v", err)
	}
	if len(matched) == 0 {
		die("caller id %q not found", id)
	}
	patched, err := applyConfigPatch(bundle.ConfigYAML, map[string]any{
		"callers": []any{map[string]any{"id": id, "status": st}},
	})
	if err != nil {
		die("%v", err)
	}
	newRef, err := publishRuntimeBundleOperator(ctx, ws.CustomerID, runtimeBundle{ConfigYAML: patched, EnvJSON: bundle.EnvJSON})
	if err != nil {
		die("%v", err)
	}
	writeManifest(ws, common.ProfileRef, newRef, common.LicenseRef, "revoke", "")
	fmt.Println(mustJSON(map[string]any{
		"caller_id":  id,
		"status":     st,
		"activation": "signed-intent-required",
		"next_step":  "sign the workspace manifest externally, then: customer create --intent <signed-intent>",
	}))
}

func customerUpdateQuota(args []string) {
	fs := flag.NewFlagSet("customer update-quota", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	var callerIDs repeatableStrings
	var ownerUsers repeatableStrings
	fs.Var(&callerIDs, "caller-id", "caller id (repeatable)")
	fs.Var(&ownerUsers, "owner-user", "update every caller owned by this user (repeatable)")
	rpm := fs.Int("rpm", -1, "rate.rpm")
	tpm := fs.Int("tpm", -1, "rate.tpm")
	concurrent := fs.Int("concurrent", -1, "rate.concurrent")
	dayTokens := fs.Int64("day-tokens", -1, "quota.day.tokens")
	dayRequests := fs.Int64("day-requests", -1, "quota.day.requests")
	monthTokens := fs.Int64("month-tokens", -1, "quota.month.tokens")
	monthRequests := fs.Int64("month-requests", -1, "quota.month.requests")
	softPct := fs.Int("soft-pct", -1, "quota.soft_pct")
	lifetime := fs.Int64("lifetime-tokens", -1, "key.lifetime_tokens")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	if len(callerIDs) == 0 && len(ownerUsers) == 0 {
		die("update-quota requires --caller-id and/or --owner-user")
	}
	patchFields := quotaPatchFields(*rpm, *tpm, *concurrent, *dayTokens, *dayRequests, *monthTokens, *monthRequests, *softPct, *lifetime)
	if len(patchFields) == 0 {
		die("update-quota requires at least one rate or quota flag")
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
	matched, err := callersMatching(bundle.ConfigYAML, callerIDs, ownerUsers)
	if err != nil {
		die("%v", err)
	}
	if len(matched) == 0 {
		die("no callers matched --caller-id / --owner-user")
	}
	for _, user := range ownerUsers {
		found := false
		for _, row := range matched {
			owner := stringField(row, "owner_user")
			if owner == "" {
				owner = stringField(row, "user")
			}
			if strings.EqualFold(owner, user) {
				found = true
				break
			}
		}
		if !found {
			die("owner-user %q has zero callers", user)
		}
	}
	patchCallers := make([]any, 0, len(matched))
	ids := make([]string, 0, len(matched))
	for _, row := range matched {
		id := stringField(row, "id")
		entry := cloneStringMap(patchFields)
		entry["id"] = id
		patchCallers = append(patchCallers, entry)
		ids = append(ids, id)
	}
	patched, err := applyConfigPatch(bundle.ConfigYAML, map[string]any{"callers": patchCallers})
	if err != nil {
		die("%v", err)
	}
	newRef, err := publishRuntimeBundleOperator(ctx, ws.CustomerID, runtimeBundle{ConfigYAML: patched, EnvJSON: bundle.EnvJSON})
	if err != nil {
		die("%v", err)
	}
	writeManifest(ws, common.ProfileRef, newRef, common.LicenseRef, "quota", "")
	fmt.Println(mustJSON(map[string]any{
		"caller_ids": ids,
		"activation": "signed-intent-required",
		"next_step":  "sign the workspace manifest externally, then: customer create --intent <signed-intent>",
	}))
}

func customerQuotaStatus(args []string) {
	fs := flag.NewFlagSet("customer quota-status", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	adminBasicFile := fs.String("admin-basic-file", "", "mode-0600 file containing username:password for admin reports")
	var callerIDs repeatableStrings
	var ownerUsers repeatableStrings
	fs.Var(&callerIDs, "caller-id", "filter by caller id (repeatable)")
	fs.Var(&ownerUsers, "owner-user", "filter by owner user (repeatable)")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	if strings.TrimSpace(*adminBasicFile) == "" {
		die("quota-status requires --admin-basic-file")
	}
	basicPath := expandHome(*adminBasicFile)
	requireMode0600File(basicPath, "admin-basic-file")
	user, pass, err := readBasicAuthFile(basicPath)
	if err != nil {
		die("%v", err)
	}
	hostname := customerQuotaHostname(ws)
	q := url.Values{}
	for _, id := range callerIDs {
		q.Add("caller_id", id)
	}
	for _, owner := range ownerUsers {
		q.Add("owner_user", owner)
	}
	endpoint := "https://" + hostname + "/admin/reports/api/quota-status"
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	code, payload, err := httpJSONBasic("GET", endpoint, user, pass, 30*time.Second)
	if err != nil {
		die("quota-status request failed: %v", err)
	}
	if code != http.StatusOK {
		die("quota-status http=%d", code)
	}
	fmt.Println(string(payload))
}

func quotaPatchFields(rpm, tpm, concurrent int, dayTokens, dayRequests, monthTokens, monthRequests int64, softPct int, lifetime int64) map[string]any {
	out := map[string]any{}
	rate := map[string]any{}
	if rpm >= 0 {
		rate["rpm"] = rpm
	}
	if tpm >= 0 {
		rate["tpm"] = tpm
	}
	if concurrent >= 0 {
		rate["concurrent"] = concurrent
	}
	if len(rate) > 0 {
		out["rate"] = rate
	}
	quota := map[string]any{}
	if dayTokens >= 0 || dayRequests >= 0 {
		day := map[string]any{}
		if dayTokens >= 0 {
			day["tokens"] = dayTokens
		}
		if dayRequests >= 0 {
			day["requests"] = dayRequests
		}
		quota["day"] = day
	}
	if monthTokens >= 0 || monthRequests >= 0 {
		month := map[string]any{}
		if monthTokens >= 0 {
			month["tokens"] = monthTokens
		}
		if monthRequests >= 0 {
			month["requests"] = monthRequests
		}
		quota["month"] = month
	}
	if softPct >= 0 {
		quota["soft_pct"] = softPct
	}
	if len(quota) > 0 {
		out["quota"] = quota
	}
	if lifetime >= 0 {
		out["key"] = map[string]any{"lifetime_tokens": lifetime}
	}
	return out
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func readBasicAuthFile(path string) (string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read admin-basic-file: %w", err)
	}
	line := strings.TrimSpace(string(raw))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	user, pass, ok := strings.Cut(line, ":")
	user = strings.TrimSpace(user)
	if !ok || user == "" || pass == "" {
		return "", "", fmt.Errorf("admin-basic-file must contain username:password")
	}
	return user, pass, nil
}

func customerQuotaHostname(ws customerWorkspace) string {
	state := ws.loadState()
	hostname, _ := state["hostname"].(string)
	if strings.TrimSpace(hostname) == "" {
		hostname = customerHostname(ws.CustomerID)
	}
	return hostname
}

func httpJSONBasic(method, rawURL, user, pass string, timeout time.Duration) (int, []byte, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}
