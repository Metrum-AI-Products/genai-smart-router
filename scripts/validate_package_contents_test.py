#!/usr/bin/env python3
"""Self-test package content validation rules."""

from __future__ import annotations

import io
import json
import tarfile
import tempfile
from pathlib import Path

import validate_package_contents

PACKAGE_DOCS = [
    "PACKAGE_README.md",
    "BINARY_INSTALL.md",
    "DOCKER_COMPOSE_INSTALL.md",
    "KUBERNETES_INSTALL.md",
    "PACKAGE_VALIDATION.md",
    "solution-brief.md",
]


def write_allowlist(root: Path) -> Path:
    allowlist = root / "allowlist.txt"
    allowlist.write_text("".join(f"docs/{doc}\n" for doc in PACKAGE_DOCS), encoding="utf-8")
    return allowlist


def elf(machine: int) -> bytes:
    header = bytearray(64)
    header[:4] = b"\x7fELF"
    header[4] = 2
    header[5] = 1
    header[18:20] = machine.to_bytes(2, "little")
    return bytes(header)


def write_tar(path: Path, files: dict[str, str | bytes]) -> None:
    with tarfile.open(path, "w:gz") as package:
        for name, content in files.items():
            data = content if isinstance(content, bytes) else content.encode("utf-8")
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


def binary_package_files(root: str = "smart-llmrouter-v1.0.0-linux-amd64") -> dict[str, str | bytes]:
    files: dict[str, str | bytes] = {
        f"{root}/bin/router": elf(62),
        f"{root}/bin/router-token-gen": elf(62),
        f"{root}/bin/router-usage-report": elf(62),
        f"{root}/bin/metrum-smartrouterctl": elf(62),
        f"{root}/config/config.example.yaml": "server: {}\n",
        f"{root}/config/env.example.json": "{}\n",
        f"{root}/config/scripts/router.ts": "export function route() {}\n",
        f"{root}/caddy/Caddyfile": ":80\n",
    }
    for doc in PACKAGE_DOCS:
        files[f"{root}/docs/{doc}"] = "package-safe docs\n"
    return files


def docker_image_tar(extra_layer_files: dict[str, str | bytes] | None = None) -> bytes:
    layer_data = io.BytesIO()
    with tarfile.open(fileobj=layer_data, mode="w") as layer:
        for name in ["app/bin/router", "app/bin/router-token-gen", "app/bin/router-usage-report"]:
            data = elf(62)
            info = tarfile.TarInfo(name)
            info.size = len(data)
            layer.addfile(info, io.BytesIO(data))
        for name, content in (extra_layer_files or {}).items():
            data = content if isinstance(content, bytes) else content.encode("utf-8")
            info = tarfile.TarInfo(name)
            info.size = len(data)
            layer.addfile(info, io.BytesIO(data))
    layer_blob = layer_data.getvalue()

    image_data = io.BytesIO()
    with tarfile.open(fileobj=image_data, mode="w") as image:
        manifest = [{"Config": "config.json", "RepoTags": ["smart-llmrouter:v1.0.0-linux-amd64"], "Layers": ["layer.tar"]}]
        config = {"architecture": "amd64", "os": "linux"}
        for name, content in {
            "manifest.json": json.dumps(manifest).encode("utf-8"),
            "config.json": json.dumps(config).encode("utf-8"),
            "layer.tar": layer_blob,
        }.items():
            info = tarfile.TarInfo(name)
            info.size = len(content)
            image.addfile(info, io.BytesIO(content))
    return image_data.getvalue()


def docker_package_files(root: str = "smart-llmrouter-v1.0.0-docker-linux-amd64") -> dict[str, str | bytes]:
    files: dict[str, str | bytes] = {
        f"{root}/compose/docker-compose.yml": "services: {}\n",
        f"{root}/compose/docker-compose.postgres-localhost.yml": "services: {}\n",
        f"{root}/compose/Caddyfile.compose": ":80\n",
        f"{root}/compose/.env.example": "SMART_LLMROUTER_VERSION=v1.0.0-linux-amd64\n",
        f"{root}/compose/.env": "SMART_LLMROUTER_VERSION=v1.0.0-linux-amd64\nPOSTGRES_PASSWORD=replace-me\n",
        f"{root}/config/config.example.yaml": "server: {}\n",
        f"{root}/config/env.example.json": "{}\n",
        f"{root}/config/scripts/router.ts": "export function route() {}\n",
        f"{root}/images/smart-llmrouter-v1.0.0-linux-amd64.tar": docker_image_tar(),
    }
    for doc in PACKAGE_DOCS:
        files[f"{root}/docs/{doc}"] = "package-safe docs\n"
    return files


def main() -> int:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        allowlist = write_allowlist(root)

        good = root / "smart-llmrouter-v1.0.0-linux-amd64.tar.gz"
        write_tar(good, binary_package_files())
        expect_ok(good, allowlist)

        good_docker = root / "smart-llmrouter-v1.0.0-docker-linux-amd64.tar.gz"
        write_tar(good_docker, docker_package_files())
        expect_ok(good_docker, allowlist)

        extra_image = root / "extra-image.tar.gz"
        extra_image_files = docker_package_files()
        extra_image_files["smart-llmrouter-v1.0.0-docker-linux-amd64/images/extra.tar"] = docker_image_tar()
        write_tar(extra_image, extra_image_files)
        expect_errors(extra_image, allowlist, ["unexpected package file included"])

        image_source_path = root / "image-source-path.tar.gz"
        image_source_files = docker_package_files()
        image_source_files["smart-llmrouter-v1.0.0-docker-linux-amd64/images/smart-llmrouter-v1.0.0-linux-amd64.tar"] = (
            docker_image_tar({"app/docs/PRODUCTION_RUNBOOK.md": "private\n"})
        )
        write_tar(image_source_path, image_source_files)
        expect_errors(image_source_path, allowlist, ["forbidden runtime/source path"])

        image_source_dot_path = root / "image-source-dot-path.tar.gz"
        image_source_dot_files = docker_package_files()
        image_source_dot_files["smart-llmrouter-v1.0.0-docker-linux-amd64/images/smart-llmrouter-v1.0.0-linux-amd64.tar"] = (
            docker_image_tar({"./app/internal/router/secret.go": "private\n"})
        )
        write_tar(image_source_dot_path, image_source_dot_files)
        expect_errors(image_source_dot_path, allowlist, ["forbidden runtime/source path"])

        apple_double = root / "appledouble.tar.gz"
        apple_files = binary_package_files()
        apple_files["smart-llmrouter-v1.0.0-linux-amd64/docs/._PACKAGE_README.md"] = "mac metadata\n"
        write_tar(apple_double, apple_files)
        expect_errors(apple_double, allowlist, ["AppleDouble metadata entry"])

        private_runbook = root / "private-runbook.tar.gz"
        runbook_files = binary_package_files()
        runbook_files["smart-llmrouter-v1.0.0-linux-amd64/docs/PRODUCTION_RUNBOOK.md"] = "private\n"
        write_tar(private_runbook, runbook_files)
        expect_errors(private_runbook, allowlist, ["forbidden local secret/state file", "not in package docs allowlist"])

        private_marker = root / "private-marker.tar.gz"
        marker_files = binary_package_files()
        marker_files["smart-llmrouter-v1.0.0-linux-amd64/docs/PACKAGE_README.md"] = "Host: 100.30.225.66\n"
        write_tar(private_marker, marker_files)
        expect_errors(private_marker, allowlist, ["private production host marker"])

        raw_token = root / "raw-token.tar.gz"
        token_files = binary_package_files()
        token_files["smart-llmrouter-v1.0.0-linux-amd64/docs/PACKAGE_README.md"] = (
            "token rtr_metrum_user_project_prod_key_abcdefghijklmnopqrstuvwxyz\n"
        )
        write_tar(raw_token, token_files)
        expect_errors(raw_token, allowlist, ["raw router token"])

        forbidden_files = root / "forbidden-files.tar.gz"
        bad_files = binary_package_files()
        bad_files.update(
            {
                "smart-llmrouter-v1.0.0-linux-amd64/config/env.json": "{}\n",
                "smart-llmrouter-v1.0.0-linux-amd64/config/config.production.yaml": "server: {}\n",
                "smart-llmrouter-v1.0.0-linux-amd64/ROUTER_TOKEN.txt": "placeholder\n",
                "smart-llmrouter-v1.0.0-linux-amd64/config/license.json": "{}\n",
                "smart-llmrouter-v1.0.0-linux-amd64/state/usage.sqlite": "not actually sqlite\n",
            }
        )
        write_tar(forbidden_files, bad_files)
        expect_errors(forbidden_files, allowlist, ["forbidden local secret/state file"])

        wrong_arch = root / "smart-llmrouter-v1.0.0-linux-arm64.tar.gz"
        write_tar(wrong_arch, binary_package_files("smart-llmrouter-v1.0.0-linux-arm64"))
        expect_errors(wrong_arch, allowlist, ["expected 183 for linux-arm64"])

        unexpected = root / "unexpected.tar.gz"
        unexpected_files = binary_package_files()
        unexpected_files["smart-llmrouter-v1.0.0-linux-amd64/docs-site/source.md"] = "source\n"
        write_tar(unexpected, unexpected_files)
        expect_errors(unexpected, allowlist, ["unexpected package file included"])

        missing_doc = root / "missing-doc.tar.gz"
        missing_files = binary_package_files()
        del missing_files["smart-llmrouter-v1.0.0-linux-amd64/docs/PACKAGE_VALIDATION.md"]
        write_tar(missing_doc, missing_files)
        expect_errors(missing_doc, allowlist, ["from package docs allowlist is missing"])

    print("package content validation self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
