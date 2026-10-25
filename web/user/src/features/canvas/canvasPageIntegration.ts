import { useEffect, useState } from 'react'
import type { MediaAsset, ReferenceAsset } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import type { CanvasDocument, CanvasEdge, CanvasNode } from './core/types'
import { inspectCanvasConnection } from './core/canvasState'
import { canvasMediaSize } from './core/canvasMediaSizing'
import { canvasNodeMediaKind, isCanvasGenerationNode } from './core/canvasNodeKinds'
import type { createCanvasStore } from './store/canvasStore'

export function canvasEstimateSufficient(balance: string | undefined, estimate: { points?: string; detail?: Record<string, unknown> }) {
  const units = (value: unknown): bigint | undefined => {
    const match = /^(\d+)(?:\.(\d{1,5}))?$/.exec(String(value ?? '').trim())
    return match ? BigInt(match[1]) * 100000n + BigInt((match[2] ?? '').padEnd(5, '0')) : undefined
  }
  const available = units(balance)
  const required = units(estimate.detail?.max_reserved_points ?? estimate.points)
  return available !== undefined && required !== undefined && available >= required
}

export function canvasConsoleBounds(viewport: { width: number; height: number }, anchor: { x: number; y: number }, contentHeight = 440) {
  const marginX = Math.min(16, Math.max(0, viewport.width / 2))
  const marginY = Math.min(16, Math.max(0, viewport.height / 2))
  const width = Math.max(0, Math.min(540, viewport.width - marginX * 2))
  const availableHeight = Math.max(0, viewport.height - marginY * 2)
  const height = Math.min(availableHeight, Math.max(0, contentHeight))
  const left = Math.max(marginX, Math.min(viewport.width - width - marginX, anchor.x))
  const top = Math.max(marginY, Math.min(viewport.height - height - marginY, anchor.y))
  return { left, top, width, maxHeight: Math.max(0, viewport.height - top - marginY) }
}

export function canvasQuoteRemainingMS(detail?: Record<string, unknown>, now = Date.now()) {
  if (!detail?.expires_at) return Infinity
  const expires = Date.parse(String(detail.expires_at))
  return Number.isFinite(expires) ? expires - now - 5000 : 0
}

export type CanvasHydratedMedia = { asset: MediaAsset; url: string; expiresAt?: string }
export function useCanvasMediaHydration(document: CanvasDocument, token?: string) {
  const [media, setMedia] = useState<Record<string, CanvasHydratedMedia>>({})
  const ids = [...new Set(document.nodes.flatMap((node) => node.asset_id ? [node.asset_id] : []))].sort().join('|')
  useEffect(() => {
    let alive = true
    let timer: ReturnType<typeof setTimeout> | undefined
    const refresh = async () => {
      const pending = ids.split('|').filter(Boolean)
      const values: Array<readonly [string, CanvasHydratedMedia] | null> = []
      const hydrate = async (id: string) => {
        try {
          const [asset, access] = await Promise.all([userApi.getMediaAsset(id), userApi.getMediaAssetAccess(id, 'preview')])
          return [id, { asset, url: userApi.mediaAssetUrl(access.url, token), expiresAt: access.expires_at }] as const
        } catch { return null }
      }
      await Promise.all(Array.from({ length: Math.min(6, pending.length) }, async () => {
        while (alive && pending.length) { const id = pending.shift()!; values.push(await hydrate(id)) }
      }))
      if (!alive) return
      setMedia(Object.fromEntries(values.filter((value): value is NonNullable<typeof value> => value !== null)))
      // Refresh signed access while the canvas stays open, without persisting URLs or touching undo history.
      const expiry = Math.min(...values.flatMap((value) => value?.[1].expiresAt ? [Date.parse(value[1].expiresAt)] : []))
      timer = setTimeout(() => void refresh(), Number.isFinite(expiry) ? Math.max(10000, Math.min(240000, expiry - Date.now() - 30000)) : 240000)
    }
    const changed = () => { if (timer) clearTimeout(timer); void refresh() }
    void refresh()
    window.addEventListener('mgs:media-assets-changed', changed)
    return () => { alive = false; if (timer) clearTimeout(timer); window.removeEventListener('mgs:media-assets-changed', changed) }
  }, [ids, token])
  return media
}
export function canvasAssetNode(asset: MediaAsset, position: { x: number; y: number }): CanvasNode {
  const size = asset.media_type === 'audio' ? { width: 280, height: 140 } : canvasMediaSize(asset.width || 16, asset.height || 9)
  return { id: `${asset.media_type}-v2-${crypto.randomUUID().slice(0, 8)}`, type: asset.media_type === 'image' ? 'image_v2' : asset.media_type === 'video' ? 'video_v2' : 'audio', asset_id: asset.id, position, size, payload: { title: asset.name, name: `ref_${asset.id.replace(/[^a-zA-Z0-9_]/g, '_')}`, mime_type: asset.mime_type, width: asset.width, height: asset.height, duration_ms: asset.duration_ms, source_type: asset.source_type, source_task_kind: asset.source_task_kind, source_task_id: asset.source_task_id } }
}
export function validateCanvasReference(document: CanvasDocument, source: CanvasNode, targetID: string, role: CanvasEdge['input_role']) {
  const target = document.nodes.find((node) => node.id === targetID)
  if (!target || !isCanvasGenerationNode(target)) return '目标节点已改变，请重新选择'
  if (!source.asset_id) return '请选择已有媒体结果的节点'
  const expected = role === 'reference_video' ? 'video' : role === 'reference_audio' ? 'audio' : 'image'
  if (canvasNodeMediaKind(source) !== expected) return `请选择${expected === 'video' ? '视频' : expected === 'audio' ? '音频' : '图片'}素材`
  if (document.edges.some((edge) => edge.target === targetID && (edge.source === source.id || document.nodes.find((node) => node.id === edge.source)?.asset_id === source.asset_id))) return '该素材已经添加'
  const error = inspectCanvasConnection(document, { id: 'candidate', source: source.id, target: targetID, input_role: role })
  return error === 'cycle' ? '当前生成关系不能形成循环' : error ? '此素材与参考角色不兼容，或该角色已有素材' : ''
}
export function insertCanvasReference(store: ReturnType<typeof createCanvasStore>, asset: MediaAsset, targetID: string, role: CanvasEdge['input_role']) {
  const document = store.getState().command.present
  const target = document.nodes.find((node) => node.id === targetID)
  if (!target) throw new Error('目标节点已删除')
  if (!['ready', 'ready_original'].includes(asset.status)) throw new Error('素材尚未准备就绪')
  const source = document.nodes.find((node) => node.asset_id === asset.id) ?? canvasAssetNode(asset, { x: target.position.x - 440, y: target.position.y })
  const candidate = document.nodes.includes(source) ? document : { ...document, nodes: [...document.nodes, source] }
  const error = validateCanvasReference(candidate, source, targetID, role)
  if (error) throw new Error(error)
  if (!document.nodes.includes(source)) store.getState().addNode(source)
  store.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: source.id, target: targetID, input_role: role, ordinal: document.edges.filter((edge) => edge.target === targetID).length })
  store.getState().select([targetID])
}
export function canvasReferenceAsset(node: CanvasNode, hydrated?: CanvasHydratedMedia): ReferenceAsset {
  return { id: node.asset_id!, name: String(node.payload?.name ?? `ref_${node.asset_id!.replace(/[^a-zA-Z0-9_]/g, '_')}`), status: 'ready', created_at: '', mime_type: hydrated?.asset.mime_type ?? String(node.payload?.mime_type ?? ''), width: hydrated?.asset.width ?? (Number(node.payload?.width) || undefined), height: hydrated?.asset.height ?? (Number(node.payload?.height) || undefined), preview_url: hydrated?.url }
}
