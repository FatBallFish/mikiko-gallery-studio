import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import {
  ArrowLeft, BoxSelect, CircleStop, ClipboardPaste, Copy, Download, Film, Focus, Image, LayoutTemplate, Link2, MousePointer2,
  Move, Music2, Plus, Redo2, RefreshCw, Save, Search, Sparkles, StickyNote, Trash2, Undo2, ZoomIn, ZoomOut,
} from 'lucide-react'
import type { CanvasRun, CreativeCanvas, MediaAsset } from '../../../../shared/api-types'
import { ApiError } from '../../../../shared/http-client'
import { userApi } from '../../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, useApp } from '../../components'
import { useProjects } from '../../ProjectContext'
import { errorMessage } from '../../useApiResource'
import { computeCanvasBounds, fitCanvasViewport, minimapGeometry, visibleCanvasNodeIDs } from './core/canvasLayout'
import { selectCanvasNodesInRect } from './core/canvasState'
import type { CanvasDocument, CanvasEdge, CanvasNode, CanvasNodeType, CanvasViewport } from './core/types'
import { CanvasAssetDrawer } from './CanvasAssetDrawer'
import { CanvasNodeSearch } from './CanvasNodeSearch'
import { createCanvasDraftWriter, decideCanvasDraftRecovery, readCanvasDraft, removeCanvasDraft } from './persistence/canvasDraftPersistence'
import { createCanvasRemoteSaveScheduler } from './persistence/canvasRemoteSave'
import { createCanvasStore } from './store/canvasStore'
import './canvas.css'

type Props = { canvasID: string; onBack: () => void }
type DragState = { startX: number; startY: number; selectedIDs: string[]; delta: { x: number; y: number } }
type SelectionState = { start: { x: number; y: number }; current: { x: number; y: number } }
type PanState = { startX: number; startY: number; viewport: CanvasViewport }
type NodeEstimate = { points: string; detail?: Record<string, unknown>; documentSignature: string }

const nodeLabels: Record<CanvasNodeType, string> = {
  prompt: '提示词', image: '图片资产', video: '视频资产', audio: '音频资产', image_generation: '图片生成', video_generation: '视频生成', note: '便签',
}
const activeRunStatuses = new Set(['submitting', 'queued', 'running', 'saving'])

export function CanvasEditorPage({ canvasID, onBack }: Props) {
  const app = useApp()
  const projects = useProjects()
  const [canvas, setCanvas] = useState<CreativeCanvas | null>(null)
  const [store, setStore] = useState<ReturnType<typeof createCanvasStore> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [runs, setRuns] = useState<CanvasRun[]>([])
  const [nodeEstimates, setNodeEstimates] = useState<Record<string, NodeEstimate>>({})
  const [busyNodeID, setBusyNodeID] = useState('')
  const [showAssets, setShowAssets] = useState(false)
  const [showSearch, setShowSearch] = useState(false)
  const [connectSource, setConnectSource] = useState('')
  const [conflict, setConflict] = useState<{ remote: CreativeCanvas; local: CanvasDocument } | null>(null)
  const [readOnly, setReadOnly] = useState(() => window.matchMedia('(max-width: 767px) and (orientation: portrait)').matches)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const [viewportSize, setViewportSize] = useState({ width: 1, height: 1 })
  const [drag, setDrag] = useState<DragState | null>(null)
  const [selection, setSelection] = useState<SelectionState | null>(null)
  const [pan, setPan] = useState<PanState | null>(null)
  const draftWriterRef = useRef(createCanvasDraftWriter())
  const remoteSaveRef = useRef<ReturnType<typeof createCanvasRemoteSaveScheduler> | null>(null)
  const state = useStore(store ?? emptyCanvasStore, (value) => value)

  const loadCanvas = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const refreshedRuns = await userApi.listCanvasRuns(canvasID, true)
      const remote = await userApi.getCanvas(canvasID)
      const local = await readCanvasDraft(String(app.profile?.id ?? ''), canvasID)
      let document = toLocalDocument(remote.document)
      let recoveredDraft = false
      if (local) {
        const same = JSON.stringify(local.document) === JSON.stringify(document)
        const decision = decideCanvasDraftRecovery(local, remote.revision, same)
        if (decision === 'recover_local') { document = local.document; recoveredDraft = true }
        if (decision === 'conflict') setConflict({ remote, local: local.document })
        if (decision === 'discard_local') await removeCanvasDraft(String(app.profile?.id ?? ''), canvasID)
      }
      setCanvas(remote)
      setStore(createCanvasStore(document, remote.revision, { recoveredDraft }))
      setRuns(refreshedRuns)
      setNodeEstimates({})
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setLoading(false)
    }
  }, [app.profile?.id, canvasID])

  useEffect(() => { void loadCanvas() }, [loadCanvas])
  useEffect(() => {
    const query = window.matchMedia('(max-width: 767px) and (orientation: portrait)')
    const update = () => setReadOnly(query.matches)
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])
  useEffect(() => {
    const element = viewportRef.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) => setViewportSize({ width: entry.contentRect.width, height: entry.contentRect.height }))
    observer.observe(element)
    return () => observer.disconnect()
  }, [store])
  useEffect(() => {
    if (!store || !canvas || !app.profile?.id) return
    const unsubscribe = store.subscribe((next) => {
      if (!next.command.dirty) return
      draftWriterRef.current.schedule({ schema_version: 1, user_id: String(app.profile!.id), canvas_id: canvasID, base_revision: next.command.revision, saved_at: new Date().toISOString(), document: next.command.present })
    })
    const flush = () => { if (document.visibilityState === 'hidden') void draftWriterRef.current.flush() }
    document.addEventListener('visibilitychange', flush)
    return () => { unsubscribe(); document.removeEventListener('visibilitychange', flush); void draftWriterRef.current.flush() }
  }, [app.profile?.id, canvas, canvasID, store])
  useEffect(() => {
    if (!store || readOnly) return
    function keydown(event: KeyboardEvent) {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement || (event.target as HTMLElement)?.isContentEditable) return
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z') {
        event.preventDefault()
        if (event.shiftKey) store!.getState().redo(); else store!.getState().undo()
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') { event.preventDefault(); void flushDocument() }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'f') { event.preventDefault(); setShowSearch(true) }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'c') { event.preventDefault(); store!.getState().copySelected() }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'v') { event.preventDefault(); store!.getState().pasteClipboard() }
      if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); store!.getState().deleteSelected() }
    }
    window.addEventListener('keydown', keydown)
    return () => window.removeEventListener('keydown', keydown)
  })

  const saveDocument = useCallback(async () => {
    if (!store) return false
    const command = store.getState().command
    if (!command.dirty) return true
    setSaving(true)
    setSaveError('')
    try {
      const saved = await userApi.saveCanvasDocument(canvasID, command.revision, toWireDocument(command.present))
      setCanvas(saved)
      store.getState().acknowledgeSave(command.present, saved.revision)
      if (app.profile?.id && !store.getState().command.dirty) await removeCanvasDraft(String(app.profile.id), canvasID)
      setConflict(null)
      return true
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 409) {
        const remote = await userApi.getCanvas(canvasID)
        setConflict({ remote, local: store.getState().command.present })
        setSaveError('保存冲突')
      } else {
        setSaveError('保存失败')
        app.notify('error', errorMessage(caught))
      }
      return false
    } finally { setSaving(false) }
  }, [app, app.profile?.id, canvasID, store])

  const flushDocument = useCallback(async () => {
    const scheduler = remoteSaveRef.current
    if (scheduler) {
      scheduler.schedule()
      await scheduler.flush()
      return !store?.getState().command.dirty
    }
    return saveDocument()
  }, [saveDocument, store])

  useEffect(() => {
    if (!store) return undefined
    const remoteSave = createCanvasRemoteSaveScheduler(async () => saveDocument())
    remoteSaveRef.current = remoteSave
    const unsubscribe = store.subscribe((next) => {
      if (!next.command.dirty) return
      setSaveError('')
      remoteSave.schedule()
    })
    if (store.getState().command.dirty) remoteSave.schedule()
    const flushWhenHidden = () => { if (document.visibilityState === 'hidden') void remoteSave.flush() }
    document.addEventListener('visibilitychange', flushWhenHidden)
    return () => {
      unsubscribe()
      document.removeEventListener('visibilitychange', flushWhenHidden)
      if (remoteSaveRef.current === remoteSave) remoteSaveRef.current = null
      void remoteSave.flush()
    }
  }, [saveDocument, store])

  useEffect(() => {
    if (!canvas || !runs.some((run) => activeRunStatuses.has(run.status))) return undefined
    let alive = true
    const timer = window.setInterval(() => {
      void userApi.listCanvasRuns(canvasID, true).then((items) => {
        if (!alive) return
        const attachedRevision = Math.max(0, ...items.map((run) => run.attached_revision ?? 0))
        if (attachedRevision > canvas.revision) void loadCanvas()
        else setRuns(items)
      }).catch(() => undefined)
    }, 3000)
    return () => { alive = false; window.clearInterval(timer) }
  }, [canvas, canvasID, loadCanvas, runs])

  if (loading) return <LoadingState label="正在打开创意画布..." />
  if (error || !canvas || !store) return <ErrorState message={error || '画布不存在'} onRetry={() => void loadCanvas()} />

  const documentState = state.command.present
  const documentSignature = JSON.stringify(documentState)
  const unplacedRuns = runs.filter((run) => run.status === 'unplaced')
  const transientNodes = drag ? documentState.nodes.map((node) => drag.selectedIDs.includes(node.id) ? { ...node, position: { x: node.position.x + drag.delta.x, y: node.position.y + drag.delta.y } } : node) : documentState.nodes
  const nodeByID = new Map(transientNodes.map((node) => [node.id, node]))
  const selectedSet = new Set(state.selectedIDs)
  const visibleNodeIDs = visibleCanvasNodeIDs(transientNodes, documentState.viewport, viewportSize, 180, [
    ...state.selectedIDs, connectSource, ...runs.filter((run) => activeRunStatuses.has(run.status)).map((run) => run.node_id),
  ].filter(Boolean))
  const minimap = minimapGeometry(transientNodes, documentState.viewport, viewportSize, { width: 220, height: 140 })

  function worldPoint(clientX: number, clientY: number) {
    const rect = viewportRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return { x: (clientX - rect.left - documentState.viewport.x) / documentState.viewport.zoom, y: (clientY - rect.top - documentState.viewport.y) / documentState.viewport.zoom }
  }
  function fitNodes(nodes = transientNodes) {
    store!.getState().setViewport(fitCanvasViewport(computeCanvasBounds(nodes), viewportSize, 64))
  }
  function addNode(type: CanvasNodeType) {
    const center = worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0))
    const id = `${type}-${crypto.randomUUID().slice(0, 8)}`
    const size = type === 'audio' ? { width: 280, height: 140 } : type.includes('generation') ? { width: 320, height: 230 } : { width: 260, height: 180 }
    store!.getState().addNode({ id, type, position: { x: center.x - size.width / 2, y: center.y - size.height / 2 }, size, payload: defaultNodePayload(type) })
  }
  function addAsset(asset: MediaAsset) {
    addAssetNode(store!, asset, worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0)))
    setShowAssets(false)
  }
  async function estimateNode(node: CanvasNode) {
    setBusyNodeID(node.id)
    try {
      if (!await flushDocument()) return
      const estimate = await userApi.estimateCanvasNode(canvas!.id, node.id)
      if (node.type === 'video_generation' && typeof estimate.detail?.quote_token === 'string') {
        store!.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: { ...(asObject(current.payload?.draft)), quote_token: estimate.detail!.quote_token } } }))
        if (!await flushDocument()) return
      }
      setNodeEstimates((current) => ({ ...current, [node.id]: { ...estimate, documentSignature: JSON.stringify(store!.getState().command.present) } }))
    } catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusyNodeID('') }
  }
  async function generateNode(node: CanvasNode) {
    const estimate = nodeEstimates[node.id]
    if (!estimate || estimate.documentSignature !== JSON.stringify(store!.getState().command.present)) {
      setNodeEstimates((current) => { const next = { ...current }; delete next[node.id]; return next })
      app.notify('error', '画布内容已变化，请重新估价')
      return
    }
    setBusyNodeID(node.id)
    try {
      if (!await flushDocument()) return
      const run = await userApi.generateCanvasNode(canvas!.id, node.id)
      setRuns((items) => [run, ...items.filter((item) => item.id !== run.id)])
      setNodeEstimates((current) => { const next = { ...current }; delete next[node.id]; return next })
      app.notify('success', `已提交${node.type === 'video_generation' ? '视频' : '图片'}生成任务`)
    } catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusyNodeID('') }
  }
  async function attachRun(run: CanvasRun) {
    setBusyNodeID(run.node_id)
    try {
      const recoveryPosition = run.status === 'unplaced' ? {
        x: (viewportSize.width / 2 - documentState.viewport.x) / documentState.viewport.zoom,
        y: (viewportSize.height / 2 - documentState.viewport.y) / documentState.viewport.zoom,
      } : undefined
      await userApi.attachCanvasRun(canvas!.id, run.id, recoveryPosition)
      await loadCanvas()
      app.notify('success', '生成结果已恢复到画布')
    } catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusyNodeID('') }
  }
  function connectTo(node: CanvasNode) {
    if (!connectSource) { setConnectSource(node.id); return }
    if (connectSource === node.id) { setConnectSource(''); return }
    const source = nodeByID.get(connectSource)
    if (!source) return
    const role = suggestedRole(source.type, node.type, documentState.edges, node.id)
    if (!role) { app.notify('error', '这两个节点不能连接'); return }
    try {
      store!.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: source.id, target: node.id, input_role: role })
      setConnectSource('')
    } catch (caught) { app.notify('error', errorMessage(caught)) }
  }

  return <main className="canvas-editor" data-canvas-editor data-readonly={readOnly}>
    <header className="canvas-editor-header">
      <button type="button" title="返回画布列表" onClick={onBack}><ArrowLeft size={18} /></button>
      <div><span>{projects.projects.find((project) => project.id === canvas.project_id)?.name ?? '项目'}</span><strong>{canvas.name}</strong></div>
      <span className="canvas-save-state" data-error={Boolean(saveError)}>{saving ? '保存中' : saveError || (state.command.dirty ? '等待保存' : `已保存 · r${state.command.revision}`)}</span>
      <div className="canvas-header-actions">
        <button type="button" title="搜索节点" onClick={() => setShowSearch(true)}><Search size={17} /></button>
        <button type="button" title="刷新" onClick={() => void loadCanvas()}><RefreshCw size={17} /></button>
        {!readOnly ? <button type="button" title="保存" disabled={saving || !state.command.dirty} onClick={() => void flushDocument()}><Save size={17} /></button> : null}
      </div>
    </header>
    {readOnly ? <div className="canvas-readonly-banner">手机仅支持查看，平板横屏或桌面端可进行完整编辑。</div> : null}
    {unplacedRuns.length ? <div className="canvas-unplaced-result" data-canvas-no-zoom>
      <span>{unplacedRuns.length} 组生成结果待归位</span>
      {!readOnly ? <button type="button" disabled={busyNodeID === unplacedRuns[0].node_id} onClick={() => void attachRun(unplacedRuns[0])}><RefreshCw size={15} />恢复到当前视图</button> : null}
    </div> : null}
    {!readOnly ? <nav className="canvas-toolbox" aria-label="画布工具" data-canvas-no-zoom>
      <button type="button" aria-pressed={state.mode === 'select'} title="选择" onClick={() => store.getState().setMode('select')}><MousePointer2 size={18} /></button>
      <button type="button" aria-pressed={state.mode === 'pan'} title="平移" onClick={() => store.getState().setMode('pan')}><Move size={18} /></button>
      <button type="button" aria-pressed={state.mode === 'connect'} title="连接节点" onClick={() => { store.getState().setMode('connect'); setConnectSource('') }}><Link2 size={18} /></button>
      <i />
      <button type="button" title="添加提示词" onClick={() => addNode('prompt')}><Plus size={13} /><Sparkles size={17} /></button>
      <button type="button" title="添加图片生成" onClick={() => addNode('image_generation')}><Plus size={13} /><Image size={17} /></button>
      <button type="button" title="添加视频生成" onClick={() => addNode('video_generation')}><Plus size={13} /><Film size={17} /></button>
      <button type="button" title="添加便签" onClick={() => addNode('note')}><StickyNote size={17} /></button>
      <button type="button" title="添加资产" onClick={() => setShowAssets(true)}><LayoutTemplate size={17} /></button>
    </nav> : null}
    <div
      ref={viewportRef}
      className="canvas-viewport"
      data-mode={state.mode}
      onWheel={(event) => {
        if ((event.target as Element).closest('[data-canvas-no-zoom]')) return
        event.preventDefault()
        const rect = event.currentTarget.getBoundingClientRect()
        const x = event.clientX - rect.left
        const y = event.clientY - rect.top
        const worldX = (x - documentState.viewport.x) / documentState.viewport.zoom
        const worldY = (y - documentState.viewport.y) / documentState.viewport.zoom
        const zoom = Math.max(0.05, Math.min(3, documentState.viewport.zoom * Math.exp(-event.deltaY * 0.0015)))
        store.getState().setViewport({ x: x - worldX * zoom, y: y - worldY * zoom, zoom })
      }}
      onPointerDown={(event) => {
        if ((event.target as Element).closest('[data-canvas-node],[data-canvas-no-zoom]')) return
        event.currentTarget.setPointerCapture(event.pointerId)
        if (readOnly || state.mode === 'pan' || event.button === 1) setPan({ startX: event.clientX, startY: event.clientY, viewport: documentState.viewport })
        else {
          const point = worldPoint(event.clientX, event.clientY)
          setSelection({ start: point, current: point })
          if (!event.shiftKey) store.getState().select([])
        }
      }}
      onPointerMove={(event) => {
        if (pan) store.getState().setViewport({ ...pan.viewport, x: pan.viewport.x + event.clientX - pan.startX, y: pan.viewport.y + event.clientY - pan.startY })
        if (selection) setSelection({ ...selection, current: worldPoint(event.clientX, event.clientY) })
      }}
      onPointerUp={(event) => {
        if (selection) {
          const rect = normalizedRect(selection.start, selection.current)
          const hit = selectCanvasNodesInRect(transientNodes, rect)
          store.getState().select(event.shiftKey ? Array.from(new Set([...state.selectedIDs, ...hit])) : hit)
        }
        setSelection(null); setPan(null)
      }}
    >
      <div className="canvas-grid" style={gridStyle(documentState.viewport)} />
      <div className="canvas-world" data-canvas-world style={{ transform: `translate(${documentState.viewport.x}px, ${documentState.viewport.y}px) scale(${documentState.viewport.zoom})` }}>
        <svg className="canvas-edges" aria-hidden="true">
          {documentState.edges.map((edge) => {
            const source = nodeByID.get(edge.source); const target = nodeByID.get(edge.target)
            if (!source || !target) return null
            const start = { x: source.position.x + source.size.width, y: source.position.y + source.size.height / 2 }
            const end = { x: target.position.x, y: target.position.y + target.size.height / 2 }
            const bend = Math.max(60, Math.abs(end.x - start.x) * 0.45)
            return <path key={edge.id} d={`M ${start.x} ${start.y} C ${start.x + bend} ${start.y}, ${end.x - bend} ${end.y}, ${end.x} ${end.y}`} vectorEffect="non-scaling-stroke" />
          })}
        </svg>
        {transientNodes.filter((node) => visibleNodeIDs.has(node.id)).map((node) => <CanvasNodeView key={node.id} node={node} selected={selectedSet.has(node.id)} readOnly={readOnly} connecting={connectSource === node.id} run={runs.find((run) => run.node_id === node.id)} estimate={nodeEstimates[node.id]?.documentSignature === documentSignature ? nodeEstimates[node.id] : undefined} busy={busyNodeID === node.id} onSelect={(event) => {
          if ((event.target as Element).closest('[data-canvas-no-drag]')) return
          if (state.mode === 'connect') { connectTo(node); return }
          const selected = selectedSet.has(node.id) ? state.selectedIDs : event.shiftKey ? [...state.selectedIDs, node.id] : [node.id]
          store.getState().select(selected)
          if (readOnly) return
          setDrag({ startX: event.clientX, startY: event.clientY, selectedIDs: selected, delta: { x: 0, y: 0 } })
          event.currentTarget.setPointerCapture(event.pointerId)
        }} onDrag={(event) => { if (drag) setDrag({ ...drag, delta: { x: (event.clientX - drag.startX) / documentState.viewport.zoom, y: (event.clientY - drag.startY) / documentState.viewport.zoom } }) }} onDragEnd={() => { if (drag) store.getState().moveSelected(drag.delta); setDrag(null) }} onUpdate={(payload) => store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, ...payload } }))} onEstimate={() => void estimateNode(node)} onGenerate={() => void generateNode(node)} onAttach={() => { const run = runs.find((item) => item.node_id === node.id && item.status === 'succeeded'); if (run) void attachRun(run) }} onCancel={() => { const run = runs.find((item) => item.node_id === node.id && activeRunStatuses.has(item.status)); if (run) void userApi.cancelCanvasRun(canvas.id, run.id).then((next) => setRuns((items) => [next, ...items.filter((item) => item.id !== next.id)])) }} />)}
        {selection ? <div className="canvas-selection-box" style={rectStyle(normalizedRect(selection.start, selection.current))} /> : null}
      </div>
      <div className="canvas-zoom-controls" data-canvas-no-zoom>
        <button type="button" title="缩小" onClick={() => store.getState().setViewport({ ...documentState.viewport, zoom: Math.max(0.05, documentState.viewport.zoom / 1.2) })}><ZoomOut size={17} /></button>
        <span>{Math.round(documentState.viewport.zoom * 100)}%</span>
        <button type="button" title="放大" onClick={() => store.getState().setViewport({ ...documentState.viewport, zoom: Math.min(3, documentState.viewport.zoom * 1.2) })}><ZoomIn size={17} /></button>
        <button type="button" title="适应视图" onClick={() => fitNodes()}><Focus size={17} /></button>
      </div>
      {!readOnly ? <div className="canvas-command-controls" data-canvas-no-zoom>
        <button type="button" title="撤销" disabled={!state.command.past.length} onClick={() => store.getState().undo()}><Undo2 size={17} /></button>
        <button type="button" title="重做" disabled={!state.command.future.length} onClick={() => store.getState().redo()}><Redo2 size={17} /></button>
        <button type="button" title="复制" disabled={!state.selectedIDs.length} onClick={() => store.getState().copySelected()}><Copy size={17} /></button>
        <button type="button" title="粘贴" disabled={!state.clipboard?.nodes.length} onClick={() => store.getState().pasteClipboard()}><ClipboardPaste size={17} /></button>
        <button type="button" title="删除" disabled={!state.selectedIDs.length} onClick={() => store.getState().deleteSelected()}><Trash2 size={17} /></button>
        <button type="button" title="自动整理选中节点" disabled={state.selectedIDs.length < 2} onClick={() => store.getState().autoLayoutSelected()}><BoxSelect size={17} /></button>
      </div> : null}
      <button className="canvas-minimap" type="button" data-canvas-minimap data-canvas-no-zoom title="点击定位视图" onClick={(event) => {
        const rect = event.currentTarget.getBoundingClientRect()
        const worldX = (event.clientX - rect.left - minimap.offset.x) / minimap.scale + minimap.bounds.x
        const worldY = (event.clientY - rect.top - minimap.offset.y) / minimap.scale + minimap.bounds.y
        store.getState().setViewport({ ...documentState.viewport, x: viewportSize.width / 2 - worldX * documentState.viewport.zoom, y: viewportSize.height / 2 - worldY * documentState.viewport.zoom })
      }}>
        {minimap.nodes.map((item) => <i key={item.id} data-type={item.type} style={{ left: item.x, top: item.y, width: item.width, height: item.height }} />)}
        <b style={{ left: minimap.viewport.x, top: minimap.viewport.y, width: minimap.viewport.width, height: minimap.viewport.height }} />
      </button>
    </div>
    {showSearch ? <CanvasNodeSearch nodes={transientNodes} onClose={() => setShowSearch(false)} onSelect={(node) => { store.getState().select([node.id]); store.getState().setViewport(fitCanvasViewport(computeCanvasBounds([node]), viewportSize, 120)); setShowSearch(false) }} /> : null}
    {showAssets ? <CanvasAssetDrawer projectID={canvas.project_id} onClose={() => setShowAssets(false)} onSelect={addAsset} /> : null}
    {conflict ? <div className="canvas-conflict" role="dialog" aria-modal="true" data-canvas-no-zoom><div><strong>画布已在其他页面更新</strong><p>远端版本 r{conflict.remote.revision}，本地草稿基于 r{state.command.revision}。请选择保留方式，系统不会自动覆盖。</p><footer><Button tone="ghost" onClick={() => { store.getState().replaceRemote(toLocalDocument(conflict.remote.document), conflict.remote.revision); setCanvas(conflict.remote); setConflict(null) }}>使用远端版本</Button><Button onClick={() => void userApi.createCanvas({ project_id: canvas.project_id, name: `${canvas.name} 本地副本`, document: toWireDocument(conflict.local) }).then((copy) => { setConflict(null); app.notify('success', `已创建副本：${copy.name}`) })}>复制本地版本</Button></footer></div></div> : null}
  </main>
}

function CanvasNodeView({ node, selected, readOnly, connecting, run, estimate, busy, onSelect, onDrag, onDragEnd, onUpdate, onEstimate, onGenerate, onAttach, onCancel }: {
  node: CanvasNode; selected: boolean; readOnly: boolean; connecting: boolean; run?: CanvasRun; estimate?: NodeEstimate; busy: boolean
  onSelect: (event: React.PointerEvent<HTMLElement>) => void; onDrag: (event: React.PointerEvent<HTMLElement>) => void; onDragEnd: () => void
  onUpdate: (payload: Record<string, unknown>) => void; onEstimate: () => void; onGenerate: () => void; onAttach: () => void; onCancel: () => void
}) {
  const editable = node.type === 'prompt' || node.type === 'note'
  const generation = node.type === 'image_generation' || node.type === 'video_generation'
  return <article className="canvas-node" data-canvas-node data-type={node.type} data-selected={selected} data-connecting={connecting} style={{ left: node.position.x, top: node.position.y, width: node.size.width, height: node.size.height }} onPointerDown={onSelect} onPointerMove={onDrag} onPointerUp={onDragEnd} onPointerCancel={onDragEnd}>
    <header><span>{nodeTypeIcon(node.type)}</span><strong>{String(node.payload?.title ?? nodeLabels[node.type])}</strong>{run ? <i data-status={run.status}>{run.status}</i> : null}</header>
    <div className="canvas-node-body" data-canvas-no-zoom={editable || generation ? '' : undefined}>
      {editable ? <textarea readOnly={readOnly} defaultValue={String(node.payload?.text ?? '')} placeholder={node.type === 'prompt' ? '描述你想生成的画面' : '记录创作想法'} onBlur={(event) => onUpdate({ text: event.target.value })} /> : null}
      {node.type === 'image' || node.type === 'video' || node.type === 'audio' ? <CanvasMediaNode node={node} /> : null}
      {generation ? <GenerationNodeBody node={node} run={run} estimate={estimate} busy={busy} readOnly={readOnly} onUpdate={onUpdate} onEstimate={onEstimate} onGenerate={onGenerate} onAttach={onAttach} onCancel={onCancel} /> : null}
    </div>
  </article>
}

function GenerationNodeBody({ node, run, estimate, busy, readOnly, onUpdate, onEstimate, onGenerate, onAttach, onCancel }: { node: CanvasNode; run?: CanvasRun; estimate?: NodeEstimate; busy: boolean; readOnly: boolean; onUpdate: (payload: Record<string, unknown>) => void; onEstimate: () => void; onGenerate: () => void; onAttach: () => void; onCancel: () => void }) {
  const draft = asObject(node.payload?.draft)
  const active = run && activeRunStatuses.has(run.status)
  const recoverable = run?.status === 'succeeded' || run?.status === 'unplaced'
  return <div className="canvas-generation-body">
    <label>模型分组<input readOnly={readOnly} defaultValue={String(draft.route_model_code ?? '')} placeholder="由后台能力决定" onBlur={(event) => onUpdate({ draft: { ...draft, route_model_code: event.target.value } })} /></label>
    <label>生成数量<select disabled={readOnly} defaultValue={String(draft.output_count ?? draft.image_count ?? 1)} onChange={(event) => onUpdate({ draft: { ...draft, output_count: Number(event.target.value), image_count: Number(event.target.value) } })}>{[1, 2, 3, 4].map((value) => <option key={value}>{value}</option>)}</select></label>
    {estimate ? <div className="canvas-generation-estimate"><span>预计积分</span><strong>{estimate.points}</strong></div> : null}
    <div className="canvas-generation-actions">{active ? <button type="button" onClick={onCancel}><CircleStop size={15} />取消</button> : recoverable ? <button type="button" onClick={onAttach}><RefreshCw size={15} />恢复结果</button> : !readOnly ? estimate ? <button type="button" disabled={busy} onClick={onGenerate}><Sparkles size={15} />确认生成</button> : <button type="button" disabled={busy} onClick={onEstimate}><Sparkles size={15} />{busy ? '正在估价' : '查看费用'}</button> : null}</div>
    {run?.error_message ? <small>{run.error_message}</small> : null}
  </div>
}

function CanvasMediaNode({ node }: { node: CanvasNode }) {
  const [previewURL, setPreviewURL] = useState('')
  const [accessError, setAccessError] = useState('')
  useEffect(() => {
    if (!node.asset_id) return undefined
    let alive = true
    void userApi.getMediaAssetAccess(node.asset_id, 'preview').then((access) => {
      if (alive) setPreviewURL(access.url)
    }).catch((caught) => { if (alive) setAccessError(errorMessage(caught)) })
    return () => { alive = false }
  }, [node.asset_id])
  const download = async () => {
    if (!node.asset_id) return
    const access = await userApi.getMediaAssetAccess(node.asset_id, 'download')
    const link = document.createElement('a')
    link.href = access.url
    link.download = String(node.payload?.name ?? '')
    link.rel = 'noopener'
    link.click()
  }
  return <div className="canvas-media-preview" data-canvas-no-drag>
    {previewURL && node.type === 'image' ? <img src={previewURL} alt={String(node.payload?.name ?? '图片结果')} loading="lazy" /> : null}
    {previewURL && node.type === 'video' ? <video src={previewURL} controls playsInline preload="metadata" /> : null}
    {previewURL && node.type === 'audio' ? <audio src={previewURL} controls preload="metadata" /> : null}
    {!previewURL ? <div className="canvas-media-placeholder">{nodeTypeIcon(node.type)}<span>{accessError || String(node.payload?.name ?? node.asset_id)}</span></div> : null}
    <button type="button" title="下载原件" aria-label="下载原件" onClick={() => void download()}><Download size={14} />下载</button>
  </div>
}

const emptyCanvasStore = createCanvasStore({ schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [], edges: [] }, 0)
function toLocalDocument(document: CreativeCanvas['document']): CanvasDocument { return document as CanvasDocument }
function toWireDocument(document: CanvasDocument): CreativeCanvas['document'] { return document as CreativeCanvas['document'] }
function asObject(value: unknown) { return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {} }
function defaultNodePayload(type: CanvasNodeType) {
  if (type === 'prompt') return { title: '提示词', text: '' }
  if (type === 'note') return { title: '便签', text: '' }
  if (type === 'image_generation') return { title: '图片生成', draft: { image_count: 1 } }
  if (type === 'video_generation') return { title: '视频生成', draft: { output_count: 1, audio_mode: 'silent' } }
  return {}
}
function addAssetNode(store: ReturnType<typeof createCanvasStore>, asset: MediaAsset, position: { x: number; y: number }) {
  const size = asset.media_type === 'audio' ? { width: 280, height: 140 } : asset.media_type === 'video' ? { width: 320, height: 200 } : { width: 280, height: 220 }
  store.getState().addNode({ id: `${asset.media_type}-${crypto.randomUUID().slice(0, 8)}`, type: asset.media_type, asset_id: asset.id, position: { x: position.x - size.width / 2, y: position.y - size.height / 2 }, size, payload: { name: asset.name, mime_type: asset.mime_type } })
}
function suggestedRole(source: CanvasNodeType, target: CanvasNodeType, edges: CanvasEdge[], targetID: string): CanvasEdge['input_role'] | null {
  if (source === 'prompt' && (target === 'image_generation' || target === 'video_generation')) return 'prompt'
  if (source === 'image' && target === 'image_generation') return 'reference'
  if (source === 'image' && target === 'video_generation') return edges.some((edge) => edge.target === targetID && edge.input_role === 'first_frame') ? 'last_frame' : 'first_frame'
  if (source === 'image_generation' && target === 'image') return 'result'
  if (source === 'video_generation' && target === 'video') return 'result'
  return null
}
function nodeTypeIcon(type: CanvasNodeType) {
  if (type === 'image' || type === 'image_generation') return <Image size={16} />
  if (type === 'video' || type === 'video_generation') return <Film size={16} />
  if (type === 'audio') return <Music2 size={16} />
  if (type === 'note') return <StickyNote size={16} />
  return <Sparkles size={16} />
}
function normalizedRect(start: { x: number; y: number }, end: { x: number; y: number }) { return { x: Math.min(start.x, end.x), y: Math.min(start.y, end.y), width: Math.abs(end.x - start.x), height: Math.abs(end.y - start.y) } }
function rectStyle(rect: { x: number; y: number; width: number; height: number }) { return { left: rect.x, top: rect.y, width: rect.width, height: rect.height } }
function gridStyle(viewport: CanvasViewport) { const size = 32 * viewport.zoom; return { backgroundSize: `${size}px ${size}px`, backgroundPosition: `${viewport.x % size}px ${viewport.y % size}px` } }
