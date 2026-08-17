import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const scriptPath = 'scripts/visual/multimedia-phase1-acceptance.mjs'

test('multimedia visual acceptance covers the release viewport and media guards', () => {
  assert.equal(existsSync(scriptPath), true, `${scriptPath} must exist`)
  const source = readFileSync(scriptPath, 'utf8')
  for (const required of [
    "name: 'desktop'", "name: 'mobile'", "name: 'tablet-landscape'",
    "'light'", "'dark'", "route: 'genpic'", "route: 'gallery'", "route: 'creative-canvas'",
    'canvasNonBlankPixels', 'originalMediaRequests', 'horizontal overflow', 'overlapping controls',
    'canvas floating controls overlap', "'.canvas-zoom-controls'", "'.canvas-command-controls'", "'.canvas-minimap'",
    "purpose === 'download'", "purpose === 'preview'", "purpose === 'poster'", "purpose === 'waveform'",
  ]) assert.ok(source.includes(required), `visual acceptance must include ${required}`)
  assert.ok(source.includes("wait_until='domcontentloaded'"), 'visual acceptance must wait for the local document instead of third-party network idleness')
  assert.ok(source.includes("page.locator('main').wait_for(state='visible')"), 'visual acceptance must wait for the rendered application shell explicitly')
  assert.equal(source.includes("wait_until='networkidle'"), false, 'visual acceptance must not depend on external font requests becoming idle')
  assert.ok(source.includes("external_font_stylesheets") && source.includes("content_type='text/css'"), 'visual acceptance must replace remote font stylesheets with a deterministic local fallback')
})
