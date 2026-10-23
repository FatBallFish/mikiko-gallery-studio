// @ts-nocheck
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')

for (const expected of [
  "const defaultPixelSizes = ['1024x1024', '1536x1024', '1024x1536', '1280x720', '720x1280', '1024x768', '768x1024']",
  'supportsCustomSize: boolean',
  'supports_custom_size: draft.supportsCustomSize',
  '允许用户自定义尺寸',
  'checked={modelDialog.supportsCustomSize}',
  'row.supports_custom_size',
  'supportsOutputCompression: boolean',
  'supports_output_compression: draft.supportsOutputCompression',
  '支持输出压缩质量',
  'checked={modelDialog.supportsOutputCompression}',
  'row.supports_output_compression',
  "const baseResolutionOptions = ['1K', '2K', '4K']",
  "toggleSizeModes(modelDialog,",
  'supportsCustomRatio: boolean',
  'supports_custom_ratio: draft.supportsCustomRatio',
  'supported_backgrounds: draft.supportedBackgrounds',
  'min_width: Number(draft.minWidth)',
  'max_width: Number(draft.maxWidth)',
  'min_height: Number(draft.minHeight)',
  'max_height: Number(draft.maxHeight)',
  'max="10" value={modelDialog.maxImageCount}',
]) {
  if (!source.includes(expected)) {
    throw new Error(`real-model editor must include ${expected}`)
  }
}

if (!source.includes("supportsCustomSize: sizeModes.includes('pixel') ? draft.supportsCustomSize : false") || !source.includes('supportedPixelSizes: defaultPixelSizes')) {
  throw new Error('pixel mode toggles must preserve default pixel presets')
}

for (const expected of [
  'selectedTestModel',
  'testModelSizeModes(selectedTestModel)',
  'testModelOptions(selectedTestModel?.quality',
  'testModelOptions(selectedTestModel?.output_format',
  'testModelOptions(selectedTestModel?.supported_backgrounds',
  'testModelOptions(selectedTestModel?.moderation',
  'rebuildTestImageDialog(testDialog, selected)',
]) {
  if (!source.includes(expected)) {
    throw new Error(`model test dialog must be capability-driven: ${expected}`)
  }
}

if (!source.includes("supportsCustomSize: sizeModes.includes('pixel') ? draft.supportsCustomSize : false")) {
  throw new Error('disabling pixel mode must also disable custom pixel sizes')
}

const editorStart = source.indexOf("{modelDialog ? (")
const editorEnd = source.indexOf("{testDialog ? (", editorStart)
const editorSource = source.slice(editorStart, editorEnd)
if (editorSource.includes('label="压缩质量"')) {
  throw new Error('real-model capability editor must not model compression support as a numeric quality')
}

const testStart = source.indexOf("{testDialog ? (")
if (!source.slice(testStart).includes('label="压缩质量"')) {
  throw new Error('single test requests should retain numeric compression quality')
}
