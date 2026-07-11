// @ts-nocheck
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')

for (const expected of [
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
