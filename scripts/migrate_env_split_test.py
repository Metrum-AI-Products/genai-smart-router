#!/usr/bin/env python3
"""Unit tests for migrate_env_split.py (fake temp files, no real secrets)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "migrate_env_split.py"


class MigrateEnvSplitTest(unittest.TestCase):
    def test_split_maps_nested_stripe_and_ops(self) -> None:
        from migrate_env_split import split_env

        raw = {
            "OPENAI_API_KEY": "sk-fake-openai",
            "ROUTER_HTTP_REFERER": "https://example.test",
            "STRIPE_KEYS": {
                "SANDBOX_KEYS": {
                    "SECRET_KEY": "sk_test_fake_secret",
                    "PUBLISHABLE_KEY": "pk_test_fake_pub",
                },
                "RESTRICTED_KEY": "rk_test_ignored",
            },
            "STRIPE_WEBHOOK_SECRET": "whsec_fake",
            "BACKUP_USER": "backup-user",
            "RESTIC_PASSWORD": "restic-fake",
            "RESTIC_REPO_HOST": "backups.example.test",
            "RESTIC_REPO_PATH": "cto",
            "WORK_ITEMS_DASHBOARD_PORT": 8765,
            "COMMERCE_FLEET_ENABLED": "1",
        }
        instance, commerce, ops = split_env(raw)
        self.assertEqual(instance["OPENAI_API_KEY"], "sk-fake-openai")
        self.assertNotIn("STRIPE_SECRET_KEY", instance)
        self.assertNotIn("BACKUP_USER", instance)
        self.assertEqual(commerce["STRIPE_SECRET_KEY"], "sk_test_fake_secret")
        self.assertEqual(commerce["STRIPE_PUBLISHABLE_KEY"], "pk_test_fake_pub")
        self.assertEqual(commerce["STRIPE_WEBHOOK_SECRET"], "whsec_fake")
        self.assertEqual(commerce["COMMERCE_FLEET_ENABLED"], "1")
        self.assertEqual(ops["WORK_ITEMS_DASHBOARD_PORT"], "8765")
        self.assertEqual(ops["BACKUP_USER"], "backup-user")
        self.assertEqual(ops["RESTIC_PASSWORD"], "restic-fake")

    def test_flat_stripe_overrides_empty_nested(self) -> None:
        from migrate_env_split import split_env

        raw = {
            "STRIPE_KEYS": {"SANDBOX_KEYS": {"SECRET_KEY": ""}},
            "STRIPE_SECRET_KEY": "sk_test_flat",
            "OPENAI_API_KEY": "x",
        }
        _, commerce, _ = split_env(raw)
        self.assertEqual(commerce["STRIPE_SECRET_KEY"], "sk_test_flat")

    def test_dry_run_and_refuse_overwrite(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            mixed = tmp_path / "mixed.json"
            mixed.write_text(
                json.dumps(
                    {
                        "OPENAI_API_KEY": "sk-fake",
                        "STRIPE_SECRET_KEY": "sk_test_x",
                        "BACKUP_USER": "u",
                    }
                ),
                encoding="utf-8",
            )
            instance_out = tmp_path / "env.json"
            commerce_out = tmp_path / "commerce.env.json"
            ops_out = tmp_path / "ops.env.json"

            dry = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--input",
                    str(mixed),
                    "--instance-out",
                    str(instance_out),
                    "--commerce-out",
                    str(commerce_out),
                    "--ops-out",
                    str(ops_out),
                    "--dry-run",
                ],
                cwd=ROOT / "scripts",
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(dry.returncode, 0, dry.stderr)
            self.assertIn("dry-run", dry.stdout)
            self.assertFalse(instance_out.exists())

            first = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--input",
                    str(mixed),
                    "--instance-out",
                    str(instance_out),
                    "--commerce-out",
                    str(commerce_out),
                    "--ops-out",
                    str(ops_out),
                    "--force",
                ],
                cwd=ROOT / "scripts",
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertTrue(commerce_out.exists())

            refuse = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--input",
                    str(mixed),
                    "--instance-out",
                    str(instance_out),
                    "--commerce-out",
                    str(commerce_out),
                    "--ops-out",
                    str(ops_out),
                ],
                cwd=ROOT / "scripts",
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(refuse.returncode, 2, refuse.stdout + refuse.stderr)
            self.assertIn("refusing to overwrite", refuse.stderr)

            # Secrets must not appear in dry-run stdout beyond key counts.
            self.assertNotIn("sk_test_x", dry.stdout)
            self.assertNotIn("sk-fake", dry.stdout)


if __name__ == "__main__":
    # Allow importing migrate_env_split from the scripts directory.
    sys.path.insert(0, str(ROOT / "scripts"))
    raise SystemExit(unittest.main())
