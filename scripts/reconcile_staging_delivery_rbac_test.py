#!/usr/bin/env python3
"""Regression tests for the guarded staging delivery RBAC recovery CLI."""

from __future__ import annotations

import hashlib
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("reconcile_staging_delivery_rbac", ROOT / "scripts/reconcile_staging_delivery_rbac.py")
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def role_payload() -> dict[str, object]:
    return {
        "metadata": {"name": MODULE.ROLE_NAME, "namespace": MODULE.NAMESPACE, "labels": MODULE.LABELS},
        "rules": MODULE.ROLE_RULES,
    }


def rolebinding_payload() -> dict[str, object]:
    return {
        "metadata": {"name": MODULE.ROLE_NAME, "namespace": MODULE.NAMESPACE, "labels": MODULE.LABELS},
        "subjects": [{"apiGroup": "rbac.authorization.k8s.io", "kind": "Group", "name": MODULE.ROLE_NAME}],
        "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": MODULE.ROLE_NAME},
    }


class RecoveryCLITest(unittest.TestCase):
    def test_exact_role_rejects_extra_permission(self) -> None:
        payload = role_payload()
        payload["rules"] = [*MODULE.ROLE_RULES, {"apiGroups": [""], "resources": ["secrets"], "verbs": ["get"]}]
        self.assertFalse(MODULE.expected_role(payload))

    def test_exact_rolebinding_rejects_subject_substitution(self) -> None:
        payload = rolebinding_payload()
        payload["subjects"] = [{"apiGroup": "rbac.authorization.k8s.io", "kind": "Group", "name": MODULE.FIELD_MANAGER}]
        self.assertFalse(MODULE.expected_rolebinding(payload))

    def test_guard_normalization_removes_only_server_defaults(self) -> None:
        spec = {
            "matchConstraints": {
                "matchPolicy": "Equivalent",
                "namespaceSelector": {},
                "objectSelector": {},
                "resourceRules": [],
            },
            "matchResources": {
                "matchPolicy": "Equivalent",
                "namespaceSelector": {},
                "objectSelector": {},
            },
            "failurePolicy": "Fail",
        }
        self.assertEqual(
            MODULE.normalized_guard_spec(spec),
            {
                "matchConstraints": {"resourceRules": []},
                "matchResources": {},
                "failurePolicy": "Fail",
            },
        )

    def test_guard_digest_accepts_only_defaulted_live_spec(self) -> None:
        reviewed = {
            "matchConstraints": {"resourceRules": []},
            "matchResources": {},
            "failurePolicy": "Fail",
        }
        digest = hashlib.sha256(json.dumps(reviewed, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        live = {
            "matchConstraints": {
                "matchPolicy": "Equivalent",
                "namespaceSelector": {},
                "objectSelector": {},
                "resourceRules": [],
            },
            "matchResources": {
                "matchPolicy": "Equivalent",
                "namespaceSelector": {},
                "objectSelector": {},
            },
            "failurePolicy": "Fail",
        }
        payload = {"metadata": {"name": MODULE.GUARD_NAME}, "spec": live}
        self.assertTrue(MODULE.expected_guard(payload, digest=digest))
        live["matchConstraints"]["namespaceSelector"] = {"matchLabels": {"unexpected": "drift"}}
        self.assertFalse(MODULE.expected_guard(payload, digest=digest))
        live["matchConstraints"]["namespaceSelector"] = {}
        live["matchResources"]["objectSelector"] = {"matchLabels": {"unexpected": "drift"}}
        self.assertFalse(MODULE.expected_guard(payload, digest=digest))

    def test_apply_uses_reviewed_namespace_manifests_only(self) -> None:
        commands: list[list[str]] = []
        original_run = MODULE.run
        try:
            MODULE.run = lambda command: commands.append(command) or ""  # type: ignore[method-assign]
            MODULE.apply(Path("/secure/kubeconfig"), dry_run=True)
        finally:
            MODULE.run = original_run
        command = commands[0]
        self.assertIn("--dry-run=server", command)
        self.assertIn("--force-conflicts", command)
        self.assertEqual(
            [value for value in command if value in {str(manifest) for manifest in MODULE.MANIFESTS}],
            [str(manifest) for manifest in MODULE.MANIFESTS],
        )
        self.assertNotIn("eks-staging-delivery-rbac.yaml", " ".join(command))

    def test_main_reports_exact_readback_only(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            kubeconfig = Path(directory) / "config"
            kubeconfig.touch(mode=0o600)
            original_authorized = MODULE.assert_authorized
            original_guard = MODULE.assert_admission_guard
            original_apply = MODULE.apply
            original_readback = MODULE.readback
            try:
                calls: list[bool] = []
                MODULE.assert_authorized = lambda _: None  # type: ignore[method-assign]
                MODULE.assert_admission_guard = lambda _: None  # type: ignore[method-assign]
                MODULE.apply = lambda _, *, dry_run: calls.append(dry_run)  # type: ignore[method-assign]
                MODULE.readback = lambda _: {"role": True, "rolebinding": True}  # type: ignore[method-assign]
                output = io.StringIO()
                with contextlib.redirect_stdout(output):
                    result = MODULE.main(["--kubeconfig", str(kubeconfig), "--confirm", MODULE.CONFIRMATION])
            finally:
                MODULE.assert_authorized = original_authorized
                MODULE.apply = original_apply
                MODULE.assert_admission_guard = original_guard
                MODULE.readback = original_readback
        self.assertEqual(result, 0)
        self.assertEqual(calls, [True, False])
        self.assertEqual(json.loads(output.getvalue())["outcome"], "reconciled")


if __name__ == "__main__":
    unittest.main()
