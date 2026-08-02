import type { CapabilityModelGroup } from '../../../shared/api-types'
import { readFileSync } from 'node:fs'
import { normalizeCapabilities, toTask } from '../../../shared/user-api'
import {
  normalizeWorkspaceOutputParameters,
  normalizeWorkspaceCustomSize,
  workspaceCompressionVisible,
  workspaceOutputOptions,
} from './workspaceParameters'

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

for (const expected of [
  'selectedModel.supports_custom_size',
  '自定义尺寸',
  'Width',
  'Height',
  'effectivePixelSize',
  "pixel_size: sizeMode === 'pixel' ? effectivePixelSize : undefined",
  '由于模型限制，最终输出会自动规整到合法尺寸：宽高均为 16 的倍数，最大边长 3840px，宽高比不超过 3:1，总像素限制为 655360-8294400。',
]) {
  if (!workspaceSource.includes(expected)) throw new Error(`workspace custom size UI must include ${expected}`)
}

const capability = normalizeCapabilities({
  model_groups: [{
    code: 'studio',
    name: 'Studio',
    task_types: ['text_to_image'],
    base_resolution: ['1k'],
    size_modes: ['ratio'],
    aspect_ratios: ['1:1'],
    quality: ['auto', 'high'],
    output_format: ['png', 'webp'],
    supports_output_compression: true,
    moderation: ['auto', 'low'],
    max_output_image_count: 2,
    prices: [],
  }],
})

const model = capability.model_groups[0]
if (!model) throw new Error('normalized capability should retain a model')

if (model.quality?.join(',') !== 'auto,high') {
  throw new Error(`quality capability should survive normalization, got ${JSON.stringify(model.quality)}`)
}
if (model.output_format?.join(',') !== 'png,webp') {
  throw new Error(`output formats should survive normalization, got ${JSON.stringify(model.output_format)}`)
}
if (!model.supports_output_compression) {
  throw new Error('compression support should survive normalization')
}
if (model.moderation?.join(',') !== 'auto,low') {
  throw new Error(`moderation capability should survive normalization, got ${JSON.stringify(model.moderation)}`)
}

const legacy = normalizeCapabilities({
  model_groups: [{ code: 'legacy', name: 'Legacy', task_types: ['text_to_image'], base_resolution: ['auto'], prices: [] }],
})
if (legacy.model_groups[0]?.supports_output_compression !== false) {
  throw new Error('missing compression support must default to false')
}

const runningTask = toTask({ id: 'running-without-percent', status: 'running', results: [] })
if (runningTask.progress !== undefined) {
  throw new Error(`running tasks must not receive a fabricated numeric progress value, got ${runningTask.progress}`)
}

const options = workspaceOutputOptions(model)
if (options.quality.join(',') !== 'auto,high' || options.outputFormat.join(',') !== 'png,webp' || options.moderation.join(',') !== 'auto,low') {
  throw new Error(`workspace options should come from the selected model, got ${JSON.stringify(options)}`)
}

const normalized = normalizeWorkspaceOutputParameters(model, {
  quality: 'unsupported',
  outputFormat: 'webp',
  outputCompression: 72,
  moderation: 'unsupported',
})
if (normalized.quality !== 'auto' || normalized.outputFormat !== 'webp' || normalized.outputCompression !== 72 || normalized.moderation !== 'auto') {
  throw new Error(`invalid selections should normalize without losing valid compression, got ${JSON.stringify(normalized)}`)
}

if (!workspaceCompressionVisible(model, 'webp') || workspaceCompressionVisible(model, 'png')) {
  throw new Error('compression should only be visible for supported JPEG/WebP output')
}

const unsupportedModel = { ...model, supports_output_compression: false } satisfies CapabilityModelGroup
if (workspaceCompressionVisible(unsupportedModel, 'webp')) {
  throw new Error('compression must remain hidden when the selected model does not support it')
}

const clamped = normalizeWorkspaceOutputParameters(model, {
  quality: 'high',
  outputFormat: 'webp',
  outputCompression: 200,
  moderation: 'low',
})
if (clamped.outputCompression !== 100) {
  throw new Error(`compression should clamp to 100, got ${clamped.outputCompression}`)
}

const customSize = normalizeWorkspaceCustomSize('1001', '777')
if (!customSize.valid || customSize.size !== '1008x784') {
  throw new Error(`custom workspace size should use shared normalization, got ${JSON.stringify(customSize)}`)
}

if (normalizeWorkspaceCustomSize('1001.5', '777').valid || normalizeWorkspaceCustomSize('', '777').valid) {
  throw new Error('custom workspace size should reject partial and non-integer input')
}
