// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package fleet

import "smart-llmrouter/internal/licensecontract"

// These aliases keep Fleet's inventory API readable while the wire contract
// remains owned by a neutral, dependency-free package.
type LicenseSafeSummary = licensecontract.SafeSummary
type LicenseLimits = licensecontract.Limits
type LicenseDeployment = licensecontract.Deployment

const LicenseIssuer = licensecontract.Issuer
