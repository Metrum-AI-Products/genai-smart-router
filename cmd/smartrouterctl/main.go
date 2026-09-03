// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

// smartrouterctl is a one-release compatibility notice for the renamed customer CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "smartrouterctl was renamed to metrum-genai-smartrouterctl; install and invoke metrum-genai-smartrouterctl instead")
	os.Exit(2)
}
