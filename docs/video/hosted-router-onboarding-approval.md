# Hosted Router Onboarding Video

Status: **Narration approved 2026-07-19; awaiting TTS selection**

Format: 16:9. Brand: Metrum AI. Pause: 0.75 seconds after every non-final scene.

## Approved narration

### S01

> A customer starts by creating an organization, selecting a subscription plan,
> and completing hosted payment. A verified payment event activates the
> entitlement; a browser redirect alone never does.

### S02

> For the launch tier, Metrum activates a logical tenant on a shared regional EKS
> router fleet. Customers get isolated access, quotas, model-group permissions,
> and usage views without waiting for a separate cluster.

### S03

> A Metrum-managed runtime license is installed before router readiness. The
> customer sees safe activation status, while the signed license and Metrum
> signing keys remain outside the console.

### S04

> Organization owners can invite teammates, create projects, and issue multiple
> scoped downstream API keys. Each key discovers only its permitted model groups
> through slash-v-one-models.

### S05

> To use a private upstream, an owner adds a provider connection. The secret goes
> directly to tenant-scoped vault storage; the router receives only an authorized
> reference, never a visible provider key.

### S06

> The connection validator tests the chosen endpoint and model capabilities.
> Failed authentication, invalid URLs, unsupported API shapes, or unavailable
> models return safe, actionable errors before traffic is activated.

### S07

> Routing changes start as a versioned draft. An imported policy or Config Copilot
> proposal is schema-checked, capability-checked, simulated, reviewed, and
> activated atomically, with rollback to the previous version.

### S08

> Config Copilot can use Codex inside an isolated server-side sandbox with a
> short-lived Metrum credential. It may generate a draft and explain errors, but
> it cannot read secrets or deploy without customer approval.

### S09

> The result is a running, governed router: customers control their upstreams and
> routing policy, while Metrum enforces isolation, licensing, validation, and
> recoverable operations.
