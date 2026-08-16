#!/usr/bin/env python3

import json
import os
import re
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import sync_playwright


BASE_URL = os.environ.get("ADMIN_VISUAL_BASE_URL", "http://127.0.0.1:43125/")
OUTPUT_DIR = Path(os.environ.get("ADMIN_VISUAL_OUTPUT_DIR", "/tmp/mgs-v025-native-video-pricing"))


def task_config(resolutions, audio_modes):
    return {
        "durations": {"values": [5, 10]},
        "resolutions": resolutions,
        "aspect_ratios": ["16:9", "9:16"],
        "audio_modes": audio_modes,
        "inputs": {
            "first_frame": {
                "required": True,
                "max_count": 1,
                "max_bytes": 30 * 1024 * 1024,
                "media_types": ["image"],
                "formats": ["jpg", "jpeg", "png", "webp"],
            }
        },
    }


ACCOUNTS = [
    {
        "id": 10,
        "name": "Seedance 主账号",
        "adapter_type": "seedance",
        "auth_type": "api_key",
        "base_url": "https://ark.cn-beijing.volces.com/api/v3",
        "status": "enabled",
        "priority": 1,
        "weight": 100,
        "concurrency_limit": 5,
        "timeout_ms": 300000,
        "extra": {"media_type": "video"},
    },
    {
        "id": 20,
        "name": "MiniMax 主账号",
        "adapter_type": "minimax",
        "auth_type": "api_key",
        "base_url": "https://api.minimaxi.com/v1",
        "status": "enabled",
        "priority": 2,
        "weight": 100,
        "concurrency_limit": 5,
        "timeout_ms": 300000,
        "extra": {"media_type": "video"},
    },
]

MODELS = {
    "10": [
        {
            "id": 101,
            "account_id": 10,
            "account_name": "Seedance 主账号",
            "model_code": "seedance-2.5",
            "display_name": "Seedance 2.5",
            "task_types": ["text_to_video", "image_to_video", "first_last_frame_to_video"],
            "base_resolution": ["720p"],
            "max_image_count": 1,
            "max_reference_image_count": 2,
            "output_format": ["mp4"],
            "enabled": True,
            "extra": {"media_type": "video"},
        }
    ],
    "20": [
        {
            "id": 201,
            "account_id": 20,
            "account_name": "MiniMax 主账号",
            "model_code": "minimax-h3",
            "display_name": "MiniMax H3",
            "task_types": ["text_to_video", "image_to_video"],
            "base_resolution": ["768p", "2k"],
            "max_image_count": 1,
            "max_reference_image_count": 5,
            "output_format": ["mp4"],
            "enabled": True,
            "extra": {"media_type": "video"},
        }
    ],
}

CAPABILITIES = [
    {
        "account_model_id": 101,
        "capability_version": "seedance-cap-v3",
        "validation_status": "verified",
        "enabled": True,
        "capability": {
            "schema_version": 1,
            "provider_native_max_n": 1,
            "prompt_max_runes": 2000,
            "task_types": {
                "text_to_video": task_config(["720p"], ["silent", "generated"]),
                "image_to_video": task_config(["720p"], ["silent", "generated"]),
                "first_last_frame_to_video": task_config(["720p"], ["silent"]),
            },
        },
    },
    {
        "account_model_id": 201,
        "capability_version": "minimax-cap-v2",
        "validation_status": "verified",
        "enabled": True,
        "capability": {
            "schema_version": 1,
            "provider_native_max_n": 1,
            "prompt_max_runes": 2000,
            "task_types": {
                "text_to_video": task_config(["768p", "2k"], ["silent"]),
                "image_to_video": task_config(["768p", "2k"], ["silent"]),
            },
        },
    },
]

RATE_CARDS = [
    {
        "id": 1001,
        "account_model_id": 101,
        "provider_code": "seedance",
        "pricing_schema": "seedance_token_v1",
        "currency": "CNY",
        "rate_version": 3,
        "source_reference": "https://docs.volcengine.com/docs/82379/1544106",
        "effective_at": "2026-08-17T00:00:00Z",
        "enabled": True,
        "rate_config": {
            "resolutions": {
                "720p": {
                    "without_input_video_million_tokens_cny": "46.00000",
                    "with_input_video_million_tokens_cny": "60.00000",
                }
            }
        },
    },
    {
        "id": 2001,
        "account_model_id": 201,
        "provider_code": "minimax",
        "pricing_schema": "minimax_h3_second_v1",
        "currency": "CNY",
        "rate_version": 2,
        "source_reference": "https://platform.minimaxi.com/docs/guides/pricing-paygo",
        "effective_at": "2026-08-17T00:00:00Z",
        "enabled": True,
        "rate_config": {
            "resolutions": {
                "768p": {"output_second_cny": "0.80000", "input_video_second_cny": "0.80000"},
                "2k": {"output_second_cny": "1.20000", "input_video_second_cny": "1.20000"},
            },
            "free_image_count": 5,
            "extra_image_cny": "0.10000",
            "input_audio_free": True,
        },
    },
]

ROUTES = [
    {
        "id": 301,
        "code": "video-pro",
        "name": "专业视频路由",
        "description": "Seedance 与 MiniMax 混合候选",
        "visibility": "public",
        "media_type": "video",
        "enabled": True,
        "sort_order": 10,
        "group_ids": [],
    }
]

CANDIDATES = [
    {"id": 401, "route_model_id": 301, "account_model_id": 101, "account_name": "Seedance 主账号", "model_code": "seedance-2.5", "priority": 1, "weight": 100, "fallback_order": 1, "enabled": True},
    {"id": 402, "route_model_id": 301, "account_model_id": 201, "account_name": "MiniMax 主账号", "model_code": "minimax-h3", "priority": 2, "weight": 100, "fallback_order": 2, "enabled": True},
]

ROUTE_CONFIG = {
    "route_model_id": 301,
    "route_code": "video-pro",
    "route_name": "专业视频路由",
    "config_version": "video-route-v4",
    "candidate_count": 2,
    "candidate_account_model_ids": [101, 201],
    "candidate_parameter_mappings": {"101": {"resolutions": {"720p": "720p"}}, "201": {"resolutions": {"720p": "768p"}}},
    "minimum_task_points": "10.00000",
    "rounding_step_points": 1,
    "task_types": ["text_to_video", "image_to_video"],
    "visible_options": {
        "combinations": [
            {"task_type": "text_to_video", "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "silent", "duration_seconds": 5},
            {"task_type": "image_to_video", "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "silent", "duration_seconds": 5},
        ]
    },
    "defaults": {"task_type": "text_to_video", "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "silent", "duration_seconds": 5},
    "max_output_count": 4,
    "enabled": True,
}

SNAPSHOT = {"capabilities": CAPABILITIES, "rate_cards": RATE_CARDS, "routes": [ROUTE_CONFIG], "impacts": [], "generated_at": "2026-08-17T00:00:00Z"}

QUOTE = {
    "route_model_id": 301,
    "config_version": "video-route-v4",
    "candidates": [
        {"route_candidate_id": 401, "account_model_id": 101, "provider_code": "seedance", "model_code": "seedance-2.5", "capability_version": "seedance-cap-v3", "pricing_schema": "seedance_token_v1", "rate_version": 3, "eligible": True, "mapped_resolution": "720p", "estimated_cny": "3.63750", "exclusion_code": "", "calculation": {}},
        {"route_candidate_id": 402, "account_model_id": 201, "provider_code": "minimax", "model_code": "minimax-h3", "capability_version": "minimax-cap-v2", "pricing_schema": "minimax_h3_second_v1", "rate_version": 2, "eligible": True, "mapped_resolution": "768p", "estimated_cny": "4.00000", "exclusion_code": "", "calculation": {}},
    ],
    "highest_account_model_id": 201,
    "highest_cny": "4.00000",
    "cny_per_point": "0.01000",
    "conversion_version": "billing-pricing-v1",
    "minimum_task_points": "10.00000",
    "rounding_step_points": 1,
    "unit_points": "400.00000",
    "total_points": "400.00000",
}


def envelope(data):
    return {"data": data, "meta": {"request_id": "visual-v025"}}


def page_data(items):
    return {"items": items, "page": 1, "page_size": 100, "total": len(items), "next_cursor": ""}


def route_api(route):
    path = urlparse(route.request.url).path
    if not path.startswith("/api/"):
        route.continue_()
        return
    if path == "/api/system/v1/bootstrap-status":
        data = {"phase": "ready"}
    elif path == "/api/ops/admin/v1/metrics/dashboard":
        data = {"operations": {}, "call_distribution": {"window": {"from": "", "to": ""}, "total_calls": 0, "groups": [], "preflight_failure_count": 0}, "metrics": [], "providers": [], "queue": [], "audit": []}
    elif path == "/api/ops/admin/v1/config-tabs":
        data = {"items": []}
    elif path == "/api/ops/admin/v1/model-accounts":
        data = page_data(ACCOUNTS)
    elif re.fullmatch(r"/api/ops/admin/v1/model-accounts/\d+/models", path):
        data = page_data(MODELS.get(path.split("/")[-2], []))
    elif path == "/api/ops/admin/v1/video/configuration":
        data = SNAPSHOT
    elif path == "/api/ops/admin/v1/route-models":
        data = page_data(ROUTES)
    elif path == "/api/ops/admin/v1/user-groups":
        data = page_data([])
    elif path == "/api/ops/admin/v1/route-model-prices":
        data = page_data([])
    elif path == "/api/ops/admin/v1/route-models/301/candidates":
        data = page_data(CANDIDATES)
    elif path == "/api/ops/admin/v1/route-models/301/video-config":
        data = ROUTE_CONFIG
    elif path == "/api/ops/admin/v1/video-routes/301/quote-simulation":
        data = QUOTE
    elif path == "/api/ops/admin/v1/auth/refresh":
        data = {"token": "visual-token", "admin_id": 1, "admin_name": "视觉验收", "role": "super_admin"}
    else:
        data = {"items": []}
    route.fulfill(status=200, content_type="application/json; charset=utf-8", body=json.dumps(envelope(data), ensure_ascii=False))


def assert_layout(page, label):
    result = page.evaluate("""() => ({
      text: document.body?.innerText || '',
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth: window.innerWidth
    })""")
    assert result["scrollWidth"] <= result["innerWidth"] + 4, f"{label}: horizontal overflow {result['scrollWidth']} > {result['innerWidth']}"
    assert "Cannot read properties" not in result["text"], f"{label}: runtime failure shown"
    return result["text"]


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for width, height, viewport_name in [(1440, 960, "desktop"), (1180, 820, "tablet")]:
                page = browser.new_page(viewport={"width": width, "height": height})
                errors = []
                page.on("console", lambda message: errors.append(f"console:{message.type}:{message.text}") if message.type == "error" else None)
                page.on("pageerror", lambda error: errors.append(f"pageerror:{error}"))
                page.route("**/*", route_api)
                page.add_init_script("""sessionStorage.setItem('pic_gallery_admin_session', JSON.stringify({token:'visual-token', admin_id:1, admin_name:'视觉验收', role:'super_admin'})); localStorage.setItem('pic_gallery_admin_theme','light');""")

                page.goto(BASE_URL + "#/access-accounts?media=video", wait_until="networkidle")
                page.get_by_text("视频接入账号", exact=True).wait_for()
                text = assert_layout(page, f"{viewport_name}/access-accounts")
                assert "Seedance 主账号" in text and "MiniMax 主账号" in text
                page.screenshot(path=str(OUTPUT_DIR / f"{viewport_name}-access-accounts.png"), full_page=True)

                page.get_by_label("编辑真实视频模型").first.click()
                page.get_by_text("编辑真实视频模型", exact=True).wait_for()
                dialog = page.get_by_role("dialog")
                dialog_text = dialog.inner_text()
                assert "不含输入视频 CNY/百万 Token" in dialog_text
                assert "包含输入视频 CNY/百万 Token" in dialog_text
                assert not any(term in dialog_text for term in ["支付费率", "目标毛利", "reserve markup", "净收入"])
                assert_layout(page, f"{viewport_name}/seedance-modal")
                page.wait_for_timeout(350)
                page.screenshot(path=str(OUTPUT_DIR / f"{viewport_name}-seedance-rate-editor.png"), full_page=True)
                dialog.get_by_role("button", name="取消").click()

                page.get_by_role("button", name=re.compile("MiniMax 主账号")).click()
                page.wait_for_timeout(200)
                page.get_by_label("编辑真实视频模型").first.click()
                dialog = page.get_by_role("dialog")
                dialog_text = dialog.inner_text()
                assert "输出视频 CNY/秒" in dialog_text
                assert "输入视频 CNY/秒" in dialog_text
                assert "超额图片 CNY/张" in dialog_text
                assert_layout(page, f"{viewport_name}/minimax-modal")
                page.wait_for_timeout(350)
                page.screenshot(path=str(OUTPUT_DIR / f"{viewport_name}-minimax-rate-editor.png"), full_page=True)
                dialog.get_by_role("button", name="取消").click()

                page.goto(BASE_URL + "#/routing?media=video", wait_until="networkidle")
                page.get_by_text("候选分辨率映射", exact=True).wait_for()
                text = assert_layout(page, f"{viewport_name}/routing")
                assert "Seedance 主账号 / seedance-2.5" in text
                assert "MiniMax 主账号 / minimax-h3" in text
                assert "删除视频配置" in text
                page.get_by_text("候选分辨率映射", exact=True).scroll_into_view_if_needed()
                page.wait_for_timeout(200)
                page.screenshot(path=str(OUTPUT_DIR / f"{viewport_name}-routing.png"), full_page=True)

                page.goto(BASE_URL + "#/pricing?media=video", wait_until="networkidle")
                page.get_by_text("视频报价总览", exact=True).wait_for()
                page.get_by_role("button", name="运行试算").click()
                page.get_by_text("最高价来源", exact=True).wait_for()
                text = assert_layout(page, f"{viewport_name}/pricing")
                for required in ["最高销售价", "全局汇率", "映射分辨率", "最高价来源", "参与最高价比较"]:
                    assert required in text, f"{viewport_name}/pricing missing {required}"
                for forbidden in ["支付费率", "目标毛利", "reserve markup", "净收入"]:
                    assert forbidden not in text, f"{viewport_name}/pricing leaked {forbidden}"
                page.screenshot(path=str(OUTPUT_DIR / f"{viewport_name}-pricing-quote.png"), full_page=True)

                assert not errors, f"{viewport_name}: browser errors: {errors}"
                page.close()
        finally:
            browser.close()
    print(f"PASS: native video pricing visual acceptance ({len(list(OUTPUT_DIR.glob('*.png')))} screenshots) -> {OUTPUT_DIR}")


if __name__ == "__main__":
    main()
