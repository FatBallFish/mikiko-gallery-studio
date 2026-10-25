import { addCanvasNode, attachCanvasResults, canvasGenerationEstimateEligible, canvasGenerationEstimateSignature, canvasImageTaskType, canvasInlinePromptTemplate, canvasPromptResourceCandidates, canvasVideoTaskType, connectCanvasNodes, copyCanvasSelection, pasteCanvasSelection, createCanvasGroup, createCanvasState, inspectCanvasConnection, moveCanvasNodes, resizeCanvasNode, updateCanvasNode } from './canvasState'
import { canvasMediaSize, resizeCanvasMediaSize } from './canvasMediaSizing'
import { canCopyCanvasNode, canCreateCanvasNode, canEditCanvasNode, canvasNodeMediaKind, isCanvasGenerationNode, isLegacyCanvasNode } from './canvasNodeKinds'
import { autoLayoutCanvasNodes } from './canvasLayout'
import type { CanvasDocument, CanvasNode } from './types'

function assert(ok: unknown, message: string): asserts ok { if (!ok) throw new Error(message) }
const node = (id: string, type: CanvasNode['type'], asset_id?: string): CanvasNode => ({ id, type, asset_id, position: { x: 0, y: 0 }, size: { width: 320, height: 240 }, payload: { name: `ref_${id}`, title: 'same display title', draft: { route_model_code: 'model', prompt_template: 'A ${subject}', variables: { subject: 'cat' } } } })
const image = node('image', 'image_v2', 'image-asset')
const video = node('video', 'video_v2', 'video-asset')
const target = node('target', 'image_v2')
const videoTarget = node('video-target', 'video_v2')
const legacy = node('legacy', 'image', 'old-asset')
const document: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [image, video, target, videoTarget, legacy], edges: [] }
let state = createCanvasState(document, 8)
assert(isCanvasGenerationNode(target) && !isCanvasGenerationNode(image), 'only empty v2 nodes execute')
assert(canCreateCanvasNode('image_v2') && canCopyCanvasNode(image) && canEditCanvasNode(image), 'v2 supports editing and copy')
assert(isLegacyCanvasNode(legacy) && !canCreateCanvasNode('image') && !canEditCanvasNode(legacy), 'legacy is read/delete only')
assert(canvasNodeMediaKind(videoTarget) === 'video', 'media kind supports empty v2')
assert(canvasGenerationEstimateEligible(document, target.id) && canvasGenerationEstimateEligible(document, videoTarget.id), 'independent image and video nodes are eligible with inline prompts')
assert(canvasInlinePromptTemplate('A ${subject} {{$old}}') === 'A {{$subject}} {{$old}}', 'inline variable adapter preserves canonical syntax')
state = connectCanvasNodes(state, { id: 'ref', source: image.id, target: target.id, input_role: 'reference' })
assert(canvasImageTaskType(state.present, target.id) === 'image_edit', 'v2 filled reference determines image task')
assert(inspectCanvasConnection(state.present, { id: 'dup', source: image.id, target: target.id, input_role: 'reference' }) === 'input_role_conflict', 'duplicate references are rejected')
for (const edge of [
  { id: 'empty', source: target.id, target: videoTarget.id, input_role: 'first_frame' as const },
  { id: 'legacy', source: legacy.id, target: target.id, input_role: 'reference' as const },
  { id: 'wrong', source: video.id, target: target.id, input_role: 'reference' as const },
  { id: 'filled', source: image.id, target: video.id, input_role: 'first_frame' as const },
]) assert(inspectCanvasConnection(state.present, edge) === 'illegal_connection', 'unready, legacy, mismatching or filled targets cannot connect')
const first = connectCanvasNodes(state, { id: 'first', source: image.id, target: videoTarget.id, input_role: 'first_frame' })
assert(canvasVideoTaskType(first.present, videoTarget.id) === 'image_to_video', 'video task derives first frame')
const last = connectCanvasNodes(first, { id: 'last', source: image.id, target: videoTarget.id, input_role: 'last_frame' })
assert(canvasVideoTaskType(last.present, videoTarget.id) === 'first_last_frame_to_video', 'video derives first/last task')
assert(inspectCanvasConnection(first.present, { id: 'first2', source: image.id, target: videoTarget.id, input_role: 'first_frame' }) === 'input_role_conflict', 'first frame singleton is enforced')
const cyclic = { ...document, edges: [{ id: 'old-cycle', source: target.id, target: image.id, input_role: 'reference' as const }] }
assert(inspectCanvasConnection(cyclic, { id: 'cycle', source: image.id, target: target.id, input_role: 'reference' }) === 'cycle', 'historical edges still participate in cycle detection')
const signature = canvasGenerationEstimateSignature(state.present, target.id)
const replacedReference = updateCanvasNode(state, image.id, (source) => ({ ...source, asset_id: 'new-owned-reference' }))
assert(canvasGenerationEstimateSignature(replacedReference.present, target.id) !== signature, 'replacing an asset on the same reference edge must invalidate the persisted graph quote')
const resizedReference = resizeCanvasNode(state, image.id, { width: 640, height: 480 })
assert(canvasGenerationEstimateSignature(resizedReference.present, target.id) === signature, 'display dimensions must not become trusted input dimensions for token pricing')
const withDraft = (patch: Record<string, unknown>) => ({ ...state.present, nodes: state.present.nodes.map((n) => n.id === target.id ? { ...n, payload: { ...n.payload, draft: { ...(n.payload?.draft as object), ...patch } } } : n) })
assert(canvasGenerationEstimateSignature(withDraft({ quote_token: 'quote' }), target.id) === signature, 'quote token never invalidates its own signature')
assert(canvasGenerationEstimateSignature(withDraft({ variables: { subject: 'dog' } }), target.id) !== signature, 'inline variables invalidate estimates')
assert(canvasGenerationEstimateSignature(withDraft({ prompt_template: 'new' }), target.id) !== signature, 'inline template invalidates estimates')
assert(!canvasGenerationEstimateEligible(withDraft({ variables: {} }), target.id), 'missing variables suppress estimates')
assert(!canvasGenerationEstimateEligible(withDraft({ prompt_template: '{{@unknown}}' }), target.id), 'unbound resource suppresses estimates')
assert(canvasPromptResourceCandidates(state.present, target.id)[0].name === 'ref_image', 'bindings use stable aliases, never duplicate titles')
const renamed = updateCanvasNode(state, image.id, (n) => ({ ...n, payload: { ...n.payload, name: 'changed', title: 'new title' } }))
assert(renamed.present.nodes[0].payload?.name === 'ref_image' && renamed.present.nodes[0].payload?.title === 'new title', 'rename preserves resource alias')
const results = [{ asset_id: 'out-1', media_type: 'image' as const, width: 4000, height: 100 }, { asset_id: 'out-2', media_type: 'image' as const, width: 100, height: 4000 }]
const attached = attachCanvasResults(state, 'run', target.id, results)
assert(attached.present.nodes.find((n) => n.id === target.id)?.asset_id === 'out-1', 'first result fills source')
assert(attached.present.nodes.filter((n) => n.asset_id === 'out-2' && n.type === 'image_v2').length === 1, 'overflow is sibling v2')
assert(attached.present.edges.length === state.present.edges.length, 'integrated outputs require no result edges')
assert(attachCanvasResults(attached, 'run', target.id, results) === attached, 'attachment retry is a no-op including history')
assert(!canvasGenerationEstimateEligible(attached.present, target.id), 'filled source cannot execute again')
const resized = resizeCanvasNode(attached, target.id, { width: 640, height: 900 }).present.nodes.find((n) => n.id === target.id)!
assert(Math.abs(resized.size.width / resized.size.height - 40) < 1e-8, 'filled resize locks original ratio')
for (const [w, h] of [[1, 1000000], [1000000, 1], [10000, 10], [10, 10000], [1920, 1080], [100, 100]]) {
  const fitted = canvasMediaSize(w, h)
  assert(fitted.width >= 1 && fitted.height >= 1 && Math.max(fitted.width, fitted.height) >= 80, 'extreme ratios satisfy backend persisted size minimum without distortion')
  assert(Math.abs(fitted.width / fitted.height / (w / h) - 1) < 1e-10, 'fit preserves ratio')
  const scaled = resizeCanvasMediaSize(fitted, { width: fitted.width * 2, height: fitted.height * 2 })
  assert(Math.abs(scaled.width / scaled.height / (w / h) - 1) < 1e-10, 'resize preserves extreme ratio')
}
assert(moveCanvasNodes(state, [legacy.id], { x: 20, y: 20 }) === state, 'legacy move does not add history')
assert(createCanvasGroup(state, { id: 'group', label: 'group', node_ids: [legacy.id, image.id] }) === state, 'legacy grouping blocked')
assert(autoLayoutCanvasNodes(document, [legacy.id, image.id]) === document, 'layout ignores legacy and avoids no-op histories')
const legacyAttached = attachCanvasResults(createCanvasState({ ...document, nodes: [node('old-gen', 'image_generation')] }, 1), 'old-run', 'old-gen', [{ asset_id: 'old-result', media_type: 'image' }])
assert(legacyAttached.present.nodes[1].type === 'image' && legacyAttached.present.edges[0].input_role === 'result', 'historical result recovery remains supported')
const cloned = createCanvasState(document, 1)
;(cloned.present.nodes[0].payload!.draft as { variables: { subject: string } }).variables.subject = 'changed'
assert((document.nodes[0].payload!.draft as { variables: { subject: string } }).variables.subject === 'cat', 'nested inline drafts are isolated from history snapshots')
assert(addCanvasNode(state, node('empty', 'video_v2')).present.nodes.length === state.present.nodes.length + 1, 'empty v2 is independently creatable')

const boundDocument = withDraft({ prompt_template: '{{@ref_image}}' })
const clipboard = copyCanvasSelection(boundDocument, [image.id, target.id])
const copiedState = pasteCanvasSelection(createCanvasState(boundDocument, 1), clipboard, { x: 40, y: 40 }, (id) => `copy-${id}`)
const copiedImage = copiedState.present.nodes.find((n) => n.id === 'copy-image')!
const copiedTarget = copiedState.present.nodes.find((n) => n.id === 'copy-target')!
assert(copiedImage.payload?.name !== image.payload?.name, 'copied resources receive distinct stable aliases')
assert((copiedTarget.payload?.draft as { prompt_template: string }).prompt_template === `{{@${copiedImage.payload?.name}}}`, 'copied templates remap internal resource aliases')
assert(canvasGenerationEstimateEligible(copiedState.present, copiedTarget.id), 'copied graph retains valid prompt bindings')
let occupied = false
try { attachCanvasResults(attached, 'another-run', target.id, [{ asset_id: 'unrelated-output', media_type: 'image' }]) } catch (error) { occupied = error instanceof Error && error.message === 'output_slot_occupied' }
assert(occupied && attached.present.nodes.find((n) => n.id === target.id)?.asset_id === 'out-1', 'result conflicts cannot overwrite existing media')

const backendWide = canvasMediaSize(10000, 10)
assert(backendWide.width === 1000 && backendWide.height === 1, '1000:1 media expands preferred box to satisfy backend minimum')
const backendResized = resizeCanvasMediaSize(backendWide, { width: 1, height: 1 })
assert(backendResized.width >= 1000 && backendResized.height >= 1 && backendResized.width / backendResized.height === 1000, 'extreme shrink remains backend-valid and ratio locked')
