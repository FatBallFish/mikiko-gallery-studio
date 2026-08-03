// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import { existsSync, readFileSync, statSync } from 'node:fs'
import * as landingContent from './landingContent'

const { landingActionInk, landingChapters } = landingContent
const landingAssetUrl = 'landingAssetUrl' in landingContent
  ? landingContent.landingAssetUrl as (baseUrl: string, assetPath: string) => string
  : null

assert.ok(landingAssetUrl, 'landing content must expose a base-path-aware asset URL helper')
assert.equal(landingAssetUrl('/studio/', '/landing/studio-showcase-1280.webp'), '/studio/landing/studio-showcase-1280.webp')
assert.equal(landingAssetUrl('/', '/landing/capability-edit.webp'), '/landing/capability-edit.webp')
assert.equal(landingActionInk, '#111218', 'landing action contrast ink drifted')

const serialized = JSON.stringify(landingChapters)

for (const claim of ['文生图', '图片编辑', '参考图', 'OpenAI 兼容', '积分预估']) {
  assert.ok(serialized.includes(claim), `missing real capability: ${claim}`)
}

for (const banned of ['99.9%', '全球顶尖', 'SECTION 01', '创作工作台']) {
  assert.ok(!serialized.includes(banned), `landing page contains unsupported or generic copy: ${banned}`)
}

assert.deepEqual(landingChapters.sections.map((section) => section.stage), ['attention', 'interest', 'desire', 'action'])
assert.equal(landingChapters.hero.actions.length, 2, 'hero must expose exactly two actions')
assert.deepEqual(landingChapters.hero.actions.map((action) => action.kind).sort(), ['create', 'docs'])
assert.ok(landingChapters.capabilities.length >= 3 && landingChapters.capabilities.length <= 5, 'bento must contain 3-5 intentional items')
assert.equal(
  landingChapters.capabilities.reduce((total, capability) => total + capability.columns * capability.rows, 0),
  24,
  'desktop bento must occupy all 24 cells',
)

function readWebPDimensions(bytes: Buffer) {
  assert.equal(bytes.toString('ascii', 0, 4), 'RIFF', 'invalid WebP RIFF header')
  assert.equal(bytes.toString('ascii', 8, 12), 'WEBP', 'invalid WebP signature')
  const chunk = bytes.toString('ascii', 12, 16)
  if (chunk === 'VP8X') {
    return {
      width: 1 + bytes.readUIntLE(24, 3),
      height: 1 + bytes.readUIntLE(27, 3),
    }
  }
  if (chunk === 'VP8 ') {
    return {
      width: bytes.readUInt16LE(26) & 0x3fff,
      height: bytes.readUInt16LE(28) & 0x3fff,
    }
  }
  if (chunk === 'VP8L') {
    assert.equal(bytes[20], 0x2f, 'invalid lossless WebP signature')
    return {
      width: 1 + bytes[21]! + ((bytes[22]! & 0x3f) << 8),
      height: 1 + ((bytes[22]! & 0xc0) >> 6) + (bytes[23]! << 2) + ((bytes[24]! & 0x0f) << 10),
    }
  }
  throw new Error(`unsupported WebP chunk: ${chunk}`)
}

function readAVIFDimensions(bytes: Buffer) {
  assert.equal(bytes.toString('ascii', 4, 8), 'ftyp', 'invalid AVIF file type box')
  const marker = bytes.indexOf(Buffer.from('ispe'))
  assert.ok(marker >= 0, 'AVIF spatial extents box is missing')
  return {
    width: bytes.readUInt32BE(marker + 8),
    height: bytes.readUInt32BE(marker + 12),
  }
}

const capabilityAssets = Object.fromEntries(landingChapters.capabilities.map((item) => [item.id, item.image]))
const modeAssets = Object.fromEntries(landingChapters.modes.map((item) => [item.id, item.image]))
const semanticAssets = [
  capabilityAssets.edit,
  capabilityAssets.reference,
  capabilityAssets.estimate,
  landingChapters.workflow.image,
  modeAssets.words,
  modeAssets.edit,
  modeAssets.reference,
]

const expectedWebP = [
  '/landing/capability-edit.webp',
  '/landing/capability-reference.webp',
  '/landing/capability-estimate.webp',
  '/landing/workflow-strip.webp',
  '/landing/mode-text.webp',
  '/landing/mode-edit.webp',
  '/landing/mode-reference.webp',
]

assert.deepEqual(semanticAssets.map((asset) => asset?.webp), expectedWebP)
assert.equal(new Set(semanticAssets.map((asset) => asset?.webp)).size, semanticAssets.length, 'semantic landing slots must use distinct WebP paths')
assert.equal(new Set(semanticAssets.map((asset) => asset?.avif)).size, semanticAssets.length, 'semantic landing slots must use distinct AVIF paths')

for (const asset of semanticAssets) {
  assert.ok(asset && asset.width > 0 && asset.height > 0, `landing asset needs stable dimensions: ${JSON.stringify(asset)}`)
  for (const path of [asset.webp, asset.avif]) {
    const file = new URL(`../../public${path}`, import.meta.url)
    assert.ok(existsSync(file), `landing asset file is missing: ${path}`)
    const size = statSync(file).size
    assert.ok(size >= 8 * 1024 && size <= 250 * 1024, `landing asset must stay between 8 KB and 250 KB: ${path} (${size} bytes)`)
    const bytes = readFileSync(file)
    const dimensions = path.endsWith('.webp') ? readWebPDimensions(bytes) : readAVIFDimensions(bytes)
    assert.deepEqual(dimensions, { width: asset.width, height: asset.height }, `landing asset dimensions drifted: ${path}`)
  }
}

console.log('OK: semantic landing asset contract passed')
