# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Simulated tools for HARBOR-02 nonce chain."""

from __future__ import annotations

import hashlib
import json
import secrets
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class ToolCall:
    name: str
    call_id: str
    arguments: dict
    output: dict | None = None
    is_error: bool = False


@dataclass
class NonceChainSession:
    workspace: Path
    nonce: str = field(default_factory=lambda: secrets.token_hex(8))
    calls: list[ToolCall] = field(default_factory=list)

    def lookup(self, *, call_id: str, key: str) -> ToolCall:
        if key != "session":
            call = ToolCall("lookup", call_id, {"key": key}, {"error": "unknown key"}, True)
        else:
            call = ToolCall("lookup", call_id, {"key": key}, {"nonce": self.nonce}, False)
        self.calls.append(call)
        return call

    def compute(self, *, call_id: str, nonce: str) -> ToolCall:
        digest = hashlib.sha256(f"harbor02:{nonce}".encode()).hexdigest()
        call = ToolCall("compute", call_id, {"nonce": nonce}, {"digest": digest}, False)
        self.calls.append(call)
        return call

    def write_result(self, *, call_id: str, nonce: str, digest: str) -> ToolCall:
        payload = {"nonce": nonce, "digest": digest}
        (self.workspace / "artifact.json").write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        call = ToolCall("write_result", call_id, payload, {"written": True}, False)
        self.calls.append(call)
        return call

    def inspect(self, *, call_id: str) -> ToolCall:
        path = self.workspace / "artifact.json"
        data = json.loads(path.read_text(encoding="utf-8")) if path.is_file() else None
        call = ToolCall("inspect", call_id, {}, {"artifact": data}, False)
        self.calls.append(call)
        return call


def run_reference_chain(workspace: Path) -> NonceChainSession:
    session = NonceChainSession(workspace=workspace)
    look = session.lookup(call_id="call_lookup_1", key="session")
    nonce = look.output["nonce"]
    comp = session.compute(call_id="call_compute_1", nonce=nonce)
    digest = comp.output["digest"]
    session.write_result(call_id="call_write_1", nonce=nonce, digest=digest)
    session.inspect(call_id="call_inspect_1")
    return session
