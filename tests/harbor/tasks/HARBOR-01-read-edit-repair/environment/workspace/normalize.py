# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Starter implementation — fails the whitespace-only edge case (returns None)."""

STARTER_MARKER = "HARBOR01_STARTER_UNCHANGED"


def normalize(value: str) -> str | None:
    text = " ".join(value.strip().split())
    if not text:
        # Deliberate starter bug: edge case returns None instead of "".
        return None
    return text.lower()
