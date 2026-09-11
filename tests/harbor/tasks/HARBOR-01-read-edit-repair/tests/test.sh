#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
python3 - <<'PY'
from pathlib import Path
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
from verify import verify
ws = Path("/app") if Path("/app/normalize.py").is_file() else Path(__file__).resolve().parents[1] / "environment" / "workspace"
result = verify(ws)
reward_dir = Path("/logs/verifier")
reward_dir.mkdir(parents=True, exist_ok=True)
(reward_dir / "reward.txt").write_text(f"{result['reward']}\n", encoding="utf-8")
print(result)
sys.exit(0 if result["passed"] else 1)
PY
