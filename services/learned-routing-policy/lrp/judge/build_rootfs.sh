#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
# Build only trusted, pinned dependencies. No dataset or verifier code is executed.
set -euo pipefail
exec python3 - "$@" <<'PY'
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tempfile

class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, "lrp-rootfs: invalid_arguments\n")

def checked(path):
    path = Path(os.path.abspath(path))
    if any(p.is_symlink() for p in (path, *path.parents)):
        raise ValueError("unsafe_path")
    if any((p / ".git").exists() for p in path.parents):
        raise ValueError("repository_storage_forbidden")
    parent = path.parent.stat()
    if parent.st_uid != os.getuid() or parent.st_mode & 0o077:
        raise ValueError("private_parent_required")
    return path

def manifest(root):
    files = []
    for path in sorted(root.rglob("*")):
        info = path.lstat()
        name = path.relative_to(root).as_posix()
        if path.is_symlink():
            files.append({"path": name, "type": "symlink", "target": os.readlink(path)})
        elif path.is_file():
            if info.st_nlink != 1:
                # Materialize ordinary base-image hardlinks before freezing modes.
                copied = path.with_name(path.name + ".lrp-copy")
                shutil.copyfile(path, copied)
                copied.replace(path)
            # Remove write, setuid and setgid bits even when inherited from the base.
            path.chmod((info.st_mode & 0o555) | 0o400)
            with path.open("rb") as stream:
                digest = hashlib.file_digest(stream, "sha256").hexdigest()
            files.append({"path": name, "type": "file", "mode": path.stat().st_mode & 0o777, "sha256": digest})
        elif path.is_dir():
            files.append({"path": name, "type": "directory"})
        else:
            raise ValueError("special_rootfs_file_forbidden")
    return files

def main():
    os.umask(0o077)
    parser = SafeParser(description="Build an immutable LRP verifier root filesystem")
    parser.add_argument("--base-image", required=True, help="Reviewed Python 3.12 slim image pinned by sha256 digest")
    parser.add_argument("--requirements-lock", type=Path, required=True, help="Reviewed hash-locked pytest/jsonschema wheels")
    parser.add_argument("--out", type=Path, required=True, help="New rootfs directory outside repositories")
    parser.add_argument("--platform", choices=("linux/amd64", "linux/arm64"), default="linux/amd64")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/:-]*@sha256:[a-f0-9]{64}", args.base_image):
        raise ValueError("digest_pinned_base_required")
    out, lock = checked(args.out), checked(args.requirements_lock)
    if out.exists():
        raise ValueError("new_output_required")
    info = lock.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_mode & 0o077 or info.st_size > 1024 * 1024:
        raise ValueError("private_bounded_lock_required")
    contents = lock.read_text()
    if "https://" in contents or "http://" in contents or "$" in contents:
        raise ValueError("only_public_index_locked_wheels_allowed")
    # Ignore generated comments, but forbid requirement indirection and pip flags.
    normalized = re.sub(r"\\\s*\n", " ", contents)
    names = set()
    for line in normalized.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        match = re.fullmatch(r"([A-Za-z0-9_.-]+)==([A-Za-z0-9.+!-]+)(?:\s+--hash=sha256:[a-f0-9]{64})+", line)
        if not match:
            raise ValueError("fully_pinned_hash_lock_required")
        names.add(match.group(1).lower())
    if not {"pytest", "jsonschema"}.issubset(names):
        raise ValueError("verifier_dependencies_required")
    with tempfile.TemporaryDirectory(prefix=".lrp-rootfs-build-", dir=out.parent) as temporary:
        context = Path(temporary)
        (context / "requirements.lock").write_text(contents)
        (context / "Dockerfile").write_text('''FROM @@BASE@@ AS verifier
COPY requirements.lock /build/requirements.lock
RUN python3 -m pip install --no-compile --no-cache-dir --only-binary=:all: --require-hashes -r /build/requirements.lock \\
 && ln -sfn /usr/local/bin/python3 /usr/bin/python3 \\
 && mkdir -p /work /proc /dev \\
 && rm -rf /build /home /root
FROM scratch AS verifier-rootfs
COPY --from=verifier / /
'''.replace("@@BASE@@", args.base_image))
        exported = context / "exported"
        subprocess.run(["docker", "buildx", "build", "--platform", args.platform,
                        "--target", "verifier-rootfs", "--output", "type=local,dest=" + str(exported),
                        str(context)], check=True)
        files = manifest(exported)
        record = {"schema_version": "lrp.verifier-rootfs.v1", "base_image": args.base_image,
                  "platform": args.platform, "requirements_sha256": hashlib.sha256(contents.encode()).hexdigest(),
                  "files": files}
        encoded = json.dumps(record, sort_keys=True, separators=(",", ":")) + "\n"
        fingerprint = hashlib.sha256(encoded.encode()).hexdigest()
        (exported / ".lrp-rootfs.json").write_text(encoded)
        (exported / ".lrp-rootfs.json").chmod(0o444)
        # Publish under a new immutable directory; no overwrite or root ownership required.
        for path in sorted(exported.rglob("*"), reverse=True):
            if path.is_dir() and not path.is_symlink():
                path.chmod(0o555)
        # Moving a directory across parents requires owner write access for '..'.
        exported.chmod(0o755)
        exported.rename(out)
        out.chmod(0o555)
        print(json.dumps({"status": "built", "rootfs_sha256": fingerprint, "security_preflight": "required"}))

try:
    main()
except ValueError as error:
    codes = {"unsafe_path", "repository_storage_forbidden", "private_parent_required",
             "special_rootfs_file_forbidden", "digest_pinned_base_required", "new_output_required",
             "private_bounded_lock_required", "only_public_index_locked_wheels_allowed",
             "fully_pinned_hash_lock_required", "verifier_dependencies_required"}
    code = str(error) if str(error) in codes else "invalid_build_input"
    raise SystemExit("lrp-rootfs: " + code) from None
except OSError as error:
    raise SystemExit("lrp-rootfs: filesystem_error_" + str(error.errno)) from None
except subprocess.SubprocessError:
    raise SystemExit("lrp-rootfs: build_failed; verify arguments, Docker availability and locked dependencies") from None
PY
