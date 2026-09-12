// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-genai-smartrouterctl is a source-only rename notice; it is not packaged.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-genai-smartrouterctl was renamed to metrum-ai-routerctl; install and invoke metrum-ai-routerctl instead")
	os.Exit(2)
}
