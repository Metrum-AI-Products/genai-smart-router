# Isolated offline verifiers

The serving image does not execute verifiers. Run the offline CLI on a dedicated
Linux worker with distro-provided `bubblewrap`, Python 3.12+, and permitted
unprivileged user, mount, network and PID namespaces. Do not grant a dataset
Docker access, host mounts, credentials, installation commands or shell hooks.
Do not solve namespace denial by disabling isolation or running privileged.
The subprocess receives only its verifier specification and candidate text.

The sandbox mounts a separate immutable root filesystem, clears the environment,
uses UID/GID 65534 and drops capabilities. `/work` is a new 16 MiB tmpfs. Each
worker has 30 seconds wall time, 30 seconds CPU, 256 MiB address space, 32
processes, 64 descriptors, 1 MiB per file and 4 KiB captured output limits.
Timeout cleanup kills the launch process group and its PID namespace.
For aggregate memory/CPU bounds across child processes, run the dedicated worker
under an operator-owned cgroup with enforced `MemoryMax`, `TasksMax` and CPU
limits; per-process resource limits do not provide a total-job memory bound.
Unsupported isolation or dependencies produce missing evidence (`quality: null`),
never a host execution fallback. A verifier pass/fail is a separate outcome from
pairwise or absolute LLM judging.

## Ubuntu 24.04 AppArmor prerequisite

Ubuntu 24.04 workers with restricted unprivileged user namespaces need the
distro-matched bubblewrap AppArmor profile before running verifiers. A successful
rootfs build does not establish this prerequisite. Ubuntu documents the
[bubblewrap profile and child confinement](https://discourse.ubuntu.com/t/understanding-apparmor-user-namespace-restriction/58007);
the [Noble `apparmor-profiles` package](https://packages.ubuntu.com/noble-updates/all/apparmor-profiles/filelist)
ships `bwrap-userns-restrict`. Use the packaged profile compatible with AppArmor
4.0, rather than copying a newer upstream profile with a different policy ABI.
The [tagged AppArmor 4.0 profile](https://gitlab.com/apparmor/apparmor/-/raw/apparmor-4.0/profiles/apparmor/profiles/extras/bwrap-userns-restrict)
documents the `bwrap` and stacked `unpriv_bwrap` profiles.

An administrator provisions the dedicated worker and reloads that exact profile:

```bash
rtk proxy sudo apt-get update
rtk proxy sudo apt-get install -y bubblewrap apparmor apparmor-profiles
rtk proxy sudo install -o root -g root -m 0644 \
  /usr/share/apparmor/extra-profiles/bwrap-userns-restrict \
  /etc/apparmor.d/lrp-ci-bwrap
rtk proxy sudo apparmor_parser -r /etc/apparmor.d/lrp-ci-bwrap
rtk proxy sudo grep -Fx 'bwrap (enforce)' /sys/kernel/security/apparmor/profiles
rtk proxy sudo grep -Fx 'unpriv_bwrap (enforce)' /sys/kernel/security/apparmor/profiles
```

Both enforcement checks must succeed. Keep the namespace restriction sysctls
enabled and retain all sandbox isolation flags. Ubuntu's bubblewrap 0.9.0
[supports `--size` and `--disable-userns`](https://github.com/containers/bubblewrap/blob/v0.9.0/bwrap.xml);
the latter requires the sandbox's explicit `--unshare-user` option. Run the
mandatory verifier gate below as the ordinary worker user, without `sudo` or
privileged containers. Namespace denial must fail the gate, never skip it.

## Build a separate rootfs artifact

Set `LRP_DATA_DIR` to a private mode-0700 directory outside **all** Git checkouts.
Use a newly named `LRP_VERIFIER_ROOTFS` directory inside it for each build.
The data directory may not contain symlink ancestors. Input files must be owned
by the operator, mode 0600 and single-link regular files.

Select and review a Python 3.12 slim base image pinned by its registry digest;
set `LRP_VERIFIER_BASE` to that full image reference. Review dependency/container
scan results. No base digest is assumed by the product. Resolve a complete wheel
lock once for that Python version and platform, review it, and retain it with the
base digest as protected build evidence:

```bash
umask 077
printf 'pytest==9.1.1\njsonschema==4.26.0\n' > "$LRP_DATA_DIR/verifier-requirements.in"
rtk proxy uv pip compile --python-version 3.12 --generate-hashes --no-emit-index-url \
  "$LRP_DATA_DIR/verifier-requirements.in" \
  --output-file "$LRP_DATA_DIR/verifier-requirements.lock"
rtk proxy bash services/learned-routing-policy/lrp/judge/build_rootfs.sh \
  --base-image "$LRP_VERIFIER_BASE" \
  --requirements-lock "$LRP_DATA_DIR/verifier-requirements.lock" \
  --platform linux/amd64 --out "$LRP_VERIFIER_ROOTFS"
```

Use `linux/arm64` on an ARM worker. The builder requires fully versioned,
SHA-256-hashed wheels from the public package index; it refuses arbitrary package
URLs and requirements-file indirection. Resolving dependencies is a build-time
operator action, never a dataset action. Subsequent builds reuse the same lock
and digest. The builder makes a temporary Dockerfile with a scratch export stage,
exports only the dependency rootfs, records all file hashes in `.lrp-rootfs.json`,
and publishes a new read-only directory. It never copies the product checkout,
datasets or operator credentials into the image. The serving Dockerfile remains
separate. `/usr/bin/python3` points at the image's Python interpreter, and both
pytest and jsonschema must be importable under isolated Python (`-I`).

Treat base-image and wheel updates as new rootfs versions requiring new scans and
the tests below. Do not modify a published rootfs in place. Retain the manifest
and emitted fingerprint with operator build evidence. The builder's success is
not evidence that the host supports the required sandbox.

## Required security preflight and CI

Use a fresh private pytest directory outside all checkouts. As the unprivileged
worker user, run the full data
tests with `LRP_REQUIRE_SANDBOX_TESTS=1`; this converts missing-rootfs skips into
failures and requires real exact/regex/schema/pytest verification, network and
host-file denial, scratch exhaustion and wall-time cleanup checks:

```bash
rtk proxy env LRP_TEST_ROOTFS="$LRP_VERIFIER_ROOTFS" LRP_REQUIRE_SANDBOX_TESTS=1 \
  uv run --project services/learned-routing-policy pytest \
  services/learned-routing-policy/tests/unit/test_collect.py \
  services/learned-routing-policy/tests/unit/test_fanout.py \
  services/learned-routing-policy/tests/unit/test_judge.py \
  --basetemp "$LRP_DATA_DIR/security-test-tmp" -q
```

The dedicated Linux CI job must provision namespace-capable workers, build the
reviewed digest/lock artifact, then run this exact command. Never count a skip or
unsupported result as passing mandatory security evidence. Local runs without a
rootfs may skip the real isolation tests and must report that limitation.
Passing this gate establishes the tested isolation behavior; it does not claim
a dependency or container vulnerability scan. Record those scan results separately.

The CLI exposes `--sandbox-rootfs`; invoke it explicitly:

```bash
rtk proxy uv run --project services/learned-routing-policy lrp judge \
  --requests "$LRP_DATA_DIR/requests.ndjson" \
  --responses "$LRP_DATA_DIR/responses.ndjson" \
  --anchor-provider openrouter --anchor "$LRP_ANCHOR_MODEL" \
  --judge "$LRP_JUDGE_MODEL" --approved-content \
  --sandbox-rootfs "$LRP_VERIFIER_ROOTFS" \
  --out "$LRP_DATA_DIR/judgments.ndjson"
```

An approved dataset with verifier kind `exact`, `regex`, `json_schema` or `pytest`
uses the sandbox before considering an LLM judge. Rows without a verifier may
send protected content to the chosen third-party judge and require separately
approved data handling. No credentials are forwarded to verifier processes.

Implementation references: [bubblewrap security model](https://github.com/containers/bubblewrap),
[bubblewrap command options](https://github.com/containers/bubblewrap/blob/main/bwrap.xml),
and [Docker filesystem exports](https://docs.docker.com/reference/cli/docker/buildx/build/).
