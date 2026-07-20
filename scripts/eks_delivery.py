#!/usr/bin/env python3
"""Fail-closed local contract for EKS staging delivery.

The script intentionally knows no account IDs, cluster names, endpoints, or
credentials.  Make supplies explicit target values; the caller supplies short
lived AWS credentials through its normal environment/profile.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


SECRET_PATTERNS = (
    re.compile(r"(?i)(authorization|bearer|token|password|secret|apikey|api_key)\s*[:=]\s*[^\s,]+"),
    re.compile(r"AKIA[0-9A-Z]{16}"),
)
DIGEST = re.compile(r"^[a-z0-9][a-z0-9./:_-]*@sha256:[0-9a-f]{64}$")
NAME = re.compile(r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")


def scrub(value: str) -> str:
    result = value
    for pattern in SECRET_PATTERNS:
        result = pattern.sub("[REDACTED]", result)
    return result[:4000]


def fail(message: str) -> None:
    raise RuntimeError(message)


def command(args: list[str], env: dict[str, str], *, quiet: bool = False) -> str:
    proc = subprocess.run(args, env=env, text=True, capture_output=True)
    if proc.returncode:
        detail = scrub(proc.stderr or proc.stdout)
        fail(f"{args[0]} failed (exit {proc.returncode}): {detail}")
    return "" if quiet else scrub(proc.stdout)


class Delivery:
    def __init__(self, args: argparse.Namespace):
        self.args = args
        self._validate()
        self.evidence_dir = Path(args.evidence_dir).resolve()
        self.evidence_dir.mkdir(parents=True, exist_ok=True)
        self.evidence: dict[str, object] = {
            "timestamp": dt.datetime.now(dt.timezone.utc).isoformat(),
            "environment": args.environment,
            "aws_region": args.aws_region,
            "eks_cluster": args.eks_cluster,
            "k8s_namespace": args.k8s_namespace,
            "kustomize_overlay": args.kustomize_overlay,
            "image_digest": args.image_digest or None,
            "action": args.action,
            "events": [],
        }

    def _validate(self) -> None:
        required = {
            "AWS_REGION": self.args.aws_region,
            "EKS_CLUSTER": self.args.eks_cluster,
            "K8S_NAMESPACE": self.args.k8s_namespace,
            "KUSTOMIZE_OVERLAY": self.args.kustomize_overlay,
            "ENVIRONMENT": self.args.environment,
        }
        missing = [key for key, value in required.items() if not value]
        if missing:
            fail("missing required explicit input(s): " + ", ".join(missing))
        if self.args.environment != "staging":
            fail("only ENVIRONMENT=staging is supported; production mutation is unavailable")
        if not NAME.fullmatch(self.args.k8s_namespace):
            fail("K8S_NAMESPACE must be a DNS-label")
        overlay = Path(self.args.kustomize_overlay)
        if not overlay.is_dir() or not (overlay / "kustomization.yaml").is_file():
            fail("KUSTOMIZE_OVERLAY must be an existing kustomization directory")
        if self.args.action in {"render", "plan", "apply", "smoke"}:
            if not self.args.image_digest or not DIGEST.fullmatch(self.args.image_digest):
                fail("IMAGE_DIGEST must be a lower-case immutable image@sha256:<64 hex> reference")
        if self.args.action in {"apply", "rollback"} and self.args.confirm != "STAGING_APPLY":
            fail("mutating action requires EKS_CONFIRM=STAGING_APPLY")

    def event(self, name: str, **fields: object) -> None:
        self.evidence["events"].append({"name": name, **fields})

    def write_evidence(self, outcome: str, error: str | None = None) -> None:
        self.evidence["outcome"] = outcome
        if error:
            self.evidence["error"] = scrub(error)
        payload = json.dumps(self.evidence, indent=2, sort_keys=True) + "\n"
        (self.evidence_dir / "evidence.json").write_text(payload)
        markdown = [f"# EKS {self.args.action} evidence", "", f"Outcome: **{outcome}**", ""]
        for key in ("environment", "aws_region", "eks_cluster", "k8s_namespace", "image_digest"):
            markdown.append(f"- {key}: `{self.evidence.get(key)}`")
        if error:
            markdown.extend(["", "## Safe failure", "", "```text", scrub(error), "```"])
        (self.evidence_dir / "summary.md").write_text("\n".join(markdown) + "\n")

    def env_with_kubeconfig(self) -> tuple[dict[str, str], tempfile.TemporaryDirectory[str]]:
        temp = tempfile.TemporaryDirectory(prefix="smartrouter-eks-")
        env = os.environ.copy()
        env["KUBECONFIG"] = str(Path(temp.name) / "kubeconfig")
        return env, temp

    def preflight(self, env: dict[str, str]) -> None:
        for tool in ("aws", "kubectl", "kustomize"):
            if not shutil.which(tool):
                fail(f"required executable not found: {tool}")
        identity = command(["aws", "sts", "get-caller-identity", "--output", "json"], env)
        cluster = command(["aws", "eks", "describe-cluster", "--region", self.args.aws_region,
                           "--name", self.args.eks_cluster, "--query", "cluster.status", "--output", "text"], env)
        if cluster.strip() != "ACTIVE":
            fail("EKS cluster is not ACTIVE")
        command(["aws", "eks", "update-kubeconfig", "--region", self.args.aws_region,
                 "--name", self.args.eks_cluster, "--kubeconfig", env["KUBECONFIG"]], env, quiet=True)
        allowed = command(["kubectl", "auth", "can-i", "get", "pods", "-n", self.args.k8s_namespace], env).strip()
        if allowed != "yes":
            fail("Kubernetes RBAC does not allow get pods in the requested namespace")
        self.event("preflight", aws_identity=scrub(identity), cluster_status=cluster.strip(), rbac_get_pods=allowed)

    def render(self, env: dict[str, str]) -> Path:
        source_root = Path(self.args.kustomize_overlay).resolve().parents[2]
        with tempfile.TemporaryDirectory(prefix="smartrouter-kustomize-") as temporary:
            copied_root = Path(temporary) / source_root.name
            shutil.copytree(source_root, copied_root)
            relative_overlay = Path(self.args.kustomize_overlay).resolve().relative_to(source_root)
            copied_overlay = copied_root / relative_overlay
            # kustomize edit operates on the process working directory; run it only
            # in the copied overlay so a render cannot modify tracked manifests.
            proc = subprocess.run(["kustomize", "edit", "set", "image", f"smart-llmrouter={self.args.image_digest}"],
                                  cwd=copied_overlay, env=env, text=True, capture_output=True)
            if proc.returncode:
                fail(f"kustomize image override failed: {scrub(proc.stderr)}")
            rendered = command(["kustomize", "build", str(copied_overlay)], env)
        if self.args.image_digest not in rendered:
            fail("rendered manifest does not contain the requested immutable IMAGE_DIGEST")
        output = self.evidence_dir / "rendered.yaml"
        output.write_text(rendered)
        self.evidence["rendered_manifest_sha256"] = hashlib.sha256(rendered.encode()).hexdigest()
        self.event("render", artifact=str(output))
        return output

    def run(self) -> None:
        env, temp = self.env_with_kubeconfig()
        try:
            if self.args.action in {"preflight", "status", "plan", "apply", "rollback", "smoke"}:
                self.preflight(env)
            manifest = self.render(env) if self.args.action in {"render", "plan", "apply", "smoke"} else None
            if self.args.action == "status":
                status = command(["kubectl", "get", "deployment,pods,service,ingress,pdb,networkpolicy,pvc",
                                  "-n", self.args.k8s_namespace, "-o", "name"], env)
                self.event("rollout_status", resources=scrub(status))
            elif self.args.action == "plan":
                command(["kubectl", "apply", "--server-side", "--dry-run=server", "-f", str(manifest)], env, quiet=True)
                self.event("server_side_dry_run", result="passed")
            elif self.args.action == "apply":
                command(["kubectl", "apply", "--server-side", "-f", str(manifest)], env, quiet=True)
                command(["kubectl", "rollout", "status", "deployment/router", "-n", self.args.k8s_namespace, "--timeout=5m"], env, quiet=True)
                self.event("apply", result="passed")
            elif self.args.action == "rollback":
                command(["kubectl", "rollout", "undo", "deployment/router", "-n", self.args.k8s_namespace], env, quiet=True)
                command(["kubectl", "rollout", "status", "deployment/router", "-n", self.args.k8s_namespace, "--timeout=5m"], env, quiet=True)
                self.event("rollback", result="passed")
            elif self.args.action == "smoke":
                if not self.args.smoke_command:
                    fail("EKS_SMOKE_COMMAND is required and is never logged")
                proc = subprocess.run(self.args.smoke_command, shell=True, env=env, text=True, capture_output=True)
                if proc.returncode:
                    fail(f"protected staging smoke failed (exit {proc.returncode}): {scrub(proc.stderr)}")
                self.event("smoke", result="passed")
            elif self.args.action == "promotion-plan":
                evidence_file = self.evidence_dir / "evidence.json"
                if not evidence_file.is_file():
                    fail("staging evidence.json is required before a promotion plan can be produced")
                prior = json.loads(evidence_file.read_text())
                if prior.get("outcome") != "passed":
                    fail("staging evidence is not a passed result")
                self.event("promotion_plan", result="review_required_no_production_apply")
            self.write_evidence("passed")
        except Exception as exc:
            self.write_evidence("failed", str(exc))
            raise
        finally:
            temp.cleanup()


def parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=("preflight", "status", "render", "plan", "apply", "rollback", "smoke", "promotion-plan"))
    p.add_argument("--aws-region", required=True)
    p.add_argument("--eks-cluster", required=True)
    p.add_argument("--k8s-namespace", required=True)
    p.add_argument("--kustomize-overlay", required=True)
    p.add_argument("--environment", required=True)
    p.add_argument("--image-digest", default="")
    p.add_argument("--confirm", default="")
    p.add_argument("--evidence-dir", required=True)
    p.add_argument("--smoke-command", default="")
    return p


def main() -> int:
    args = parser().parse_args()
    try:
        Delivery(args).run()
        print(json.dumps({"action": args.action, "outcome": "passed", "evidence_dir": args.evidence_dir}))
        return 0
    except Exception as exc:
        print(f"EKS delivery contract failed: {scrub(str(exc))}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
