#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Validate a Fleet production runtime bundle config for EKS path and caller parity.

Reads a local config.yaml (for example production-identical.yaml) and checks that
state paths, trusted proxy CIDRs, and caller token_sha256 entries are present.
Never reads or prints raw router tokens.
"""

from __future__ import annotations

import argparse
import pathlib
import sys

import yaml


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("config", type=pathlib.Path)
    parser.add_argument(
        "--require-trusted-proxy",
        default="192.168.0.0/16",
        help="Expected server.client_ip.trusted_proxy_cidrs entry",
    )
    args = parser.parse_args()
    data = yaml.safe_load(args.config.read_text())
    server = data.get("server") or {}
    state_path = data.get("state_path", "")
    errors: list[str] = []
    if not str(state_path).startswith("/var/lib/smart-llmrouter"):
        errors.append(f"state_path must use /var/lib/smart-llmrouter, got {state_path!r}")
    client_ip = server.get("client_ip") or {}
    cidrs = client_ip.get("trusted_proxy_cidrs") or []
    if args.require_trusted_proxy not in cidrs:
        errors.append(f"missing trusted_proxy_cidrs entry {args.require_trusted_proxy}")
    callers = data.get("callers") or []
    if not callers:
        errors.append("callers list is empty")
    for caller in callers:
        token_hash = caller.get("token_sha256") or ""
        if len(token_hash) != 64:
            errors.append(f"caller {caller.get('id')} missing valid token_sha256")
    if errors:
        for err in errors:
            print(err, file=sys.stderr)
        return 1
    print(f"ok: {len(callers)} callers, trusted proxy {args.require_trusted_proxy}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
