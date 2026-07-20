#!/usr/bin/env python3
"""Regression tests for local EKS session file safety helpers."""

from __future__ import annotations

import configparser
import importlib.util
import json
import stat
import sys
import tempfile
import threading
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("bootstrap_eks_session", ROOT / "scripts/bootstrap_eks_session.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load bootstrap session helpers")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def mode(path: Path) -> int:
    return stat.S_IMODE(path.stat().st_mode)


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


def run_main_with_stubs(cleanup: Path, command) -> int:
    original_argv = sys.argv
    original_command = MODULE.command
    original_mfa = MODULE.mfa_code_from_keychain
    original_profiles = MODULE.write_profiles
    sys.argv = bootstrap_argv(cleanup)
    MODULE.command = command
    MODULE.mfa_code_from_keychain = lambda service, account: "123456"
    MODULE.write_profiles = lambda profile, role_profile, role_arn, region, credentials: None
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
                return json.dumps({"Arn": "arn:aws:sts::123456789012:assumed-role/genai-smart-router-eks-discovery/test"})
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
    print("EKS session bootstrap safety tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
