import { API_PATHS } from '../../../shared/api-types'
import type { StorageConfigView } from '../../../shared/api-types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import {
  activateSavedStorageConfig,
  storageActivationLabel,
  storageActivationVersion,
  storageConfigNeedsProbe,
  storageDraftIsDirty,
} from './storageActivation'
const storagePageSource = readFileSync(new URL('./StorageConfigPage.tsx', import.meta.url), 'utf8')

for (const editorContract of ['InlineFeedback', 'data-admin-storage-editor', 'data-storage-section=', 'sticky bottom-0', 'editorState', 'const editorLocked = saving || probing || refreshing || activationPhase', 'disabled={editorLocked}', 'aria-busy={editorLocked}']) {
  if (!storagePageSource.includes(editorContract)) {
    throw new Error(`storage configuration should implement grouped editor state with ${editorContract}`)
  }
}

for (const state of ['pristine', 'dirty', 'validating', 'saving', 'saved', 'failed']) {
  if (!storagePageSource.includes(`'${state}'`)) {
    throw new Error(`storage editor should expose ${state} state`)
  }
}

for (const leaveContract of ['onDirtyChange?: (dirty: boolean) => void', 'onBusyChange?: (busy: boolean) => void', 'onBusyChange?.(editorLocked)', "addEventListener('beforeunload'", 'window.confirm']) {
  if (!storagePageSource.includes(leaveContract)) {
    throw new Error(`storage editor should protect dirty navigation with ${leaveContract}`)
  }
}

for (const operationLabel of ['测试草稿连接', '探测已保存配置', '设为默认']) {
  if (!storagePageSource.includes(operationLabel)) {
    throw new Error(`storage actions should remain semantically distinct with ${operationLabel}`)
  }
}

for (const namespaceContract of [
  'const namespaceLocked = Boolean(draft.id)',
  'disabled={namespaceLocked}',
  '对象地址创建后不可修改；迁移时请新建配置、验证后切换默认写入。',
]) {
  if (!storagePageSource.includes(namespaceContract)) {
    throw new Error(`persisted storage namespace must be immutable with ${namespaceContract}`)
  }
}

for (const concurrencyContract of [
  'saving || probing || refreshing || activationPhase',
  'saving || probing || refreshing || isDirty || activationPhase',
  '正在刷新配置列表与已保存基线',
]) {
  if (!storagePageSource.includes(concurrencyContract)) {
    throw new Error(`storage mutations must stay mutually exclusive with ${concurrencyContract}`)
  }
}

if (storagePageSource.includes('当前编辑内容保持不变')) {
  throw new Error('storage refresh feedback must not claim a draft is preserved when load rebuilds the saved baseline')
}

if (!storagePageSource.includes('当前配置有未保存修改，请先保存后再设为只读。')) {
  throw new Error('persisted read-only mutation must not discard a dirty storage draft')
}

if (!storagePageSource.includes("selectedID ? items.find((item) => item.id === selectedID) ?? null : null")) {
  throw new Error('a new storage draft must not inherit secret or status context from the first saved config')
}

for (const listStateContract of ['last_probe', 'is_default', 'item.status', 'dirty={']) {
  if (!storagePageSource.includes(listStateContract)) {
    throw new Error(`storage object list should expose operational state with ${listStateContract}`)
  }
}

for (const forbiddenPattern of ['<details', 'rounded-2xl', 'rounded-3xl', 'tracking-[', 'uppercase']) {
  if (storagePageSource.includes(forbiddenPattern)) {
    throw new Error(`storage configuration should remove legacy visual drift ${forbiddenPattern}`)
  }
}

if (API_PATHS.ops.storageConfigs !== '/api/ops/admin/v1/storage-configs') {
  throw new Error(`storage configs API path should be stable, got ${API_PATHS.ops.storageConfigs}`)
}

if (API_PATHS.ops.storageConfigDetailProbe !== '/api/ops/admin/v1/storage-configs/{storage_config_id}:probe') {
  throw new Error(`storage config probe API path should be stable, got ${API_PATHS.ops.storageConfigDetailProbe}`)
}

if (API_PATHS.ops.storageConfigSetDefault !== '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default') {
  throw new Error(`storage config set-default API path should be stable, got ${API_PATHS.ops.storageConfigSetDefault}`)
}

if (!storagePageSource.includes('export function StorageConfigPage')) {
  throw new Error('StorageConfigPage should be exported as a React page component')
}

const savedConfig: StorageConfigView = {
  id: 'storage-1',
  code: 'r2-prod',
  name: 'R2 Prod',
  driver: 's3',
  provider: 'r2',
  status: 'enabled',
  read_enabled: true,
  write_enabled: true,
  is_default: false,
  endpoint: 'https://example.r2.cloudflarestorage.com',
  region: 'auto',
  bucket: 'pic-gallery',
  prefix: 'prod',
  force_path_style: false,
  public_base_url: '',
  local_root: '',
  secret_status: { has_secret: true, fingerprint: 'abc123', secret_fields: ['access_key_id', 'secret_access_key'] },
  last_probe: { status: 'never' },
  version: 3,
}

const cleanDraft = {
  ...savedConfig,
  access_key_id: '',
  secret_access_key: '',
}

if (storageDraftIsDirty(cleanDraft, savedConfig)) {
  throw new Error('a draft reconstructed from the saved config should be clean')
}

if (!storageDraftIsDirty({ ...cleanDraft, bucket: 'unsaved-bucket' }, savedConfig)) {
  throw new Error('an edited storage field should mark activation as dirty')
}

if (!storageDraftIsDirty({ ...cleanDraft, secret_access_key: 'unsaved-secret' }, savedConfig)) {
  throw new Error('a pending secret write should mark activation as dirty')
}

if (!storageConfigNeedsProbe(savedConfig) || storageConfigNeedsProbe({ ...savedConfig, last_probe: { status: 'success' } })) {
  throw new Error('only a successful persisted probe should make a storage config activation-ready')
}

if (storageActivationLabel('validating', true) !== '验证连接中...' || storageActivationLabel('activating', false) !== '设为默认中...') {
  throw new Error('storage activation should expose distinct validating and activating labels')
}

const probeResponse = { ...savedConfig, last_probe: { status: 'success' }, version: 4 }
if (storageActivationVersion(savedConfig, probeResponse) !== 4) {
  throw new Error('activation must use the version returned by the persisted probe')
}

const dirtyCalls: string[] = []
try {
  await activateSavedStorageConfig({
    draft: { ...cleanDraft, name: 'Unsaved name' },
    saved: savedConfig,
    probe: async () => { dirtyCalls.push('probe'); return probeResponse },
    setDefault: async () => { dirtyCalls.push('set-default'); return probeResponse },
  })
  throw new Error('dirty activation should have been rejected')
} catch (error) {
  if (!(error instanceof Error) || error.message !== '当前配置有未保存修改，请先保存后再设为默认。') throw error
}
if (dirtyCalls.length !== 0) {
  throw new Error(`dirty activation must not call storage APIs, got ${dirtyCalls.join(',')}`)
}

const workflowCalls: string[] = []
const workflowResult = await activateSavedStorageConfig({
  draft: cleanDraft,
  saved: savedConfig,
  onPhase: (phase) => workflowCalls.push(`phase:${phase}`),
  probe: async (id) => { workflowCalls.push(`probe:${id}`); return probeResponse },
  setDefault: async (id, version) => {
    workflowCalls.push(`set-default:${id}:v${version}`)
    return { ...probeResponse, is_default: true, version: 5 }
  },
})
if (workflowCalls.join('|') !== 'phase:validating|probe:storage-1|phase:activating|set-default:storage-1:v4') {
  throw new Error(`activation should probe then use the returned version, got ${workflowCalls.join('|')}`)
}
if (!workflowResult.is_default) {
  throw new Error('activation should return the set-default response')
}

const readyCalls: string[] = []
await activateSavedStorageConfig({
  draft: cleanDraft,
  saved: { ...savedConfig, last_probe: { status: 'success' } },
  onPhase: (phase) => readyCalls.push(`phase:${phase}`),
  probe: async () => { readyCalls.push('probe'); return probeResponse },
  setDefault: async (_id, version) => { readyCalls.push(`set-default:v${version}`); return probeResponse },
})
if (readyCalls.join('|') !== 'phase:activating|set-default:v3') {
  throw new Error(`a probe-ready config should activate directly, got ${readyCalls.join('|')}`)
}

try {
  await activateSavedStorageConfig({
    draft: cleanDraft,
    saved: savedConfig,
    probe: async () => { throw new Error('bucket permission denied') },
    setDefault: async () => probeResponse,
  })
  throw new Error('probe failure should have been retained')
} catch (error) {
  if (!(error instanceof Error) || error.message !== 'bucket permission denied') throw error
}
