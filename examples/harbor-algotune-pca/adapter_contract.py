# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Pure Harbor adapter contract helpers (no Harbor runtime import).

AGENT-01..06 offline surface for the custom Codex adapter and Claude env setup.
Live Harbor/provider runs are deliberately out of scope for these helpers.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping
from urllib.parse import urlsplit, urlunsplit


# Disposition vocabulary (EVAL-02 / AGENT-03). A skip or blocked cell is never a pass.
DISPOSITION_PASS = "pass"
DISPOSITION_FAIL = "fail"
DISPOSITION_SKIPPED = "skipped"
DISPOSITION_BLOCKED = "blocked"
DISPOSITION_ERROR = "error"

NON_PASS_DISPOSITIONS = frozenset(
    {
        DISPOSITION_FAIL,
        DISPOSITION_SKIPPED,
        DISPOSITION_BLOCKED,
        DISPOSITION_ERROR,
    }
)


def resolve_router_model_group(model_name: str | None) -> str:
    """Return the router model-group ID Codex should request.

    Harbor's stock Codex adapter strips ``provider/`` via ``split('/')[-1]``.
    Router group IDs are opaque and may themselves contain ``/`` (for example
    ``team/coding`` or ``org/big-coder``). Stripping would route the wrong
    group. Preserve the full identifier after trimming whitespace.

    Raises:
        ValueError: when the model/group is missing or empty after trim.
    """
    if model_name is None:
        raise ValueError("Model name is required")
    group = model_name.strip()
    if not group:
        raise ValueError("Model name is required")
    return group


def normalize_router_base_url(raw: str | None, *, require_v1_suffix: bool = True) -> str:
    """Normalize a router OpenAI-compatible base URL for Codex config.

    - Trims whitespace
    - Drops a trailing slash (Codex joins paths; ``.../v1/`` would double-slash)
    - Optionally ensures a final ``/v1`` segment when the operator passed the
      router root (``http://host:port``) rather than the OpenAI mount
    """
    if raw is None:
        raise ValueError("Router base URL is required")
    value = raw.strip()
    if not value:
        raise ValueError("Router base URL is required")
    while value.endswith("/"):
        value = value[:-1]
    if require_v1_suffix:
        parts = urlsplit(value)
        path = parts.path.rstrip("/")
        if not path.endswith("/v1"):
            path = f"{path}/v1" if path else "/v1"
            value = urlunsplit((parts.scheme, parts.netloc, path, parts.query, parts.fragment))
    return value


def normalize_anthropic_base_url(raw: str | None) -> str:
    """Normalize Claude Code ``ANTHROPIC_BASE_URL`` (no forced ``/v1``).

    Prefer ``$ROUTER_BASE_URL/anthropic`` for new validation; legacy
    ``$ROUTER_BASE_URL`` remains accepted. Always strip trailing slashes.
    """
    if raw is None:
        raise ValueError("Anthropic base URL is required")
    value = raw.strip()
    if not value:
        raise ValueError("Anthropic base URL is required")
    while value.endswith("/"):
        value = value[:-1]
    return value


@dataclass(frozen=True)
class CredentialAssessment:
    """Credential readiness for a router-mode agent trial."""

    disposition: str
    reason: str
    client: str

    @property
    def ready(self) -> bool:
        return self.disposition == DISPOSITION_PASS


def assess_codex_router_credentials(
    env: Mapping[str, str | None] | None,
) -> CredentialAssessment:
    """Classify Codex router-mode credential readiness.

    Missing ``METRUM_ROUTER_KEY`` (and empty auth.json fallback input) is
    **blocked**, not a green pass. Direct OpenAI keys are ignored for
    router-mode trials so accidental provider fallback is not treated as ready.
    """
    env = env or {}
    key = (env.get("METRUM_ROUTER_KEY") or "").strip()
    base = (env.get("METRUM_ROUTER_BASE_URL") or env.get("OPENAI_BASE_URL") or "").strip()
    if not key:
        return CredentialAssessment(
            disposition=DISPOSITION_BLOCKED,
            reason="missing METRUM_ROUTER_KEY for router-mode Codex trial",
            client="codex",
        )
    if not base:
        return CredentialAssessment(
            disposition=DISPOSITION_BLOCKED,
            reason="missing METRUM_ROUTER_BASE_URL/OPENAI_BASE_URL for router-mode Codex trial",
            client="codex",
        )
    return CredentialAssessment(
        disposition=DISPOSITION_PASS,
        reason="router credentials present",
        client="codex",
    )


def assess_claude_router_credentials(
    env: Mapping[str, str | None] | None,
) -> CredentialAssessment:
    """Classify Claude Code router-mode credential readiness.

    Requires ``ANTHROPIC_AUTH_TOKEN`` and ``ANTHROPIC_BASE_URL``. Presence of
    ``ANTHROPIC_API_KEY`` alone is not router-mode readiness (risk of direct
    Anthropic fallback).
    """
    env = env or {}
    token = (env.get("ANTHROPIC_AUTH_TOKEN") or "").strip()
    base = (env.get("ANTHROPIC_BASE_URL") or "").strip()
    if not token:
        return CredentialAssessment(
            disposition=DISPOSITION_BLOCKED,
            reason="missing ANTHROPIC_AUTH_TOKEN for router-mode Claude Code trial",
            client="claude-code",
        )
    if not base:
        return CredentialAssessment(
            disposition=DISPOSITION_BLOCKED,
            reason="missing ANTHROPIC_BASE_URL for router-mode Claude Code trial",
            client="claude-code",
        )
    return CredentialAssessment(
        disposition=DISPOSITION_PASS,
        reason="router credentials present",
        client="claude-code",
    )


def evidence_is_green_pass(disposition: str) -> bool:
    """True only for an explicit pass. blocked/skipped/fail/error are never green."""
    return disposition == DISPOSITION_PASS


def build_codex_router_config(
    *,
    model_group: str,
    base_url: str,
    provider_name: str = "metrum-ai-router",
    env_key: str = "METRUM_ROUTER_KEY",
    wire_api: str = "responses",
) -> str:
    """Render the Codex ``config.toml`` fragment for router Responses traffic."""
    group = resolve_router_model_group(model_group)
    url = normalize_router_base_url(base_url)
    return (
        f'model = "{group}"\n'
        f'model_provider = "{provider_name}"\n'
        "\n"
        f'[model_providers."{provider_name}"]\n'
        'name = "Metrum AI Router"\n'
        f'base_url = "{url}"\n'
        f'env_key = "{env_key}"\n'
        f'wire_api = "{wire_api}"\n'
    )


def clean_codex_home_markers(home_entries: set[str] | frozenset[str]) -> list[str]:
    """Return unexpected host Codex home leftovers that would pollute a clean trial.

    Router-mode Harbor trials use an isolated remote ``CODEX_HOME``. Host
    leftovers listed here must not be treated as success evidence for a clean
    setup; they are diagnostics for contamination risk.
    """
    unexpected = []
    for name in sorted(home_entries):
        if name in {"auth.json", "config.toml", "sessions", "history.jsonl"}:
            unexpected.append(name)
    return unexpected


def classify_harbor_job_result(result: Mapping[str, Any] | None) -> dict[str, Any]:
    """Classify a Harbor job JSON result without inventing a pass.

    Missing/empty/malformed reward, nonzero exceptions, or absent stats are
    failures or blocked cells — never a permissive default pass.
    """
    if result is None:
        return {
            "disposition": DISPOSITION_BLOCKED,
            "reason": "missing Harbor job result",
            "reward": None,
            "errors": None,
        }
    if not isinstance(result, Mapping):
        return {
            "disposition": DISPOSITION_ERROR,
            "reason": "malformed Harbor job result (not an object)",
            "reward": None,
            "errors": None,
        }
    stats = result.get("stats")
    if stats is None:
        return {
            "disposition": DISPOSITION_BLOCKED,
            "reason": "Harbor job result missing stats",
            "reward": None,
            "errors": None,
        }
    if not isinstance(stats, Mapping):
        return {
            "disposition": DISPOSITION_ERROR,
            "reason": "malformed Harbor stats",
            "reward": None,
            "errors": None,
        }

    raw_errors = stats.get("n_errored_trials")
    try:
        errors = int(raw_errors) if raw_errors is not None else 0
    except (TypeError, ValueError):
        return {
            "disposition": DISPOSITION_ERROR,
            "reason": "malformed n_errored_trials",
            "reward": None,
            "errors": None,
        }

    rewards: list[float] = []
    evals = stats.get("evals") or {}
    if not isinstance(evals, Mapping):
        return {
            "disposition": DISPOSITION_ERROR,
            "reason": "malformed Harbor evals",
            "reward": None,
            "errors": errors,
        }
    for eval_result in evals.values():
        if not isinstance(eval_result, Mapping):
            continue
        for metric in eval_result.get("metrics") or []:
            if not isinstance(metric, Mapping):
                continue
            if "mean" in metric and metric["mean"] is not None:
                try:
                    rewards.append(float(metric["mean"]))
                except (TypeError, ValueError):
                    return {
                        "disposition": DISPOSITION_ERROR,
                        "reason": "malformed reward metric",
                        "reward": None,
                        "errors": errors,
                    }

    if not rewards:
        return {
            "disposition": DISPOSITION_FAIL,
            "reason": "missing or empty reward metrics",
            "reward": None,
            "errors": errors,
        }

    reward = min(rewards)
    if errors != 0:
        return {
            "disposition": DISPOSITION_FAIL,
            "reason": f"Harbor reported {errors} errored trial(s)",
            "reward": reward,
            "errors": errors,
        }
    if reward != 1.0:
        return {
            "disposition": DISPOSITION_FAIL,
            "reason": f"reward {reward:g} is not 1",
            "reward": reward,
            "errors": errors,
        }
    return {
        "disposition": DISPOSITION_PASS,
        "reason": "reward 1 with zero errored trials",
        "reward": reward,
        "errors": errors,
    }


def protocol_observation_complete(observations: Mapping[str, bool]) -> bool:
    """AGENT-06 offline gate: require real client lifecycle markers.

    A fake user-agent label alone is insufficient. Required markers for a
    supported tool-capable trial: startup, initial request, tool turn, second
    request, final response.
    """
    required = (
        "client_startup",
        "initial_request",
        "tool_turn",
        "second_request",
        "final_response",
    )
    return all(bool(observations.get(key)) for key in required)
