// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	fleetBinaryName     = "metrum-genai-smartrouter-fleetctl"
	fleetSignBinaryName = "metrum-genai-smartrouter-fleet-sign"
	tokenGenBinaryName  = "router-token-gen"
	hostnameSuffix      = "apps.metrum.ai"
)

var customerIDRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)
var deleteNonceRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type customerWorkspace struct {
	CustomerID string
	Home       string
}

func normalizeCustomerID(raw string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	if !customerIDRE.MatchString(id) {
		return "", fmt.Errorf("customer-id must match ^[a-z][a-z0-9-]{1,30}$")
	}
	return id, nil
}

func fleetRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		die("resolve home directory: %v", err)
	}
	return filepath.Join(home, ".local", "share", "metrum-fleet")
}

func prepareCustomerWorkspace(customerID string) customerWorkspace {
	id, err := normalizeCustomerID(customerID)
	if err != nil {
		die("%v", err)
	}
	home := filepath.Join(fleetRoot(), id)
	if err := os.MkdirAll(home, 0o700); err != nil {
		die("create customer workspace: %v", err)
	}
	_ = os.Chmod(home, 0o700)
	return customerWorkspace{CustomerID: id, Home: home}
}

func (w customerWorkspace) statePath() string {
	return filepath.Join(w.Home, "lifecycle-state.json")
}

func (w customerWorkspace) loadState() map[string]any {
	raw, err := os.ReadFile(w.statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}
		}
		die("read lifecycle state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		die("parse lifecycle state: %v", err)
	}
	if state == nil {
		state = map[string]any{}
	}
	return state
}

func (w customerWorkspace) saveState(updates map[string]any) map[string]any {
	state := w.loadState()
	for k, v := range updates {
		if v == nil {
			continue
		}
		state[k] = v
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		die("encode lifecycle state: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(w.statePath(), raw, 0o600); err != nil {
		die("write lifecycle state: %v", err)
	}
	_ = os.Chmod(w.statePath(), 0o600)
	return state
}

func sharedRegistryPath() string {
	root := filepath.Join(fleetRoot(), "registry")
	if err := os.MkdirAll(root, 0o700); err != nil {
		die("create fleet registry directory: %v", err)
	}
	return filepath.Join(root, "tenant-deployments.sqlite")
}

func defaultRegistryPath() string {
	path := sharedRegistryPath()
	if fileExists(path) {
		return path
	}
	return "./tenant-deployments.sqlite"
}

func customerHostname(customerID string) string {
	return customerID + "." + hostnameSuffix
}

func customerBundleRef(customerID string) string {
	return "aws-secretsmanager:///smartrouter/fleet/customers/" + customerID + "/runtime-bundle"
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func writeMode0600(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		die("write %s: %v", filepath.Base(path), err)
	}
	_ = os.Chmod(path, 0o600)
}

// requireMode0600File refuses world/group-readable protected documents.
func requireMode0600File(path, label string) {
	path = strings.TrimSpace(path)
	if path == "" {
		die("%s is required", label)
	}
	st, err := os.Stat(path)
	if err != nil {
		die("%s not found: %s", label, filepath.Base(path))
	}
	if st.IsDir() {
		die("%s must be a regular file", label)
	}
	if st.Mode().Perm()&0o077 != 0 {
		die("%s must be mode 0600 (got %04o)", label, st.Mode().Perm())
	}
}
