#!/usr/bin/env python3

import json
import os
import re
from pathlib import Path

from playwright.sync_api import expect, sync_playwright


BASE_URL = os.environ.get("BASE_URL", "http://127.0.0.1:8088").rstrip("/")
USER_WEB_URL = os.environ.get("USER_WEB_URL", "http://127.0.0.1:8088").rstrip("/")
ADMIN_WEB_URL = os.environ.get("ADMIN_WEB_URL", "http://127.0.0.1:8088/admin").rstrip("/")
USER_TOKEN = os.environ["E2E_USER_TOKEN"]
RUN_ID = os.environ["E2E_RUN_ID"]
OUTPUT_DIR = Path(os.environ.get("E2E_BROWSER_OUTPUT_DIR", "tmp/e2e/prompt-workflow"))
ORIGINAL_PROMPT = "A quiet glass pavilion beside a rain-soaked garden"


def envelope(response):
    assert response.ok, f"API request failed: {response.status} {response.text()[:500]}"
    payload = response.json()
    assert isinstance(payload, dict) and "data" in payload, f"API response lacked data: {payload}"
    return payload["data"]


def assert_no_overlap(page):
    result = page.evaluate("""() => {
      const visible = (element) => {
        const style = getComputedStyle(element);
        const rect = element.getBoundingClientRect();
        return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0;
      };
      const dialog = document.querySelector('[role="dialog"]');
      const controls = dialog ? Array.from(dialog.querySelectorAll('button, textarea')).filter(visible) : [];
      const overlaps = [];
      for (let left = 0; left < controls.length; left += 1) {
        const a = controls[left].getBoundingClientRect();
        for (let right = left + 1; right < controls.length; right += 1) {
          const b = controls[right].getBoundingClientRect();
          const width = Math.min(a.right, b.right) - Math.max(a.left, b.left);
          const height = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
          if (width > 2 && height > 2) overlaps.push([controls[left].getAttribute('aria-label') || controls[left].textContent, controls[right].getAttribute('aria-label') || controls[right].textContent]);
        }
      }
      const dialogRect = dialog?.getBoundingClientRect();
      return {
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
        overlaps,
        dialogOutsideViewport: Boolean(dialogRect && (dialogRect.left < -1 || dialogRect.right > window.innerWidth + 1 || dialogRect.top < -1 || dialogRect.bottom > window.innerHeight + 1)),
      };
    }""")
    assert result["scrollWidth"] <= result["innerWidth"] + 4, f"horizontal overflow: {result}"
    assert not result["overlaps"], f"dialog controls overlap: {result['overlaps']}"
    assert not result["dialogOutsideViewport"], f"dialog is outside viewport: {result}"


def assert_dialog_above_toasts(page):
    result = page.evaluate("""() => {
      const dialog = document.querySelector('[role="dialog"]');
      const backdrop = dialog?.closest('[role="presentation"]');
      const toastStack = Array.from(document.querySelectorAll('[aria-live="polite"]'))
        .find(element => getComputedStyle(element).position === 'fixed');
      return {
        modalZ: Number.parseInt(backdrop ? getComputedStyle(backdrop).zIndex : '', 10),
        toastZ: Number.parseInt(toastStack ? getComputedStyle(toastStack).zIndex : '', 10),
      };
    }""")
    assert result["modalZ"] > result["toastZ"], f"dialog must render above toasts: {result}"


def install_sessions(context, page):
    profile = envelope(context.request.get(
        f"{BASE_URL}/api/agent/user/v1/profile",
        headers={"Authorization": f"Bearer {USER_TOKEN}"},
    ))
    page.goto(f"{USER_WEB_URL}/", wait_until="domcontentloaded")
    page.evaluate(
        "([token, profile]) => localStorage.setItem('pic-gallery-user-session', JSON.stringify({ token, profile }))",
        [USER_TOKEN, profile],
    )

    admin_login = envelope(context.request.post(
        f"{BASE_URL}/api/ops/admin/v1/auth/login",
        data={"email": "admin@example.com", "password": "admin123456"},
    ))
    admin_session = {
        "token": admin_login["access_token"],
        "access_token": admin_login["access_token"],
        "expires_in_seconds": admin_login.get("expires_in_seconds", 0),
        "admin_id": admin_login["admin_id"],
        "email": admin_login.get("email"),
        "admin_name": admin_login.get("email") or f"Admin {admin_login['admin_id']}",
        "role": admin_login["role"],
        "permissions": admin_login.get("permissions", []),
    }
    page.goto(f"{ADMIN_WEB_URL}/", wait_until="domcontentloaded")
    page.evaluate(
        "session => sessionStorage.setItem('pic_gallery_admin_session', JSON.stringify(session))",
        admin_session,
    )
    page.reload(wait_until="domcontentloaded")


def verify_admin_text_models(page):
    page.goto(f"{ADMIN_WEB_URL}/#/system-settings?tab=text-models", wait_until="domcontentloaded")
    chat_account = page.get_by_text(f"Docker E2E Text Chat {RUN_ID}", exact=True)
    expect(chat_account).to_be_visible(timeout=15000)
    expect(page.get_by_text(f"Docker E2E Text Responses {RUN_ID}", exact=True)).to_be_visible()
    chat_account.click()
    expect(page.get_by_text("默认优化模型", exact=False).first).to_be_visible()
    bounds = chat_account.evaluate("""element => {
      const button = element.closest('button');
      const content = button?.querySelector(':scope > span');
      const aside = button?.closest('aside');
      const buttonRect = button?.getBoundingClientRect();
      const contentRect = content?.getBoundingClientRect();
      const asideRect = aside?.getBoundingClientRect();
      return {
        contentRight: contentRect?.right,
        buttonRight: buttonRect?.right,
        asideRight: asideRect?.right,
      };
    }""")
    assert bounds["contentRight"] <= bounds["buttonRight"] + 1, f"account label escaped its button: {bounds}"
    assert bounds["contentRight"] <= bounds["asideRight"] + 1, f"account label escaped its sidebar: {bounds}"
    assert_no_overlap(page)
    page.screenshot(path=OUTPUT_DIR / "desktop-admin-text-models.png", full_page=True)


def verify_prompt_optimization(page):
    page.set_viewport_size({"width": 1280, "height": 800})
    page.goto(f"{USER_WEB_URL}/#/genpic", wait_until="domcontentloaded")
    compact_prompt = page.get_by_placeholder("描述想要生成的内容...")
    expect(compact_prompt).to_be_visible(timeout=15000)
    compact_prompt.fill(ORIGINAL_PROMPT)
    page.get_by_role("button", name="展开提示词编辑器").click()
    dialog = page.get_by_role("dialog", name="提示词编辑器")
    expect(dialog).to_be_visible()
    assert_dialog_above_toasts(page)
    expanded_prompt = page.locator("#expanded-prompt-editor")
    expect(expanded_prompt).to_have_value(ORIGINAL_PROMPT)
    dialog.get_by_role("button", name="优化提示词").click()
    expect(dialog.get_by_text("预计 0.00000 积分", exact=False)).to_be_visible(timeout=15000)
    assert_no_overlap(page)
    page.screenshot(path=OUTPUT_DIR / "desktop-prompt-confirm.png", full_page=True)
    dialog.get_by_role("button", name="确认优化").click()
    expect(dialog.get_by_text("优化后", exact=True)).to_be_visible(timeout=15000)
    expect(dialog.get_by_text("Optimized chat prompt for Docker E2E", exact=True)).to_be_visible()
    assert_no_overlap(page)
    page.screenshot(path=OUTPUT_DIR / "desktop-prompt-compare.png", full_page=True)
    dialog.get_by_role("button", name="应用优化").click()
    expect(expanded_prompt).to_have_value("Optimized chat prompt for Docker E2E")
    dialog.get_by_role("button", name="撤销提示词优化").click()
    expect(expanded_prompt).to_have_value(ORIGINAL_PROMPT)
    dialog.get_by_role("button", name="关闭").click()
    expect(compact_prompt).to_have_value(ORIGINAL_PROMPT)

    compact_prompt.fill("Compact optimizer entry")
    page.get_by_role("button", name="优化提示词").click()
    compact_dialog = page.get_by_role("dialog", name="优化提示词")
    expect(compact_dialog.get_by_text("预计 0.00000 积分", exact=False)).to_be_visible(timeout=15000)
    compact_dialog.get_by_role("button", name="取消").click()


def verify_mobile_expansion(page):
    page.set_viewport_size({"width": 390, "height": 844})
    page.goto(f"{USER_WEB_URL}/#/genpic", wait_until="domcontentloaded")
    page.locator('[data-workspace-sheet-handle="true"]').click()
    prompt = page.get_by_placeholder("描述想要生成的内容...")
    expect(prompt).to_be_visible(timeout=15000)
    prompt.fill("Mobile expanded prompt")
    page.get_by_role("button", name="展开提示词编辑器").click()
    dialog = page.get_by_role("dialog", name="提示词编辑器")
    expect(dialog).to_be_visible()
    assert_dialog_above_toasts(page)
    expect(page.locator("#expanded-prompt-editor")).to_have_value("Mobile expanded prompt")
    expect(dialog.get_by_role("button", name="优化提示词")).to_be_visible()
    assert_no_overlap(page)
    page.screenshot(path=OUTPUT_DIR / "mobile-prompt-expanded.png", full_page=True)
    dialog.get_by_role("button", name="关闭").click()


def verify_prompt_copy_and_reuse(context, page):
    page.set_viewport_size({"width": 1280, "height": 800})
    context.grant_permissions(["clipboard-read", "clipboard-write"], origin=USER_WEB_URL)
    page.goto(f"{USER_WEB_URL}/#/gallery", wait_until="domcontentloaded")
    open_image = page.get_by_role("button", name=re.compile(r"查看docker e2e prompt", re.IGNORECASE)).first
    expect(open_image).to_be_visible(timeout=15000)
    open_image.click()
    dialog = page.get_by_role("dialog", name="图片详情")
    expect(dialog.get_by_text("docker e2e prompt", exact=True)).to_be_visible(timeout=15000)
    dialog.get_by_role("button", name="复制 Prompt").click()
    copied = page.evaluate("navigator.clipboard.readText()")
    assert copied == "docker e2e prompt", f"full prompt was not copied: {copied!r}"
    dialog.get_by_role("button", name="复用配置").click()
    prompt = page.get_by_placeholder("描述想要生成的内容...")
    expect(prompt).to_have_value("docker e2e prompt", timeout=15000)
    expect(page.get_by_text("Basic", exact=True).first).to_be_visible()
    assert_no_overlap(page)
    page.screenshot(path=OUTPUT_DIR / "desktop-reused-configuration.png", full_page=True)


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1280, "height": 800})
        page = context.new_page()
        try:
            install_sessions(context, page)
            verify_admin_text_models(page)
            verify_prompt_optimization(page)
            verify_mobile_expansion(page)
            verify_prompt_copy_and_reuse(context, page)
        except Exception:
            page.screenshot(path=OUTPUT_DIR / "failure.png", full_page=True)
            (OUTPUT_DIR / "failure-url.txt").write_text(page.url, encoding="utf-8")
            raise
        finally:
            context.close()
            browser.close()
    print(json.dumps({"status": "pass", "screenshots": str(OUTPUT_DIR)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
