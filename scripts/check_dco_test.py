#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Contract tests for the DCO trailer checker."""

from __future__ import annotations

import importlib.util
from pathlib import Path


PATH = Path(__file__).with_name("check_dco.py")
SPEC = importlib.util.spec_from_file_location("check_dco", PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main() -> None:
    accepted = [
        "Change\n\nSigned-off-by: Example Person <person@example.com>\n",
        "Change\r\n\r\nsigned-off-by: Example Person <person@example.com>\r\n",
    ]
    rejected = [
        "Change only",
        "Signed-off-by: no-address",
        "Signed-off-by: Example <person@example.com> trailing",
    ]
    assert all(MODULE.TRAILER.search(message) for message in accepted)
    assert not any(MODULE.TRAILER.search(message) for message in rejected)
    print("DCO checker contract tests passed")


if __name__ == "__main__":
    main()
