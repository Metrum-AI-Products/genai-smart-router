// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-token-gen is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-token-gen was renamed to metrum-ai-router-token-gen; install and invoke metrum-ai-router-token-gen instead")
	os.Exit(2)
}
