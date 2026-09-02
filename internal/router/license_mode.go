//go:build !dev_no_license

// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package router

const licenseCompileMode = "required"

var licenseRequired = true

func licenseEnforcementRequired() bool {
	return licenseRequired
}
