// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import "time"

// defaultHTTPServerTimeouts returns non-zero body/connection timeouts.
// ReadHeaderTimeout remains configured separately on http.Server.
func defaultHTTPServerTimeouts() (read, write, idle time.Duration) {
	return 60 * time.Second, 120 * time.Second, 120 * time.Second
}
