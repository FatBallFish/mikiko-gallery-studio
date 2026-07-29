#!/usr/bin/env python3

import json
import os
from pathlib import Path
from urllib.parse import unquote, urlparse

from playwright.sync_api import expect, sync_playwright


BASE_URL = os.environ["BASE_URL"].rstrip("/")
USER_WEB_URL = os.environ.get("USER_WEB_URL", "").rstrip("/")
ADMIN_WEB_URL = os.environ.get("ADMIN_WEB_URL", "").rstrip("/")
DIRECT_USER_WEB_URL = os.environ.get("DIRECT_USER_WEB_URL", "").rstrip("/")
DIRECT_ADMIN_WEB_URL = os.environ.get("DIRECT_ADMIN_WEB_URL", "").rstrip("/")
DIRECT_DOCS_WEB_URL = os.environ.get("DIRECT_DOCS_WEB_URL", "").rstrip("/")
GATEWAY_DOCS_WEB_URL = os.environ.get("GATEWAY_DOCS_WEB_URL", "").rstrip("/")
SETUP_TOKEN = os.environ["SETUP_TOKEN"]
PROFILE = os.environ.get("DEPLOYMENT_PROFILE", "full")
REDIRECT_SETUP_URL = os.environ.get("REDIRECT_SETUP_URL", f"{BASE_URL}/setup")
OUTPUT_DIR = Path(os.environ["E2E_EVIDENCE_DIR"])
APPLY_SETUP = os.environ.get("E2E_APPLY_SETUP", "false").lower() == "true"
EXPECT_INTERRUPTION = os.environ.get("E2E_EXPECT_INTERRUPTION", "false").lower() == "true"
INTERRUPTION_READY_FILE = os.environ.get("E2E_INTERRUPTION_READY_FILE", "")

RUNTIME_ENV_FIELDS = {
    "DATABASE_URL": "SETUP_DATABASE_URL",
    "REDIS_URL": "SETUP_REDIS_URL",
    "REDIS_KEY_PREFIX": "SETUP_REDIS_KEY_PREFIX",
    "STORAGE_DRIVER": "SETUP_STORAGE_DRIVER",
    "STORAGE_S3_ENDPOINT": "SETUP_STORAGE_S3_ENDPOINT",
    "STORAGE_S3_REGION": "SETUP_STORAGE_S3_REGION",
    "STORAGE_S3_BUCKET": "SETUP_STORAGE_S3_BUCKET",
    "STORAGE_S3_ACCESS_KEY_ID": "SETUP_STORAGE_S3_ACCESS_KEY_ID",
    "STORAGE_S3_SECRET_ACCESS_KEY": "SETUP_STORAGE_S3_SECRET_ACCESS_KEY",
    "STORAGE_S3_FORCE_PATH_STYLE": "SETUP_STORAGE_S3_FORCE_PATH_STYLE",
    "STORAGE_S3_PREFIX": "SETUP_STORAGE_S3_PREFIX",
    "CORS_ALLOWED_ORIGINS": "SETUP_CORS_ALLOWED_ORIGINS",
}


def assert_layout(page):
    metrics = page.evaluate("""() => ({
      width: document.documentElement.scrollWidth,
      viewport: window.innerWidth,
      headings: document.querySelectorAll('h1, h2').length,
      model: Boolean(document.querySelector('#setup-model')),
      background: getComputedStyle(document.body).backgroundColor,
    })""")
    assert metrics["width"] <= metrics["viewport"] + 2, metrics
    assert metrics["headings"] >= 2 and metrics["model"], metrics
    assert metrics["background"] not in ("rgba(0, 0, 0, 0)", "transparent"), metrics


def verify_setup_page(page, screenshot):
    page.goto(f"{BASE_URL}/setup", wait_until="domcontentloaded")
    expect(page.locator("#setup-console")).to_be_visible()
    expect(page.get_by_text("deployctl setup token show", exact=True)).to_be_visible()
    expect(page.get_by_text("deployctl setup token reset", exact=True)).to_be_visible()
    assert_layout(page)
    page.screenshot(path=OUTPUT_DIR / screenshot, full_page=True)


def authenticate(page):
    page.locator("#setup-token").fill(SETUP_TOKEN)
    page.locator("#authenticate").click()
    expect(page.locator("#workspace")).to_be_visible(timeout=15000)
    managed = PROFILE == "full"
    for key in ("DATABASE_URL", "REDIS_URL", "STORAGE_DRIVER"):
        field = page.locator(f'[data-field="{key}"]')
        expect(field).to_have_count(1)
        assert (field.locator(".managed-note").count() == 1) is managed, (PROFILE, key)
    expect(page.locator("#database-fields .field-help").first).to_contain_text(" / ")
    expect(page.locator("#redis-fields .field-help").first).to_contain_text(" / ")
    expect(page.locator("#storage-fields .field-help").first).to_contain_text(" / ")


def wait_for_setup_redirect(page, return_url, setup_url=REDIRECT_SETUP_URL):
    prefix = f"{setup_url}#return_to="
    page.wait_for_url(lambda current: str(current).startswith(prefix), timeout=15000)
    fragment = urlparse(page.url).fragment
    assert fragment.startswith("return_to="), page.url
    assert unquote(fragment.removeprefix("return_to=")) == return_url, page.url


def verify_redirect(browser, url, request_urls):
    if not url:
        return
    page = browser.new_page(viewport={"width": 1280, "height": 800})
    failures = []
    responses = []
    page.on("request", lambda request: request_urls.append(request.url))
    page.on("requestfailed", lambda request: failures.append({"url": request.url, "failure": request.failure}))
    page.on("response", lambda response: responses.append({"url": response.url, "status": response.status, "content_type": response.headers.get("content-type", "")}))
    expected_setup_url = f"{BASE_URL}/setup" if url in {DIRECT_USER_WEB_URL, DIRECT_ADMIN_WEB_URL} else REDIRECT_SETUP_URL
    try:
        page.goto(url, wait_until="domcontentloaded")
        wait_for_setup_redirect(page, url, expected_setup_url)
    except Exception:
        slug = "admin" if "/admin" in url else "user"
        (OUTPUT_DIR / f"{slug}-redirect-debug.json").write_text(json.dumps({
            "start_url": url,
            "expected_url": expected_setup_url,
            "current_url": page.url,
            "title": page.title(),
            "text": page.locator("body").inner_text()[:2000],
            "requests": request_urls,
            "responses": responses,
            "failures": failures,
        }, indent=2), encoding="utf-8")
        raise
    finally:
        page.close()


def verify_docs(browser, url, request_urls):
    if not url:
        return
    page = browser.new_page(viewport={"width": 1280, "height": 800})
    page_errors = []
    page.on("request", lambda request: request_urls.append(request.url))
    page.on("pageerror", lambda error: page_errors.append(str(error)))
    page.goto(url, wait_until="networkidle")
    assert not page.url.startswith(REDIRECT_SETUP_URL), page.url
    expect(page.locator(".docs-brand")).to_be_visible(timeout=30000)
    expect(page.locator(".guide-heading h1")).to_be_visible(timeout=30000)
    expect(page.locator(".reference-error")).to_have_count(0)
    assert not page_errors, (url, page_errors)
    page.close()


def verify_ready_app(browser, url, request_urls):
    if not url:
        return
    page = browser.new_page(viewport={"width": 1280, "height": 800})
    page_errors = []
    page.on("request", lambda request: request_urls.append(request.url))
    page.on("pageerror", lambda error: page_errors.append(str(error)))
    page.goto(url, wait_until="domcontentloaded")
    assert not page.url.startswith(REDIRECT_SETUP_URL), page.url
    if url == DIRECT_ADMIN_WEB_URL:
        expect(page.get_by_role("heading", name="管理员登录")).to_be_visible(timeout=30000)
        expect(page.get_by_text("后台服务暂不可用", exact=True)).to_have_count(0)
    else:
        expect(page.get_by_text("开始创作", exact=True).first).to_be_visible(timeout=30000)
        expect(page.get_by_text("服务暂不可用", exact=True)).to_have_count(0)
    assert not page_errors, (url, page_errors)
    page.close()


def fill_runtime_fields(page):
    for key, environment_key in RUNTIME_ENV_FIELDS.items():
        configured = os.environ.get(environment_key)
        if configured is None or configured == "":
            continue
        field = page.locator(f'[data-runtime-field="{key}"]')
        expect(field).to_have_count(1)
        input_type = field.get_attribute("type")
        if input_type == "checkbox":
            field.set_checked(configured.lower() == "true")
        elif field.evaluate("element => element.tagName") == "SELECT":
            field.select_option(configured)
        else:
            field.fill(configured)


def run_probe(page, kind):
    button = page.locator(f'[data-probe="{kind}"]')
    button.click()
    expect(page.locator(f"#{kind}-probe-status")).to_contain_text("Connected", timeout=30000)


def drive_setup_apply(browser, request_urls):
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    page.on("request", lambda request: request_urls.append(request.url))
    user_url = urlparse(USER_WEB_URL)
    history_seed_url = f"{user_url.scheme}://{user_url.netloc}/developer-docs/"
    page.goto(history_seed_url, wait_until="domcontentloaded")
    page.evaluate("url => window.location.assign(url)", USER_WEB_URL)
    wait_for_setup_redirect(page, USER_WEB_URL)
    authenticate(page)
    fill_runtime_fields(page)

    if PROFILE == "core":
        database = page.locator('[data-runtime-field="DATABASE_URL"]')
        configured_database = database.input_value()
        database.fill("not-a-postgres-url")
        page.locator('[data-probe="database"]').click()
        expect(page.locator("#database-probe-status")).to_contain_text("INVALID_CONFIGURATION", timeout=30000)
        database.fill(configured_database)

    for kind in ("database", "redis", "storage"):
        run_probe(page, kind)

    page.locator("#admin-email").fill(os.environ.get("SETUP_ADMIN_EMAIL", "admin@example.com"))
    page.locator("#admin-password").fill(os.environ.get("SETUP_ADMIN_PASSWORD", "admin123456"))
    page.locator("#apply-setup").click()
    if EXPECT_INTERRUPTION:
        if not INTERRUPTION_READY_FILE:
            raise RuntimeError("E2E_INTERRUPTION_READY_FILE is required for interrupted setup")
        page.screenshot(path=OUTPUT_DIR / "setup-interrupted-pending.png", full_page=True)
        Path(INTERRUPTION_READY_FILE).write_text("apply-submitted\n", encoding="utf-8")
        page.wait_for_timeout(300000)
        raise RuntimeError("interrupted setup browser was not terminated by the E2E controller")
    expect(page.locator("#progress-panel")).to_be_visible(timeout=30000)
    expect(page.locator("#restart-countdown")).to_contain_text("Restart countdown", timeout=300000)
    page.screenshot(path=OUTPUT_DIR / "setup-restart-countdown.png", full_page=True)
    expect(page.locator("#completion-panel")).to_be_visible(timeout=180000)
    page.screenshot(path=OUTPUT_DIR / "setup-complete.png", full_page=True)
    page.wait_for_url(lambda current: str(current).startswith(USER_WEB_URL), timeout=30000)
    expect(page.locator("body")).to_be_visible()
    page.screenshot(path=OUTPUT_DIR / "setup-returned-to-user.png", full_page=True)
    page.close()
    verify_ready_app(browser, DIRECT_USER_WEB_URL, request_urls)
    verify_ready_app(browser, DIRECT_ADMIN_WEB_URL, request_urls)


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    request_urls = []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        if APPLY_SETUP:
            drive_setup_apply(browser, request_urls)
        else:
            page = browser.new_page(viewport={"width": 1440, "height": 900})
            page.on("request", lambda request: request_urls.append(request.url))
            verify_setup_page(page, "desktop-setup.png")
            authenticate(page)
            page.goto(f"{BASE_URL}/api/system/v1/bootstrap-status")
            page.go_back(wait_until="domcontentloaded")
            expect(page.locator("#setup-console")).to_be_visible()

            mobile = browser.new_page(viewport={"width": 390, "height": 844})
            verify_setup_page(mobile, "mobile-setup.png")
            authenticate(mobile)
            assert_layout(mobile)
            mobile.close()

            verify_redirect(browser, USER_WEB_URL, request_urls)
            verify_redirect(browser, ADMIN_WEB_URL, request_urls)
            verify_redirect(browser, DIRECT_USER_WEB_URL, request_urls)
            verify_redirect(browser, DIRECT_ADMIN_WEB_URL, request_urls)
            verify_docs(browser, DIRECT_DOCS_WEB_URL, request_urls)
            verify_docs(browser, GATEWAY_DOCS_WEB_URL, request_urls)
        browser.close()

    setup_origin = urlparse(BASE_URL).netloc
    setup_requests = [url for url in request_urls if urlparse(url).netloc == setup_origin]
    unexpected_assets = [url for url in setup_requests if "/assets/" in url]
    assert not unexpected_assets, unexpected_assets
    (OUTPUT_DIR / "request_urls.json").write_text(json.dumps(request_urls, indent=2), encoding="utf-8")
    print("setup browser E2E passed; request_urls captured")


if __name__ == "__main__":
    main()
