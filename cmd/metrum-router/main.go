// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-router is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-router was renamed to metrum-ai-router; install and invoke metrum-ai-router instead")
	os.Exit(2)
}
