// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-fleet-sign is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-fleet-sign was renamed to metrum-ai-router-fleet-sign; install and invoke metrum-ai-router-fleet-sign instead")
	os.Exit(2)
}
