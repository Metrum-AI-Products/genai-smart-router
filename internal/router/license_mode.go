// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

const licenseCompileMode = "optional"

var licenseRequired = false

func licenseEnforcementRequired() bool {
	return licenseRequired
}
