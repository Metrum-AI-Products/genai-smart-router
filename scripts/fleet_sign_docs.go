//go:build ignore

// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

// Deprecated entrypoint. Fleet document signing ships only as the packaged
// binary built from ./cmd/metrum-fleet-sign (bin/metrum-fleet-sign in release
// tarballs). Do not distribute this script or require go run on operator hosts.
//
//	metrum-fleet-sign intent|admission|delete ...
//	# or during packaging/dev:
//	go build -o metrum-fleet-sign ./cmd/metrum-fleet-sign
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "scripts/fleet_sign_docs.go is retired; use packaged binary metrum-fleet-sign from ./cmd/metrum-fleet-sign")
	os.Exit(2)
}
