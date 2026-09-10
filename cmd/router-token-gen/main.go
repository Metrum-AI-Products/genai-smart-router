// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-token-gen is a one-release compatibility notice for the renamed token CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-token-gen was renamed to metrum-router-token-gen; install and invoke metrum-router-token-gen instead")
	os.Exit(2)
}
