//go:build dev_no_license

// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

const licenseCompileMode = "dev_no_license"

var licenseRequired = false

func licenseEnforcementRequired() bool {
	return licenseRequired
}
