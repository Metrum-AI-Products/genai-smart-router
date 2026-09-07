#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline regression tests for generic Fleet IAM-user enrollment."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "enroll_fleet_operator.py"
spec = importlib.util.spec_from_file_location("fleet_operator_enrollment", MODULE_PATH)
if spec is None or spec.loader is None:
    raise RuntimeError("unable to load Fleet enrollment tool")
MODULE = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = MODULE
spec.loader.exec_module(MODULE)

ACCOUNT = "123456789012"
PRINCIPAL = f"arn:aws:iam::{ACCOUNT}:user/smart-router-lifecycle/test-operator"
PLATFORM_IAC_CALLER = (
    f"arn:aws:sts::{ACCOUNT}:assumed-role/{MODULE.PLATFORM_IAC_ROLE_NAME}/enrollment-session"
)


class FakeAWS:
    def __init__(
        self,
        *,
        root: bool = False,
        tag: bool = True,
        member: bool = False,
        caller_arn: str | None = None,
    ) -> None:
        self.root = root
        self.tag = tag
        self.member = member
        self.caller_arn = caller_arn
        self.calls: list[list[str]] = []

    def __call__(self, profile: str, args: list[str]) -> dict[str, Any]:
        self.calls.append(args)
        if args[:2] == ["sts", "get-caller-identity"]:
            if self.root:
                arn = f"arn:aws:iam::{ACCOUNT}:root"
            elif self.caller_arn is not None:
                arn = self.caller_arn
            else:
                arn = PLATFORM_IAC_CALLER
            return {"Account": ACCOUNT, "Arn": arn}
        if args[:2] == ["iam", "get-user"]:
            return {"User": {"Path": "/smart-router-lifecycle/"}}
        if args[:2] == ["iam", "list-user-tags"]:
            return {"Tags": [{"Key": "GenAISmartRouterLifecycle", "Value": "true"}] if self.tag else []}
        if args[:2] == ["iam", "get-group"]:
            return {"Group": {"GroupName": MODULE.LIFECYCLE_GROUP}}
        if args[:2] == ["iam", "list-groups-for-user"]:
            return {"Groups": [{"GroupName": MODULE.LIFECYCLE_GROUP}] if self.member else []}
        if args[:2] == ["iam", "add-user-to-group"]:
            self.member = True
            return {}
        raise AssertionError(f"unexpected AWS command: {args}")


def expect_error(
    fake: FakeAWS,
    *,
    principal: str = PRINCIPAL,
    apply: bool = False,
    confirmation: str = "",
) -> str:
    MODULE.run_aws = fake
    try:
        MODULE.enroll("platform-iac", principal, apply=apply, confirmation=confirmation)
    except MODULE.EnrollmentError as exc:
        return str(exc)
    raise AssertionError("expected enrollment to fail")


def main() -> int:
    fake = FakeAWS()
    MODULE.run_aws = fake
    preflight = MODULE.enroll("platform-iac", PRINCIPAL, apply=False, confirmation="")
    if preflight["outcome"] != "ready" or preflight["already_member"]:
        raise AssertionError(f"unexpected preflight: {preflight}")
    if any(command[:2] == ["iam", "add-user-to-group"] for command in fake.calls):
        raise AssertionError("preflight mutated IAM membership")
    if "test-operator" in json.dumps(preflight):
        raise AssertionError("safe preflight output exposed a personal IAM user name")

    fake = FakeAWS()
    MODULE.run_aws = fake
    applied = MODULE.enroll("platform-iac", PRINCIPAL, apply=True, confirmation=MODULE.APPLY_CONFIRMATION)
    if applied["outcome"] != "enrolled" or not fake.member:
        raise AssertionError(f"apply did not enroll generic principal: {applied}")
    if sum(command[:2] == ["iam", "add-user-to-group"] for command in fake.calls) != 1:
        raise AssertionError("apply did not issue exactly one group-membership mutation")

    if "root credentials" not in expect_error(FakeAWS(root=True)):
        raise AssertionError("root caller was not rejected")
    wrong_role = FakeAWS(
        caller_arn=f"arn:aws:sts::{ACCOUNT}:assumed-role/genai-smart-router-eks-staging-lifecycle-operator/session"
    )
    if "platform-iac" not in expect_error(wrong_role):
        raise AssertionError("non-platform-iac assumed role was not rejected")
    iam_user_caller = FakeAWS(caller_arn=f"arn:aws:iam::{ACCOUNT}:user/smart-router-lifecycle/test-operator")
    if "platform-iac" not in expect_error(iam_user_caller):
        raise AssertionError("direct IAM-user caller was not rejected")
    if "required lifecycle enrollment tag" not in expect_error(FakeAWS(tag=False)):
        raise AssertionError("untagged principal was not rejected")
    if "dedicated lifecycle IAM-user path" not in expect_error(
        FakeAWS(), principal=f"arn:aws:iam::{ACCOUNT}:user/unrelated"
    ):
        raise AssertionError("wrong principal path was not rejected")
    if MODULE.APPLY_CONFIRMATION not in expect_error(FakeAWS(), apply=True, confirmation="wrong"):
        raise AssertionError("apply without exact confirmation was not rejected")

    source = MODULE_PATH.read_text(encoding="utf-8")
    for leak in ("aditya1", "aditya", "chetan", f"{ACCOUNT}:user"):
        if leak in source:
            raise AssertionError("human test identity leaked into enrollment source")
    print("generic Fleet operator enrollment tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
