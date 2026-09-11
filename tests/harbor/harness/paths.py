# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

from pathlib import Path

HARBOR_ROOT = Path(__file__).resolve().parents[1]
TASKS_DIR = HARBOR_ROOT / "tasks"
MANIFEST_PATH = HARBOR_ROOT / "manifest" / "harbor.yaml"

TASK_IDS = (
    "HARBOR-01",
    "HARBOR-02",
    "HARBOR-03",
    "HARBOR-04",
    "HARBOR-05",
    "HARBOR-06",
)

TASK_DIR_BY_ID = {
    "HARBOR-01": TASKS_DIR / "HARBOR-01-read-edit-repair",
    "HARBOR-02": TASKS_DIR / "HARBOR-02-nonce-chain",
    "HARBOR-03": TASKS_DIR / "HARBOR-03-parallel-tool-ids",
    "HARBOR-04": TASKS_DIR / "HARBOR-04-tool-error-retry",
    "HARBOR-05": TASKS_DIR / "HARBOR-05-large-stream-patch",
    "HARBOR-06": TASKS_DIR / "HARBOR-06-no-replay-disconnect",
}
