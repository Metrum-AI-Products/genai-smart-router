// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-genai-customer-lifecycle is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-genai-customer-lifecycle was renamed to metrum-ai-router-customer-lifecycle; install and invoke metrum-ai-router-customer-lifecycle instead")
	os.Exit(2)
}
