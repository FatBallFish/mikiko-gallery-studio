import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { nativeImageProfile } from '../../../../shared/native-image'
import { bindReferenceAnnotations, remapReferenceAnnotations, userNativeImageFields } from './nativeImageUX'
const seedream = nativeImageProfile('doubao-seedream-5-0-pro-260628')!
const qwen = nativeImageProfile('qwen-image-3.0')!
const old = { operation: 'generate' as const, seed: 4, negative_prompt: 'bad', native_options: { seed: 5, watermark: true, qwen: { enable_thinking: true }, seedream: { optimize_prompt_mode: 'fast' as const }, negative_prompt: 'blur', annotations: [{ point: [12, 34] as [number, number], reference_index: 2 }] } }
assert.equal(userNativeImageFields(old, seedream, 2).operation, 'edit')
assert.deepEqual(userNativeImageFields(old, seedream, 2).native_options, { annotations: old.native_options.annotations })
assert.deepEqual(userNativeImageFields(old, qwen, 2).native_options, { negative_prompt: 'blur' })
assert.equal(userNativeImageFields({ operation: 'edit' }, qwen, 0).operation, 'generate')
assert.equal(userNativeImageFields({ operation: 'layer_decomposition' }, seedream, 1).operation, 'layer_decomposition')
assert.equal(userNativeImageFields(old, undefined, 1).native_options, undefined)
const bound = bindReferenceAnnotations(old.native_options.annotations, ['a', 'b'])
assert.deepEqual(remapReferenceAnnotations(bound, ['b']), [{ point: [12, 34], reference_index: 1 }])
assert.deepEqual(remapReferenceAnnotations(bound, ['a', 'new']), [])
const modal = readFileSync(new URL('./ReferenceMarkModal.tsx', import.meta.url), 'utf8')
for (const required of ['<Modal', 'viewBox="0 0 999 999"', '一键清空所有标记', 'onPointerCancel', 'await onSave(marks, file)', '草稿已保留', 'onCancel']) assert.ok(modal.includes(required), required)
const controls = readFileSync(new URL('./NativeImageControls.tsx', import.meta.url), 'utf8')
for (const removed of ['模型专属参数', '随机种子', '添加水印', '智能扩展提示词', '扩展模式']) assert.ok(!controls.includes(removed), removed)
assert.ok(controls.includes('await onUploadMarkedReference(file, asset.id)'))
assert.ok(controls.includes('await onRestoreReference(asset.id, original.id)'))
console.log('native image UX contracts passed')

for (const required of ['persistedMarks', 'onPersistMarks?.(metadata)', 'saved: savedMarks.current', 'originals: [...originals.current]', 'sessionStorage.setItem']) assert.ok(controls.includes(required), required)

const editor = readFileSync(new URL('../canvas/CanvasEditorPage.tsx', import.meta.url), 'utf8')
for (const required of ['remoteSessionRef.current !== uploadSession', 'uploadSignature', 'document.querySelector']) assert.ok(editor.includes(required), required)
for (const required of ['persist({ drafts: nextDrafts, pending: nextPending, painted: nextPainted })', '!live.current.active', 'changedModel && originals.current.size']) assert.ok(controls.includes(required), required)

const { compareCanvasReferenceEdges, nextCanvasReferenceOrdinal } = await import('../canvas/core/canvasState')
const edges = [
  { id: 'a', source: 'a', target: 'target', input_role: 'reference' as const, ordinal: 0 },
  { id: 'c', source: 'c', target: 'target', input_role: 'reference' as const, ordinal: 2 },
]
assert.equal(nextCanvasReferenceOrdinal(edges, 'target'), 3)
const tie = [{ ...edges[1], source: 'z' }, { ...edges[1], source: 'b' }]
assert.deepEqual(tie.sort(compareCanvasReferenceEdges).map((edge) => edge.source), ['b', 'z'])

for (const required of ['useMediaRefreshOnce(', 'onMediaRefresh', 'void retryImage(true)', '刷新图片', 'markMediaLoaded()', 'setImageError', '圈画已记录', 'Math.min(...xs)', 'points.length']) assert.ok(modal.includes(required), required)
assert.ok(controls.includes('userApi.refreshReferenceAssetAccess(original.id)'))
const consoleSource = readFileSync(new URL('../canvas/CanvasGenerationConsole.tsx', import.meta.url), 'utf8')
assert.ok(consoleSource.includes("userApi.getMediaAssetAccess(id, 'preview')"))

const { canvasReferenceMarksModelChanged } = await import('../canvas/canvasMediaV2')
assert.equal(canvasReferenceMarksModelChanged(seedream.model, undefined), false, 'missing capability preserves marked reference and original mapping')
assert.equal(canvasReferenceMarksModelChanged(seedream.model, seedream), false)
assert.equal(canvasReferenceMarksModelChanged(seedream.model, qwen), true, 'confirmed model change restores originals before clearing marks')
assert.ok(editor.includes('canvasReferenceMarksModelChanged(marks.model, profile)'))
assert.ok(editor.includes('asset_id: original.id') && editor.includes('native_options: undefined, ui_reference_marks: undefined'))

assert.ok(controls.includes('nativeLayerUnavailableReason(profile, referenceCount, layerPriceReady, layerUnavailableReason)'), 'layer entry must display the shared specific reason')
assert.ok(consoleSource.includes('layerUnavailableReason={model?.native_image_layer_unavailable_reason}'), 'canvas entry must use task-scoped layer reason')
assert.ok(consoleSource.includes('nativeLayerUnavailableReason(model?.native_image_profile, references.length, model?.native_image_layer_pricing_confirmed === true, model?.native_image_layer_unavailable_reason)'), 'canvas generation blocking must use the same reason')
const workspace = readFileSync(new URL('../../pages/WorkspacePage.tsx', import.meta.url), 'utf8')
assert.ok(workspace.includes('layerUnavailableReason={editModel?.native_image_layer_unavailable_reason}'), 'workspace entry must use editing task reason')
assert.ok(workspace.includes('nativeLayerUnavailableReason(selectedModel?.native_image_profile, editRefs.length, selectedModel?.native_image_layer_pricing_confirmed === true, selectedModel?.native_image_layer_unavailable_reason)'), 'workspace generation blocking must use selected task reason')

for (const references of [0, 1, 2]) {
  assert.equal(userNativeImageFields({ operation: 'layer_decomposition' }, seedream, references).operation, 'layer_decomposition', 'layer choice survives reference changes')
  assert.equal(userNativeImageFields({ operation: 'layer_decomposition' }, qwen, references).operation, 'layer_decomposition', 'unsupported model keeps explicit mode for validation')
}
assert.equal(userNativeImageFields({ operation: 'layer_decomposition' }, undefined, 0).operation, 'layer_decomposition', 'capability loading must not reset mode')
assert.ok(controls.includes("variant === 'canvas'"))
assert.ok(!controls.includes('references.length !== 1'))
assert.ok(workspace.includes("multiple={nativeDraft.operation !== 'layer_decomposition'}"))
assert.ok(consoleSource.includes("disabled={locked || draft.operation === 'layer_decomposition'}"))
