#!/usr/bin/env node

import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const baseURL = process.env.ADMIN_BASE_URL || 'http://127.0.0.1:8088/admin/'
const adminEmail = process.env.ADMIN_EMAIL || 'admin@example.com'
const adminPassword = process.env.ADMIN_PASSWORD || 'admin123456'
const outputDir = resolve(process.cwd(), process.env.ADMIN_SNAPSHOT_DIR || 'docs/audits/admin-ui-ux-2026-07-07/screenshots/post-remediation')

const desktopRoutes = [
  'dashboard',
  'monitoring',
  'users',
  'user-groups',
  'reviews',
  'orders',
  'packages',
  'cashier-config',
  'access-accounts',
  'routing',
  'pricing',
  'call-records',
  'audit',
  'system-users',
  'system-settings',
  'system-settings?tab=security',
  'system-settings?tab=storage',
]

const mobileRoutes = ['dashboard', 'users', 'system-settings?tab=storage']

async function main() {
  const playwright = await importPlaywright()
  if (!playwright) {
    runPythonPlaywright()
    return
  }
  const { chromium } = playwright
  await mkdir(outputDir, { recursive: true })

  const browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } })
  try {
    await login(page)
    for (const route of desktopRoutes) {
      await captureRoute(page, route, 'desktop')
    }

    await page.setViewportSize({ width: 390, height: 844 })
    for (const route of mobileRoutes) {
      await captureRoute(page, route, 'mobile')
    }
  } finally {
    await browser.close()
  }
  console.log(`Admin UI snapshots written to ${outputDir}`)
}

async function importPlaywright() {
  try {
    return await import('playwright')
  } catch {
    return null
  }
}

function runPythonPlaywright() {
  const source = String.raw`
import json
import os
from pathlib import Path
from playwright.sync_api import sync_playwright

base_url = os.environ.get("ADMIN_BASE_URL", ${JSON.stringify(baseURL)})
admin_email = os.environ.get("ADMIN_EMAIL", ${JSON.stringify(adminEmail)})
admin_password = os.environ.get("ADMIN_PASSWORD", ${JSON.stringify(adminPassword)})
output_dir = Path(os.environ.get("ADMIN_SNAPSHOT_DIR", ${JSON.stringify(outputDir)}))
desktop_routes = ${JSON.stringify(desktopRoutes)}
mobile_routes = ${JSON.stringify(mobileRoutes)}
output_dir.mkdir(parents=True, exist_ok=True)

def assert_page(page, route, viewport_name):
    result = page.evaluate("""() => {
      const text = document.body?.innerText || '';
      const oldChromeTerms = ['CURRENT VIEW', 'ADMIN ROLE', 'QUEUE ALERTS', 'PENDING REVIEW'];
      const pageHeaderActions = document.querySelector('header > div:last-child');
      const visiblePrimaryButtons = Array.from((pageHeaderActions || document.createElement('div')).querySelectorAll('button'))
        .filter((button) => {
          const rect = button.getBoundingClientRect();
          const style = getComputedStyle(button);
          if (rect.width <= 0 || rect.height <= 0 || style.visibility === 'hidden' || style.display === 'none') return false;
          return String(button.className).includes('bg-[var(--accent)]');
        })
        .map((button) => button.textContent?.trim() || button.getAttribute('aria-label') || '');
      return {
        forbidden: oldChromeTerms.filter((term) => text.includes(term)),
        primaryCount: visiblePrimaryButtons.length,
        primaryButtons: visiblePrimaryButtons,
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
      };
    }""")
    if result["scrollWidth"] > result["innerWidth"] + 4:
      raise AssertionError(f"{viewport_name}/{route} has horizontal overflow: {result['scrollWidth']} > {result['innerWidth']}")
    if result["forbidden"]:
      raise AssertionError(f"{viewport_name}/{route} still contains old shell terms: {', '.join(result['forbidden'])}")
    if result["primaryCount"] > 1:
      raise AssertionError(f"{viewport_name}/{route} has too many primary buttons: {', '.join(result['primaryButtons'])}")

def login(page):
    page.goto(base_url, wait_until="networkidle")
    if page.get_by_text("运营总览").count() > 0:
      return
    page.get_by_label("管理员邮箱").fill(admin_email)
    page.get_by_label("密码").fill(admin_password)
    page.get_by_role("button", name="进入控制台").click()
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(5500)

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1280, "height": 720})
    try:
      login(page)
      for route in desktop_routes:
        page.goto(f"{base_url}#/{route}", wait_until="networkidle")
        page.wait_for_timeout(250)
        assert_page(page, route, "desktop")
        page.screenshot(path=str(output_dir / f"desktop-{route}.png"), full_page=True)

      page.set_viewport_size({"width": 390, "height": 844})
      for route in mobile_routes:
        page.goto(f"{base_url}#/{route}", wait_until="networkidle")
        page.wait_for_timeout(250)
        assert_page(page, route, "mobile")
        page.screenshot(path=str(output_dir / f"mobile-{route}.png"), full_page=True)
    finally:
      browser.close()
print(f"Admin UI snapshots written to {output_dir}")
`
  const result = spawnSync('python3', ['-c', source], { stdio: 'inherit', env: process.env })
  if (result.status !== 0) process.exit(result.status ?? 1)
}

async function login(page) {
  await page.goto(baseURL, { waitUntil: 'networkidle' })
  if (await page.getByText('运营总览').count()) return
  await page.getByLabel('管理员邮箱').fill(adminEmail)
  await page.getByLabel('密码').fill(adminPassword)
  await page.getByRole('button', { name: '进入控制台' }).click()
  await page.waitForURL(/#\/dashboard|\/admin\/?$/, { timeout: 15000 })
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(5500)
}

async function captureRoute(page, route, viewportName) {
  await page.goto(`${baseURL}#/${route}`, { waitUntil: 'networkidle' })
  await page.waitForTimeout(250)
  const result = await page.evaluate(() => {
    const text = document.body?.innerText || ''
    const oldChromeTerms = ['CURRENT VIEW', 'ADMIN ROLE', 'QUEUE ALERTS', 'PENDING REVIEW']
    const pageHeaderActions = document.querySelector('header > div:last-child')
    const visiblePrimaryButtons = Array.from((pageHeaderActions || document.createElement('div')).querySelectorAll('button'))
      .filter((button) => {
        const rect = button.getBoundingClientRect()
        const style = getComputedStyle(button)
        if (rect.width <= 0 || rect.height <= 0 || style.visibility === 'hidden' || style.display === 'none') return false
        return String(button.className).includes('bg-[var(--accent)]')
      })
      .map((button) => button.textContent?.trim() || button.getAttribute('aria-label') || '')

    return {
      forbidden: oldChromeTerms.filter((term) => text.includes(term)),
      primaryCount: visiblePrimaryButtons.length,
      primaryButtons: visiblePrimaryButtons,
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth: window.innerWidth,
    }
  })

  assert(result.scrollWidth <= result.innerWidth + 4, `${viewportName}/${route} has horizontal overflow: ${result.scrollWidth} > ${result.innerWidth}`)
  assert(result.forbidden.length === 0, `${viewportName}/${route} still contains old shell terms: ${result.forbidden.join(', ')}`)
  assert(result.primaryCount <= 1, `${viewportName}/${route} has too many primary buttons: ${result.primaryButtons.join(', ')}`)

  await page.screenshot({ path: resolve(outputDir, `${viewportName}-${route}.png`), fullPage: true })
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

main().catch((error) => {
  console.error(error.message)
  process.exit(1)
})
