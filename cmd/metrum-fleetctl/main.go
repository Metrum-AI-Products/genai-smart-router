// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-fleetctl is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-fleetctl was renamed to metrum-ai-router-fleetctl; install and invoke metrum-ai-router-fleetctl instead")
	os.Exit(2)
}
