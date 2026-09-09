// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// CK-1: capture enabled without local key material fails closed with a non-secret error.
func TestContentCaptureCK1MissingLocalKeyFailsClosed(t *testing.T) {
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", "")
	t.Setenv("CONTENT_CAPTURE_KMS_KEY", "")
	// Ambiguous provider/router env names must not satisfy content-store key loading.
	t.Setenv("ROUTER_TOKEN", strings.Repeat("11", 32))
	t.Setenv("OPENAI_API_KEY", strings.Repeat("22", 32))
	t.Setenv("ANTHROPIC_API_KEY", strings.Repeat("33", 32))

	_, err := envContentCaptureKeyResolver{}.ResolveContentCaptureKey("any-id")
	if err == nil {
		t.Fatal("ResolveContentCaptureKey() accepted missing content-capture key material")
	}
	if !strings.Contains(err.Error(), "CONTENT_CAPTURE_LOCAL_KEY is required") {
		t.Fatalf("error=%v, want non-secret missing-key message", err)
	}
	msg := err.Error()
	if strings.Contains(msg, strings.Repeat("11", 8)) || strings.Contains(msg, "ROUTER_TOKEN") || strings.Contains(msg, "OPENAI_API_KEY") {
		t.Fatalf("error leaked key material or ambiguous env names: %v", err)
	}
}

// CK-2: invalid key length/encoding is rejected with a non-secret error.
func TestContentCaptureCK2InvalidLocalKeyFailsClosed(t *testing.T) {
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", "not-32-bytes")
	t.Setenv("CONTENT_CAPTURE_KMS_KEY", "")
	_, err := envContentCaptureKeyResolver{}.ResolveContentCaptureKey("any-id")
	if err == nil {
		t.Fatal("ResolveContentCaptureKey() accepted invalid key material")
	}
	if !strings.Contains(err.Error(), "CONTENT_CAPTURE_LOCAL_KEY must be 32-byte hex or base64") {
		t.Fatalf("error=%v, want non-secret invalid-key message", err)
	}
}

// CK-3: encrypt/decrypt round-trip with a test-only 32-byte key from process env.
func TestContentCaptureCK3LocalKeyRoundTrip(t *testing.T) {
	const localKeyID = "test-local-key"
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", strings.Repeat("ab", 32))
	t.Setenv("CONTENT_CAPTURE_KMS_KEY", "")

	resolver := envContentCaptureKeyResolver{}
	key, err := resolver.ResolveContentCaptureKey(localKeyID)
	if err != nil {
		t.Fatalf("ResolveContentCaptureKey() error=%v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length=%d, want 32", len(key))
	}

	ciphertext, nonce, err := encryptContentCaptureValue(resolver, localKeyID, "governed capture plaintext")
	if err != nil {
		t.Fatalf("encryptContentCaptureValue() error=%v", err)
	}
	if ciphertext == "" || nonce == "" {
		t.Fatal("encryptContentCaptureValue() returned empty ciphertext or nonce")
	}
	if ciphertext == "governed capture plaintext" {
		t.Fatal("encryptContentCaptureValue() left plaintext unencrypted")
	}

	got, err := decryptContentCaptureValue(key, localKeyID, ciphertext, nonce)
	if err != nil {
		t.Fatalf("decryptContentCaptureValue() error=%v", err)
	}
	if got != "governed capture plaintext" {
		t.Fatalf("round-trip plaintext=%q, want original", got)
	}
}

// CK-4: YAML loads local_key_id; deprecated kms_key_id remains a rollout alias only.
func TestContentCaptureCK4YAMLLoadsLocalKeyID(t *testing.T) {
	var current ContentCaptureEncryptionConfig
	if err := yaml.Unmarshal([]byte("enabled: true\nlocal_key_id: current-id\n"), &current); err != nil {
		t.Fatalf("Unmarshal current error=%v", err)
	}
	if !current.Enabled || current.LocalKeyID != "current-id" {
		t.Fatalf("current config=%+v", current)
	}

	var legacy ContentCaptureEncryptionConfig
	if err := yaml.Unmarshal([]byte("enabled: true\nkms_key_id: legacy-id\n"), &legacy); err != nil {
		t.Fatalf("Unmarshal legacy error=%v", err)
	}
	if !legacy.Enabled || legacy.LocalKeyID != "legacy-id" {
		t.Fatalf("legacy config=%+v", legacy)
	}

	var preferLocal ContentCaptureEncryptionConfig
	if err := yaml.Unmarshal([]byte("enabled: true\nlocal_key_id: preferred\nkms_key_id: ignored\n"), &preferLocal); err != nil {
		t.Fatalf("Unmarshal preferLocal error=%v", err)
	}
	if preferLocal.LocalKeyID != "preferred" {
		t.Fatalf("preferLocal LocalKeyID=%q, want preferred", preferLocal.LocalKeyID)
	}
}

// CK-5 companion: empty local key id fails closed even when key material is present.
func TestContentCaptureCK5EmptyLocalKeyIDFailsClosed(t *testing.T) {
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", strings.Repeat("cd", 32))
	_, err := envContentCaptureKeyResolver{}.ResolveContentCaptureKey("  ")
	if err == nil {
		t.Fatal("ResolveContentCaptureKey() accepted empty local key id")
	}
	if !strings.Contains(err.Error(), "local key id is required") {
		t.Fatalf("error=%v, want local key id required", err)
	}
}

func TestContentCaptureDeprecatedKMSEnvAlias(t *testing.T) {
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", "")
	t.Setenv("CONTENT_CAPTURE_KMS_KEY", strings.Repeat("ef", 32))
	key, err := envContentCaptureKeyResolver{}.ResolveContentCaptureKey("legacy-id")
	if err != nil {
		t.Fatalf("ResolveContentCaptureKey() error=%v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length=%d, want 32", len(key))
	}
}
