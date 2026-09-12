// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-license is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-license was renamed to metrum-ai-router-license; install and invoke metrum-ai-router-license instead")
	os.Exit(2)
}
