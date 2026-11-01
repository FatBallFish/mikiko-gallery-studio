import type { MediaAsset } from '../../../../shared/api-types'
import { canvasNodeMediaKind } from './core/canvasNodeKinds'
import type { CanvasDocument } from './core/types'

export function canvasAssetUnavailableReason(asset: Pick<MediaAsset, 'status'>): string {
  switch (asset.status) {
    case 'ready': case 'ready_original': return ''
    case 'uploading': return '资产正在上传，请等待上传完成'
    case 'processing': return '资产正在处理，请稍后再试'
    case 'failed': return '资产处理失败，请在媒体库重试处理'
    case 'deleting': return '资产正在删除，无法使用'
    case 'deleted': return '资产已删除，请重新选择'
    case 'expired': return '资产已过期，请重新上传'
    default: return '资产尚不可用，请刷新后重试'
  }
}

export async function resolveCanvasMarkedReference(initial: MediaAsset, options: {
  getAsset: (id: string) => Promise<MediaAsset>
  isCurrent: () => boolean
  replace: (asset: MediaAsset) => void
  wait?: () => Promise<void>
}): Promise<string> {
  let asset = initial
  const assertCurrent = () => {
    if (!options.isCurrent()) throw new Error('引用已变化，已上传资产可从资产库恢复')
  }
  while (true) {
    assertCurrent()
    if (asset.status !== 'processing' && asset.status !== 'uploading') break
    await (options.wait?.() ?? new Promise<void>((resolve) => setTimeout(resolve, 3000)))
    assertCurrent()
    asset = await options.getAsset(initial.id)
  }
  assertCurrent()
  const reason = canvasAssetUnavailableReason(asset)
  if (reason) throw new Error(reason)
  if (asset.media_type !== 'image') throw new Error('标记引用必须为图片资产')
  options.replace(asset)
  return asset.id
}

export function mergeCanvasAssetPage(current: MediaAsset[], incoming: MediaAsset[]): MediaAsset[] {
  const assets = new Map(current.map((asset) => [asset.id, asset]))
  incoming.forEach((asset) => assets.set(asset.id, asset))
  return [...assets.values()]
}

const inputRoles = new Set(['reference', 'first_frame', 'last_frame', 'reference_image', 'reference_video', 'reference_audio'])
export function canvasGenerationAssetIDs(document: CanvasDocument, nodeID: string): string[] {
  const sources = new Set(document.edges.filter((edge) => edge.target === nodeID && inputRoles.has(edge.input_role)).map((edge) => edge.source))
  return [...new Set(document.nodes.filter((node) => sources.has(node.id) && node.asset_id).map((node) => node.asset_id!))]
}

export async function validateCanvasGenerationAssets(document: CanvasDocument, nodeID: string, getAsset: (id: string) => Promise<MediaAsset>): Promise<void> {
  await Promise.all(canvasGenerationAssetIDs(document, nodeID).map(async (id) => {
    const asset = await getAsset(id)
    // Existing canvas nodes may reference owned media from another project.
    // Ownership and current permissions are enforced by getMediaAsset and generation.
    const sourceTypes = document.nodes.filter((node) => node.asset_id === id && document.edges.some((edge) => edge.source === node.id && edge.target === nodeID && inputRoles.has(edge.input_role))).map(canvasNodeMediaKind)
    if (sourceTypes.some((type) => type !== asset.media_type)) throw new Error(`${asset.name}：资产类型与输入节点不匹配，请重新选择`)
    const reason = canvasAssetUnavailableReason(asset)
    if (reason) throw new Error(`${asset.name}：${reason}`)
  }))
}
