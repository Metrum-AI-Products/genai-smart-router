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
import tempfile
import time
import uuid
from pathlib import Path


SAFE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")
ARN = re.compile(r"^arn:aws:iam::[0-9]{12}:(?:user|role)/[A-Za-z0-9+=,.@_/-]+$")
SERIAL = re.compile(r"^arn:aws:iam::[0-9]{12}:mfa/[A-Za-z0-9+=,.@_/-]+$")
RESERVATION_ID = re.compile(r"^[a-f0-9]{32}$")
ACCESS_KEY_ID = re.compile(r"^[A-Z0-9]{16,128}$")
RECOVERY_RECORD_VERSION = 1
RESERVED_BEFORE_CREATE = "reserved_before_create"
ACCESS_KEY_CREATED = "access_key_created"


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


def configured_aws_profile_paths() -> tuple[Path, Path]:
    """Return the exact AWS CLI profile files selected for this process."""
    aws_dir = Path.home() / ".aws"
    credentials_path = Path(os.environ.get("AWS_SHARED_CREDENTIALS_FILE") or aws_dir / "credentials").expanduser()
    config_path = Path(os.environ.get("AWS_CONFIG_FILE") or aws_dir / "config").expanduser()
    if credentials_path == config_path:
        raise RuntimeError("AWS config and shared credentials paths must be different")
    return credentials_path, config_path


def write_profiles(
    profile: str,
    role_profile: str,
    role_arn: str,
    region: str,
    credentials: dict[str, str],
    *,
    credentials_path: Path,
    config_path: Path,
) -> None:
    for directory in {credentials_path.parent, config_path.parent}:
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    creds = configparser.RawConfigParser()
    creds.read(credentials_path)
    creds[profile] = credentials
    atomic_write_config(credentials_path, creds)
    config = configparser.RawConfigParser()
    config.read(config_path)
    config[f"profile {profile}"] = {"region": region}
    config[f"profile {role_profile}"] = {"role_arn": role_arn, "source_profile": profile, "region": region}
    atomic_write_config(config_path, config)


def profile_verification_environment(credentials_path: Path, config_path: Path, region: str) -> dict[str, str]:
    """Pin role verification to newly written profiles, not ambient credentials."""
    env = os.environ.copy()
    for name in (
        "AWS_ACCESS_KEY_ID",
        "AWS_SECRET_ACCESS_KEY",
        "AWS_SESSION_TOKEN",
        "AWS_SECURITY_TOKEN",
        "AWS_PROFILE",
        "AWS_REGION",
        "AWS_DEFAULT_REGION",
        "AWS_ROLE_ARN",
        "AWS_ROLE_SESSION_NAME",
        "AWS_WEB_IDENTITY_TOKEN_FILE",
    ):
        env.pop(name, None)
    env.update(
        {
            "AWS_CONFIG_FILE": str(config_path),
            "AWS_SHARED_CREDENTIALS_FILE": str(credentials_path),
            "AWS_DEFAULT_REGION": region,
        }
    )
    return env


def atomic_write_config(path: Path, config: configparser.RawConfigParser) -> None:
    temporary = path.with_name(f".{path.name}.{os.getpid()}.tmp")
    try:
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            config.write(file)
            file.flush()
            os.fsync(file.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def reservation_payload(source_user: str, reservation_id: str) -> dict[str, object]:
    return {
        "record_version": RECOVERY_RECORD_VERSION,
        "state": RESERVED_BEFORE_CREATE,
        "source_user": source_user,
        "reservation_id": reservation_id,
        "required_action": "reconcile source-user access keys before retrying",
    }


def key_cleanup_payload(source_user: str, reservation_id: str, key_id: str) -> dict[str, object]:
    return {
        "record_version": RECOVERY_RECORD_VERSION,
        "state": ACCESS_KEY_CREATED,
        "source_user": source_user,
        "reservation_id": reservation_id,
        "access_key_id": key_id,
        "required_action": "delete temporary source access key before removing this record",
    }


def fsync_parent(path: Path) -> None:
    """Persist a directory entry after creating or deleting a recovery record."""
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    descriptor = os.open(path.parent, flags)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def read_recovery_record(path: Path) -> dict[str, object]:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError("recovery record could not be read safely; preserve it and reconcile before retrying") from exc
    if not isinstance(payload, dict):
        raise RuntimeError("recovery record is not an object; preserve it and reconcile before retrying")
    return payload


def write_temporary_record(path: Path, payload: dict[str, object]) -> Path:
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            os.fchmod(file.fileno(), 0o600)
            file.write(json.dumps(payload, sort_keys=True) + "\n")
            file.flush()
            os.fsync(file.fileno())
    except Exception:
        if temporary.exists():
            temporary.unlink()
        raise
    return temporary


def reserve_recovery_record(path: Path, source_user: str) -> dict[str, object]:
    """Durably reserve the only recovery slot before the IAM create call."""
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    payload = reservation_payload(source_user, uuid.uuid4().hex)
    temporary = write_temporary_record(path, payload)
    try:
        # link(2) is an atomic exclusive create. A concurrent invocation or a
        # crash residue can never be overwritten by a later create attempt.
        os.link(temporary, path)
        fsync_parent(path)
    except FileExistsError as exc:
        raise RuntimeError(
            "an unresolved temporary source access-key cleanup record already exists; "
            "refusing to overwrite it"
        ) from exc
    finally:
        if temporary.exists():
            temporary.unlink()
    return payload


def replace_owned_recovery_record(path: Path, expected: dict[str, object], replacement: dict[str, object]) -> None:
    """Advance only this invocation's reservation to its known access-key state."""
    if read_recovery_record(path) != expected:
        raise RuntimeError("recovery record changed; preserving it for operator reconciliation")
    temporary = write_temporary_record(path, replacement)
    try:
        os.replace(temporary, path)
        fsync_parent(path)
    finally:
        if temporary.exists():
            temporary.unlink()


def promote_recovery_reservation(path: Path, reservation: dict[str, object], source_user: str, key_id: str) -> dict[str, object]:
    payload = key_cleanup_payload(source_user, str(reservation["reservation_id"]), key_id)
    replace_owned_recovery_record(path, reservation, payload)
    return payload


def remove_own_recovery_record(path: Path, reservation: dict[str, object], source_user: str, key_id: str) -> None:
    """Remove only this invocation's reservation after confirmed key deletion."""
    current = read_recovery_record(path)
    key_record = key_cleanup_payload(source_user, str(reservation["reservation_id"]), key_id)
    if current not in (reservation, key_record):
        raise RuntimeError("temporary source key was deleted but its recovery record changed; preserving it for operator review")
    path.unlink()
    fsync_parent(path)


def recovery_status(path: Path) -> dict[str, object]:
    """Return only actionable, non-secret local recovery state."""
    payload = read_recovery_record(path)
    state = payload.get("state")
    common = {"record_version", "state", "source_user", "reservation_id", "required_action"}
    if (
        state == RESERVED_BEFORE_CREATE
        and set(payload) == common
        and payload.get("record_version") == RECOVERY_RECORD_VERSION
        and isinstance(payload.get("source_user"), str)
        and SAFE_NAME.fullmatch(payload["source_user"])
        and isinstance(payload.get("reservation_id"), str)
        and RESERVATION_ID.fullmatch(payload["reservation_id"])
        and payload.get("required_action") == "reconcile source-user access keys before retrying"
    ):
        return payload
    key_fields = common | {"access_key_id"}
    if (
        state == ACCESS_KEY_CREATED
        and set(payload) == key_fields
        and payload.get("record_version") == RECOVERY_RECORD_VERSION
        and isinstance(payload.get("source_user"), str)
        and SAFE_NAME.fullmatch(payload["source_user"])
        and isinstance(payload.get("reservation_id"), str)
        and RESERVATION_ID.fullmatch(payload["reservation_id"])
        and isinstance(payload.get("access_key_id"), str)
        and ACCESS_KEY_ID.fullmatch(payload["access_key_id"])
        and payload.get("required_action") == "delete temporary source access key before removing this record"
    ):
        return payload
    raise RuntimeError("recovery record has an unknown format; preserve it and reconcile before retrying")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--admin-profile", default="default")
    parser.add_argument("--source-user")
    parser.add_argument("--mfa-serial")
    parser.add_argument("--macos-keychain-service")
    parser.add_argument("--macos-keychain-account")
    parser.add_argument("--session-profile", default="smartrouter")
    parser.add_argument("--role-profile", default="genai-smart-router-eks-discovery")
    parser.add_argument("--role-arn")
    parser.add_argument("--region")
    parser.add_argument("--duration-seconds", type=int, default=3600)
    parser.add_argument("--propagation-wait-seconds", type=int, default=8)
    parser.add_argument("--cleanup-record", type=Path)
    parser.add_argument("--recovery-status", type=Path, help="read-only safe status for an existing recovery record")
    args = parser.parse_args()
    if args.recovery_status:
        print(json.dumps(recovery_status(args.recovery_status), sort_keys=True))
        return 0
    for name in ("source_user", "mfa_serial", "macos_keychain_service", "macos_keychain_account", "role_arn", "region", "cleanup_record"):
        if getattr(args, name) in {None, ""}:
            parser.error(f"--{name.replace('_', '-')} is required for session bootstrap")
    for value in (args.admin_profile, args.source_user, args.macos_keychain_account, args.session_profile, args.role_profile):
        if not SAFE_NAME.fullmatch(value):
            parser.error("profile and identity names must be simple identifiers")
    if not SERIAL.fullmatch(args.mfa_serial) or not ARN.fullmatch(args.role_arn) or not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)+", args.region):
        parser.error("invalid MFA serial, role ARN, or region")
    if not 900 <= args.duration_seconds <= 43200 or not 0 <= args.propagation_wait_seconds <= 30:
        parser.error("invalid session duration or propagation wait")
    assert args.cleanup_record is not None
    if str(args.cleanup_record) in {"", "."} or args.cleanup_record.is_dir():
        parser.error("--cleanup-record must be a non-directory protected file path")
    credentials_path, config_path = configured_aws_profile_paths()
    approved_account = args.role_arn.split(":")[4]
    if args.mfa_serial.split(":")[4] != approved_account:
        parser.error("role ARN and MFA serial must use the same approved account")
    admin_identity = json.loads(command(["aws", "sts", "get-caller-identity", "--profile", args.admin_profile, "--output", "json"]))
    if str(admin_identity.get("Account", "")) != approved_account or str(admin_identity.get("Arn", "")).endswith(":root"):
        raise RuntimeError("admin profile must be a non-root identity in the approved account before creating an access key")

    # This atomic reservation is intentionally immediately before the only IAM
    # mutation. If this process is killed or the create result is ambiguous, it
    # remains in place and blocks a retry until an operator reconciles it.
    reservation = reserve_recovery_record(args.cleanup_record, args.source_user)
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
        try:
            access = json.loads(command(["aws", "iam", "create-access-key", "--profile", args.admin_profile, "--user-name", args.source_user, "--output", "json"]))["AccessKey"]
            candidate_key_id = access["AccessKeyId"]
            if not isinstance(candidate_key_id, str) or not ACCESS_KEY_ID.fullmatch(candidate_key_id):
                raise ValueError("create-access-key did not return a safe access-key identifier")
            key_id = candidate_key_id
        except (KeyError, TypeError, ValueError, json.JSONDecodeError, RuntimeError) as exc:
            raise RuntimeError(
                f"source key creation outcome is unknown; do not retry until recovery record is reconciled: {args.cleanup_record}"
            ) from exc
        # The reservation already protects the narrow create-result window.
        # Promote it to an exact key-ID record before any later network call.
        promote_recovery_reservation(args.cleanup_record, reservation, args.source_user, key_id)
        if args.propagation_wait_seconds:
            time.sleep(args.propagation_wait_seconds)
        env = os.environ.copy()
        for name in ("AWS_PROFILE", "AWS_SESSION_TOKEN"):
            env.pop(name, None)
        env.update({"AWS_CONFIG_FILE": os.devnull, "AWS_SHARED_CREDENTIALS_FILE": os.devnull, "AWS_ACCESS_KEY_ID": access["AccessKeyId"], "AWS_SECRET_ACCESS_KEY": access["SecretAccessKey"], "AWS_DEFAULT_REGION": args.region})
        session = json.loads(command(["aws", "sts", "get-session-token", "--serial-number", args.mfa_serial, "--token-code", mfa_code_from_keychain(args.macos_keychain_service, args.macos_keychain_account), "--duration-seconds", str(args.duration_seconds), "--output", "json"], env=env))["Credentials"]
        write_profiles(
            args.session_profile,
            args.role_profile,
            args.role_arn,
            args.region,
            {"aws_access_key_id": session["AccessKeyId"], "aws_secret_access_key": session["SecretAccessKey"], "aws_session_token": session["SessionToken"]},
            credentials_path=credentials_path,
            config_path=config_path,
        )
        deleted = delete_source_key()
        if not deleted:
            raise RuntimeError("temporary source key deletion failed after retries")
        remove_own_recovery_record(args.cleanup_record, reservation, args.source_user, key_id)
        key_id = ""
        role_identity = json.loads(
            command(
                ["aws", "sts", "get-caller-identity", "--profile", args.role_profile, "--region", args.region, "--output", "json"],
                env=profile_verification_environment(credentials_path, config_path, args.region),
            )
        )
        print(json.dumps({"session_profile": args.session_profile, "role_profile": args.role_profile, "role_identity": role_identity["Arn"], "session_expiration": session["Expiration"], "source_access_key_deleted": True}))
        return 0
    finally:
        if key_id and not deleted:
            deleted = delete_source_key()
            if deleted:
                remove_own_recovery_record(args.cleanup_record, reservation, args.source_user, key_id)
                key_id = ""
            else:
                raise RuntimeError(f"temporary source key deletion failed; cleanup record: {args.cleanup_record}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(f"EKS session bootstrap failed safely: {exc}", file=sys.stderr)
        raise SystemExit(2)
