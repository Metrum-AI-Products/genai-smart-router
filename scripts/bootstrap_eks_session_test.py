#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for local EKS session file safety helpers."""

from __future__ import annotations

import configparser
import importlib.util
import json
import os
import stat
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("bootstrap_eks_session", ROOT / "scripts/bootstrap_eks_session.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load bootstrap session helpers")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def mode(path: Path) -> int:
    return stat.S_IMODE(path.stat().st_mode)


def wait_for_path(path: Path, description: str, timeout_seconds: float = 5) -> None:
    deadline = time.monotonic() + timeout_seconds
    while not path.exists():
        if time.monotonic() >= deadline:
            raise AssertionError(f"timed out waiting for {description}")
        time.sleep(0.01)


def start_profile_lock_contender(lock_path: Path, root: Path) -> tuple[subprocess.Popen[bytes], Path, Path, Path]:
    attempted = root / "lock-contender-attempted"
    acquired = root / "lock-contender-acquired"
    release = root / "lock-contender-release"
    script = """
import fcntl
import os
from pathlib import Path
import sys
import time

lock_path = Path(sys.argv[1])
attempted = Path(sys.argv[2])
acquired = Path(sys.argv[3])
release = Path(sys.argv[4])
descriptor = os.open(lock_path, os.O_RDWR | os.O_CREAT, 0o600)
try:
    attempted.write_text("attempted", encoding="utf-8")
    fcntl.flock(descriptor, fcntl.LOCK_EX)
    acquired.write_text("acquired", encoding="utf-8")
    while not release.exists():
        time.sleep(0.01)
finally:
    fcntl.flock(descriptor, fcntl.LOCK_UN)
    os.close(descriptor)
"""
    process = subprocess.Popen(
        [sys.executable, "-c", script, str(lock_path), str(attempted), str(acquired), str(release)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return process, attempted, acquired, release


def finish_lock_contender(process: subprocess.Popen[bytes], release: Path) -> None:
    release.touch()
    try:
        _, stderr = process.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        _, stderr = process.communicate(timeout=5)
        raise AssertionError("profile lock contender did not exit")
    if process.returncode:
        raise AssertionError(f"profile lock contender failed: {stderr.decode('utf-8', errors='replace')}")


def bootstrap_argv(cleanup: Path) -> list[str]:
    return [
        "bootstrap_eks_session.py",
        "--admin-profile", "admin",
        "--source-user", "smartrouter",
        "--mfa-serial", "arn:aws:iam::123456789012:mfa/smartrouter",
        "--macos-keychain-service", "test-mfa",
        "--macos-keychain-account", "smartrouter",
        "--role-arn", "arn:aws:iam::123456789012:role/genai-smart-router-eks-discovery",
        "--region", "us-east-1",
        "--propagation-wait-seconds", "0",
        "--cleanup-record", str(cleanup),
    ]


def run_main_with_stubs(cleanup: Path, command, *, stub_profiles: bool = True) -> int:
    original_argv = sys.argv
    original_command = MODULE.command
    original_mfa = MODULE.mfa_code_from_keychain
    original_profiles = MODULE.write_profiles
    sys.argv = bootstrap_argv(cleanup)
    MODULE.command = command
    MODULE.mfa_code_from_keychain = lambda service, account: "123456"
    if stub_profiles:
        MODULE.write_profiles = lambda *args, **kwargs: MODULE.profile_files_snapshot(  # type: ignore[method-assign]
            kwargs["credentials_path"],
            kwargs["config_path"],
        )
    try:
        return MODULE.main()
    finally:
        sys.argv = original_argv
        MODULE.command = original_command
        MODULE.mfa_code_from_keychain = original_mfa
        MODULE.write_profiles = original_profiles


def assert_precreate_reservation_order(root: Path) -> None:
    cleanup = root / "ordered" / "recovery.json"

    def command(args: list[str], *, env=None) -> str:
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            profile = args[args.index("--profile") + 1]
            if profile == "admin":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
            if profile == "genai-smart-router-eks-discovery":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/genai-smart-router-eks-discovery/test"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            if MODULE.recovery_status(cleanup)["state"] != MODULE.RESERVED_BEFORE_CREATE:
                raise AssertionError("IAM create was attempted before the exclusive recovery reservation")
            return json.dumps({"AccessKey": {"AccessKeyId": "AKIAEXAMPLEKEYID", "SecretAccessKey": "test-only-secret"}})
        if args[:3] == ["aws", "sts", "get-session-token"]:
            if MODULE.recovery_status(cleanup)["state"] != MODULE.ACCESS_KEY_CREATED:
                raise AssertionError("known source key was not recorded before STS exchange")
            return json.dumps({"Credentials": {"AccessKeyId": "ASIAEXAMPLEKEYID", "SecretAccessKey": "test-only-session-secret", "SessionToken": "test-only-session-token", "Expiration": "2030-01-01T00:00:00Z"}})
        if args[:3] == ["aws", "iam", "delete-access-key"]:
            return ""
        raise AssertionError(f"unexpected command: {args}")

    if run_main_with_stubs(cleanup, command) != 0:
        raise AssertionError("stubbed bootstrap did not complete")
    if cleanup.exists():
        raise AssertionError("confirmed source-key deletion did not release its reservation")


def assert_ambiguous_create_preserves_reservation(root: Path) -> None:
    cleanup = root / "ambiguous" / "recovery.json"

    def command(args: list[str], *, env=None) -> str:
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            if MODULE.recovery_status(cleanup)["state"] != MODULE.RESERVED_BEFORE_CREATE:
                raise AssertionError("ambiguous create was attempted without a reservation")
            raise RuntimeError("simulated ambiguous create response")
        raise AssertionError(f"unexpected command: {args}")

    try:
        run_main_with_stubs(cleanup, command)
    except RuntimeError as exc:
        if "outcome is unknown" not in str(exc):
            raise
    else:
        raise AssertionError("ambiguous create outcome must fail closed")
    if MODULE.recovery_status(cleanup)["state"] != MODULE.RESERVED_BEFORE_CREATE:
        raise AssertionError("ambiguous create outcome did not retain the reconciliation reservation")


def assert_concurrent_reservation_is_exclusive(root: Path) -> None:
    """Only one simultaneous bootstrap can reach the IAM-create boundary."""
    cleanup = root / "concurrent" / "recovery.json"
    barrier = threading.Barrier(8)
    lock = threading.Lock()
    successes: list[dict[str, object]] = []
    failures: list[Exception] = []

    def reserve() -> None:
        barrier.wait()
        try:
            reservation = MODULE.reserve_recovery_record(cleanup, "smartrouter")
        except RuntimeError as exc:
            with lock:
                failures.append(exc)
        else:
            with lock:
                successes.append(reservation)

    workers = [threading.Thread(target=reserve) for _ in range(8)]
    for worker in workers:
        worker.start()
    for worker in workers:
        worker.join()
    if len(successes) != 1 or len(failures) != 7:
        raise AssertionError("simultaneous bootstrap attempts must have exactly one recovery reservation winner")
    if MODULE.read_recovery_record(cleanup) != successes[0]:
        raise AssertionError("concurrent bootstrap attempts changed the winning recovery reservation")


def assert_configured_profile_paths_and_role_verification(root: Path) -> None:
    cleanup = root / "configured-profiles" / "recovery.json"
    credentials_path = root / "custom-aws" / "credentials"
    config_path = root / "custom-aws" / "config"
    names = (
        "AWS_CONFIG_FILE",
        "AWS_SHARED_CREDENTIALS_FILE",
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
    )
    original = {name: os.environ.get(name) for name in names}
    os.environ.update(
        {
            "AWS_CONFIG_FILE": str(config_path),
            "AWS_SHARED_CREDENTIALS_FILE": str(credentials_path),
            "AWS_ACCESS_KEY_ID": "ambient-test-access-key",
            "AWS_SECRET_ACCESS_KEY": "ambient-test-secret",
            "AWS_SESSION_TOKEN": "ambient-test-session-token",
            "AWS_SECURITY_TOKEN": "ambient-test-security-token",
            "AWS_PROFILE": "ambient-profile",
            "AWS_REGION": "us-west-2",
            "AWS_DEFAULT_REGION": "us-west-2",
            "AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/ambient-role",
            "AWS_ROLE_SESSION_NAME": "ambient-session",
            "AWS_WEB_IDENTITY_TOKEN_FILE": "/tmp/ambient-web-identity-token",
        }
    )
    role_verification_seen = False

    def command(args: list[str], *, env=None) -> str:
        nonlocal role_verification_seen
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            profile = args[args.index("--profile") + 1]
            if profile == "admin":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
            if profile == "genai-smart-router-eks-discovery":
                if env is None:
                    raise AssertionError("role verification must use an explicitly pinned profile environment")
                if env.get("AWS_CONFIG_FILE") != str(config_path) or env.get("AWS_SHARED_CREDENTIALS_FILE") != str(credentials_path):
                    raise AssertionError("role verification did not use the files written by bootstrap")
                if env.get("AWS_DEFAULT_REGION") != "us-east-1" or args[args.index("--region") + 1] != "us-east-1":
                    raise AssertionError("role verification did not use the requested region")
                if any(
                    name in env
                    for name in (
                        "AWS_ACCESS_KEY_ID",
                        "AWS_SECRET_ACCESS_KEY",
                        "AWS_SESSION_TOKEN",
                        "AWS_SECURITY_TOKEN",
                        "AWS_PROFILE",
                        "AWS_REGION",
                        "AWS_ROLE_ARN",
                        "AWS_ROLE_SESSION_NAME",
                        "AWS_WEB_IDENTITY_TOKEN_FILE",
                    )
                ):
                    raise AssertionError("role verification inherited ambient AWS credentials or role selection")
                role_verification_seen = True
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/genai-smart-router-eks-discovery/test"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            return json.dumps({"AccessKey": {"AccessKeyId": "AKIAEXAMPLEKEYID", "SecretAccessKey": "test-only-secret"}})
        if args[:3] == ["aws", "sts", "get-session-token"]:
            if env is None:
                raise AssertionError("temporary-key STS exchange must use an explicit credential environment")
            if any(name in env for name in ("AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN")):
                raise AssertionError("temporary-key STS exchange inherited an ambient session token")
            return json.dumps({"Credentials": {"AccessKeyId": "ASIAEXAMPLEKEYID", "SecretAccessKey": "test-only-session-secret", "SessionToken": "test-only-session-token", "Expiration": "2030-01-01T00:00:00Z"}})
        if args[:3] == ["aws", "iam", "delete-access-key"]:
            return ""
        raise AssertionError(f"unexpected command: {args}")

    try:
        if run_main_with_stubs(cleanup, command, stub_profiles=False) != 0:
            raise AssertionError("bootstrap with configured AWS profile paths did not complete")
    finally:
        for name, value in original.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value

    if not role_verification_seen:
        raise AssertionError("configured profile role verification was not attempted")
    if mode(credentials_path) != 0o600 or mode(config_path) != 0o600:
        raise AssertionError("configured AWS profile files must be atomically created with mode 0600")
    credentials = configparser.RawConfigParser()
    credentials.read(credentials_path)
    if credentials["smartrouter"].get("aws_access_key_id") != "ASIAEXAMPLEKEYID":
        raise AssertionError("bootstrap did not write the session to AWS_SHARED_CREDENTIALS_FILE")
    config = configparser.RawConfigParser()
    config.read(config_path)
    if config["profile genai-smart-router-eks-discovery"].get("source_profile") != "smartrouter":
        raise AssertionError("bootstrap did not write the role profile to AWS_CONFIG_FILE")


def assert_profile_write_rolls_back_credentials_when_config_write_fails(root: Path) -> None:
    credentials_path = root / "profile-rollback" / "credentials"
    config_path = root / "profile-rollback" / "config"
    credentials_path.parent.mkdir()
    original_credentials = b"[smartrouter]\naws_access_key_id = prior-test-session\n"
    original_config = b"[profile genai-smart-router-eks-discovery]\nregion = us-west-2\n"
    credentials_path.write_bytes(original_credentials)
    config_path.write_bytes(original_config)
    credentials_path.chmod(0o640)
    config_path.chmod(0o600)
    original_atomic_write = MODULE.atomic_write_config

    def fail_only_config(path: Path, config: configparser.RawConfigParser) -> tuple[bytes, int]:
        if path == config_path:
            raise OSError("simulated config publication failure")
        return original_atomic_write(path, config)

    MODULE.atomic_write_config = fail_only_config  # type: ignore[method-assign]
    try:
        try:
            MODULE.write_profiles(
                "smartrouter",
                "genai-smart-router-eks-discovery",
                "arn:aws:iam::123456789012:role/genai-smart-router-eks-discovery",
                "us-east-1",
                {
                    "aws_access_key_id": "new-test-session",
                    "aws_secret_access_key": "new-test-secret",
                    "aws_session_token": "new-test-token",
                },
                credentials_path=credentials_path,
                config_path=config_path,
            )
        except OSError as exc:
            if str(exc) != "simulated config publication failure":
                raise
        else:
            raise AssertionError("profile write accepted a simulated config publication failure")
    finally:
        MODULE.atomic_write_config = original_atomic_write  # type: ignore[method-assign]
    if credentials_path.read_bytes() != original_credentials or mode(credentials_path) != 0o640:
        raise AssertionError("config publication failure did not restore the prior credentials profile exactly")
    if config_path.read_bytes() != original_config:
        raise AssertionError("config publication failure changed the prior role configuration")
    if any(path.name.startswith(f".{credentials_path.name}.") for path in credentials_path.parent.iterdir()):
        raise AssertionError("profile rollback left a credentials temporary file")


def assert_unsafe_profile_paths_fail_before_iam(root: Path) -> None:
    safe_directory = root / "safe-profile-paths"
    safe_credentials = safe_directory / "credentials"
    safe_config = safe_directory / "config"
    symlink_target = root / "symlink-target"
    symlink_target.mkdir(mode=0o700)
    symlink_directory = root / "symlink-profile-path"
    symlink_directory.symlink_to(symlink_target, target_is_directory=True)
    unsafe_paths = (
        ROOT / "unsafe-aws-credentials",
        Path("relative-aws-credentials"),
        symlink_directory / "credentials",
    )
    names = ("AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE")
    original = {name: os.environ.get(name) for name in names}
    commands: list[list[str]] = []

    def command(args: list[str], *, env=None) -> str:
        commands.append(args)
        raise AssertionError("unsafe profile path reached an AWS command")

    try:
        for unsafe in unsafe_paths:
            os.environ["AWS_SHARED_CREDENTIALS_FILE"] = str(unsafe)
            os.environ["AWS_CONFIG_FILE"] = str(safe_config)
            try:
                run_main_with_stubs(root / f"cleanup-{len(commands)}.json", command)
            except RuntimeError:
                pass
            else:
                raise AssertionError("unsafe shared-credentials path was accepted")
        os.environ["AWS_SHARED_CREDENTIALS_FILE"] = str(safe_credentials)
        os.environ["AWS_CONFIG_FILE"] = str(ROOT / "unsafe-aws-config")
        try:
            run_main_with_stubs(root / "cleanup-config.json", command)
        except RuntimeError:
            pass
        else:
            raise AssertionError("repository AWS config path was accepted")
    finally:
        for name, value in original.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
    if commands:
        raise AssertionError("unsafe AWS profile paths were not rejected before IAM or STS commands")


def assert_unsafe_cleanup_paths_fail_before_aws(root: Path) -> None:
    safe_directory = root / "safe-cleanup-paths"
    safe_directory.mkdir(mode=0o700)
    safe_credentials = safe_directory / "credentials"
    safe_config = safe_directory / "config"
    writable_parent = root / "writable-cleanup-parent"
    writable_parent.mkdir(mode=0o700)
    writable_parent.chmod(0o777)
    symlink_target = root / "cleanup-symlink-target"
    symlink_target.write_text("not-a-record", encoding="utf-8")
    symlink_record = safe_directory / "cleanup-symlink.json"
    symlink_record.symlink_to(symlink_target)
    symlink_parent_target = root / "cleanup-parent-target"
    symlink_parent_target.mkdir(mode=0o700)
    symlink_parent = root / "cleanup-parent-symlink"
    symlink_parent.symlink_to(symlink_parent_target, target_is_directory=True)
    existing_directory = safe_directory / "cleanup-directory"
    existing_directory.mkdir(mode=0o700)
    broad_record = safe_directory / "cleanup-broad.json"
    broad_record.write_text("{}", encoding="utf-8")
    broad_record.chmod(0o644)
    unsafe_paths = (
        ROOT / "unsafe-cleanup-record.json",
        Path("relative-cleanup-record.json"),
        writable_parent / "cleanup.json",
        symlink_record,
        symlink_parent / "cleanup.json",
        existing_directory,
        broad_record,
    )
    names = ("AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE")
    original = {name: os.environ.get(name) for name in names}
    commands: list[list[str]] = []

    def command(args: list[str], *, env=None) -> str:
        commands.append(args)
        raise AssertionError("unsafe cleanup path reached an AWS command")

    try:
        os.environ["AWS_SHARED_CREDENTIALS_FILE"] = str(safe_credentials)
        os.environ["AWS_CONFIG_FILE"] = str(safe_config)
        for unsafe in unsafe_paths:
            try:
                run_main_with_stubs(unsafe, command)
            except RuntimeError:
                pass
            else:
                raise AssertionError(f"unsafe cleanup path was accepted: {unsafe}")
        absent = root / "new-private-cleanup-parent" / "nested" / "cleanup.json"
        if MODULE.validate_cleanup_record_path(absent) != absent:
            raise AssertionError("private absent cleanup path was not accepted")
        if mode(absent.parent) & 0o022:
            raise AssertionError("new cleanup-record parent must be private")
    finally:
        for name, value in original.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
    if commands:
        raise AssertionError("unsafe cleanup paths were not rejected before AWS identity or IAM calls")


def assert_profile_pair_lock_blocks_a_concurrent_process(root: Path) -> None:
    profile_root = root / "profile-lock"
    profile_root.mkdir(mode=0o700)
    credentials_path = profile_root / "credentials"
    config_path = profile_root / "config"
    lock_path = MODULE.profile_pair_lock_path(credentials_path, config_path)
    contender = None
    release = None
    try:
        with MODULE.locked_profile_pair(credentials_path, config_path):
            if mode(lock_path) != 0o600:
                raise AssertionError("profile-pair lock must be created with mode 0600")
            contender, attempted, acquired, release = start_profile_lock_contender(lock_path, root)
            wait_for_path(attempted, "concurrent profile-lock attempt")
            time.sleep(0.1)
            if acquired.exists():
                raise AssertionError("concurrent profile writer acquired the lock before the protected operation ended")
        wait_for_path(acquired, "concurrent profile-lock acquisition after release")
    finally:
        if contender is not None and release is not None:
            finish_lock_contender(contender, release)


def assert_concurrent_profile_update_is_not_overwritten(root: Path) -> None:
    profile_root = root / "concurrent-profile-update"
    profile_root.mkdir(mode=0o700)
    credentials_path = profile_root / "credentials"
    config_path = profile_root / "config"
    original_credentials = b"[smartrouter]\naws_access_key_id = prior-session\n"
    original_config = b"[profile genai-smart-router-eks-discovery]\nregion = us-west-2\n"
    credentials_path.write_bytes(original_credentials)
    config_path.write_bytes(original_config)
    credentials_path.chmod(0o600)
    config_path.chmod(0o600)
    prior = MODULE.profile_files_snapshot(credentials_path, config_path)
    published = MODULE.write_profiles(
        "smartrouter",
        "genai-smart-router-eks-discovery",
        "arn:aws:iam::123456789012:role/genai-smart-router-eks-discovery",
        "us-east-1",
        {
            "aws_access_key_id": "new-test-session",
            "aws_secret_access_key": "new-test-secret",
            "aws_session_token": "new-test-token",
        },
        credentials_path=credentials_path,
        config_path=config_path,
    )
    concurrent_credentials = b"[other-session]\naws_access_key_id = concurrent-session\n"
    credentials_path.write_bytes(concurrent_credentials)
    credentials_path.chmod(0o600)
    try:
        MODULE.restore_profile_files(
            credentials_path,
            config_path,
            prior,
            expected_current=published,
        )
    except RuntimeError as exc:
        if str(exc) != "AWS profile files changed concurrently; refusing to overwrite them during rollback":
            raise
    else:
        raise AssertionError("rollback overwrote a concurrent profile update")
    if credentials_path.read_bytes() != concurrent_credentials:
        raise AssertionError("conditional rollback discarded a concurrent profile update")


def assert_failed_role_verification_restores_profile_files(root: Path) -> None:
    profile_root = root / "post-write-rollback"
    profile_root.mkdir(mode=0o700)

    def command(args: list[str], *, env=None) -> str:
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            profile = args[args.index("--profile") + 1]
            if profile == "admin":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
            if profile == "genai-smart-router-eks-discovery":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/wrong-role/test"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            return json.dumps({"AccessKey": {"AccessKeyId": "AKIAEXAMPLEKEYID", "SecretAccessKey": "test-only-secret"}})
        if args[:3] == ["aws", "sts", "get-session-token"]:
            return json.dumps({"Credentials": {"AccessKeyId": "ASIAEXAMPLEKEYID", "SecretAccessKey": "test-only-session-secret", "SessionToken": "test-only-session-token", "Expiration": "2030-01-01T00:00:00Z"}})
        if args[:3] == ["aws", "iam", "delete-access-key"]:
            return ""
        raise AssertionError(f"unexpected command: {args}")

    def run_failure(credentials_path: Path, config_path: Path, cleanup: Path) -> None:
        names = ("AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE")
        original = {name: os.environ.get(name) for name in names}
        os.environ["AWS_SHARED_CREDENTIALS_FILE"] = str(credentials_path)
        os.environ["AWS_CONFIG_FILE"] = str(config_path)
        try:
            try:
                run_main_with_stubs(cleanup, command, stub_profiles=False)
            except RuntimeError as exc:
                if str(exc) != "configured role profile is not the expected discovery assumed role":
                    raise
            else:
                raise AssertionError("bootstrap accepted a wrong configured discovery role")
        finally:
            for name, value in original.items():
                if value is None:
                    os.environ.pop(name, None)
                else:
                    os.environ[name] = value
        if cleanup.exists():
            raise AssertionError("failed role verification did not preserve source-key cleanup")

    credentials_path = profile_root / "credentials"
    config_path = profile_root / "config"
    original_credentials = b"[smartrouter]\naws_access_key_id = prior-session\n"
    original_config = b"[profile genai-smart-router-eks-discovery]\nregion = us-west-2\n"
    credentials_path.write_bytes(original_credentials)
    config_path.write_bytes(original_config)
    credentials_path.chmod(0o640)
    config_path.chmod(0o600)
    run_failure(credentials_path, config_path, root / "post-write-existing-cleanup.json")
    if credentials_path.read_bytes() != original_credentials or mode(credentials_path) != 0o640:
        raise AssertionError("failed role verification did not restore the prior credentials profile")
    if config_path.read_bytes() != original_config or mode(config_path) != 0o600:
        raise AssertionError("failed role verification did not restore the prior config profile")

    new_profile_root = root / "post-write-remove"
    new_profile_root.mkdir(mode=0o700)
    new_credentials = new_profile_root / "credentials"
    new_config = new_profile_root / "config"
    run_failure(new_credentials, new_config, root / "post-write-new-cleanup.json")
    if new_credentials.exists() or new_config.exists():
        raise AssertionError("failed role verification did not remove newly published AWS profile files")


def assert_cleanup_record_failure_still_restores_profiles(root: Path) -> None:
    profile_root = root / "cleanup-record-profile-rollback"
    profile_root.mkdir(mode=0o700)
    credentials_path = profile_root / "credentials"
    config_path = profile_root / "config"
    original_credentials = b"[smartrouter]\naws_access_key_id = prior-session\n"
    original_config = b"[profile genai-smart-router-eks-discovery]\nregion = us-west-2\n"
    credentials_path.write_bytes(original_credentials)
    config_path.write_bytes(original_config)
    credentials_path.chmod(0o640)
    config_path.chmod(0o600)
    cleanup = root / "cleanup-record-profile-rollback.json"
    names = ("AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE")
    original_environment = {name: os.environ.get(name) for name in names}
    original_remove = MODULE.remove_own_recovery_record

    def command(args: list[str], *, env=None) -> str:
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            profile = args[args.index("--profile") + 1]
            if profile == "admin":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
            if profile == "genai-smart-router-eks-discovery":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/wrong-role/test"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            return json.dumps({"AccessKey": {"AccessKeyId": "AKIAEXAMPLEKEYID", "SecretAccessKey": "test-only-secret"}})
        if args[:3] == ["aws", "sts", "get-session-token"]:
            return json.dumps({"Credentials": {"AccessKeyId": "ASIAEXAMPLEKEYID", "SecretAccessKey": "test-only-session-secret", "SessionToken": "test-only-session-token", "Expiration": "2030-01-01T00:00:00Z"}})
        if args[:3] == ["aws", "iam", "delete-access-key"]:
            return ""
        raise AssertionError(f"unexpected command: {args}")

    os.environ["AWS_SHARED_CREDENTIALS_FILE"] = str(credentials_path)
    os.environ["AWS_CONFIG_FILE"] = str(config_path)
    MODULE.remove_own_recovery_record = lambda *args, **kwargs: (_ for _ in ()).throw(RuntimeError("simulated cleanup record failure"))  # type: ignore[method-assign]
    try:
        try:
            run_main_with_stubs(cleanup, command, stub_profiles=False)
        except RuntimeError as exc:
            if str(exc) != "temporary source key was deleted but its cleanup record could not be removed; preserve it for operator reconciliation":
                raise
        else:
            raise AssertionError("bootstrap accepted a failed cleanup-record removal")
    finally:
        MODULE.remove_own_recovery_record = original_remove  # type: ignore[method-assign]
        for name, value in original_environment.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
    if credentials_path.read_bytes() != original_credentials or mode(credentials_path) != 0o640:
        raise AssertionError("cleanup-record failure did not restore the prior credentials profile")
    if config_path.read_bytes() != original_config or mode(config_path) != 0o600:
        raise AssertionError("cleanup-record failure did not restore the prior config profile")


def assert_unexpected_role_identity_is_rejected(root: Path) -> None:
    cleanup = root / "unexpected-role" / "recovery.json"
    source_key_deleted = False

    def command(args: list[str], *, env=None) -> str:
        nonlocal source_key_deleted
        if args[:3] == ["aws", "sts", "get-caller-identity"]:
            profile = args[args.index("--profile") + 1]
            if profile == "admin":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/smartrouter"})
            if profile == "genai-smart-router-eks-discovery":
                return json.dumps({"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/unexpected-role/test"})
        if args[:3] == ["aws", "iam", "create-access-key"]:
            return json.dumps({"AccessKey": {"AccessKeyId": "AKIAEXAMPLEKEYID", "SecretAccessKey": "test-only-secret"}})
        if args[:3] == ["aws", "sts", "get-session-token"]:
            return json.dumps({"Credentials": {"AccessKeyId": "ASIAEXAMPLEKEYID", "SecretAccessKey": "test-only-session-secret", "SessionToken": "test-only-session-token", "Expiration": "2030-01-01T00:00:00Z"}})
        if args[:3] == ["aws", "iam", "delete-access-key"]:
            source_key_deleted = True
            return ""
        raise AssertionError(f"unexpected command: {args}")

    try:
        run_main_with_stubs(cleanup, command)
    except RuntimeError as exc:
        if str(exc) != "configured role profile is not the expected discovery assumed role":
            raise
    else:
        raise AssertionError("bootstrap accepted an unexpected assumed role")
    if not source_key_deleted or cleanup.exists():
        raise AssertionError("unexpected role identity did not preserve temporary-key cleanup guarantees")
    for identity in (
        {"Account": "000000000000", "Arn": "arn:aws:sts::000000000000:assumed-role/genai-smart-router-eks-discovery/test"},
        {"Account": "123456789012", "Arn": "arn:aws:sts::123456789012:assumed-role/unexpected-role/test"},
    ):
        try:
            MODULE.validate_discovery_role_identity(identity, "123456789012")
        except RuntimeError:
            pass
        else:
            raise AssertionError("role identity validation accepted a wrong account or role")


def main() -> int:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        config = configparser.RawConfigParser()
        config["session"] = {"aws_access_key_id": "test-id"}
        credentials = root / "credentials"
        MODULE.atomic_write_config(credentials, config)
        if mode(credentials) != 0o600:
            raise AssertionError("credential profile must be atomically created with mode 0600")
        if ".credentials." in "".join(path.name for path in root.iterdir()):
            raise AssertionError("atomic credential temporary file was not removed")

        reservation_path = root / "cleanup" / "source-key-reservation.json"
        reservation = MODULE.reserve_recovery_record(reservation_path, "smartrouter")
        if mode(reservation_path) != 0o600:
            raise AssertionError("pre-create recovery reservation must be atomically created with mode 0600")
        reservation_payload = reservation_path.read_text(encoding="utf-8")
        status = MODULE.recovery_status(reservation_path)
        if status["state"] != MODULE.RESERVED_BEFORE_CREATE or "access_key_id" in status:
            raise AssertionError("pre-create reservation must be actionable without pretending a key ID is known")
        try:
            MODULE.reserve_recovery_record(reservation_path, "smartrouter")
        except RuntimeError:
            pass
        else:
            raise AssertionError("a pre-create reservation must block a concurrent/retry IAM mutation")
        if reservation_path.read_text(encoding="utf-8") != reservation_payload:
            raise AssertionError("a pre-create reservation was overwritten by a concurrent/retry attempt")

        cleanup = root / "cleanup" / "source-key.json"
        key_reservation = MODULE.reserve_recovery_record(cleanup, "smartrouter")
        try:
            wrong_reservation = dict(key_reservation)
            wrong_reservation["reservation_id"] = "0" * 32
            MODULE.promote_recovery_reservation(cleanup, wrong_reservation, "smartrouter", "AKIASECONDKEYID")
        except RuntimeError:
            pass
        else:
            raise AssertionError("a different invocation must not promote this reservation")
        if MODULE.recovery_status(cleanup)["state"] != MODULE.RESERVED_BEFORE_CREATE:
            raise AssertionError("a mismatched promotion changed the recovery reservation")
        MODULE.promote_recovery_reservation(cleanup, key_reservation, "smartrouter", "AKIAEXAMPLEKEYID")
        key_status = MODULE.recovery_status(cleanup)
        if key_status["state"] != MODULE.ACCESS_KEY_CREATED or key_status.get("access_key_id") != "AKIAEXAMPLEKEYID":
            raise AssertionError("known source key must promote the reservation to its exact recovery record")
        payload = cleanup.read_text(encoding="utf-8")
        if "SecretAccessKey" in payload:
            raise AssertionError("recovery record must never contain an access-key secret")
        MODULE.remove_own_recovery_record(cleanup, key_reservation, "smartrouter", "AKIAEXAMPLEKEYID")
        if cleanup.exists():
            raise AssertionError("confirmed source-key deletion must remove only its own cleanup record")
        assert_precreate_reservation_order(root)
        assert_ambiguous_create_preserves_reservation(root)
        assert_concurrent_reservation_is_exclusive(root)
        assert_configured_profile_paths_and_role_verification(root)
        assert_profile_write_rolls_back_credentials_when_config_write_fails(root)
        assert_unsafe_profile_paths_fail_before_iam(root)
        assert_unsafe_cleanup_paths_fail_before_aws(root)
        assert_profile_pair_lock_blocks_a_concurrent_process(root)
        assert_concurrent_profile_update_is_not_overwritten(root)
        assert_failed_role_verification_restores_profile_files(root)
        assert_cleanup_record_failure_still_restores_profiles(root)
        assert_unexpected_role_identity_is_rejected(root)
    print("EKS session bootstrap safety tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
