#!/usr/bin/env python3
"""Mint a bounded MFA session and configure a read-only EKS role profile."""

from __future__ import annotations

import argparse
import base64
import configparser
import hashlib
import hmac
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path


SAFE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")
ARN = re.compile(r"^arn:aws:iam::[0-9]{12}:(?:user|role)/[A-Za-z0-9+=,.@_/-]+$")
SERIAL = re.compile(r"^arn:aws:iam::[0-9]{12}:mfa/[A-Za-z0-9+=,.@_/-]+$")


def command(args: list[str], *, env: dict[str, str] | None = None) -> str:
    result = subprocess.run(args, env=env, text=True, capture_output=True, check=False)
    if result.returncode:
        raise RuntimeError(f"AWS command failed ({' '.join(args[:3])})")
    return result.stdout


def mfa_code_from_keychain(service: str, account: str) -> str:
    seed = command(["security", "find-generic-password", "-a", account, "-s", service, "-w"]).strip().upper()
    seed += "=" * (-len(seed) % 8)
    key = base64.b32decode(seed, casefold=True)
    digest = hmac.new(key, int(time.time() // 30).to_bytes(8, "big"), hashlib.sha1).digest()
    offset = digest[-1] & 15
    value = ((digest[offset] & 127) << 24) | (digest[offset + 1] << 16) | (digest[offset + 2] << 8) | digest[offset + 3]
    return f"{value % 1_000_000:06d}"


def write_profiles(profile: str, role_profile: str, role_arn: str, region: str, credentials: dict[str, str]) -> None:
    aws_dir = Path.home() / ".aws"
    aws_dir.mkdir(mode=0o700, exist_ok=True)
    credentials_path = aws_dir / "credentials"
    config_path = aws_dir / "config"
    creds = configparser.RawConfigParser()
    creds.read(credentials_path)
    creds[profile] = credentials
    with credentials_path.open("w", encoding="utf-8") as file:
        creds.write(file)
    credentials_path.chmod(0o600)
    config = configparser.RawConfigParser()
    config.read(config_path)
    config[f"profile {profile}"] = {"region": region}
    config[f"profile {role_profile}"] = {"role_arn": role_arn, "source_profile": profile, "region": region}
    with config_path.open("w", encoding="utf-8") as file:
        config.write(file)
    config_path.chmod(0o600)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--admin-profile", default="default")
    parser.add_argument("--source-user", required=True)
    parser.add_argument("--mfa-serial", required=True)
    parser.add_argument("--macos-keychain-service", required=True)
    parser.add_argument("--macos-keychain-account", required=True)
    parser.add_argument("--session-profile", default="smartrouter")
    parser.add_argument("--role-profile", default="genai-smart-router-eks-discovery")
    parser.add_argument("--role-arn", required=True)
    parser.add_argument("--region", required=True)
    parser.add_argument("--duration-seconds", type=int, default=3600)
    parser.add_argument("--propagation-wait-seconds", type=int, default=8)
    parser.add_argument("--cleanup-record", required=True, type=Path)
    args = parser.parse_args()
    for value in (args.admin_profile, args.source_user, args.macos_keychain_account, args.session_profile, args.role_profile):
        if not SAFE_NAME.fullmatch(value):
            parser.error("profile and identity names must be simple identifiers")
    if not SERIAL.fullmatch(args.mfa_serial) or not ARN.fullmatch(args.role_arn) or not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)+", args.region):
        parser.error("invalid MFA serial, role ARN, or region")
    if not 900 <= args.duration_seconds <= 43200 or not 0 <= args.propagation_wait_seconds <= 30:
        parser.error("invalid session duration or propagation wait")

    key_id = ""
    deleted = False
    def delete_source_key() -> bool:
        for _ in range(3):
            try:
                command(["aws", "iam", "delete-access-key", "--profile", args.admin_profile, "--user-name", args.source_user, "--access-key-id", key_id])
                return True
            except RuntimeError:
                time.sleep(1)
        return False
    try:
        access = json.loads(command(["aws", "iam", "create-access-key", "--profile", args.admin_profile, "--user-name", args.source_user, "--output", "json"]))["AccessKey"]
        key_id = access["AccessKeyId"]
        if args.propagation_wait_seconds:
            time.sleep(args.propagation_wait_seconds)
        env = os.environ.copy()
        for name in ("AWS_PROFILE", "AWS_SESSION_TOKEN"):
            env.pop(name, None)
        env.update({"AWS_CONFIG_FILE": os.devnull, "AWS_SHARED_CREDENTIALS_FILE": os.devnull, "AWS_ACCESS_KEY_ID": access["AccessKeyId"], "AWS_SECRET_ACCESS_KEY": access["SecretAccessKey"], "AWS_DEFAULT_REGION": args.region})
        session = json.loads(command(["aws", "sts", "get-session-token", "--serial-number", args.mfa_serial, "--token-code", mfa_code_from_keychain(args.macos_keychain_service, args.macos_keychain_account), "--duration-seconds", str(args.duration_seconds), "--output", "json"], env=env))["Credentials"]
        write_profiles(args.session_profile, args.role_profile, args.role_arn, args.region, {"aws_access_key_id": session["AccessKeyId"], "aws_secret_access_key": session["SecretAccessKey"], "aws_session_token": session["SessionToken"]})
        deleted = delete_source_key()
        if not deleted:
            raise RuntimeError("temporary source key deletion failed after retries")
        key_id = ""
        role_identity = json.loads(command(["aws", "sts", "get-caller-identity", "--profile", args.role_profile, "--output", "json"]))
        print(json.dumps({"session_profile": args.session_profile, "role_profile": args.role_profile, "role_identity": role_identity["Arn"], "session_expiration": session["Expiration"], "source_access_key_deleted": True}))
        return 0
    finally:
        if key_id:
            if not delete_source_key() and not deleted:
                args.cleanup_record.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                args.cleanup_record.write_text(json.dumps({"source_user": args.source_user, "access_key_id": key_id, "required_action": "delete temporary source access key"}) + "\n", encoding="utf-8")
                args.cleanup_record.chmod(0o600)
                raise RuntimeError(f"temporary source key deletion failed; cleanup record: {args.cleanup_record}")


if __name__ == "__main__":
    raise SystemExit(main())
