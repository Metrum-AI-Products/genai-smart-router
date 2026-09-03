// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"smart-llmrouter/internal/router"
)

func customerList(args []string) {
	fs := flag.NewFlagSet("customer list", flag.ExitOnError)
	_ = fs.Parse(args)

	rows := map[string]map[string]any{}

	registryPath := sharedRegistryPath()
	if fileExists(registryPath) {
		store, err := router.OpenTenantDeploymentStoreReadOnly(registryPath)
		if err != nil {
			die("open deployment registry: %v", err)
		}
		defer store.Close()
		tenants, err := store.ListFleetTenants(context.Background())
		if err != nil {
			die("list fleet tenants: %v", err)
		}
		for _, tenant := range tenants {
			rows[tenant.CustomerID] = map[string]any{
				"customer_id":    tenant.CustomerID,
				"hostname":       tenant.Hostname,
				"job_id":         tenant.LatestJobID,
				"observed_state": tenant.LatestJobState,
			}
		}
	}

	root := fleetRoot()
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		die("read fleet workspace root: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "registry" {
			continue
		}
		id := entry.Name()
		if !customerIDRE.MatchString(id) {
			continue
		}
		ws := customerWorkspace{CustomerID: id, Home: filepath.Join(root, id)}
		row := rows[id]
		if row == nil {
			row = map[string]any{
				"customer_id": id,
				"hostname":    customerHostname(id),
			}
			rows[id] = row
		}
		mergeWorkspaceListFields(row, ws)
	}

	customers := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		customers = append(customers, row)
	}
	sort.Slice(customers, func(i, j int) bool {
		return customers[i]["customer_id"].(string) < customers[j]["customer_id"].(string)
	})

	fmt.Println(mustJSON(map[string]any{
		"schema":    "metrum.ai/smartrouter-fleet-customer-list/v1",
		"registry":  registryPath,
		"customers": customers,
	}))
}

func mergeWorkspaceListFields(row map[string]any, ws customerWorkspace) {
	plan := readWorkspacePlanOptional(ws)
	state := ws.loadState()

	jobID := firstNonEmpty(plan.JobID, stringFromAny(state["job_id"]), stringFromAny(row["job_id"]))
	if jobID != "" {
		row["job_id"] = jobID
	}
	if hostname := firstNonEmpty(plan.Hostname, customerHostname(ws.CustomerID)); hostname != "" {
		row["hostname"] = hostname
	}
	if deleted, ok := state["deleted"].(bool); ok && deleted {
		row["observed_state"] = "deleted"
		return
	}
	if observed := firstNonEmpty(stringFromAny(state["observed_state"]), stringFromAny(row["observed_state"])); observed != "" {
		row["observed_state"] = observed
	}
}

func readWorkspacePlanOptional(ws customerWorkspace) planPayload {
	raw, err := os.ReadFile(filepath.Join(ws.Home, "plan.json"))
	if err != nil {
		return planPayload{}
	}
	var plan planPayload
	if err := json.Unmarshal(raw, &plan); err != nil {
		return planPayload{}
	}
	return plan
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
