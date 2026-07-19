# Personas

Commercial model: **base monthly subscription + model-mix included allowance + x402 overage** ([DECISIONS.md](../../../../DECISIONS.md) D3).

| ID | Persona | Goals | Primary surfaces | Key errors they must understand |
| --- | --- | --- | --- | --- |
| P1 | Prospect | Evaluate fit, choose plan | Pricing page, docs, signup | Region unsupported |
| P2 | Org owner / billing admin | Subscribe, manage Portal, watch allowance/x402 spend | Console billing, Stripe Checkout/Portal | `entitlement-past-due`, invoice failure |
| P3 | Developer / API caller | Ship requests through router | Keys, `/v1/models`, Chat/Responses/Messages, Codex/Claude Code | `payment-required`, `entitlement-inactive`, quota, upstream |
| P4 | Tenant admin (fast-follow) | BYOK, connections, routing studio | #535–#538 mocks | Validation failed, egress denied |
| P5 | Enterprise operator (self-host) | Install package + license, smoke, renew | Binary/Compose/K8s install, license status | License expired/grace |
| P6 | Metrum support | Suspend/resume, safe diagnostics | Support console | Cross-tenant forbidden |
| P7 | Metrum finance | Plan catalog, ledger, Stripe reconcile | #540, #523, #524 | Unbalanced journal, webhook lag |
| P8 | Metrum SRE / on-call | Provision, SLOs, status | #526, #531, release gates | Stuck provision, grant expiry |
| P9 | Metrum security | Isolation, abuse, x402 treasury gates | #529, #530 | Facilitator SSRF, tenant escape |
| P10 | Metrum model ops | Canary, auto-tune, catalog evidence | #528, #541 | Quality gate breach |
| P11 | Deployment admin (shipped) | Operate admin reports | Browser admin (~50 tabs) | 403 metrics-forbidden |

## Access map (how each persona enters)

| Persona | Auth | Entry URL (fictional) |
| --- | --- | --- |
| P1–P3 | OIDC + verified email | `https://console.example.invalid` |
| P4 | Org admin RBAC | Console → Settings / Routing |
| P5 | Host SSH / local | Package + `license.json` path |
| P6–P10 | SSO + MFA, break-glass audit | `https://ops.example.invalid` |
| P11 | HTTP Basic + Casbin | `https://router.example.invalid/admin/` |
