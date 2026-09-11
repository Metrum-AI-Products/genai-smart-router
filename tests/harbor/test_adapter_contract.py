# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline Harbor agent-adapter contract tests (AGENT-01..06).

These tests never contact providers, Harbor cloud, or a live router. They
import pure helpers from ``examples/harbor-algotune-pca/adapter_contract.py``.
"""

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
EXAMPLE = ROOT / "examples" / "harbor-algotune-pca"
PINS_PATH = Path(__file__).resolve().parent / "pins.py"


def _load(module_name: str, path: Path):
    spec = importlib.util.spec_from_file_location(module_name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


contract = _load("harbor_adapter_contract", EXAMPLE / "adapter_contract.py")
pins = _load("harbor_adapter_pins", PINS_PATH)


class TestVersionPins(unittest.TestCase):
    """AGENT-01: pinned baselines exist and canary is separate."""

    def test_baseline_pins_are_concrete(self) -> None:
        for key in ("harbor", "codex_cli", "claude_code"):
            value = pins.PINNED_BASELINE[key]
            self.assertTrue(value and "latest" not in value.lower(), key)
            self.assertNotIn("current-stable", value.lower(), key)

    def test_canary_is_separately_reported(self) -> None:
        self.assertIn("harbor", pins.CANARY_TRACK)
        self.assertIn("current-stable", pins.CANARY_TRACK["harbor"].lower())
        self.assertNotEqual(
            pins.PINNED_BASELINE["harbor"],
            pins.CANARY_TRACK["harbor"],
        )

    def test_primary_task_is_pinned(self) -> None:
        self.assertEqual(
            pins.PINNED_TASKS["primary_task"],
            "aider/polyglot_python_two-bucket",
        )


class TestNamespacedGroupIds(unittest.TestCase):
    """AGENT-03: slash-containing router group IDs must be preserved."""

    def test_simple_group_preserved(self) -> None:
        self.assertEqual(contract.resolve_router_model_group("big-coder"), "big-coder")

    def test_namespaced_group_preserved(self) -> None:
        # Stock Harbor Codex uses split('/')[-1]; that would yield "coding".
        self.assertEqual(
            contract.resolve_router_model_group("team/coding"),
            "team/coding",
        )

    def test_multi_segment_group_preserved(self) -> None:
        self.assertEqual(
            contract.resolve_router_model_group("org/team/big-coder"),
            "org/team/big-coder",
        )

    def test_whitespace_trimmed(self) -> None:
        self.assertEqual(contract.resolve_router_model_group("  small  "), "small")

    def test_empty_rejected(self) -> None:
        with self.assertRaises(ValueError):
            contract.resolve_router_model_group("")
        with self.assertRaises(ValueError):
            contract.resolve_router_model_group(None)

    def test_config_embeds_full_namespaced_id(self) -> None:
        text = contract.build_codex_router_config(
            model_group="team/coding",
            base_url="http://127.0.0.1:18080/v1",
        )
        self.assertIn('model = "team/coding"', text)
        self.assertNotIn('model = "coding"', text)


class TestBaseUrlNormalization(unittest.TestCase):
    """AGENT-03: trailing slash / base URL handling."""

    def test_strips_trailing_slash(self) -> None:
        self.assertEqual(
            contract.normalize_router_base_url("http://127.0.0.1:18080/v1/"),
            "http://127.0.0.1:18080/v1",
        )

    def test_adds_v1_when_missing(self) -> None:
        self.assertEqual(
            contract.normalize_router_base_url("http://127.0.0.1:18080"),
            "http://127.0.0.1:18080/v1",
        )

    def test_preserves_existing_v1(self) -> None:
        self.assertEqual(
            contract.normalize_router_base_url("https://router.example/v1"),
            "https://router.example/v1",
        )

    def test_anthropic_base_strips_slash_without_forcing_v1(self) -> None:
        self.assertEqual(
            contract.normalize_anthropic_base_url("http://127.0.0.1:18080/anthropic/"),
            "http://127.0.0.1:18080/anthropic",
        )


class TestCredentialDisposition(unittest.TestCase):
    """AGENT-03: missing credentials are blocked, never a green pass."""

    def test_missing_codex_key_blocked(self) -> None:
        result = contract.assess_codex_router_credentials(
            {"METRUM_ROUTER_BASE_URL": "http://127.0.0.1:18080/v1"}
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_BLOCKED)
        self.assertFalse(result.ready)
        self.assertFalse(contract.evidence_is_green_pass(result.disposition))

    def test_missing_codex_base_blocked(self) -> None:
        result = contract.assess_codex_router_credentials(
            {"METRUM_ROUTER_KEY": "rtr_test"}
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_BLOCKED)
        self.assertFalse(contract.evidence_is_green_pass(result.disposition))

    def test_openai_key_alone_not_router_ready(self) -> None:
        result = contract.assess_codex_router_credentials(
            {"OPENAI_API_KEY": "sk-direct-provider"}
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_BLOCKED)

    def test_codex_ready_when_key_and_base_present(self) -> None:
        result = contract.assess_codex_router_credentials(
            {
                "METRUM_ROUTER_KEY": "rtr_test",
                "METRUM_ROUTER_BASE_URL": "http://127.0.0.1:18080/v1",
            }
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_PASS)
        self.assertTrue(result.ready)

    def test_missing_claude_token_blocked(self) -> None:
        result = contract.assess_claude_router_credentials(
            {"ANTHROPIC_BASE_URL": "http://127.0.0.1:18080/anthropic"}
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_BLOCKED)
        self.assertFalse(contract.evidence_is_green_pass(result.disposition))

    def test_anthropic_api_key_alone_not_router_ready(self) -> None:
        result = contract.assess_claude_router_credentials(
            {"ANTHROPIC_API_KEY": "sk-ant-direct"}
        )
        self.assertEqual(result.disposition, contract.DISPOSITION_BLOCKED)


class TestCleanHomeSetup(unittest.TestCase):
    """AGENT-03: clean home checks flag host contamination risk."""

    def test_empty_home_is_clean(self) -> None:
        self.assertEqual(contract.clean_codex_home_markers(set()), [])

    def test_auth_and_config_flagged(self) -> None:
        markers = contract.clean_codex_home_markers({"auth.json", "config.toml", "notes.txt"})
        self.assertEqual(markers, ["auth.json", "config.toml"])


class TestResultParser(unittest.TestCase):
    """AGENT-05: no permissive default may manufacture a pass."""

    def test_missing_result_blocked(self) -> None:
        classified = contract.classify_harbor_job_result(None)
        self.assertEqual(classified["disposition"], contract.DISPOSITION_BLOCKED)
        self.assertFalse(contract.evidence_is_green_pass(classified["disposition"]))

    def test_missing_reward_fails(self) -> None:
        classified = contract.classify_harbor_job_result(
            {"stats": {"n_errored_trials": 0, "evals": {}}}
        )
        self.assertEqual(classified["disposition"], contract.DISPOSITION_FAIL)

    def test_reward_zero_fails(self) -> None:
        classified = contract.classify_harbor_job_result(
            {
                "stats": {
                    "n_errored_trials": 0,
                    "evals": {"t": {"metrics": [{"mean": 0.0}]}},
                }
            }
        )
        self.assertEqual(classified["disposition"], contract.DISPOSITION_FAIL)
        self.assertEqual(classified["reward"], 0.0)

    def test_errored_trials_fail(self) -> None:
        classified = contract.classify_harbor_job_result(
            {
                "stats": {
                    "n_errored_trials": 1,
                    "evals": {"t": {"metrics": [{"mean": 1.0}]}},
                }
            }
        )
        self.assertEqual(classified["disposition"], contract.DISPOSITION_FAIL)

    def test_reward_one_passes(self) -> None:
        classified = contract.classify_harbor_job_result(
            {
                "stats": {
                    "n_errored_trials": 0,
                    "evals": {"t": {"metrics": [{"mean": 1.0}]}},
                }
            }
        )
        self.assertEqual(classified["disposition"], contract.DISPOSITION_PASS)

    def test_malformed_reward_errors(self) -> None:
        classified = contract.classify_harbor_job_result(
            {
                "stats": {
                    "n_errored_trials": 0,
                    "evals": {"t": {"metrics": [{"mean": "not-a-number"}]}},
                }
            }
        )
        self.assertEqual(classified["disposition"], contract.DISPOSITION_ERROR)


class TestProtocolObservations(unittest.TestCase):
    """AGENT-06: fake user-agent labels are not protocol coverage."""

    def test_user_agent_alone_insufficient(self) -> None:
        self.assertFalse(
            contract.protocol_observation_complete({"user_agent_label": True})
        )

    def test_full_lifecycle_required(self) -> None:
        self.assertTrue(
            contract.protocol_observation_complete(
                {
                    "client_startup": True,
                    "initial_request": True,
                    "tool_turn": True,
                    "second_request": True,
                    "final_response": True,
                }
            )
        )

    def test_partial_lifecycle_rejected(self) -> None:
        self.assertFalse(
            contract.protocol_observation_complete(
                {
                    "client_startup": True,
                    "initial_request": True,
                    "final_response": True,
                }
            )
        )


class TestConfigEndpointTargeting(unittest.TestCase):
    """AGENT-02 offline: generated config targets the router Responses provider."""

    def test_config_points_at_router_provider(self) -> None:
        text = contract.build_codex_router_config(
            model_group="small",
            base_url="http://127.0.0.1:18080/v1/",
        )
        self.assertIn('model_provider = "metrum-router"', text)
        self.assertIn('base_url = "http://127.0.0.1:18080/v1"', text)
        self.assertIn('wire_api = "responses"', text)
        self.assertIn('env_key = "METRUM_ROUTER_KEY"', text)


if __name__ == "__main__":
    unittest.main()
