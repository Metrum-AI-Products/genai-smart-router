// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-fleet-sign issues protected Fleet documents (intent, admission, delete).
// It is distributed only as a binary-package CLI for Metrum Fleet operators.
// Customer Docker images must not include this binary.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/fleet"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	if len(os.Args) < 2 {
		die("usage: metrum-fleet-sign <intent|admission|delete|--version> ...")
	}
	switch os.Args[1] {
	case "intent":
		signIntent(os.Args[2:])
	case "admission":
		signAdmission(os.Args[2:])
	case "delete":
		signDelete(os.Args[2:])
	default:
		die("unknown command %q", os.Args[1])
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func loadSeed(path string) ed25519.PrivateKey {
	raw, err := os.ReadFile(path)
	if err != nil {
		die("read key: %v", err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		die("key seed invalid")
	}
	return ed25519.NewKeyFromSeed(seed)
}

func signIntent(args []string) {
	// args: key intent_id profile_ref manifest.json out.json hours
	if len(args) != 6 {
		die("intent: <key.b64> <intent_id> <profile_ref> <manifest.json> <out.json> <lifetime_hours>")
	}
	key := loadSeed(args[0])
	var manifest fleet.TenantDeploymentManifest
	data, err := os.ReadFile(args[3])
	if err != nil {
		die("read manifest: %v", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		die("manifest json: %v", err)
	}
	hours, err := time.ParseDuration(args[5] + "h")
	if err != nil {
		die("lifetime: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	intent := fleet.TenantDeploymentIntent{
		APIVersion: fleet.TenantDeploymentIntentAPIVersion,
		IntentID:   args[1],
		IssuerRole: "fleet-lifecycle-admin",
		IssuedAt:   now,
		ExpiresAt:  now.Add(hours),
		ProfileRef: args[2],
		Manifest:   manifest,
	}
	payload, err := fleet.TenantDeploymentIntentSigningPayload(intent)
	if err != nil {
		die("payload: %v", err)
	}
	intent.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	out, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		die("marshal: %v", err)
	}
	if err := os.WriteFile(args[4], out, 0o600); err != nil {
		die("write: %v", err)
	}
}

func signAdmission(args []string) {
	// key approval_id profile_id environment database_profile job_id namespace manifest_sha256 out.json
	if len(args) != 9 {
		die("admission: <key.b64> <approval_id> <profile_id> <environment> <database_profile> <job_id> <namespace> <manifest_sha256> <out.json>")
	}
	key := loadSeed(args[0])
	now := time.Now().UTC().Truncate(time.Second)
	doc := map[string]any{
		"api_version":      fleet.TenantDeploymentRDSAdmissionAPIVersion,
		"action":           fleet.TenantDeploymentRDSAdmissionActionDisposableE2E,
		"approval_id":      args[1],
		"issuer_role":      "fleet-lifecycle-admin",
		"profile_id":       args[2],
		"environment":      args[3],
		"database_profile": args[4],
		"job_id":           args[5],
		"namespace":        args[6],
		"manifest_sha256":  args[7],
		"approved_at":      now,
		"expires_at":       now.Add(12 * time.Hour),
	}
	payload, err := json.Marshal(struct {
		APIVersion      string    `json:"api_version"`
		Action          string    `json:"action"`
		ApprovalID      string    `json:"approval_id"`
		IssuerRole      string    `json:"issuer_role"`
		ProfileID       string    `json:"profile_id"`
		Environment     string    `json:"environment"`
		DatabaseProfile string    `json:"database_profile"`
		JobID           string    `json:"job_id"`
		Namespace       string    `json:"namespace"`
		ManifestSHA256  string    `json:"manifest_sha256"`
		ApprovedAt      time.Time `json:"approved_at"`
		ExpiresAt       time.Time `json:"expires_at"`
	}{
		APIVersion: doc["api_version"].(string), Action: doc["action"].(string),
		ApprovalID: args[1], IssuerRole: "fleet-lifecycle-admin",
		ProfileID: args[2], Environment: args[3], DatabaseProfile: args[4],
		JobID: args[5], Namespace: args[6], ManifestSHA256: args[7],
		ApprovedAt: now, ExpiresAt: now.Add(12 * time.Hour),
	})
	if err != nil {
		die("payload: %v", err)
	}
	doc["signature"] = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		die("marshal: %v", err)
	}
	if err := os.WriteFile(args[8], out, 0o600); err != nil {
		die("write: %v", err)
	}
}

func signDelete(args []string) {
	// key job_id nonce out.json retain_database(true|false) [retain_pvc(true|false)]
	if len(args) != 5 && len(args) != 6 {
		die("delete: <key.b64> <job_id> <nonce> <out.json> <retain_database> [retain_pvc]")
	}
	key := loadSeed(args[0])
	now := time.Now().UTC().Truncate(time.Second)
	retainDB := args[4] == "true"
	retainPVC := false
	if len(args) == 6 {
		retainPVC = args[5] == "true"
	}
	payload, err := json.Marshal(struct {
		APIVersion     string    `json:"api_version"`
		JobID          string    `json:"job_id"`
		Action         string    `json:"action"`
		IssuerRole     string    `json:"issuer_role"`
		ExpiresAt      time.Time `json:"expires_at"`
		RetainPVC      bool      `json:"retain_pvc"`
		RetainDatabase bool      `json:"retain_database"`
		Nonce          string    `json:"nonce"`
	}{
		APIVersion: fleet.TenantDeletionApprovalAPIVersion, JobID: args[1], Action: "delete",
		IssuerRole: "fleet-lifecycle-admin", ExpiresAt: now.Add(12 * time.Hour),
		RetainPVC: retainPVC, RetainDatabase: retainDB, Nonce: args[2],
	})
	if err != nil {
		die("payload: %v", err)
	}
	doc := map[string]any{
		"api_version":     fleet.TenantDeletionApprovalAPIVersion,
		"job_id":          args[1],
		"action":          "delete",
		"issuer_role":     "fleet-lifecycle-admin",
		"expires_at":      now.Add(12 * time.Hour),
		"retain_pvc":      retainPVC,
		"retain_database": retainDB,
		"nonce":           args[2],
		"signature":       base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload)),
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		die("marshal: %v", err)
	}
	if err := os.WriteFile(args[3], out, 0o600); err != nil {
		die("write: %v", err)
	}
}
