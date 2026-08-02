import type { CapabilityModelGroup } from '../../../shared/api-types'
import { readFileSync } from 'node:fs'
import { normalizeCapabilities, toTask } from '../../../shared/user-api'
import {
  normalizeWorkspaceOutputParameters,
  normalizeWorkspaceCustomSize,
  workspaceCustomSizeSupported,
  workspaceCompressionVisible,
  workspaceModelForTask,
  workspaceOutputOptions,
} from './workspaceParameters'

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

for (const expected of [
  'workspaceModelForTask(rawSelectedModel, taskType)',
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

const taskScopedModel = {
  ...model,
  size_modes: ['ratio', 'pixel'],
  quality: ['auto', 'high'],
  output_format: ['png', 'webp'],
  supports_output_compression: true,
  supports_custom_size: true,
  capabilities_by_task_type: {
    text_to_image: {
      base_resolution: ['auto', '2k'], auto_base_resolution: '2k', size_modes: ['ratio'], aspect_ratios: ['1:1'], pixel_sizes: [],
      quality: ['high'], output_format: ['jpeg'], supports_output_compression: true, supports_custom_size: false,
      moderation: ['auto'], max_output_image_count: 2, max_reference_image_count: 0,
    },
    image_edit: {
      base_resolution: ['auto', '1k'], auto_base_resolution: '1k', size_modes: ['pixel'], aspect_ratios: [], pixel_sizes: ['1024x1024'],
      quality: ['low'], output_format: ['webp'], supports_output_compression: false, supports_custom_size: true,
      moderation: ['low'], max_output_image_count: 1, max_reference_image_count: 3,
    },
  },
} satisfies CapabilityModelGroup
const textModel = workspaceModelForTask(taskScopedModel, 'text_to_image')
const editModel = workspaceModelForTask(taskScopedModel, 'image_edit')
if (!textModel || !editModel) throw new Error('task capability projection must preserve the model')
if (textModel.size_modes?.join(',') !== 'ratio' || textModel.quality?.join(',') !== 'high' || textModel.output_format?.join(',') !== 'jpeg' || textModel.max_reference_image_count !== 0 || workspaceCustomSizeSupported(textModel)) {
  throw new Error(`text-to-image must not inherit image-edit capabilities: ${JSON.stringify(textModel)}`)
}
if (editModel.size_modes?.join(',') !== 'pixel' || editModel.pixel_sizes?.join(',') !== '1024x1024' || editModel.max_reference_image_count !== 3 || !workspaceCustomSizeSupported(editModel)) {
  throw new Error(`image-edit must use its scoped capabilities: ${JSON.stringify(editModel)}`)
}
if (!workspaceCustomSizeSupported({ ...model, supports_custom_size: true })) {
  throw new Error('legacy capability payloads must fall back to the aggregate custom-size flag')
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
