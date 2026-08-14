package main

import (
	"encoding/json"
	"fmt"
	"io"
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

var donorKeyCustomers = []string{"disposable-e2e", "acme-rehearsal", "acme2", "acme3"}

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

func (w customerWorkspace) privateKeyPath() string {
	return filepath.Join(w.Home, "lifecycle_approval_private_key.b64")
}

func (w customerWorkspace) publicKeyPath() string {
	return filepath.Join(w.Home, "lifecycle_approval_public_key.b64")
}

func (w customerWorkspace) ensureKeys() (privPath, pubPath string) {
	priv := w.privateKeyPath()
	pub := w.publicKeyPath()
	if fileExists(priv) && fileExists(pub) {
		return priv, pub
	}
	root := fleetRoot()
	for _, donorName := range donorKeyCustomers {
		donor := filepath.Join(root, donorName)
		dpriv := filepath.Join(donor, "lifecycle_approval_private_key.b64")
		dpub := filepath.Join(donor, "lifecycle_approval_public_key.b64")
		dseed := filepath.Join(donor, "lifecycle-ed25519.seed")
		if fileExists(dpriv) && fileExists(dpub) {
			if err := os.MkdirAll(w.Home, 0o700); err != nil {
				die("create customer workspace: %v", err)
			}
			_ = os.Chmod(w.Home, 0o700)
			copyFileMode(dpriv, priv, 0o600)
			copyFileMode(dpub, pub, 0o600)
			return priv, pub
		}
		if fileExists(dseed) && fileExists(dpub) {
			if err := os.MkdirAll(w.Home, 0o700); err != nil {
				die("create customer workspace: %v", err)
			}
			_ = os.Chmod(w.Home, 0o700)
			copyFileMode(dseed, priv, 0o600)
			copyFileMode(dpub, pub, 0o600)
			return priv, pub
		}
	}
	die("missing lifecycle approval keys under ~/.local/share/metrum-fleet/")
	return "", ""
}

func sharedRegistryPath() string {
	root := filepath.Join(fleetRoot(), "registry")
	if err := os.MkdirAll(root, 0o700); err != nil {
		die("create fleet registry directory: %v", err)
	}
	return filepath.Join(root, "tenant-deployments.sqlite")
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

func copyFileMode(src, dst string, mode os.FileMode) {
	in, err := os.Open(src)
	if err != nil {
		die("open donor key: %v", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		die("write key: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		die("copy key: %v", err)
	}
	_ = out.Chmod(mode)
}

func writeMode0600(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		die("write %s: %v", filepath.Base(path), err)
	}
	_ = os.Chmod(path, 0o600)
}
