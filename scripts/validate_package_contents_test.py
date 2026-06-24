#!/usr/bin/env python3
"""Self-test package content validation rules."""

from __future__ import annotations

import io
import tarfile
import tempfile
from pathlib import Path

import validate_package_contents


def write_allowlist(root: Path) -> Path:
    allowlist = root / "allowlist.txt"
    allowlist.write_text("README.md\ndocs/DEPLOYMENT.md\n", encoding="utf-8")
    return allowlist


def write_tar(path: Path, files: dict[str, str]) -> None:
    with tarfile.open(path, "w:gz") as package:
        for name, text in files.items():
            data = text.encode("utf-8")
            info = tarfile.TarInfo(name)
            info.size = len(data)
            package.addfile(info, io.BytesIO(data))


def expect_errors(archive: Path, allowlist: Path, want: list[str]) -> None:
    errors = validate_package_contents.validate_archives([archive], allowlist)
    for expected in want:
        if not any(expected in error for error in errors):
            raise AssertionError(f"{archive}: missing {expected!r} in errors {errors!r}")


def expect_ok(archive: Path, allowlist: Path) -> None:
    errors = validate_package_contents.validate_archives([archive], allowlist)
    if errors:
        raise AssertionError(f"{archive}: unexpected errors {errors!r}")


def main() -> int:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        allowlist = write_allowlist(root)

        good = root / "good.tar.gz"
        write_tar(
            good,
            {
                "pkg/docs/README.md": "package-safe docs\n",
                "pkg/docs/DEPLOYMENT.md": "generic deployment docs\n",
            },
        )
        expect_ok(good, allowlist)

        private_runbook = root / "private-runbook.tar.gz"
        write_tar(
            private_runbook,
            {
                "pkg/docs/README.md": "package-safe docs\n",
                "pkg/docs/DEPLOYMENT.md": "generic deployment docs\n",
                "pkg/docs/PRODUCTION_RUNBOOK.md": "private\n",
            },
        )
        expect_errors(private_runbook, allowlist, ["forbidden private runbook", "not in package docs allowlist"])

        private_marker = root / "private-marker.tar.gz"
        write_tar(
            private_marker,
            {
                "pkg/docs/README.md": "Host: 100.30.225.66\n",
                "pkg/docs/DEPLOYMENT.md": "generic deployment docs\n",
            },
        )
        expect_errors(private_marker, allowlist, ["private production host marker"])

        raw_token = root / "raw-token.tar.gz"
        write_tar(
            raw_token,
            {
                "pkg/docs/README.md": "token rtr_metrum_user_project_prod_key_abcdefghijklmnopqrstuvwxyz\n",
                "pkg/docs/DEPLOYMENT.md": "generic deployment docs\n",
            },
        )
        expect_errors(raw_token, allowlist, ["raw router token"])

        missing_doc = root / "missing-doc.tar.gz"
        write_tar(missing_doc, {"pkg/docs/README.md": "package-safe docs\n"})
        expect_errors(missing_doc, allowlist, ["from package docs allowlist is missing"])

    print("package content validation self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
