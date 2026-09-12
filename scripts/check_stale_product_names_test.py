#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Tests for the stale product-name scanner."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest import mock

import check_stale_product_names as scanner


class StaleProductNameScanTest(unittest.TestCase):
    def test_scan_dist_rejects_obsolete_archives(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "metrum-router-v1.4.4-linux-amd64.tar.gz").write_bytes(b"x")
            (root / "metrum-ai-router-v2.0.0-linux-amd64.tar.gz").write_bytes(b"y")
            with mock.patch.object(scanner, "ROOT", root):
                errors = scanner.scan_dist(root)
            self.assertTrue(any("metrum-router" in error for error in errors), errors)
            self.assertFalse(any("metrum-ai-router-v2.0.0" in error for error in errors), errors)

    def test_scan_docs_links_optional(self) -> None:
        self.assertEqual([], scanner.scan_docs_links(enforce_docs_origin=False))

    def test_canonical_product_contract_file(self) -> None:
        # The checked-in constants module must always declare the canonical slug.
        errors = scanner.scan_contract_files()
        self.assertFalse(any("canonical_product.py" in error for error in errors), errors)


if __name__ == "__main__":
    raise SystemExit(unittest.main())
