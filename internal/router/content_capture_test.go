// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestContentCaptureLocalKeyRoundTrip(t *testing.T) {
	const localKeyID = "test-local-key"
	// Test-only 32-byte key material supplied through process env (64 hex chars).
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

func TestContentCaptureLocalKeyMissingFailsClosed(t *testing.T) {
	t.Setenv("CONTENT_CAPTURE_LOCAL_KEY", "")
	t.Setenv("CONTENT_CAPTURE_KMS_KEY", "")
	_, err := envContentCaptureKeyResolver{}.ResolveContentCaptureKey("any-id")
	if err == nil {
		t.Fatal("ResolveContentCaptureKey() accepted missing key material")
	}
	if !strings.Contains(err.Error(), "CONTENT_CAPTURE_LOCAL_KEY is required") {
		t.Fatalf("error=%v, want non-secret missing-key message", err)
	}
	msg := err.Error()
	if strings.Contains(msg, strings.Repeat("ab", 8)) || strings.Contains(msg, "governed capture plaintext") {
		t.Fatalf("error leaked key or content material: %v", err)
	}
}

func TestContentCaptureLocalKeyInvalidFailsClosed(t *testing.T) {
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

func TestContentCaptureLocalKeyEmptyIDFailsClosed(t *testing.T) {
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

func TestContentCaptureEncryptionYAMLLocalKeyAndDeprecatedAlias(t *testing.T) {
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
