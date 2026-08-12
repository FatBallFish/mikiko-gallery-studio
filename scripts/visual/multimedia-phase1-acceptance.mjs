#!/usr/bin/env node

import { createServer } from 'node:net'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { spawn } from 'node:child_process'

const root = resolve(import.meta.dirname, '../..')
const outputDir = resolve(root, process.env.MULTIMEDIA_VISUAL_DIR || 'docs/reviews/screenshots/multimedia-phase1')
const viewports = [
  { name: 'desktop', width: 1440, height: 960 },
  { name: 'mobile', width: 390, height: 844 },
  { name: 'tablet-landscape', width: 1180, height: 820 },
]
const themes = ['light', 'dark']
const routes = [
  { route: 'genpic', hash: '/genpic?media=video' },
  { route: 'gallery', hash: '/gallery' },
  { route: 'creative-canvas', hash: '/creative-canvas?canvas_id=canvas-visual-1' },
]
const requiredMediaPurposes = ['download', 'preview', 'poster', 'waveform'].filter((purpose) => (
  purpose === 'download' || purpose === 'preview' || purpose === 'poster' || purpose === 'waveform'
))

async function main() {
  if (requiredMediaPurposes.length !== 4) throw new Error('visual media purpose guard is incomplete')
  await mkdir(outputDir, { recursive: true })
  const port = await freePort()
  const baseURL = `http://127.0.0.1:${port}/`
  const server = spawn('npm', ['--prefix', 'web/user', 'run', 'preview', '--', '--host', '127.0.0.1', '--port', String(port)], {
    cwd: root,
    env: process.env,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let logs = ''
  server.stdout.on('data', (chunk) => { logs += chunk })
  server.stderr.on('data', (chunk) => { logs += chunk })
  try {
    await waitForHTTP(baseURL, server)
    const result = await runPythonAcceptance({ baseURL, outputDir })
    if (result.code !== 0) throw new Error(result.stderr || result.stdout || 'visual acceptance failed')
    process.stdout.write(result.stdout)
    if (result.stderr) process.stderr.write(result.stderr)
  } finally {
    server.kill('SIGTERM')
    await Promise.race([new Promise((done) => server.once('exit', done)), delay(3000)])
    if (server.exitCode === null) server.kill('SIGKILL')
  }
}

async function freePort() {
  return new Promise((resolvePort, reject) => {
    const server = createServer()
    server.unref()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      const port = typeof address === 'object' && address ? address.port : 0
      server.close((error) => error ? reject(error) : resolvePort(port))
    })
  })
}

async function waitForHTTP(url, processHandle) {
  const deadline = Date.now() + 20_000
  while (Date.now() < deadline) {
    if (processHandle.exitCode !== null) throw new Error(`production preview exited early (${processHandle.exitCode})`)
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch {
      // Preview is still starting.
    }
    await delay(100)
  }
  throw new Error(`production preview did not start: ${logs}`)
}

function delay(milliseconds) {
  return new Promise((done) => setTimeout(done, milliseconds))
}

async function runPythonAcceptance(options) {
  const { spawnSync } = await import('node:child_process')
  const result = spawnSync('python3', ['-c', pythonSource], {
    cwd: root,
    env: {
      ...process.env,
      MULTIMEDIA_VISUAL_BASE_URL: options.baseURL,
      MULTIMEDIA_VISUAL_OUTPUT_DIR: options.outputDir,
      MULTIMEDIA_VISUAL_MATRIX: JSON.stringify({ viewports, themes, routes }),
    },
    encoding: 'utf8',
    maxBuffer: 20 * 1024 * 1024,
  })
  return { code: result.status ?? 1, stdout: result.stdout ?? '', stderr: result.stderr ?? '' }
}

const pythonSource = String.raw`
import json
import os
import struct
import zlib
from collections import Counter
from pathlib import Path
from urllib.parse import parse_qs, urlparse
from playwright.sync_api import sync_playwright

base_url = os.environ['MULTIMEDIA_VISUAL_BASE_URL']
output_dir = Path(os.environ['MULTIMEDIA_VISUAL_OUTPUT_DIR'])
matrix = json.loads(os.environ['MULTIMEDIA_VISUAL_MATRIX'])
output_dir.mkdir(parents=True, exist_ok=True)

project = {'id': 'project-default', 'name': '默认项目', 'is_default': True, 'status': 'active', 'version': 1, 'created_at': '2026-08-12T08:00:00Z', 'updated_at': '2026-08-12T08:00:00Z'}
profile = {'id': '1001', 'email': 'visual@example.test', 'has_password': True, 'display_name': '视觉验收', 'avatar_initials': '视觉', 'tier': 'PRO', 'group': 'DEFAULT', 'signature': '', 'preferences': {'model_group': 'image-pro', 'base_resolution': '1k', 'quality': 'high', 'aspect_ratio': '16:9', 'image_count': 1}}
assets = [
  {'id': 'asset-image', 'project_id': project['id'], 'name': '森林产品概念图.jpg', 'group_name': '镜头素材', 'media_type': 'image', 'source_type': 'generated', 'status': 'ready', 'visibility_status': 'private', 'storage_driver': 's3', 'mime_type': 'image/jpeg', 'file_size_bytes': 2400000, 'width': 1600, 'height': 900, 'version': 1, 'created_at': '2026-08-12T08:00:00Z', 'updated_at': '2026-08-12T08:00:00Z'},
  {'id': 'asset-video', 'project_id': project['id'], 'name': '城市运镜成片.mp4', 'group_name': '成片', 'media_type': 'video', 'source_type': 'generated', 'status': 'ready', 'visibility_status': 'private', 'storage_driver': 's3', 'mime_type': 'video/mp4', 'file_size_bytes': 18600000, 'width': 1920, 'height': 1080, 'duration_ms': 10000, 'version': 1, 'created_at': '2026-08-12T08:01:00Z', 'updated_at': '2026-08-12T08:01:00Z'},
  {'id': 'asset-audio', 'project_id': project['id'], 'name': '旁白配乐.wav', 'group_name': '声音', 'media_type': 'audio', 'source_type': 'local_upload', 'status': 'ready', 'visibility_status': 'private', 'storage_driver': 's3', 'mime_type': 'audio/wav', 'file_size_bytes': 7200000, 'duration_ms': 18000, 'version': 1, 'created_at': '2026-08-12T08:02:00Z', 'updated_at': '2026-08-12T08:02:00Z'},
]
canvas_document = {
  'schema_version': 1,
  'viewport': {'x': 100, 'y': 100, 'zoom': 0.82},
  'nodes': [
    {'id': 'prompt-1', 'type': 'prompt', 'position': {'x': 40, 'y': 120}, 'size': {'width': 260, 'height': 180}, 'payload': {'title': '场景提示词', 'text': '晨雾中的未来森林城市'}},
    {'id': 'image-gen-1', 'type': 'image_generation', 'position': {'x': 390, 'y': 80}, 'size': {'width': 320, 'height': 230}, 'payload': {'title': '生成关键帧', 'draft': {'route_model_code': 'image-pro', 'image_count': 1}}},
    {'id': 'image-1', 'type': 'image', 'asset_id': 'asset-image', 'position': {'x': 800, 'y': 70}, 'size': {'width': 280, 'height': 220}, 'payload': {'name': '森林产品概念图.jpg', 'mime_type': 'image/jpeg'}},
    {'id': 'video-gen-1', 'type': 'video_generation', 'position': {'x': 800, 'y': 390}, 'size': {'width': 320, 'height': 230}, 'payload': {'title': '生成动态镜头', 'draft': {'route_model_code': 'seedance-2.5', 'output_count': 1, 'audio_mode': 'generated'}}},
  ],
  'edges': [
    {'id': 'edge-1', 'source': 'prompt-1', 'target': 'image-gen-1', 'input_role': 'prompt'},
    {'id': 'edge-2', 'source': 'image-gen-1', 'target': 'image-1', 'input_role': 'result'},
    {'id': 'edge-3', 'source': 'image-1', 'target': 'video-gen-1', 'input_role': 'first_frame'},
  ],
}
canvas = {'id': 'canvas-visual-1', 'project_id': project['id'], 'name': '品牌短片探索', 'revision': 3, 'metadata_version': 2, 'document': canvas_document, 'node_count': 4, 'edge_count': 3, 'running_task_count': 0, 'failed_task_count': 0, 'status': 'active', 'created_at': '2026-08-12T08:00:00Z', 'updated_at': '2026-08-12T08:05:00Z'}

def envelope(data):
  return {'data': data, 'meta': {'request_id': 'visual-acceptance'}}

def fulfill_json(route, data, status=200):
  route.fulfill(status=status, content_type='application/json; charset=utf-8', body=json.dumps(envelope(data), ensure_ascii=False))

def api_data(path, query):
  if path == '/api/system/v1/bootstrap-status': return {'phase': 'ready'}
  if path == '/api/agent/features/v1': return {'video_creation': True, 'creative_canvas': True, 'media_upload': True}
  if path == '/api/agent/user/v1/profile': return profile
  if path == '/api/agent/billing/v1/balance': return {'available_points': '8888.00000', 'frozen_points': '0.00000', 'plan_name': 'PRO', 'first_purchase_bonus': False}
  if path == '/api/agent/project/v1/projects': return {'items': [project], 'default_project_id': project['id']}
  if path == '/api/agent/video/v1/capabilities':
    combinations = []
    for task_type in ['text_to_video', 'image_to_video', 'first_last_frame_to_video']:
      for duration in [5, 10]:
        combinations.append({'task_type': task_type, 'duration_seconds': duration, 'resolution': '1080p', 'aspect_ratio': '16:9', 'audio_mode': 'silent'})
        combinations.append({'task_type': task_type, 'duration_seconds': duration, 'resolution': '1080p', 'aspect_ratio': '16:9', 'audio_mode': 'generated'})
    return {'groups': [{'route_model_code': 'seedance-2.5', 'name': 'Seedance 2.5', 'description': '高质量镜头运动与音画生成', 'config_version': 'visual-1', 'capability_version': 'visual-1', 'max_output_count': 4, 'task_types': ['text_to_video', 'image_to_video', 'first_last_frame_to_video'], 'combinations': combinations}]}
  if path == '/api/agent/video/v1/tasks': return {'items': [], 'next_cursor': ''}
  if path == '/api/agent/media/v1/assets': return {'items': assets, 'next_cursor': ''}
  if path.startswith('/api/agent/media/v1/assets/') and path.endswith('/access'):
    asset_id = path.split('/')[6]
    purpose = query.get('purpose', ['preview'])[0]
    return {'url': base_url + 'visual-media/' + asset_id + '/' + purpose, 'expires_at': '2099-01-01T00:00:00Z', 'range_supported': True}
  if path == '/api/agent/canvas/v1/canvases': return {'items': [canvas]}
  if path == '/api/agent/canvas/v1/canvases/canvas-visual-1': return canvas
  if path == '/api/agent/canvas/v1/canvases/canvas-visual-1/runs': return {'items': []}
  if path == '/api/agent/image/v1/tasks': return {'items': []}
  if path == '/api/agent/image/v1/history/tasks': return {'items': []}
  if path == '/api/agent/image/v1/capabilities':
    return {'model_groups': [{'code': 'image-pro', 'name': '图像专业组', 'description': '稳定的图片生成能力', 'task_types': ['text_to_image', 'image_edit'], 'size_modes': ['auto', 'ratio'], 'base_resolution': ['1k'], 'aspect_ratios': ['1:1', '16:9'], 'max_output_image_count': 4, 'max_reference_image_count': 4, 'supports_reference': True, 'prices': [{'task_type': 'text_to_image', 'base_resolution': '1k', 'charged_points': '2.00000', 'display_points': '2.00'}]}]}
  return None

def install_routes(page, counters):
  def handler(route):
    parsed = urlparse(route.request.url)
    path = parsed.path
    if path.startswith('/visual-media/'):
      purpose = path.rsplit('/', 1)[-1]
      counters['served'][purpose] = counters['served'].get(purpose, 0) + 1
      route.fulfill(status=200, content_type='image/png', body=bytes.fromhex('89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000d49444154789c6360f8cfc000000301010018dd8db10000000049454e44ae426082'))
      return
    if not path.startswith('/api/'):
      route.continue_()
      return
    query = parse_qs(parsed.query)
    if path.startswith('/api/agent/media/v1/assets/') and path.endswith('/access'):
      purpose = query.get('purpose', ['preview'])[0]
      counters['purposes'][purpose] = counters['purposes'].get(purpose, 0) + 1
      if purpose == 'download': counters['originalMediaRequests'] += 1
    data = api_data(path, query)
    if data is None:
      fulfill_json(route, {'items': []})
    else:
      fulfill_json(route, data)
  page.route('**/*', handler)

def png_non_blank_pixels(data):
  assert data[:8] == b'\x89PNG\r\n\x1a\n'
  cursor, width, height, color_type, raw = 8, 0, 0, 0, b''
  while cursor < len(data):
    length = struct.unpack('>I', data[cursor:cursor+4])[0]
    kind = data[cursor+4:cursor+8]
    chunk = data[cursor+8:cursor+8+length]
    cursor += 12 + length
    if kind == b'IHDR':
      width, height, bit_depth, color_type = struct.unpack('>IIBB', chunk[:10])
      assert bit_depth == 8 and color_type in (2, 6)
    elif kind == b'IDAT': raw += chunk
    elif kind == b'IEND': break
  channels = 4 if color_type == 6 else 3
  stride = width * channels
  decoded, previous, offset = bytearray(), bytearray(stride), 0
  source = zlib.decompress(raw)
  for _ in range(height):
    filter_type = source[offset]
    row = bytearray(source[offset+1:offset+1+stride])
    offset += stride + 1
    for index in range(stride):
      left = row[index-channels] if index >= channels else 0
      above = previous[index]
      upper_left = previous[index-channels] if index >= channels else 0
      if filter_type == 1: row[index] = (row[index] + left) & 255
      elif filter_type == 2: row[index] = (row[index] + above) & 255
      elif filter_type == 3: row[index] = (row[index] + ((left + above) // 2)) & 255
      elif filter_type == 4:
        estimate = left + above - upper_left
        pa, pb, pc = abs(estimate-left), abs(estimate-above), abs(estimate-upper_left)
        predictor = left if pa <= pb and pa <= pc else above if pb <= pc else upper_left
        row[index] = (row[index] + predictor) & 255
      elif filter_type != 0: raise AssertionError('unsupported PNG filter')
    decoded.extend(row)
    previous = row
  colors = Counter(bytes(decoded[index:index+channels]) for index in range(0, len(decoded), channels))
  return width * height - colors.most_common(1)[0][1]

def assert_layout(page, label):
  result = page.evaluate("""() => {
    const visible = (element) => {
      const style = getComputedStyle(element); const rect = element.getBoundingClientRect();
      return rect.width > 3 && rect.height > 3 && style.display !== 'none' && style.visibility !== 'hidden';
    };
    const controls = Array.from(document.querySelectorAll('button:not([disabled]), input:not([type=hidden]), select, textarea, [role=button]')).filter(visible);
    const overlapping = [];
    for (let left = 0; left < controls.length; left += 1) for (let right = left + 1; right < controls.length; right += 1) {
      const a = controls[left], b = controls[right];
      if (a.contains(b) || b.contains(a)) continue;
      if (a.closest('.media-asset-card') && a.closest('.media-asset-card') === b.closest('.media-asset-card')) continue;
      const mobileNavA = a.closest('nav[aria-label$="移动导航"]'), mobileNavB = b.closest('nav[aria-label$="移动导航"]');
      if ((mobileNavA && !mobileNavB) || (mobileNavB && !mobileNavA)) {
        const other = mobileNavA ? b : a;
        if (!other.closest('.media-upload-tray')) continue;
      }
      const ar = a.getBoundingClientRect(), br = b.getBoundingClientRect();
      const area = Math.max(0, Math.min(ar.right, br.right) - Math.max(ar.left, br.left)) * Math.max(0, Math.min(ar.bottom, br.bottom) - Math.max(ar.top, br.top));
      const smaller = Math.min(ar.width * ar.height, br.width * br.height);
      if (smaller > 0 && area / smaller > 0.65) overlapping.push({a: (a.getAttribute('aria-label') || a.getAttribute('title') || a.textContent || a.tagName).trim().slice(0, 40), b: (b.getAttribute('aria-label') || b.getAttribute('title') || b.textContent || b.tagName).trim().slice(0, 40), ar: {x: ar.x, y: ar.y, width: ar.width, height: ar.height}, br: {x: br.x, y: br.y, width: br.width, height: br.height}});
    }
    return { scrollWidth: document.documentElement.scrollWidth, innerWidth: window.innerWidth, overlapping };
  }""")
  assert result['scrollWidth'] <= result['innerWidth'] + 4, f"{label} has horizontal overflow: {result['scrollWidth']} > {result['innerWidth']}"
  assert not result['overlapping'], f"{label} has overlapping controls: {result['overlapping'][:4]}"

def assert_canvas_floating_controls(page, label):
  overlaps = page.evaluate("""() => {
    const tray = document.querySelector('.media-upload-tray');
    if (!tray) return [];
    const trayRect = tray.getBoundingClientRect();
    return ['.canvas-zoom-controls', '.canvas-command-controls', '.canvas-minimap'].flatMap((selector) => {
      const control = document.querySelector(selector);
      if (!control) return [];
      const rect = control.getBoundingClientRect();
      const horizontal = Math.max(0, Math.min(trayRect.right, rect.right) - Math.max(trayRect.left, rect.left));
      const vertical = Math.max(0, Math.min(trayRect.bottom, rect.bottom) - Math.max(trayRect.top, rect.top));
      return horizontal * vertical > 0 ? [selector] : [];
    });
  }""")
  assert not overlaps, f"{label} canvas floating controls overlap: {overlaps}"

with sync_playwright() as playwright:
  browser = playwright.chromium.launch(headless=True)
  summary = []
  try:
    for viewport in matrix['viewports']:
      for theme in matrix['themes']:
        context = browser.new_context(viewport={'width': viewport['width'], 'height': viewport['height']}, color_scheme=theme, accept_downloads=True)
        init_values = json.dumps([profile, theme], ensure_ascii=False)
        context.add_init_script("""(() => {
          const [profile, theme] = %s;
          localStorage.setItem('pic-gallery-user-session', JSON.stringify({token: 'visual-token', profile: {...profile, theme: theme + ':amber', preferences: {...profile.preferences, theme_mode: theme, accent_theme: 'amber'}}}));
          localStorage.setItem('pic-gallery-user-theme', JSON.stringify({mode: theme, accent: 'amber'}));
          localStorage.setItem('mikiko.creation.media-mode.v1', 'video');
        })()""" % init_values)
        for route_info in matrix['routes']:
          page = context.new_page()
          counters = {'purposes': {}, 'served': {}, 'originalMediaRequests': 0}
          install_routes(page, counters)
          label = f"{viewport['name']}/{theme}/{route_info['route']}"
          page.goto(base_url + '#' + route_info['hash'], wait_until='networkidle')
          page.wait_for_timeout(500)
          assert page.locator('main').count() > 0, f'{label} did not render the real application page'
          screenshot = output_dir / f"{viewport['name']}-{theme}-{route_info['route']}.png"
          page.screenshot(path=str(screenshot), full_page=True)
          assert_layout(page, label)
          canvasNonBlankPixels = None
          if route_info['route'] == 'creative-canvas':
            canvas_world = page.locator('[data-canvas-world]')
            assert canvas_world.count() == 1 and page.locator('[data-canvas-node]').count() >= 1, f'{label} canvas did not render a visible node'
            canvasNonBlankPixels = png_non_blank_pixels(page.locator('.canvas-viewport').screenshot())
            assert canvasNonBlankPixels > 500, f'{label} canvas pixel check is blank: {canvasNonBlankPixels}'
            assert_canvas_floating_controls(page, label)
            if viewport['name'] == 'tablet-landscape':
              assert page.locator('[data-canvas-editor][data-readonly=false]').count() == 1, 'tablet landscape must expose full canvas editing'
          if route_info['route'] == 'gallery':
            assert page.get_by_role('button', name='预览 森林产品概念图.jpg').count() == 1, f'{label} image asset is missing'
            assert counters['originalMediaRequests'] == 0, f'{label} loaded an original before explicit download'
            page.get_by_role('button', name='预览 森林产品概念图.jpg').click()
            with page.expect_download(timeout=3000) as download_info:
              page.get_by_role('button', name='下载原件').click()
            download_info.value.path()
            assert counters['originalMediaRequests'] == 1, f"{label} download fetched {counters['originalMediaRequests']} originals instead of one"
            page.keyboard.press('Escape')
          else:
            assert counters['originalMediaRequests'] == 0, f'{label} loaded an original without a download action'
          page.screenshot(path=str(screenshot), full_page=True)
          summary.append({'viewport': viewport['name'], 'theme': theme, 'route': route_info['route'], 'canvasNonBlankPixels': canvasNonBlankPixels, 'originalMediaRequests': counters['originalMediaRequests'], 'purposes': counters['purposes'], 'screenshot': str(screenshot)})
          page.close()
        context.close()
    required_purposes = {'download', 'preview', 'poster', 'waveform'}
    observed = {purpose for item in summary for purpose, count in item['purposes'].items() if count > 0}
    assert required_purposes <= observed, f'missing media access purposes: {sorted(required_purposes - observed)}'
    print(json.dumps({'status': 'PASS', 'checks': len(summary), 'observed_purposes': sorted(observed), 'results': summary}, ensure_ascii=False, indent=2))
  finally:
    browser.close()
`

main().catch((error) => {
  console.error(error.message)
  process.exit(1)
})
