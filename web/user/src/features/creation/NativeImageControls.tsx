import { useEffect, useRef, useState } from 'react'
import type { NativeImageProfile, NativeImageRequestFields } from '../../../../shared/native-image'
import { userApi } from '../../../../shared/user-api'
import { ReferenceMarkModal, markAnnotations, type Mark } from './ReferenceMarkModal'
import { bindReferenceAnnotations, remapReferenceAnnotations, userNativeImageFields } from './nativeImageUX'
import { nativeLayerUnavailableReason } from '../../pages/workspaceParameters'
import './nativeImageUX.css'

export function NativeImageControls({ profile, value, onChange, disabled, references = [], editingReferenceID, onEditingReferenceChange, onUploadMarkedReference, onPendingChange, onEditedChange, onRestoreReference, accessToken, layerPriceReady = false, layerUnavailableReason, mappingManaged = false, persistedMarks, onPersistMarks, persistenceKey, onRefreshReference }: {
  onRefreshReference?: (assetID: string) => Promise<string>
  persistedMarks?: unknown; onPersistMarks?: (value: unknown) => void; persistenceKey?: string
  mappingManaged?: boolean; profile?: NativeImageProfile; value: NativeImageRequestFields; onChange: (value: NativeImageRequestFields) => void; disabled?: boolean; accessToken?: string | null
  references?: Array<{ id: string; name: string; preview_url?: string }>
  editingReferenceID?: string; onEditingReferenceChange?: (id: string) => void
  onRestoreReference?: (assetID: string, originalID: string) => Promise<void>
  onUploadMarkedReference?: (file: File, assetID: string) => Promise<string>
  onPendingChange?: (pending: boolean) => void; onEditedChange?: (ids: string[]) => void; layerPriceReady?: boolean; layerUnavailableReason?: string
}) {
  const originals = useRef(new Map<string, { id: string; name: string; preview_url?: string }>())
  const savedMarks = useRef<Record<string, Mark[]>>({})
  const [drafts, setDrafts] = useState<Record<string, Mark[]>>({})
  const [pendingIDs, setPendingIDs] = useState<string[]>([])
  const [paintedIDs, setPaintedIDs] = useState<string[]>([])
  const live = useRef({ active: true, model: profile?.model, ids: references.map((item) => item.id) })
  live.current.model = profile?.model; live.current.ids = references.map((item) => item.id)
  useEffect(() => { live.current.active = true; return () => { live.current.active = false } }, [])
  const hydrated = useRef(false)
  useEffect(() => {
    if (hydrated.current || !profile) return
    hydrated.current = true
    try {
      previous.current.model = profile.model
      const stored: any = persistedMarks ?? (persistenceKey ? JSON.parse(sessionStorage.getItem(persistenceKey) ?? 'null') : null)
      if (stored && typeof stored === 'object' && stored.model === profile?.model) {
        const activeIDs = references.map((item) => item.id)
        originals.current = new Map((stored.originals ?? []).filter(([id]: [string]) => activeIDs.includes(id)))
        savedMarks.current = Object.fromEntries(Object.entries(stored.saved ?? {}).filter(([id]) => activeIDs.includes(id))) as Record<string, Mark[]>
        const restoredDrafts = Object.fromEntries(Object.entries(stored.drafts ?? {}).filter(([id]) => activeIDs.includes(id))) as Record<string, Mark[]>
        const restoredPending = (stored.pending ?? []).filter((id: string) => activeIDs.includes(id))
        const restoredPainted = (stored.painted ?? []).filter((id: string) => activeIDs.includes(id))
        persist({ drafts: restoredDrafts, pending: restoredPending, painted: restoredPainted })
        setDrafts(restoredDrafts); setPendingIDs(restoredPending); setPaintedIDs(restoredPainted)
      }
    } catch { /* An invalid local draft never affects generation parameters. */ }
  }, [profile?.model])
  function persist(next: { drafts?: Record<string, Mark[]>; pending?: string[]; painted?: string[] } = {}) {
    const metadata = { model: profile?.model, originals: [...originals.current], saved: savedMarks.current, drafts, pending: pendingIDs, painted: paintedIDs, ...next }
    onPersistMarks?.(metadata)
    if (persistenceKey) sessionStorage.setItem(persistenceKey, JSON.stringify(metadata))
  }
  const previous = useRef({ ids: references.map((item) => item.id), model: profile?.model })
  const bound = useRef(bindReferenceAnnotations(value.native_options?.annotations, previous.current.ids))
  const ids = references.map((item) => item.id)
  const supported = profile?.adapter === 'seedream_image'
  useEffect(() => {
    if (!profile && !hydrated.current) return
    const changedModel = previous.current.model !== profile?.model
    if (changedModel && originals.current.size) {
      void (async () => {
        try {
          for (const [markedID, original] of originals.current) if (ids.includes(markedID)) await onRestoreReference?.(markedID, original.id)
          originals.current.clear(); savedMarks.current = {}; bound.current = {}
          persist({ drafts: {}, pending: [], painted: [] }); setDrafts({}); setPendingIDs([]); setPaintedIDs([])
        } catch { onPendingChange?.(true) }
      })()
      previous.current = { ids, model: profile?.model }
      return
    }
    if (changedModel || !supported) {
      originals.current.clear(); savedMarks.current = {}
      bound.current = {}; persist({ drafts: {}, pending: [], painted: [] }); setDrafts({}); setPendingIDs([]); setPaintedIDs([]); onEditingReferenceChange?.('')
    } else if (JSON.stringify(previous.current.ids) === JSON.stringify(ids)) {
      bound.current = bindReferenceAnnotations(value.native_options?.annotations, ids)
    } else {
      const annotations = remapReferenceAnnotations(bound.current, ids)
      if (!mappingManaged) onChange({ ...userNativeImageFields(value, profile, ids.length), native_options: { ...(profile?.negative_prompt ? { negative_prompt: value.native_options?.negative_prompt } : {}), annotations } })
      const nextDrafts = Object.fromEntries(Object.entries(drafts).filter(([id]) => ids.includes(id)))
      const nextPending = pendingIDs.filter((id) => ids.includes(id))
      const nextPainted = paintedIDs.filter((id) => ids.includes(id))
      originals.current = new Map([...originals.current].filter(([id]) => ids.includes(id)))
      savedMarks.current = Object.fromEntries(Object.entries(savedMarks.current).filter(([id]) => ids.includes(id)))
      persist({ drafts: nextDrafts, pending: nextPending, painted: nextPainted })
      setDrafts(nextDrafts); setPendingIDs(nextPending); setPaintedIDs(nextPainted)
    }
    previous.current = { ids, model: profile?.model }
  }, [JSON.stringify(ids), profile?.model, supported, JSON.stringify(value.native_options?.annotations)])
  useEffect(() => { onPendingChange?.(pendingIDs.length > 0) }, [pendingIDs, onPendingChange])
  useEffect(() => { onEditedChange?.(ids.filter((id) => paintedIDs.includes(id) || bound.current[id]?.length)) }, [JSON.stringify(ids), paintedIDs, JSON.stringify(value.native_options?.annotations), onEditedChange])
  const asset = supported ? references.find((item) => item.id === editingReferenceID) : undefined
  const initial: Mark[] = asset ? drafts[asset.id] ?? savedMarks.current[asset.id] ?? (bound.current[asset.id] ?? []).map((annotation) => annotation.point ? { kind: 'point', points: [annotation.point] } : { kind: 'box', points: [[annotation.bbox[0], annotation.bbox[1]], [annotation.bbox[2], annotation.bbox[3]]] }) : []
  return <>
    {profile && (profile.layer_decomposition || profile.operations.includes('layer_decomposition')) ? <div className="native-layer-entry"><button className="u-btn" type="button" disabled={disabled || value.operation !== 'layer_decomposition' && (!profile.layer_decomposition || !profile.operations.includes('layer_decomposition') || references.length !== 1 || !layerPriceReady)} aria-pressed={value.operation === 'layer_decomposition'} onClick={() => { if (value.operation !== 'layer_decomposition' && (!profile.layer_decomposition || !profile.operations.includes('layer_decomposition') || references.length !== 1 || !layerPriceReady)) return; onChange({ ...userNativeImageFields(value, profile, references.length), operation: value.operation === 'layer_decomposition' ? references.length ? 'edit' : 'generate' : 'layer_decomposition' }) }}>{value.operation === 'layer_decomposition' ? '退出图层拆分' : '图层拆分'}</button><small role="status">{nativeLayerUnavailableReason(profile, references.length, layerPriceReady, layerUnavailableReason) || '独立拆层功能，提交前以有效报价为准'}</small></div> : null}
    {pendingIDs.length ? <p className="u-meta" role="status">有未保存的引用标记，请打开对应参考图保存，或取消更改。</p> : null}
    {asset ? <ReferenceMarkModal key={asset.id} asset={originals.current.get(asset.id) ?? asset} initial={initial} accessToken={accessToken} onMediaRefresh={async () => {
      const original = originals.current.get(asset.id) ?? asset
      const url = onRefreshReference ? await onRefreshReference(original.id) : (await userApi.refreshReferenceAssetAccess(original.id)).url
      return userApi.imageAssetUrl(url, accessToken)
    }} onDraft={(marks) => { persist({ drafts: { ...drafts, [asset.id]: marks }, pending: [...new Set([...pendingIDs, asset.id])] }); setDrafts((current) => ({ ...current, [asset.id]: marks })); setPendingIDs((current) => [...new Set([...current, asset.id])]) }} onClose={() => onEditingReferenceChange?.('')} onCancel={() => { const remaining = { ...drafts }; delete remaining[asset.id]; persist({ drafts: remaining, pending: pendingIDs.filter((id) => id !== asset.id) }); setDrafts((current) => { const next = { ...current }; delete next[asset.id]; return next }); setPendingIDs((current) => current.filter((id) => id !== asset.id)); onEditingReferenceChange?.('') }} onSave={async (marks, file) => {
      const saveModel = profile?.model
      const saveIDs = JSON.stringify(ids)
      let targetID = asset.id
      const original = originals.current.get(asset.id) ?? asset
      if (!file && originals.current.has(asset.id)) {
        if (!onRestoreReference) throw new Error('无法恢复原始引用，请重试')
        await onRestoreReference(asset.id, original.id); targetID = original.id
      }
      if (file) {
        if (!onUploadMarkedReference) throw new Error('当前入口暂不支持上传标记图，草稿已保留')
        targetID = await onUploadMarkedReference(file, asset.id)
        if (!targetID) throw new Error('上传尚未完成，草稿已保留')
      }
      if (!live.current.active || live.current.model !== saveModel || (JSON.stringify(live.current.ids) !== saveIDs && JSON.stringify(live.current.ids) !== JSON.stringify(ids.map((id) => id === asset.id ? targetID : id)))) throw new Error('引用上下文已变化，未写入旧标记')
      originals.current.delete(asset.id)
      if (file) originals.current.set(targetID, original)
      delete savedMarks.current[asset.id]; savedMarks.current[targetID] = marks
      const nextIDs = ids.map((id) => id === asset.id ? targetID : id)
      const nextBound = { ...bound.current, [targetID]: markAnnotations(marks) }
      if (targetID !== asset.id) delete nextBound[asset.id]
      bound.current = nextBound; previous.current = { ids: nextIDs, model: profile?.model }
      onChange({ ...userNativeImageFields(value, profile, nextIDs.length), native_options: { ...(profile?.negative_prompt ? { negative_prompt: value.native_options?.negative_prompt } : {}), annotations: remapReferenceAnnotations(nextBound, nextIDs) } })
      const remaining = { ...drafts }; delete remaining[asset.id]
      persist({ drafts: remaining, pending: pendingIDs.filter((id) => id !== asset.id), painted: [...paintedIDs.filter((id) => id !== asset.id), ...(file ? [targetID] : [])] })
      setDrafts((current) => { const next = { ...current }; delete next[asset.id]; return next })
      setPendingIDs((current) => current.filter((id) => id !== asset.id))
      setPaintedIDs((current) => [...current.filter((id) => id !== asset.id), ...(file ? [targetID] : [])])
    }} /> : null}
  </>
}
