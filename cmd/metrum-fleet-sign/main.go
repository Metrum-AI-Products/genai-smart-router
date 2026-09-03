// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// metrum-fleet-sign is a one-release compatibility notice for the renamed Fleet signing CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-fleet-sign was renamed to metrum-genai-smartrouter-fleet-sign; install and invoke metrum-genai-smartrouter-fleet-sign instead")
	os.Exit(2)
}
