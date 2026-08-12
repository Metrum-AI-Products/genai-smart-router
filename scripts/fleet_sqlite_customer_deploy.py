#!/usr/bin/env python3
"""Repeatable non-production Fleet SQLite customer deploy.

Operator-only. Not a customer CLI. Does not package into Docker images.

Default path omits database_profile so metrum-fleetctl uses SQLite on the
owned PVC (no RDS, no --rds-admission-file). Production-identical runtime
bundles are rewritten at bind time to sqlite + auto-safe.

Example:
  rtk python3 scripts/fleet_sqlite_customer_deploy.py --customer-id acme3
  rtk python3 scripts/fleet_sqlite_customer_deploy.py --customer-id acme4 --delete-first
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
DEFAULT_PROFILE_REF = "aws-ssm:///metrum/smartrouter/profiles/staging"
DEFAULT_RUNTIME_BUNDLE = (
    "aws-secretsmanager:///smartrouter/fleet/production-identical/runtime-bundle"
)
DEFAULT_LICENSE_REF = "aws-ssm:///metrum/smartrouter/fleet/acme-rehearsal/license-request"
CUSTOMER_ID_RE = re.compile(r"^[a-z][a-z0-9-]{1,30}$")


def die(msg: str, code: int = 1) -> None:
    print(msg, file=sys.stderr)
    raise SystemExit(code)


def run(cmd: list[str], *, cwd: Path | None = None, env: dict | None = None) -> subprocess.CompletedProcess:
    print("+", " ".join(cmd), flush=True)
    return subprocess.run(cmd, cwd=cwd or REPO, env=env, check=False, text=True, capture_output=True)


def require_ok(proc: subprocess.CompletedProcess, what: str) -> None:
    if proc.returncode != 0:
        sys.stderr.write(proc.stdout or "")
        sys.stderr.write(proc.stderr or "")
        die(f"{what} failed (exit {proc.returncode})")


def fleet_home(customer_id: str) -> Path:
    return Path.home() / ".local/share/metrum-fleet" / customer_id


def ensure_keys(home: Path) -> tuple[Path, Path]:
    priv = home / "lifecycle_approval_private_key.b64"
    pub = home / "lifecycle_approval_public_key.b64"
    if priv.is_file() and pub.is_file():
        return priv, pub
    # Reuse disposable-e2e keys when present so profile verification matches.
    for donor_name in ("disposable-e2e", "acme-rehearsal", "acme2"):
        donor = Path.home() / ".local/share/metrum-fleet" / donor_name
        dpriv, dpub = donor / "lifecycle_approval_private_key.b64", donor / "lifecycle_approval_public_key.b64"
        if dpriv.is_file() and dpub.is_file():
            home.mkdir(parents=True, exist_ok=True)
            home.chmod(0o700)
            shutil.copy2(dpriv, priv)
            shutil.copy2(dpub, pub)
            priv.chmod(0o600)
            pub.chmod(0o600)
            return priv, pub
    die("missing lifecycle approval keys under ~/.local/share/metrum-fleet/")


def build_fleetctl(bin_dir: Path) -> Path:
    bin_dir.mkdir(parents=True, exist_ok=True)
    out = bin_dir / "metrum-fleetctl"
    proc = run(["go", "build", "-o", str(out), "./cmd/metrum-fleetctl"])
    require_ok(proc, "go build metrum-fleetctl")
    return out


def assume_role(home: Path, customer_id: str) -> dict[str, str]:
    role = os.environ.get(
        "METRUM_FLEET_LIFECYCLE_ROLE_ARN",
        "arn:aws:iam::121701826775:role/genai-smart-router-eks-fleet-lifecycle",
    )
    region = os.environ.get("AWS_REGION", "us-east-1")
    env = os.environ.copy()
    # Prefer default profile for assume-role; clear stale session env.
    for k in ("AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"):
        env.pop(k, None)
    env["AWS_PROFILE"] = env.get("AWS_PROFILE", "default")
    env["AWS_REGION"] = region
    proc = run(
        [
            "aws",
            "sts",
            "assume-role",
            "--role-arn",
            role,
            "--role-session-name",
            f"metrum-fleetctl-{customer_id}",
            "--duration-seconds",
            "3600",
            "--output",
            "json",
        ],
        env=env,
    )
    require_ok(proc, "sts assume-role")
    creds = json.loads(proc.stdout)["Credentials"]
    session = home / "aws-session.env"
    session.write_text(
        "\n".join(
            [
                f"export AWS_ACCESS_KEY_ID={creds['AccessKeyId']}",
                f"export AWS_SECRET_ACCESS_KEY={creds['SecretAccessKey']}",
                f"export AWS_SESSION_TOKEN={creds['SessionToken']}",
                f"export AWS_REGION={region}",
                "unset AWS_PROFILE",
                "",
            ]
        ),
        encoding="utf-8",
    )
    session.chmod(0o600)
    out_env = os.environ.copy()
    out_env["AWS_ACCESS_KEY_ID"] = creds["AccessKeyId"]
    out_env["AWS_SECRET_ACCESS_KEY"] = creds["SecretAccessKey"]
    out_env["AWS_SESSION_TOKEN"] = creds["SessionToken"]
    out_env["AWS_REGION"] = region
    out_env.pop("AWS_PROFILE", None)
    return out_env


def write_manifest(home: Path, customer_id: str, runtime_bundle: str, license_ref: str) -> Path:
    stamp = time.strftime("%Y%m%dt%H%M%Sz", time.gmtime())
    manifest = {
        "api_version": "metrum.ai/smartrouter-deployment/v1",
        "customer_id": customer_id,
        "stage": "nonproduction",
        "release": "latest-approved",
        "resource_profile": "small",
        "state_profile": "sqlite-rwo-small",
        # Intentionally omit database_profile → SQLite default (no RDS).
        "runtime_bundle_ref": runtime_bundle,
        "config_revision": f"{customer_id}-sqlite-{stamp}",
        "license": {"request_ref": license_ref, "validity": "168h"},
    }
    path = home / "manifest.json"
    path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    path.chmod(0o600)
    return path


def sign_intent(priv: Path, intent_id: str, profile_ref: str, manifest: Path, out: Path) -> None:
    proc = run(
        [
            "go",
            "run",
            "scripts/fleet_sign_docs.go",
            "intent",
            str(priv),
            intent_id,
            profile_ref,
            str(manifest),
            str(out),
            "12",
        ]
    )
    require_ok(proc, "sign intent")
    out.chmod(0o600)


def sign_admission(
    priv: Path,
    approval_id: str,
    profile_id: str,
    environment: str,
    database_profile: str,
    job_id: str,
    namespace: str,
    manifest_sha256: str,
    out: Path,
) -> None:
    proc = run(
        [
            "go",
            "run",
            "scripts/fleet_sign_docs.go",
            "admission",
            str(priv),
            approval_id,
            profile_id,
            environment,
            database_profile,
            job_id,
            namespace,
            manifest_sha256,
            str(out),
        ]
    )
    require_ok(proc, "sign RDS admission")
    out.chmod(0o600)


def sign_delete(priv: Path, job_id: str, out: Path, retain_database: bool, retain_pvc: bool) -> None:
    nonce = f"delete-{job_id}-{int(time.time())}"
    proc = run(
        [
            "go",
            "run",
            "scripts/fleet_sign_docs.go",
            "delete",
            str(priv),
            job_id,
            nonce,
            str(out),
            "true" if retain_database else "false",
            "true" if retain_pvc else "false",
        ]
    )
    require_ok(proc, "sign delete approval")
    out.chmod(0o600)


def fleetctl_json(bin_path: Path, args: list[str], env: dict[str, str]) -> dict:
    proc = run([str(bin_path), *args], env=env)
    # fleetctl may print JSON then die; try parse stdout anyway
    text = (proc.stdout or "").strip()
    if not text:
        sys.stderr.write(proc.stderr or "")
        die(f"fleetctl {' '.join(args)} produced no JSON (exit {proc.returncode})")
    try:
        payload = json.loads(text)
    except json.JSONDecodeError:
        sys.stderr.write(proc.stdout or "")
        sys.stderr.write(proc.stderr or "")
        die(f"fleetctl {' '.join(args)} returned non-JSON")
    if proc.returncode != 0:
        print(json.dumps({k: payload.get(k) for k in ("job_id", "state", "error_class", "observed_state") if k in payload}, indent=2))
        die(f"fleetctl {' '.join(args)} failed (exit {proc.returncode})")
    return payload


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--customer-id", required=True, help="deployment-defined id; hostname becomes {id}.apps.metrum.ai")
    ap.add_argument("--profile-ref", default=DEFAULT_PROFILE_REF)
    ap.add_argument("--runtime-bundle-ref", default=DEFAULT_RUNTIME_BUNDLE)
    ap.add_argument("--license-ref", default=DEFAULT_LICENSE_REF)
    ap.add_argument("--delete-first", action="store_true", help="delete prior job for this workspace if present")
    ap.add_argument("--retain-pvc", action="store_true", help="when deleting, retain the state PVC")
    ap.add_argument("--skip-build", action="store_true")
    args = ap.parse_args()

    customer_id = args.customer_id.strip().lower()
    if not CUSTOMER_ID_RE.match(customer_id):
        die("customer-id must match ^[a-z][a-z0-9-]{1,30}$")

    home = fleet_home(customer_id)
    home.mkdir(parents=True, exist_ok=True)
    home.chmod(0o700)
    priv, _pub = ensure_keys(home)
    bin_dir = home / "bin"
    fleet = build_fleetctl(bin_dir) if not args.skip_build else bin_dir / "metrum-fleetctl"
    if not fleet.is_file():
        die(f"missing fleetctl binary at {fleet}")

    env = assume_role(home, customer_id)
    registry = str(home / "registry" / "tenant-deployments.sqlite")
    (home / "registry").mkdir(parents=True, exist_ok=True)

    if args.delete_first:
        prior_intent = home / "intent.json"
        prior_plan = home / "plan.json"
        if prior_intent.is_file() and prior_plan.is_file():
            job_id = json.loads(prior_plan.read_text())["job_id"]
            delete_path = home / "delete-approval.json"
            # SQLite jobs need no RDS admission; retain_database is irrelevant.
            sign_delete(
                priv,
                job_id,
                delete_path,
                retain_database=False,
                retain_pvc=args.retain_pvc,
            )
            print(f"deleting prior job {job_id} ...", flush=True)
            # Dedicated-RDS prior jobs need admission; detect from plan.
            prior = json.loads(prior_plan.read_text())
            del_args = [
                "delete",
                "--intent",
                str(prior_intent),
                "--registry",
                registry,
                "--confirm-file",
                str(delete_path),
                "--output",
                "json",
            ]
            if prior.get("database_id"):
                admission = home / "rds-admission.json"
                sign_admission(
                    priv,
                    approval_id=f"adm-{customer_id}-{time.strftime('%Y%m%dt%H%M%Sz', time.gmtime())}",
                    profile_id=prior.get("profile_id") or "staging-fleet-nonprod",
                    environment=prior.get("environment") or "nonproduction",
                    database_profile=prior["database_profile"],
                    job_id=job_id,
                    namespace=prior["namespace"],
                    manifest_sha256=prior["manifest_sha256"],
                    out=admission,
                )
                del_args.extend(["--rds-admission-file", str(admission)])
            try:
                fleetctl_json(fleet, del_args, env)
            except SystemExit:
                print("delete reported failure; continuing only if namespace is gone is operator-owned", flush=True)

    manifest = write_manifest(home, customer_id, args.runtime_bundle_ref, args.license_ref)
    intent_id = f"intent-{customer_id}-{time.strftime('%Y%m%dt%H%M%Sz', time.gmtime())}"
    (home / "intent-id.txt").write_text(intent_id + "\n", encoding="utf-8")
    intent_path = home / "intent.json"
    sign_intent(priv, intent_id, args.profile_ref, manifest, intent_path)

    plan = fleetctl_json(fleet, ["plan", "--intent", str(intent_path), "--output", "json"], env)
    if plan.get("database_id") or plan.get("database_profile") or "dedicated_rds" in (plan.get("actions") or []):
        die("refusing deploy: plan selected dedicated RDS; omit database_profile for SQLite")
    (home / "plan.json").write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "customer_id": customer_id,
                "hostname": plan.get("hostname"),
                "namespace": plan.get("namespace"),
                "job_id": plan.get("job_id"),
                "actions": plan.get("actions"),
                "database": "sqlite",
            },
            indent=2,
        ),
        flush=True,
    )

    status = fleetctl_json(
        fleet,
        ["deploy", "--intent", str(intent_path), "--registry", registry, "--output", "json"],
        env,
    )
    safe = {k: status.get(k) for k in ("job_id", "state", "error_class", "observed_state", "hostname", "namespace") if k in status or True}
    # hostname/namespace live on plan, not always status
    safe["hostname"] = plan.get("hostname")
    safe["namespace"] = plan.get("namespace")
    print(json.dumps(safe, indent=2), flush=True)
    if status.get("state") != "ready":
        die(f"deploy ended with state={status.get('state')} error_class={status.get('error_class')}")
    print(
        f"SQLite deploy ready: https://{plan.get('hostname')}/readyz "
        f"(job_id={status.get('job_id') or plan.get('job_id')})",
        flush=True,
    )


if __name__ == "__main__":
    main()
