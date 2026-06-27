//go:build dev_no_license

package router

const licenseCompileMode = "dev_no_license"

var licenseRequired = false

func licenseEnforcementRequired() bool {
	return licenseRequired
}
