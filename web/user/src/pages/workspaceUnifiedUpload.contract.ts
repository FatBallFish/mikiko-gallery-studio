import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { toReferenceAsset } from '../../../shared/user-api'

const workspace = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
const api = readFileSync(new URL('../../../shared/user-api.ts', import.meta.url), 'utf8')
assert.ok(!workspace.includes('importReferenceAssetsFromMedia'), 'workbench must not create reference aliases for unified media')
assert.ok(workspace.includes('userApi.uploadReferenceAsset(file, selectedProjectID, { signal: controller.signal, accessToken: app.session?.token })'), 'uploads must capture explicit selected project and cancellation')
assert.ok(workspace.includes('userApi.getReadyMediaReference(detail.asset, selectedProjectID, { signal: controller.signal })'), 'UploadTray completion may still be processing and must wait before projection')
assert.ok(workspace.includes('detail.target || detail.projectID !== selectedProjectID'), 'canvas-targeted and other-project completions must not alter workbench references')
assert.ok(workspace.includes('window.addEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, onMediaUploadCompleted)'))
assert.ok(workspace.includes('workspaceSubmissionIsCurrent(selection, projectSelectionRef.current)'), 'late results must check project generation')
assert.ok(workspace.includes('controllers.forEach((controller) => controller.abort())'), 'completion polling must cancel on effect disposal')
assert.ok(!workspace.includes('.catch(() => undefined)'), 'workspace upload errors must not be silently discarded')
assert.ok(workspace.includes('Promise.allSettled(accepted.map'), 'all sibling uploads must settle before releasing their shared reservation')
assert.ok(workspace.includes('admitReferences(accepted, undefined, async () => {'), 'file upload batches must retain native atomic admission')
assert.ok(workspace.includes('admitReferences([detail.asset], (asset) => asset.id, async () => {'), 'completion events must reserve capacity while polling')
assert.ok(workspace.includes('reserved: reservations.pending(request.token)'), 'completion must revalidate capacity excluding only its own reservation')
assert.ok(workspace.includes('userApi.importReferenceAssetsFromGallery([...new Set(selectedIds)])'), 'gallery imports keep their snapshot-bearing API')
const snapshot = { task_type: 'image_edit', route_model_code: 'existing-model', native_options: { seed: 42 } }
assert.deepEqual(toReferenceAsset({ id: 'legacy-reference', generation_snapshot: snapshot }).generation_snapshot, snapshot, 'reference DTO keeps gallery generation snapshots')
assert.ok(api.includes('native_options: JSON.stringify(native_options)'), 'native estimate options keep their main serialization')
assert.ok(api.includes('rawBody: chunk'), 'unified proxy uploads keep the shared authenticated request transport')
const upload = api.slice(api.indexOf('  uploadReferenceAsset:'), api.indexOf('  listReferenceAssets:'))
assert.ok(upload.includes('uploadUnifiedImage(file, projectID, userApi, options)'))
assert.ok(upload.includes('readUnifiedImageReference(asset, projectID, userApi, options)'))
assert.ok(!upload.includes('formData') && !upload.includes('API_PATHS.agent.referenceAssets'), 'normal uploads use unified media sessions')
console.log('workspace unified upload wiring contracts passed')
