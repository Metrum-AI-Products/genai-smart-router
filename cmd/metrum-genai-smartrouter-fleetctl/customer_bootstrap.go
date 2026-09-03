// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
)

func customerBootstrap(args []string) {
	fs := flag.NewFlagSet("customer bootstrap", flag.ExitOnError)
	var common customerCommonFlags
	addCustomerCommonFlags(fs, &common)
	configFile := fs.String("config-file", "", "router config.yaml (required)")
	envFile := fs.String("env-file", "", "router env.json (required)")
	rewritePaths := fs.String("rewrite-paths", "fleet-eks", "path rewrite profile for publish (fleet-eks)")
	signWithKey := fs.String("sign-with-key", "", "mode-0600 lifecycle approval private key (required)")
	ownerUser := fs.String("owner-user", "", "probe caller owner user (required)")
	project := fs.String("project", "", "probe caller project (required)")
	tokenOut := fs.String("token-out", "", "mode-0600 probe token output path (required)")
	smokeModel := fs.String("model", "high", "model group for final smoke")
	allowFromConfig := fs.Bool("allow-from-config", true, "grant probe caller every model group from bundle")
	_ = fs.Parse(args)
	ws := requireCustomerID(common)
	requireExplicitRefs(common)
	if strings.TrimSpace(*configFile) == "" || strings.TrimSpace(*envFile) == "" {
		die("bootstrap requires --config-file and --env-file")
	}
	if strings.TrimSpace(*signWithKey) == "" {
		die("bootstrap requires --sign-with-key")
	}
	if strings.TrimSpace(*ownerUser) == "" || strings.TrimSpace(*project) == "" || strings.TrimSpace(*tokenOut) == "" {
		die("bootstrap requires --owner-user, --project, and --token-out")
	}

	ctx := context.Background()
	configYAML, envJSON, err := loadRuntimeBundleFiles(*configFile, *envFile)
	if err != nil {
		die("%v", err)
	}
	bundle, _, err := transformRuntimeBundle(configYAML, envJSON, bundleTransformOptions{
		StripCallers: true,
		RewritePaths: strings.TrimSpace(*rewritePaths),
		TrimCatalog:  true,
	})
	if err != nil {
		die("%v", err)
	}
	if err := enforceSecretsManagerSizeLimit(bundle); err != nil {
		die("%v", err)
	}
	if _, err := publishRuntimeBundleOperator(ctx, ws.CustomerID, bundle); err != nil {
		die("publish runtime bundle: %v", err)
	}
	writeManifest(ws, common.ProfileRef, customerBundleRef(ws.CustomerID), common.LicenseRef, "bootstrap")
	intentPath := signWorkspaceManifest(ws, *signWithKey)

	fleetBin, err := resolveFleetctlBinary()
	if err != nil {
		die("%v", err)
	}
	_, fleetEnv, err := assumeFleetRole(ctx, ws)
	if err != nil {
		die("%v", err)
	}
	plan, _ := planAndDeployIntent(ws, fleetBin, intentPath, fleetEnv)

	grantArgs := []string{
		"grant-caller",
		"--customer-id", ws.CustomerID,
		"--profile-ref", common.ProfileRef,
		"--runtime-bundle-ref", customerBundleRef(ws.CustomerID),
		"--license-ref", common.LicenseRef,
		"--owner-user", *ownerUser,
		"--project", *project,
		"--token-out", *tokenOut,
	}
	if *allowFromConfig {
		grantArgs = append(grantArgs, "--allow-from-config")
	}
	customerGrantCaller(grantArgs[1:])

	intentPath = signWorkspaceManifest(ws, *signWithKey)
	plan, _ = planAndDeployIntent(ws, fleetBin, intentPath, fleetEnv)

	if err := runCustomerSmoke(ws, smokeOptions{
		TokenFile:         *tokenOut,
		Model:             *smokeModel,
		ExpectModelsCount: smokeExpectModelsUnset,
	}); err != nil {
		die("bootstrap smoke: %v", err)
	}
	fmt.Println(mustJSON(map[string]any{
		"customer_id": ws.CustomerID,
		"hostname":    plan.Hostname,
		"state":       "ready",
		"token_file":  expandHome(*tokenOut),
		"smoke_model": *smokeModel,
	}))
}
