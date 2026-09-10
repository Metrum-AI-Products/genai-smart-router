// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router is a one-release compatibility notice for the renamed runtime binary.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router was renamed to metrum-router; install and invoke metrum-router instead")
	os.Exit(2)
}
