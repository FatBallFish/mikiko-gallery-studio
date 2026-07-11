import type { StorageConfigView } from '../../../shared/api-types'

export type StorageActivationPhase = 'idle' | 'validating' | 'activating'

export type StorageEditableDraft = Pick<StorageConfigView,
  | 'id'
  | 'code'
  | 'name'
  | 'driver'
  | 'provider'
  | 'status'
  | 'read_enabled'
  | 'write_enabled'
  | 'endpoint'
  | 'region'
  | 'bucket'
  | 'prefix'
  | 'force_path_style'
  | 'public_base_url'
  | 'local_root'
> & {
  access_key_id: string
  secret_access_key: string
}

function normalizedEditableValues(source: StorageEditableDraft | StorageConfigView) {
  return [
    source.id,
    source.code.trim(),
    source.name.trim(),
    source.driver,
    source.provider,
    source.status,
    source.read_enabled,
    source.write_enabled,
    source.endpoint?.trim() ?? '',
    source.region?.trim() ?? '',
    source.bucket?.trim() ?? '',
    source.prefix?.trim() ?? '',
    source.force_path_style,
    source.public_base_url?.trim() ?? '',
    source.local_root?.trim() ?? '',
  ]
}

export function storageDraftIsDirty(draft: StorageEditableDraft, saved: StorageConfigView) {
  if (draft.access_key_id.trim() || draft.secret_access_key.trim()) return true
  return JSON.stringify(normalizedEditableValues(draft)) !== JSON.stringify(normalizedEditableValues(saved))
}

export function storageConfigNeedsProbe(saved: StorageConfigView) {
  return saved.last_probe?.status?.trim().toLowerCase() !== 'success'
}

export function storageActivationLabel(phase: StorageActivationPhase, needsProbe: boolean) {
  if (phase === 'validating') return '验证连接中...'
  if (phase === 'activating') return '设为默认中...'
  return needsProbe ? '验证并设为默认' : '设为默认'
}

export function storageActivationVersion(saved: StorageConfigView, probed?: StorageConfigView) {
  const activationSource = probed ?? saved
  if (storageConfigNeedsProbe(activationSource)) {
    throw new Error(activationSource.last_probe?.message || '存储配置连接测试未通过，默认存储未切换。')
  }
  return activationSource.version
}

export async function activateSavedStorageConfig(options: {
  draft: StorageEditableDraft
  saved: StorageConfigView
  probe: (id: string) => Promise<StorageConfigView>
  setDefault: (id: string, version: number) => Promise<StorageConfigView>
  onPhase?: (phase: StorageActivationPhase) => void
}) {
  const { draft, saved, probe, setDefault, onPhase } = options
  if (storageDraftIsDirty(draft, saved)) {
    throw new Error('当前配置有未保存修改，请先保存后再设为默认。')
  }

  let version: number
  if (storageConfigNeedsProbe(saved)) {
    onPhase?.('validating')
    const probed = await probe(saved.id)
    version = storageActivationVersion(saved, probed)
  } else {
    version = storageActivationVersion(saved)
  }

  onPhase?.('activating')
  return setDefault(saved.id, version)
}
