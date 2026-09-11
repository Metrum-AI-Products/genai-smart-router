# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""SSE framing helpers that tolerate incremental byte delivery."""

from __future__ import annotations

import json
from typing import Iterator


def parse_sse_frames(raw: bytes | str) -> list[tuple[str, str]]:
    """Parse complete SSE frames into (event_name, data) pairs.

    Data is returned as the raw payload string (joined multiline data fields).
    """
    text = raw.decode() if isinstance(raw, (bytes, bytearray)) else raw
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    events: list[tuple[str, str]] = []
    for frame in text.split("\n\n"):
        if not frame.strip():
            continue
        event_name = ""
        data_lines: list[str] = []
        for line in frame.split("\n"):
            if line.startswith(":"):
                continue
            if line.startswith("event:"):
                event_name = line[6:].lstrip()
            elif line.startswith("data:"):
                data_lines.append(line[5:].lstrip() if line.startswith("data: ") else line[5:])
        if not data_lines and not event_name:
            continue
        events.append((event_name, "\n".join(data_lines)))
    return events


def sse_events(raw: bytes | str) -> list[tuple[str, object]]:
    """Parse SSE and JSON-decode data payloads except the Chat [DONE] sentinel."""
    out: list[tuple[str, object]] = []
    for name, data in parse_sse_frames(raw):
        if data == "[DONE]":
            out.append((name, "[DONE]"))
        elif data == "":
            out.append((name, ""))
        else:
            out.append((name, json.loads(data)))
    return out


def iter_sse_incremental(chunks: Iterator[bytes]) -> Iterator[tuple[str, str]]:
    """Yield complete SSE frames as bytes arrive (CRLF/LF safe)."""
    buf = bytearray()
    for chunk in chunks:
        buf.extend(chunk)
        while True:
            # Prefer CRLF frame separator, then LF.
            sep = buf.find(b"\r\n\r\n")
            n = 4
            if sep < 0:
                sep = buf.find(b"\n\n")
                n = 2
            if sep < 0:
                break
            frame = bytes(buf[:sep])
            del buf[: sep + n]
            text = frame.decode().replace("\r\n", "\n")
            event_name = ""
            data_lines: list[str] = []
            for line in text.split("\n"):
                if line.startswith(":"):
                    continue
                if line.startswith("event:"):
                    event_name = line[6:].lstrip()
                elif line.startswith("data:"):
                    data_lines.append(line[5:].lstrip() if line.startswith("data: ") else line[5:])
            if data_lines or event_name:
                yield event_name, "\n".join(data_lines)
