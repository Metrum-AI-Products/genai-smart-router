# Productization Sprint 1 Backlog

Epic: [#520](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/520)
(`productization-sprint-1`). This index is the execution map; GitHub issue bodies
are the implementation contracts and evidence checklists.

| Issue | Launch class | Area labels | Primary dependencies |
| --- | --- | --- | --- |
| #521 Control plane lifecycle | Blocking | `area:control-plane`, `NEEDS-HUMAN` | #7, #507, D1/D6/D7 |
| #522 Balance grants and router admission | Blocking | `area:control-plane`, `area:ledger`, `NEEDS-HUMAN` | #521, #523, #507, D2/D3/D4/D7 |
| #523 Double-entry ledger/settlement | Blocking | `area:ledger`, `NEEDS-HUMAN` | #507, #522 contract, D2/D3/D4 |
| #524 Stripe checkout/webhooks/reconciliation | Blocking | `area:payments`, `NEEDS-HUMAN` | #521-#523, D3/D4/D5/D7 |
| #525 Prosumer console | Blocking | `area:console-ui` | #521-#524 |
| #526 EKS tenant provisioning | Blocking | `area:provisioning`, `NEEDS-HUMAN` | #7, #507, #516-#519, #521 |
| #527 Release engineering | Blocking | `area:cicd` | #507, #516-#519 |
| #528 Experimental model canaries | Fast follow | `area:routing-canary` | #7, #505, #507, #531 |
| #529 Abuse/fraud/cost protection | Blocking | `area:security`, `area:payments` | #521-#524, #531 |
| #530 Security/compliance assessment | Blocking | `area:security` | #521, #524, #526, #527 |
| #531 SLOs/on-call/reconciliation observability | Blocking | `area:observability` | #521-#527 |
| #532 Marketplace/AMI/container readiness | Fast follow | `area:marketplace`, `NEEDS-HUMAN` | #521, #523, #527, D5/D7 |
| #533 Docs/legal/support | Blocking | `area:docs-legal`, `NEEDS-HUMAN` | all customer-visible blockers, D3-D7 |

## Suggested execution order

```text
Resolve D1-D7
  ├─ #507 + #516-#519 ──> #527
  └─ #7 ──> #521 ──> #523 ──> #522 ──> #524 ──> #525
                               │              ├─> #529
                               │              └─> #531 ──> #528
                               └─> #526 ──> #530

#521/#524/#525/#529/#530/#531 ──> #533 launch go/no-go
#521/#523/#527 ──> #532 (fast follow)
```

## Launch evidence standard

Each blocking issue must attach a redacted terminal replay or screenshot/video
showing its exact acceptance flow, its test command/result, a safe observable
(status/ledger/audit/metric/config state), and rollback or failure-path proof.
Never attach card data, raw prompts/images/tools, raw router tokens or hashes,
provider keys, full configuration, or production secrets.

## Existing-roadmap relationship

This backlog deliberately does not edit or close existing work. It extends #7,
#505, #507, #508-#511, #516-#519, and commercial/lease work #159/#169-#171.
Closed #37/#38/#41 are historical, `not_planned` enterprise-first deferrals—not
delivered hosted-product work—and are referenced rather than reopened.
