import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import {
  ArrowLeft, BoxSelect, CircleStop, ClipboardPaste, Copy, Download, Film, Focus, Image, ImagePlus, LayoutTemplate, Link2, MousePointer2,
  Move, Music2, Plus, Redo2, RefreshCw, Save, Search, Sparkles, StickyNote, Trash2, Undo2, Upload, ZoomIn, ZoomOut,
} from 'lucide-react'
import type { Capability, CanvasRun, CreativeCanvas, MediaAsset, ReferenceAsset, VideoCapability } from '../../../../shared/api-types'
import { ApiError } from '../../../../shared/http-client'
import { userApi } from '../../../../shared/user-api'
import { normalizeCanvasDocument } from '../../../../shared/canvas-document'
import { Button, EmptyState, ErrorState, LoadingState, useApp } from '../../components'
import { useProjects } from '../../ProjectContext'
import { errorMessage } from '../../useApiResource'
import { userHashForRoute } from '../../routeState'
import { MediaPreviewDialog } from '../media/MediaPreviewDialog'
import { mediaCreationActions } from '../media/mediaExperience'
import { promptVariableNames } from '../../pages/promptTemplateEditorModel'
import { PromptTemplateEditor } from '../../pages/PromptTemplateEditor'
import { PromptVariableForm } from '../../pages/PromptVariableForm'
import { parsePromptTemplate } from '../../pages/promptTemplateParser'
import { computeCanvasBounds, fitCanvasViewport, minimapGeometry, nextCanvasNodePosition, visibleCanvasNodeIDs } from './core/canvasLayout'
import {
  canvasGenerationEstimateSignature, canvasImageDraftForTask, canvasImageParameterErrors, canvasImageTaskType, canvasNodeMinimumSize, canvasPromptResourceCandidates, compatibleCanvasTargets, inspectCanvasConnection,
  canvasImageSizeDraftPatch, prepareCanvasEstimate, rejectCanvasEstimate, resolveCanvasEstimate, selectCanvasNodesInRect, startCanvasEstimate,
  type CanvasEstimateState, type CanvasPromptResourceCandidate,
} from './core/canvasState'
import type { CanvasDocument, CanvasEdge, CanvasNode, CanvasNodeType, CanvasViewport } from './core/types'
import { CanvasAssetDrawer } from './CanvasAssetDrawer'
import { CanvasNodeSearch } from './CanvasNodeSearch'
import { createCanvasDraftWriter, decideCanvasDraftRecovery, readCanvasDraft, removeCanvasDraft } from './persistence/canvasDraftPersistence'
import { createCanvasRemoteSaveScheduler } from './persistence/canvasRemoteSave'
import { createCanvasStore } from './store/canvasStore'
import { MEDIA_UPLOAD_COMPLETED_EVENT, QUEUE_MEDIA_UPLOAD_EVENT, type MediaUploadCompletedDetail, type QueueMediaUploadDetail } from '../media/UploadTray'
import { mediaTypeForFile } from '../media/uploadManager'
import { normalizeWorkspaceImageCount, workspaceTaskImageSafetyLimit } from '../../pages/workspaceViewModel'
import './canvas.css'

type Props = { canvasID: string; onBack: () => void }
type DragState = { startX: number; startY: number; selectedIDs: string[]; delta: { x: number; y: number } }
type ResizeState = { pointerID: number; nodeID: string; startX: number; startY: number; startSize: { width: number; height: number }; size: { width: number; height: number } }
type SelectionState = { start: { x: number; y: number }; current: { x: number; y: number } }
type PanState = { startX: number; startY: number; viewport: CanvasViewport }
type PinchState = { distance: number; center: { x: number; y: number }; viewport: CanvasViewport }
type ConnectionDraft = { pointerID: number; sourceID: string; point: { x: number; y: number }; targetID: string; error: string | null }
type NodeMenuState = { point: { x: number; y: number }; sourceID?: string; options: Array<{ type: CanvasNodeType; role?: CanvasEdge['input_role'] }> }
type GenerationInputSummary = {
  prompts: number
  images: number
  promptNodes: CanvasNode[]
  selectedPromptID: string
  referenceBindings: Array<{ name: string; assetName?: string; assetID?: string }>
  errors: string[]
}

const nodeLabels: Record<CanvasNodeType, string> = {
  prompt: '提示词', image: '图片框', video: '视频资产', audio: '音频资产', image_generation: '图片生成', video_generation: '视频生成', note: '便签',
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
  const [nodeEstimates, setNodeEstimates] = useState<Record<string, CanvasEstimateState>>({})
  const [imageCapability, setImageCapability] = useState<Capability | null>(null)
  const [videoCapability, setVideoCapability] = useState<VideoCapability | null>(null)
  const [previewAsset, setPreviewAsset] = useState<MediaAsset | null>(null)
  const [busyNodeID, setBusyNodeID] = useState('')
  const [showAssets, setShowAssets] = useState(false)
  const [assetTargetNodeID, setAssetTargetNodeID] = useState('')
  const [showSearch, setShowSearch] = useState(false)
  const [connectSource, setConnectSource] = useState('')
  const [connectionDraft, setConnectionDraft] = useState<ConnectionDraft | null>(null)
  const [nodeMenu, setNodeMenu] = useState<NodeMenuState | null>(null)
  const [conflict, setConflict] = useState<{ remote: CreativeCanvas; local: CanvasDocument } | null>(null)
  const [readOnly, setReadOnly] = useState(() => window.matchMedia('(max-width: 767px) and (orientation: portrait)').matches)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const imageUploadInputRef = useRef<HTMLInputElement | null>(null)
  const imageUploadTargetRef = useRef('')
  const [viewportSize, setViewportSize] = useState({ width: 1, height: 1 })
  const [drag, setDrag] = useState<DragState | null>(null)
  const [resize, setResize] = useState<ResizeState | null>(null)
  const [selection, setSelection] = useState<SelectionState | null>(null)
  const [pan, setPan] = useState<PanState | null>(null)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const [estimateRetryVersion, setEstimateRetryVersion] = useState(0)
  const activePointersRef = useRef(new Map<number, { x: number; y: number }>())
  const pinchRef = useRef<PinchState | null>(null)
  const longPressRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const draftWriterRef = useRef(createCanvasDraftWriter())
  const remoteSaveRef = useRef<ReturnType<typeof createCanvasRemoteSaveScheduler> | null>(null)
  const nodeEstimatesRef = useRef<Record<string, CanvasEstimateState>>({})
  const estimateSequenceRef = useRef(0)
  const estimateRetryNodesRef = useRef(new Set<string>())
  const estimateTimersRef = useRef(new Map<string, number>())
  const state = useStore(store ?? emptyCanvasStore, (value) => value)
  const documentState = state.command.present

  const updateNodeEstimate = useCallback((nodeID: string, updater: (current: CanvasEstimateState | undefined) => CanvasEstimateState | undefined) => {
    setNodeEstimates((current) => {
      const value = updater(current[nodeID])
      const next = { ...current }
      if (value) next[nodeID] = value
      else delete next[nodeID]
      nodeEstimatesRef.current = next
      return next
    })
  }, [])

  useEffect(() => {
    if (!store) return undefined
    const complete = (event: Event) => {
      const detail = (event as CustomEvent<MediaUploadCompletedDetail>).detail
      if (!detail?.target || detail.target.canvasID !== canvasID || detail.mediaType !== 'image') return
      const current = store.getState().command.present.nodes.find((node) => node.id === detail.target?.nodeID)
      if (!current || current.type !== 'image') return
      store.getState().updateNode(current.id, (node) => fillImageFrame(node, detail.asset))
    }
    window.addEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete)
    return () => window.removeEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete)
  }, [canvasID, store])

  const loadCanvas = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [refreshedRuns, remote, imageOptions, videoOptions] = await Promise.all([
        userApi.listCanvasRuns(canvasID, true), userApi.getCanvas(canvasID), userApi.getCapabilities(), userApi.getVideoCapabilities(),
      ])
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
      setImageCapability(imageOptions)
      setVideoCapability(videoOptions)
      nodeEstimatesRef.current = {}
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
    const visualViewport = window.visualViewport
    if (!visualViewport) return undefined
    const update = () => setKeyboardOpen(visualViewport.height < window.innerHeight * 0.78)
    visualViewport.addEventListener('resize', update)
    return () => visualViewport.removeEventListener('resize', update)
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
    const unboundImage = command.present.nodes.find((node) => node.type === 'image' && !node.asset_id && !command.present.edges.some((edge) => edge.target === node.id && edge.input_role === 'result'))
    if (unboundImage) {
      setSaveError('请选择图片或连接生成输出')
      return false
    }
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
    if (readOnly || !store || !imageCapability) return
    for (const node of documentState.nodes) {
      if (node.type !== 'image_generation') continue
      const draft = asObject(node.payload?.draft)
      const next = canvasImageDraftForTask(draft, imageCapability, canvasImageTaskType(documentState, node.id))
      if (JSON.stringify(next) === JSON.stringify(draft)) continue
      store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: next } }))
    }
  }, [documentState, imageCapability, readOnly, store])

  useEffect(() => {
    if (!store || !canvas || !imageCapability) return undefined
    const activeNodeIDs = new Set(runs.filter((run) => activeRunStatuses.has(run.status)).map((run) => run.node_id))
    const imageNodes = documentState.nodes.filter((item) => item.type === 'image_generation')
    const imageNodeIDs = new Set(imageNodes.map((node) => node.id))
    estimateTimersRef.current.forEach((timer, nodeID) => {
      if (!imageNodeIDs.has(nodeID)) {
        window.clearTimeout(timer)
        estimateTimersRef.current.delete(nodeID)
      }
    })
    for (const node of imageNodes) {
      const signature = canvasGenerationEstimateSignature(documentState, node.id)
      const errors = [...generationInputSummary(node, documentState).errors, ...canvasImageParameterErrors(asObject(node.payload?.draft), imageCapability, canvasImageTaskType(documentState, node.id), workspaceTaskImageSafetyLimit)]
      const eligible = errors.length === 0 && !activeNodeIDs.has(node.id)
      const current = nodeEstimatesRef.current[node.id]
      const forced = estimateRetryNodesRef.current.delete(node.id)
      if (!eligible) {
        const timer = estimateTimersRef.current.get(node.id)
        if (timer !== undefined) window.clearTimeout(timer)
        estimateTimersRef.current.delete(node.id)
        if (current) updateNodeEstimate(node.id, () => undefined)
        continue
      }
      if (!forced && current?.signature === signature) continue
      const previousTimer = estimateTimersRef.current.get(node.id)
      if (previousTimer !== undefined) window.clearTimeout(previousTimer)
      const requestID = ++estimateSequenceRef.current
      updateNodeEstimate(node.id, (value) => prepareCanvasEstimate(forced ? undefined : value, signature, true, requestID))
      const timer = window.setTimeout(() => {
        estimateTimersRef.current.delete(node.id)
        updateNodeEstimate(node.id, (value) => startCanvasEstimate(value, signature, requestID))
        void (async () => {
          if (!await flushDocument()) {
            updateNodeEstimate(node.id, (value) => rejectCanvasEstimate(value, signature, requestID, '画布尚未保存，暂时无法估价'))
            return
          }
          const latestSignature = canvasGenerationEstimateSignature(store.getState().command.present, node.id)
          if (latestSignature !== signature) return
          try {
            const estimate = await userApi.estimateCanvasNode(canvas.id, node.id)
            if (canvasGenerationEstimateSignature(store.getState().command.present, node.id) !== signature) return
            updateNodeEstimate(node.id, (value) => resolveCanvasEstimate(value, signature, requestID, estimate))
          } catch (caught) {
            updateNodeEstimate(node.id, (value) => rejectCanvasEstimate(value, signature, requestID, errorMessage(caught)))
          }
        })()
      }, 300)
      estimateTimersRef.current.set(node.id, timer)
    }
    return undefined
  }, [canvas, documentState, estimateRetryVersion, flushDocument, imageCapability, runs, store, updateNodeEstimate])

  useEffect(() => () => {
    estimateTimersRef.current.forEach((timer) => window.clearTimeout(timer))
    estimateTimersRef.current.clear()
  }, [])

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

  const unplacedRuns = runs.filter((run) => run.status === 'unplaced')
  const movedNodes = drag ? documentState.nodes.map((node) => drag.selectedIDs.includes(node.id) ? { ...node, position: { x: node.position.x + drag.delta.x, y: node.position.y + drag.delta.y } } : node) : documentState.nodes
  const transientNodes = resize ? movedNodes.map((node) => node.id === resize.nodeID ? { ...node, size: resize.size } : node) : movedNodes
  const nodeByID = new Map(transientNodes.map((node) => [node.id, node]))
  const selectedSet = new Set(state.selectedIDs)
  const selectedEdgeSet = new Set(state.selectedEdgeIDs)
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
  function addNode(type: CanvasNodeType, at?: { x: number; y: number }) {
    const center = at ?? worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0))
    const id = `${type}-${crypto.randomUUID().slice(0, 8)}`
    const size = type === 'audio' ? { width: 280, height: 140 } : type.includes('generation') ? { width: 320, height: 230 } : { width: 260, height: 180 }
    const position = nextCanvasNodePosition(store!.getState().command.present.nodes, center, size)
    store!.getState().addNode({ id, type, position, size, payload: defaultNodePayload(type, imageCapability, videoCapability) })
    return id
  }
  function addAsset(asset: MediaAsset) {
    if (assetTargetNodeID) {
      if (asset.media_type !== 'image') { app.notify('error', '图片框只能选择图片资产'); return }
      store!.getState().updateNode(assetTargetNodeID, (node) => fillImageFrame(node, asset))
      setAssetTargetNodeID('')
    } else {
      addAssetNode(store!, asset, worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0)))
    }
    setShowAssets(false)
  }
  function chooseImageForFrame(nodeID: string) {
    setAssetTargetNodeID(nodeID)
    setShowAssets(true)
  }
  function uploadImageForFrame(nodeID: string) {
    imageUploadTargetRef.current = nodeID
    imageUploadInputRef.current?.click()
  }
  function queueImageFrameUpload(files: FileList | null) {
    const file = files?.[0]
    const nodeID = imageUploadTargetRef.current
    if (!file || !nodeID) return
    if (mediaTypeForFile(file) !== 'image') {
      app.notify('error', '图片框仅支持 JPG、PNG、WEBP 图片')
      if (imageUploadInputRef.current) imageUploadInputRef.current.value = ''
      return
    }
    const detail: QueueMediaUploadDetail = { files: [file], projectID: canvas!.project_id, target: { canvasID, nodeID } }
    window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail }))
    if (imageUploadInputRef.current) imageUploadInputRef.current.value = ''
  }
  async function estimateNode(node: CanvasNode) {
    const requestID = ++estimateSequenceRef.current
    const signature = canvasGenerationEstimateSignature(store!.getState().command.present, node.id)
    updateNodeEstimate(node.id, () => prepareCanvasEstimate(undefined, signature, true, requestID))
    updateNodeEstimate(node.id, (value) => startCanvasEstimate(value, signature, requestID))
    setBusyNodeID(node.id)
    try {
      if (!await flushDocument()) return
      const estimate = await userApi.estimateCanvasNode(canvas!.id, node.id)
      if (node.type === 'video_generation' && typeof estimate.detail?.quote_token === 'string') {
        store!.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: { ...(asObject(current.payload?.draft)), quote_token: estimate.detail!.quote_token } } }))
        if (!await flushDocument()) return
      }
      const latestSignature = canvasGenerationEstimateSignature(store!.getState().command.present, node.id)
      updateNodeEstimate(node.id, () => resolveCanvasEstimate(prepareCanvasEstimate(undefined, latestSignature, true, requestID), latestSignature, requestID, estimate))
    } catch (caught) {
      updateNodeEstimate(node.id, (value) => rejectCanvasEstimate(value, signature, requestID, errorMessage(caught)))
      app.notify('error', errorMessage(caught))
    } finally { setBusyNodeID('') }
  }
  async function generateNode(node: CanvasNode) {
    const signature = canvasGenerationEstimateSignature(store!.getState().command.present, node.id)
    const estimate = nodeEstimatesRef.current[node.id]
    if (!estimate || estimate.status !== 'ready' || estimate.signature !== signature) {
      if (node.type === 'image_generation') {
        estimateRetryNodesRef.current.add(node.id)
        setEstimateRetryVersion((value) => value + 1)
      }
      app.notify('error', '画布内容已变化，正在重新估价')
      return
    }
    setBusyNodeID(node.id)
    try {
      if (!await flushDocument()) return
      if (canvasGenerationEstimateSignature(store!.getState().command.present, node.id) !== estimate.signature) {
        estimateRetryNodesRef.current.add(node.id)
        setEstimateRetryVersion((value) => value + 1)
        app.notify('error', '画布内容已变化，正在重新估价')
        return
      }
      const run = await userApi.generateCanvasNode(canvas!.id, node.id)
      setRuns((items) => [run, ...items.filter((item) => item.id !== run.id)])
      updateNodeEstimate(node.id, () => undefined)
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
    } catch (caught) { app.notify('error', caught instanceof Error && caught.message === 'output_slot_occupied' ? '已有图片不能作为新的生成输出，请使用空图片框' : errorMessage(caught)) }
  }
  function connectionCandidate(sourceID: string, targetID: string) {
    const source = nodeByID.get(sourceID)
    const target = nodeByID.get(targetID)
    if (!source || !target || sourceID === targetID) return { edge: null, error: 'illegal_connection' }
    const role = suggestedRole(source.type, target.type, documentState.edges, target.id)
    if (!role) return { edge: null, error: 'illegal_connection' }
    const edge: CanvasEdge = { id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: sourceID, target: targetID, input_role: role }
    return { edge, error: inspectCanvasConnection(documentState, edge) }
  }
  function finishConnection(draft: ConnectionDraft, targetID: string) {
    const candidate = connectionCandidate(draft.sourceID, targetID)
    if (candidate.edge && !candidate.error) {
      store!.getState().connect(candidate.edge)
      setConnectionDraft(null)
      setConnectSource('')
      return
    }
    if (candidate.error === 'cycle') app.notify('error', '当前生成关系不能形成循环')
    else if (candidate.error === 'input_role_conflict') app.notify('error', '首帧和尾帧均已设置，请先移除或替换现有连接')
    else if (candidate.error === 'output_slot_occupied') app.notify('error', '已有图片不能作为新的生成输出，请使用空图片框')
    else app.notify('error', '这两个节点不能连接')
    setConnectionDraft(null)
  }
  function openNodeMenu(clientX: number, clientY: number, sourceID?: string) {
    const point = worldPoint(clientX, clientY)
    const options = sourceID ? compatibleCanvasTargets(documentState, sourceID) : [
      { type: 'prompt' as const }, { type: 'image' as const }, { type: 'image_generation' as const }, { type: 'video_generation' as const }, { type: 'note' as const },
    ]
    if (!options.length) return
    setNodeMenu({ point, sourceID, options })
  }
  function chooseNodeMenuOption(option: { type: CanvasNodeType; role?: CanvasEdge['input_role'] }) {
    if (!nodeMenu) return
    const nodeID = addNode(option.type, nodeMenu.point)
    if (nodeMenu.sourceID && option.role) {
      store!.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: nodeMenu.sourceID, target: nodeID, input_role: option.role })
    }
    setNodeMenu(null)
    setConnectionDraft(null)
  }
  function addGenerationFromMedia(node: CanvasNode, type: 'image_generation' | 'video_generation') {
    const generationID = addNode(type, { x: node.position.x + node.size.width + 120, y: node.position.y + node.size.height / 2 })
    const role: CanvasEdge['input_role'] = type === 'image_generation' ? 'reference' : 'first_frame'
    store!.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: node.id, target: generationID, input_role: role })
  }

  return <main className="canvas-editor" data-canvas-editor data-readonly={readOnly} data-canvas-keyboard-open={keyboardOpen || undefined}>
    <input ref={imageUploadInputRef} hidden type="file" accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp" onChange={(event) => queueImageFrameUpload(event.target.files)} />
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
      <button type="button" title="添加图片框" onClick={() => addNode('image')}><Plus size={13} /><ImagePlus size={17} /></button>
      <button type="button" title="添加图片生成" onClick={() => addNode('image_generation')}><Plus size={13} /><Image size={17} /></button>
      <button type="button" title="添加视频生成" onClick={() => addNode('video_generation')}><Plus size={13} /><Film size={17} /></button>
      <button type="button" title="添加便签" onClick={() => addNode('note')}><StickyNote size={17} /></button>
      <button type="button" title="添加资产" onClick={() => setShowAssets(true)}><LayoutTemplate size={17} /></button>
    </nav> : null}
    <div
      ref={viewportRef}
      className="canvas-viewport"
      data-mode={state.mode}
      onDoubleClick={(event) => {
        if (readOnly || (event.target as Element).closest('[data-canvas-node],[data-canvas-no-zoom]')) return
        openNodeMenu(event.clientX, event.clientY)
      }}
      onContextMenu={(event) => {
        if (readOnly || (event.target as Element).closest('[data-canvas-node],[data-canvas-no-zoom]')) return
        event.preventDefault()
        openNodeMenu(event.clientX, event.clientY)
      }}
      onDragOver={(event) => {
        if (event.dataTransfer.types.includes('application/x-canvas-asset')) {
          event.preventDefault()
          event.dataTransfer.dropEffect = 'copy'
        }
      }}
      onDrop={(event) => {
        const raw = event.dataTransfer.getData('application/x-canvas-asset')
        if (!raw || readOnly) return
        event.preventDefault()
        try { addAssetNode(store, JSON.parse(raw) as MediaAsset, worldPoint(event.clientX, event.clientY)); setShowAssets(false) } catch { app.notify('error', '资产数据无效，请重新拖入') }
      }}
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
        if (event.pointerType === 'touch') {
          activePointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY })
          if (activePointersRef.current.size === 2) {
            const [first, second] = Array.from(activePointersRef.current.values())
            pinchRef.current = { distance: pointerDistance(first, second), center: pointerCenter(first, second), viewport: documentState.viewport }
            if (longPressRef.current) clearTimeout(longPressRef.current)
            longPressRef.current = null
            setSelection(null)
            setPan(null)
            event.currentTarget.setPointerCapture(event.pointerId)
            return
          } else if (!readOnly) {
            longPressRef.current = setTimeout(() => openNodeMenu(event.clientX, event.clientY), 560)
          }
        }
        event.currentTarget.setPointerCapture(event.pointerId)
        if (readOnly || state.mode === 'pan' || event.button === 1) setPan({ startX: event.clientX, startY: event.clientY, viewport: documentState.viewport })
        else {
          const point = worldPoint(event.clientX, event.clientY)
          setSelection({ start: point, current: point })
          if (!event.shiftKey) store.getState().select([])
        }
      }}
      onPointerMove={(event) => {
        if (event.pointerType === 'touch' && activePointersRef.current.has(event.pointerId)) {
          activePointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY })
          if (longPressRef.current) { clearTimeout(longPressRef.current); longPressRef.current = null }
          const pinch = pinchRef.current
          if (pinch && activePointersRef.current.size >= 2) {
            const [first, second] = Array.from(activePointersRef.current.values())
            const center = pointerCenter(first, second)
            const rect = event.currentTarget.getBoundingClientRect()
            const anchorX = pinch.center.x - rect.left
            const anchorY = pinch.center.y - rect.top
            const worldX = (anchorX - pinch.viewport.x) / pinch.viewport.zoom
            const worldY = (anchorY - pinch.viewport.y) / pinch.viewport.zoom
            const zoom = Math.max(0.05, Math.min(3, pinch.viewport.zoom * pointerDistance(first, second) / Math.max(1, pinch.distance)))
            store.getState().setViewport({ x: center.x - rect.left - worldX * zoom, y: center.y - rect.top - worldY * zoom, zoom })
            return
          }
        }
        if (pan) store.getState().setViewport({ ...pan.viewport, x: pan.viewport.x + event.clientX - pan.startX, y: pan.viewport.y + event.clientY - pan.startY })
        if (selection) setSelection({ ...selection, current: worldPoint(event.clientX, event.clientY) })
        if (connectionDraft) {
          const target = (document.elementFromPoint(event.clientX, event.clientY) as Element | null)?.closest<HTMLElement>('[data-canvas-node]')?.dataset.nodeId ?? ''
          const candidate = target ? connectionCandidate(connectionDraft.sourceID, target) : { error: null }
          setConnectionDraft({ ...connectionDraft, point: worldPoint(event.clientX, event.clientY), targetID: target, error: candidate.error })
        }
      }}
      onPointerUp={(event) => {
        activePointersRef.current.delete(event.pointerId)
        if (activePointersRef.current.size < 2) pinchRef.current = null
        if (longPressRef.current) { clearTimeout(longPressRef.current); longPressRef.current = null }
        if (connectionDraft) {
          if (event.currentTarget.hasPointerCapture(connectionDraft.pointerID)) event.currentTarget.releasePointerCapture(connectionDraft.pointerID)
          if (connectionDraft.targetID) finishConnection(connectionDraft, connectionDraft.targetID)
          else {
            setNodeMenu({ point: worldPoint(event.clientX, event.clientY), sourceID: connectionDraft.sourceID, options: compatibleCanvasTargets(documentState, connectionDraft.sourceID) })
            setConnectionDraft(null)
          }
          return
        }
        if (selection) {
          const rect = normalizedRect(selection.start, selection.current)
          const hit = selectCanvasNodesInRect(transientNodes, rect)
          store.getState().select(event.shiftKey ? Array.from(new Set([...state.selectedIDs, ...hit])) : hit)
        }
        setSelection(null); setPan(null)
      }}
      onPointerCancel={(event) => {
        activePointersRef.current.delete(event.pointerId)
        pinchRef.current = null
        if (longPressRef.current) clearTimeout(longPressRef.current)
        longPressRef.current = null
        setSelection(null); setPan(null); setConnectionDraft(null)
      }}
    >
      <div className="canvas-grid" style={gridStyle(documentState.viewport)} />
      <div className="canvas-world" data-canvas-world style={{ transform: `translate(${documentState.viewport.x}px, ${documentState.viewport.y}px) scale(${documentState.viewport.zoom})` }}>
        <svg className="canvas-edges" aria-label="画布连接">
          {documentState.edges.map((edge) => {
            const source = nodeByID.get(edge.source); const target = nodeByID.get(edge.target)
            if (!source || !target) return null
            const start = { x: source.position.x + source.size.width, y: source.position.y + source.size.height / 2 }
            const end = { x: target.position.x, y: target.position.y + target.size.height / 2 }
            const bend = Math.max(60, Math.abs(end.x - start.x) * 0.45)
            const path = `M ${start.x} ${start.y} C ${start.x + bend} ${start.y}, ${end.x - bend} ${end.y}, ${end.x} ${end.y}`
            return <g key={edge.id} data-selected={selectedEdgeSet.has(edge.id)}>
              <path className="canvas-edge-visible" d={path} vectorEffect="non-scaling-stroke" />
              {!readOnly ? <path className="canvas-edge-hit" data-canvas-edge-hit d={path} vectorEffect="non-scaling-stroke" onPointerDown={(event) => { event.stopPropagation(); store.getState().selectEdges([edge.id]) }} /> : null}
            </g>
          })}
          {connectionDraft && nodeByID.get(connectionDraft.sourceID) ? <path className="canvas-edge-draft" data-error={Boolean(connectionDraft.error)} d={connectionPath(nodeByID.get(connectionDraft.sourceID)!, connectionDraft.point)} vectorEffect="non-scaling-stroke" /> : null}
        </svg>
        {transientNodes.filter((node) => visibleNodeIDs.has(node.id)).map((node) => {
          const targetCandidate = connectionDraft?.sourceID && connectionDraft.sourceID !== node.id ? connectionCandidate(connectionDraft.sourceID, node.id) : null
          const estimate = nodeEstimates[node.id]
          const currentEstimate = estimate?.signature === canvasGenerationEstimateSignature(documentState, node.id) ? estimate : undefined
          const summary = generationInputSummary(node, documentState)
          const inputSummary = node.type === 'image_generation' && imageCapability
            ? { ...summary, errors: [...summary.errors, ...canvasImageParameterErrors(asObject(node.payload?.draft), imageCapability, canvasImageTaskType(documentState, node.id), workspaceTaskImageSafetyLimit)] }
            : summary
          return <CanvasNodeView key={node.id} node={node} selected={selectedSet.has(node.id)} readOnly={readOnly} connecting={connectSource === node.id} connectValid={Boolean(targetCandidate?.edge && !targetCandidate.error)} connectInvalid={Boolean(targetCandidate?.error)} run={runs.find((run) => run.node_id === node.id)} estimate={currentEstimate} busy={busyNodeID === node.id} imageCapability={imageCapability} videoCapability={videoCapability} balance={app.balance?.available_points ?? '0.00000'} inputSummary={inputSummary} promptResourceCandidates={node.type === 'prompt' ? canvasPromptResourceCandidates(documentState, node.id) : []} onStartConnection={(event) => {
            event.stopPropagation()
            setConnectionDraft({ pointerID: event.pointerId, sourceID: node.id, point: worldPoint(event.clientX, event.clientY), targetID: '', error: null })
            viewportRef.current?.setPointerCapture(event.pointerId)
          }} onFinishConnection={(event) => {
            event.stopPropagation()
            if (connectionDraft) finishConnection(connectionDraft, node.id)
          }} onSelect={(event) => {
          const target = event.target as Element
          const interactive = target.closest('[data-canvas-interactive]')
          const dragHandle = target.closest('[data-canvas-drag-handle]')
          if (interactive && !selectedSet.has(node.id)) store.getState().select([node.id])
          if (interactive || !dragHandle) return
          if (state.mode === 'connect') { connectTo(node); return }
          const selected = selectedSet.has(node.id) ? state.selectedIDs : event.shiftKey ? [...state.selectedIDs, node.id] : [node.id]
          store.getState().select(selected)
          if (readOnly) return
          setDrag({ startX: event.clientX, startY: event.clientY, selectedIDs: selected, delta: { x: 0, y: 0 } })
          event.currentTarget.setPointerCapture(event.pointerId)
        }} onDrag={(event) => { if (drag) setDrag({ ...drag, delta: { x: (event.clientX - drag.startX) / documentState.viewport.zoom, y: (event.clientY - drag.startY) / documentState.viewport.zoom } }) }} onDragEnd={() => { if (drag) store.getState().moveSelected(drag.delta); setDrag(null) }} onResizeStart={(event) => {
          event.stopPropagation()
          store.getState().select([node.id])
          setResize({ pointerID: event.pointerId, nodeID: node.id, startX: event.clientX, startY: event.clientY, startSize: node.size, size: node.size })
          event.currentTarget.setPointerCapture(event.pointerId)
        }} onResize={(event) => {
          if (!resize || resize.nodeID !== node.id || resize.pointerID !== event.pointerId) return
          event.stopPropagation()
          const minimum = canvasNodeMinimumSize(node.type)
          setResize({ ...resize, size: { width: Math.max(minimum.width, resize.startSize.width + (event.clientX - resize.startX) / documentState.viewport.zoom), height: Math.max(minimum.height, resize.startSize.height + (event.clientY - resize.startY) / documentState.viewport.zoom) } })
        }} onResizeEnd={(event) => {
          if (!resize || resize.nodeID !== node.id || resize.pointerID !== event.pointerId) return
          event.stopPropagation()
          store.getState().resizeNode(node.id, resize.size)
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
          setResize(null)
        }} onUpdate={(payload) => store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, ...payload } }))} onEstimate={() => {
          if (node.type === 'image_generation') {
            estimateRetryNodesRef.current.add(node.id)
            setEstimateRetryVersion((value) => value + 1)
          } else void estimateNode(node)
        }} onGenerate={() => void generateNode(node)} onAttach={() => { const run = runs.find((item) => item.node_id === node.id && item.status === 'succeeded'); if (run) void attachRun(run) }} onCancel={() => { const run = runs.find((item) => item.node_id === node.id && activeRunStatuses.has(item.status)); if (run) void userApi.cancelCanvasRun(canvas.id, run.id).then((next) => setRuns((items) => [next, ...items.filter((item) => item.id !== next.id)])) }} onMediaDetail={() => { if (node.asset_id) void userApi.getMediaAsset(node.asset_id).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught))) }} onChooseImage={() => chooseImageForFrame(node.id)} onUploadImage={() => uploadImageForFrame(node.id)} canUpload={app.featureFlags.media_upload} onContinueImage={() => addGenerationFromMedia(node, 'image_generation')} onContinueVideo={() => addGenerationFromMedia(node, 'video_generation')} onReuseVideo={() => { const taskID = String(node.payload?.source_task_id ?? '').trim(); if (taskID) window.location.hash = userHashForRoute('genpic', { media: 'video', taskId: taskID }) }} />
        })}
        {selection ? <div className="canvas-selection-box" style={rectStyle(normalizedRect(selection.start, selection.current))} /> : null}
        {nodeMenu ? <div className="canvas-node-menu" data-canvas-no-zoom style={{ left: nodeMenu.point.x, top: nodeMenu.point.y }} role="menu" aria-label={nodeMenu.sourceID ? '添加兼容节点' : '添加节点'}>
          {nodeMenu.options.map((option) => <button key={`${option.type}-${option.role ?? ''}`} type="button" role="menuitem" onClick={() => chooseNodeMenuOption(option)}>{nodeTypeIcon(option.type)}<span>{nodeLabels[option.type]}</span></button>)}
        </div> : null}
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
        <button type="button" title="删除" disabled={!state.selectedIDs.length && !state.selectedEdgeIDs.length} onClick={() => store.getState().deleteSelected()}><Trash2 size={17} /></button>
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
    {showAssets ? <CanvasAssetDrawer projectID={canvas.project_id} mediaType={assetTargetNodeID ? 'image' : undefined} onClose={() => { setShowAssets(false); setAssetTargetNodeID('') }} onSelect={addAsset} /> : null}
    {previewAsset ? <MediaPreviewDialog asset={previewAsset} projects={projects.projects} creationActions={mediaCreationActions(previewAsset)} onClose={() => setPreviewAsset(null)} onChanged={setPreviewAsset} onDeleted={() => setPreviewAsset(null)} onContinue={(options) => { window.location.hash = userHashForRoute('genpic', options) }} /> : null}
    {conflict ? <div className="canvas-conflict" role="dialog" aria-modal="true" data-canvas-no-zoom><div><strong>画布已在其他页面更新</strong><p>远端版本 r{conflict.remote.revision}，本地草稿基于 r{state.command.revision}。请选择保留方式，系统不会自动覆盖。</p><footer><Button tone="ghost" onClick={() => { store.getState().replaceRemote(toLocalDocument(conflict.remote.document), conflict.remote.revision); setCanvas(conflict.remote); setConflict(null) }}>使用远端版本</Button><Button onClick={() => void userApi.createCanvas({ project_id: canvas.project_id, name: `${canvas.name} 本地副本`, document: toWireDocument(conflict.local) }).then((copy) => { setConflict(null); app.notify('success', `已创建副本：${copy.name}`) })}>复制本地版本</Button></footer></div></div> : null}
  </main>
}

function CanvasNodeView({ node, selected, readOnly, connecting, connectValid, connectInvalid, run, estimate, busy, imageCapability, videoCapability, balance, inputSummary, promptResourceCandidates, canUpload, onSelect, onDrag, onDragEnd, onResizeStart, onResize, onResizeEnd, onStartConnection, onFinishConnection, onUpdate, onEstimate, onGenerate, onAttach, onCancel, onMediaDetail, onChooseImage, onUploadImage, onContinueImage, onContinueVideo, onReuseVideo }: {
  node: CanvasNode; selected: boolean; readOnly: boolean; connecting: boolean; connectValid: boolean; connectInvalid: boolean; run?: CanvasRun; estimate?: CanvasEstimateState; busy: boolean
  imageCapability: Capability | null; videoCapability: VideoCapability | null; balance: string; inputSummary: GenerationInputSummary; promptResourceCandidates: CanvasPromptResourceCandidate[]; canUpload: boolean
  onSelect: (event: React.PointerEvent<HTMLElement>) => void; onDrag: (event: React.PointerEvent<HTMLElement>) => void; onDragEnd: () => void
  onResizeStart: (event: React.PointerEvent<HTMLButtonElement>) => void; onResize: (event: React.PointerEvent<HTMLButtonElement>) => void; onResizeEnd: (event: React.PointerEvent<HTMLButtonElement>) => void
  onStartConnection: (event: React.PointerEvent<HTMLButtonElement>) => void; onFinishConnection: (event: React.PointerEvent<HTMLButtonElement>) => void
  onUpdate: (payload: Record<string, unknown>) => void; onEstimate: () => void; onGenerate: () => void; onAttach: () => void; onCancel: () => void
  onMediaDetail: () => void; onChooseImage: () => void; onUploadImage: () => void; onContinueImage: () => void; onContinueVideo: () => void; onReuseVideo: () => void
}) {
  const editable = node.type === 'prompt' || node.type === 'note'
  const generation = node.type === 'image_generation' || node.type === 'video_generation'
  return <article className="canvas-node" data-canvas-node data-node-id={node.id} data-type={node.type} data-selected={selected} data-connecting={connecting} data-connect-valid={connectValid || undefined} data-connect-invalid={connectInvalid || undefined} style={{ left: node.position.x, top: node.position.y, width: node.size.width, height: node.size.height }} onPointerDown={onSelect} onPointerMove={onDrag} onPointerUp={onDragEnd} onPointerCancel={onDragEnd}>
    {!readOnly ? <button type="button" className="canvas-port canvas-port-target" data-canvas-interactive data-canvas-port="target" title="连接到此节点" aria-label={`连接到${nodeLabels[node.type]}`} onPointerUp={onFinishConnection} /> : null}
    <header data-canvas-drag-handle><span>{nodeTypeIcon(node.type)}</span><strong>{String(node.payload?.title ?? nodeLabels[node.type])}</strong>{run ? <i data-status={run.status}>{run.status}</i> : null}</header>
    <div className="canvas-node-body" data-canvas-interactive data-canvas-no-zoom={editable || generation ? '' : undefined}>
      {node.type === 'prompt' ? <PromptNodeBody node={node} readOnly={readOnly} busy={busy} resourceCandidates={promptResourceCandidates} onUpdate={onUpdate} /> : null}
      {node.type === 'note' ? <textarea readOnly={readOnly} defaultValue={String(node.payload?.text ?? '')} placeholder="记录创作想法" onBlur={(event) => onUpdate({ text: event.target.value })} /> : null}
      {node.type === 'image' || node.type === 'video' || node.type === 'audio' ? <CanvasMediaNode node={node} readOnly={readOnly} canUpload={canUpload} onChooseImage={onChooseImage} onUploadImage={onUploadImage} onDetail={onMediaDetail} onContinueImage={onContinueImage} onContinueVideo={onContinueVideo} onReuseVideo={onReuseVideo} /> : null}
      {generation ? <GenerationNodeBody node={node} run={run} estimate={estimate} busy={busy} readOnly={readOnly} imageCapability={imageCapability} videoCapability={videoCapability} balance={balance} inputSummary={inputSummary} onUpdate={onUpdate} onEstimate={onEstimate} onGenerate={onGenerate} onAttach={onAttach} onCancel={onCancel} /> : null}
    </div>
    {!readOnly ? <button type="button" className="canvas-port canvas-port-source" data-canvas-interactive data-canvas-port="source" title="从此节点连接" aria-label={`从${nodeLabels[node.type]}连接`} onPointerDown={onStartConnection} /> : null}
    {!readOnly && selected ? <button type="button" className="canvas-node-resize" data-canvas-interactive data-canvas-resize-handle title="调整节点大小" aria-label={`调整${nodeLabels[node.type]}大小`} onPointerDown={onResizeStart} onPointerMove={onResize} onPointerUp={onResizeEnd} onPointerCancel={onResizeEnd} /> : null}
  </article>
}

function GenerationNodeBody({ node, run, estimate, busy, readOnly, imageCapability, videoCapability, balance, inputSummary, onUpdate, onEstimate, onGenerate, onAttach, onCancel }: { node: CanvasNode; run?: CanvasRun; estimate?: CanvasEstimateState; busy: boolean; readOnly: boolean; imageCapability: Capability | null; videoCapability: VideoCapability | null; balance: string; inputSummary: GenerationInputSummary; onUpdate: (payload: Record<string, unknown>) => void; onEstimate: () => void; onGenerate: () => void; onAttach: () => void; onCancel: () => void }) {
  const draft = asObject(node.payload?.draft)
  const active = run && activeRunStatuses.has(run.status)
  const recoverable = run?.status === 'succeeded' || run?.status === 'unplaced'
  const imageModel = imageCapability?.model_groups.find((group) => group.code === draft.route_model_code) ?? imageCapability?.model_groups[0]
  const videoModel = videoCapability?.model_groups.find((group) => group.code === draft.route_model_code) ?? videoCapability?.model_groups[0]
  const imageTaskType = inputSummary.images > 0 ? 'image_edit' : 'text_to_image'
  const imageOptions = imageModel?.capabilities_by_task_type?.[imageTaskType] ?? imageModel
  const videoTaskType = String(draft.task_type ?? videoModel?.defaults.task_type ?? 'text_to_video') as 'text_to_video' | 'image_to_video' | 'first_last_frame_to_video'
  const videoOptions = videoModel?.options_by_task_type[videoTaskType]
  const models = node.type === 'image_generation' ? imageCapability?.model_groups ?? [] : videoCapability?.model_groups ?? []
  const countMax = Math.max(1, Math.min(10, videoModel?.max_output_count ?? 1))
  const imageCount = normalizeWorkspaceImageCount(Number(draft.output_image_count ?? 1))
  const estimateReady = estimate?.status === 'ready'
  const estimatePending = estimate?.status === 'waiting' || estimate?.status === 'loading'
  const patchDraft = (patch: Record<string, unknown>) => onUpdate({ draft: { ...draft, ...patch } })
  return <div className="canvas-generation-body">
    <label>模型分组<select disabled={readOnly || !models.length} value={String(draft.route_model_code ?? models[0]?.code ?? '')} onChange={(event) => {
      if (node.type === 'image_generation') {
        const model = imageCapability?.model_groups.find((item) => item.code === event.target.value)
        const taskType = imageTaskType
        const options = model?.capabilities_by_task_type?.[taskType] ?? model
        const sizeMode = options?.size_modes?.includes('auto') ? 'auto' : options?.size_modes?.[0] ?? 'auto'
        patchDraft({ route_model_code: event.target.value, task_type: taskType, ...canvasImageSizeDraftPatch(sizeMode, options ?? {}), quality: options?.quality?.[0] ?? '', output_format: options?.output_format?.[0] ?? '', output_image_count: imageCount })
      } else {
        const model = videoCapability?.model_groups.find((item) => item.code === event.target.value)
        const taskType = model?.defaults.task_type ?? model?.task_types[0] ?? 'text_to_video'
        patchDraft({ route_model_code: event.target.value, task_type: taskType, duration_seconds: model?.defaults.duration_seconds, resolution: model?.defaults.resolution, aspect_ratio: model?.defaults.aspect_ratio, audio_mode: model?.defaults.generate_audio ? 'generated' : 'silent', output_count: 1 })
      }
    }}>{models.map((model) => <option key={model.code} value={model.code}>{model.name}</option>)}</select></label>
    {node.type === 'image_generation' ? <>
      <label>尺寸模式<select disabled={readOnly} value={String(draft.size_mode ?? imageOptions?.size_modes?.[0] ?? 'auto')} onChange={(event) => patchDraft(canvasImageSizeDraftPatch(event.target.value, imageOptions ?? {}))}>{(imageOptions?.size_modes ?? ['auto']).map((value) => <option key={value} value={value}>{value === 'auto' ? '自动' : value === 'ratio' ? '按比例' : '按像素'}</option>)}</select></label>
      {draft.size_mode === 'pixel' ? <label>像素尺寸<select disabled={readOnly} value={String(draft.requested_size ?? imageOptions?.pixel_sizes?.[0] ?? '')} onChange={(event) => patchDraft({ requested_size: event.target.value })}>{(imageOptions?.pixel_sizes ?? []).map((value) => <option key={value}>{value}</option>)}</select></label> : draft.size_mode === 'ratio' ? <><label>基础分辨率<select disabled={readOnly} value={String(draft.base_resolution ?? imageOptions?.base_resolution?.[0] ?? '')} onChange={(event) => patchDraft({ base_resolution: event.target.value })}>{(imageOptions?.base_resolution ?? []).map((value) => <option key={value}>{value}</option>)}</select></label><label>比例<select disabled={readOnly} value={String(draft.aspect_ratio ?? imageOptions?.aspect_ratios?.[0] ?? '')} onChange={(event) => patchDraft({ aspect_ratio: event.target.value })}>{(imageOptions?.aspect_ratios ?? []).map((value) => <option key={value}>{value}</option>)}</select></label></> : null}
      <label>质量<select disabled={readOnly} value={String(draft.quality ?? imageOptions?.quality?.[0] ?? '')} onChange={(event) => patchDraft({ quality: event.target.value })}>{(imageOptions?.quality ?? []).map((value) => <option key={value}>{value}</option>)}</select></label>
      <label>输出格式<select disabled={readOnly} value={String(draft.output_format ?? imageOptions?.output_format?.[0] ?? '')} onChange={(event) => patchDraft({ output_format: event.target.value })}>{(imageOptions?.output_format ?? []).map((value) => <option key={value}>{value.toUpperCase()}</option>)}</select></label>
    </> : <>
      <label>生成方式<select disabled={readOnly} value={videoTaskType} onChange={(event) => patchDraft({ task_type: event.target.value })}>{(videoModel?.task_types ?? []).map((value) => <option key={value} value={value}>{value === 'text_to_video' ? '文生视频' : value === 'image_to_video' ? '图生视频' : '首尾帧生视频'}</option>)}</select></label>
      <label>时长<select disabled={readOnly} value={String(draft.duration_seconds ?? videoModel?.defaults.duration_seconds ?? '')} onChange={(event) => patchDraft({ duration_seconds: Number(event.target.value) })}>{(videoOptions?.durations ?? []).map((value) => <option key={value} value={value}>{value} 秒</option>)}</select></label>
      <label>清晰度<select disabled={readOnly} value={String(draft.resolution ?? videoModel?.defaults.resolution ?? '')} onChange={(event) => patchDraft({ resolution: event.target.value })}>{(videoOptions?.resolutions ?? []).map((value) => <option key={value}>{value.toUpperCase()}</option>)}</select></label>
      <label>比例<select disabled={readOnly} value={String(draft.aspect_ratio ?? videoModel?.defaults.aspect_ratio ?? '')} onChange={(event) => patchDraft({ aspect_ratio: event.target.value })}>{(videoOptions?.aspect_ratios ?? []).map((value) => <option key={value}>{value}</option>)}</select></label>
    </>}
    {node.type === 'image_generation'
      ? <label>生成数量<input type="number" min={1} max={workspaceTaskImageSafetyLimit} step={1} disabled={readOnly} value={imageCount} onChange={(event) => patchDraft({ output_image_count: normalizeWorkspaceImageCount(event.target.valueAsNumber) })} /></label>
      : <label>生成数量<select disabled={readOnly} value={String(draft.output_count ?? 1)} onChange={(event) => patchDraft({ output_count: Number(event.target.value) })}>{Array.from({ length: countMax }, (_, index) => index + 1).map((value) => <option key={value}>{value}</option>)}</select></label>}
    {inputSummary.promptNodes.length ? <label>提示词来源<select disabled={readOnly || inputSummary.promptNodes.length === 1} value={inputSummary.selectedPromptID} onChange={(event) => onUpdate({ active_prompt_node_id: event.target.value })}>
      {inputSummary.promptNodes.length > 1 && !inputSummary.selectedPromptID ? <option value="">请选择提示词</option> : null}
      {inputSummary.promptNodes.map((prompt) => <option key={prompt.id} value={prompt.id}>{String(prompt.payload?.title ?? prompt.payload?.text ?? prompt.id).slice(0, 36)}</option>)}
    </select></label> : null}
    {inputSummary.referenceBindings.length ? <div className="canvas-generation-bindings"><strong>资源绑定</strong>{inputSummary.referenceBindings.map((binding) => <span key={binding.name} data-valid={Boolean(binding.assetID)}><b>@{binding.name}</b><i>{binding.assetName ?? '未关联同名资产'}</i></span>)}</div> : null}
    <div className="canvas-generation-inputs">提示词 {inputSummary.prompts} · 图片 {inputSummary.images}</div>
    {inputSummary.errors.length ? <div className="canvas-generation-errors" role="alert">{inputSummary.errors.join('；')}</div> : null}
    <div className="canvas-generation-estimate"><span>{node.type === 'image_generation' || estimate ? '预计积分' : '当前余额'}</span><strong>{estimateReady ? estimate.points : estimatePending ? '计算中' : node.type === 'image_generation' ? '--' : balance}</strong></div>
    {estimate?.status === 'error' ? <small className="canvas-generation-errors">{estimate.error}</small> : null}
    <div className="canvas-generation-actions">{active ? <button type="button" onClick={onCancel}><CircleStop size={15} />取消</button> : recoverable ? <button type="button" onClick={onAttach}><RefreshCw size={15} />恢复结果</button> : !readOnly ? node.type === 'image_generation'
      ? estimateReady ? <button type="button" disabled={busy} onClick={onGenerate}><Sparkles size={15} />确认生成</button> : estimate?.status === 'error' ? <button type="button" disabled={busy} onClick={onEstimate}><RefreshCw size={15} />重新估价</button> : <button type="button" disabled><Sparkles size={15} />{estimatePending ? '正在估价' : '等待参数'}</button>
      : estimateReady ? <button type="button" disabled={busy} onClick={onGenerate}><Sparkles size={15} />确认生成</button> : <button type="button" disabled={busy || estimatePending} onClick={onEstimate}>{estimate?.status === 'error' ? <RefreshCw size={15} /> : <Sparkles size={15} />}{estimate?.status === 'error' ? '重新估价' : busy || estimatePending ? '正在估价' : '查看费用'}</button> : null}</div>
    {run?.error_message ? <small className="canvas-generation-errors">{run.error_message}</small> : null}
  </div>
}

function PromptNodeBody({ node, readOnly, resourceCandidates, onUpdate }: { node: CanvasNode; readOnly: boolean; busy: boolean; resourceCandidates: CanvasPromptResourceCandidate[]; onUpdate: (payload: Record<string, unknown>) => void }) {
  const [text, setText] = useState(String(node.payload?.text ?? ''))
  const [optimizing, setOptimizing] = useState(false)
  const [assets, setAssets] = useState<ReferenceAsset[]>([])
  const variables = asObject(node.payload?.variables)
  const resourceCandidateSignature = resourceCandidates.map((candidate) => `${candidate.assetID}:${candidate.name}:${candidate.duplicateName}:${candidate.mimeType ?? ''}:${candidate.width ?? ''}:${candidate.height ?? ''}`).join('|')
  useEffect(() => setText(String(node.payload?.text ?? '')), [node.payload?.text])
  useEffect(() => {
    let alive = true
    const usable = resourceCandidates.filter((candidate) => !candidate.duplicateName)
    setAssets(usable.map(canvasPromptReferenceAsset))
    void Promise.all(usable.map(async (candidate) => {
      try {
        const access = await userApi.getMediaAssetAccess(candidate.assetID, 'preview')
        return { ...canvasPromptReferenceAsset(candidate), preview_url: access.url, download_url: access.url, preview_expires_at: access.expires_at }
      } catch {
        return canvasPromptReferenceAsset(candidate)
      }
    })).then((next) => { if (alive) setAssets(next) })
    return () => { alive = false }
  }, [resourceCandidateSignature])
  const commitText = (next: string) => {
    setText(next)
    onUpdate({ text: next, variables: Object.fromEntries(promptVariableNames(next).map((name) => [name, String(variables[name] ?? '')])) })
  }
  const optimize = async () => {
    const value = text.trim()
    if (Array.from(value).length < 8) return
    setOptimizing(true)
    try {
      const estimate = await userApi.estimatePromptOptimization(value)
      if (!window.confirm(`优化提示词预计消耗 ${estimate.estimated_points} 积分，是否继续？`)) return
      const result = await userApi.optimizePrompt(value, estimate.quote)
      commitText(result.optimized_prompt)
    } finally { setOptimizing(false) }
  }
  return <div className="canvas-prompt-body">
    <div className="canvas-prompt-actions"><button type="button" disabled={readOnly || optimizing || Array.from(text.trim()).length < 8} onClick={() => void optimize()}>{optimizing ? '优化中' : '优化提示词'}</button></div>
    <PromptTemplateEditor value={text} assets={assets} variables={Object.fromEntries(Object.entries(variables).map(([name, value]) => [name, String(value ?? '')]))} disabled={readOnly} placeholder="描述你想生成的画面" onChange={commitText} />
    <PromptVariableForm template={text} values={Object.fromEntries(Object.entries(variables).map(([name, value]) => [name, String(value ?? '')]))} disabled={readOnly} onChange={(name, value) => onUpdate({ text, variables: { ...variables, [name]: value } })} />
  </div>
}

function canvasPromptReferenceAsset(candidate: CanvasPromptResourceCandidate): ReferenceAsset {
  return { id: candidate.assetID, name: candidate.name, status: 'ready', created_at: '', mime_type: candidate.mimeType, width: candidate.width, height: candidate.height }
}

function CanvasMediaNode({ node, readOnly, canUpload, onChooseImage, onUploadImage, onDetail, onContinueImage, onContinueVideo, onReuseVideo }: { node: CanvasNode; readOnly: boolean; canUpload: boolean; onChooseImage: () => void; onUploadImage: () => void; onDetail: () => void; onContinueImage: () => void; onContinueVideo: () => void; onReuseVideo: () => void }) {
  const [previewURL, setPreviewURL] = useState('')
  const [accessError, setAccessError] = useState('')
  useEffect(() => {
    setPreviewURL('')
    setAccessError('')
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
  const dimensions = Number(node.payload?.width) && Number(node.payload?.height) ? `${node.payload?.width} x ${node.payload?.height}` : ''
  const duration = Number(node.payload?.duration_ms) ? `${(Number(node.payload?.duration_ms) / 1000).toFixed(1)} 秒` : ''
  if (node.type === 'image' && !node.asset_id) return <div className="canvas-image-frame-empty">
    <div className="canvas-media-placeholder"><ImagePlus size={24} /><span>选择图片，或连接为生成输出</span></div>
    {!readOnly ? <div className="canvas-media-actions"><button type="button" onClick={onChooseImage}><Image size={14} />选择资产</button>{canUpload ? <button type="button" onClick={onUploadImage}><Upload size={14} />上传图片</button> : null}</div> : null}
  </div>
  return <div className="canvas-media-node" data-canvas-no-drag>
    <div className="canvas-media-preview">
      {previewURL && node.type === 'image' ? <img src={previewURL} alt={String(node.payload?.name ?? '图片结果')} loading="lazy" /> : null}
      {previewURL && node.type === 'video' ? <video src={previewURL} controls playsInline preload="metadata" /> : null}
      {previewURL && node.type === 'audio' ? <audio src={previewURL} controls preload="metadata" /> : null}
      {!previewURL ? <div className="canvas-media-placeholder">{nodeTypeIcon(node.type)}<span>{accessError || String(node.payload?.name ?? node.asset_id)}</span></div> : null}
    </div>
    <div className="canvas-media-facts"><strong>{String(node.payload?.name ?? node.asset_id ?? nodeLabels[node.type])}</strong><span>{[dimensions || duration, String(node.payload?.source_type ?? '平台资产')].filter(Boolean).join(' · ')}</span></div>
    <div className="canvas-media-actions"><button type="button" onClick={onDetail}>查看详情</button><button type="button" onClick={() => void download()}>下载</button>{node.type === 'image' ? <><button type="button" onClick={onContinueImage}>继续生图</button><button type="button" onClick={onContinueVideo}>生成视频</button></> : null}{node.type === 'video' && node.payload?.source_task_id ? <button type="button" onClick={onReuseVideo}>复用参数</button> : null}</div>
  </div>
}

const emptyCanvasStore = createCanvasStore({ schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [], edges: [] }, 0)
function toLocalDocument(document: CreativeCanvas['document']): CanvasDocument { return normalizeCanvasDocument(document) as CanvasDocument }
function toWireDocument(document: CanvasDocument): CreativeCanvas['document'] { return document as CreativeCanvas['document'] }
function asObject(value: unknown) { return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {} }
function defaultNodePayload(type: CanvasNodeType, imageCapability?: Capability | null, videoCapability?: VideoCapability | null) {
  if (type === 'prompt') return { title: '提示词', text: '' }
  if (type === 'image') return { title: '图片框' }
  if (type === 'note') return { title: '便签', text: '' }
  if (type === 'image_generation') {
    const model = imageCapability?.model_groups.find((item) => item.task_types.includes('text_to_image')) ?? imageCapability?.model_groups[0]
    const taskType = 'text_to_image'
    const options = model?.capabilities_by_task_type?.[taskType] ?? model
    const sizeMode = options?.size_modes?.includes('auto') ? 'auto' : options?.size_modes?.[0] ?? 'auto'
    return { title: '图片生成', draft: { route_model_code: model?.code ?? '', task_type: taskType, ...canvasImageSizeDraftPatch(sizeMode, options ?? {}), quality: options?.quality?.[0] ?? '', output_format: options?.output_format?.[0] ?? '', output_image_count: 1 } }
  }
  if (type === 'video_generation') {
    const model = videoCapability?.model_groups[0]
    return { title: '视频生成', draft: { route_model_code: model?.code ?? '', task_type: model?.defaults.task_type ?? model?.task_types[0] ?? 'text_to_video', duration_seconds: model?.defaults.duration_seconds, resolution: model?.defaults.resolution, aspect_ratio: model?.defaults.aspect_ratio, audio_mode: model?.defaults.generate_audio ? 'generated' : 'silent', output_count: 1 } }
  }
  return {}
}
function fillImageFrame(node: CanvasNode, asset: MediaAsset): CanvasNode {
  return {
    ...node,
    asset_id: asset.id,
    payload: { ...node.payload, name: asset.name, mime_type: asset.mime_type, width: asset.width, height: asset.height, source_type: asset.source_type, source_task_kind: asset.source_task_kind, source_task_id: asset.source_task_id },
  }
}
function addAssetNode(store: ReturnType<typeof createCanvasStore>, asset: MediaAsset, position: { x: number; y: number }) {
  const size = asset.media_type === 'audio' ? { width: 280, height: 140 } : asset.media_type === 'video' ? { width: 320, height: 200 } : { width: 280, height: 220 }
  store.getState().addNode({ id: `${asset.media_type}-${crypto.randomUUID().slice(0, 8)}`, type: asset.media_type, asset_id: asset.id, position: { x: position.x - size.width / 2, y: position.y - size.height / 2 }, size, payload: { name: asset.name, mime_type: asset.mime_type, width: asset.width, height: asset.height, duration_ms: asset.duration_ms, source_type: asset.source_type, source_task_kind: asset.source_task_kind, source_task_id: asset.source_task_id } })
}
function generationInputSummary(node: CanvasNode, document: CanvasDocument): GenerationInputSummary {
  if (node.type !== 'image_generation' && node.type !== 'video_generation') return { prompts: 0, images: 0, promptNodes: [], selectedPromptID: '', referenceBindings: [], errors: [] }
  const incoming = document.edges.filter((edge) => edge.target === node.id)
  const nodeByID = new Map(document.nodes.map((item) => [item.id, item]))
  const promptNodes = incoming.filter((edge) => edge.input_role === 'prompt').map((edge) => nodeByID.get(edge.source)).filter((item): item is CanvasNode => Boolean(item))
  const imageNodes = incoming.filter((edge) => ['reference', 'first_frame', 'last_frame'].includes(edge.input_role)).map((edge) => nodeByID.get(edge.source)).filter((item): item is CanvasNode => Boolean(item?.asset_id))
  const prompts = promptNodes.length
  const images = imageNodes.length
  const errors: string[] = []
  const draft = asObject(node.payload?.draft)
  const configuredPromptID = String(node.payload?.active_prompt_node_id ?? '')
  const selectedPromptID = promptNodes.some((item) => item.id === configuredPromptID) ? configuredPromptID : promptNodes.length === 1 ? promptNodes[0].id : ''
  const selectedPrompt = promptNodes.find((item) => item.id === selectedPromptID)
  const referenceBindings = buildCanvasPromptBindings(String(selectedPrompt?.payload?.text ?? selectedPrompt?.payload?.template ?? selectedPrompt?.payload?.prompt ?? ''), imageNodes)
  if (!prompts && !String(draft.prompt ?? draft.prompt_template ?? '').trim()) errors.push('缺少提示词')
  if (prompts > 1 && !selectedPromptID) errors.push('请选择一条提示词来源')
  const variables = asObject(selectedPrompt?.payload?.variables)
  const missingVariables = selectedPrompt ? promptVariableNames(String(selectedPrompt.payload?.text ?? '')).filter((name) => !String(variables[name] ?? '').trim()) : []
  if (missingVariables.length) errors.push(`请填写变量：${missingVariables.join('、')}`)
  const missingReferences = referenceBindings.filter((binding) => !binding.assetID).map((binding) => binding.name)
  if (missingReferences.length) errors.push(`未关联同名资产：${missingReferences.join('、')}`)
  if (!String(draft.route_model_code ?? draft.abstract_model ?? '').trim()) errors.push('请选择模型分组')
  if (node.type === 'video_generation' && String(draft.task_type ?? '').includes('image') && !images) errors.push('缺少首帧图片')
  return { prompts, images, promptNodes, selectedPromptID, referenceBindings, errors }
}
function buildCanvasPromptBindings(template: string, imageNodes: CanvasNode[]) {
  const parsed = parsePromptTemplate(template)
  const names = parsed.error ? [] : parsed.referenceNames
  const assets = new Map<string, CanvasNode[]>()
  imageNodes.forEach((item) => {
    const name = String(item.payload?.name ?? '').trim()
    if (name) assets.set(name, [...(assets.get(name) ?? []), item])
  })
  return names.map((name) => {
    const matches = assets.get(name) ?? []
    const asset = matches.length === 1 ? matches[0] : undefined
    return { name, assetName: asset ? String(asset.payload?.name ?? '') : matches.length > 1 ? '存在多个同名资产' : undefined, assetID: asset?.asset_id }
  })
}
function suggestedRole(source: CanvasNodeType, target: CanvasNodeType, edges: CanvasEdge[], targetID: string): CanvasEdge['input_role'] | null {
  if (source === 'prompt' && (target === 'image_generation' || target === 'video_generation')) return 'prompt'
  if (source === 'image' && target === 'image_generation') return 'reference'
  if (source === 'image' && target === 'video_generation') return edges.some((edge) => edge.target === targetID && edge.input_role === 'first_frame') ? 'last_frame' : 'first_frame'
  if (source === 'image_generation' && target === 'image') return 'result'
  if (source === 'video_generation' && target === 'video') return 'result'
  return null
}
function connectionPath(source: CanvasNode, end: { x: number; y: number }) {
  const start = { x: source.position.x + source.size.width, y: source.position.y + source.size.height / 2 }
  const bend = Math.max(60, Math.abs(end.x - start.x) * 0.45)
  return `M ${start.x} ${start.y} C ${start.x + bend} ${start.y}, ${end.x - bend} ${end.y}, ${end.x} ${end.y}`
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
function pointerDistance(first: { x: number; y: number }, second: { x: number; y: number }) { return Math.hypot(second.x - first.x, second.y - first.y) }
function pointerCenter(first: { x: number; y: number }, second: { x: number; y: number }) { return { x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 } }
