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
})
