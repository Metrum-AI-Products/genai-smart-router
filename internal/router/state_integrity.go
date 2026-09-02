// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const stateIntegrityVersion = 1

type integrityEnvelope[T any] struct {
	SchemaVersion int       `json:"schema_version"`
	State         T         `json:"state"`
	Integrity     integrity `json:"integrity"`
}

type integrity struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

var errStateIntegrity = errors.New("state integrity check failed")

func marshalIntegrityState[T any](state T, keyID string, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errStateIntegrity
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	env := integrityEnvelope[T]{
		SchemaVersion: stateIntegrityVersion,
		State:         state,
		Integrity: integrity{
			Algorithm: "hmac-sha256",
			KeyID:     keyID,
			Value:     stateMAC(stateRaw, keyID, key),
		},
	}
	return json.MarshalIndent(env, "", "  ")
}

func unmarshalIntegrityState[T any](raw []byte, keyID string, key []byte) (T, bool, error) {
	var zero T
	var env integrityEnvelope[T]
	if err := json.Unmarshal(raw, &env); err != nil {
		return zero, false, err
	}
	if env.SchemaVersion != stateIntegrityVersion || env.Integrity.Algorithm != "hmac-sha256" || env.Integrity.KeyID == "" {
		return zero, false, nil
	}
	if env.Integrity.KeyID != keyID || len(key) == 0 {
		return zero, true, errStateIntegrity
	}
	stateRaw, err := json.Marshal(env.State)
	if err != nil {
		return zero, true, err
	}
	want := stateMAC(stateRaw, env.Integrity.KeyID, key)
	if !hmac.Equal([]byte(want), []byte(env.Integrity.Value)) {
		return zero, true, errStateIntegrity
	}
	return env.State, true, nil
}

func stateMAC(stateRaw []byte, keyID string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(keyID))
	mac.Write([]byte{0})
	mac.Write(stateRaw)
	return hex.EncodeToString(mac.Sum(nil))
}

func stateIntegrityMaterial(parts ...string) ([]byte, string) {
	h := sha256.New()
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return sum, fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:8]))
}
