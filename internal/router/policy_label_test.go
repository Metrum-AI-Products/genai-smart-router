// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"strings"
	"testing"
)

func TestSafePolicyClassLabel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		nil  bool
	}{
		{name: "empty", in: " ", nil: true},
		{name: "safe", in: "prompt-size:cheap", want: "prompt-size:cheap"},
		{name: "html", in: "<script>alert(1)</script>", want: unsafePolicyClassLabel},
		{name: "newline", in: "secret\nvalue", want: unsafePolicyClassLabel},
		{name: "long", in: strings.Repeat("a", 65), want: unsafePolicyClassLabel},
		{name: "prompt text", in: "my SSN is 123-45-6789", want: unsafePolicyClassLabel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safePolicyClassLabel(tt.in)
			if tt.nil {
				if got != nil {
					t.Fatalf("label=%q, want nil", *got)
				}
				return
			}
			if got == nil || *got != tt.want {
				t.Fatalf("label=%v, want %q", got, tt.want)
			}
		})
	}
}
