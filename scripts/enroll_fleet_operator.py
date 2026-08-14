#!/usr/bin/env python3
"""Enroll an eligible IAM user in the fixed Fleet lifecycle-operator group.

This is a platform-IaC administration helper. It accepts a runtime principal ARN,
never creates users or credentials, and emits only safe generic evidence. The
federated/Identity Center role path remains the preferred enrollment mechanism.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass
from typing import Any


LIFECYCLE_USER_PATH = "/smart-router-lifecycle/"
LIFECYCLE_TAG_KEY = "GenAISmartRouterLifecycle"
LIFECYCLE_TAG_VALUE = "true"
LIFECYCLE_GROUP = "genai-smart-router-eks-staging-lifecycle-operators"
APPLY_CONFIRMATION = "ENROLL_FLEET_OPERATOR"
USER_ARN = re.compile(r"^arn:(aws|aws-us-gov|aws-cn):iam::(?P<account>[0-9]{12}):user(?P<path>/[A-Za-z0-9+=,.@_/-]+)$")


class EnrollmentError(RuntimeError):
    pass


@dataclass(frozen=True)
class Principal:
    account_id: str
    user_name: str


def run_aws(profile: str, args: list[str]) -> dict[str, Any]:
    command = ["aws", *args, "--profile", profile, "--output", "json"]
    try:
        completed = subprocess.run(command, check=True, capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise EnrollmentError("aws CLI is unavailable") from exc
    except subprocess.CalledProcessError as exc:
        raise EnrollmentError("AWS authorization or request failed") from exc
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise EnrollmentError("AWS returned invalid JSON") from exc
    if not isinstance(payload, dict):
        raise EnrollmentError("AWS returned an invalid response")
    return payload


def parse_principal_arn(value: str) -> Principal:
    match = USER_ARN.fullmatch(value)
    if match is None:
        raise EnrollmentError("principal ARN must name one IAM user")
    path = match.group("path")
    if not path.startswith(LIFECYCLE_USER_PATH) or path == LIFECYCLE_USER_PATH:
        raise EnrollmentError("principal must use the dedicated lifecycle IAM-user path")
    user_name = path.rsplit("/", 1)[-1]
    if not user_name:
        raise EnrollmentError("principal ARN must name one IAM user")
    return Principal(account_id=match.group("account"), user_name=user_name)


def caller_account(profile: str) -> str:
    identity = run_aws(profile, ["sts", "get-caller-identity"])
    arn = identity.get("Arn")
    account = identity.get("Account")
    if not isinstance(arn, str) or arn.endswith(":root"):
        raise EnrollmentError("root credentials are forbidden for Fleet operator enrollment")
    if not isinstance(account, str) or not re.fullmatch(r"[0-9]{12}", account):
        raise EnrollmentError("AWS caller identity is invalid")
    return account


def user_tags(profile: str, user_name: str) -> dict[str, str]:
    payload = run_aws(profile, ["iam", "list-user-tags", "--user-name", user_name])
    tags = payload.get("Tags")
    if not isinstance(tags, list):
        raise EnrollmentError("IAM user tags are invalid")
    result: dict[str, str] = {}
    for tag in tags:
        if not isinstance(tag, dict) or not isinstance(tag.get("Key"), str) or not isinstance(tag.get("Value"), str):
            raise EnrollmentError("IAM user tags are invalid")
        result[tag["Key"]] = tag["Value"]
    return result


def is_group_member(profile: str, user_name: str) -> bool:
    payload = run_aws(profile, ["iam", "list-groups-for-user", "--user-name", user_name])
    groups = payload.get("Groups")
    if not isinstance(groups, list):
        raise EnrollmentError("IAM user groups are invalid")
    return any(isinstance(group, dict) and group.get("GroupName") == LIFECYCLE_GROUP for group in groups)


def assert_group_exists(profile: str) -> None:
    payload = run_aws(profile, ["iam", "get-group", "--group-name", LIFECYCLE_GROUP])
    group = payload.get("Group")
    if not isinstance(group, dict) or group.get("GroupName") != LIFECYCLE_GROUP:
        raise EnrollmentError("reviewed lifecycle-operator group is unavailable")


def safe_output(*, action: str, outcome: str, account_id: str, already_member: bool) -> dict[str, Any]:
    return {
        "action": action,
        "outcome": outcome,
        "account_id": account_id,
        "principal_class": "dedicated_lifecycle_iam_user",
        "principal_path": LIFECYCLE_USER_PATH,
        "required_principal_tag": f"{LIFECYCLE_TAG_KEY}={LIFECYCLE_TAG_VALUE}",
        "group": LIFECYCLE_GROUP,
        "already_member": already_member,
    }


def enroll(profile: str, principal_arn: str, *, apply: bool, confirmation: str) -> dict[str, Any]:
    principal = parse_principal_arn(principal_arn)
    account_id = caller_account(profile)
    if account_id != principal.account_id:
        raise EnrollmentError("principal account must match the platform administrator account")
    user = run_aws(profile, ["iam", "get-user", "--user-name", principal.user_name]).get("User")
    if not isinstance(user, dict) or user.get("Path") != LIFECYCLE_USER_PATH:
        raise EnrollmentError("principal must use the dedicated lifecycle IAM-user path")
    if user_tags(profile, principal.user_name).get(LIFECYCLE_TAG_KEY) != LIFECYCLE_TAG_VALUE:
        raise EnrollmentError("principal lacks the required lifecycle enrollment tag")
    assert_group_exists(profile)
    already_member = is_group_member(profile, principal.user_name)
    if not apply:
        return safe_output(action="preflight-fleet-operator-enrollment", outcome="already-enrolled" if already_member else "ready", account_id=account_id, already_member=already_member)
    if confirmation != APPLY_CONFIRMATION:
        raise EnrollmentError(f"confirmation must equal {APPLY_CONFIRMATION}")
    if not already_member:
        run_aws(profile, ["iam", "add-user-to-group", "--group-name", LIFECYCLE_GROUP, "--user-name", principal.user_name])
    if not is_group_member(profile, principal.user_name):
        raise EnrollmentError("lifecycle-operator membership did not converge")
    return safe_output(action="enroll-fleet-operator", outcome="already-enrolled" if already_member else "enrolled", account_id=account_id, already_member=already_member)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    parser.add_argument("--profile", required=True, help="approved non-root platform-IaC AWS CLI profile")
    parser.add_argument("--principal-arn", required=True, help="runtime IAM-user ARN under the reviewed lifecycle path")
    parser.add_argument("--apply", action="store_true", help="perform the explicit IAM group membership mutation")
    parser.add_argument("--confirm", default="", help=f"required with --apply: {APPLY_CONFIRMATION}")
    args = parser.parse_args(argv)
    print(json.dumps(enroll(args.profile, args.principal_arn, apply=args.apply, confirmation=args.confirm), sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except EnrollmentError as exc:
        print(f"Fleet operator enrollment failed: {exc}", file=sys.stderr)
        raise SystemExit(2)
