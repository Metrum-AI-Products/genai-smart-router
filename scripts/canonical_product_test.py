#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Unit tests for canonical product identifiers."""

from __future__ import annotations

import unittest

import canonical_product as product


class CanonicalProductTest(unittest.TestCase):
    def test_slug_and_title(self) -> None:
        self.assertEqual("Metrum AI Router", product.PRODUCT_TITLE)
        self.assertEqual("metrum-ai-router", product.PRODUCT_SLUG)
        self.assertEqual(product.PRODUCT_SLUG, product.PKG_NAME)
        self.assertEqual(product.PRODUCT_SLUG, product.IMAGE_NAME)

    def test_docs_url(self) -> None:
        self.assertEqual("https://llm-api.apps.metrum.ai/docs", product.DOCS_SITE_URL)
        self.assertTrue(product.is_allowed_docs_url(product.DOCS_SITE_URL))
        self.assertTrue(product.is_allowed_docs_url(product.DOCS_SITE_URL + "/overview"))
        self.assertFalse(product.is_allowed_docs_url("https://llm-api.apps.metrum.ai/v1"))
        self.assertFalse(product.is_allowed_docs_url("https://docs.metrum.ai/docs/overview"))

    def test_strip_allowed_docs_urls(self) -> None:
        text = "See https://llm-api.apps.metrum.ai/docs/overview and llm-api.apps.metrum.ai/v1"
        stripped = product.strip_allowed_docs_urls(text)
        self.assertNotIn("https://llm-api.apps.metrum.ai/docs/overview", stripped)
        self.assertIn("llm-api.apps.metrum.ai/v1", stripped)

    def test_release_archive_re(self) -> None:
        match = product.RELEASE_ARCHIVE_RE.fullmatch("metrum-ai-router-v2.0.0-linux-amd64.tar.gz")
        self.assertIsNotNone(match)
        assert match is not None
        self.assertEqual("v2.0.0", match.group("version"))
        self.assertIsNone(match.group("docker"))
        docker = product.RELEASE_ARCHIVE_RE.fullmatch(
            "metrum-ai-router-v2.0.0-docker-linux-arm64.tar.gz"
        )
        self.assertIsNotNone(docker)
        assert docker is not None
        self.assertEqual("-docker", docker.group("docker"))
        self.assertIsNone(product.RELEASE_ARCHIVE_RE.fullmatch("metrum-router-v1.4.4-linux-amd64.tar.gz"))

    def test_obsolete_slug_in_filename(self) -> None:
        self.assertEqual(
            "metrum-router",
            product.obsolete_slug_in_filename("metrum-router-v1.4.4-linux-amd64.tar.gz"),
        )
        self.assertEqual(
            "genai-smart-router",
            product.obsolete_slug_in_filename("genai-smart-router-v1.2.0-source.tar.gz"),
        )
        self.assertEqual(
            "smart-llmrouter",
            product.obsolete_slug_in_filename("smart-llmrouter-v1.0.2-linux-amd64.tar.gz"),
        )
        self.assertIsNone(product.obsolete_slug_in_filename("metrum-ai-router-v2.0.0-linux-amd64.tar.gz"))
        self.assertIsNone(product.obsolete_slug_in_filename("bin/metrum-ai-routerctl"))


if __name__ == "__main__":
    raise SystemExit(unittest.main())
