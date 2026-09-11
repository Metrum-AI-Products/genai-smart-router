# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline Harbor agent-adapter contract suite (issue #94 AGENT-01..06).

Run (stdlib, no network, no Harbor install)::

    make harbor-adapter-test

Or::

    python3 -m unittest discover -s tests/harbor -p 'test_adapter*.py' -v

Harbor P0 local-task pytest suite (separate target)::

    make harbor-local

    # or: cd tests/harbor && uv sync --locked && uv run --offline pytest

HARBOR-07..16 offline task stubs live under ``tests/harbor/p1/`` and are
invoked separately via ``python3 tests/harbor/p1/run_offline_tests.py``.

These tests do **not** run live Harbor against real providers. Missing
credentials must surface as blocked/skipped evidence, never a green pass.
Live Harbor matrices remain opt-in operator workflows under
``examples/harbor-algotune-pca/``.
"""
