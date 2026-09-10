// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-usage-report is a one-release compatibility notice for the renamed usage-report CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-usage-report was renamed to metrum-router-usage-report; install and invoke metrum-router-usage-report instead")
	os.Exit(2)
}
