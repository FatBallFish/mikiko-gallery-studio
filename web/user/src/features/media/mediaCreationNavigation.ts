import type { MediaAsset } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import type { UserRouteOptions } from '../../routeState'
import { userHashForRoute } from '../../routeState'
import { stageWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot } from '../../pages/workspaceCreationDraft'

export const creationReuseEvent = 'mgs:creation-draft'

// Stage the complete image configuration before navigation, including on the current route.
export async function continueMediaCreation(options: UserRouteOptions) {
  if (options.media === 'image') {
    if (options.taskId) {
      const task = await userApi.getTask(options.taskId)
      stageWorkspaceCreationDraft(workspaceCreationDraftFromSnapshot(task), window.sessionStorage, window.history)
    } else if (options.assetId) {
      const assets = await userApi.importReferenceAssetsFromMedia([options.assetId])
      if (!assets.length) throw new Error('图片无法用作编辑参考')
      stageWorkspaceCreationDraft({ version: 1, intent: 'asset-edit', prompt: '', task_type: 'image_edit', reference_asset_ids: assets.map((asset) => asset.id) }, window.sessionStorage, window.history)
    }
  }
  window.location.hash = userHashForRoute('genpic', options)
  window.dispatchEvent(new Event(creationReuseEvent))
}

export function mediaDownloadFilename(asset: Pick<MediaAsset, 'name' | 'mime_type'>) {
  const extension = ({ 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp', 'image/gif': 'gif', 'image/avif': 'avif', 'video/mp4': 'mp4', 'video/webm': 'webm', 'audio/mpeg': 'mp3', 'audio/wav': 'wav', 'audio/ogg': 'ogg' } as Record<string, string>)[asset.mime_type.toLowerCase()]
  const name = asset.name.replace(/[\\/\\\\\u0000-\u001f]/g, '_').trim() || 'download'
  const existingExtension = name.split('.').pop()?.toLowerCase()
  return extension && existingExtension !== extension && !(extension === 'jpg' && existingExtension === 'jpeg') ? `${name}.${extension}` : name
}

export function mediaDisplayName(asset: Pick<MediaAsset, 'id'> & { name?: string }) {
  return asset.name?.trim() || `资产 ${asset.id.slice(0, 8)}`
}
