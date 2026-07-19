# V5 — Admin reports tour (approval package)

**Status:** AWAITING NARRATION APPROVAL  
**Target:** ~70s · 16:9 · Metrum brand · pause 0.75s  

## Complete narration

**Scene 1.** Deployment admins open the browser admin reports with Basic authentication and Casbin roles. Ordinary API callers cannot reach metrics or admin data.

**Scene 2.** Global filters set the time window and project scope. Overview shows cost, latency, and error trends without raw prompts.

**Scene 3.** Savings tabs compare against a baseline model so you can explain chargeback by user, key, group, or project.

**Scene 4.** Performance and traffic-shaping views explain slow clients and upstream pressure. Routing and admission tabs show why targets were chosen or denied.

**Scene 5.** Request drilldown searches by request ID using safe scalars only—never tokens, hashes, or prompt bodies.

## Visual intent

shipped/admin-reports-shell.html — click through Overview, Savings, Latency, Admission, Requests.

## Source notes

Shipped admin reports; metrics_admin isolation.
