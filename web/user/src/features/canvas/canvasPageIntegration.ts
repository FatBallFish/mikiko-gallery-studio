import { useEffect, useState } from 'react'
import type { Capability, MediaAsset, ReferenceAsset, VideoCapability } from '../../../../shared/api-types'
import { referenceAdmission, type ReferenceAdmissionPolicy } from '../../../../shared/reference-admission'
import { VIDEO_TASK_INPUT_ROLES, resolveInputCapabilities, videoInputAssetReason, type VideoDraftInputRole } from '../creation/videoDraft'
import { userApi } from '../../../../shared/user-api'
import type { CanvasDocument, CanvasEdge, CanvasNode } from './core/types'
import { inspectCanvasConnection, nextCanvasReferenceOrdinal } from './core/canvasState'
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
  return { id: `${asset.media_type}-v2-${crypto.randomUUID().slice(0, 8)}`, type: asset.media_type === 'image' ? 'image_v2' : asset.media_type === 'video' ? 'video_v2' : 'audio', asset_id: asset.id, position, size, payload: { title: asset.name, name: `ref_${asset.id.replace(/[^a-zA-Z0-9_]/g, '_')}`, mime_type: asset.mime_type, width: asset.width, height: asset.height, duration_ms: asset.duration_ms, file_size_bytes: asset.file_size_bytes, source_type: asset.source_type, source_task_kind: asset.source_task_kind, source_task_id: asset.source_task_id } }
}
export type CanvasReferenceKind = 'image' | 'video' | 'audio' | 'first_frame' | 'last_frame'
export type CanvasReferenceEntry = { id: string; kind: CanvasReferenceKind }
export type CanvasAdmissionContext = { image: Capability | null; video: VideoCapability | null; reserved?: Partial<Record<CanvasReferenceKind, number>> }
export function canvasReferenceKind(role: CanvasEdge['input_role']): CanvasReferenceKind {
  return role === 'first_frame' || role === 'last_frame' ? role : role === 'reference_video' ? 'video' : role === 'reference_audio' ? 'audio' : 'image'
}
export function canvasReferenceEntries(document: CanvasDocument, targetID: string): CanvasReferenceEntry[] {
  return document.edges.filter((edge) => edge.target === targetID && edge.input_role !== 'prompt').flatMap((edge) => {
    const source = document.nodes.find((node) => node.id === edge.source)
    return source?.asset_id ? [{ id: source.asset_id, kind: canvasReferenceKind(edge.input_role) }] : []
  })
}
export function canvasReferencePolicy(document: CanvasDocument, targetID: string, context: CanvasAdmissionContext, adding = false): ReferenceAdmissionPolicy<CanvasReferenceKind> {
  const node = document.nodes.find((item) => item.id === targetID)
  const draft = (node?.payload?.draft ?? {}) as Record<string, unknown>
  const code = String(draft.route_model_code ?? draft.abstract_model ?? '')
  if (!node || !isCanvasGenerationNode(node)) return { limits: {}, reason: '目标节点已改变，请重新选择' }
  if (node.type === 'image_v2') {
    if (!context.image) return { limits: {}, reason: '正在加载图片模型能力' }
    const model = context.image.model_groups.find((item) => item.code === code)
    if (!model) return { limits: {}, reason: '当前图片型号不可用，请手动选择可用型号' }
    const task = adding || canvasReferenceEntries(document, targetID).length ? 'image_edit' : 'text_to_image'
    if (!model.task_types.includes(task)) return { limits: {}, reason: '当前图片型号不支持参考编辑，请手动选择支持的型号' }
    const scoped = model.capabilities_by_task_type?.[task]
    const profile = scoped?.native_image_profile ?? model.native_image_profile
    const limit = scoped?.max_reference_image_count ?? (model.capabilities_by_task_type ? undefined : model.max_reference_image_count) ?? profile?.max_reference_images
    if (draft.operation === 'layer_decomposition' && !profile?.operations.includes('layer_decomposition')) return { limits: {}, reason: '当前型号不支持图层拆分，请手动调整操作' }
    return { limits: { image: draft.operation === 'layer_decomposition' ? 1 : limit, video: 0, audio: 0, first_frame: 0, last_frame: 0 } }
  }
  if (!context.video) return { limits: {}, reason: '正在加载视频模型能力' }
  const model = context.video.model_groups.find((item) => item.code === code)
  const task = String(draft.task_type ?? 'text_to_video') as keyof typeof VIDEO_TASK_INPUT_ROLES
  if (!model || !model.task_types.includes(task) || !model.options_by_task_type[task]) return { limits: {}, reason: '当前视频型号或生成方式不可用，请手动调整' }
  if (task === 'text_to_video') return { limits: { image: 0, video: 0, audio: 0, first_frame: 0, last_frame: 0 } }
  const inputs = resolveInputCapabilities(model, task)
  if (!inputs) return { limits: {}, reason: '当前视频型号的输入能力尚未公开，请刷新或手动选择支持的型号' }
  const limits: ReferenceAdmissionPolicy<CanvasReferenceKind>['limits'] = {}
  for (const role of ['first_frame', 'last_frame', 'reference_image', 'reference_video', 'reference_audio'] as VideoDraftInputRole[]) {
    const kind = canvasReferenceKind(role)
    if (!VIDEO_TASK_INPUT_ROLES[task].includes(role)) { limits[kind] = 0; continue }
    const input = inputs[role]
    const media = role === 'reference_video' ? 'video' : role === 'reference_audio' ? 'audio' : 'image'
    limits[kind] = input?.media_types.includes(media) ? input.max_count : input ? 0 : null
  }
  return { limits }
}
export function canvasReferenceAdmission(document: CanvasDocument, targetID: string, incoming: CanvasReferenceEntry[], context: CanvasAdmissionContext) {
  return referenceAdmission({ policy: canvasReferencePolicy(document, targetID, context, incoming.length > 0), current: canvasReferenceEntries(document, targetID), incoming, reserved: context.reserved, kind: (item) => item.kind, key: (item) => item.id })
}
export function validateCanvasReference(document: CanvasDocument, source: CanvasNode, targetID: string, role: CanvasEdge['input_role'], context?: CanvasAdmissionContext) {
  const target = document.nodes.find((node) => node.id === targetID)
  if (!target || !isCanvasGenerationNode(target)) return '目标节点已改变，请重新选择'
  if (!source.asset_id) return '请选择已有媒体结果的节点'
  const expected = role === 'reference_video' ? 'video' : role === 'reference_audio' ? 'audio' : 'image'
  if (canvasNodeMediaKind(source) !== expected) return `请选择${expected === 'video' ? '视频' : expected === 'audio' ? '音频' : '图片'}素材`
  if (document.edges.some((edge) => edge.target === targetID && (edge.source === source.id || document.nodes.find((node) => node.id === edge.source)?.asset_id === source.asset_id))) return '该素材已经添加'
  if (target.type === 'video_v2' && context?.video) {
    const draft = (target.payload?.draft ?? {}) as Record<string, unknown>
    const model = context.video.model_groups.find((item) => item.code === draft.route_model_code)
    const reason = videoInputAssetReason(model, draft.task_type as import('../../../../shared/api-types').VideoTaskType, role as VideoDraftInputRole, { media_type: expected, mime_type: String(source.payload?.mime_type ?? ''), name: String(source.payload?.title ?? ''), file_size_bytes: Number(source.payload?.file_size_bytes ?? 0) })
    if (reason) return reason
  }
  const admission = canvasReferenceAdmission(document, targetID, [{ id: source.asset_id, kind: canvasReferenceKind(role) }], context ?? { image: null, video: null })
  if (!admission.ok) return admission.reason
  const error = inspectCanvasConnection(document, { id: 'candidate', source: source.id, target: targetID, input_role: role })
  return error === 'cycle' ? '当前生成关系不能形成循环' : error ? '此素材与参考角色不兼容，或该角色已有素材' : ''
}
export function insertCanvasReference(store: ReturnType<typeof createCanvasStore>, asset: MediaAsset, targetID: string, role: CanvasEdge['input_role'], context?: CanvasAdmissionContext) {
  const document = store.getState().command.present
  const target = document.nodes.find((node) => node.id === targetID)
  if (!target) throw new Error('目标节点已删除')
  if (!['ready', 'ready_original'].includes(asset.status)) throw new Error('素材尚未准备就绪')
  const source = document.nodes.find((node) => node.asset_id === asset.id) ?? canvasAssetNode(asset, { x: target.position.x - 440, y: target.position.y })
  const candidate = document.nodes.includes(source) ? document : { ...document, nodes: [...document.nodes, source] }
  const error = validateCanvasReference(candidate, source, targetID, role, context)
  if (error) throw new Error(error)
  if (!document.nodes.includes(source)) store.getState().addNode(source)
  store.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: source.id, target: targetID, input_role: role, ordinal: nextCanvasReferenceOrdinal(document.edges, targetID) })
  store.getState().select([targetID])
}
export function canvasReferenceAsset(node: CanvasNode, hydrated?: CanvasHydratedMedia): ReferenceAsset {
  return { id: node.asset_id!, name: String(node.payload?.name ?? `ref_${node.asset_id!.replace(/[^a-zA-Z0-9_]/g, '_')}`), status: 'ready', created_at: '', mime_type: hydrated?.asset.mime_type ?? String(node.payload?.mime_type ?? ''), width: hydrated?.asset.width ?? (Number(node.payload?.width) || undefined), height: hydrated?.asset.height ?? (Number(node.payload?.height) || undefined), preview_url: hydrated?.url }
}

export function settleCanvasReferenceUpload<T extends { reservation?: number }>(targets: Map<string, T>, token: string, status: 'failed' | 'cancelled', release: (target: T) => void) {
  const target = targets.get(token)
  if (!target) return false
  release(target)
  if (status === 'cancelled') targets.delete(token)
  else targets.set(token, { ...target, reservation: undefined })
  return true
}
