#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Mint a bounded MFA session and configure a read-only EKS role profile."""

from __future__ import annotations

import argparse
import base64
import configparser
import fcntl
import hashlib
import hmac
import io
import json
import os
import re
import stat
import subprocess
import sys
import tempfile
import time
import uuid
from contextlib import contextmanager
from collections.abc import Iterator
from pathlib import Path


SAFE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")
ARN = re.compile(r"^arn:aws:iam::[0-9]{12}:(?:user|role)/[A-Za-z0-9+=,.@_/-]+$")
SERIAL = re.compile(r"^arn:aws:iam::[0-9]{12}:mfa/[A-Za-z0-9+=,.@_/-]+$")
RESERVATION_ID = re.compile(r"^[a-f0-9]{32}$")
ACCESS_KEY_ID = re.compile(r"^[A-Z0-9]{16,128}$")
RECOVERY_RECORD_VERSION = 1
RESERVED_BEFORE_CREATE = "reserved_before_create"
ACCESS_KEY_CREATED = "access_key_created"
DISCOVERY_ROLE_NAME = "genai-smart-router-eks-discovery"
ROOT = Path(__file__).resolve().parents[1]


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


def validate_profile_file_path(path: Path, label: str) -> Path:
    """Require a private, non-repository file target before writing STS credentials."""
    if not path.is_absolute() or path.parent == Path("/"):
        raise RuntimeError(f"{label} must be an absolute non-root file path")
    try:
        # Reject the selected file and immediate directory when either is a
        # symlink.  Ancestor aliases such as macOS /var -> /private/var are
        # permitted after resolving the final path outside this repository.
        if path.is_symlink() or path.parent.is_symlink():
            raise RuntimeError(f"{label} must not be a symlink")
        resolved = path.resolve(strict=False)
        if resolved.is_relative_to(ROOT.resolve()):
            raise RuntimeError(f"{label} must remain outside the repository")
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        parent_metadata = path.parent.stat()
        if not stat.S_ISDIR(parent_metadata.st_mode) or stat.S_IMODE(parent_metadata.st_mode) & 0o022:
            raise RuntimeError(f"{label} parent directory must not be group- or world-writable")
        if path.exists() and not stat.S_ISREG(path.stat().st_mode):
            raise RuntimeError(f"{label} must be a regular file when it already exists")
    except RuntimeError:
        raise
    except OSError as exc:
        raise RuntimeError(f"{label} could not be prepared safely") from exc
    return path


def configured_aws_profile_paths() -> tuple[Path, Path]:
    """Return private AWS CLI profile files selected for this process."""
    aws_dir = Path.home() / ".aws"
    credentials_path = validate_profile_file_path(
        Path(os.environ.get("AWS_SHARED_CREDENTIALS_FILE") or aws_dir / "credentials").expanduser(),
        "AWS shared credentials path",
    )
    config_path = validate_profile_file_path(
        Path(os.environ.get("AWS_CONFIG_FILE") or aws_dir / "config").expanduser(),
        "AWS config path",
    )
    if credentials_path.resolve(strict=False) == config_path.resolve(strict=False):
        raise RuntimeError("AWS config and shared credentials paths must be different")
    return credentials_path, config_path


def profile_pair_lock_path(credentials_path: Path, config_path: Path) -> Path:
    """Return the private, deterministic advisory lock for one AWS profile pair."""
    resolved_paths = sorted(
        (str(credentials_path.resolve(strict=False)), str(config_path.resolve(strict=False)))
    )
    pair_identity = "\0".join(resolved_paths).encode("utf-8")
    digest = hashlib.sha256(pair_identity).hexdigest()
    return credentials_path.resolve(strict=False).parent / f".genai-smart-router-profile-{digest}.lock"


@contextmanager
def locked_profile_pair(credentials_path: Path, config_path: Path) -> Iterator[None]:
    """Serialize one profile pair from snapshot through publication or rollback.

    The lock is a separate file because profile publication uses atomic rename;
    locking either profile inode would cease protecting its replacement.
    """
    lock_path = profile_pair_lock_path(credentials_path, config_path)
    flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptor = -1
    acquired = False
    try:
        if lock_path.is_symlink():
            raise RuntimeError("AWS profile lock must not be a symlink")
        descriptor = os.open(lock_path, flags, 0o600)
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode) or stat.S_IMODE(metadata.st_mode) & 0o077:
            raise RuntimeError("AWS profile lock must be a private regular file")
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        acquired = True
        yield
    except RuntimeError:
        raise
    except OSError as exc:
        raise RuntimeError("AWS profile lock could not be acquired safely") from exc
    finally:
        if descriptor >= 0:
            try:
                if acquired:
                    fcntl.flock(descriptor, fcntl.LOCK_UN)
            finally:
                os.close(descriptor)


def validate_cleanup_record_path(path: Path) -> Path:
    """Require durable local recovery state before any IAM mutation."""
    if not path.is_absolute() or path.parent == Path("/"):
        raise RuntimeError("cleanup record must be an absolute non-root file path")
    try:
        # Reject the record and its immediate parent when either is a symlink.
        # Existing platform aliases such as macOS /var are permitted after
        # resolving the final record outside this repository.
        if path.is_symlink() or path.parent.is_symlink():
            raise RuntimeError("cleanup record must not be a symlink")
        resolved = path.resolve(strict=False)
        if resolved.is_relative_to(ROOT.resolve()):
            raise RuntimeError("cleanup record must remain outside the repository")
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        parent_metadata = path.parent.stat()
        if not stat.S_ISDIR(parent_metadata.st_mode) or stat.S_IMODE(parent_metadata.st_mode) & 0o022:
            raise RuntimeError("cleanup record parent directory must not be group- or world-writable")
        try:
            metadata = path.lstat()
        except FileNotFoundError:
            return path
        if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISREG(metadata.st_mode):
            raise RuntimeError("cleanup record must be a regular non-symlink file when it already exists")
        if stat.S_IMODE(metadata.st_mode) & 0o077:
            raise RuntimeError("cleanup record must be private when it already exists")
    except RuntimeError:
        raise
    except OSError as exc:
        raise RuntimeError("cleanup record could not be prepared safely") from exc
    return path


def write_profiles(
    profile: str,
    role_profile: str,
    role_arn: str,
    region: str,
    credentials: dict[str, str],
    *,
    credentials_path: Path,
    config_path: Path,
) -> tuple[tuple[bytes, int] | None, tuple[bytes, int] | None]:
    for directory in {credentials_path.parent, config_path.parent}:
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    prior_profiles = profile_files_snapshot(credentials_path, config_path)
    creds = configparser.RawConfigParser()
    creds.read(credentials_path)
    creds[profile] = credentials
    published_profiles = prior_profiles
    try:
        published_credentials = atomic_write_config(credentials_path, creds)
        published_profiles = (published_credentials, prior_profiles[1])
        if profile_files_snapshot(credentials_path, config_path) != published_profiles:
            raise RuntimeError("AWS profile files changed concurrently; refusing to publish paired profiles")
        config = configparser.RawConfigParser()
        config.read(config_path)
        config[f"profile {profile}"] = {"region": region}
        config[f"profile {role_profile}"] = {"role_arn": role_arn, "source_profile": profile, "region": region}
        published_config = atomic_write_config(config_path, config)
        return published_credentials, published_config
    except Exception:
        try:
            restore_profile_files(
                credentials_path,
                config_path,
                prior_profiles,
                expected_current=published_profiles,
            )
        except Exception as rollback_error:
            raise RuntimeError(
                "AWS profile update failed and prior profile files could not be restored; do not use either profile"
            ) from rollback_error
        raise


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


def validate_discovery_role_identity(identity: object, approved_account: str) -> str:
    """Require the exact approved discovery role before bootstrap can succeed."""
    if not isinstance(identity, dict) or str(identity.get("Account", "")) != approved_account:
        raise RuntimeError("configured role profile identity account does not match the approved account")
    arn = str(identity.get("Arn", ""))
    expected_prefix = f"arn:aws:sts::{approved_account}:assumed-role/{DISCOVERY_ROLE_NAME}/"
    if not arn.startswith(expected_prefix):
        raise RuntimeError("configured role profile is not the expected discovery assumed role")
    return arn


def atomic_write_config(path: Path, config: configparser.RawConfigParser) -> tuple[bytes, int]:
    serialized = io.StringIO()
    config.write(serialized)
    content = serialized.getvalue().encode("utf-8")
    temporary = path.with_name(f".{path.name}.{os.getpid()}.tmp")
    try:
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "wb") as file:
            os.fchmod(file.fileno(), 0o600)
            file.write(content)
            file.flush()
            os.fsync(file.fileno())
        os.replace(temporary, path)
        return content, 0o600
    finally:
        if temporary.exists():
            temporary.unlink()


def profile_file_snapshot(path: Path) -> tuple[bytes, int] | None:
    """Retain only the previous local credentials bytes long enough to roll back a paired write."""
    if path.is_symlink():
        raise RuntimeError("AWS profile file must not be a symlink")
    if not path.exists():
        return None
    metadata = path.stat()
    if not stat.S_ISREG(metadata.st_mode):
        raise RuntimeError("AWS shared credentials path must be a regular file")
    return path.read_bytes(), stat.S_IMODE(metadata.st_mode)


def profile_files_snapshot(credentials_path: Path, config_path: Path) -> tuple[tuple[bytes, int] | None, tuple[bytes, int] | None]:
    """Capture both profile files before an operation that may publish local authority."""
    return profile_file_snapshot(credentials_path), profile_file_snapshot(config_path)


def atomic_write_bytes(path: Path, content: bytes, mode: int) -> None:
    temporary = path.with_name(f".{path.name}.{os.getpid()}.rollback.tmp")
    try:
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
        with os.fdopen(descriptor, "wb") as file:
            os.fchmod(file.fileno(), mode)
            file.write(content)
            file.flush()
            os.fsync(file.fileno())
        os.replace(temporary, path)
        fsync_parent(path)
    finally:
        if temporary.exists():
            temporary.unlink()


def restore_profile_file(path: Path, snapshot: tuple[bytes, int] | None) -> None:
    """Undo only this invocation's credentials update when config publication fails."""
    if snapshot is None:
        path.unlink(missing_ok=True)
        fsync_parent(path)
        return
    content, mode = snapshot
    atomic_write_bytes(path, content, mode)


def restore_profile_files(
    credentials_path: Path,
    config_path: Path,
    snapshots: tuple[tuple[bytes, int] | None, tuple[bytes, int] | None],
    *,
    expected_current: tuple[tuple[bytes, int] | None, tuple[bytes, int] | None] | None = None,
) -> None:
    """Restore both AWS profile files after any failed authority publication."""
    if expected_current is not None and profile_files_snapshot(credentials_path, config_path) != expected_current:
        raise RuntimeError("AWS profile files changed concurrently; refusing to overwrite them during rollback")
    credentials_snapshot, config_snapshot = snapshots
    restore_profile_file(config_path, config_snapshot)
    restore_profile_file(credentials_path, credentials_snapshot)


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


def bootstrap_with_locked_profile_pair(
    args: argparse.Namespace,
    credentials_path: Path,
    config_path: Path,
    approved_account: str,
    cleanup_record: Path,
) -> int:
    """Publish and verify local AWS profiles without racing a paired invocation."""
    # The lock begins before the rollback snapshot and remains held until this
    # invocation either succeeds or completes its rollback in the finally block.
    with locked_profile_pair(credentials_path, config_path):
        profile_snapshots = profile_files_snapshot(credentials_path, config_path)

        # This atomic reservation is intentionally immediately before the only IAM
        # mutation. If this process is killed or the create result is ambiguous, it
        # remains in place and blocks a retry until an operator reconciles it.
        reservation = reserve_recovery_record(cleanup_record, args.source_user)
        key_id = ""
        deleted = False
        profiles_published = False
        published_profiles: tuple[tuple[bytes, int] | None, tuple[bytes, int] | None] | None = None
        completed = False

        def delete_source_key() -> bool:
            for _ in range(3):
                try:
                    command([
                        "aws",
                        "iam",
                        "delete-access-key",
                        "--profile",
                        args.admin_profile,
                        "--user-name",
                        args.source_user,
                        "--access-key-id",
                        key_id,
                    ])
                    return True
                except RuntimeError:
                    time.sleep(1)
            return False

        try:
            try:
                access = json.loads(command([
                    "aws",
                    "iam",
                    "create-access-key",
                    "--profile",
                    args.admin_profile,
                    "--user-name",
                    args.source_user,
                    "--output",
                    "json",
                ]))["AccessKey"]
                candidate_key_id = access["AccessKeyId"]
                if not isinstance(candidate_key_id, str) or not ACCESS_KEY_ID.fullmatch(candidate_key_id):
                    raise ValueError("create-access-key did not return a safe access-key identifier")
                key_id = candidate_key_id
            except (KeyError, TypeError, ValueError, json.JSONDecodeError, RuntimeError) as exc:
                raise RuntimeError(
                    f"source key creation outcome is unknown; do not retry until recovery record is reconciled: {cleanup_record}"
                ) from exc
            # The reservation already protects the narrow create-result window.
            # Promote it to an exact key-ID record before any later network call.
            promote_recovery_reservation(cleanup_record, reservation, args.source_user, key_id)
            if args.propagation_wait_seconds:
                time.sleep(args.propagation_wait_seconds)
            env = os.environ.copy()
            for name in ("AWS_PROFILE", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN"):
                env.pop(name, None)
            env.update({
                "AWS_CONFIG_FILE": os.devnull,
                "AWS_SHARED_CREDENTIALS_FILE": os.devnull,
                "AWS_ACCESS_KEY_ID": access["AccessKeyId"],
                "AWS_SECRET_ACCESS_KEY": access["SecretAccessKey"],
                "AWS_DEFAULT_REGION": args.region,
            })
            session = json.loads(command([
                "aws",
                "sts",
                "get-session-token",
                "--serial-number",
                args.mfa_serial,
                "--token-code",
                mfa_code_from_keychain(args.macos_keychain_service, args.macos_keychain_account),
                "--duration-seconds",
                str(args.duration_seconds),
                "--output",
                "json",
            ], env=env))["Credentials"]
            published_profiles = write_profiles(
                args.session_profile,
                args.role_profile,
                args.role_arn,
                args.region,
                {
                    "aws_access_key_id": session["AccessKeyId"],
                    "aws_secret_access_key": session["SecretAccessKey"],
                    "aws_session_token": session["SessionToken"],
                },
                credentials_path=credentials_path,
                config_path=config_path,
            )
            profiles_published = True
            role_identity = json.loads(
                command(
                    [
                        "aws",
                        "sts",
                        "get-caller-identity",
                        "--profile",
                        args.role_profile,
                        "--region",
                        args.region,
                        "--output",
                        "json",
                    ],
                    env=profile_verification_environment(credentials_path, config_path, args.region),
                )
            )
            verified_role_arn = validate_discovery_role_identity(role_identity, approved_account)
            deleted = delete_source_key()
            if not deleted:
                raise RuntimeError("temporary source key deletion failed after retries")
            remove_own_recovery_record(cleanup_record, reservation, args.source_user, key_id)
            key_id = ""
            completed = True
            print(json.dumps({
                "session_profile": args.session_profile,
                "role_profile": args.role_profile,
                "role_identity": verified_role_arn,
                "session_expiration": session["Expiration"],
                "source_access_key_deleted": True,
            }))
            return 0
        finally:
            cleanup_error: Exception | None = None
            rollback_error: Exception | None = None
            if key_id and not deleted:
                deleted = delete_source_key()
                if deleted:
                    try:
                        remove_own_recovery_record(cleanup_record, reservation, args.source_user, key_id)
                    except Exception as exc:
                        cleanup_error = exc
                    else:
                        key_id = ""
            if profiles_published and not completed:
                try:
                    if published_profiles is None:
                        raise RuntimeError("AWS profile publication state was not captured; refusing unsafe rollback")
                    restore_profile_files(
                        credentials_path,
                        config_path,
                        profile_snapshots,
                        expected_current=published_profiles,
                    )
                except Exception as exc:
                    rollback_error = exc
            if rollback_error is not None:
                raise RuntimeError("failed EKS bootstrap could not restore prior AWS profile files; do not use either profile") from rollback_error
            if cleanup_error is not None:
                raise RuntimeError("temporary source key was deleted but its cleanup record could not be removed; preserve it for operator reconciliation") from cleanup_error
            if key_id and not deleted:
                raise RuntimeError(f"temporary source key deletion failed; cleanup record: {cleanup_record}")


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
    cleanup_record = validate_cleanup_record_path(args.cleanup_record)
    credentials_path, config_path = configured_aws_profile_paths()
    approved_account = args.role_arn.split(":")[4]
    if args.mfa_serial.split(":")[4] != approved_account:
        parser.error("role ARN and MFA serial must use the same approved account")
    expected_role_arn = f"arn:aws:iam::{approved_account}:role/{DISCOVERY_ROLE_NAME}"
    if args.role_arn != expected_role_arn:
        parser.error("--role-arn must be the exact approved discovery role")
    admin_identity = json.loads(command(["aws", "sts", "get-caller-identity", "--profile", args.admin_profile, "--output", "json"]))
    if str(admin_identity.get("Account", "")) != approved_account or str(admin_identity.get("Arn", "")).endswith(":root"):
        raise RuntimeError("admin profile must be a non-root identity in the approved account before creating an access key")
    return bootstrap_with_locked_profile_pair(
        args,
        credentials_path,
        config_path,
        approved_account,
        cleanup_record,
    )


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(f"EKS session bootstrap failed safely: {exc}", file=sys.stderr)
        raise SystemExit(2)
