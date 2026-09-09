# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Offline training commands. Secrets are environment-only; errors omit input data."""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Any, NoReturn

import yaml


class SafeParser(argparse.ArgumentParser):
    def error(self, message: str) -> NoReturn:
        self.exit(2, "lrp: invalid_arguments; use --help for supported options\n")


def parser() -> argparse.ArgumentParser:
    root = SafeParser(prog="lrp", description="Metrum learned routing policy")
    sub = root.add_subparsers(dest="command", required=True)
    collect = sub.add_parser("collect", help="Normalize approved content; logs alone are metadata")
    for name in ("router-log", "content-capture", "dataset"):
        collect.add_argument(f"--{name}", type=Path)
    collect.add_argument(
        "--usage-db",
        help="Read-only SQLite path or PostgreSQL DSN for explore metadata joins",
    )
    collect.add_argument("--out", type=Path, required=True)
    collect.add_argument("--approved-content", action="store_true")

    usage = sub.add_parser(
        "import-usage",
        help="Export read-only explore metadata from usage DB (no prompts)",
    )
    usage.add_argument(
        "--db",
        required=True,
        help="SQLite path/URI or PostgreSQL DSN (opened read-only)",
    )
    usage.add_argument("--out", type=Path, required=True)
    usage.add_argument("--class-label", default="lrp:explore")
    usage.add_argument(
        "--content-ids",
        type=Path,
        help="Governed content JSONL; only matching explore request IDs are written",
    )

    fanout = sub.add_parser("fanout", help="Collect candidate responses in protected storage")
    fanout.add_argument("--requests", type=Path, required=True)
    fanout.add_argument("--targets", type=Path, required=True)
    fanout.add_argument("--out", type=Path, required=True)
    fanout.add_argument("--via", choices=("router", "openrouter"), default="router")
    fanout.add_argument("--base-url")
    fanout.add_argument("--refresh-pricing", action="store_true", default=True)
    fanout.add_argument("--approved-content", action="store_true")
    fanout.add_argument("--allow-uncapped", action="store_true")
    fanout.add_argument("--max-total-cost-usd", type=float)
    fanout.add_argument("--concurrency", type=int, choices=range(1, 5), default=4)

    judge = sub.add_parser("judge", help="Verify or send approved content to a third-party judge")
    for name in ("requests", "responses", "out"):
        judge.add_argument(f"--{name}", type=Path, required=True)
    judge.add_argument("--judge", default="anthropic/claude-sonnet-4.6")
    judge.add_argument("--anchor", required=True, help="Actual upstream model ID")
    judge.add_argument("--anchor-provider", default="openrouter")
    judge.add_argument("--approved-content", action="store_true")
    judge.add_argument("--sandbox-rootfs", type=Path)

    features = sub.add_parser("featurize")
    features.add_argument("--requests", type=Path, required=True)
    features.add_argument("--out", type=Path, required=True)
    features.add_argument("--embedding-model", type=Path)
    features.add_argument("--tokenizer", type=Path)
    features.add_argument("--synthetic", action="store_true", help="Wiring only; never promotable")
    features.add_argument("--seed", type=int, default=42)

    train = sub.add_parser("train")
    for name in ("features", "judgments", "responses", "out"):
        train.add_argument(f"--{name}", type=Path, required=True)
    train.add_argument("--embedding-model", type=Path)
    train.add_argument("--tokenizer", type=Path)
    train.add_argument("--synthetic", action="store_true")
    train.add_argument("--seed", type=int, default=42)
    train.add_argument("--anchor", required=True, help="Actual upstream model ID")
    train.add_argument("--anchor-provider", default="openrouter")

    evaluate = sub.add_parser("eval")
    for name in ("bundle", "features", "judgments", "responses", "config", "out"):
        evaluate.add_argument(f"--{name}", type=Path, required=True)
    evaluate.add_argument("--split", choices=("test",), default="test")
    evaluate.add_argument(
        "--synthetic", action="store_true", help="Evaluate gates without promotion"
    )

    serving = sub.add_parser("serve")
    serving.add_argument("--bundle", type=Path, required=True)
    serving.add_argument("--config", type=Path, required=True)
    serving.add_argument("--port", type=int, default=18093)
    serving.add_argument("--admin-port", type=int, default=18094)
    serving.add_argument("--enable-admin", action="store_true")
    serving.add_argument("--deadline-ms", type=int, help="Override YAML deadline_ms (1..4500; default 200)")
    validate = sub.add_parser("validate")
    validate.add_argument("--bundle", type=Path, required=True)
    return root


def embedding_spec(args: argparse.Namespace) -> dict[str, Any]:
    if args.synthetic:
        return {"kind": "synthetic"}
    if args.embedding_model is None or args.tokenizer is None:
        raise ValueError("explicit embedding model and tokenizer required")
    return {
        "kind": "onnx",
        "model_path": str(args.embedding_model),
        "tokenizer_path": str(args.tokenizer),
        "pooling": "cls",
    }


def execute(args: argparse.Namespace) -> int:
    from lrp.collect import DataError, protected_path

    if hasattr(args, "out"):
        args.out = protected_path(args.out)
        if args.command == "eval":
            for suffix in (".md", ".svg"):
                protected_path(args.out.with_suffix(suffix))

    def read_yaml(path: Path) -> Any:
        if path.is_symlink() or path.stat().st_size > 1_048_576:
            raise DataError("invalid_config_file")
        return yaml.safe_load(path.read_text())

    if args.command == "collect":
        from lrp.collect import collect

        stats = collect(
            out=args.out,
            dataset=args.dataset,
            content_capture=args.content_capture,
            router_log=args.router_log,
            usage_db=args.usage_db,
            approved_content=args.approved_content,
        )
        print(json.dumps({"counts": stats}))
    elif args.command == "import-usage":
        from lrp.usage_import import import_usage

        stats = import_usage(
            dsn=args.db,
            out=args.out,
            class_label=args.class_label,
            content_ids=args.content_ids,
        )
        print(json.dumps({"counts": stats}))
    elif args.command == "fanout":
        from lrp.fanout import run_fanout

        configured = read_yaml(args.targets)
        targets = configured["targets"] if isinstance(configured, dict) else configured
        stats = asyncio.run(
            run_fanout(
                requests=args.requests,
                out=args.out,
                targets=targets,
                via=args.via,
                base_url=args.base_url,
                refresh_pricing=args.refresh_pricing,
                approved_content=args.approved_content,
                allow_uncapped=args.allow_uncapped,
                    max_total_cost_usd=args.max_total_cost_usd,
                    concurrency=args.concurrency,
            )
        )
        print(json.dumps({"counts": stats}))
    elif args.command == "judge":
        from lrp.judge import judge_requests
        from lrp.judge.sandbox import Sandbox

        stats = asyncio.run(
            judge_requests(
                requests=args.requests,
                responses=args.responses,
                out=args.out,
                anchor=(args.anchor_provider, args.anchor),
                judge_model=args.judge,
                    approved_content=args.approved_content,
                    sandbox=Sandbox(args.sandbox_rootfs) if args.sandbox_rootfs else None,
            )
        )
        print(json.dumps({"counts": stats}))
    elif args.command == "featurize":
        from lrp.features import FeatureBuilder, ONNXEmbedder, SyntheticEmbedder, featurize

        spec = embedding_spec(args)
        embedder = (
            SyntheticEmbedder()
            if spec["kind"] == "synthetic"
            else ONNXEmbedder(Path(spec["model_path"]), Path(spec["tokenizer_path"]), threads=1)
        )
        featurize(args.requests, args.out, builder=FeatureBuilder(embedder), seed=args.seed)
    elif args.command == "train":
        from lrp.train import train

        produced = train(
            args.features,
            args.judgments,
            args.responses,
            args.out,
            embedding=embedding_spec(args),
            seed=args.seed,
            threads=1,
            min_train_rows=200,
            anchor=(args.anchor_provider, args.anchor),
        )
        print(json.dumps({"bundle_version": produced.name}))
    elif args.command == "eval":
        from lrp.bundle import load_bundle
        from lrp.eval import evaluate

        config = read_yaml(args.config)
        report = evaluate(
            load_bundle(args.bundle),
            args.features,
            args.judgments,
            args.responses,
            config,
            out=args.out,
        )
        if not args.synthetic and not report.get("promotion_passed", False):
            return 1
    elif args.command == "validate":
        from lrp.bundle import load_bundle

        load_bundle(args.bundle, threads=1)
    elif args.command == "serve":
        from lrp.serve import serve

        serve(
            bundle=args.bundle,
            config=args.config,
            port=args.port,
            admin_port=args.admin_port,
            enable_admin=args.enable_admin,
            deadline_ms=args.deadline_ms,
        )
    return 0


def main() -> int:
    from lrp.collect import DataError
    os.umask(0o077)
    args = parser().parse_args()
    try:
        result = execute(args)
    except KeyboardInterrupt:
        return 130
    except DataError as exc:
        print(f"lrp: {exc}", file=sys.stderr)
        return 1
    except Exception:  # noqa: BLE001 - content-safe boundary for third-party failures
        # Do not print exception messages: third-party validation and HTTP errors
        # can contain content, credentials, URLs or paths from protected inputs.
        print(
            "lrp: command_failed; check input schema, configuration and protected evidence",
            file=sys.stderr,
        )
        return 1
    if args.command != "serve":
        print(
            json.dumps({"command": args.command, "status": "ok" if result == 0 else "gates_failed"})
        )
    return result


if __name__ == "__main__":
    raise SystemExit(main())
