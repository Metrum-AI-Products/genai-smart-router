#!/usr/bin/env python3
"""Focused regressions for protected staging ECR controls."""

from __future__ import annotations

import importlib.util
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "validate_eks_bootstrap_assets",
    ROOT / "scripts/validate_eks_bootstrap_assets.py",
)
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load EKS bootstrap validator")
VALIDATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VALIDATOR)


def rejected(template: str, expected: str) -> None:
    try:
        VALIDATOR.validate_staging_ecr_contract(template)
    except ValueError as exc:
        if expected not in str(exc):
            raise AssertionError(f"unexpected rejection: {exc}") from exc
    else:
        raise AssertionError("unsafe staging ECR contract was accepted")


def main() -> int:
    template = VALIDATOR.STAGING_IDENTITY_STACK.read_text(encoding="utf-8")
    VALIDATOR.validate_staging_ecr_contract(template)

    broad = template.replace(
        "Resource: !GetAtt SmartRouterStagingImageRepository.Arn",
        'Resource: "*"',
        1,
    )
    rejected(broad, "forbidden wildcard scope")

    displaced = template.replace('                Resource: "*"\n', "", 1).replace(
        "                Resource: !GetAtt SmartRouterStagingImageRepository.Arn",
        '                Resource: "*"',
        1,
    )
    rejected(displaced, "token-only IAM statement")

    count_expiry = template.replace(
        '"tagPrefixList": ["cleanup-approved-"],\n                  "countType": "sinceImagePushed",\n                  "countUnit": "days",\n                  "countNumber": 7',
        '"tagPrefixList": ["staging-"],\n                  "countType": "imageCountMoreThan",\n                  "countNumber": 30',
        1,
    )
    rejected(count_expiry, "explicitly cleanup-approved tags")

    untagged_expiry = template.replace(
        '"tagStatus": "tagged",\n                  "tagPrefixList": ["cleanup-approved-"],',
        '"tagStatus": "untagged",',
        1,
    )
    rejected(untagged_expiry, "explicitly cleanup-approved tags")

    print("staging ECR contract regression tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
