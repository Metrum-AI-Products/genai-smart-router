# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

from pathlib import Path

REFERENCE = '''# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Reference solution for HARBOR-01."""

STARTER_MARKER = "HARBOR01_SOLUTION"


def normalize(value: str) -> str:
    return " ".join(value.strip().split()).lower()
'''

WRONG = '''# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

STARTER_MARKER = "HARBOR01_WRONG"


def normalize(value: str) -> str:
    # Wrong: uppercases and does not collapse whitespace correctly.
    return value.strip().upper()
'''


def apply(workspace: Path, mode: str) -> None:
    target = workspace / "normalize.py"
    if mode == "solution":
        target.write_text(REFERENCE, encoding="utf-8")
    elif mode == "wrong":
        target.write_text(WRONG, encoding="utf-8")
    else:
        raise ValueError(f"unsupported mode {mode!r}")
