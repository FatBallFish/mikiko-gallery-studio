import assert from 'node:assert/strict'
import fs from 'node:fs'
import { canvasAssetUnavailableReason, canvasGenerationAssetIDs, mergeCanvasAssetPage, resolveCanvasMarkedReference, validateCanvasGenerationAssets } from './canvasAssets'
import type { MediaAsset } from '../../../../shared/api-types'
import type { CanvasDocument } from './core/types'

const asset = (id: string, status = 'ready'): MediaAsset => ({ id, project_id: 'project-a', name: id, status, media_type: 'image' } as MediaAsset)
for (const status of ['ready', 'ready_original']) assert.equal(canvasAssetUnavailableReason(asset('a', status)), '')
for (const [status, copy] of [['uploading', '上传'], ['processing', '处理'], ['failed', '失败'], ['deleted', '已删除'], ['deleting', '正在删除'], ['expired', '已过期'], ['unknown', '尚不可用']]) {
  assert.ok(canvasAssetUnavailableReason(asset('a', status)).includes(copy))
}
const merged = mergeCanvasAssetPage([asset('a'), asset('b', 'processing')], [asset('b'), asset('c')])
assert.deepEqual(merged.map((item) => item.id), ['a', 'b', 'c'])
assert.equal(merged[1].status, 'ready')
const document = {
  nodes: [{ id: 'image-a', type: 'image_v2', asset_id: 'a' }, { id: 'image-b', type: 'image_v2', asset_id: 'a' }, { id: 'unrelated', type: 'image', asset_id: 'unused' }],
  edges: [{ source: 'image-a', target: 'gen', input_role: 'reference' }, { source: 'image-b', target: 'gen', input_role: 'reference' }, { source: 'unrelated', target: 'other', input_role: 'reference' }],
} as CanvasDocument
assert.deepEqual(canvasGenerationAssetIDs(document, 'gen'), ['a'])
const calls: string[] = []
await validateCanvasGenerationAssets(document, 'gen', async (id) => { calls.push(id); return asset(id, 'ready_original') })
assert.deepEqual(calls, ['a'])
await assert.rejects(validateCanvasGenerationAssets(document, 'gen', async (id) => asset(id, 'processing')), /正在处理/)
await assert.rejects(validateCanvasGenerationAssets(document, 'gen', async (id) => asset(id, 'failed')), /处理失败/)
await validateCanvasGenerationAssets(document, 'gen', async (id) => ({ ...asset(id), project_id: 'other' }))
await assert.rejects(validateCanvasGenerationAssets(document, 'gen', async (id) => ({ ...asset(id), media_type: 'video' })), /类型/)
await assert.rejects(validateCanvasGenerationAssets(document, 'gen', async () => { throw new Error('资产不存在') }), /不存在/)
await validateCanvasGenerationAssets(document, 'no-inputs', async () => { throw new Error('must not fetch unused assets') })

// Marked references keep the ready original and pending marks until processing succeeds.
let selected = 'original-ready'
let markDraft = 'pending-marks'
const replace = (next: MediaAsset) => { selected = next.id }
await assert.rejects(resolveCanvasMarkedReference(asset('marked', 'processing'), {
  getAsset: async () => asset('marked', 'failed'), isCurrent: () => true, wait: async () => {}, replace,
}).then(() => { markDraft = '' }), /处理失败/)
assert.equal(selected, 'original-ready')
assert.equal(markDraft, 'pending-marks')
const readyID = await resolveCanvasMarkedReference(asset('marked', 'processing'), {
  getAsset: async () => asset('marked', 'ready_original'), isCurrent: () => true,
  wait: async () => { assert.equal(selected, 'original-ready') }, replace,
})
assert.equal(readyID, 'marked')
assert.equal(selected, 'marked')
selected = 'original-ready'
let contextCurrent = true
await assert.rejects(resolveCanvasMarkedReference(asset('late', 'processing'), {
  getAsset: async () => { contextCurrent = false; return asset('late', 'ready_original') },
  isCurrent: () => contextCurrent, wait: async () => {}, replace,
}), /引用已变化/)
assert.equal(selected, 'original-ready')

const drawer = fs.readFileSync(new URL('./CanvasAssetDrawer.tsx', import.meta.url), 'utf8')
for (const marker of ['media_type: mediaType', 'page.next_cursor', 'cursor: append ? cursor', 'mergeCanvasAssetPage', 'if (!alive) return', 'MEDIA_ASSETS_CHANGED_EVENT', 'detail.projectID === projectID', 'schedulePoll()', 'userApi.getMediaAsset', 'draggable={!reason}', 'canvasAssetUnavailableReason', '加载更多']) assert.ok(drawer.includes(marker), `missing drawer contract: ${marker}`)
const editor = fs.readFileSync(new URL('./CanvasEditorPage.tsx', import.meta.url), 'utf8')
assert.ok(editor.includes('void resolveCanvasMarkedReference(detail.asset,'))
assert.ok(editor.includes('uploadSession?.store === store'))
assert.ok(editor.includes('projects.selectProject(canvas.project_id)'))
assert.ok(editor.includes('if (!canUseAsset(asset)) return'))
const generate = editor.slice(editor.indexOf('async function generateNode'), editor.indexOf('async function attachRun'))
assert.ok(generate.indexOf('await validateCanvasGenerationAssets') < generate.indexOf('await userApi.generateCanvasNode'))
assert.ok(generate.includes('canvasGenerationEstimateSignature'))

assert.ok(editor.includes('insertCanvasReference(store!, asset, assetTargetNodeID, assetRole'))
assert.ok(editor.includes('settleCanvasReferenceUpload'))
assert.ok(editor.includes('userApi.generateCanvasNode(canvas!.id, node.id, crypto.randomUUID())'))
assert.ok(!editor.includes("asset.project_id !== canvas!.project_id"), 'owned cross-project canvas references must remain usable')
assert.ok(editor.includes('const isCurrentUpload = () => alive && activeUserRef.current === uploadUser && activeCanvasIDRef.current === canvasID && remoteSessionRef.current === uploadSession'))
assert.ok(editor.includes('uploadTargetsRef.current.get(token) === target'), 'processing response must still own its upload destination')
assert.ok(editor.includes('uploadTargetsRef.current.clear()'), 'navigation must discard old upload destinations and reservations')
