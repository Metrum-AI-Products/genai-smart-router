#!/usr/bin/env python3
"""Deterministic contract check for generated migration release notes."""
import release_notes_from_git as notes

for required in ("2026071901", "2026072301", "2026080501", "usage", "online/bounded", "restore-required"):
    assert required in notes.MIGRATION_CONTRACT, required

print("release migration contract: ok")
