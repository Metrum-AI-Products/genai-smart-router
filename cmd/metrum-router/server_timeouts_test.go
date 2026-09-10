// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestDefaultHTTPServerTimeoutsPositive(t *testing.T) {
	read, write, idle := defaultHTTPServerTimeouts()
	if read <= 0 || write <= 0 || idle <= 0 {
		t.Fatalf("timeouts must be positive: read=%v write=%v idle=%v", read, write, idle)
	}
}
