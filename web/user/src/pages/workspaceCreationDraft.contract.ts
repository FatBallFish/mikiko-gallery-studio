import type { Capability } from '../../../shared/api-types'
import { consumeWorkspaceCreationDraft, normalizeWorkspaceCreationDraft, stageWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot, workspaceCreationDraftStorageKey } from './workspaceCreationDraft'

const storage = new Map<string, string>()
const session = { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => { storage.set(key, value) }, removeItem: (key: string) => { storage.delete(key) } }
let historyState: unknown = null
const history = { state: historyState, replaceState: (state: unknown) => { historyState = state; history.state = state } }

stageWorkspaceCreationDraft({
  version: 1, prompt: 'a very long private prompt', task_type: 'image_edit', route_model_code: 'missing', size_mode: 'ratio',
  base_resolution: '4K', aspect_ratio: '16:9', quality: 'high', output_format: 'webp', output_compression: 80,
  moderation: 'low', image_count: 3, reference_asset_ids: ['ref-1'],
}, session, history)
if (JSON.stringify(historyState).includes('url')) throw new Error('creation draft must not place prompt or asset ids in URL fields')
const consumed = consumeWorkspaceCreationDraft(session, history)
if (!consumed || consumed.prompt !== 'a very long private prompt' || consumed.reference_asset_ids?.[0] !== 'ref-1') throw new Error('creation draft was not restored')
if (consumeWorkspaceCreationDraft(session, history) !== null) throw new Error('creation draft must be consumed exactly once')

const capability: Capability = {
  task_types: ['text_to_image', 'image_edit'], aspect_ratios: ['1:1'], max_image_count: 4,
  model_groups: [{ id: 'plus', code: 'plus', name: 'Plus', task_types: ['text_to_image', 'image_edit'], base_resolution: ['1K', '2K'], size_modes: ['ratio'], aspect_ratios: ['1:1'], quality: ['auto'], output_format: ['png'], moderation: ['auto'], supports_output_compression: false, max_output_image_count: 2, max_reference_image_count: 2, prices: [], supports_reference: true }],
}
const normalized = normalizeWorkspaceCreationDraft(consumed, capability)
if (normalized.values.route_model_code !== 'plus' || normalized.values.base_resolution !== '1K' || normalized.values.aspect_ratio !== '1:1' || normalized.values.quality !== 'auto' || normalized.values.output_format !== 'png' || normalized.values.image_count !== 2) throw new Error(`creation draft fallbacks drifted: ${JSON.stringify(normalized)}`)
if (normalized.notices.length < 4) throw new Error('every unsupported restored field should produce a fallback notice')

const customSizeCapability: Capability = {
  task_types: ['text_to_image'], aspect_ratios: ['1:1'], pixel_sizes: ['1024x1024'], max_image_count: 1,
  model_groups: [{ id: 'custom', code: 'custom', name: 'Custom', task_types: ['text_to_image'], base_resolution: ['1K'], size_modes: ['pixel'], pixel_sizes: ['1024x1024'], supports_custom_size: true, quality: ['auto'], output_format: ['png'], moderation: ['auto'], supports_output_compression: false, max_output_image_count: 1, max_reference_image_count: 0, prices: [], supports_reference: false }],
}
const restoredCustomSize = normalizeWorkspaceCreationDraft({
  version: 1, prompt: 'custom dimensions', task_type: 'text_to_image', route_model_code: 'custom', size_mode: 'pixel', pixel_size: '1001x777',
}, customSizeCapability)
if (restoredCustomSize.values.pixel_size !== '1008x784') {
  throw new Error(`custom-enabled drafts must preserve their normalized size, got ${JSON.stringify(restoredCustomSize)}`)
}

const presetOnlySize = normalizeWorkspaceCreationDraft({
  version: 1, prompt: 'preset only', task_type: 'text_to_image', route_model_code: 'custom', size_mode: 'pixel', pixel_size: '1001x777',
}, { ...customSizeCapability, model_groups: [{ ...customSizeCapability.model_groups[0], supports_custom_size: false }] })
if (presetOnlySize.values.pixel_size !== '1024x1024') {
  throw new Error(`preset-only drafts must fall back to a configured preset, got ${JSON.stringify(presetOnlySize)}`)
}

const publicEditWithoutAccessibleSources = normalizeWorkspaceCreationDraft({
  version: 1, prompt: 'public edit prompt', task_type: 'image_edit', route_model_code: 'plus', reference_asset_ids: [],
}, capability)
if (publicEditWithoutAccessibleSources.values.task_type !== 'text_to_image' || !publicEditWithoutAccessibleSources.notices.some((notice) => notice.includes('image_edit'))) {
  throw new Error('image edit reuse without accessible sources must explicitly fall back to text-to-image')
}

const reused = workspaceCreationDraftFromSnapshot({
  prompt: 'reuse every supported field', task_type: 'image_edit', route_model_code: 'plus', size_mode: 'pixel',
  requested_size: '1536x1024', base_resolution: '2K', aspect_ratio: '3:2', quality: 'high',
  output_format: 'webp', output_compression: 72, moderation: 'low', requested_output_image_count: 4,
  reference_asset_ids: ['ref-1', 'ref-2'],
})
if (reused.pixel_size !== '1536x1024' || reused.image_count !== 4 || reused.output_format !== 'webp' || reused.output_compression !== 72 || reused.reference_asset_ids?.length !== 2) {
  throw new Error(`reused creation snapshot lost generation fields: ${JSON.stringify(reused)}`)
}

storage.set(workspaceCreationDraftStorageKey, JSON.stringify({ version: 1, prompt: 'malformed', task_type: 'text_to_image', route_model_code: 42 }))
history.replaceState({ __picGalleryWorkspaceCreationDraft: { version: 1, prompt: 'malformed', task_type: 'text_to_image', route_model_code: 42 } })
let malformed: unknown = 'not-consumed'
try {
  malformed = consumeWorkspaceCreationDraft(session, history)
} catch (error) {
  throw new Error(`malformed stored drafts must not crash the workspace: ${String(error)}`)
}
if (malformed !== null) throw new Error('malformed stored drafts must be discarded')
