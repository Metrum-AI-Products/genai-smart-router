// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-usage-report is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-usage-report was renamed to metrum-ai-router-usage-report; install and invoke metrum-ai-router-usage-report instead")
	os.Exit(2)
}
