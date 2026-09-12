// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-genai-smartrouter-fleetctl is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-genai-smartrouter-fleetctl was renamed to metrum-ai-router-fleetctl; install and invoke metrum-ai-router-fleetctl instead")
	os.Exit(2)
}
