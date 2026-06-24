#!/usr/bin/env python3
"""Tests for env example secret guardrails."""

from __future__ import annotations

import unittest

import check_env_example_secrets as checker


class SecretKeyErrorsTest(unittest.TestCase):
    def test_allows_empty_sensitive_values(self) -> None:
        errors = list(
            checker.secret_key_errors(
                checker.ROOT / "env.example.json",
                {"UNKNOWN_PROVIDER_API_KEY": "", "PASSWORD": ""},
            )
        )

        self.assertEqual([], errors)

    def test_rejects_concrete_password_value(self) -> None:
        errors = list(
            checker.secret_key_errors(
                checker.ROOT / "env.example.json",
                {"PASSWORD": "super-secret-value"},
            )
        )

        self.assertIn("env.example.json:PASSWORD must be empty or a placeholder", errors[0])

    def test_rejects_unknown_provider_concrete_key_value(self) -> None:
        errors = list(
            checker.secret_key_errors(
                checker.ROOT / "env.example.json",
                {"UNKNOWN_PROVIDER_API_KEY": "super-secret-value"},
            )
        )

        self.assertIn(
            "env.example.json:UNKNOWN_PROVIDER_API_KEY must be empty or a placeholder",
            errors[0],
        )

    def test_allows_placeholder_values(self) -> None:
        errors = list(
            checker.secret_key_errors(
                checker.ROOT / "env.example.json",
                {
                    "OPENAI_API_KEY": "YOUR_OPENAI_API_KEY",
                    "ROUTER_TOKEN": "<router-token>",
                },
            )
        )

        self.assertEqual([], errors)


if __name__ == "__main__":
    unittest.main()
