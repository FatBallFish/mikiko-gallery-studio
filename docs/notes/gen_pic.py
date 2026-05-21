#!/usr/bin/env python3
"""
Studio Media OpenAI-compatible API examples.

Environment variables:
STUDIO_API_KEY Required. Your API key.
STUDIO_API_BASE_URL Optional. Defaults to http://localhost:8080/studio/v1

Examples:
python gen_pic.py models
python gen_pic.py image --model your-image-model --prompt "A product photo"
python gen_pic.py edit --model your-edit-model --image ./input.png --prompt "Replace background"
"""


import base64
import argparse
import json
import mimetypes
import os
import re
import sys
from pathlib import Path
from typing import Any, Optional

try:
    import requests
except ImportError:
    print("Missing dependency: requests. Install with: pip install -r requirements.txt", file=sys.stderr)
    raise

DEFAULT_BASE_URL = "https://direct.ruok.xin/v1"
OPENROUTER_TIMEOUT_SECONDS = 600
DATA_URL_PATTERN = re.compile(r"^data:(?P<mime>[-\w.]+/[-+\w.]+);base64,(?P<data>.+)$", re.DOTALL)

class StudioMediaAPIError(RuntimeError):
    pass


def get_config() -> tuple[str, str]:
    api_key = os.getenv("STUDIO_API_KEY", "").strip()
    if not api_key:
        raise StudioMediaAPIError("Please set STUDIO_API_KEY first.")

    base_url = os.getenv("STUDIO_API_BASE_URL", DEFAULT_BASE_URL).strip().rstrip("/")
    return base_url, api_key


def headers(api_key: str) -> dict[str, str]:
    return {
        "Authorization": f"Bearer {api_key}",
        "Accept": "application/json",
    }


def parse_response(response: requests.Response) -> dict[str, Any]:
    try:
        data = response.json()
    except ValueError as exc:
        raise StudioMediaAPIError(
            f"HTTP {response.status_code}: non-JSON response: {response.text[:500]}"
        ) from exc

    if response.status_code >= 400:
        error = data.get("error", data)
        if isinstance(error, dict):
            code = error.get("code") or error.get("type") or "api_error"
            message = error.get("message") or json.dumps(error, ensure_ascii=False)
        else:
            code = "api_error"
            message = str(error)
        raise StudioMediaAPIError(f"HTTP {response.status_code} {code}: {message}")

    return data


def request_json(
    method: str,
    path: str,
    payload: Optional[dict[str, Any]] = None,
    timeout: int = 120,
) -> dict[str, Any]:
    base_url, api_key = get_config()
    response = requests.request(
        method,
        f"{base_url}{path}",
        headers={**headers(api_key), "Content-Type": "application/json"},
        json=payload,
        timeout=timeout,
    )
    return parse_response(response)


def list_models() -> dict[str, Any]:
    return request_json("GET", "/models", timeout=30)


def should_use_openrouter_model(model: str) -> bool:
    normalized = model.strip().lower()
    return "/" in normalized and not normalized.startswith("gpt-image-")


def build_openrouter_image_params(size: str, n: int, quality: str) -> dict[str, Any]:
    payload: dict[str, Any] = {}
    if size:
        payload["size"] = size
    if n > 0:
        payload["n"] = n
    if quality:
        payload["quality"] = quality
    return payload


def build_image_data_url(image_path: Path) -> str:
    if not image_path.exists():
        raise StudioMediaAPIError(f"Image file does not exist: {image_path}")

    content_type = mimetypes.guess_type(str(image_path))[0] or "image/png"
    image_bytes = image_path.read_bytes()
    encoded = base64.b64encode(image_bytes).decode("ascii")
    return f"data:{content_type};base64,{encoded}"


def parse_data_url(url: str) -> tuple[Optional[str], Optional[str]]:
    match = DATA_URL_PATTERN.match(url.strip())
    if not match:
        return None, None

    mime_type = match.group("mime").lower()
    image_format = mime_type.split("/", 1)[1] if "/" in mime_type else None
    return image_format, match.group("data")


def normalize_openrouter_image_response(data: dict[str, Any]) -> dict[str, Any]:
    # Convert OpenRouter chat-completions image output into the same shape
    # used by the native image endpoint so the CLI output code stays unchanged.
    choices = data.get("choices") or []
    if not isinstance(choices, list):
        raise StudioMediaAPIError("OpenRouter response does not include choices.")

    normalized_items: list[dict[str, Any]] = []
    result_urls: list[str] = []

    for choice in choices:
        if not isinstance(choice, dict):
            continue
        message = choice.get("message") or {}
        if not isinstance(message, dict):
            continue
        images = message.get("images") or []
        if not isinstance(images, list):
            continue
        for image in images:
            if not isinstance(image, dict):
                continue
            image_url = image.get("image_url") or {}
            if not isinstance(image_url, dict):
                continue
            url = str(image_url.get("url") or "").strip()
            if not url:
                continue
            image_format, b64_json = parse_data_url(url)
            item: dict[str, Any] = {}
            if b64_json:
                item["b64_json"] = b64_json
            else:
                item["url"] = url
                result_urls.append(url)
            if image_format:
                item["format"] = image_format
            normalized_items.append(item)

    if not normalized_items:
        raise StudioMediaAPIError("OpenRouter response does not include generated images.")

    normalized = {"data": normalized_items}
    if result_urls:
        normalized["result_urls"] = result_urls
    return normalized


def build_openrouter_chat_payload(
    model: str,
    prompt: str,
    size: str,
    n: int,
    quality: str,
    image_data_url: Optional[str] = None,
) -> dict[str, Any]:
    if image_data_url:
        content: Any = [
            {"type": "text", "text": prompt},
            {"type": "image_url", "image_url": {"url": image_data_url}},
        ]
    else:
        content = prompt

    payload: dict[str, Any] = {
        "model": model,
        "messages": [{"role": "user", "content": content}],
        "modalities": ["image", "text"],
    }
    payload.update(build_openrouter_image_params(size, n, quality))
    return payload


def generate_image(
    model: str,
    prompt: str,
    size: str,
    n: int,
    quality: str,
    response_format: str,
) -> dict[str, Any]:
    if should_use_openrouter_model(model):
        response = request_json(
            "POST",
            "/chat/completions",
            build_openrouter_chat_payload(model, prompt, size, n, quality),
            timeout=OPENROUTER_TIMEOUT_SECONDS,
        )
        return normalize_openrouter_image_response(response)

    payload = {
        "model": model,
        "prompt": prompt,
        "size": size,
        "n": n,
        "quality": quality,
        "response_format": response_format,
    }
    return request_json("POST", "/images/generations", payload, timeout=180)


def edit_image(
    model: str,
    prompt: str,
    image_path: Path,
    size: str,
    n: int,
    quality: str,
    response_format: str,
) -> dict[str, Any]:
    if not image_path.exists():
        raise StudioMediaAPIError(f"Image file does not exist: {image_path}")

    if should_use_openrouter_model(model):
        # OpenRouter image editing is modeled as a multimodal chat request:
        # text instructions plus the source image as a data URL.
        response = request_json(
            "POST",
            "/chat/completions",
            build_openrouter_chat_payload(
                model,
                prompt,
                size,
                n,
                quality,
                image_data_url=build_image_data_url(image_path),
            ),
            timeout=OPENROUTER_TIMEOUT_SECONDS,
        )
        return normalize_openrouter_image_response(response)

    base_url, api_key = get_config()
    content_type = mimetypes.guess_type(str(image_path))[0] or "image/png"

    with image_path.open("rb") as image_file:
        response = requests.post(
            f"{base_url}/images/edits",
            headers=headers(api_key),
            data={
                "model": model,
                "prompt": prompt,
                "size": size,
                "n": str(n),
                "quality": quality,
                "response_format": response_format,
            },
            files={
                "image": (image_path.name, image_file, content_type),
            },
            timeout=180,
        )
    return parse_response(response)


def infer_image_suffix(item: dict[str, Any], default_suffix: str = ".png") -> str:
    image_format = str(item.get("format") or "").strip().lower()
    if image_format:
        return f".{image_format.lstrip('.')}"
    return default_suffix


def build_output_path(output_path: Path, index: int, total: int, suffix: str) -> Path:
    if total <= 1:
        return output_path if output_path.suffix else output_path.with_suffix(suffix)

    if output_path.suffix:
        return output_path.with_name(f"{output_path.stem}_{index + 1}{output_path.suffix}")
    return output_path / f"image_{index + 1}{suffix}"


def write_image_files(data: dict[str, Any], output_path: Path) -> list[Path]:
    items = data.get("data") or []
    if not isinstance(items, list) or not items:
        raise StudioMediaAPIError("No image data found in response.")

    if len(items) > 1 and output_path.suffix and output_path.exists() and not output_path.is_file():
        raise StudioMediaAPIError(f"Output path is not a file: {output_path}")

    written_files: list[Path] = []
    for index, item in enumerate(items):
        if not isinstance(item, dict):
            continue
        b64_json = item.get("b64_json")
        if not b64_json:
            raise StudioMediaAPIError("Image response does not include b64_json.")
        try:
            image_bytes = base64.b64decode(b64_json, validate=True)
        except (ValueError, TypeError) as exc:
            raise StudioMediaAPIError("Failed to decode image base64 payload.") from exc

        target = build_output_path(output_path, index, len(items), infer_image_suffix(item))
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(image_bytes)
        written_files.append(target)

    if not written_files:
        raise StudioMediaAPIError("No image files were written.")
    return written_files


def print_image_base64(data: dict[str, Any]) -> None:
    items = data.get("data") or []
    if not isinstance(items, list) or not items:
        raise StudioMediaAPIError("No image data found in response.")

    found = False
    for index, item in enumerate(items, start=1):
        if not isinstance(item, dict):
            continue
        b64_json = item.get("b64_json")
        if not b64_json:
            continue
        found = True
        if len(items) > 1:
            print(f"IMAGE_BASE64[{index}]")
        print(str(b64_json))

    if not found:
        raise StudioMediaAPIError("Image response does not include b64_json.")


def print_result(data: dict[str, Any]) -> None:
    print(json.dumps(data, ensure_ascii=False, indent=2))

    urls: list[str] = []
    for item in data.get("data") or []:
        if isinstance(item, dict) and item.get("url"):
            urls.append(str(item["url"]))
    for item in data.get("result_urls") or []:
        urls.append(str(item))

    if urls:
        print("\nResult URLs:")
        for url in urls:
            print(url)


def resolve_image_response_format(output_mode: str) -> str:
    if output_mode in {"stdout", "file"}:
        return "b64_json"
    raise StudioMediaAPIError(f"Unsupported output mode: {output_mode}")


def output_image_result(
    data: dict[str, Any],
    output_mode: str,
    output_path: Optional[Path],
) -> None:
    if output_mode == "stdout":
        print_image_base64(data)
        return
    if output_mode == "file":
        if output_path is None:
            raise StudioMediaAPIError("Please provide --output-path when --output file is used.")
        written_files = write_image_files(data, output_path)
        for file_path in written_files:
            print(file_path)
        return
    raise StudioMediaAPIError(f"Unsupported output mode: {output_mode}")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Studio Media API Python examples")
    subparsers = parser.add_subparsers(dest="command", required=True)

    subparsers.add_parser("models", help="List available Studio media models")

    image = subparsers.add_parser("image", help="Generate image")
    image.add_argument("--model", required=True)
    image.add_argument("--prompt", required=True)
    image.add_argument("--size", default="1024x1024")
    image.add_argument("--n", type=int, default=1)
    image.add_argument("--quality", default="standard")
    image.add_argument("--output", choices=("stdout", "file"), default="stdout")
    image.add_argument("--output-path", type=Path)

    edit = subparsers.add_parser("edit", help="Edit image with multipart upload")
    edit.add_argument("--model", required=True)
    edit.add_argument("--prompt", required=True)
    edit.add_argument("--image", required=True, type=Path)
    edit.add_argument("--size", default="1024x1024")
    edit.add_argument("--n", type=int, default=1)
    edit.add_argument("--quality", default="standard")
    edit.add_argument("--output", choices=("stdout", "file"), default="stdout")
    edit.add_argument("--output-path", type=Path)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    try:
        if args.command == "models":
            print_result(list_models())
        elif args.command == "image":
            output_image_result(
                generate_image(
                    args.model,
                    args.prompt,
                    args.size,
                    args.n,
                    args.quality,
                    resolve_image_response_format(args.output),
                ),
                args.output,
                args.output_path,
            )
        elif args.command == "edit":
            output_image_result(
                edit_image(
                    args.model,
                    args.prompt,
                    args.image,
                    args.size,
                    args.n,
                    args.quality,
                    resolve_image_response_format(args.output),
                ),
                args.output,
                args.output_path,
            )
        else:
            parser.print_help()
            return 2
        return 0
    except StudioMediaAPIError as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
