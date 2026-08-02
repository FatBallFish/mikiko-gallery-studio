// @ts-nocheck
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')

for (const expected of [
  "const defaultPixelSizes = ['1024x1024', '1536x1024', '1024x1536', '1280x720', '720x1280', '1024x768', '768x1024']",
  'supportsCustomSize: boolean',
  'supports_custom_size: modelDialog.supportsCustomSize',
  '允许用户自定义尺寸',
  'checked={modelDialog.supportsCustomSize}',
  'row.supports_custom_size',
  'supportsOutputCompression: boolean',
  'supports_output_compression: modelDialog.supportsOutputCompression',
  '是否支持压缩质量',
  'checked={modelDialog.supportsOutputCompression}',
  'row.supports_output_compression',
]) {
  if (!source.includes(expected)) {
    throw new Error(`real-model editor must include ${expected}`)
  }
}

if (!source.includes("mode === 'pixel' && checked") || !source.includes('supportedPixelSizes: defaultPixelSizes')) {
  throw new Error('enabling pixel mode must seed all default pixel presets')
}

if (!source.includes("mode === 'pixel' && !checked") || !source.includes('supportsCustomSize: false')) {
  throw new Error('disabling pixel mode must also disable custom pixel sizes')
}

const editorStart = source.indexOf("{modelDialog ? (")
const editorEnd = source.indexOf("{testDialog ? (", editorStart)
const editorSource = source.slice(editorStart, editorEnd)
if (editorSource.includes('<Field label="压缩质量"><input type="number"')) {
  throw new Error('real-model capability editor must not model compression support as a numeric quality')
}

const testStart = source.indexOf("{testDialog ? (")
if (!source.slice(testStart).includes('<Field label="压缩质量"><input type="number"')) {
  throw new Error('single test requests should retain numeric compression quality')
}
