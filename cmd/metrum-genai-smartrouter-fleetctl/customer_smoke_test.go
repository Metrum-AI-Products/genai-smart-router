// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestCustomerSmokeRequiresExplicitModel(t *testing.T) {
	err := runCustomerSmoke(customerWorkspace{CustomerID: "e2eprobe"}, smokeOptions{TimeoutSec: 1})
	if err == nil || !strings.Contains(err.Error(), "--model is required") {
		t.Fatalf("expected explicit-model error, got %v", err)
	}
}

func TestChatSmokeSucceededRequiresExpectedContent(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		content string
		want    bool
	}{
		{name: "exact success", code: http.StatusOK, content: "OK", want: true},
		{name: "surrounding whitespace", code: http.StatusOK, content: " OK\n", want: true},
		{name: "empty 200", code: http.StatusOK, content: "", want: false},
		{name: "wrong 200", code: http.StatusOK, content: "not ok", want: false},
		{name: "non-200", code: http.StatusBadGateway, content: "OK", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatSmokeSucceeded(tt.code, tt.content); got != tt.want {
				t.Fatalf("chatSmokeSucceeded(%d, %q) = %t, want %t", tt.code, tt.content, got, tt.want)
			}
		})
	}
}

func TestHasModelIDUsesExactDeploymentDefinedGroup(t *testing.T) {
	ids := []string{"customer-code", "customer-fast"}
	if !hasModelID(ids, "customer-code") {
		t.Fatal("expected exact configured group to be visible")
	}
	if hasModelID(ids, "default") {
		t.Fatal("unexpected reference group fallback")
	}
}
