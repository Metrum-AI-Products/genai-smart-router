//go:build !dev_no_license

package router

const licenseCompileMode = "required"

var licenseRequired = true

func licenseEnforcementRequired() bool {
	return licenseRequired
}
