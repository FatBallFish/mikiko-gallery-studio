import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { PublicImageDetail } from '../../components'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { mediaDisplayName, mediaDownloadFilename } from './mediaCreationNavigation'
import { mediaCreationActions } from './mediaExperience'
import { toReferenceAsset } from '../../../../shared/user-api'
import { reuseVideoTask } from '../creation/videoDraft'
import type { VideoTask } from '../../../../shared/api-types'

assert.equal(mediaDisplayName({ id: 'old-image', name: '重名 {标题}' }), '重名 {标题}')
assert.equal(mediaDisplayName({ id: 'old-image' }), '资产 old-imag')
assert.equal(mediaDownloadFilename({ name: '无扩展名', mime_type: 'image/png' }), '无扩展名.png')
assert.equal(mediaDownloadFilename({ name: 'photo.jpeg', mime_type: 'image/jpeg' }), 'photo.jpeg')
assert.equal(mediaDownloadFilename({ name: 'video', mime_type: 'video/mp4' }), 'video.mp4')
const ref = toReferenceAsset({ id: 'ref', name: 'reference_1', display_name: '重复 {名称}' })
assert.equal(ref.name, 'reference_1')
assert.equal(ref.display_name, '重复 {名称}')
const actions = mediaCreationActions({ id: 'image', media_type: 'image', source_task_kind: 'image', source_task_id: 'task' })
assert.equal(new Set(actions.map((action) => action.label)).size, actions.length)
assert.deepEqual(actions.find((action) => action.label === '编辑图片')?.options, { media: 'image', assetId: 'image' })
assert.deepEqual(actions.find((action) => action.label === '首帧生视频')?.options, { media: 'video', assetId: 'image' })
const task = { id: 'task', project_id: 'project', status: 'succeeded', items: [], route_model_code: 'model', task_type: 'image_to_video', prompt_template: '{{@original_alias}} {{$weather}}', prompt_binding_snapshot: { variables: [{ name: 'weather', value: '雨' }], references: [{ name: 'original_alias', asset_id: 'image' }] }, inputs: [{ asset_id: 'image', role: 'first_frame', ordinal: 0 }], duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent', requested_output_count: 1 } as VideoTask
const reused = reuseVideoTask(task)
assert.deepEqual(reused.prompt_variables, [{ name: 'weather', value: '雨' }])
assert.deepEqual(reused.reference_bindings, [{ name: 'original_alias', asset_id: 'image' }])
assert.equal(reused.inputs[0].asset_id, 'image')
const component = readFileSync(new URL('../../components.tsx', import.meta.url), 'utf8')
assert.ok(!component.includes('复用 Prompt'))
assert.ok(component.includes('owned.key === action.key || owned.label === action.label'), 'shared owned actions must deduplicate caller actions')
const creation = readFileSync(new URL('../creation/CreationPage.tsx', import.meta.url), 'utf8')
assert.ok(creation.includes('creationReuseEvent') && creation.includes('key={videoEntryRevision}'), 'same video task reuse must restore even when hash is unchanged')
const preview = readFileSync(new URL('./MediaPreviewDialog.tsx', import.meta.url), 'utf8')
assert.ok(preview.includes("asset.media_type !== 'video' ? <label"), 'video must not expose canvas selector')
assert.ok(preview.includes("asset.media_type === 'image' ? 'image_v2'"), 'asset insertion must create the current canvas image type')
assert.ok(preview.includes('name: `asset_${asset.id.replace('), 'canvas display names must not become prompt binding aliases')
const detail = { id: 'image', name: '公开作品', width: 1024, height: 1024, publish_status: 'approved', visibility_status: 'approved', prompt: '公开提示词', route_model_code: 'secret-route', abstract_model: 'secret-model', project: { id: 'project', name: 'secret-project' }, image_count: 13 } as Parameters<typeof PublicImageDetail>[0]['image']
const publicMarkup = renderToStaticMarkup(createElement(PublicImageDetail, { context: 'public', image: detail, onCopyPrompt: () => undefined, actions: [{ key: 'delete', label: '删除', icon: null, onClick: () => undefined }, { key: 'edit-image', label: '编辑图片', icon: null, onClick: () => undefined }], organization: createElement('span', null, 'secret-organization') }))
for (const privateValue of ['secret-route', 'secret-model', 'secret-project', 'secret-organization', '输出数量', '编辑图片', '删除']) assert.ok(!publicMarkup.includes(privateValue), `public detail leaked ${privateValue}`)
assert.ok(publicMarkup.includes('已公开') && !publicMarkup.includes('私有'), 'approved alias must show published visibility')
const ownedMarkup = renderToStaticMarkup(createElement(PublicImageDetail, { context: 'owned', image: detail, onCopyPrompt: () => undefined }))
assert.ok(ownedMarkup.includes('secret-project') && ownedMarkup.includes('输出数量'), 'owned detail must retain owner metadata')
assert.ok(!component.includes('window.prompt') && !preview.includes('window.prompt'), 'detail editing must use custom dialogs')
assert.ok(!component.includes('async function share()'), 'detail share action must be removed')
console.log('media detail actions contract passed')
