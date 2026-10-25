function canonical(value: unknown): unknown { return Array.isArray(value) ? value.map(canonical) : value && typeof value === 'object' ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)])) : value }
const assert = { equal(actual: unknown, expected: unknown) { if (actual !== expected) throw new Error(`${String(actual)} !== ${String(expected)}`) }, notEqual(actual: unknown, expected: unknown) { if (actual === expected) throw new Error('values must differ') }, deepEqual(actual: unknown, expected: unknown) { if (JSON.stringify(canonical(actual)) !== JSON.stringify(canonical(expected))) throw new Error(`values differ: ${JSON.stringify(actual)} vs ${JSON.stringify(expected)}`) } }
import { nativeImageFields, nativeImageProfile, normalizeNativeImagePoint, nativeImagePresetConfig, resolveNativeImageOperation, nativeImageValidation, nativeWorkspaceModel, nativeImageDimensions, nativeImageCountLimit, nativeImageReferencePolicy, nativeImageReferenceFileError, nativeImageReferenceAccept } from './native-image'
import { buildCreateTaskWireRequest, buildEstimateWireRequest, normalizeCapabilities, toTask } from './user-api'
import { workspaceCreationDraftFromSnapshot, stageWorkspaceCreationDraft, consumeWorkspaceCreationDraft } from '../user/src/pages/workspaceCreationDraft'
import { workspaceEstimateKey } from '../user/src/pages/workspaceEstimate'
import { attachCanvasResults, createCanvasState } from '../user/src/features/canvas/core/canvasState'

const profile = nativeImageProfile('qwen-image-3.0')!
assert.equal(profile.max_reference_images, 3)
assert.equal(nativeImageProfile('qwen-image-2.1-pro')!.negative_prompt, false)
assert.deepEqual(nativeImageProfile('doubao-seedream-5-0-flash-260915')!.optimize_prompt_modes, ['standard'])
assert.equal(nativeImageProfile('endpoint-alias'), undefined)
assert.deepEqual(normalizeNativeImagePoint(50, 25, 100, 100), [500, 250])
assert.deepEqual(normalizeNativeImagePoint(-4, 400, 100, 100), [0, 999])
const fields = { operation: 'edit' as const, seed: 0, negative_prompt: 'legacy negative', native_options: { seed: 0, watermark: false, negative_prompt: 'blur', qwen: { prompt_extend: false, enable_thinking: false }, annotations: [{ reference_index: 2, point: [0, 999] as [number, number] }] } }
const request = { ...fields, task_type: 'image_edit' as const, route_model_code: 'native', image_count: 1, prompt: 'edit', reference_asset_ids: ['a', 'b'] }
assert.deepEqual(nativeImageFields(buildEstimateWireRequest(request)), fields)
assert.deepEqual(nativeImageFields(buildCreateTaskWireRequest(request).body), fields)
assert.notEqual(workspaceEstimateKey(request), workspaceEstimateKey({ ...request, native_options: { ...fields.native_options, watermark: true } }))
const task = toTask({ id: 'task', ...request, results: [{ id: 'layer', width: 20, height: 30, layer: { z_index: 3, is_base: false, bounding_box: { absolute: [10, 20, 30, 50], normalized: [100, 200, 300, 500] } } }] })
assert.equal(task.results[0].layer?.z_index, 3)
assert.deepEqual(nativeImageFields(workspaceCreationDraftFromSnapshot(task)), fields)
const data = new Map<string, string>(); const history = { state: {}, replaceState(next: unknown) { this.state = next as {} } }
stageWorkspaceCreationDraft(workspaceCreationDraftFromSnapshot(task), { getItem: (key) => data.get(key) ?? null, setItem: (key, value) => { data.set(key, value) }, removeItem: (key) => { data.delete(key) } }, history)
assert.deepEqual(consumeWorkspaceCreationDraft({ getItem: (key) => data.get(key) ?? null, setItem: (key, value) => { data.set(key, value) }, removeItem: (key) => { data.delete(key) } }, history)?.native_options, fields.native_options)
const capability = normalizeCapabilities({ task_types: ['text_to_image'], model_groups: [{ code: 'native', native_image_profile: profile, task_types: ['text_to_image'] }] })
assert.deepEqual(capability.model_groups[0].native_image_profile, profile)
const canvas = createCanvasState({ schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [{ id: 'source', type: 'image_v2', position: { x: 0, y: 0 }, size: { width: 100, height: 100 } }], edges: [] }, 1)
const restored = attachCanvasResults(canvas, 'run', 'source', [{ asset_id: 'layer', media_type: 'image', width: 20, height: 30, layer: task.results[0].layer }, { asset_id: 'base', media_type: 'image', width: 100, height: 100, layer: { is_base: true, z_index: 0 } }])
assert.equal(restored.present.nodes[1].asset_id, 'base')
assert.deepEqual(restored.present.nodes[2].position, { x: 190, y: 20 })
assert.deepEqual(restored.present.nodes[2].size, { width: 20, height: 30 })
assert.equal(restored.past.length, 1)

const confirmedPricing = { model: profile.model, layer_pricing_confirmed: true, layer_base_cny: 0.3, output_tiers: [{ name: 'all', max_pixels: 4194304, cny: 0.25 }] }
const originalConfig = { model: profile.model, profile, pricing: confirmedPricing, sale_pricing: { ...confirmedPricing, layer_base_cny: 0.9 }, custom_setting: 'preserved' }
const changedConfig = nativeImagePresetConfig(originalConfig, nativeImageProfile('qwen-image-2.1-pro')!)
assert.equal(changedConfig.pricing, undefined)
assert.equal(changedConfig.sale_pricing, undefined)
assert.equal(changedConfig.custom_setting, 'preserved')
assert.equal(originalConfig.pricing.layer_pricing_confirmed, true)
assert.deepEqual(nativeImagePresetConfig(originalConfig, profile).pricing, confirmedPricing)
assert.equal(resolveNativeImageOperation('generate', 1), 'generate')
assert.equal(resolveNativeImageOperation('edit', 0), 'edit')
assert.equal(resolveNativeImageOperation(undefined, 1), 'edit')
assert.notEqual(nativeImageValidation(profile, { operation: resolveNativeImageOperation('generate', 1) }, 1), '')
assert.notEqual(nativeImageValidation(profile, { operation: resolveNativeImageOperation('edit', 0) }, 0), '')
assert.equal(nativeImageValidation(profile, { operation: 'edit' }, 1), '')

// Native profiles override lossy legacy capability projections.
const nativeLegacy = nativeWorkspaceModel({ native_image_profile: profile, supports_custom_size: false, size_modes: ['ratio'], pixel_sizes: [] })
assert.equal(nativeLegacy.supports_custom_size, true)
assert.deepEqual(nativeLegacy.size_modes, ['auto', 'pixel'])
assert.equal(nativeImageDimensions(profile, '1024', '1024').valid, true)
assert.equal(nativeImageDimensions(profile, '4096', '4096').valid, false)
assert.equal(nativeImageDimensions(profile, '2048', '256').valid, true)
const seedreamProfile = nativeImageProfile('doubao-seedream-5-0-pro-260628')!
assert.deepEqual(nativeWorkspaceModel({ native_image_profile: seedreamProfile, supports_custom_size: false }).size_modes, ['pixel'])
assert.equal(nativeImageDimensions(seedreamProfile, '1024', '1024').valid, true)
assert.equal(nativeImageDimensions(seedreamProfile, '512', '512').valid, false)
assert.equal(nativeImageCountLimit(seedreamProfile, 1000), 1)
assert.equal(nativeImageCountLimit(profile, 1000), 6)
assert.equal(nativeImageCountLimit(undefined, 1000), 1000)
assert.equal(nativeImageReferencePolicy(profile).maxBytes, 10 * 1024 * 1024)
assert.equal(nativeImageReferencePolicy(seedreamProfile).maxBytes, 30 * 1024 * 1024)
assert.notEqual(nativeImageReferenceFileError({ name: 'large.png', type: 'image/png', size: 11 * 1024 * 1024 }, profile), '')
assert.equal(nativeImageReferenceFileError({ name: 'sample.tif', type: 'image/tiff', size: 100 }, profile), '')
assert.equal(nativeImageReferenceFileError({ name: 'sample.heic', type: 'image/heic', size: 100 }, seedreamProfile), '')
assert.notEqual(nativeImageReferenceFileError({ name: 'sample.heic', type: 'image/heic', size: 100 }, profile), '')
assert.equal(nativeImageReferenceAccept(profile).includes('.tif'), true)
// Resolve the real user helper through its own module location. UI imports are exercised by user contracts.
async function verifyNativeControlOperations() {
  const { userNativeImageFields } = await import('../user/src/features/creation/nativeImageUX')
  for (const [before, after] of [[0, 1], [1, 0]]) {
    const changed = userNativeImageFields({ operation: before ? 'edit' : 'generate', native_options: { watermark: true, seed: 9 } }, seedreamProfile, after)
    assert.equal(changed.operation, after ? 'edit' : 'generate')
    assert.equal(changed.native_options, undefined)
    assert.equal(nativeImageValidation(seedreamProfile, changed, after), '')
  }
  assert.equal(userNativeImageFields({ operation: 'layer_decomposition' }, seedreamProfile, 1).operation, 'layer_decomposition')
}
void verifyNativeControlOperations().catch((error) => { throw error })

const autoRequest = buildEstimateWireRequest({ task_type: 'text_to_image', route_model_code: 'qwen', size_mode: 'auto', aspect_ratio: '1:1', image_count: 1 })
assert.equal(autoRequest.aspect_ratio, undefined)
assert.equal(autoRequest.requested_size, undefined)
