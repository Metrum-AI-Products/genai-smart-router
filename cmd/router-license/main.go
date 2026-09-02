// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// router-license is a one-release compatibility notice for the renamed license CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-license was renamed to metrum-genai-smartrouter-license; install and invoke metrum-genai-smartrouter-license instead")
	os.Exit(2)
}
