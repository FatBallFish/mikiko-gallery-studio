const SIZE_PATTERN = /^\s*(\d+)\s*[xX×]\s*(\d+)\s*$/
const RATIO_PATTERN = /^\s*(\d+(?:\.\d+)?)\s*[:xX×]\s*(\d+(?:\.\d+)?)\s*$/
const SIZE_MULTIPLE = 16
const MAX_EDGE = 3840
const MAX_ASPECT_RATIO = 3
const MIN_PIXELS = 655_360
const MAX_PIXELS = 8_294_400
const MAX_RATIO_ERROR = 0.01

type SizeTier = '1k' | '2k' | '4k'
type PresetRatio = '1:1' | '3:2' | '2:3' | '16:9' | '9:16' | '4:3' | '3:4' | '21:9'

const legacySizeMap: Record<string, string> = {
  '1:1': '1024x1024',
  '16:9': '1536x864',
  '9:16': '864x1536',
  '4:3': '1536x1152',
  '3:4': '1152x1536',
}

const tierPixelBudget: Record<SizeTier, number> = {
  '1k': 1_572_864,
  '2k': 4_194_304,
  '4k': MAX_PIXELS,
}

const commonSizePresets: Record<SizeTier, Record<PresetRatio, string>> = {
  '1k': {
    '1:1': '1024x1024',
    '3:2': '1536x1024',
    '2:3': '1024x1536',
    '16:9': '1280x720',
    '9:16': '720x1280',
    '4:3': '1024x768',
    '3:4': '768x1024',
    '21:9': '1280x544',
  },
  '2k': {
    '1:1': '2048x2048',
    '3:2': '2160x1440',
    '2:3': '1440x2160',
    '16:9': '2560x1440',
    '9:16': '1440x2560',
    '4:3': '2048x1536',
    '3:4': '1536x2048',
    '21:9': '2560x1088',
  },
  '4k': {
    '1:1': '2880x2880',
    '3:2': '3456x2304',
    '2:3': '2304x3456',
    '16:9': '3840x2160',
    '9:16': '2160x3840',
    '4:3': '3200x2400',
    '3:4': '2400x3200',
    '21:9': '3840x1600',
  },
}

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b)
}

function normalizeQualityBucket(value: string): SizeTier | null {
  switch (value.trim().toLowerCase()) {
    case '1k':
    case 'low':
      return '1k'
    case '2k':
    case 'medium':
      return '2k'
    case '4k':
    case 'high':
      return '4k'
    default:
      return null
  }
}

function parseRatio(value: string) {
  const match = value.match(RATIO_PATTERN)
  if (!match) return null

  const width = Number(match[1])
  const height = Number(match[2])
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return null
  }

  return { width, height }
}

function getPresetRatioKey(ratioWidth: number, ratioHeight: number): PresetRatio | null {
  if (!Number.isInteger(ratioWidth) || !Number.isInteger(ratioHeight)) return null
  const divisor = gcd(ratioWidth, ratioHeight)
  const key = `${ratioWidth / divisor}:${ratioHeight / divisor}`
  return key in commonSizePresets['1k'] ? key as PresetRatio : null
}

export function isExplicitImageSize(value: string) {
  return SIZE_PATTERN.test(value.trim())
}

export function calculateImageSizeForQuality(quality: string, ratio: string) {
  const tier = normalizeQualityBucket(quality)
  if (!tier) return legacySizeMap[ratio] ?? ratio

  const parsed = parseRatio(ratio)
  if (!parsed) return legacySizeMap[ratio] ?? ratio

  const { width: ratioWidth, height: ratioHeight } = parsed
  const ratioValue = ratioWidth / ratioHeight
  if (Math.max(ratioValue, 1 / ratioValue) > MAX_ASPECT_RATIO) return legacySizeMap[ratio] ?? ratio

  const presetRatioKey = getPresetRatioKey(ratioWidth, ratioHeight)
  if (presetRatioKey) return commonSizePresets[tier][presetRatioKey]

  const targetRatio = ratioWidth / ratioHeight
  const pixelBudget = tierPixelBudget[tier]
  let bestWidth = 0
  let bestHeight = 0
  let bestPixels = 0

  for (let width = SIZE_MULTIPLE; width <= MAX_EDGE; width += SIZE_MULTIPLE) {
    const idealHeight = width / targetRatio
    const candidates = [
      Math.floor(idealHeight / SIZE_MULTIPLE) * SIZE_MULTIPLE,
      Math.ceil(idealHeight / SIZE_MULTIPLE) * SIZE_MULTIPLE,
    ]

    for (const height of candidates) {
      if (height < SIZE_MULTIPLE || height > MAX_EDGE) continue

      const pixels = width * height
      if (pixels > pixelBudget || pixels < MIN_PIXELS) continue
      if (Math.max(width / height, height / width) > MAX_ASPECT_RATIO) continue

      const actualRatio = width / height
      const ratioError = Math.abs(actualRatio - targetRatio) / targetRatio
      if (ratioError > MAX_RATIO_ERROR) continue

      if (pixels > bestPixels) {
        bestPixels = pixels
        bestWidth = width
        bestHeight = height
      }
    }
  }

  if (bestPixels === 0) return legacySizeMap[ratio] ?? ratio
  return `${bestWidth}x${bestHeight}`
}

export const calculateImageSizeForBaseResolution = calculateImageSizeForQuality
