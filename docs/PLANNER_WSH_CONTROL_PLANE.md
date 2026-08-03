# Planner-Owned tmux and WSH Control Plane

This runbook defines the v1 local-host control plane for sprint workers. It is an internal operator procedure, not router runtime configuration and not a customer-facing deployment feature.

## Boundary and ownership

The sprint planner owns one named tmux session on its host. It creates exactly these long-lived windows:

- `wsh-server` for the persistent WSH server.
- `sprint-planner` for one profile-named, tagged WSH planner session running the approved interactive Codex planner prompt.

Each assignment creates one additional `agent-<issue>-<role>` window. That window starts exactly one named WSH worker session and then the profile-approved Codex worker command. The topology is flat:

```text
tmux control session -> WSH planner or worker session -> Codex
```

Do not start a worker-local tmux server or a nested WSH session. V1 workers run on the planner host only; remote workers, cross-host tmux, generic remote shell access, and WSH MCP are out of scope.

The launcher keeps worktree leases, assignment ownership, idempotency, and cleanup in the planner's protected local state. WSH CLI/REST status is a bounded metadata source; raw WSH REST must not be treated as enforcing planner lease or profile authorization policy.

V1 is a cooperative trusted same-UID model: planner and workers under that UID are mutually trusted. Profile/state mode-0600/0700 and WSH/profile/session ownership prevent accidental access and configuration drift, but are not hostile-worker isolation or a security boundary against code running under the same identity. Do not run untrusted agents or code under this identity. A deployment that needs hostile-worker isolation must use a separate authenticated OS or service identity with an appropriate authorization boundary.

## Runtime profile

Copy [`config/planner-wsh-control.example.json`](../config/planner-wsh-control.example.json) into a protected, owner-readable runtime location and adapt it to the installed WSH CLI version and approved Codex commands. The profile is operational configuration and must never be committed with private socket paths, remote URLs, ports, credentials, tokens, or certificate material.

The profile supplies all host-specific values: `profile_id`, logical `planner_owner`, tmux session name, the required opaque `wsh_server_identity`, local state directory, base ref, lease TTL, forbidden WSH environment markers, and the approved command templates. The launcher derives the Git repository from the current directory and discovers registered worktrees through `git worktree list`; it does not contain a host, port, socket, checkout path, URL, or token.

## Deterministic recovery names

All externally discoverable names are stable and persisted in protected state: the profile supplies the tmux session, WSH server instance, and `planner_wsh_session_id`; a worker window is `agent-<issue>-<role>` and its WSH session is `<worker_session_prefix>-<issue>-<role>`. Inputs are lowercase safe names, so a later planner terminal can derive and monitor the same identities from the protected profile plus issue/role without process memory. Profile IDs, worker prefixes, issue, and role must not be changed while a lease exists; the profile must choose unique tmux/WSH names per concurrently active project/profile to avoid collisions. Audit event UUIDs are audit-only and never session identifiers.

For cooperative same-host/shared-filesystem profiles of one repository, the launcher also stores a canonical-worktree keyed lease registry under Git's canonical common directory. An OS lock plus temp-file/file-fsync/atomic-rename/parent-directory-fsync update (where directory fsync is supported) serializes reservations and lifecycle generation values across profile state directories. If a write fails before rename, the previous durable record remains authoritative; if it fails after rename or while syncing the directory, the launcher reports persistence uncertainty and retains the matching lease for recovery rather than guessing or releasing it. Registry corruption, owner/generation mismatch, and ambiguous recovery fail closed; only the owning profile may release its matching lease. This is same-host cooperative correctness, not a hostile-worker security boundary.

The checked-in template grammar uses `wsh -L <profile> server`, the authoritative `wsh -L <profile> identity --json` handshake, `wsh -L <profile> list` for bounded lifecycle discovery, and `wsh -L <profile> kill <name>` for exact cleanup. Before any session inventory, creation, relay, stop, or cleanup, the loader requires exactly one `server_identity` JSON field whose safe opaque value exactly matches `wsh_server_identity`. It never infers identity from a server name, URL, path, DNS name, or token. Missing, mismatch, unavailable, malformed, or duplicate/ambiguous responses fail closed: no alternate profile, stale binding, retry against another server, or WSH command is allowed. Audit records only the safe validation outcome class and disposition, never either identity value or transport/topology material. The profile contains a safe `planner_wsh_session_id`; `scripts/planner_wsh_planner.sh` starts that one named, `planner-<profile>`-tagged session with the supported interactive Codex form and a fixed planner prompt. `scripts/planner_wsh_worker.sh` does the same for a worker with `codex --cd <canonical-worktree> <planner-issued-assignment-prompt>`. Both adapters shell-quote fixed argv and do not accept profile-supplied shell text. Do not add unverified CLI subcommands or replace an adapter with a generic shell command.

The profile must be a planner-owned, regular non-symlink mode-0600 file; the `state_directory` must be a planner-owned, non-symlink mode-0700 directory. The loader rejects shell wrappers, nested tmux, network/listener arguments, non-WSH lifecycle templates, and a worker template that is not exactly the approved adapter. The checked-in `codex_preflight` is the bounded supported `codex --help` form. `codex [PROMPT]` launches the interactive TUI and `codex exec [PROMPT]` is noninteractive; this control plane intentionally uses the interactive form for long-lived tmux/WSH windows. Codex has no `--agent` CLI selector. The planner-issued role is bounded assignment-prompt context only, while Codex reads the repository `AGENTS.md` from the canonical worktree; it does not select a named TOML agent. That prompt explicitly limits intake to the planner, forbids tmux/WSH/WSH-MCP control, requires the assigned external worktree and reporting to the planner, and requires QA to use an independent lease. Any alternate command requires a compatible CLI preflight and a profile-contract test. It stores only planner metadata: assignment ID, branch, opaque worktree fingerprint, lease ID, WSH session ID, window name, lifecycle timestamps, and profile/repository fingerprints. It does not retain terminal transcripts, prompts, environment values, commands typed by a worker, or secrets.

`wsh_status` uses the portable bounded textual `wsh -L <profile> list` surface. The launcher emits `listed` only when that bounded output contains the exact planner-issued session name; assignment and lease association remain planner-state metadata, not WSH identity evidence or authorization. A defensive JSON parser accepts a future compatible adapter response only when its session and assignment fields match the lease, but the checked-in `wsh list` profile does not claim JSON support. Status has an enforced timeout, total output cap, and per-field cap, and never emits terminal content. The profile is administrator-controlled, so its command templates are the approved command allowlist. Owner-readable permissions and state-directory permissions are same-UID accidental-access hygiene, not a local-host authorization boundary against hostile workers. The launcher records the logical `planner_owner` in immutable state and refuses another cooperative profile owner. Workers do not pass raw shell commands to the launcher.

## Planner procedure

Run the launcher from the planner checkout, not from a worker terminal:

```bash
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json bootstrap
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json preflight
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json status
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json attach
```

`bootstrap` starts or reuses the local `wsh-server` window, performs the required identity handshake, and creates the `sprint-planner` session only after an exact match. `attach` also requires a current identity match before it attaches to the existing named session. Detaching and later attaching does not recreate WSH sessions or discard planner metadata.

Do not assume any prior WSH server is running. `preflight` first requires the authoritative server-identity handshake, then uses the bounded list surface to prove exactly one planner session name, and finally runs the installed exact-session read-only `wsh -L <profile> tag <planner-session>` query. It parses only the tag-value substring after WSH's exact `Session '<planner-session>':` delimiter and requires `planner-<profile>` as a complete tag token; a matching session name never authorizes itself. It also runs the bounded `codex --help` acceptance check without launching a worker task. Before any worker lease allocation, `launch-worker` repeats that fresh server and planner identity check and rejects an absent, duplicate, or wrongly tagged planner identity. A failed handshake or preflight never creates a worker window or writer lease. Halt allocation and escalate to the human supervisor/approved recovery controller; do not select another profile, reuse a stale connection, clean up sessions, or retry another endpoint.

Allocate a worker only after the planner has selected an open issue, approved role, and external issue worktree:

```bash
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json \
  launch-worker --issue 573 --role software_engineer --worktree /worktrees/router-issue-573
```

Before terminal creation, the launcher rejects the primary checkout, paths supplied through symlinks, worktrees not registered with the current Git repository, detached worktrees, incompatible base refs, nested WSH environment markers, an unbootstrapped control session, duplicate assignment/session IDs, and any already-leased worktree. A QA role must use its own worktree; sharing an implementation worktree fails as a duplicate writer lease. A TTL only marks an unreleased lease as expired for recovery visibility; it never frees a possibly running writer automatically.

The planner owns profile and lease authorization. Names/tags help discovery but are not authorization. Do not use direct tmux, WSH MCP, or raw WSH terminal-input APIs to bypass the launcher.

## Recovery, cleanup, and rollback

For a finished worker, stop its exact WSH session and remove its exact tmux window through the launcher before reusing the worktree:

```bash
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json \
  release-worker --issue 573 --role software_engineer \
  --outcome merged --cleanup-authorized
```

`release-worker` requires an explicit recorded terminal outcome (`merged` or `abandoned`) and `--cleanup-authorized`; it releases the lease only after the configured exact WSH kill command and exact window removal succeed. A completed `wsh -c` worker may already have removed its own WSH session: in that case the launcher reclaims the lease only after a fresh successful list proves the exact session is absent *and* the exact tmux worker window is absent. Any unavailable list result or remaining window leaves the lease in `release-failed` for planner recovery, rather than freeing a potentially live writer. Shutdown similarly kills only the persisted exact planner WSH session before its exact planner window and refuses an unavailable, duplicate, or wrongly tagged planner identity. Lifecycle audit events are append-only protected JSONL: event ID, timestamp, action/outcome, assignment, lease, and opaque worktree fingerprint. They survive shutdown and intentionally exclude terminal/session content.

Worker launch is monotonic. Before tmux creation is attempted, ordinary template, state, list, or preflight failures roll back only the matching generation-tagged provisional lease. Once the tmux creation call is attempted, the record becomes `start-uncertain` if that call returns an error; it stays leased until `release-worker` proves the exact WSH session and exact tmux window are both absent. Do not delete state or registry files to force reuse.

After every worker lease is released, shut the plane down explicitly:

```bash
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json \
  shutdown --confirm-shutdown
```

Shutdown refuses active leases and refuses to kill a tmux session containing an unexpected window. This makes rollback simple and local: stop/release the affected worker, shut down the two control windows, correct the protected profile, then bootstrap again. Never roll back by deleting the state directory while a WSH process may still be running. For an orphaned or stale process, require planner/human authorization, record the safe lifecycle evidence, terminate the exact process/session, then use the normal release flow.

If shutdown fails after the repository drain begins, the durable drain record blocks all profiles from allocating a new writer. It is intentionally not cleared by a retry of `shutdown`. After planner cleanup authority has been recorded, resume only that exact drain with:

```bash
python3 scripts/planner_wsh_control.py --profile /protected/planner-wsh-profile.json \
  recover-shutdown --cleanup-authorized
```

Recovery is owner-profile bound and refuses a different profile, a legacy/ambiguous drain, or any shared worker lease. It repeats exact planner WSH and tmux cleanup, then clears the drain only after a fresh bounded result proves the named planner session is absent and the exact tmux control session is absent. The sole exception is a list-surface outage after the launcher has durably recorded the exact owner/profile/drain-generation/registry-generation/planner-session absence proof and the exact `wsh-server` window is confirmed gone; tag-query failures, duplicate, wrong, malformed, or extra proofs retain the drain. Never manually clear the drain, delete the registry, or infer that a same-UID process is harmless. This remains a cooperative same-host/shared-filesystem procedure, not cross-host coordination or hostile-worker containment.

## Worker and QA instructions

Workers receive only the planner-provisioned session/worktree assignment. Only the planner uses WSH; workers must not create, attach, inspect, or steer tmux/WSH sessions and must not use WSH MCP. If the planner reports a server-identity validation failure, workers must not retry, select another profile, or clean up sessions; they report it as `MANAGER ATTENTION NEEDED` for human-supervised recovery. They work only in their assigned external worktree and report progress, evidence, blockers, and every required decision to the sprint planner as `MANAGER ATTENTION NEEDED`.

The planner-issued effective prompt requires a focused issue-linked PR; monitored and actioned PR/issue feedback; authority-gated out-of-scope follow-ups (otherwise `MANAGER ATTENTION NEEDED` escalation); independent QA; required checks, CODEOWNERS where applicable, no unresolved blocker, and rollback review. Merge requires explicit human authorization of the exact PR head and named target. Before issue closeout, the planner records a binding issue comment with PR, head, checks, QA, rollback, and merge-authority evidence. Workers do not remove or delete anything after merge. Only after GitHub confirms the authorized head merged into the named target may the planner, with separately recorded explicit cleanup authority, use the planner-controlled cleanup path for only that merged issue's leased worktree and local branch. Missing/changed head or target, unconfirmed merge, dirty/unpushed worktree, active/uncertain lease or session, cleanup failure, or branch/worktree mismatch is `MANAGER ATTENTION NEEDED` and leaves files and branches intact. Never remove the primary/shared/QA worktree, target/default branch, or remote branch.

QA gets a separately leased external worktree and independent session. It must not share an implementation lease, inspect terminal scrollback, or treat a WSH name/tag as authority. Normal verification uses the repository, issue/PR, safe lifecycle status, and independently repeatable tests.
