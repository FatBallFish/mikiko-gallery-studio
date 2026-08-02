import { calculateImageSizeForBaseResolution, normalizeCustomImageSize } from './image-size'

if (calculateImageSizeForBaseResolution('1K', '16:9') !== '1280x720') {
  throw new Error('1K 16:9 should map to 1280x720')
}

if (calculateImageSizeForBaseResolution('2K', '16:9') !== '2560x1440') {
  throw new Error('2K 16:9 should map to 2560x1440')
}

if (calculateImageSizeForBaseResolution('4K', '1:1') !== '2880x2880') {
  throw new Error('4K 1:1 should map to 2880x2880')
}

if (calculateImageSizeForBaseResolution('auto', '16:9') !== '1280x720') {
  throw new Error('auto 16:9 should preview the default 1K route bucket')
}

const customCases = [
  { width: 1024, height: 1024, size: '1024x1024' },
  { width: 256, height: 256, size: '816x816' },
  { width: 5000, height: 5000, size: '2880x2880' },
  { width: 1001, height: 777, size: '1008x784' },
  { width: 4000, height: 500, size: '2448x816' },
  { width: 500, height: 4000, size: '816x2448' },
  { width: 5000, height: 3000, size: '3712x2224' },
]

for (const fixture of customCases) {
  const normalized = normalizeCustomImageSize(fixture.width, fixture.height)
  if (!normalized.valid) throw new Error(`expected valid normalized size for ${JSON.stringify(fixture)}`)
  if (fixture.size && normalized.size !== fixture.size) {
    throw new Error(`normalize ${fixture.width}x${fixture.height} = ${normalized.size}, want ${fixture.size}`)
  }
  if (normalized.width % 16 || normalized.height % 16 || normalized.width > 3840 || normalized.height > 3840) {
    throw new Error(`normalized size violates edge/grid constraints: ${JSON.stringify(normalized)}`)
  }
  const pixels = normalized.width * normalized.height
  const ratio = Math.max(normalized.width / normalized.height, normalized.height / normalized.width)
  if (pixels < 655_360 || pixels > 8_294_400 || ratio > 3) {
    throw new Error(`normalized size violates pixel/ratio constraints: ${JSON.stringify(normalized)}`)
  }
  const again = normalizeCustomImageSize(normalized.width, normalized.height)
  if (!again.valid || again.size !== normalized.size) {
    throw new Error(`custom size normalization must be idempotent: ${JSON.stringify(normalized)} -> ${JSON.stringify(again)}`)
  }
}

for (const [width, height] of [[0, 1024], [1024, -1], [Number.NaN, 1024]]) {
  if (normalizeCustomImageSize(width, height).valid) {
    throw new Error(`invalid custom size should be rejected: ${width}x${height}`)
  }
}
