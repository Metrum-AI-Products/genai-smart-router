#!/usr/bin/env python3
"""Backup release package tarballs from DIST_DIR to the Metrum CTO restic repo.

Credentials come from ignored env.json (or the process environment):
  BACKUP_USER, BACKUP_PASS, RESTIC_PASSWORD

Optional overrides:
  RESTIC_REPOSITORY  full restic repo URL (rarely needed)
  RESTIC_REPO_HOST   default backups.metrum.ai
  RESTIC_REPO_PATH   default metrum-cto

Backs up fleet-admin binary packages (include metrum-fleetctl) and customer
Docker packages. Customer license payloads are never packaged and are not
included in this backup.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import urllib.parse
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_ENV_JSON = ROOT / "env.json"
DEFAULT_HOST = "backups.metrum.ai"
DEFAULT_PATH = "metrum-cto"
PACKAGE_GLOB = "*.tar.gz"
DOCKER_PACKAGE_RE = re.compile(
    r"^smart-llmrouter-.+-docker-linux-(amd64|arm64)\.tar\.gz$"
)
BINARY_PACKAGE_RE = re.compile(
    r"^smart-llmrouter-.+-linux-(amd64|arm64)\.tar\.gz$"
)
REQUIRED_KINDS = (
    ("binary", "amd64"),
    ("binary", "arm64"),
    ("docker", "amd64"),
    ("docker", "arm64"),
)


def is_docker_package(name: str) -> bool:
    return DOCKER_PACKAGE_RE.match(name) is not None


def is_binary_package(name: str) -> bool:
    return BINARY_PACKAGE_RE.match(name) is not None and not is_docker_package(name)


def is_release_package(name: str) -> bool:
    return is_binary_package(name) or is_docker_package(name)


def package_arch(name: str) -> str | None:
    match = re.search(r"linux-(amd64|arm64)\.tar\.gz$", name)
    return match.group(1) if match else None


def package_kind(name: str) -> str | None:
    if is_docker_package(name):
        return "docker"
    if is_binary_package(name):
        return "binary"
    return None


def package_version(name: str) -> str | None:
    if is_docker_package(name):
        match = re.match(
            r"^smart-llmrouter-(.+)-docker-linux-(?:amd64|arm64)\.tar\.gz$",
            name,
        )
    elif is_binary_package(name):
        match = re.match(
            r"^smart-llmrouter-(.+)-linux-(?:amd64|arm64)\.tar\.gz$",
            name,
        )
    else:
        return None
    return match.group(1) if match else None


def list_release_packages(dist_dir: Path) -> list[Path]:
    if not dist_dir.is_dir():
        raise SystemExit(f"dist directory not found: {dist_dir}")
    return sorted(
        path
        for path in dist_dir.glob(PACKAGE_GLOB)
        if path.is_file() and is_release_package(path.name)
    )


def missing_required_packages(packages: list[Path]) -> list[str]:
    present = {
        (package_kind(path.name), package_arch(path.name))
        for path in packages
        if package_kind(path.name) and package_arch(path.name)
    }
    missing: list[str] = []
    for kind, arch in REQUIRED_KINDS:
        if (kind, arch) not in present:
            missing.append(f"{kind} linux-{arch}")
    return missing


def version_tags(packages: list[Path]) -> list[str]:
    tags = {"cto", "smart-llmrouter", "release-packages"}
    for path in packages:
        kind = package_kind(path.name)
        if kind == "docker":
            tags.add("customer-docker")
        elif kind == "binary":
            tags.add("fleet-admin-binary")
        version = package_version(path.name)
        if version:
            tags.add(f"version:{version}")
    return sorted(tags)


def load_env_json(path: Path) -> dict[str, str]:
    if not path.is_file():
        return {}
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"{path}: must contain a JSON object")
    out: dict[str, str] = {}
    for key, value in data.items():
        if isinstance(value, str) and value != "":
            out[str(key)] = value
    return out


def merge_credentials(env_file: dict[str, str]) -> dict[str, str]:
    merged = dict(env_file)
    for key in (
        "BACKUP_USER",
        "BACKUP_PASS",
        "RESTIC_PASSWORD",
        "RESTIC_REPOSITORY",
        "RESTIC_REPO_HOST",
        "RESTIC_REPO_PATH",
    ):
        value = os.environ.get(key)
        if value:
            merged[key] = value
    return merged


def require_credential(creds: dict[str, str], key: str) -> str:
    value = creds.get(key, "").strip()
    if not value:
        raise SystemExit(
            f"{key} is required in ignored env.json or the process environment"
        )
    return value


def build_repository_url(creds: dict[str, str]) -> str:
    explicit = creds.get("RESTIC_REPOSITORY", "").strip()
    if explicit:
        return explicit
    user = urllib.parse.quote(require_credential(creds, "BACKUP_USER"), safe="")
    password = urllib.parse.quote(require_credential(creds, "BACKUP_PASS"), safe="")
    host = creds.get("RESTIC_REPO_HOST", DEFAULT_HOST).strip() or DEFAULT_HOST
    path = creds.get("RESTIC_REPO_PATH", DEFAULT_PATH).strip().strip("/") or DEFAULT_PATH
    return f"rest:https://{user}:{password}@{host}/{path}"


def run_restic(
    repository: str,
    restic_password: str,
    args: list[str],
    *,
    dry_run: bool,
) -> None:
    if shutil.which("restic") is None:
        raise SystemExit("restic is required on PATH")
    env = os.environ.copy()
    env["RESTIC_REPOSITORY"] = repository
    env["RESTIC_PASSWORD"] = restic_password
    # Prefer env-based repo so argv does not echo the HTTP password.
    cmd = ["restic", *args]
    if dry_run:
        print(f"dry-run: restic {' '.join(args)}")
        return
    subprocess.run(cmd, check=True, env=env)


def backup_packages(
    dist_dir: Path,
    packages: list[Path],
    creds: dict[str, str],
    *,
    dry_run: bool,
) -> None:
    if not packages:
        raise SystemExit(
            f"no release package tarballs found under {dist_dir}; "
            "run `make package-all package-docker-all` first"
        )
    missing = missing_required_packages(packages)
    if missing:
        raise SystemExit(
            "incomplete release set for CTO backup; missing: "
            + ", ".join(missing)
            + ". Build with `make package-all package-docker-all`."
        )

    repository = build_repository_url(creds)
    restic_password = require_credential(creds, "RESTIC_PASSWORD")
    tags = version_tags(packages)
    tag_args: list[str] = []
    for tag in tags:
        tag_args.extend(["--tag", tag])

    print(f"backing up {len(packages)} package(s) from {dist_dir}:")
    for path in packages:
        kind = "customer-docker" if is_docker_package(path.name) else "fleet-admin-binary"
        print(f"  - {path.name} ({kind})")

    # Backup only the release tarballs, not unpack/build trees under dist/.
    run_restic(
        repository,
        restic_password,
        ["backup", *tag_args, "--", *[str(path) for path in packages]],
        dry_run=dry_run,
    )
    print("restic backup complete")


def self_test() -> None:
    with_creds = {
        "BACKUP_USER": "chetan",
        "BACKUP_PASS": "example-pass",
        "RESTIC_PASSWORD": "example-restic",
    }
    url = build_repository_url(with_creds)
    expected = "rest:https://chetan:example-pass@backups.metrum.ai/metrum-cto"
    if url != expected:
        raise AssertionError(f"repository URL mismatch: {url!r}")

    encoded = build_repository_url(
        {
            "BACKUP_USER": "u/n",
            "BACKUP_PASS": "p@ss:word",
            "RESTIC_PASSWORD": "x",
        }
    )
    if "u%2Fn" not in encoded or "p%40ss%3Aword" not in encoded:
        raise AssertionError(f"credentials were not URL-encoded: {encoded!r}")

    explicit = build_repository_url({"RESTIC_REPOSITORY": "rest:https://example/repo"})
    if explicit != "rest:https://example/repo":
        raise AssertionError("explicit RESTIC_REPOSITORY override failed")

    names = [
        Path("smart-llmrouter-v1-linux-amd64.tar.gz"),
        Path("smart-llmrouter-v1-linux-arm64.tar.gz"),
        Path("smart-llmrouter-v1-docker-linux-amd64.tar.gz"),
        Path("smart-llmrouter-v1-docker-linux-arm64.tar.gz"),
    ]
    if missing_required_packages(names):
        raise AssertionError("complete set reported missing packages")
    if not missing_required_packages(names[:2]):
        raise AssertionError("incomplete set should report missing docker packages")
    if missing_required_packages(names[2:]) != ["binary linux-amd64", "binary linux-arm64"]:
        raise AssertionError("docker-only set should report missing binary packages")
    if is_binary_package("smart-llmrouter-v1-docker-linux-amd64.tar.gz"):
        raise AssertionError("docker package must not classify as binary")
    if package_version("smart-llmrouter-v1-docker-linux-amd64.tar.gz") != "v1":
        raise AssertionError("docker package version parse failed")

    tags = version_tags(names)
    for required in (
        "cto",
        "smart-llmrouter",
        "release-packages",
        "fleet-admin-binary",
        "customer-docker",
        "version:v1",
    ):
        if required not in tags:
            raise AssertionError(f"missing tag {required!r} in {tags}")
    if "version:v1-docker" in tags:
        raise AssertionError(f"docker suffix leaked into version tag: {tags}")
    print("backup_dist_restic self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist-dir", default=os.environ.get("DIST_DIR", "dist"))
    parser.add_argument("--env-json", type=Path, default=DEFAULT_ENV_JSON)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        self_test()
        return 0

    dist_dir = Path(args.dist_dir)
    if not dist_dir.is_absolute():
        dist_dir = ROOT / dist_dir

    creds = merge_credentials(load_env_json(args.env_json))
    packages = list_release_packages(dist_dir)
    backup_packages(dist_dir, packages, creds, dry_run=args.dry_run)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as exc:
        print(f"restic failed with exit code {exc.returncode}", file=sys.stderr)
        raise SystemExit(exc.returncode) from exc
