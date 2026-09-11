# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

from conftest import assert_generated_artifacts_redacted


def test_generated_artifacts_redact_synthetic_auth_and_provider_values(router):
    assert_generated_artifacts_redacted(router)
