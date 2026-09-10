// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-migrate is a one-release compatibility notice for the renamed migrate CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-migrate was renamed to metrum-router-migrate; install and invoke metrum-router-migrate instead")
	os.Exit(2)
}
