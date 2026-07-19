#!/usr/bin/env -S uv run python
"""Call Gemini Interactions API for Nano Banana image generation/editing."""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
import shutil
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path


DEFAULT_MODEL = "gemini-3-pro-image"
API_BASE = "https://generativelanguage.googleapis.com"
SCRIPT_DIR = Path(__file__).resolve().parent
SKILL_DIR = SCRIPT_DIR.parent
DEFAULT_BRAND_ASSET = SKILL_DIR / "assets" / "metrum-logo-white.png"
METRUM_STYLE_PROMPT = (
    "Apply the Metrum AI brand system: near-black background, white primary text, "
    "restrained gray secondary text, red-to-blue gradient accents "
    "(#FF3132 → #FE005F → #EE0089 → #CC28AF → #9948CB → #465CDA), "
    "Geist display typography, Geist Mono technical labels, thin technical rules, "
    "subtle grid/topology cues, square or minimally rounded UI elements, "
    "deliberate empty space. Avoid glossy 3D, cartoon illustration, "
    "rounded consumer-app cards, and generic template styles."
)
MODEL_PRESETS = {
    "pro": "gemini-3-pro-image",
    "flash": "gemini-3.1-flash-image",
    "lite": "gemini-3.1-flash-lite-image",
    "legacy": "gemini-2.5-flash-image",
}
IMAGE_SIZES_BY_MODEL = {
    "gemini-3-pro-image": {"1K", "2K", "4K"},
    "gemini-3.1-flash-image": {"512", "1K", "2K", "4K"},
    "gemini-3.1-flash-lite-image": {"1K"},
}
REFERENCE_LIMITS = {
    "gemini-3-pro-image": {"total": 14, "object": 6, "character": 5, "style": 3},
    "gemini-3.1-flash-image": {"total": 14, "object": 10, "character": 4, "style": 0},
    "gemini-3.1-flash-lite-image": {"total": 14, "object": 14, "character": 0, "style": 0},
}
ROLE_INSTRUCTIONS = {
    "object": "Use the following reference image for high-fidelity object details.",
    "character": "Use the following reference image to preserve character identity and consistency.",
    "style": "Use the following reference image only as a style reference.",
}


def parse_json(value: str, label: str):
    try:
        return json.loads(value)
    except json.JSONDecodeError as exc:
        raise SystemExit(f"Invalid JSON for {label}: {exc}") from exc


def merge_dict(base: dict, extra: dict) -> dict:
    for key, value in extra.items():
        if isinstance(value, dict) and isinstance(base.get(key), dict):
            base[key] = merge_dict(base[key], value)
        else:
            base[key] = value
    return base


def image_block(path: str) -> dict:
    image_path = Path(path).expanduser()
    if not image_path.exists():
        raise SystemExit(f"Image not found: {image_path}")
    mime_type = mimetypes.guess_type(image_path.name)[0] or "image/png"
    with image_path.open("rb") as handle:
        encoded = base64.b64encode(handle.read()).decode("utf-8")
    return {"type": "image", "data": encoded, "mime_type": mime_type}


def strip_video_audio(path: Path) -> Path:
    ffmpeg = shutil.which("ffmpeg")
    if not ffmpeg:
        raise SystemExit("ffmpeg is required for --strip-video-audio")
    output_path = path.with_name(f"{path.stem}-no-audio{path.suffix}")
    command = [
        ffmpeg,
        "-y",
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        str(path),
        "-an",
        "-c:v",
        "copy",
        str(output_path),
    ]
    try:
        subprocess.run(command, check=True)
    except subprocess.CalledProcessError as exc:
        raise SystemExit(f"ffmpeg failed to strip audio from {path}") from exc
    return output_path


def media_block(
    path: str,
    block_type: str,
    default_mime_type: str,
    strip_audio: bool = False,
) -> dict:
    media_path = Path(path).expanduser()
    if not media_path.exists():
        raise SystemExit(f"{block_type.title()} not found: {media_path}")
    if block_type == "video" and strip_audio:
        media_path = strip_video_audio(media_path)
    mime_type = mimetypes.guess_type(media_path.name)[0] or default_mime_type
    with media_path.open("rb") as handle:
        encoded = base64.b64encode(handle.read()).decode("utf-8")
    return {"type": block_type, "data": encoded, "mime_type": mime_type}


def uri_video_block(uri: str, mime_type: str | None) -> dict:
    guessed = mimetypes.guess_type(uri)[0]
    return {"type": "video", "uri": uri, "mime_type": mime_type or guessed or "video/mp4"}


def role_image_blocks(role: str, paths: list[str]) -> list[dict]:
    blocks: list[dict] = []
    for path in paths:
        blocks.append({"type": "text", "text": ROLE_INSTRUCTIONS[role]})
        blocks.append(image_block(path))
    return blocks


def split_csv(value: str | None) -> list[str]:
    if not value:
        return []
    return [item.strip() for item in value.split(",") if item.strip()]


def collect_key(value, key: str) -> list:
    found = []
    if isinstance(value, dict):
        for item_key, item_value in value.items():
            if item_key == key:
                found.append(item_value)
            found.extend(collect_key(item_value, key))
    elif isinstance(value, list):
        for item in value:
            found.extend(collect_key(item, key))
    return found


def effective_model(args: argparse.Namespace) -> str:
    if args.model_preset:
        return MODEL_PRESETS[args.model_preset]
    return args.model


def validate_args(args: argparse.Namespace, image_json_blocks: list[dict]) -> None:
    model = effective_model(args)

    if args.image_size:
        allowed_sizes = IMAGE_SIZES_BY_MODEL.get(model)
        if allowed_sizes and args.image_size not in allowed_sizes:
            allowed = ", ".join(sorted(allowed_sizes))
            raise SystemExit(f"{model} supports image sizes: {allowed}")
        if args.image_size.endswith("k"):
            raise SystemExit("Use uppercase image sizes such as 1K, 2K, or 4K")

    brand_style_images = resolve_brand_style_images(args)
    brand_object_images = resolve_brand_object_images(args)
    role_counts = {
        "object": len(args.object_image) + len(brand_object_images),
        "character": len(args.character_image),
        "style": len(args.style_image) + len(brand_style_images),
    }
    total_references = (
        len(args.image)
        + sum(role_counts.values())
        + sum(1 for block in image_json_blocks if block.get("type") == "image")
    )
    limits = REFERENCE_LIMITS.get(model)
    if limits:
        if total_references > limits["total"]:
            raise SystemExit(f"{model} supports up to {limits['total']} reference images")
        for role, count in role_counts.items():
            if count > limits[role]:
                raise SystemExit(f"{model} supports up to {limits[role]} {role} reference images")

    search_types = split_csv(args.search_types)
    if args.google_search and model == "gemini-3.1-flash-lite-image":
        raise SystemExit("gemini-3.1-flash-lite-image does not support Google Search grounding")
    if "image_search" in search_types and model != "gemini-3.1-flash-image":
        raise SystemExit("image_search grounding is only supported with gemini-3.1-flash-image")

    if args.thinking_level and model == "gemini-3.1-flash-image":
        if args.thinking_level not in {"minimal", "high"}:
            raise SystemExit("gemini-3.1-flash-image supports thinking_level minimal or high")


def resolve_brand_style_images(args: argparse.Namespace) -> list[str]:
    """Return style-reference paths implied by --brand.

    - brand=metrum (default): inject the bundled Metrum white logo unless the
      caller already passed --style-image or --no-brand-style.
    - brand=none: no automatic brand reference.
    - brand=<path>: inject that path as a style reference.
    """
    if getattr(args, "no_brand_style", False):
        return []
    brand = (getattr(args, "brand", None) or "metrum").strip()
    if brand.lower() in {"none", "off", "false", "0"}:
        return []
    if brand.lower() == "metrum":
        # Only auto-inject when the caller did not already supply style refs.
        if args.style_image:
            return []
        if DEFAULT_BRAND_ASSET.exists():
            return [str(DEFAULT_BRAND_ASSET)]
        print(
            f"warning: default brand asset not found at {DEFAULT_BRAND_ASSET}; "
            "pass --brand /path/to/logo.png or --style-image PATH",
            file=sys.stderr,
        )
        return []
    brand_path = Path(brand).expanduser()
    if not brand_path.exists():
        raise SystemExit(f"Brand asset not found: {brand_path}")
    return [str(brand_path)]


def resolve_brand_object_images(args: argparse.Namespace) -> list[str]:
    """Return object-reference paths implied by --brand.

    - brand=metrum (default): inject the bundled Metrum white logo as a
      high-fidelity object reference unless the caller already passed
      --object-image or --no-brand-style.
    - brand=none: no automatic brand reference.
    - brand=<path>: inject that path as an object reference.
    """
    if getattr(args, "no_brand_style", False):
        return []
    brand = (getattr(args, "brand", None) or "metrum").strip()
    if brand.lower() in {"none", "off", "false", "0"}:
        return []
    if brand.lower() == "metrum":
        # Only auto-inject when the caller did not already supply object refs.
        if args.object_image:
            return []
        if DEFAULT_BRAND_ASSET.exists():
            return [str(DEFAULT_BRAND_ASSET)]
        print(
            f"warning: default brand asset not found at {DEFAULT_BRAND_ASSET}; "
            "pass --brand /path/to/logo.png or --object-image PATH",
            file=sys.stderr,
        )
        return []
    brand_path = Path(brand).expanduser()
    if not brand_path.exists():
        raise SystemExit(f"Brand asset not found: {brand_path}")
    return [str(brand_path)]


def resolve_brand_style_prompt(args: argparse.Namespace) -> str:
    """Return the Metrum style prompt to prepend when --brand=metrum."""
    if getattr(args, "no_brand_style", False):
        return ""
    brand = (getattr(args, "brand", None) or "metrum").strip()
    if brand.lower() in {"none", "off", "false", "0"}:
        return ""
    if brand.lower() == "metrum":
        return METRUM_STYLE_PROMPT
    return ""


def build_request(args: argparse.Namespace) -> dict:
    if args.request_json:
        request = parse_json(args.request_json, "--request-json")
        if not isinstance(request, dict):
            raise SystemExit("--request-json must be a JSON object")
    else:
        if not args.prompt and not args.prompt_file:
            raise SystemExit("Provide --prompt, --prompt-file, or --request-json")

        prompt = args.prompt or Path(args.prompt_file).read_text(encoding="utf-8")
        brand_style_prompt = resolve_brand_style_prompt(args)
        if brand_style_prompt:
            prompt = f"{brand_style_prompt}\n\n{prompt}"
        image_json_blocks = [parse_json(item, "--image-json") for item in args.image_json]
        video_json_blocks = [parse_json(item, "--video-json") for item in args.video_json]
        validate_args(args, image_json_blocks)

        inputs: list[dict] = []
        inputs.extend(
            media_block(path, "video", "video/mp4", args.strip_video_audio)
            for path in args.video
        )
        inputs.extend(uri_video_block(uri, args.video_mime_type) for uri in args.video_uri)
        inputs.extend(video_json_blocks)
        inputs.append({"type": "text", "text": prompt})
        inputs.extend(image_block(path) for path in args.image)
        brand_object_images = resolve_brand_object_images(args)
        inputs.extend(role_image_blocks("object", brand_object_images + list(args.object_image)))
        inputs.extend(role_image_blocks("character", args.character_image))
        brand_style_images = resolve_brand_style_images(args)
        inputs.extend(role_image_blocks("style", brand_style_images + list(args.style_image)))
        inputs.extend(image_json_blocks)

        request = {
            "model": effective_model(args),
            "input": inputs if len(inputs) > 1 else prompt,
        }

        if args.previous_interaction_id:
            request["previous_interaction_id"] = args.previous_interaction_id
        if args.system_instruction:
            request["system_instruction"] = args.system_instruction

        if args.response_format_json:
            request["response_format"] = parse_json(
                args.response_format_json, "--response-format-json"
            )
        elif any([args.aspect_ratio, args.image_size, args.mime_type]):
            response_format = {"type": "image"}
            if args.mime_type:
                response_format["mime_type"] = args.mime_type
            if args.aspect_ratio:
                response_format["aspect_ratio"] = args.aspect_ratio
            if args.image_size:
                response_format["image_size"] = args.image_size
            request["response_format"] = response_format

        if args.response_modalities:
            request["response_modalities"] = [
                item.strip() for item in args.response_modalities.split(",") if item.strip()
            ]

        generation_config = {}
        if args.generation_config_json:
            generation_config = parse_json(
                args.generation_config_json, "--generation-config-json"
            )
            if not isinstance(generation_config, dict):
                raise SystemExit("--generation-config-json must be a JSON object")
        if args.thinking_level:
            generation_config["thinking_level"] = args.thinking_level
        if args.max_output_tokens is not None:
            generation_config["max_output_tokens"] = args.max_output_tokens
        if args.seed is not None:
            generation_config["seed"] = args.seed
        if args.temperature is not None:
            generation_config["temperature"] = args.temperature
        if generation_config:
            request["generation_config"] = generation_config

        if args.tools_json:
            request["tools"] = parse_json(args.tools_json, "--tools-json")
        elif args.google_search:
            tool = {"type": "google_search"}
            if args.search_types:
                tool["search_types"] = split_csv(args.search_types)
            request["tools"] = [tool]

    if args.extra_json:
        extra = parse_json(args.extra_json, "--extra-json")
        if not isinstance(extra, dict):
            raise SystemExit("--extra-json must be a JSON object")
        request = merge_dict(request, extra)

    return request


def request_interaction(body: dict, api_key: str, api_version: str) -> dict:
    url = f"{API_BASE}/{api_version}/interactions"
    data = json.dumps(body).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "x-goog-api-key": api_key,
            "Content-Type": "application/json",
            "User-Agent": "codex-nabapro-skill/1.0",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=300) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        if "Audio input modality is not enabled for this model" in detail:
            detail += (
                "\n\nVideo input note: gemini-3.1-flash-image accepts video frames for "
                "video-to-image, but videos with audio tracks can be rejected by the "
                "Gemini API. For local videos, retry with --strip-video-audio. Public "
                "YouTube URLs may fail when the API attempts to process audio; use a "
                "local silent/no-audio video or Files API video URI instead."
            )
        raise SystemExit(f"Gemini API HTTP {exc.code}: {detail}") from exc
    except urllib.error.URLError as exc:
        raise SystemExit(f"Gemini API request failed: {exc}") from exc


def content_blocks(response: dict):
    for step in response.get("steps") or []:
        for block in step.get("content") or []:
            yield block
    for key in ("output_image", "output_text"):
        value = response.get(key)
        if isinstance(value, dict):
            yield value


def save_outputs(response: dict, output_dir: Path, basename: str) -> list[Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    saved: list[Path] = []
    image_count = 0
    text_count = 0
    thought_image_count = 0
    thought_text_count = 0
    search_result_count = 0

    for block in content_blocks(response):
        block_type = block.get("type")
        if block_type == "image":
            image_count += 1
            mime_type = block.get("mime_type") or "image/jpeg"
            ext = ".jpg" if mime_type == "image/jpeg" else mimetypes.guess_extension(mime_type) or ".img"
            if block.get("data"):
                path = output_dir / f"{basename}-{image_count}{ext}"
                path.write_bytes(base64.b64decode(block["data"]))
                saved.append(path)
            elif block.get("uri"):
                path = output_dir / f"{basename}-{image_count}.uri.txt"
                path.write_text(block["uri"], encoding="utf-8")
                saved.append(path)
        elif block_type == "text" and block.get("text"):
            text_count += 1
            path = output_dir / f"{basename}-text-{text_count}.txt"
            path.write_text(block["text"], encoding="utf-8")
            saved.append(path)

    for step in response.get("steps") or []:
        step_type = step.get("type")
        if step_type == "thought":
            for block in step.get("summary") or []:
                block_type = block.get("type")
                if block_type == "image" and block.get("data"):
                    thought_image_count += 1
                    mime_type = block.get("mime_type") or "image/jpeg"
                    ext = ".jpg" if mime_type == "image/jpeg" else mimetypes.guess_extension(mime_type) or ".img"
                    path = output_dir / f"{basename}-thought-{thought_image_count}{ext}"
                    path.write_bytes(base64.b64decode(block["data"]))
                    saved.append(path)
                elif block_type == "text" and block.get("text"):
                    thought_text_count += 1
                    path = output_dir / f"{basename}-thought-text-{thought_text_count}.txt"
                    path.write_text(block["text"], encoding="utf-8")
                    saved.append(path)
        elif step_type == "google_search_result":
            search_result_count += 1
            path = output_dir / f"{basename}-google-search-result-{search_result_count}.json"
            path.write_text(json.dumps(step, indent=2, sort_keys=True), encoding="utf-8")
            saved.append(path)

    search_suggestions = collect_key(response, "search_suggestions")
    if search_suggestions:
        path = output_dir / f"{basename}-search-suggestions.json"
        path.write_text(json.dumps(search_suggestions, indent=2, sort_keys=True), encoding="utf-8")
        saved.append(path)

    url_citations = collect_key(response, "url_citation")
    if url_citations:
        path = output_dir / f"{basename}-url-citations.json"
        path.write_text(json.dumps(url_citations, indent=2, sort_keys=True), encoding="utf-8")
        saved.append(path)

    metadata_path = output_dir / f"{basename}-interaction.json"
    metadata_path.write_text(json.dumps(response, indent=2, sort_keys=True), encoding="utf-8")
    saved.append(metadata_path)
    return saved


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate or edit images with Nano Banana Pro via Gemini Interactions API."
    )
    parser.add_argument("--prompt", help="Text prompt.")
    parser.add_argument("--prompt-file", help="Read prompt from a UTF-8 text file.")
    parser.add_argument("--model", default=DEFAULT_MODEL, help=f"Model ID. Default: {DEFAULT_MODEL}")
    parser.add_argument("--model-preset", choices=sorted(MODEL_PRESETS), help="Model preset: pro, flash, lite, or legacy.")
    parser.add_argument("--image", action="append", default=[], help="Local reference/edit image. Repeatable.")
    parser.add_argument("--object-image", action="append", default=[], help="Object fidelity reference image. Repeatable.")
    parser.add_argument("--character-image", action="append", default=[], help="Character consistency reference image. Repeatable.")
    parser.add_argument("--style-image", action="append", default=[], help="Style reference image. Repeatable.")
    parser.add_argument("--image-json", action="append", default=[], help="Exact JSON image content block. Repeatable.")
    parser.add_argument("--brand", default="metrum", help="Brand preset. 'metrum' injects the bundled Metrum logo as a style reference. 'none' disables. Pass a path to use a custom logo. Default: metrum.")
    parser.add_argument("--no-brand-style", action="store_true", help="Skip injecting the default brand style reference even when --brand is set.")
    parser.add_argument("--video", action="append", default=[], help="Local video reference. Repeatable; use Files API for large videos.")
    parser.add_argument("--strip-video-audio", action="store_true", help="Use ffmpeg to remove audio from local --video inputs before upload.")
    parser.add_argument("--video-uri", action="append", default=[], help="Public video URI such as a YouTube URL. Repeatable.")
    parser.add_argument("--video-mime-type", help="MIME type for --video-uri. Default: inferred or video/mp4.")
    parser.add_argument("--video-json", action="append", default=[], help="Exact JSON video content block. Repeatable.")
    parser.add_argument("--previous-interaction-id", help="Continue a previous interaction.")
    parser.add_argument("--system-instruction", help="Optional system instruction.")
    parser.add_argument("--aspect-ratio", help="Image aspect ratio, e.g. 1:1, 16:9, 4:5, 21:9.")
    parser.add_argument("--image-size", help="Image size: 512, 1K, 2K, or 4K.")
    parser.add_argument("--mime-type", default="image/jpeg", help="Output MIME type. Default: image/jpeg.")
    parser.add_argument("--response-format-json", help="Exact response_format JSON object or array.")
    parser.add_argument("--response-modalities", help="Comma-separated response modalities, e.g. text,image.")
    parser.add_argument("--generation-config-json", help="Exact generation_config JSON object.")
    parser.add_argument("--thinking-level", choices=["minimal", "low", "medium", "high"])
    parser.add_argument("--max-output-tokens", type=int)
    parser.add_argument("--seed", type=int)
    parser.add_argument("--temperature", type=float)
    parser.add_argument("--google-search", action="store_true", help="Enable Google Search grounding.")
    parser.add_argument("--search-types", help="Comma-separated search types, e.g. web_search,image_search.")
    parser.add_argument("--tools-json", help="Exact tools JSON array.")
    parser.add_argument("--request-json", help="Full Interactions request body JSON.")
    parser.add_argument("--extra-json", help="Extra top-level request fields to merge into the body.")
    parser.add_argument("--api-version", default="v1beta", help="API version path. Default: v1beta.")
    parser.add_argument("--output-dir", default="nabapro-output", help="Directory for saved outputs.")
    parser.add_argument("--basename", default="nabapro", help="Output filename prefix.")
    parser.add_argument("--dry-run", action="store_true", help="Print request JSON without calling the API.")
    args = parser.parse_args()

    body = build_request(args)
    if args.dry_run:
        print(json.dumps(body, indent=2, sort_keys=True))
        return 0

    api_key = os.environ.get("GEMINI_API_KEY")
    if not api_key:
        raise SystemExit("Set GEMINI_API_KEY before calling the Gemini API")

    response = request_interaction(body, api_key, args.api_version)
    saved = save_outputs(response, Path(args.output_dir), args.basename)

    print(f"interaction_id={response.get('id', '')}")
    print(f"status={response.get('status', '')}")
    for path in saved:
        print(path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
