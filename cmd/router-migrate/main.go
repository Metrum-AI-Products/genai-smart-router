// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// router-migrate is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "router-migrate was renamed to metrum-ai-router-migrate; install and invoke metrum-ai-router-migrate instead")
	os.Exit(2)
}
