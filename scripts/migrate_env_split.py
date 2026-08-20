#!/usr/bin/env python3
"""Split a mixed env.json into instance env.json + commerce.env.json + ops.env.json.

Never prints secret values. Maps nested STRIPE_KEYS.SANDBOX_KEYS to flat STRIPE_*.
Shell / operator: copy commerce.env.example.json and ops.env.example.json first if needed.

Usage:
  python3 scripts/migrate_env_split.py --input env.json --dry-run
  python3 scripts/migrate_env_split.py --input env.json --force
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]

COMMERCE_KEYS = frozenset(
    {
        "STRIPE_SECRET_KEY",
        "STRIPE_PUBLISHABLE_KEY",
        "STRIPE_WEBHOOK_SECRET",
        "COMMERCE_ADMIN_TOKEN",
        "COMMERCE_FLEET_ENABLED",
        "COMMERCE_FLEET_MODE",
        "COMMERCE_FLEET_BINARY",
        "COMMERCE_FLEET_PROFILE_REF",
        "COMMERCE_FLEET_LICENSE_REF",
        "COMMERCE_FLEET_CONFIG_FILE",
        "COMMERCE_FLEET_ENV_FILE",
        "COMMERCE_FLEET_SIGN_KEY",
        "COMMERCE_FLEET_OWNER_USER",
        "COMMERCE_FLEET_PROJECT",
        "COMMERCE_FLEET_TOKEN_OUT",
    }
)

OPS_KEYS = frozenset(
    {
        "WORK_ITEMS_DASHBOARD_PORT",
        "BACKUP_USER",
        "BACKUP_PASS",
        "RESTIC_PASSWORD",
        "RESTIC_REPO_HOST",
        "RESTIC_REPO_PATH",
        "RESTIC_REPOSITORY",
    }
)

# Nested Stripe map from older mixed env.json files.
STRIPE_NESTED_ROOT = "STRIPE_KEYS"
SANDBOX_KEYS = "SANDBOX_KEYS"
SANDBOX_FLAT = {
    "SECRET_KEY": "STRIPE_SECRET_KEY",
    "PUBLISHABLE_KEY": "STRIPE_PUBLISHABLE_KEY",
    "WEBHOOK_SECRET": "STRIPE_WEBHOOK_SECRET",
}


def _as_str(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bool):
        return "1" if value else "0"
    if isinstance(value, (int, float)):
        # Preserve integers as decimal strings (e.g. dashboard port).
        if isinstance(value, float) and value.is_integer():
            return str(int(value))
        return str(value)
    return str(value)


def flatten_stripe_keys(raw: dict[str, Any]) -> dict[str, str]:
    """Extract flat STRIPE_* from flat keys and/or STRIPE_KEYS.SANDBOX_KEYS."""
    out: dict[str, str] = {}
    nested = raw.get(STRIPE_NESTED_ROOT)
    if isinstance(nested, dict):
        sandbox = nested.get(SANDBOX_KEYS)
        if isinstance(sandbox, dict):
            for nested_key, flat_key in SANDBOX_FLAT.items():
                if nested_key in sandbox and flat_key not in out:
                    out[flat_key] = _as_str(sandbox[nested_key])
    for flat_key in (
        "STRIPE_SECRET_KEY",
        "STRIPE_PUBLISHABLE_KEY",
        "STRIPE_WEBHOOK_SECRET",
    ):
        if flat_key in raw and _as_str(raw[flat_key]):
            out[flat_key] = _as_str(raw[flat_key])
        elif flat_key in raw and flat_key not in out:
            out[flat_key] = _as_str(raw[flat_key])
    return out


def split_env(raw: dict[str, Any]) -> tuple[dict[str, str], dict[str, str], dict[str, str]]:
    """Return (instance, commerce, ops) string maps. Never logs values."""
    instance: dict[str, str] = {}
    commerce: dict[str, str] = {}
    ops: dict[str, str] = {}

    commerce.update(flatten_stripe_keys(raw))

    for key, value in raw.items():
        if key == STRIPE_NESTED_ROOT:
            continue
        if key in COMMERCE_KEYS:
            # Prefer already-flattened Stripe values; otherwise take flat entry.
            if key.startswith("STRIPE_") and key in commerce and commerce[key]:
                continue
            commerce[key] = _as_str(value)
            continue
        if key in OPS_KEYS:
            ops[key] = _as_str(value)
            continue
        if isinstance(value, dict):
            # Unknown nested objects stay out of instance env (avoid secret blobs).
            continue
        instance[key] = _as_str(value)

    return instance, commerce, ops


def write_json(path: Path, data: dict[str, str], *, force: bool) -> None:
    if path.exists() and not force:
        raise FileExistsError(f"refusing to overwrite {path} without --force")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    path.chmod(0o600)


def summarize(name: str, data: dict[str, str]) -> str:
    nonempty = sum(1 for v in data.values() if v != "")
    return f"{name}: {len(data)} keys ({nonempty} non-empty)"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--input",
        type=Path,
        default=ROOT / "env.json",
        help="mixed env.json path (default: ./env.json)",
    )
    parser.add_argument(
        "--instance-out",
        type=Path,
        default=ROOT / "env.json",
        help="instance env.json output (default: ./env.json)",
    )
    parser.add_argument(
        "--commerce-out",
        type=Path,
        default=ROOT / "commerce.env.json",
        help="commerce.env.json output",
    )
    parser.add_argument(
        "--ops-out",
        type=Path,
        default=ROOT / "ops.env.json",
        help="ops.env.json output",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print key counts only; do not write files",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="overwrite existing output files",
    )
    args = parser.parse_args(argv)

    if not args.input.is_file():
        print(f"input not found: {args.input}", file=sys.stderr)
        return 1

    try:
        raw = json.loads(args.input.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"invalid JSON in {args.input}: {exc}", file=sys.stderr)
        return 1
    if not isinstance(raw, dict):
        print(f"{args.input} must be a JSON object", file=sys.stderr)
        return 1

    instance, commerce, ops = split_env(raw)
    print(summarize("instance", instance))
    print(summarize("commerce", commerce))
    print(summarize("ops", ops))

    if args.dry_run:
        print("dry-run: no files written")
        return 0

    # When instance-out == input, writing instance requires --force once mixed file exists.
    targets = [args.instance_out, args.commerce_out, args.ops_out]
    for path in targets:
        if path.exists() and not args.force:
            print(
                f"refusing to overwrite {path}; pass --force after reviewing dry-run",
                file=sys.stderr,
            )
            return 2

    try:
        write_json(args.instance_out, instance, force=True)
        write_json(args.commerce_out, commerce, force=True)
        write_json(args.ops_out, ops, force=True)
    except OSError as exc:
        print(f"write failed: {exc}", file=sys.stderr)
        return 1

    print(f"wrote {args.instance_out}")
    print(f"wrote {args.commerce_out}")
    print(f"wrote {args.ops_out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
