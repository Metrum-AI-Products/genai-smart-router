#!/usr/bin/env python3
"""Single-command Fleet SQLite customer lifecycle (operator-only).

Not a customer CLI. Not packaged into Docker images.
Fleet CLIs (metrum-fleetctl, router-token-gen, metrum-fleet-sign) must come from a
release binary package or METRUM_FLEET_BIN_DIR — never go build/go run on operator hosts.

Commands:
  create        SQLite deploy from production-identical upstream bundle
  status        exact-job Fleet status + observed state
  smoke         public readyz + authenticated /v1/models (+ optional chat)
  grant-caller  generate caller, publish per-customer bundle, redeploy, smoke
  update-config merge YAML patch into runtime bundle, publish, redeploy, smoke
  delete        signed Fleet delete for the workspace job

Examples:
  rtk python3 scripts/fleet_customer_lifecycle.py create --customer-id acme4
  rtk python3 scripts/fleet_customer_lifecycle.py smoke --customer-id acme4
  rtk python3 scripts/fleet_customer_lifecycle.py grant-caller --customer-id acme4 \\
    --owner-user acme-admin --project acme --allow high --token-out ~/.local/share/metrum-fleet/acme4/CALLER_TOKEN_ADMIN.txt
  rtk python3 scripts/fleet_customer_lifecycle.py update-config --customer-id acme4 --patch-file /path/patch.yaml
  rtk python3 scripts/fleet_customer_lifecycle.py delete --customer-id acme4
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

import yaml

REPO = Path(__file__).resolve().parents[1]
DEFAULT_PROFILE_REF = "aws-ssm:///metrum/smartrouter/profiles/staging"
DEFAULT_RUNTIME_BUNDLE = (
    "aws-secretsmanager:///smartrouter/fleet/production-identical/runtime-bundle"
)
DEFAULT_LICENSE_REF = "aws-ssm:///metrum/smartrouter/fleet/acme-rehearsal/license-request"
CUSTOMER_ID_RE = re.compile(r"^[a-z][a-z0-9-]{1,30}$")
HOSTNAME_SUFFIX = "apps.metrum.ai"


def die(msg: str, code: int = 1) -> None:
    print(msg, file=sys.stderr)
    raise SystemExit(code)


def run(
    cmd: list[str],
    *,
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    quiet: bool = False,
) -> subprocess.CompletedProcess:
    if not quiet:
        print("+", " ".join(cmd), flush=True)
    return subprocess.run(cmd, cwd=cwd or REPO, env=env, check=False, text=True, capture_output=True)


def require_ok(proc: subprocess.CompletedProcess, what: str) -> None:
    if proc.returncode != 0:
        sys.stderr.write(proc.stdout or "")
        sys.stderr.write(proc.stderr or "")
        die(f"{what} failed (exit {proc.returncode})")


def fleet_home(customer_id: str) -> Path:
    return Path.home() / ".local/share/metrum-fleet" / customer_id


def state_path(home: Path) -> Path:
    return home / "lifecycle-state.json"


def load_state(home: Path) -> dict[str, Any]:
    path = state_path(home)
    if not path.is_file():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def save_state(home: Path, **updates: Any) -> dict[str, Any]:
    state = load_state(home)
    state.update({k: v for k, v in updates.items() if v is not None})
    path = state_path(home)
    path.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")
    path.chmod(0o600)
    return state


def ensure_keys(home: Path) -> tuple[Path, Path]:
    priv = home / "lifecycle_approval_private_key.b64"
    pub = home / "lifecycle_approval_public_key.b64"
    if priv.is_file() and pub.is_file():
        return priv, pub
    for donor_name in ("disposable-e2e", "acme-rehearsal", "acme2", "acme3"):
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


def _copy_or_link_binary(src: Path, dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() or dest.is_symlink():
        dest.unlink()
    shutil.copy2(src, dest)
    dest.chmod(0o755)


def resolve_packaged_binary(name: str, bin_dir: Path) -> Path | None:
    """Locate a packaged CLI binary. Never returns a source path."""
    candidates: list[Path] = []
    env_dir = os.environ.get("METRUM_FLEET_BIN_DIR", "").strip()
    if env_dir:
        candidates.append(Path(env_dir).expanduser() / name)
    candidates.append(bin_dir / name)
    which = shutil.which(name)
    if which:
        candidates.append(Path(which))
    for path in candidates:
        if path.is_file() and os.access(path, os.X_OK):
            return path
    return None


def resolve_operator_binaries(bin_dir: Path, *, build_from_source: bool) -> tuple[Path, Path, Path]:
    """
    Fleet operator CLIs are distributed as binaries only.

    Default: use METRUM_FLEET_BIN_DIR, workspace bin/, or PATH packaged binaries.
    --build-from-source is a developer escape hatch that still writes binaries
    into bin_dir and must never be required on customer or operator hosts.
    """
    bin_dir.mkdir(parents=True, exist_ok=True)
    names = ("metrum-fleetctl", "router-token-gen", "metrum-fleet-sign")
    if build_from_source:
        require_ok(run(["go", "build", "-o", str(bin_dir / "metrum-fleetctl"), "./cmd/metrum-fleetctl"]), "build metrum-fleetctl")
        require_ok(run(["go", "build", "-o", str(bin_dir / "router-token-gen"), "./cmd/router-token-gen"]), "build router-token-gen")
        require_ok(run(["go", "build", "-o", str(bin_dir / "metrum-fleet-sign"), "./cmd/metrum-fleet-sign"]), "build metrum-fleet-sign")
        return bin_dir / "metrum-fleetctl", bin_dir / "router-token-gen", bin_dir / "metrum-fleet-sign"

    resolved: dict[str, Path] = {}
    missing: list[str] = []
    for name in names:
        found = resolve_packaged_binary(name, bin_dir)
        if found is None:
            missing.append(name)
            continue
        dest = bin_dir / name
        if found.resolve() != dest.resolve():
            _copy_or_link_binary(found, dest)
        resolved[name] = dest
    if missing:
        die(
            "missing packaged Fleet binaries: "
            + ", ".join(missing)
            + ". Install from a release binary package (bin/), set METRUM_FLEET_BIN_DIR, "
            + "or pass --build-from-source only on a trusted development checkout."
        )
    return resolved["metrum-fleetctl"], resolved["router-token-gen"], resolved["metrum-fleet-sign"]


def operator_env() -> dict[str, str]:
    """Default IAM profile: Secrets Manager create/put for per-customer bundles."""
    env = os.environ.copy()
    for k in ("AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"):
        env.pop(k, None)
    env["AWS_PROFILE"] = env.get("AWS_PROFILE", "default")
    env["AWS_REGION"] = env.get("AWS_REGION", "us-east-1")
    return env


def assume_fleet_role(home: Path, customer_id: str) -> dict[str, str]:
    role = os.environ.get(
        "METRUM_FLEET_LIFECYCLE_ROLE_ARN",
        "arn:aws:iam::121701826775:role/genai-smart-router-eks-fleet-lifecycle",
    )
    region = os.environ.get("AWS_REGION", "us-east-1")
    base = operator_env()
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
        env=base,
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
    out = os.environ.copy()
    out["AWS_ACCESS_KEY_ID"] = creds["AccessKeyId"]
    out["AWS_SECRET_ACCESS_KEY"] = creds["SecretAccessKey"]
    out["AWS_SESSION_TOKEN"] = creds["SessionToken"]
    out["AWS_REGION"] = region
    out.pop("AWS_PROFILE", None)
    return out


def sm_name_from_ref(ref: str) -> str:
    prefix = "aws-secretsmanager:///"
    if not ref.startswith(prefix):
        die(f"runtime bundle ref must be {prefix}...")
    return ref[len(prefix) :]


def customer_bundle_ref(customer_id: str) -> str:
    return f"aws-secretsmanager:///smartrouter/fleet/customers/{customer_id}/runtime-bundle"


def fetch_runtime_bundle(ref: str, env: dict[str, str]) -> dict[str, str]:
    name = sm_name_from_ref(ref)
    proc = run(
        [
            "aws",
            "secretsmanager",
            "get-secret-value",
            "--secret-id",
            name,
            "--query",
            "SecretString",
            "--output",
            "text",
        ],
        env=env,
        quiet=True,
    )
    require_ok(proc, f"get-secret-value {name}")
    bundle = json.loads(proc.stdout)
    if not isinstance(bundle, dict) or "config.yaml" not in bundle or "env.json" not in bundle:
        die("invalid runtime bundle shape")
    if set(bundle.keys()) != {"config.yaml", "env.json"}:
        die("runtime bundle must contain exactly config.yaml and env.json")
    return {"config.yaml": bundle["config.yaml"], "env.json": bundle["env.json"]}


def publish_runtime_bundle(customer_id: str, bundle: dict[str, str], env: dict[str, str]) -> str:
    name = sm_name_from_ref(customer_bundle_ref(customer_id))
    payload = json.dumps({"config.yaml": bundle["config.yaml"], "env.json": bundle["env.json"]})
    # Validate shapes without logging contents.
    yaml.safe_load(bundle["config.yaml"])
    json.loads(bundle["env.json"])
    describe = run(
        ["aws", "secretsmanager", "describe-secret", "--secret-id", name, "--query", "Name", "--output", "text"],
        env=env,
        quiet=True,
    )
    if describe.returncode != 0:
        proc = run(
            [
                "aws",
                "secretsmanager",
                "create-secret",
                "--name",
                name,
                "--secret-string",
                payload,
                "--query",
                "Name",
                "--output",
                "text",
            ],
            env=env,
            quiet=True,
        )
        if proc.returncode != 0:
            sys.stderr.write(proc.stderr or "")
            die(
                "CreateSecret denied for per-customer runtime bundle. "
                "Operator IAM must allow secretsmanager:CreateSecret/PutSecretValue "
                f"on {name} (Fleet lifecycle role is read-only for deploy)."
            )
    else:
        proc = run(
            [
                "aws",
                "secretsmanager",
                "put-secret-value",
                "--secret-id",
                name,
                "--secret-string",
                payload,
                "--query",
                "Name",
                "--output",
                "text",
            ],
            env=env,
            quiet=True,
        )
        if proc.returncode != 0:
            sys.stderr.write(proc.stderr or "")
            die(f"PutSecretValue failed for {name}")
    ref = customer_bundle_ref(customer_id)
    print(json.dumps({"published_runtime_bundle_ref": ref, "config_sha256": hashlib.sha256(bundle["config.yaml"].encode()).hexdigest()[:16]}), flush=True)
    return ref


def deep_merge(base: Any, patch: Any) -> Any:
    if isinstance(base, dict) and isinstance(patch, dict):
        out = dict(base)
        for key, value in patch.items():
            if key == "callers" and isinstance(value, list) and isinstance(out.get("callers"), list):
                out["callers"] = merge_callers(out["callers"], value)
            elif key in out:
                out[key] = deep_merge(out[key], value)
            else:
                out[key] = value
        return out
    return patch


def merge_callers(existing: list[Any], patch: list[Any]) -> list[Any]:
    by_id: dict[str, dict[str, Any]] = {}
    order: list[str] = []
    for row in existing:
        if not isinstance(row, dict):
            continue
        cid = str(row.get("id") or "")
        if not cid:
            continue
        by_id[cid] = dict(row)
        order.append(cid)
    for row in patch:
        if not isinstance(row, dict):
            continue
        cid = str(row.get("id") or "")
        if not cid:
            die("caller patch entries require id")
        if cid in by_id:
            by_id[cid] = deep_merge(by_id[cid], row)
        else:
            by_id[cid] = dict(row)
            order.append(cid)
    return [by_id[cid] for cid in order]


def apply_config_patch(config_yaml: str, patch: dict[str, Any]) -> str:
    cfg = yaml.safe_load(config_yaml)
    if not isinstance(cfg, dict):
        die("config.yaml must be a mapping")
    merged = deep_merge(cfg, patch)
    return yaml.safe_dump(merged, sort_keys=False)


def write_manifest(home: Path, customer_id: str, runtime_bundle: str, license_ref: str, revision_prefix: str) -> Path:
    stamp = time.strftime("%Y%m%dt%H%M%Sz", time.gmtime())
    manifest = {
        "api_version": "metrum.ai/smartrouter-deployment/v1",
        "customer_id": customer_id,
        "stage": "nonproduction",
        "release": "latest-approved",
        "resource_profile": "small",
        "state_profile": "sqlite-rwo-small",
        "runtime_bundle_ref": runtime_bundle,
        "config_revision": f"{customer_id}-{revision_prefix}-{stamp}",
        "license": {"request_ref": license_ref, "validity": "168h"},
    }
    path = home / "manifest.json"
    path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    path.chmod(0o600)
    return path


def sign_intent(sign_bin: Path, priv: Path, intent_id: str, profile_ref: str, manifest: Path, out: Path) -> None:
    proc = run(
        [
            str(sign_bin),
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


def sign_delete(sign_bin: Path, priv: Path, job_id: str, out: Path, retain_pvc: bool) -> None:
    nonce = f"delete-{job_id}-{int(time.time())}"
    proc = run(
        [
            str(sign_bin),
            "delete",
            str(priv),
            job_id,
            nonce,
            str(out),
            "false",
            "true" if retain_pvc else "false",
        ]
    )
    require_ok(proc, "sign delete approval")
    out.chmod(0o600)


def sign_admission(
    sign_bin: Path,
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
            str(sign_bin),
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


def fleetctl_json(bin_path: Path, args: list[str], env: dict[str, str], *, allow_fail: bool = False) -> dict[str, Any]:
    proc = run([str(bin_path), *args], env=env)
    text = (proc.stdout or "").strip()
    err = (proc.stderr or "").strip()
    if not text:
        sys.stderr.write(err + "\n")
        if allow_fail:
            return {"_exit": proc.returncode, "_error": err or "no_json"}
        die(f"fleetctl {' '.join(args)} produced no JSON (exit {proc.returncode})")
    try:
        payload = json.loads(text)
    except json.JSONDecodeError:
        sys.stderr.write(proc.stdout or "")
        sys.stderr.write(err + "\n")
        if allow_fail:
            return {"_exit": proc.returncode, "_error": err or "bad_json"}
        die(f"fleetctl {' '.join(args)} returned non-JSON")
    if proc.returncode != 0 and not allow_fail:
        print(json.dumps({k: payload.get(k) for k in ("job_id", "state", "error_class", "observed_state") if k in payload}, indent=2))
        die(f"fleetctl {' '.join(args)} failed (exit {proc.returncode})")
    if proc.returncode != 0:
        payload["_exit"] = proc.returncode
        payload["_error"] = err
    return payload


def registry_path(home: Path) -> str:
    (home / "registry").mkdir(parents=True, exist_ok=True)
    return str(home / "registry" / "tenant-deployments.sqlite")


def current_bundle_ref(home: Path, default_ref: str) -> str:
    state = load_state(home)
    return str(state.get("runtime_bundle_ref") or default_ref)


def plan_and_deploy(
    *,
    home: Path,
    fleet: Path,
    sign_bin: Path,
    priv: Path,
    customer_id: str,
    profile_ref: str,
    runtime_bundle: str,
    license_ref: str,
    fleet_env: dict[str, str],
    revision_prefix: str,
) -> dict[str, Any]:
    manifest = write_manifest(home, customer_id, runtime_bundle, license_ref, revision_prefix)
    intent_id = f"intent-{customer_id}-{time.strftime('%Y%m%dt%H%M%Sz', time.gmtime())}"
    (home / "intent-id.txt").write_text(intent_id + "\n", encoding="utf-8")
    intent_path = home / "intent.json"
    sign_intent(sign_bin, priv, intent_id, profile_ref, manifest, intent_path)
    plan = fleetctl_json(fleet, ["plan", "--intent", str(intent_path), "--output", "json"], fleet_env)
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
                "runtime_bundle_ref": runtime_bundle,
            },
            indent=2,
        ),
        flush=True,
    )
    status = fleetctl_json(
        fleet,
        ["deploy", "--intent", str(intent_path), "--registry", registry_path(home), "--output", "json"],
        fleet_env,
    )
    if status.get("state") != "ready":
        die(f"deploy ended with state={status.get('state')} error_class={status.get('error_class')}")
    save_state(
        home,
        customer_id=customer_id,
        hostname=plan.get("hostname"),
        namespace=plan.get("namespace"),
        job_id=status.get("job_id") or plan.get("job_id"),
        runtime_bundle_ref=runtime_bundle,
        profile_ref=profile_ref,
        license_ref=license_ref,
        intent_id=intent_id,
    )
    return {"plan": plan, "status": status}


def cmd_create(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    priv, _ = ensure_keys(home)
    fleet, _, sign_bin = resolve_operator_binaries(home / "bin", build_from_source=args.build_from_source)
    fleet_env = assume_fleet_role(home, customer_id)
    runtime_ref = args.runtime_bundle_ref
    result = plan_and_deploy(
        home=home,
        fleet=fleet,
        sign_bin=sign_bin,
        priv=priv,
        customer_id=customer_id,
        profile_ref=args.profile_ref,
        runtime_bundle=runtime_ref,
        license_ref=args.license_ref,
        fleet_env=fleet_env,
        revision_prefix="sqlite",
    )
    hostname = result["plan"]["hostname"]
    print(f"SQLite create ready: https://{hostname}/readyz", flush=True)


def cmd_status(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    fleet, _, sign_bin = resolve_operator_binaries(home / "bin", build_from_source=args.build_from_source)
    fleet_env = assume_fleet_role(home, customer_id)
    state = load_state(home)
    plan = json.loads((home / "plan.json").read_text(encoding="utf-8")) if (home / "plan.json").is_file() else {}
    job_id = state.get("job_id") or plan.get("job_id")
    profile_ref = args.profile_ref or state.get("profile_ref") or DEFAULT_PROFILE_REF
    if not job_id:
        die("no job_id in lifecycle state or plan.json; run create first")
    status = fleetctl_json(
        fleet,
        [
            "status",
            "--profile-ref",
            profile_ref,
            "--job",
            str(job_id),
            "--registry",
            registry_path(home),
            "--output",
            "json",
        ],
        fleet_env,
    )
    safe = {k: status.get(k) for k in ("job_id", "state", "hostname", "namespace", "observed_state", "error_class", "config_revision")}
    print(json.dumps(safe, indent=2), flush=True)


def http_json(url: str, *, token: str | None = None, body: dict | None = None, timeout: int = 60) -> tuple[int, Any]:
    headers = {"Accept": "application/json"}
    data = None
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers=headers, method="POST" if body is not None else "GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        try:
            parsed = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            parsed = {"_raw_len": len(raw)}
        return exc.code, parsed
    except Exception as exc:  # noqa: BLE001
        return 0, {"error": str(exc)}


def resolve_smoke_token(home: Path, token_file: str | None) -> str:
    candidates: list[Path] = []
    if token_file:
        candidates.append(Path(token_file).expanduser())
    candidates.extend(
        [
            home / "CALLER_TOKEN.txt",
            home / "CALLER_TOKEN_ADMIN.txt",
            Path.home() / ".local/share/metrum-fleet/acme-rehearsal/CALLER_TOKEN_ACME.txt",
        ]
    )
    for path in candidates:
        if path.is_file():
            return path.read_text(encoding="utf-8").strip()
    die("no caller token file found; pass --token-file or grant-caller first")


def cmd_smoke(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    state = load_state(home)
    hostname = state.get("hostname") or f"{customer_id}.{HOSTNAME_SUFFIX}"
    base = f"https://{hostname}"
    deadline = time.time() + float(getattr(args, "timeout_sec", 180) or 180)
    last_err = "smoke not attempted"
    while time.time() < deadline:
        code, ready = http_json(f"{base}/readyz", timeout=20)
        if code != 200 or not (ready or {}).get("ok"):
            last_err = f"readyz http={code}"
            time.sleep(5)
            continue
        token = resolve_smoke_token(home, getattr(args, "token_file", "") or None)
        code, models = http_json(f"{base}/v1/models", token=token, timeout=30)
        ids = [m.get("id") for m in (models.get("data") or [])]
        if code != 200 or not ids:
            last_err = f"models http={code}"
            time.sleep(5)
            continue
        print(json.dumps({"readyz": {"http": 200, "ok": True, "version": (ready or {}).get("version")}}), flush=True)
        print(json.dumps({"models": {"http": code, "count": len(ids), "sample": ids[:8]}}), flush=True)
        expect = getattr(args, "expect_models_count", None)
        if expect is not None and len(ids) != int(expect):
            last_err = f"models count={len(ids)} want={expect}"
            time.sleep(5)
            continue
        if getattr(args, "skip_chat", False):
            return
        group = getattr(args, "model", None) or next((g for g in ("high", "default", "fast") if g in ids), ids[0])
        code, chat = http_json(
            f"{base}/v1/chat/completions",
            token=token,
            body={"model": group, "messages": [{"role": "user", "content": "Reply with exactly: OK"}], "max_tokens": 64, "stream": False},
            timeout=90,
        )
        content = (((chat.get("choices") or [{}])[0].get("message") or {}).get("content") or "")
        print(
            json.dumps(
                {
                    "chat": {
                        "http": code,
                        "model": chat.get("model"),
                        "group": group,
                        "content_len": len(content),
                        "has_OK": "OK" in content,
                    }
                }
            ),
            flush=True,
        )
        if code == 200:
            return
        last_err = f"chat http={code}"
        time.sleep(5)
    die(f"smoke failed after retries: {last_err}")

def cmd_grant_caller(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    priv, _ = ensure_keys(home)
    fleet, token_gen, sign_bin = resolve_operator_binaries(home / "bin", build_from_source=args.build_from_source)
    op_env = operator_env()
    fleet_env = assume_fleet_role(home, customer_id)
    source_ref = current_bundle_ref(home, args.runtime_bundle_ref)
    # Read with fleet role (authorized GetSecretValue).
    bundle = fetch_runtime_bundle(source_ref, fleet_env)
    allow = [x.strip() for x in args.allow.split(",") if x.strip()]
    if not allow:
        die("--allow requires at least one model group")
    key_args = [
        str(token_gen),
        "generate",
        "--owner-user",
        args.owner_user,
        "--project",
        args.project,
        "--env",
        args.env,
        "--allow",
        ",".join(allow),
        "--format",
        "json",
    ]
    if args.key:
        key_args.extend(["--key", args.key])
    proc = run(key_args, env=os.environ.copy())
    require_ok(proc, "router-token-gen")
    generated = json.loads(proc.stdout)
    token = generated["token"]
    caller = generated["caller"]
    # Match production-identical legacy caller fields used by the live bundle.
    caller_row = {
        "id": caller.get("id"),
        "user": caller.get("owner_user") or caller.get("user"),
        "project": caller.get("project"),
        "environment": caller.get("environment"),
        "token_id": caller.get("token_id") or generated.get("token_id"),
        "token_sha256": caller.get("token_sha256") or generated.get("token_sha256"),
        "allow": caller.get("allow") or allow,
        "metrics_admin": False,
    }
    token_out = Path(args.token_out).expanduser()
    if token_out.exists():
        die(f"token-out already exists: {token_out}")
    token_out.parent.mkdir(parents=True, exist_ok=True)
    token_out.write_text(token + "\n", encoding="utf-8")
    token_out.chmod(0o600)
    patched = apply_config_patch(bundle["config.yaml"], {"callers": [caller_row]})
    new_bundle = {"config.yaml": patched, "env.json": bundle["env.json"]}
    # Publish with operator identity (CreateSecret/PutSecretValue).
    new_ref = publish_runtime_bundle(customer_id, new_bundle, op_env)
    plan_and_deploy(
        home=home,
        fleet=fleet,
        sign_bin=sign_bin,
        priv=priv,
        customer_id=customer_id,
        profile_ref=args.profile_ref,
        runtime_bundle=new_ref,
        license_ref=args.license_ref,
        fleet_env=fleet_env,
        revision_prefix="grant",
    )
    print(json.dumps({"granted_caller_id": caller_row["id"], "token_file": str(token_out), "activation": "fleet-deployed"}), flush=True)
    # Smoke with the new token only.
    args.token_file = str(token_out)
    args.skip_chat = False
    args.model = allow[0]
    cmd_smoke(args)


def cmd_update_config(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    priv, _ = ensure_keys(home)
    fleet, _, sign_bin = resolve_operator_binaries(home / "bin", build_from_source=args.build_from_source)
    op_env = operator_env()
    fleet_env = assume_fleet_role(home, customer_id)
    patch_path = Path(args.patch_file).expanduser()
    if not patch_path.is_file():
        die(f"patch file not found: {patch_path}")
    patch = yaml.safe_load(patch_path.read_text(encoding="utf-8"))
    if not isinstance(patch, dict):
        die("patch-file must be a YAML mapping")
    source_ref = current_bundle_ref(home, args.runtime_bundle_ref)
    bundle = fetch_runtime_bundle(source_ref, fleet_env)
    patched = apply_config_patch(bundle["config.yaml"], patch)
    new_bundle = {"config.yaml": patched, "env.json": bundle["env.json"]}
    new_ref = publish_runtime_bundle(customer_id, new_bundle, op_env)
    plan_and_deploy(
        home=home,
        fleet=fleet,
        sign_bin=sign_bin,
        priv=priv,
        customer_id=customer_id,
        profile_ref=args.profile_ref,
        runtime_bundle=new_ref,
        license_ref=args.license_ref,
        fleet_env=fleet_env,
        revision_prefix="cfg",
    )
    cmd_smoke(args)


def cmd_delete(args: argparse.Namespace) -> None:
    customer_id = normalize_customer_id(args.customer_id)
    home = prepare_home(customer_id)
    priv, _ = ensure_keys(home)
    fleet, _, sign_bin = resolve_operator_binaries(home / "bin", build_from_source=args.build_from_source)
    fleet_env = assume_fleet_role(home, customer_id)
    intent_path = home / "intent.json"
    plan_path = home / "plan.json"
    if not intent_path.is_file() or not plan_path.is_file():
        die("delete requires workspace intent.json and plan.json from a prior create/update")
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    job_id = plan["job_id"]
    delete_path = home / "delete-approval.json"
    sign_delete(sign_bin, priv, job_id, delete_path, retain_pvc=args.retain_pvc)
    del_args = [
        "delete",
        "--intent",
        str(intent_path),
        "--registry",
        registry_path(home),
        "--confirm-file",
        str(delete_path),
        "--output",
        "json",
    ]
    if plan.get("database_id"):
        admission = home / "rds-admission.json"
        sign_admission(
            sign_bin,
            priv,
            approval_id=f"adm-{customer_id}-{time.strftime('%Y%m%dt%H%M%Sz', time.gmtime())}",
            profile_id=plan.get("profile_id") or "staging-fleet-nonprod",
            environment=plan.get("environment") or "nonproduction",
            database_profile=plan["database_profile"],
            job_id=job_id,
            namespace=plan["namespace"],
            manifest_sha256=plan["manifest_sha256"],
            out=admission,
        )
        del_args.extend(["--rds-admission-file", str(admission)])

    result = fleetctl_json(fleet, del_args, fleet_env, allow_fail=True)
    err_blob = f"{result.get('_error','')} {json.dumps(result)}".lower()
    if result.get("_exit") and ("record not found" in err_blob or result.get("error_class") == "record_not_found"):
        # Recreate registry row via idempotent deploy of the same intent, then delete.
        print("registry missing job; re-binding via deploy then delete", flush=True)
        fleetctl_json(
            fleet,
            ["deploy", "--intent", str(intent_path), "--registry", registry_path(home), "--output", "json"],
            fleet_env,
        )
        sign_delete(sign_bin, priv, job_id, delete_path, retain_pvc=args.retain_pvc)
        result = fleetctl_json(fleet, del_args, fleet_env)

    if result.get("_exit"):
        die(f"delete failed: state={result.get('state')} error_class={result.get('error_class')}")

    hostname = plan.get("hostname") or f"{customer_id}.{HOSTNAME_SUFFIX}"
    code, _ = http_json(f"https://{hostname}/readyz", timeout=15)
    print(json.dumps({"deleted_job_id": job_id, "hostname": hostname, "readyz_http": code, "state": result.get("state")}), flush=True)
    save_state(home, deleted=True, deleted_job_id=job_id)


def normalize_customer_id(raw: str) -> str:
    customer_id = raw.strip().lower()
    if not CUSTOMER_ID_RE.match(customer_id):
        die("customer-id must match ^[a-z][a-z0-9-]{1,30}$")
    return customer_id


def prepare_home(customer_id: str) -> Path:
    home = fleet_home(customer_id)
    home.mkdir(parents=True, exist_ok=True)
    home.chmod(0o700)
    return home


def add_common(sp: argparse.ArgumentParser) -> None:
    sp.add_argument("--customer-id", required=True)
    sp.add_argument("--profile-ref", default=DEFAULT_PROFILE_REF)
    sp.add_argument("--runtime-bundle-ref", default=DEFAULT_RUNTIME_BUNDLE)
    sp.add_argument("--license-ref", default=DEFAULT_LICENSE_REF)
    sp.add_argument(
        "--build-from-source",
        action="store_true",
        help="developer-only: build Fleet CLIs from this checkout; operators must use packaged binaries",
    )
    # Deprecated alias kept so existing operator scripts do not break; packaged binaries are always preferred.
    sp.add_argument("--skip-build", action="store_true", help=argparse.SUPPRESS)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="command", required=True)

    create_p = sub.add_parser("create", help="SQLite create/deploy")
    add_common(create_p)
    create_p.set_defaults(func=cmd_create)

    status_p = sub.add_parser("status", help="Fleet exact-job status")
    add_common(status_p)
    status_p.set_defaults(func=cmd_status)

    smoke_p = sub.add_parser("smoke", help="readyz + models (+ chat)")
    add_common(smoke_p)
    smoke_p.add_argument("--token-file", default="")
    smoke_p.add_argument("--model", default="")
    smoke_p.add_argument("--skip-chat", action="store_true")
    smoke_p.add_argument("--timeout-sec", type=int, default=180)
    smoke_p.add_argument("--expect-models-count", type=int, default=None)
    smoke_p.set_defaults(func=cmd_smoke)

    grant_p = sub.add_parser("grant-caller", help="generate+activate caller via Fleet")
    add_common(grant_p)
    grant_p.add_argument("--owner-user", required=True)
    grant_p.add_argument("--project", required=True)
    grant_p.add_argument("--env", default="nonproduction")
    grant_p.add_argument("--key", default="")
    grant_p.add_argument("--allow", required=True)
    grant_p.add_argument("--token-out", required=True)
    grant_p.set_defaults(func=cmd_grant_caller)

    update_p = sub.add_parser("update-config", help="merge patch, publish bundle, redeploy")
    add_common(update_p)
    update_p.add_argument("--patch-file", required=True)
    update_p.add_argument("--token-file", default="")
    update_p.add_argument("--model", default="")
    update_p.add_argument("--skip-chat", action="store_true")
    update_p.add_argument("--timeout-sec", type=int, default=180)
    update_p.add_argument("--expect-models-count", type=int, default=None)
    update_p.set_defaults(func=cmd_update_config)

    delete_p = sub.add_parser("delete", help="signed Fleet delete")
    add_common(delete_p)
    delete_p.add_argument("--retain-pvc", action="store_true")
    delete_p.set_defaults(func=cmd_delete)

    args = ap.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
