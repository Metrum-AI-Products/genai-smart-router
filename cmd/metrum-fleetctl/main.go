// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// metrum-fleetctl is a one-release compatibility notice for the renamed Fleet CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-fleetctl was renamed to metrum-genai-smartrouter-fleetctl; install and invoke metrum-genai-smartrouter-fleetctl instead")
	os.Exit(2)
}
