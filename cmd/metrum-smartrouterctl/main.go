// metrum-smartrouterctl is a one-release compatibility notice for the renamed fleet CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "metrum-smartrouterctl was renamed to metrum-genai-smartrouter-fleetctl; install and invoke metrum-genai-smartrouter-fleetctl instead")
	os.Exit(2)
}
