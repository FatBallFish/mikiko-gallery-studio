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
E2E_ADMIN_EMAIL = os.environ.get("E2E_ADMIN_EMAIL", "admin@example.com")
E2E_ADMIN_PASSWORD = os.environ.get("E2E_ADMIN_PASSWORD", "admin123456")
RUN_ID = os.environ["E2E_RUN_ID"]
OUTPUT_DIR = Path(os.environ.get("E2E_BROWSER_OUTPUT_DIR", "tmp/e2e/prompt-workflow"))
ORIGINAL_PROMPT = "A quiet glass pavilion beside a rain-soaked garden"


def prompt_editor(scope):
    return scope.get_by_role("textbox", name="提示词", exact=True)


def expect_prompt(editor, value):
    actual = editor.evaluate("""root => {
      const serialize = node => {
        if (node.nodeType === Node.TEXT_NODE) return node.textContent || '';
        if (!(node instanceof HTMLElement)) return '';
        const kind = node.dataset.promptTokenKind;
        const name = node.dataset.promptTokenName;
        if (kind && name) return kind === 'reference' ? `{{@${name}}}` : `{{$${name}}}`;
        if (node.tagName === 'BR') return node.nextSibling ? '\\n' : '';
        return Array.from(node.childNodes).map(serialize).join('');
      };
      return Array.from(root.childNodes).map(serialize).join('');
    }""")
    assert actual == value, f"expected prompt {value!r}, got {actual!r}"


def replace_prompt(page, editor, value):
    editor.click()
    editor.press("ControlOrMeta+A")
    editor.press("Backspace")
    expect_prompt(editor, "")
    page.wait_for_timeout(1)
    editor.press("Shift")
    editor.fill(value)
    expect_prompt(editor, value)
    shell = editor.locator("xpath=ancestor::*[contains(concat(' ', normalize-space(@class), ' '), ' prompt-template-shell ')]")
    expect(shell.locator(".prompt-template-count")).to_have_text(f"{len(value)} / 4000")


def set_prompt_caret(editor, offset):
    editor.evaluate("""(root, offset) => {
      root.focus();
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      let remaining = offset;
      let node = walker.nextNode();
      while (node && remaining > node.textContent.length) {
        remaining -= node.textContent.length;
        node = walker.nextNode();
      }
      if (!node) throw new Error(`prompt caret offset ${offset} is outside the editor`);
      const selection = window.getSelection();
      const range = document.createRange();
      range.setStart(node, Math.min(remaining, node.textContent.length));
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
      document.dispatchEvent(new Event('selectionchange'));
    }""", offset)


def set_prompt_caret_at_end(editor):
    editor.evaluate("""root => {
      root.focus();
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      let node = walker.nextNode();
      let last = node;
      while (node) {
        last = node;
        node = walker.nextNode();
      }
      if (!last) throw new Error('prompt editor has no text node');
      const selection = window.getSelection();
      const range = document.createRange();
      range.setStart(last, last.textContent.length);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
      document.dispatchEvent(new Event('selectionchange'));
    }""")


def select_prompt_token(token):
    token.evaluate("""node => {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNode(node);
      selection.removeAllRanges();
      selection.addRange(range);
    }""")


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
    user_session = json.dumps({"token": USER_TOKEN, "profile": profile})
    context.add_init_script(
        f"localStorage.setItem('pic-gallery-user-session', {json.dumps(user_session)});"
    )
    page.goto(f"{USER_WEB_URL}/", wait_until="domcontentloaded")

    admin_login = envelope(context.request.post(
        f"{BASE_URL}/api/ops/admin/v1/auth/login",
        data={"email": E2E_ADMIN_EMAIL, "password": E2E_ADMIN_PASSWORD},
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
    compact_prompt = prompt_editor(page)
    expect(compact_prompt).to_be_visible(timeout=15000)
    model_group = page.locator("#workspace-model-group")
    expect(model_group.locator(".model-group-select-copy strong")).not_to_be_empty()
    expect(model_group.locator(".model-group-select-subject")).to_contain_text("◈")
    model_group.click()
    model_listbox = page.get_by_role("listbox", name="选择模型分组")
    expect(model_listbox).to_be_visible()
    selected_model_option = model_listbox.get_by_role("option", selected=True)
    expect(selected_model_option.locator(".model-group-select-copy strong")).not_to_be_empty()
    expect(selected_model_option.locator(".model-group-select-subject")).to_contain_text("◈")
    expect(selected_model_option).to_be_focused()
    page.keyboard.press("Escape")
    expect(model_group).to_be_focused()
    compact_prompt.fill("Create a {{$subject}} portrait")
    variable_token = compact_prompt.locator('[data-prompt-token-kind="variable"][data-prompt-token-name="subject"]')
    expect(variable_token).to_be_visible(timeout=15000)
    expect(variable_token.locator(".prompt-token-label")).to_have_text("subject")
    expect(variable_token).not_to_contain_text("{{$")
    variable_input = page.get_by_placeholder('填写“subject”的内容')
    expect(variable_input).to_be_visible()
    variable_input.fill("glass astronaut")
    variable_token.hover()
    expect(page.locator(".prompt-token-preview").get_by_text("glass astronaut", exact=True)).to_be_visible()
    variable_token.focus()
    expect(variable_token).to_have_attribute("tabindex", "0")
    expect(variable_token).to_have_attribute("aria-label", re.compile(r"subject"))
    expect(page.locator(".prompt-token-preview").get_by_text("glass astronaut", exact=True)).to_be_visible()

    replace_prompt(page, compact_prompt, "{{$已有}} ")
    set_prompt_caret_at_end(compact_prompt)
    page.get_by_role("button", name="插入变量").first.click()
    variable_menu = page.get_by_role("listbox", name="选择或添加变量")
    variable_name_input = variable_menu.get_by_placeholder("新变量名称")
    variable_name_input.fill("{")
    expect(variable_menu.get_by_role("button", name="添加", exact=True)).to_be_disabled()
    expect(variable_menu.get_by_text("占位符名称包含不允许的字符", exact=True)).to_be_visible()
    variable_name_input.fill("新建")
    variable_name_input.press("Enter")
    expect(compact_prompt.locator('[data-prompt-token-name="新建"]')).to_be_visible()

    set_prompt_caret_at_end(compact_prompt)
    page.get_by_role("button", name="插入变量").first.click()
    variable_menu = page.get_by_role("listbox", name="选择或添加变量")
    keyboard_options = variable_menu.get_by_role("option")
    assert keyboard_options.count() >= 2, "variable keyboard navigation requires two existing options"
    keyboard_target = keyboard_options.nth(1).locator("strong").inner_text()
    existing_target_count = compact_prompt.locator(f'[data-prompt-token-name="{keyboard_target}"]').count()
    keyboard_input = variable_menu.get_by_placeholder("新变量名称")
    expect(keyboard_input).to_be_focused()
    keyboard_input.press("ArrowDown")
    keyboard_input.press("Enter")
    expect(compact_prompt.locator(f'[data-prompt-token-name="{keyboard_target}"]')).to_have_count(existing_target_count + 1)

    page.get_by_role("button", name="插入变量").first.click()
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_be_visible()
    compact_prompt.press("Escape")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_have_count(0)
    expect(compact_prompt).to_be_focused()

    replace_prompt(page, compact_prompt, "前后")
    set_prompt_caret(compact_prompt, 1)
    page.get_by_role("button", name="插入变量").first.click()
    variable_menu = page.get_by_role("listbox", name="选择或添加变量")
    variable_menu.get_by_placeholder("新变量名称").fill("镜头")
    variable_menu.get_by_role("button", name="添加", exact=True).click()
    expect_prompt(compact_prompt, "前{{$镜头}} 后")

    replace_prompt(page, compact_prompt, "场景 ")
    set_prompt_caret_at_end(compact_prompt)
    compact_prompt.press("$")
    variable_menu = page.get_by_role("listbox", name="选择或添加变量")
    expect(variable_menu).to_be_visible()
    variable_menu.get_by_placeholder("新变量名称").fill("光线")
    variable_menu.get_by_role("button", name="添加", exact=True).click()
    expect(compact_prompt.locator('[data-prompt-token-name="光线"]')).to_be_visible()

    replace_prompt(page, compact_prompt, "前置")
    set_prompt_caret(compact_prompt, len("前置"))
    compact_prompt.press("X")
    page.evaluate("navigator.clipboard.writeText('Y')")
    compact_prompt.press("ControlOrMeta+V")
    expect_prompt(compact_prompt, "前置XY")
    page.get_by_role("button", name="撤销").first.click()
    expect_prompt(compact_prompt, "前置X")
    page.get_by_role("button", name="重做").first.click()
    expect_prompt(compact_prompt, "前置XY")

    page.evaluate("text => navigator.clipboard.writeText(text)", "粘贴\n{{$材质}}")
    compact_prompt.click()
    compact_prompt.press("ControlOrMeta+A")
    compact_prompt.press("ControlOrMeta+V")
    expect(compact_prompt.locator('[data-prompt-token-name="材质"]')).to_be_visible()
    expect_prompt(compact_prompt, "粘贴\n{{$材质}}")
    compact_prompt.press("End")
    compact_prompt.press("Space")
    page.keyboard.insert_text("完成")
    expect(compact_prompt).to_contain_text("完成")
    page.get_by_role("button", name="撤销").first.click()
    expect(compact_prompt).not_to_contain_text("完成")
    page.get_by_role("button", name="重做").first.click()
    expect(compact_prompt).to_contain_text("完成")

    material_token = compact_prompt.locator('[data-prompt-token-name="材质"]')
    select_prompt_token(material_token)
    compact_prompt.press("Delete")
    expect(material_token).to_have_count(0)

    replace_prompt(page, compact_prompt, "")
    compact_prompt.dispatch_event("compositionstart")
    page.keyboard.insert_text("中文$输入")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_have_count(0)
    compact_prompt.dispatch_event("compositionend")
    expect_prompt(compact_prompt, "中文$输入")

    replace_prompt(page, compact_prompt, "中文文本")
    compact_prompt.press("End")
    compact_prompt.press("$")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_be_visible()
    assert compact_prompt.evaluate("node => document.activeElement === node"), "typing $ must keep focus in the prompt editor"
    compact_prompt.press("Escape")
    expect_prompt(compact_prompt, "中文文本$")
    page.keyboard.insert_text("普通文本")
    expect_prompt(compact_prompt, "中文文本$普通文本")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_have_count(0)
    page.keyboard.insert_text(" $")
    expect_prompt(compact_prompt, "中文文本$普通文本 $")
    variable_menu = page.get_by_role("listbox", name="选择或添加变量")
    expect(variable_menu).to_be_visible()
    page.get_by_text("图片创作", exact=True).click()
    expect(variable_menu).to_have_count(0)
    compact_prompt.click()
    set_prompt_caret_at_end(compact_prompt)
    expect(variable_menu).to_have_count(0)
    page.keyboard.insert_text("继续输入")
    expect(variable_menu).to_have_count(0)

    replace_prompt(page, compact_prompt, "$普通文本 @")
    reference_menu = page.get_by_role("listbox", name="选择资产")
    expect(reference_menu).to_be_visible()
    compact_prompt.press("Escape")
    set_prompt_caret(compact_prompt, 1)
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_be_visible()
    compact_prompt.press("Escape")

    replace_prompt(page, compact_prompt, "删除后重输")
    compact_prompt.press("End")
    compact_prompt.press("$")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_be_visible()
    compact_prompt.press("Escape")
    compact_prompt.press("Backspace")
    compact_prompt.press("$")
    expect(page.get_by_role("listbox", name="选择或添加变量")).to_be_visible()
    compact_prompt.press("Escape")

    replace_prompt(page, compact_prompt, "{{$重复}} + {{$重复}}")
    repeated_tokens = compact_prompt.locator('[data-prompt-token-name="重复"]')
    expect(repeated_tokens).to_have_count(2)
    repeated_tokens.first.get_by_role("button", name="删除当前变量 重复").click()
    expect(repeated_tokens).to_have_count(1)
    expect_prompt(compact_prompt, " + {{$重复}}")

    replace_prompt(page, compact_prompt, "使用 ")
    compact_prompt.press("End")
    page.get_by_role("button", name="插入资产").first.click()
    page.get_by_role("button", name="添加资产").click()
    import_dialog = page.get_by_role("dialog", name="从资产导入参考图")
    expect(import_dialog).to_be_visible(timeout=15000)
    import_dialog.get_by_role("button", name=re.compile(r"docker e2e prompt", re.IGNORECASE)).first.click()
    import_dialog.get_by_role("button", name="确定", exact=True).click()
    reference_token = compact_prompt.locator('[data-prompt-token-kind="reference"]')
    expect(reference_token).to_be_visible(timeout=15000)
    reference_name = reference_token.get_attribute("data-prompt-token-name")
    assert reference_name, "gallery toolbar insertion did not create a named reference token"

    page.get_by_role("button", name=f"重命名资产 {reference_name}").click()
    renamed_reference = f"浏览器引用-{RUN_ID}"
    rename_dialog = page.get_by_role("dialog", name="重命名资产")
    expect(rename_dialog).to_be_visible()
    rename_dialog.get_by_role("textbox", name="资产名称").fill(renamed_reference)
    rename_dialog.get_by_role("button", name="保存", exact=True).click()
    expect(compact_prompt.locator(f'[data-prompt-token-name="{renamed_reference}"]')).to_be_visible(timeout=15000)
    expect(compact_prompt.locator(f'[data-prompt-token-name="{reference_name}"]')).to_have_count(0)
    reference_name = renamed_reference

    replace_prompt(page, compact_prompt, "再次 ")
    compact_prompt.press("End")
    compact_prompt.press("@")
    reference_menu = page.get_by_role("listbox", name="选择资产")
    expect(reference_menu).to_be_visible()
    reference_menu.get_by_role("option").filter(has_text=reference_name).click()
    expect(compact_prompt.locator(f'[data-prompt-token-name="{reference_name}"]')).to_be_visible()

    replace_prompt(page, compact_prompt, "第一行")
    compact_prompt.press("End")
    compact_prompt.press("Enter")
    page.keyboard.insert_text("第二行")
    expect(page.get_by_text("7 / 4000", exact=True)).to_be_visible()

    replace_prompt(page, compact_prompt, ORIGINAL_PROMPT)
    expect(page.get_by_text(f"{len(ORIGINAL_PROMPT)} / 4000", exact=True)).to_be_visible()
    page.get_by_role("button", name="展开提示词编辑器").click()
    dialog = page.get_by_role("dialog", name="提示词编辑器")
    expect(dialog).to_be_visible()
    assert_dialog_above_toasts(page)
    expanded_prompt = prompt_editor(dialog)
    expect_prompt(expanded_prompt, ORIGINAL_PROMPT)
    set_prompt_caret(expanded_prompt, len(ORIGINAL_PROMPT))
    dialog.get_by_role("button", name="插入变量").click()
    expanded_variable_menu = dialog.get_by_role("listbox", name="选择或添加变量")
    expanded_variable_menu.get_by_placeholder("新变量名称").fill("弹窗变量")
    expanded_variable_menu.get_by_role("button", name="添加", exact=True).click()
    expect(expanded_prompt.locator('[data-prompt-token-name="弹窗变量"]')).to_be_visible()
    dialog.get_by_role("button", name="撤销").click()
    expect_prompt(expanded_prompt, ORIGINAL_PROMPT)
    dialog.get_by_role("button", name="重做").click()
    expect(expanded_prompt.locator('[data-prompt-token-name="弹窗变量"]')).to_be_visible()
    dialog.get_by_role("button", name="撤销").click()
    replace_prompt(page, expanded_prompt, ORIGINAL_PROMPT)
    set_prompt_caret_at_end(expanded_prompt)
    page.keyboard.insert_text("$")
    expanded_variable_menu = dialog.get_by_role("listbox", name="选择或添加变量")
    expect(expanded_variable_menu).to_be_visible()
    expanded_variable_menu.get_by_placeholder("新变量名称").fill("弹窗触发")
    expanded_variable_menu.get_by_role("button", name="添加", exact=True).click()
    expect(expanded_prompt.locator('[data-prompt-token-name="弹窗触发"]')).to_be_visible()
    page.evaluate("navigator.clipboard.writeText('弹窗粘贴 {{$弹窗材质}}')")
    expanded_prompt.click()
    expanded_prompt.press("ControlOrMeta+A")
    expanded_prompt.press("ControlOrMeta+V")
    expanded_material_token = expanded_prompt.locator('[data-prompt-token-name="弹窗材质"]')
    expect(expanded_material_token).to_be_visible()
    select_prompt_token(expanded_material_token)
    expanded_prompt.press("Delete")
    expect(expanded_material_token).to_have_count(0)
    replace_prompt(page, expanded_prompt, "")
    expanded_prompt.dispatch_event("compositionstart")
    page.keyboard.insert_text("弹窗中文$输入")
    expect(dialog.get_by_role("listbox", name="选择或添加变量")).to_have_count(0)
    expanded_prompt.dispatch_event("compositionend")
    expect_prompt(expanded_prompt, "弹窗中文$输入")
    replace_prompt(page, expanded_prompt, ORIGINAL_PROMPT)
    expanded_prompt.press("End")
    expanded_prompt.press("@")
    expanded_reference_menu = dialog.get_by_role("listbox", name="选择资产")
    expect(expanded_reference_menu).to_be_visible()
    expanded_reference_menu.get_by_role("option").filter(has_text=reference_name).click()
    expanded_reference_token = expanded_prompt.locator(f'[data-prompt-token-name="{reference_name}"]')
    expect(expanded_reference_token).to_be_visible()
    dialog.get_by_role("button", name="撤销").click()
    expect(expanded_reference_token).to_have_count(0)
    if expanded_prompt.inner_text().endswith("@"):
        dialog.get_by_role("button", name="撤销").click()
    expect_prompt(expanded_prompt, ORIGINAL_PROMPT)
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
    expect_prompt(expanded_prompt, "Optimized chat prompt for Docker E2E")
    dialog.get_by_role("button", name="撤销提示词优化").click()
    expect_prompt(expanded_prompt, ORIGINAL_PROMPT)
    dialog.get_by_role("button", name="关闭").click()
    expect_prompt(compact_prompt, ORIGINAL_PROMPT)

    compact_prompt.fill("Compact optimizer entry")
    page.get_by_role("button", name="优化提示词").click()
    compact_dialog = page.get_by_role("dialog", name="优化提示词")
    expect(compact_dialog.get_by_text("预计 0.00000 积分", exact=False)).to_be_visible(timeout=15000)
    compact_dialog.get_by_role("button", name="取消").click()


def drag_pointer(page, pointer_type, start, end, modifiers=0):
    session = page.context.new_cdp_session(page)
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": start[0], "y": start[1], "modifiers": modifiers, "pointerType": pointer_type})
    session.send("Input.dispatchMouseEvent", {"type": "mousePressed", "x": start[0], "y": start[1], "button": "left", "buttons": 1, "clickCount": 1, "modifiers": modifiers, "pointerType": pointer_type})
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": end[0], "y": end[1], "button": "left", "buttons": 1, "modifiers": modifiers, "pointerType": pointer_type})
    page.wait_for_timeout(120)
    session.send("Input.dispatchMouseEvent", {"type": "mouseReleased", "x": end[0], "y": end[1], "button": "left", "buttons": 0, "clickCount": 1, "modifiers": modifiers, "pointerType": pointer_type})
    session.detach()


def verify_gallery_selection(page):
    page.set_viewport_size({"width": 1280, "height": 1000})
    page.goto(f"{USER_WEB_URL}/#/gallery", wait_until="domcontentloaded")
    cards = page.locator("[data-gallery-image-id]")
    expect(cards.first).to_be_visible(timeout=15000)
    assert cards.count() >= 2, "gallery selection browser test requires at least two assets"
    first, second = cards.nth(0), cards.nth(1)
    first_select = first.locator("[data-gallery-selection-control] button")
    second_select = second.locator("[data-gallery-selection-control] button")
    first_select.click()
    expect(first_select).to_have_attribute("aria-pressed", "true")
    visual = first_select.locator("span").evaluate("element => ({ opacity: Number(getComputedStyle(element).opacity), width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height })")
    assert visual["opacity"] >= 0.8 and visual["width"] >= 20 and visual["height"] >= 20, f"selected indicator lacks contrast or size: {visual}"
    second.locator("[data-gallery-card-open]").click()
    expect(second_select).to_have_attribute("aria-pressed", "true")
    expect(page.get_by_role("dialog", name="图片详情")).to_have_count(0)
    page.keyboard.press("Escape")
    expect(first_select).to_have_attribute("aria-pressed", "false")
    expect(second_select).to_have_attribute("aria-pressed", "false")

    first_box = first.locator("[data-gallery-card-open]").bounding_box()
    second_box = second.locator("[data-gallery-card-open]").bounding_box()
    assert first_box and second_box
    drag_pointer(page, "mouse", (first_box["x"] + first_box["width"] / 2, first_box["y"] + first_box["height"] / 2), (second_box["x"] + second_box["width"] / 2, second_box["y"] + second_box["height"] / 2))
    expect(first_select).to_have_attribute("aria-pressed", "true")
    expect(second_select).to_have_attribute("aria-pressed", "true")
    page.keyboard.press("Escape")
    drag_pointer(page, "mouse", (first_box["x"] + first_box["width"] * 0.35, first_box["y"] + first_box["height"] * 0.35), (first_box["x"] + first_box["width"] * 0.65, first_box["y"] + first_box["height"] * 0.65))
    drag_pointer(page, "mouse", (second_box["x"] + second_box["width"] * 0.45, second_box["y"] + second_box["height"] * 0.45), (second_box["x"] + second_box["width"] * 0.55, second_box["y"] + second_box["height"] * 0.55), modifiers=4)
    expect(first_select).to_have_attribute("aria-pressed", "true")
    expect(second_select).to_have_attribute("aria-pressed", "true")
    page.keyboard.press("Escape")
    drag_pointer(page, "pen", (first_box["x"] + first_box["width"] * 0.35, first_box["y"] + first_box["height"] * 0.35), (first_box["x"] + first_box["width"] * 0.65, first_box["y"] + first_box["height"] * 0.65))
    expect(first_select).to_have_attribute("aria-pressed", "true")
    page.keyboard.press("Escape")

    drag_pointer(page, "mouse", (first_box["x"] + first_box["width"] * 0.35, first_box["y"] + first_box["height"] * 0.35), (first_box["x"] + first_box["width"] * 0.65, first_box["y"] + first_box["height"] * 0.65))
    drag_pointer(page, "mouse", (second_box["x"] + second_box["width"] * 0.45, second_box["y"] + second_box["height"] * 0.45), (second_box["x"] + second_box["width"] * 0.55, second_box["y"] + second_box["height"] * 0.55), modifiers=2)
    expect(first_select).to_have_attribute("aria-pressed", "true")
    expect(second_select).to_have_attribute("aria-pressed", "true")
    page.keyboard.press("Escape")

    first.hover()
    first.get_by_role("button", name="复制提示词").click()
    expect(first_select).to_have_attribute("aria-pressed", "false")

    selection_surface = page.locator("[data-gallery-selection-surface]")
    surface_box = selection_surface.bounding_box()
    assert surface_box
    edge_x = surface_box["x"] + surface_box["width"] - 2
    edge_y = surface_box["y"] + min(40, surface_box["height"] / 2)
    session = page.context.new_cdp_session(page)
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": edge_x, "y": edge_y, "pointerType": "mouse"})
    session.send("Input.dispatchMouseEvent", {"type": "mousePressed", "x": edge_x, "y": edge_y, "button": "left", "buttons": 1, "clickCount": 1, "pointerType": "mouse"})
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": edge_x + 10, "y": edge_y, "button": "left", "buttons": 1, "pointerType": "mouse"})
    expect(page.locator(".gallery-selection-marquee")).to_be_visible()
    session.send("Input.dispatchMouseEvent", {"type": "mouseReleased", "x": edge_x + 10, "y": edge_y, "button": "left", "buttons": 0, "clickCount": 1, "pointerType": "mouse"})
    session.detach()
    page.keyboard.press("Escape")

    filter_input = page.get_by_placeholder("搜索标题、提示词或模型")
    selected_project = page.get_by_role("combobox", name="当前项目").input_value()
    first_title = first_select.get_attribute("aria-label").removeprefix("选择 ")
    first_select.click()
    second_select.click()
    filter_input.fill(first_title)
    expect(page.get_by_text("已选择 1 个已加载资产", exact=True)).to_be_visible()
    matching_select = page.locator(f'[data-gallery-image-id="{first.get_attribute("data-gallery-image-id")}"] [data-gallery-selection-control] button')
    expect(matching_select).to_have_attribute("aria-pressed", "true")
    page.get_by_role("button", name="刷新资产").click()
    expect(filter_input).to_have_value(first_title)
    expect(page.get_by_role("combobox", name="当前项目")).to_have_value(selected_project)
    expect(matching_select).to_have_attribute("aria-pressed", "true")
    page.keyboard.press("Escape")
    filter_input.fill("")

    page.set_viewport_size({"width": 1280, "height": 520})
    main_scroll = page.locator("main")
    main_scroll.evaluate("element => { element.scrollTop = 0 }")
    first_box = first.locator("[data-gallery-card-open]").bounding_box()
    assert first_box
    start_scroll = main_scroll.evaluate("element => element.scrollTop")
    session = page.context.new_cdp_session(page)
    start_x = first_box["x"] + first_box["width"] / 2
    start_y = first_box["y"] + first_box["height"] / 2
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": start_x, "y": start_y, "pointerType": "mouse"})
    session.send("Input.dispatchMouseEvent", {"type": "mousePressed", "x": start_x, "y": start_y, "button": "left", "buttons": 1, "clickCount": 1, "pointerType": "mouse"})
    session.send("Input.dispatchMouseEvent", {"type": "mouseMoved", "x": start_x, "y": 516, "button": "left", "buttons": 1, "pointerType": "mouse"})
    page.wait_for_timeout(350)
    session.send("Input.dispatchMouseEvent", {"type": "mouseReleased", "x": start_x, "y": 516, "button": "left", "buttons": 0, "clickCount": 1, "pointerType": "mouse"})
    session.detach()
    assert main_scroll.evaluate("element => element.scrollTop") > start_scroll, "gallery marquee edge scrolling did not advance the viewport"
    page.keyboard.press("Escape")

    main_scroll.evaluate("element => { element.scrollTop = 0 }")
    session = page.context.new_cdp_session(page)
    session.send("Emulation.setTouchEmulationEnabled", {"enabled": True, "maxTouchPoints": 1})
    session.send("Input.dispatchTouchEvent", {"type": "touchStart", "touchPoints": [{"x": 640, "y": 430, "radiusX": 2, "radiusY": 2, "force": 1, "id": 1}]})
    session.send("Input.dispatchTouchEvent", {"type": "touchMove", "touchPoints": [{"x": 640, "y": 180, "radiusX": 2, "radiusY": 2, "force": 1, "id": 1}]})
    session.send("Input.dispatchTouchEvent", {"type": "touchEnd", "touchPoints": []})
    page.wait_for_timeout(250)
    session.send("Emulation.setTouchEmulationEnabled", {"enabled": False})
    session.detach()
    assert main_scroll.evaluate("element => element.scrollTop") > 0, "touch gesture must scroll instead of entering marquee selection"
    expect(first_select).to_have_attribute("aria-pressed", "false")

    def abort_image_requests(route):
        if route.request.resource_type == "image":
            route.abort()
        else:
            route.continue_()

    page.route("**/*", abort_image_requests)
    page.reload(wait_until="domcontentloaded")
    page.locator("main").evaluate("element => { element.scrollTop = 0 }")
    retry = page.locator("[data-gallery-selection-surface]").get_by_role("button", name="重试").first
    expect(retry).to_be_visible(timeout=15000)
    retry.click()
    expect(page.get_by_role("dialog", name="图片详情")).to_have_count(0)
    page.unroute("**/*", abort_image_requests)


def verify_mobile_expansion(page):
    page.set_viewport_size({"width": 390, "height": 844})
    page.goto(f"{USER_WEB_URL}/#/genpic", wait_until="domcontentloaded")
    page.locator('[data-workspace-sheet-handle="true"]').click()
    prompt = prompt_editor(page)
    expect(prompt).to_be_visible(timeout=15000)
    prompt.fill("Mobile expanded prompt")
    page.get_by_role("button", name="展开提示词编辑器").click()
    dialog = page.get_by_role("dialog", name="提示词编辑器")
    expect(dialog).to_be_visible()
    assert_dialog_above_toasts(page)
    expect_prompt(prompt_editor(dialog), "Mobile expanded prompt")
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
    prompt = prompt_editor(page)
    expect(prompt).to_have_text("docker e2e prompt", timeout=15000)
    expect(page.locator("#workspace-model-group")).to_have_attribute("data-value", "basic")
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
            context.grant_permissions(["clipboard-read", "clipboard-write"], origin=USER_WEB_URL)
            verify_admin_text_models(page)
            verify_prompt_optimization(page)
            verify_gallery_selection(page)
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
