import { MEDIA_UPLOAD_TERMINATED_EVENT, type MediaUploadTerminatedDetail } from '../media/uploadManager'
import { videoInputAssetReason, videoDraftInvalidReason } from '../creation/videoDraft'
import { nativeImageValidation, type NativeImageRequestFields } from '../../../../shared/native-image'
import { createReferenceReservations } from '../../../../shared/reference-admission'
import { canvasReferenceAdmission, canvasReferenceEntries, canvasReferenceKind, canvasReferencePolicy, type CanvasReferenceKind } from './canvasPageIntegration'
import { compareCanvasReferenceEdges } from './core/canvasState'
import { userNativeImageFields, bindReferenceAnnotations, remapReferenceAnnotations } from '../creation/nativeImageUX'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import {
  ArrowLeft, Library, BoxSelect, ClipboardPaste, Copy, Film, Focus, Group, Image, Keyboard, Link2, MousePointer2,
  Music2, Redo2, RefreshCw, Save, Search, Sparkles, StickyNote, Trash2, Undo2, Ungroup, ZoomIn, ZoomOut,
} from 'lucide-react'
import type { Capability, CanvasRun, CreativeCanvas, MediaAsset, VideoCapability } from '../../../../shared/api-types'
import { ApiError } from '../../../../shared/http-client'
import { userApi } from '../../../../shared/user-api'
import { normalizeCanvasDocument } from '../../../../shared/canvas-document'
import { Button, ErrorState, LoadingState, Modal, useApp } from '../../components'
import { useProjects } from '../../ProjectContext'
import { errorMessage } from '../../useApiResource'
import { userHashForRoute } from '../../routeState'
import { MediaPreviewDialog } from '../media/MediaPreviewDialog'
import { mediaCreationActions } from '../media/mediaExperience'
import { computeCanvasBounds, fitCanvasViewport, minimapGeometry, nextCanvasNodePosition, visibleCanvasNodeIDs } from './core/canvasLayout'
import {
  canvasGenerationEstimateEligible, canvasGenerationEstimateSignature, canvasGroupBounds, canvasGroupMembers, canvasGroupsForNode, canvasImageDraftForTask, canvasImageParameterErrors, canvasImageTaskType, canvasNodeMinimumSize, compatibleCanvasTargets,
  canvasImageSizeDraftPatch, prepareCanvasEstimate, rejectCanvasEstimate, resolveCanvasEstimate, selectCanvasNodesInRect, startCanvasEstimate,
  type CanvasEstimateState,
} from './core/canvasState'
import type { CanvasDocument, CanvasEdge, CanvasNode, CanvasNodeType, CanvasViewport } from './core/types'
import { CanvasAssetDrawer } from './CanvasAssetDrawer'
import { CanvasNodeSearch } from './CanvasNodeSearch'
import { createCanvasDraftWriter, decideCanvasDraftRecovery, readCanvasDraft, removeCanvasDraft } from './persistence/canvasDraftPersistence'
import { createCanvasRemoteSaveScheduler } from './persistence/canvasRemoteSave'
import { canvasRemoteDocumentsEqual, createCanvasRemoteOperationQueue, rebaseCanvasRemoteState } from './persistence/canvasRemoteReconcile'
import { createCanvasStore } from './store/canvasStore'
import { MEDIA_UPLOAD_COMPLETED_EVENT, QUEUE_MEDIA_UPLOAD_EVENT, type MediaUploadCompletedDetail, type QueueMediaUploadDetail } from '../media/UploadTray'
import { mediaTypeForFile } from '../media/uploadManager'
import { workspaceTaskImageSafetyLimit } from '../../pages/workspaceViewModel'
import { CanvasMediaNodeV2 } from './CanvasMediaNodeV2'
import { canvasReferenceMarksModelChanged, normalizeCanvasImageDraft, normalizeCanvasVideoDraft, normalizeCanvasVideoGraphDraft, canvasConsoleParameterErrors } from './canvasMediaV2'
import { CanvasGenerationConsole } from './CanvasGenerationConsole'
import { canvasNodeMediaKind, isCanvasV2MediaNode, isCanvasGenerationNode, canCreateCanvasNode, canCopyCanvasNode } from './core/canvasNodeKinds'
import { canvasMediaSize, resizeCanvasMediaSize } from './core/canvasMediaSizing'
import { settleCanvasReferenceUpload, canvasAssetNode, canvasConsoleBounds, canvasEstimateSufficient, canvasQuoteRemainingMS, canvasReferenceAsset, insertCanvasReference, useCanvasMediaHydration, validateCanvasReference, type CanvasHydratedMedia } from './canvasPageIntegration'
import { canvasConnectionGeometry, canvasHandle, canvasPointerIntent, canvasConnectionRelease, resolveCanvasConnection, type CanvasConnectionDraft } from './core/canvasConnections'
import { createCanvasEstimateCoordinator, type CanvasEstimateTarget } from './core/canvasEstimateCoordinator'
import './canvas.css'

type Props = { canvasID: string; onBack: () => void }
type DragState = { pointerID: number; startX: number; startY: number; selectedIDs: string[]; duplicate: boolean; started: boolean; delta: { x: number; y: number } }
type ResizeState = { pointerID: number; nodeID: string; startX: number; startY: number; startSize: { width: number; height: number }; size: { width: number; height: number } }
type SelectionState = { start: { x: number; y: number }; current: { x: number; y: number } }
type PanState = { startX: number; startY: number; viewport: CanvasViewport }
type PinchState = { distance: number; center: { x: number; y: number }; viewport: CanvasViewport }
type CanvasBackgroundPointer = { pointerID: number; startX: number; startY: number; button: number; shift: boolean; space: boolean; moved: boolean; suppressed: boolean }
type DockDragState = { pointerID: number; startX: number; startOffset: number }
type ConnectionDraft = CanvasConnectionDraft
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
  image_v2: '图片', video_v2: '视频', prompt: '提示词', image: '图片框', video: '视频资产', audio: '音频资产', image_generation: '图片生成', video_generation: '视频生成', note: '便签',
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
  const [showShortcuts, setShowShortcuts] = useState(false)
  const [referencePick, setReferencePick] = useState<{ nodeID: string; role: CanvasEdge['input_role'] } | null>(null)
  const [assetRole, setAssetRole] = useState<CanvasEdge['input_role']>('reference')
  const nativeReferenceIDs = useRef(new Map<string, string[]>())
  const markedSources = useRef(new Map<string, string>())
  const uploadTargetsRef = useRef(new Map<string, { nodeID: string; role: CanvasEdge['input_role']; reservation?: number }>())
  const [reservationVersion, setReservationVersion] = useState(0)
  const referenceReservations = useRef(new Map<string, ReturnType<typeof createReferenceReservations<CanvasReferenceKind>>>())
  function reservationsFor(nodeID: string) {
    let value = referenceReservations.current.get(nodeID)
    if (!value) { value = createReferenceReservations<CanvasReferenceKind>(); referenceReservations.current.set(nodeID, value) }
    return value
  }
  const admissionCapabilities = useRef({ image: imageCapability, video: videoCapability })
  admissionCapabilities.current = { image: imageCapability, video: videoCapability }
  function admissionContext(nodeID: string, exclude?: number) { return { ...admissionCapabilities.current, reserved: reservationsFor(nodeID).pending(exclude) } }
  const [groupConfigID, setGroupConfigID] = useState('')
  const spacePressedRef = useRef(false)
  const [spacePressed, setSpacePressed] = useState(false)
  const [connectionDraft, setConnectionDraft] = useState<ConnectionDraft | null>(null)
  const connectionRef = useRef<ConnectionDraft | null>(null)
  function updateConnection(draft: ConnectionDraft | null) { connectionRef.current = draft; setConnectionDraft(draft) }
  const connectSource = connectionDraft?.sourceID ?? ''
  const [nodeMenu, setNodeMenu] = useState<NodeMenuState | null>(null)
  const [conflict, setConflict] = useState<{ remote: CreativeCanvas; local: CanvasDocument } | null>(null)
  const [readOnly, setReadOnly] = useState(() => window.matchMedia('(max-width: 767px) and (orientation: portrait)').matches)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const imageUploadInputRef = useRef<HTMLInputElement | null>(null)
  const imageUploadTargetRef = useRef('')
  const [viewportSize, setViewportSize] = useState({ width: 1, height: 1 })
  const [consoleHeight, setConsoleHeight] = useState(440)
  const consoleObserverRef = useRef<ResizeObserver | null>(null)
  const observeConsole = useCallback((element: HTMLDivElement | null) => {
    consoleObserverRef.current?.disconnect()
    consoleObserverRef.current = null
    const content = element?.firstElementChild
    if (!content) return
    const measure = () => {
      const height = Math.ceil(content.getBoundingClientRect().height)
      if (height > 0) setConsoleHeight((previous) => previous === height ? previous : height)
    }
    consoleObserverRef.current = new ResizeObserver(measure)
    consoleObserverRef.current.observe(content)
    measure()
  }, [])
  const [drag, setDrag] = useState<DragState | null>(null)
  const dragRef = useRef<DragState | null>(null)
  function updateDrag(next: DragState | null) { dragRef.current = next; setDrag(next) }
  const [resize, setResize] = useState<ResizeState | null>(null)
  const [selection, setSelection] = useState<SelectionState | null>(null)
  const [pan, setPan] = useState<PanState | null>(null)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const [dockOffset, setDockOffset] = useState(0)
  const backgroundPointerRef = useRef<CanvasBackgroundPointer | null>(null)
  const dockRef = useRef<HTMLElement | null>(null)
  const dockDragRef = useRef<DockDragState | null>(null)
  const [estimateRetryVersion, setEstimateRetryVersion] = useState(0)
  const activePointersRef = useRef(new Map<number, { x: number; y: number }>())
  const pinchRef = useRef<PinchState | null>(null)
  const longPressRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const draftWriterRef = useRef(createCanvasDraftWriter())
  const remoteSaveRef = useRef<ReturnType<typeof createCanvasRemoteSaveScheduler> | null>(null)
  const remoteSessionRef = useRef<{
    canvasID: string; store: ReturnType<typeof createCanvasStore>; base: CanvasDocument; revision: number
    enqueue: ReturnType<typeof createCanvasRemoteOperationQueue>; blocked: boolean
  } | null>(null)
  const activeUserRef = useRef(app.profile?.id)
  activeUserRef.current = app.profile?.id
  const activeCanvasIDRef = useRef(canvasID)
  activeCanvasIDRef.current = canvasID
  const loadSequenceRef = useRef(0)
  const notifyRef = useRef(app.notify)
  notifyRef.current = app.notify
  const nodeEstimatesRef = useRef<Record<string, CanvasEstimateState>>({})
  const estimateRetryNodesRef = useRef(new Set<string>())
  const estimateCoordinatorRef = useRef<ReturnType<typeof createCanvasEstimateCoordinator<{ points: string; detail?: Record<string, unknown> }>> | null>(null)
  const state = useStore(store ?? emptyCanvasStore, (value) => value)
  const documentState = state.command.present
  const hydratedMedia = useCanvasMediaHydration(documentState, app.session?.token)

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
      if (!detail?.target || detail.target.canvasID !== canvasID) return
      const target = uploadTargetsRef.current.get(detail.target.nodeID)
      if (!target) return
      uploadTargetsRef.current.delete(detail.target.nodeID)
      try { insertCanvasReference(store, detail.asset, target.nodeID, target.role, admissionContext(target.nodeID, target.reservation)) }
      catch (caught) { app.notify('error', errorMessage(caught)) }
      finally { if (target.reservation !== undefined) reservationsFor(target.nodeID).release(target.reservation); setReservationVersion((value) => value + 1) }
    }
    const terminated = (event: Event) => {
      const detail = (event as CustomEvent<MediaUploadTerminatedDetail>).detail
      if (detail?.target?.canvasID !== canvasID) return
      // Failed uploads may resume; keep the destination and revalidate their eventual success.
      const changed = settleCanvasReferenceUpload(uploadTargetsRef.current, detail.target.nodeID, detail.status, (target) => {
        if (target.reservation !== undefined) reservationsFor(target.nodeID).release(target.reservation)
      })
      if (!changed) return
      setReservationVersion((value) => value + 1)
    }
    window.addEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete)
    window.addEventListener(MEDIA_UPLOAD_TERMINATED_EVENT, terminated)
    return () => { window.removeEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete); window.removeEventListener(MEDIA_UPLOAD_TERMINATED_EVENT, terminated) }
  }, [canvasID, store, imageCapability, videoCapability])

  const loadCanvas = useCallback(async () => {
    const sequence = ++loadSequenceRef.current
    const current = () => sequence === loadSequenceRef.current && activeCanvasIDRef.current === canvasID
    setLoading(true)
    setError('')
    try {
      const [refreshedRuns, imageOptions, videoOptions] = await Promise.all([
        userApi.listCanvasRuns(canvasID, true), userApi.getCapabilities(), userApi.getVideoCapabilities(),
      ])
      // Refresh attaches completed results and advances revision; read only afterwards.
      if (!current()) return
      const remote = await userApi.getCanvas(canvasID)
      const local = await readCanvasDraft(String(app.profile?.id ?? ''), canvasID)
      if (!current()) return
      let document = toLocalDocument(remote.document)
      let recoveredDraft = false
      let draftConflict = false
      setConflict(null)
      if (local) {
        const same = JSON.stringify(canonicalCanvasDocument(local.document)) === JSON.stringify(canonicalCanvasDocument(document))
        const decision = decideCanvasDraftRecovery(local, remote.revision, same)
        if (decision === 'recover_local') { document = local.document; recoveredDraft = true }
        if (decision === 'conflict') { setConflict({ remote, local: local.document }); draftConflict = true }
        if (decision === 'discard_local') await removeCanvasDraft(String(app.profile?.id ?? ''), canvasID)
      }
      if (!current()) return
      const nextStore = createCanvasStore(document, remote.revision, { recoveredDraft })
      remoteSessionRef.current = { canvasID, store: nextStore, base: toLocalDocument(remote.document), revision: remote.revision, enqueue: createCanvasRemoteOperationQueue(), blocked: draftConflict }
      setCanvas(remote)
      setStore(nextStore)
      setRuns(refreshedRuns)
      setImageCapability(imageOptions)
      setVideoCapability(videoOptions)
      nodeEstimatesRef.current = {}
      setNodeEstimates({})
    } catch (caught) {
      if (current()) setError(errorMessage(caught))
    } finally {
      if (current()) setLoading(false)
    }
  }, [app.profile?.id, canvasID])

  useEffect(() => { void loadCanvas(); return () => { ++loadSequenceRef.current } }, [loadCanvas])
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
    const dock = dockRef.current
    const container = dock?.parentElement
    if (!dock || !container) return
    const reposition = () => {
      dockDragRef.current = null
      setDockOffset((offset) => clampDockOffset(offset))
    }
    const observer = new ResizeObserver(reposition)
    observer.observe(container)
    observer.observe(dock)
    return () => observer.disconnect()
  }, [store, readOnly, loading])
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
    function isTypingTarget(target: EventTarget | null) {
      return target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || (target as HTMLElement)?.isContentEditable || (target as HTMLElement)?.closest?.('[data-canvas-no-zoom]')
    }
    function keydown(event: KeyboardEvent) {
      if (document.querySelector('[role="dialog"]')) return
      if (event.code === 'Space' && !isTypingTarget(event.target)) {
        event.preventDefault()
        if (!spacePressedRef.current) { spacePressedRef.current = true; setSpacePressed(true) }
        return
      }
      if (event.key === 'Escape' && !(event.target instanceof Element && event.target.closest('input,textarea,select,[contenteditable="true"],[role="dialog"]'))) {
        event.preventDefault()
        clearCanvasTransientState()
        return
      }
      if (isTypingTarget(event.target)) return
      const state = store!.getState()
      const mod = event.metaKey || event.ctrlKey
      const key = event.key.toLowerCase()
      if (mod && key === 'z') {
        event.preventDefault()
        if (event.shiftKey) state.redo(); else state.undo()
      } else if (mod && key === 's') {
        event.preventDefault(); void flushDocument()
      } else if (mod && key === 'f') {
        event.preventDefault(); setShowSearch(true)
      } else if (mod && key === 'c') {
        event.preventDefault(); if (state.selectedIDs.every((id) => { const node = state.command.present.nodes.find((item) => item.id === id); return node && canCopyCanvasNode(node) })) state.copySelected()
      } else if (mod && key === 'v') {
        event.preventDefault(); state.pasteClipboard()
      } else if (mod && key === 'a') {
        event.preventDefault(); state.select(state.command.present.nodes.map((node) => node.id))
      } else if (mod && key === 'd') {
        event.preventDefault(); if (state.selectedIDs.length && state.selectedIDs.every((id) => { const node = state.command.present.nodes.find((item) => item.id === id); return node && canCopyCanvasNode(node) })) { state.copySelected(); state.pasteClipboard() }
      } else if (mod) {
        return
      } else if (event.key === 'Delete' || event.key === 'Backspace') {
        event.preventDefault(); state.deleteSelected()
      } else if (key === 'v' || key === 'h') {
        state.setMode('select'); updateConnection(null)
      } else if (key === 'l') {
        state.setMode('connect'); updateConnection(null)
      } else if (key === 'g') {
        if (event.shiftKey) state.disbandSelectedGroups(); else if (state.selectedIDs.length >= 2) state.groupSelected()
      } else if (key === '0') {
        fitNodes()
      } else if (key === '?') {
        setShowShortcuts(true)
      } else if (event.key === 'Enter') {
        const selected = state.command.present.nodes.find((node) => node.id === state.selectedIDs[0] && isCanvasGenerationNode(node))
        if (selected) { event.preventDefault(); void generateNode(selected) }
      } else if (event.key.startsWith('Arrow')) {
        if (!state.selectedIDs.length) return
        event.preventDefault()
        const step = event.shiftKey ? 16 : 2
        const delta = event.key === 'ArrowLeft' ? { x: -step, y: 0 } : event.key === 'ArrowRight' ? { x: step, y: 0 } : event.key === 'ArrowUp' ? { x: 0, y: -step } : { x: 0, y: step }
        state.moveSelected(delta)
      }
    }
    function keyup(event: KeyboardEvent) {
      if (event.code === 'Space') { spacePressedRef.current = false; setSpacePressed(false) }
    }
    window.addEventListener('keydown', keydown)
    window.addEventListener('keyup', keyup)
    // Note: no ref reset here — this effect re-registers on every render and
    // resetting would drop a held Space key mid-interaction.
    return () => { window.removeEventListener('keydown', keydown); window.removeEventListener('keyup', keyup) }
  })

  const reconcileRemote = useCallback((remote: CreativeCanvas, session: NonNullable<typeof remoteSessionRef.current>) => {
    if (remoteSessionRef.current !== session || activeCanvasIDRef.current !== session.canvasID) return false
    if (remote.revision < session.revision) return true
    const rebased = rebaseCanvasRemoteState(session.store.getState().command, session.base, toLocalDocument(remote.document), remote.revision)
    if (rebased.conflicts.length) {
      session.blocked = true
      setConflict({ remote, local: session.store.getState().command.present })
      setSaveError('保存冲突')
      return false
    }
    session.base = toLocalDocument(remote.document)
    session.revision = remote.revision
    session.store.setState({ command: rebased.command })
    setCanvas(remote)
    return true
  }, [])

  const saveDocument = useCallback(async () => {
    const session = remoteSessionRef.current
    if (!session || session.store !== store || session.canvasID !== canvasID) return false
    return session.enqueue(async () => {
      const current = () => remoteSessionRef.current === session && activeCanvasIDRef.current === canvasID
      if (!current() || session.blocked) return false
      if (!store.getState().command.dirty) return true
      if (canvasRemoteDocumentsEqual(store.getState().command.present, session.base)) {
        store.getState().acknowledgeSave(store.getState().command.present, session.revision)
        return true
      }
      setSaving(true)
      setSaveError('')
      try {
        // One safe retry may absorb server result attachment, never generation submission.
        for (let attempt = 0; attempt < 2; attempt++) {
          const command = store.getState().command
          try {
            const saved = await userApi.saveCanvasDocument(canvasID, command.revision, toWireDocument(command.present))
            if (!current()) return false
            // Edits made during the request are compared against the submitted snapshot.
            session.base = command.present
            if (!reconcileRemote(saved, session)) return false
            if (app.profile?.id && !store.getState().command.dirty) await removeCanvasDraft(String(app.profile.id), canvasID)
            return true
          } catch (caught) {
            if (!(caught instanceof ApiError) || caught.status !== 409) throw caught
            const remote = await userApi.getCanvas(canvasID)
            if (!current() || !reconcileRemote(remote, session)) return false
            if (!store.getState().command.dirty) return true
            if (attempt === 1) {
              session.blocked = true
              setConflict({ remote, local: store.getState().command.present })
              setSaveError('保存冲突')
            }
          }
        }
        return false
      } catch (caught) {
        if (current()) { setSaveError('保存失败'); notifyRef.current('error', errorMessage(caught)) }
        return false
      } finally { if (current()) setSaving(false) }
    })
  }, [app.profile?.id, canvasID, reconcileRemote, store])

  const saveDocumentRef = useRef(saveDocument)
  saveDocumentRef.current = saveDocument
  const flushDocument = useCallback(async () => {
    const scheduler = remoteSaveRef.current
    if (scheduler) {
      scheduler.schedule()
      await scheduler.flush()
      return !remoteSessionRef.current?.blocked && !store?.getState().command.dirty
    }
    return saveDocumentRef.current()
  }, [store])

  useEffect(() => {
    if (readOnly || !store || !imageCapability) return
    for (const node of documentState.nodes) {
      if (node.type !== 'image_v2' || node.asset_id) continue
      const draft = asObject(node.payload?.draft)
      const normalized = canvasImageDraftForTask(draft, imageCapability, canvasImageTaskType(documentState, node.id))
      const ids = documentState.edges.filter((edge) => edge.target === node.id && edge.input_role === 'reference').sort(compareCanvasReferenceEdges).flatMap((edge) => { const source = documentState.nodes.find((item) => item.id === edge.source); return source?.asset_id ? [source.asset_id] : [] })
      const profile = imageCapability.model_groups.find((item) => item.code === normalized.route_model_code)?.native_image_profile
      const marks = asObject(draft.ui_reference_marks)
      if (canvasReferenceMarksModelChanged(marks.model, profile) && Array.isArray(marks.originals)) {
        for (const [markedID, original] of marks.originals as Array<[string, { id: string }]>) {
          const source = documentState.nodes.find((item) => item.asset_id === markedID && documentState.edges.some((edge) => edge.target === node.id && edge.source === item.id))
          if (source) store.getState().updateNode(source.id, (current) => ({ ...current, asset_id: original.id }))
        }
        store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: { ...normalized, native_options: undefined, ui_reference_marks: undefined } } }))
        continue
      }
      const selectedModel = imageCapability.model_groups.find((item) => item.code === normalized.route_model_code)
      if (!selectedModel || !selectedModel.task_types.includes(normalized.task_type)) continue
      if (draft.operation === 'layer_decomposition' && !profile?.operations.includes('layer_decomposition')) continue
      if (!profile && draft.ui_reference_marks) continue
      const clean = userNativeImageFields(normalized as import('../../../../shared/native-image').NativeImageRequestFields, profile, ids.length)
      const previousIDs = nativeReferenceIDs.current.get(node.id)
      if (previousIDs && JSON.stringify(previousIDs) !== JSON.stringify(ids) && clean.native_options?.annotations) clean.native_options.annotations = remapReferenceAnnotations(bindReferenceAnnotations(clean.native_options.annotations, previousIDs), ids)
      nativeReferenceIDs.current.set(node.id, ids)
      const prunedMarks = marks.model ? {
        ...marks,
        pending: Array.isArray(marks.pending) ? marks.pending.filter((id) => ids.includes(String(id))) : [],
        painted: Array.isArray(marks.painted) ? marks.painted.filter((id) => ids.includes(String(id))) : [],
        drafts: Object.fromEntries(Object.entries(asObject(marks.drafts)).filter(([id]) => ids.includes(id))),
        saved: Object.fromEntries(Object.entries(asObject(marks.saved)).filter(([id]) => ids.includes(id))),
        originals: Array.isArray(marks.originals) ? marks.originals.filter((entry) => Array.isArray(entry) && ids.includes(String(entry[0]))) : [],
      } : normalized.ui_reference_marks
      const next = { ...normalized, ...clean, ...(prunedMarks ? { ui_reference_marks: prunedMarks } : {}) }
      if (JSON.stringify(next) === JSON.stringify(draft)) continue
      store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: next } }))
    }
  }, [documentState, imageCapability, readOnly, store])

  const videoGraphErrorsRef = useRef(new Map<string, string>())
  useEffect(() => {
    if (readOnly || !store || !videoCapability) return
    for (const node of documentState.nodes) {
      if (node.type !== 'video_v2' || node.asset_id) continue
      const { draft, error } = normalizeCanvasVideoGraphDraft(documentState, node.id, videoCapability)
      if (error && videoGraphErrorsRef.current.get(node.id) !== error) app.notify('error', error)
      if (error) videoGraphErrorsRef.current.set(node.id, error)
      else videoGraphErrorsRef.current.delete(node.id)
      if (JSON.stringify(draft) !== JSON.stringify(asObject(node.payload?.draft))) store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft } }))
    }
  }, [documentState, videoCapability, readOnly, store])

  useEffect(() => {
    if (!store) return undefined
    const coordinator = createCanvasEstimateCoordinator<{ points: string; detail?: Record<string, unknown> }>({
      flush: flushDocument,
      canReuse: (estimate) => canvasQuoteRemainingMS(estimate.detail) > 0,
      isCurrent: (nodeID, signature) => activeCanvasIDRef.current === canvasID && canvasGenerationEstimateSignature(store.getState().command.present, nodeID) === signature,
      request: (nodeID, signal) => userApi.estimateCanvasNode(canvasID, nodeID, signal),
      prepare: ({ nodeID, signature, requestID }) => updateNodeEstimate(nodeID, () => prepareCanvasEstimate(undefined, signature, true, requestID)),
      start: ({ nodeID, signature, requestID }) => updateNodeEstimate(nodeID, (value) => startCanvasEstimate(value, signature, requestID)),
      resolve: ({ nodeID, signature, requestID }, result) => updateNodeEstimate(nodeID, (value) => resolveCanvasEstimate(value, signature, requestID, result)),
      reject: ({ nodeID, signature, requestID }, caught) => updateNodeEstimate(nodeID, (value) => rejectCanvasEstimate(value, signature, requestID, errorMessage(caught))),
      clear: (nodeID) => updateNodeEstimate(nodeID, () => undefined),
      commit: async ({ nodeID }, estimate, current) => {
        const node = store.getState().command.present.nodes.find((item) => item.id === nodeID)
        if (node?.type === 'video_v2' && typeof estimate.detail?.quote_token === 'string') {
          const token = estimate.detail.quote_token
          if (asObject(node.payload?.draft).quote_token !== token) {
            store.getState().updateNode(nodeID, (value) => ({ ...value, payload: { ...value.payload, draft: { ...asObject(value.payload?.draft), quote_token: token } } }))
            if (!await flushDocument() || !current()) return false
          }
        }
        return current()
      },
      timing: (timing) => console.debug('[canvas estimate phases]', timing),
    })
    estimateCoordinatorRef.current = coordinator
    return () => {
      coordinator.dispose()
      if (estimateCoordinatorRef.current === coordinator) estimateCoordinatorRef.current = null
    }
  }, [canvasID, flushDocument, store, updateNodeEstimate])

  useEffect(() => {
    const coordinator = estimateCoordinatorRef.current
    if (!coordinator) return
    if (readOnly || !store || !imageCapability || !videoCapability) { coordinator.sync([]); return }
    const activeNodeIDs = new Set(runs.filter((run) => activeRunStatuses.has(run.status)).map((run) => run.node_id))
    const targets: CanvasEstimateTarget[] = []
    for (const node of documentState.nodes.filter(isCanvasGenerationNode)) {
      const signature = canvasGenerationEstimateSignature(documentState, node.id)
      const errors = generationErrors(node, documentState)
      const eligible = errors.length === 0 && canvasGenerationEstimateEligible(documentState, node.id) && !activeNodeIDs.has(node.id)
      if (!eligible) continue
      targets.push({ nodeID: node.id, signature, force: estimateRetryNodesRef.current.delete(node.id) })
    }
    coordinator.sync(targets)
  }, [canvasID, documentState, estimateRetryVersion, flushDocument, imageCapability, videoCapability, readOnly, reservationVersion, runs, store, hydratedMedia])

  useEffect(() => {
    const timers = Object.entries(nodeEstimates).flatMap(([nodeID, estimate]) => {
      if (estimate.status !== 'ready' || !Number.isFinite(canvasQuoteRemainingMS(estimate.detail))) return []
      return [window.setTimeout(() => {
        updateNodeEstimate(nodeID, () => undefined)
        estimateRetryNodesRef.current.add(nodeID)
        setEstimateRetryVersion((value) => value + 1)
      }, Math.max(0, canvasQuoteRemainingMS(estimate.detail)))]
    })
    return () => timers.forEach((timer) => window.clearTimeout(timer))
  }, [nodeEstimates, updateNodeEstimate])

  useEffect(() => {
    if (!store || remoteSessionRef.current?.store !== store || remoteSessionRef.current.canvasID !== canvasID) return undefined
    const remoteSave = createCanvasRemoteSaveScheduler(() => saveDocumentRef.current())
    remoteSaveRef.current = remoteSave
    const unsubscribe = store.subscribe((next) => {
      const session = remoteSessionRef.current
      // replaceRemote is the explicit conflict-dialog acknowledgement.
      if (session?.store === store && session.blocked && !next.command.dirty) {
        session.base = next.command.present
        session.revision = next.command.revision
        session.blocked = false
        setSaveError('')
      }
      if (!next.command.dirty || session?.blocked) return
      setSaveError('')
      remoteSave.schedule()
    })
    if (store.getState().command.dirty) remoteSave.schedule()
    const flushWhenHidden = () => { if (document.visibilityState === 'hidden') void remoteSave.flush() }
    document.addEventListener('visibilitychange', flushWhenHidden)
    return () => {
      unsubscribe()
      document.removeEventListener('visibilitychange', flushWhenHidden)
      remoteSave.cancel()
      if (remoteSaveRef.current === remoteSave) remoteSaveRef.current = null
    }
  }, [canvasID, store])

  const refreshRemoteRuns = useCallback(async () => {
    const session = remoteSessionRef.current
    if (!session || session.canvasID !== canvasID) return
    await session.enqueue(async () => {
      const current = () => remoteSessionRef.current === session && activeCanvasIDRef.current === canvasID
      if (!current() || session.blocked) return
      const items = await userApi.listCanvasRuns(canvasID, true)
      if (!current()) return
      // Fetch after refresh even if no attached_revision is exposed by an older server.
      const remote = await userApi.getCanvas(canvasID)
      if (!current()) return
      reconcileRemote(remote, session)
      setRuns(items)
    })
  }, [canvasID, reconcileRemote])

  const hasActiveRuns = runs.some((run) => activeRunStatuses.has(run.status))
  useEffect(() => {
    if (!store || !hasActiveRuns) return undefined
    let pending = false
    const timer = window.setInterval(() => {
      if (pending) return
      pending = true
      void refreshRemoteRuns().catch(() => undefined).finally(() => { pending = false })
    }, 3000)
    return () => window.clearInterval(timer)
  }, [hasActiveRuns, refreshRemoteRuns, store])

  if (loading) return <LoadingState label="正在打开创意画布..." />
  if (error || !canvas || !store) return <ErrorState message={error || '画布不存在'} onRetry={() => void loadCanvas()} />

  const unplacedRuns = runs.filter((run) => run.status === 'unplaced')
  const movedNodes = drag ? documentState.nodes.map((node) => drag.selectedIDs.includes(node.id) ? { ...node, position: { x: node.position.x + drag.delta.x, y: node.position.y + drag.delta.y } } : node) : documentState.nodes
  const transientNodes = resize ? movedNodes.map((node) => node.id === resize.nodeID ? { ...node, size: resize.size } : node) : movedNodes
  const nodeByID = new Map(transientNodes.map((node) => [node.id, node]))
  const selectedSet = new Set(state.selectedIDs)
  const selectedEdgeSet = new Set(state.selectedEdgeIDs)
  function clearCanvasTransientState() {
    if (connectionRef.current) updateConnection(null)
    if (longPressRef.current) clearTimeout(longPressRef.current)
    longPressRef.current = null
    backgroundPointerRef.current = null
    activePointersRef.current.clear()
    pinchRef.current = null
    setReferencePick(null)
    setNodeMenu(null)
    setGroupConfigID('')
    setSelection(null)
    setPan(null)
    updateDrag(null)
    setResize(null)
    store?.getState().select([])
  }

  function clampDockOffset(offset: number) {
    const dock = dockRef.current
    const container = dock?.parentElement
    const bounds = container?.getBoundingClientRect()
    const dockBounds = dock?.getBoundingClientRect()
    if (!container || !bounds || !dockBounds) return 0
    let left = bounds.left + 12
    let right = bounds.right - 12
    for (const selector of ['.canvas-minimap', '.canvas-zoom-controls']) {
      const control = container.querySelector<HTMLElement>(selector)
      if (!control || !control.getClientRects().length) continue
      const rect = control.getBoundingClientRect()
      if (rect.top >= dockBounds.bottom || rect.bottom <= dockBounds.top) continue
      if (selector === '.canvas-minimap') left = Math.max(left, rect.right + 12)
      else right = Math.min(right, rect.left - 12)
    }
    const centeredLeft = bounds.left + (bounds.width - dockBounds.width) / 2
    const minimum = left - centeredLeft
    const maximum = right - dockBounds.width - centeredLeft
    return minimum <= maximum ? Math.max(minimum, Math.min(maximum, offset)) : 0
  }

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
    if (!canCreateCanvasNode(type)) throw new Error('历史节点仅支持查看')
    const center = at ?? worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0))
    const id = `${type}-${crypto.randomUUID().slice(0, 8)}`
    const size = type === 'audio' ? { width: 360, height: 180 } : type === 'image_v2' ? { width: 420, height: 340 } : type === 'video_v2' ? { width: 440, height: 360 } : type === 'prompt' ? { width: 380, height: 260 } : { width: 340, height: 300 }
    const position = nextCanvasNodePosition(store!.getState().command.present.nodes, center, size)
    const payload: Record<string, unknown> = defaultNodePayload(type, imageCapability, videoCapability)
    if (isCanvasV2MediaNode(type)) {
      const prefix = type === 'image_v2' ? '图片节点' : '视频节点'
      const existing = store!.getState().command.present.nodes.filter((node) => node.type === type)
      const ordinal = existing.reduce((maximum, node) => {
        const match = String(node.payload?.title ?? '').match(new RegExp(`^${prefix}(\\d+)$`))
        return Math.max(maximum, match ? Number(match[1]) : 0)
      }, 0) + 1
      payload.title = `${prefix}${ordinal}`
      payload.name = `node_${id.replace(/[^a-zA-Z0-9_]/g, '_')}`
    }
    store!.getState().addNode({ id, type, position, size, payload })
    return id
  }
  function addAsset(asset: MediaAsset) {
    try {
      if (assetTargetNodeID) insertCanvasReference(store!, asset, assetTargetNodeID, assetRole, admissionContext(assetTargetNodeID))
      else addAssetNode(store!, asset, worldPoint(viewportSize.width / 2 + (viewportRef.current?.getBoundingClientRect().left ?? 0), viewportSize.height / 2 + (viewportRef.current?.getBoundingClientRect().top ?? 0)))
      setAssetTargetNodeID(''); setShowAssets(false)
    } catch (caught) { app.notify('error', errorMessage(caught)) }
  }
  function chooseImageForFrame(nodeID: string, role: CanvasEdge['input_role'] = 'reference') {
    setAssetTargetNodeID(nodeID); setAssetRole(role); setShowAssets(true)
  }
  function uploadImageForFrame(nodeID: string, role: CanvasEdge['input_role'] = 'reference') {
    const token = `reference-${crypto.randomUUID()}`
    uploadTargetsRef.current.set(token, { nodeID, role }); imageUploadTargetRef.current = token
    imageUploadInputRef.current?.click()
  }
  function queueImageFrameUpload(files: FileList | null) {
    const file = files?.[0], token = imageUploadTargetRef.current
    const target = uploadTargetsRef.current.get(token)
    if (!file || !target) return
    const expected = target.role === 'reference_video' ? 'video' : target.role === 'reference_audio' ? 'audio' : 'image'
    if (mediaTypeForFile(file) !== expected) { app.notify('error', '文件类型与参考角色不匹配'); return }
    const document = store!.getState().command.present
    const targetNode = document.nodes.find((node) => node.id === target.nodeID)
    if (targetNode?.type === 'video_v2') {
      const draft = asObject(targetNode.payload?.draft)
      const reason = videoInputAssetReason(videoCapability?.model_groups.find((item) => item.code === draft.route_model_code), draft.task_type as import('../../../../shared/api-types').VideoTaskType, target.role as import('../creation/videoDraft').VideoDraftInputRole, { media_type: expected, mime_type: file.type, name: file.name, file_size_bytes: file.size })
      if (reason) { app.notify('error', reason); return }
    }
    const reservation = reservationsFor(target.nodeID).reserve(canvasReferencePolicy(document, target.nodeID, admissionContext(target.nodeID), true), canvasReferenceEntries(document, target.nodeID), [{ id: token, kind: canvasReferenceKind(target.role) }], (item) => item.kind, (item) => item.id)
    if (!reservation.ok) { app.notify('error', reservation.reason); return }
    uploadTargetsRef.current.set(token, { ...target, reservation: reservation.token })
    setReservationVersion((value) => value + 1)
    const detail: QueueMediaUploadDetail = { files: [file], projectID: canvas!.project_id, target: { canvasID, nodeID: token } }
    window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail }))
    if (imageUploadInputRef.current) imageUploadInputRef.current.value = ''
  }
  function estimateNode(node: CanvasNode) {
    estimateRetryNodesRef.current.add(node.id); setEstimateRetryVersion((value) => value + 1)
  }
  function generationErrors(node: CanvasNode, document: CanvasDocument) {
    const inputs = document.edges.filter((edge) => edge.target === node.id && edge.input_role !== 'prompt').map((edge) => ({ role: edge.input_role, asset_id: document.nodes.find((source) => source.id === edge.source)?.asset_id }))
    const errors = [...generationInputSummary(node, document).errors, ...canvasConsoleParameterErrors({ ...asObject(node.payload?.draft), inputs }, imageCapability, videoCapability, node.type === 'video_v2')]
    if (Object.values(reservationsFor(node.id).pending()).some((count) => count > 0)) errors.push('参考素材正在上传，请等待完成后生成')
    const admission = canvasReferenceAdmission(document, node.id, [], admissionContext(node.id))
    if (!admission.ok) errors.push(admission.reason)
    if (node.type === 'video_v2' && videoCapability) {
      const error = videoDraftInvalidReason({ ...asObject(node.payload?.draft), inputs } as unknown as import('../creation/videoDraft').VideoDraft, videoCapability, Object.fromEntries(Object.entries(hydratedMedia).map(([id, media]) => [id, media.asset])))
      if (error) errors.push(error)
    }
    if (node.type === 'image_v2') {
      const draft = asObject(node.payload?.draft)
      const model = imageCapability?.model_groups.find((item) => item.code === String(draft.route_model_code ?? draft.abstract_model ?? ''))
      if (draft.operation === 'layer_decomposition' && !model?.native_image_profile?.operations.includes('layer_decomposition')) errors.push('当前模型不支持图层拆分，请手动选择支持的模型或生成方式')
      const error = nativeImageValidation(model?.native_image_profile, draft as NativeImageRequestFields, inputs.length)
      if (error) errors.push(error)
    }
    return errors
  }
  async function generateNode(node: CanvasNode) {
    const currentNode = store!.getState().command.present.nodes.find((item) => item.id === node.id)
    if (readOnly || !currentNode || !isCanvasGenerationNode(currentNode) || busyNodeID || runs.some((run) => run.node_id === node.id && activeRunStatuses.has(run.status))) return
    if (Array.isArray(asObject(asObject(currentNode.payload?.draft).ui_reference_marks).pending) && (asObject(asObject(currentNode.payload?.draft).ui_reference_marks).pending as unknown[]).length) { app.notify('error', '请先保存或放弃引用标记'); return }
    const errors = generationErrors(currentNode, store!.getState().command.present)
    if (errors.length) { app.notify('error', errors[0]); return }
    const signature = canvasGenerationEstimateSignature(store!.getState().command.present, node.id)
    const estimate = nodeEstimatesRef.current[node.id]
    if (!estimate || estimate.status !== 'ready' || estimate.signature !== signature || canvasQuoteRemainingMS(estimate.detail) <= 0) {
      estimateRetryNodesRef.current.add(node.id)
      setEstimateRetryVersion((value) => value + 1)
      app.notify('error', '画布内容已变化，正在重新估价')
      return
    }
    if (!canvasEstimateSufficient(app.balance?.available_points, estimate)) { app.notify('error', '积分不足，请充值后重试'); return }
    setBusyNodeID(node.id)
    try {
      if (!await flushDocument()) return
      const latest = store!.getState().command.present
      if (!latest.nodes.some((item) => item.id === node.id && isCanvasGenerationNode(item)) || canvasGenerationEstimateSignature(latest, node.id) !== estimate.signature || nodeEstimatesRef.current[node.id] !== estimate || canvasQuoteRemainingMS(estimate.detail) <= 0) {
        estimateRetryNodesRef.current.add(node.id)
        setEstimateRetryVersion((value) => value + 1)
        app.notify('error', '画布内容已变化，正在重新估价')
        return
      }
      const latestErrors = generationErrors(latest.nodes.find((item) => item.id === node.id)!, latest)
      if (latestErrors.length) { app.notify('error', latestErrors[0]); return }
      if (node.type === 'video_v2' && !String(asObject(latest.nodes.find((item) => item.id === node.id)?.payload?.draft).quote_token ?? '')) { estimateNode(node); app.notify('error', '视频报价已失效，正在重新估价'); return }
      if (!canvasEstimateSufficient(app.balance?.available_points, estimate)) { app.notify('error', '积分不足，请充值后重试'); return }
      const run = await userApi.generateCanvasNode(canvas!.id, node.id, crypto.randomUUID())
      setRuns((items) => [run, ...items.filter((item) => item.id !== run.id)])
      updateNodeEstimate(node.id, () => undefined)
      app.notify('success', `已提交${node.type === 'video_v2' ? '视频' : '图片'}生成任务`)
    } catch (caught) {
      updateNodeEstimate(node.id, () => undefined); estimateNode(node)
      app.notify('error', errorMessage(caught))
    } finally { setBusyNodeID('') }
  }
  async function attachRun(run: CanvasRun) {
    setBusyNodeID(run.node_id)
    try {
      const recoveryPosition = run.status === 'unplaced' ? {
        x: (viewportSize.width / 2 - documentState.viewport.x) / documentState.viewport.zoom,
        y: (viewportSize.height / 2 - documentState.viewport.y) / documentState.viewport.zoom,
      } : undefined
      const session = remoteSessionRef.current
      if (!session || session.canvasID !== canvasID || session.blocked) return
      await session.enqueue(async () => {
        if (remoteSessionRef.current !== session || activeCanvasIDRef.current !== canvasID) return
        await userApi.attachCanvasRun(session.canvasID, run.id, recoveryPosition)
        const remote = await userApi.getCanvas(session.canvasID)
        reconcileRemote(remote, session)
      })
      await refreshRemoteRuns()
      app.notify('success', '生成结果已恢复到画布')
    } catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusyNodeID('') }
  }
  async function renameNode(node: CanvasNode, name: string) {
    try {
      if (node.asset_id) {
        const asset = await userApi.getMediaAsset(node.asset_id)
        const renamed = await userApi.updateMediaAsset(asset, { name })
        // payload.name is the stable reference alias. Canonical display names never replace it.
        store!.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, title: renamed.name } }))
        window.dispatchEvent(new Event('mgs:media-assets-changed'))
      } else store!.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, title: name } }))
    } catch (caught) { app.notify('error', errorMessage(caught)) }
  }
  function referenceCandidateError(node: CanvasNode, pick: { nodeID: string; role: CanvasEdge['input_role'] }) {
    const media = node.asset_id ? hydratedMedia[node.asset_id] : undefined
    if (!media || !['ready', 'ready_original'].includes(media.asset.status)) return '素材尚未准备就绪，请稍后重试'
    return validateCanvasReference(store!.getState().command.present, node, pick.nodeID, pick.role, admissionContext(pick.nodeID))
  }
  function connectionCandidate(sourceID: string, targetID: string, sourceHandle?: 'left' | 'right', targetHandle?: 'left' | 'right') {
    return resolveCanvasConnection(store!.getState().command.present, sourceID, targetID, { id: `edge-${crypto.randomUUID().slice(0, 12)}`, startHandle: sourceHandle, endHandle: targetHandle })
  }
  function startConnection(event: React.PointerEvent<HTMLElement>, node: CanvasNode, handle: 'left' | 'right' = 'right') {
    event.stopPropagation()
    const current = connectionRef.current
    if (current) { finishConnection(current, node.id, handle); return }
    const point = worldPoint(event.clientX, event.clientY)
    updateConnection({ pointerID: event.pointerId, sourceID: node.id, sourceHandle: handle, point, origin: point, targetID: '', error: null })
    viewportRef.current?.setPointerCapture(event.pointerId)
  }
  function finishConnection(draft: ConnectionDraft, targetID: string, handle?: 'left' | 'right') {
    const candidate = connectionCandidate(draft.sourceID, targetID, draft.sourceHandle, handle ?? draft.targetHandle)
    if (candidate.edge && !candidate.error) {
      const source = store!.getState().command.present.nodes.find((node) => node.id === candidate.edge!.source)
      if (source?.asset_id && candidate.edge.input_role !== 'prompt' && candidate.edge.input_role !== 'result') {
        const error = validateCanvasReference(store!.getState().command.present, source, candidate.edge.target, candidate.edge.input_role, admissionContext(candidate.edge.target))
        if (error) { app.notify('error', error); updateConnection(null); return }
      }
      store!.getState().connect(candidate.edge)
      updateConnection(null)
      return
    }
    if (candidate.error === 'cycle') app.notify('error', '当前生成关系不能形成循环')
    else if (candidate.error === 'input_role_conflict') app.notify('error', '首帧和尾帧均已设置，请先移除或替换现有连接')
    else if (candidate.error === 'output_slot_occupied') app.notify('error', '已有图片不能作为新的生成输出，请使用空图片框')
    else app.notify('error', '这两个节点不能连接')
    updateConnection(null)
  }
  function openNodeMenu(clientX: number, clientY: number, sourceID?: string) {
    const point = worldPoint(clientX, clientY)
    const options = sourceID ? compatibleCanvasTargets(documentState, sourceID) : [
      { type: 'image_v2' as const }, { type: 'video_v2' as const }, { type: 'note' as const },
    ]
    if (!options.length) return
    setNodeMenu({ point, sourceID, options: options.filter((option) => canCreateCanvasNode(option.type)) })
  }
  function chooseNodeMenuOption(option: { type: CanvasNodeType; role?: CanvasEdge['input_role'] }) {
    if (!nodeMenu) return
    const source = documentState.nodes.find((node) => node.id === nodeMenu.sourceID)
    if (source?.asset_id && option.role) {
      const target: CanvasNode = { id: 'candidate-target', type: option.type, position: nodeMenu.point, size: { width: 420, height: 340 }, payload: defaultNodePayload(option.type, imageCapability, videoCapability) }
      const candidate = { ...documentState, nodes: [...documentState.nodes, target] }
      const error = validateCanvasReference(candidate, source, target.id, option.role, { image: imageCapability, video: videoCapability })
      if (error) { app.notify('error', error); return }
    }
    const nodeID = addNode(option.type, nodeMenu.point)
    if (nodeMenu.sourceID && option.role) {
      store!.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: nodeMenu.sourceID, target: nodeID, input_role: option.role })
    }
    setNodeMenu(null)
    updateConnection(null)
  }

  return <main className="canvas-editor" data-canvas-editor data-readonly={readOnly} data-canvas-keyboard-open={keyboardOpen || undefined}>
    <input ref={imageUploadInputRef} hidden type="file" accept="image/*,video/*,audio/*" onChange={(event) => queueImageFrameUpload(event.target.files)} />
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
    {!readOnly ? <nav ref={dockRef} className="canvas-dock" aria-label="画布工具" data-canvas-no-zoom style={{ marginLeft: dockOffset }}>

      <button type="button" aria-pressed={state.mode !== 'connect'} title="选择节点 / 拖动背景平移 (V / H，Shift 框选)" onClick={() => { store.getState().setMode('select'); updateConnection(null) }}><MousePointer2 size={18} /></button>
      <button type="button" aria-pressed={state.mode === 'connect'} title="连接节点 (L)" onClick={() => { store.getState().setMode('connect'); updateConnection(null) }}><Link2 size={18} /></button>
      <i />
      <div className="canvas-create-tools" aria-label="创建节点">
        <button type="button" title="添加图片" onClick={() => addNode('image_v2')}><Image size={18} /></button>
        <button type="button" title="添加视频" onClick={() => addNode('video_v2')}><Film size={18} /></button>
        <button type="button" title="添加便签" onClick={() => addNode('note')}><StickyNote size={18} /></button>
      </div>
      <i />
      <button type="button" className="canvas-dock-grip" aria-label="水平移动画布工具栏" title="拖动移动工具栏" data-canvas-no-zoom onPointerDown={(event) => {
        if (event.pointerType === 'mouse' && event.button !== 0) return
        dockDragRef.current = { pointerID: event.pointerId, startX: event.clientX, startOffset: dockOffset }
        event.currentTarget.setPointerCapture(event.pointerId)
      }} onPointerMove={(event) => {
        const drag = dockDragRef.current
        if (!drag || drag.pointerID !== event.pointerId) return
        setDockOffset(clampDockOffset(drag.startOffset + event.clientX - drag.startX))
      }} onPointerUp={(event) => {
        if (dockDragRef.current?.pointerID !== event.pointerId) return
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        dockDragRef.current = null
      }} onPointerCancel={(event) => {
        if (dockDragRef.current?.pointerID !== event.pointerId) return
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        dockDragRef.current = null
      }} onLostPointerCapture={() => { dockDragRef.current = null }}><span aria-hidden="true" /></button>
      <button type="button" title="资产库" onClick={() => { setAssetTargetNodeID(''); setShowAssets(true) }}><Library size={18} /></button>
      <button type="button" title="搜索节点 (Ctrl+F)" onClick={() => setShowSearch(true)}><Search size={18} /></button>
      <button type="button" title="快捷键 (?)" onClick={() => setShowShortcuts(true)}><Keyboard size={18} /></button>
    </nav> : null}
    <div
      ref={viewportRef}
      className="canvas-viewport"
      data-mode={state.mode}
      data-space={spacePressed || undefined}
      onDoubleClick={(event) => {
        const target = event.target as Element
        if (target.closest('input,textarea,select,button,a,[contenteditable]:not([contenteditable="false"]),[data-canvas-interactive],[data-canvas-no-zoom],[data-canvas-port],[data-canvas-resize-handle]')) return
        const nodeID = target.closest<HTMLElement>('[data-canvas-node]')?.dataset.nodeId
        if (nodeID) {
          if (state.mode === 'connect' || connectionRef.current || referencePick || spacePressedRef.current) return
          const node = documentState.nodes.find((item) => item.id === nodeID)
          if (!node) return
          event.preventDefault()
          const selectedNode = documentState.nodes.find((item) => item.id === state.selectedIDs[0])
          const consoleVisible = !readOnly && state.selectedIDs.length === 1 && selectedNode && isCanvasGenerationNode(selectedNode)
          const bottom = consoleVisible ? Math.min(consoleHeight, Math.max(0, viewportSize.height - 160)) + 36 : 0
          store.getState().setViewport(fitCanvasViewport(computeCanvasBounds([node]), viewportSize, 32, { minZoom: 0.000001, maxZoom: 1.25, insets: { bottom } }))
          return
        }
        if (readOnly || target.closest('[data-canvas-edge-hit]')) return
        openNodeMenu(event.clientX, event.clientY)
      }}
      onContextMenu={(event) => {
        if (readOnly || (event.target as Element).closest('[data-canvas-node],[data-canvas-no-zoom],[data-canvas-edge-hit]')) return
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
        if ((event.target as Element).closest('[data-canvas-node],[data-canvas-no-zoom],[data-canvas-edge-hit]')) return
        if ((connectionRef.current && connectionRef.current.pointerID !== null) || (event.button !== 0 && event.button !== 1)) return
        if (event.pointerType === 'touch') {
          activePointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY })
          if (activePointersRef.current.size >= 2) {
            backgroundPointerRef.current = null
            const [first, second] = Array.from(activePointersRef.current.values())
            pinchRef.current = { distance: pointerDistance(first, second), center: pointerCenter(first, second), viewport: documentState.viewport }
            if (longPressRef.current) clearTimeout(longPressRef.current)
            longPressRef.current = null
            setSelection(null)
            setPan(null)
            event.currentTarget.setPointerCapture(event.pointerId)
            return
          } else if (!readOnly) {
            longPressRef.current = setTimeout(() => {
              const background = backgroundPointerRef.current
              if (!background || background.pointerID !== event.pointerId || background.moved) return
              background.suppressed = true
              longPressRef.current = null
              openNodeMenu(event.clientX, event.clientY)
            }, 560)
          }
        }
        event.currentTarget.setPointerCapture(event.pointerId)
        backgroundPointerRef.current = { pointerID: event.pointerId, startX: event.clientX, startY: event.clientY, button: event.button, shift: event.shiftKey, space: spacePressedRef.current, moved: false, suppressed: false }
      }}
      onPointerMove={(event) => {
        if (event.pointerType === 'touch' && activePointersRef.current.has(event.pointerId)) {
          activePointersRef.current.set(event.pointerId, { x: event.clientX, y: event.clientY })
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
        const background = backgroundPointerRef.current
        if (background?.pointerID === event.pointerId && !background.suppressed) {
          const movedX = event.clientX - background.startX
          const movedY = event.clientY - background.startY
          if (!background.moved && Math.hypot(movedX, movedY) > 5) {
            background.moved = true
            if (longPressRef.current) { clearTimeout(longPressRef.current); longPressRef.current = null }
            if (readOnly || canvasPointerIntent({ node: false, space: background.space, button: background.button, shift: background.shift, connect: false }) === 'pan') {
              setPan({ startX: background.startX, startY: background.startY, viewport: documentState.viewport })
            } else {
              const point = worldPoint(background.startX, background.startY)
              setSelection({ start: point, current: worldPoint(event.clientX, event.clientY) })
            }
          }
        }
        if (pan) store.getState().setViewport({ ...pan.viewport, x: pan.viewport.x + event.clientX - pan.startX, y: pan.viewport.y + event.clientY - pan.startY })
        if (selection) setSelection({ ...selection, current: worldPoint(event.clientX, event.clientY) })
        const draft = connectionRef.current
        if (draft) {
          const element = document.elementFromPoint(event.clientX, event.clientY) as Element | null
          const target = element?.closest<HTMLElement>('[data-canvas-node]')?.dataset.nodeId ?? ''
          const handle = element?.closest<HTMLElement>('[data-canvas-port]')?.dataset.canvasPort
          const targetHandle = handle ? canvasHandle(handle) : undefined
          const candidate = target ? connectionCandidate(draft.sourceID, target, draft.sourceHandle, targetHandle) : { error: null }
          updateConnection({ ...draft, point: worldPoint(event.clientX, event.clientY), targetID: target, targetHandle, error: candidate.error })
        }
      }}
      onPointerUp={(event) => {
        activePointersRef.current.delete(event.pointerId)
        if (activePointersRef.current.size < 2) pinchRef.current = null
        if (longPressRef.current) { clearTimeout(longPressRef.current); longPressRef.current = null }
        if (pan) {
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
          setPan(null); backgroundPointerRef.current = null; return
        }
        const draft = connectionRef.current
        // Pending click-to-connect drafts do not own a background gesture.
        if (draft && draft.pointerID !== null && draft.pointerID === event.pointerId) {
          if (draft.pointerID !== null && event.currentTarget.hasPointerCapture(draft.pointerID)) event.currentTarget.releasePointerCapture(draft.pointerID)
          const element = document.elementFromPoint(event.clientX, event.clientY) as Element | null
          const targetID = element?.closest<HTMLElement>('[data-canvas-node]')?.dataset.nodeId ?? ''
          const action = canvasConnectionRelease(draft, worldPoint(event.clientX, event.clientY), targetID)
          if (action === 'finish') finishConnection(draft, targetID, element?.closest('[data-canvas-port]') ? canvasHandle((element.closest('[data-canvas-port]') as HTMLElement).dataset.canvasPort) : undefined)
          else if (action === 'pending') updateConnection({ ...draft, pointerID: null })
          else {
            const options = compatibleCanvasTargets(documentState, draft.sourceID)
            if (options.length) setNodeMenu({ point: worldPoint(event.clientX, event.clientY), sourceID: draft.sourceID, options })
            updateConnection(null)
          }
          return
        }
        const background = backgroundPointerRef.current
        if (!background || background.pointerID !== event.pointerId) return
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        if (selection) {
          if (background?.moved) {
            const rect = normalizedRect(selection.start, selection.current)
            const hit = selectCanvasNodesInRect(transientNodes, rect)
            store.getState().select(event.shiftKey ? Array.from(new Set([...state.selectedIDs, ...hit])) : hit)
          }
        } else if (!background.moved && !background.suppressed && background.button === 0 && !background.space) {
          clearCanvasTransientState()
        }
        backgroundPointerRef.current = null
        setSelection(null); setPan(null)
      }}
      onPointerCancel={(event) => {
        activePointersRef.current.delete(event.pointerId)
        pinchRef.current = null
        if (longPressRef.current) clearTimeout(longPressRef.current)
        longPressRef.current = null
        setSelection(null); setPan(null); backgroundPointerRef.current = null; updateConnection(null)
      }}
    >
      <div className="canvas-grid" style={gridStyle(documentState.viewport)} />
      <div className="canvas-world" data-canvas-world style={{ transform: `translate(${documentState.viewport.x}px, ${documentState.viewport.y}px) scale(${documentState.viewport.zoom})` }}>
        {(documentState.groups ?? []).map((group) => {
          const bounds = canvasGroupBounds(documentState, group)
          if (!bounds) return null
          const groupSelected = group.node_ids.every((id) => selectedSet.has(id))
          return <div key={group.id} className="canvas-group" data-group-id={group.id} data-selected={groupSelected} style={{ left: bounds.x, top: bounds.y, width: bounds.width, height: bounds.height, ...(group.background ? { background: `${group.background}1f`, borderColor: `${group.background}66` } : null) }}>
            <button type="button" className="canvas-group-label" data-canvas-no-zoom title={`${group.label}（双击配置分组）`} onPointerDown={(event) => { event.stopPropagation(); store.getState().select(group.node_ids) }} onDoubleClick={(event) => { event.stopPropagation(); setGroupConfigID(group.id) }}>{group.label}</button>
          </div>
        })}
        <svg className="canvas-edges" aria-label="画布连接">
          {documentState.edges.map((edge) => {
            const source = nodeByID.get(edge.source); const target = nodeByID.get(edge.target)
            if (!source || !target) return null
            const path = canvasConnectionGeometry(source, target, canvasHandle(edge.source_handle), canvasHandle(edge.target_handle, 'left')).path
            return <g key={edge.id} data-selected={selectedEdgeSet.has(edge.id)}>
              <path className="canvas-edge-visible" d={path} vectorEffect="non-scaling-stroke" />
              {!readOnly ? <path className="canvas-edge-hit" data-canvas-edge-hit d={path} vectorEffect="non-scaling-stroke" onPointerDown={(event) => { event.stopPropagation(); store.getState().selectEdges([edge.id]) }} /> : null}
            </g>
          })}
          {connectionDraft && nodeByID.get(connectionDraft.sourceID) ? <path className="canvas-edge-draft" data-error={Boolean(connectionDraft.error)} d={canvasConnectionGeometry(nodeByID.get(connectionDraft.sourceID)!, nodeByID.get(connectionDraft.targetID) ?? connectionDraft.point, connectionDraft.sourceHandle, connectionDraft.targetHandle ?? 'left').path} vectorEffect="non-scaling-stroke" /> : null}
        </svg>
        {transientNodes.filter((node) => visibleNodeIDs.has(node.id)).map((node) => {
          const targetCandidate = connectionDraft?.sourceID && connectionDraft.sourceID !== node.id ? connectionCandidate(connectionDraft.sourceID, node.id, connectionDraft.sourceHandle) : null
          const referenceError = referencePick ? referenceCandidateError(node, referencePick) : null
          return <CanvasNodeView key={node.id} node={node} selected={selectedSet.has(node.id)} readOnly={readOnly} connecting={connectSource === node.id} referenceEligible={Boolean(referencePick && !referenceError)} connectValid={!referencePick && Boolean(targetCandidate?.edge && !targetCandidate.error)} connectInvalid={!referencePick && Boolean(targetCandidate?.error)} run={runs.find((run) => run.node_id === node.id)} media={node.asset_id ? hydratedMedia[node.asset_id] : undefined} onRename={(name) => void renameNode(node, name)} onStartConnection={(event) => {
            startConnection(event, node, canvasHandle(event.currentTarget.dataset.canvasPort))
          }} onSelect={(event) => {
          if (spacePressedRef.current || event.button === 1) {
            event.stopPropagation()
            setPan({ startX: event.clientX, startY: event.clientY, viewport: documentState.viewport })
            viewportRef.current?.setPointerCapture(event.pointerId)
            return
          }
          if (referencePick) {
            event.stopPropagation()
            const error = referenceCandidateError(node, referencePick)
            if (error) { app.notify('error', error); return }
            store.getState().connect({ id: `edge-${crypto.randomUUID().slice(0, 12)}`, source: node.id, target: referencePick.nodeID, input_role: referencePick.role })
            store.getState().select([referencePick.nodeID]); setReferencePick(null); return
          }
          if (state.mode === 'connect' || connectionRef.current) { if (!readOnly) startConnection(event, node); return }
          const target = event.target as Element
          const interactive = target.closest('[data-canvas-interactive]')
          const dragHandle = target.closest('[data-canvas-drag-handle]')
          if (interactive && !selectedSet.has(node.id)) store.getState().select([node.id])
          if (interactive || !dragHandle) return
          const selected = selectedSet.has(node.id) ? state.selectedIDs : event.shiftKey ? [...state.selectedIDs, node.id] : [node.id]
          store.getState().select(selected)
          if (readOnly) return
          // Start a drag only after a deliberate screen-space movement. In particular,
          // the two pointer sequences of a double click must not create move history.
          updateDrag({ pointerID: event.pointerId, startX: event.clientX, startY: event.clientY, selectedIDs: selected, duplicate: event.altKey, started: false, delta: { x: 0, y: 0 } })
          event.currentTarget.setPointerCapture(event.pointerId)
        }} onDrag={(event) => {
          const pending = dragRef.current
          if (!pending || pending.pointerID !== event.pointerId) return
          const dx = event.clientX - pending.startX
          const dy = event.clientY - pending.startY
          if (!pending.started && Math.hypot(dx, dy) <= 5) return
          let dragIDs = pending.selectedIDs
          if (!pending.started) {
            if (pending.duplicate && dragIDs.every((id) => { const item = documentState.nodes.find((node) => node.id === id); return item && canCopyCanvasNode(item) })) {
              store.getState().copySelected()
              store.getState().pasteClipboard()
              dragIDs = store.getState().selectedIDs
            }
            dragIDs = canvasGroupMembers(store.getState().command.present, dragIDs)
            store.getState().select(dragIDs)
          }
          updateDrag({ ...pending, selectedIDs: dragIDs, started: true, delta: { x: dx / documentState.viewport.zoom, y: dy / documentState.viewport.zoom } })
        }} onDragEnd={(event) => {
          const pending = dragRef.current
          if (!pending || pending.pointerID !== event.pointerId) return
          if (event.type !== 'pointercancel' && pending.started) store.getState().moveSelected(pending.delta)
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
          updateDrag(null)
        }} onResizeStart={(event) => {
          event.stopPropagation()
          store.getState().select([node.id])
          setResize({ pointerID: event.pointerId, nodeID: node.id, startX: event.clientX, startY: event.clientY, startSize: node.size, size: node.size })
          event.currentTarget.setPointerCapture(event.pointerId)
        }} onResize={(event) => {
          if (!resize || resize.nodeID !== node.id || resize.pointerID !== event.pointerId) return
          event.stopPropagation()
          const minimum = canvasNodeMinimumSize(node.type)
          const requested = { width: resize.startSize.width + (event.clientX - resize.startX) / documentState.viewport.zoom, height: resize.startSize.height + (event.clientY - resize.startY) / documentState.viewport.zoom }
          setResize({ ...resize, size: isCanvasV2MediaNode(node) && node.asset_id ? resizeCanvasMediaSize(resize.startSize, requested) : { width: Math.max(minimum.width, requested.width), height: Math.max(minimum.height, requested.height) } })
        }} onResizeEnd={(event) => {
          if (!resize || resize.nodeID !== node.id || resize.pointerID !== event.pointerId) return
          event.stopPropagation()
          store.getState().resizeNode(node.id, resize.size)
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
          setResize(null)
        }} onUpdate={(payload) => store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, ...payload } }))} onAttach={() => { const run = runs.find((item) => item.node_id === node.id && ['succeeded', 'unplaced'].includes(item.status)); if (run) void attachRun(run) }} onCancel={() => { const run = runs.find((item) => item.node_id === node.id && activeRunStatuses.has(item.status)); if (run) void userApi.cancelCanvasRun(canvas.id, run.id).then((next) => setRuns((items) => [next, ...items.filter((item) => item.id !== next.id)])).catch((caught) => app.notify('error', errorMessage(caught))) }} onMediaDetail={() => { if (node.asset_id) void userApi.getMediaAsset(node.asset_id).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught))) }} onMediaDimensions={(size) => {
            // Only metadata's first discovery may fit the node; subsequent loads preserve user sizing.
            if (!node.asset_id || Number(node.payload?.width) > 0 && Number(node.payload?.height) > 0) return
            store.getState().updateNode(node.id, (current) => ({ ...current, size: canvasMediaSize(size.width, size.height), payload: { ...current.payload, width: size.width, height: size.height } }))
          }} />
        })}
        {selection ? <div className="canvas-selection-box" style={rectStyle(normalizedRect(selection.start, selection.current))} /> : null}
        {nodeMenu ? <div className="canvas-node-menu" data-canvas-no-zoom style={{ left: nodeMenu.point.x, top: nodeMenu.point.y }} role="menu" aria-label={nodeMenu.sourceID ? '添加兼容节点' : '添加节点'}>
          {nodeMenu.options.map((option, index) => {
          const duplicated = nodeMenu.options.some((other, otherIndex) => otherIndex !== index && other.type === option.type)
          return <button key={`${option.type}-${option.role ?? ''}`} type="button" role="menuitem" onClick={() => chooseNodeMenuOption(option)}>{nodeTypeIcon(option.type)}<span>{nodeLabels[option.type]}{duplicated ? `（${canvasEdgeRoleLabel(option.role)}）` : ''}</span></button>
        })}
        </div> : null}
      </div>
      {referencePick ? <div className="canvas-reference-pick" data-canvas-no-zoom role="status">选择画布上的{canvasEdgeRoleLabel(referencePick.role)}素材 <button type="button" onClick={() => setReferencePick(null)}>取消 (Escape)</button></div> : null}
      {!readOnly && !referencePick && state.selectedIDs.length === 1 ? (() => {
        const node = transientNodes.find((item) => item.id === state.selectedIDs[0])
        if (!node || !isCanvasGenerationNode(node)) return null
        const signature = canvasGenerationEstimateSignature(documentState, node.id)
        const estimate = nodeEstimates[node.id]
        const valid = estimate?.status === 'ready' && estimate.signature === signature && canvasQuoteRemainingMS(estimate.detail) > 0
        const errors = generationErrors(node, documentState)
        const sufficient = estimate ? canvasEstimateSufficient(app.balance?.available_points, estimate) : false
        const references = documentState.edges.filter((edge) => edge.target === node.id && edge.input_role !== 'prompt').sort(compareCanvasReferenceEdges).flatMap((edge) => {
          const source = documentState.nodes.find((item) => item.id === edge.source)
          if (!source?.asset_id) return []
          const media = hydratedMedia[source.asset_id]
          const asset = canvasReferenceAsset(source, media)
          return [{ id: edge.id, alias: String(source.payload?.name ?? asset.name), title: media?.asset.name ?? String(source.payload?.title ?? source.payload?.name ?? source.id), asset, role: edge.input_role }]
        })
        return <div ref={observeConsole} className="canvas-console-anchor" data-canvas-no-zoom style={{ ...canvasConsoleBounds(viewportSize, { x: node.position.x * documentState.viewport.zoom + documentState.viewport.x, y: (node.position.y + node.size.height) * documentState.viewport.zoom + documentState.viewport.y + 20 }, Math.min(consoleHeight, Math.max(0, viewportSize.height - 160))), maxHeight: Math.max(0, viewportSize.height - 160) }}>
          <CanvasGenerationConsole key={node.id} node={{ ...node, type: node.type as 'image_v2' | 'video_v2' }} draft={asObject(node.payload?.draft)} imageCapability={imageCapability} videoCapability={videoCapability} references={references} estimate={{ estimated_points: estimate?.points, display_points: estimate?.points, valid: valid && !errors.length, sufficient, error: errors[0] || estimate?.error || (valid && !sufficient ? '积分不足，请充值后重试' : undefined) }} run={adaptCanvasRun(runs.find((run) => run.node_id === node.id))} busy={busyNodeID === node.id || runs.some((run) => run.node_id === node.id && activeRunStatuses.has(run.status))} estimating={estimate?.status === 'waiting' || estimate?.status === 'loading'} accessToken={app.session?.token} onPatchDraft={(patch) => store.getState().updateNode(node.id, (current) => ({ ...current, payload: { ...current.payload, draft: { ...asObject(current.payload?.draft), ...patch, quote_token: undefined } } }))} onRestoreReference={async (assetID, originalID) => {
            const sourceID = markedSources.current.get(assetID) ?? store.getState().command.present.edges.filter((edge) => edge.target === node.id).map((edge) => edge.source).find((id) => store.getState().command.present.nodes.some((item) => item.id === id && item.asset_id === assetID))
            const source = store.getState().command.present.nodes.find((item) => item.id === sourceID && item.asset_id === assetID)
            if (!source) throw new Error('引用已变化，无法恢复原图')
            nativeReferenceIDs.current.set(node.id, (nativeReferenceIDs.current.get(node.id) ?? []).map((id) => id === assetID ? originalID : id))
            store.getState().updateNode(source.id, (current) => ({ ...current, asset_id: originalID }))
          }} onUploadMarkedReference={async (file, assetID) => {
            const uploadDocument = store.getState().command.present
            const source = uploadDocument.nodes.find((item) => item.asset_id === assetID && uploadDocument.edges.some((edge) => edge.target === node.id && edge.source === item.id))
            const uploadUser = activeUserRef.current
            const uploadSession = remoteSessionRef.current
            const uploadModel = asObject(uploadDocument.nodes.find((item) => item.id === node.id)?.payload?.draft).route_model_code
            const uploadEdges = uploadDocument.edges.filter((edge) => edge.target === node.id && edge.input_role !== 'prompt').sort(compareCanvasReferenceEdges)
            const uploadSignature = JSON.stringify(uploadEdges)
            if (!source) throw new Error('引用已移除，请重新选择')
            const admission = canvasReferenceAdmission(uploadDocument, node.id, [], admissionContext(node.id))
            if (!admission.ok) throw new Error(admission.reason)
            const token = `marked-${crypto.randomUUID()}`
            return await new Promise<string>((resolve, reject) => {
              const timer = window.setTimeout(() => { window.removeEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete); reject(new Error('上传尚未完成，草稿已保留。请检查上传队列后重试。')) }, 120000)
              const complete = (event: Event) => {
                const detail = (event as CustomEvent<MediaUploadCompletedDetail>).detail
                if (detail.target?.nodeID !== token || detail.target.canvasID !== canvasID) return
                window.clearTimeout(timer); window.removeEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete)
                const latest = store.getState().command.present.nodes.find((item) => item.id === source.id)
                const currentDocument = store.getState().command.present
                const target = currentDocument.nodes.find((item) => item.id === node.id)
                if (activeUserRef.current !== uploadUser || activeCanvasIDRef.current !== canvasID || remoteSessionRef.current !== uploadSession || !target || asObject(target.payload?.draft).route_model_code !== uploadModel || JSON.stringify(currentDocument.edges.filter((edge) => edge.target === node.id && edge.input_role !== 'prompt').sort(compareCanvasReferenceEdges)) !== uploadSignature || latest?.asset_id !== assetID) { reject(new Error('引用已变化，已上传资产可从资产库恢复')); return }
                const admission = canvasReferenceAdmission(currentDocument, node.id, [], admissionContext(node.id))
                if (!admission.ok) { reject(new Error(admission.reason)); return }
                nativeReferenceIDs.current.set(node.id, (nativeReferenceIDs.current.get(node.id) ?? []).map((id) => id === assetID ? detail.asset.id : id))
                markedSources.current.set(detail.asset.id, source.id)
                store.getState().updateNode(source.id, (current) => ({ ...current, asset_id: detail.asset.id }))
                resolve(detail.asset.id)
              }
              window.addEventListener(MEDIA_UPLOAD_COMPLETED_EVENT, complete)
              const detail: QueueMediaUploadDetail = { files: [file], projectID: canvas!.project_id, target: { canvasID, nodeID: token } }
              window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail }))
            })
          }} onUpload={(role = 'reference') => uploadImageForFrame(node.id, role as CanvasEdge['input_role'])} onLibrary={(role = 'reference') => chooseImageForFrame(node.id, role as CanvasEdge['input_role'])} onPickCanvasReference={(role = 'reference') => setReferencePick({ nodeID: node.id, role: role as CanvasEdge['input_role'] })} onRemoveReference={(edgeID) => { store.getState().selectEdges([edgeID]); store.getState().deleteSelected(); store.getState().select([node.id]) }} onGenerate={() => void generateNode(node)} onEstimate={() => estimateNode(node)} />
        </div>
      })() : null}
      <div className="canvas-zoom-controls" data-canvas-no-zoom>
        <button type="button" title="缩小" onClick={() => store.getState().setViewport({ ...documentState.viewport, zoom: Math.max(0.05, documentState.viewport.zoom / 1.2) })}><ZoomOut size={17} /></button>
        <span>{Math.round(documentState.viewport.zoom * 100)}%</span>
        <button type="button" title="放大" onClick={() => store.getState().setViewport({ ...documentState.viewport, zoom: Math.min(3, documentState.viewport.zoom * 1.2) })}><ZoomIn size={17} /></button>
        <button type="button" title="适应视图" onClick={() => fitNodes()}><Focus size={17} /></button>
      </div>
      {!readOnly ? <div className="canvas-command-controls" data-canvas-no-zoom>
        <button type="button" title="撤销" disabled={!state.command.past.length} onClick={() => store.getState().undo()}><Undo2 size={17} /></button>
        <button type="button" title="重做" disabled={!state.command.future.length} onClick={() => store.getState().redo()}><Redo2 size={17} /></button>
        <button type="button" title="复制" disabled={!state.selectedIDs.length || documentState.nodes.some((node) => selectedSet.has(node.id) && !canCopyCanvasNode(node))} onClick={() => store.getState().copySelected()}><Copy size={17} /></button>
        <button type="button" title="粘贴" disabled={!state.clipboard?.nodes.length} onClick={() => store.getState().pasteClipboard()}><ClipboardPaste size={17} /></button>
        <button type="button" title="删除" disabled={!state.selectedIDs.length && !state.selectedEdgeIDs.length} onClick={() => store.getState().deleteSelected()}><Trash2 size={17} /></button>
        <button type="button" title="成组 (G)" disabled={state.selectedIDs.length < 2} onClick={() => store.getState().groupSelected()}><Group size={17} /></button>
        <button type="button" title="解组 (Shift+G)" disabled={!(documentState.groups ?? []).some((group) => group.node_ids.some((id) => selectedSet.has(id)))} onClick={() => store.getState().disbandSelectedGroups()}><Ungroup size={17} /></button>
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
    {showAssets ? <CanvasAssetDrawer projectID={canvas.project_id} mediaType={assetTargetNodeID ? assetRole === 'reference_video' ? 'video' : assetRole === 'reference_audio' ? 'audio' : 'image' : undefined} onClose={() => { setShowAssets(false); setAssetTargetNodeID('') }} onSelect={addAsset} /> : null}
    {groupConfigID ? (() => {
      const group = (documentState.groups ?? []).find((item) => item.id === groupConfigID)
      if (!group) return null
      return <Modal title="分组配置" onClose={() => setGroupConfigID('')} className="canvas-group-config-dialog">
        <label className="canvas-group-field"><span>分组名称</span><input value={group.label} maxLength={64} onChange={(event) => store.getState().updateGroup(group.id, { label: event.target.value })} /></label>
        <div className="canvas-group-field"><span>背景颜色</span><div className="canvas-group-swatches">
          {['', '#f59e0b', '#10b981', '#3b82f6', '#8b5cf6', '#ec4899', '#64748b'].map((color) => <button key={color || 'default'} type="button" data-active={(group.background ?? '') === color} style={color ? { background: `${color}33`, borderColor: color } : undefined} title={color || '默认'} aria-label={color || '默认背景'} onClick={() => store.getState().updateGroup(group.id, { background: color })} />)}
        </div></div>
        <div className="canvas-group-field"><span>成员节点（{group.node_ids.length}）</span><ul>{group.node_ids.map((id) => { const member = documentState.nodes.find((item) => item.id === id); return <li key={id}>{member ? `${nodeLabels[member.type]} · ${String(member.payload?.title ?? '')}` : id}</li> })}</ul></div>
        <div className="canvas-group-config-actions">
          <Button tone="ghost" onClick={() => { store.getState().select(group.node_ids); setGroupConfigID('') }}>选中成员</Button>
          <Button onClick={() => { store.getState().disbandSelectedGroups(); store.getState().select(group.node_ids); setGroupConfigID('') }}>解散分组</Button>
        </div>
      </Modal>
    })() : null}
    {showShortcuts ? <Modal title="快捷键" onClose={() => setShowShortcuts(false)} className="canvas-shortcuts-dialog">
      <div className="canvas-shortcuts-grid">
        {[['V / H · Space · Shift', '选择与背景平移 · 按住平移 · 框选'], ['L', '连接节点'], ['G / Shift+G', '成组 / 解组'], ['双击节点', '打开参数配置'], ['Ctrl+Z / Shift+Z', '撤销 / 重做'], ['Ctrl+C / V / D', '复制 / 粘贴 / 副本'], ['Ctrl+A / F', '全选 / 搜索节点'], ['Alt+拖动', '拖动复制节点'], ['Enter', '生成所选节点'], ['0', '适应视图'], ['方向键', '微移（Shift 加速）'], ['Delete', '删除所选']].map(([keys, label]) => <div key={keys}><kbd>{keys}</kbd><span>{label}</span></div>)}
      </div>
    </Modal> : null}
    {previewAsset ? <MediaPreviewDialog asset={previewAsset} projects={projects.projects} creationActions={mediaCreationActions(previewAsset)} onClose={() => setPreviewAsset(null)} onChanged={setPreviewAsset} onDeleted={() => setPreviewAsset(null)} onContinue={(options) => { window.location.hash = userHashForRoute('genpic', options) }} /> : null}
    {conflict ? <div className="canvas-conflict" role="dialog" aria-modal="true" data-canvas-no-zoom><div><strong>画布已在其他页面更新</strong><p>远端版本 r{conflict.remote.revision}，本地草稿基于 r{state.command.revision}。请选择保留方式，系统不会自动覆盖。</p><footer><Button tone="ghost" onClick={() => { store.getState().replaceRemote(toLocalDocument(conflict.remote.document), conflict.remote.revision); setCanvas(conflict.remote); setConflict(null) }}>使用远端版本</Button><Button onClick={() => void userApi.createCanvas({ project_id: canvas.project_id, name: `${canvas.name} 本地副本`, document: toWireDocument(conflict.local) }).then((copy) => { app.notify('success', `已创建副本：${copy.name}`); app.navigate('creative-canvas', { canvasId: copy.id }) }).catch((caught) => app.notify('error', errorMessage(caught)))}>复制本地版本</Button></footer></div></div> : null}
  </main>
}

function CanvasNodeView({ node, selected, readOnly, connecting, connectValid, connectInvalid, referenceEligible, run, media, onRename, onSelect, onDrag, onDragEnd, onResizeStart, onResize, onResizeEnd, onStartConnection, onUpdate, onAttach, onCancel, onMediaDetail, onMediaDimensions }: {
  node: CanvasNode; selected: boolean; readOnly: boolean; connecting: boolean; connectValid: boolean; connectInvalid: boolean; referenceEligible: boolean; run?: CanvasRun; media?: CanvasHydratedMedia
  onSelect: (event: React.PointerEvent<HTMLElement>) => void; onDrag: (event: React.PointerEvent<HTMLElement>) => void; onDragEnd: (event: React.PointerEvent<HTMLElement>) => void
  onResizeStart: (event: React.PointerEvent<HTMLButtonElement>) => void; onResize: (event: React.PointerEvent<HTMLButtonElement>) => void; onResizeEnd: (event: React.PointerEvent<HTMLButtonElement>) => void
  onStartConnection: (event: React.PointerEvent<HTMLButtonElement>) => void
  onUpdate: (payload: Record<string, unknown>) => void; onRename: (name: string) => void; onAttach: () => void; onCancel: () => void; onMediaDetail: () => void; onMediaDimensions: (size: { width: number; height: number }) => void
}) {
  const v2 = isCanvasV2MediaNode(node)
  return <article className={`canvas-node${v2 ? ' canvas-node-v2' : ''}`} data-canvas-node data-canvas-drag-handle={v2 ? '' : undefined} data-node-id={node.id} data-layer={node.payload?.layer ? true : undefined} data-type={node.type} data-selected={selected} data-connecting={connecting} data-connect-valid={connectValid || undefined} data-connect-invalid={connectInvalid || undefined} data-reference-eligible={referenceEligible || undefined} style={{ left: node.position.x, top: node.position.y, width: node.size.width, height: node.size.height }} onPointerDown={onSelect} onPointerMove={onDrag} onPointerUp={onDragEnd} onPointerCancel={onDragEnd}>
    {!readOnly && selected && v2 ? <button type="button" className="canvas-port canvas-port-target" data-canvas-interactive data-canvas-port="left" title="连接节点（左侧）" onPointerDown={onStartConnection} /> : null}
    {v2 ? <CanvasMediaNodeV2 node={{ ...node, type: node.type as 'image_v2' | 'video_v2' }} title={media?.asset.name ?? String(node.payload?.title ?? nodeLabels[node.type])} mediaUrl={media?.url} aspectRatio={node.size.width / node.size.height} selected={selected} run={adaptCanvasRun(run)} onRename={readOnly ? undefined : onRename} onPreview={onMediaDetail} onMediaDimensions={onMediaDimensions} /> : <>
      <header data-canvas-drag-handle><span>{nodeTypeIcon(node.type)}</span><strong>{String(node.payload?.title ?? nodeLabels[node.type])}</strong></header>
      <div className="canvas-node-body" data-canvas-interactive data-canvas-no-zoom>
        {node.type === 'note' ? <textarea readOnly={readOnly} defaultValue={String(node.payload?.text ?? '')} placeholder="记录创作想法" onBlur={(event) => onUpdate({ text: event.target.value })} /> : <LegacyNodeBody node={node} run={run} media={media} onDetail={onMediaDetail} onAttach={onAttach} onCancel={readOnly ? () => undefined : onCancel} />}
      </div>
    </>}
    {!readOnly && selected && v2 ? <button type="button" className="canvas-port canvas-port-source" data-canvas-interactive data-canvas-port="right" title="连接节点（右侧）" onPointerDown={onStartConnection} /> : null}
    {!readOnly && selected && (v2 || node.type === 'note') ? <button type="button" className="canvas-node-resize" data-canvas-interactive data-canvas-resize-handle title="调整节点大小" onPointerDown={onResizeStart} onPointerMove={onResize} onPointerUp={onResizeEnd} onPointerCancel={onResizeEnd} /> : null}
  </article>
}
function adaptCanvasRun(run?: CanvasRun) {
  return run ? { status: run.status === 'canceled' ? 'cancelled' : run.status === 'attached' || run.status === 'unplaced' ? 'succeeded' : run.status, stage: run.progress_stage || undefined, progress_message: run.progress_message || undefined, error: run.error_message } : undefined
}

function LegacyNodeBody({ node, run, media, onDetail, onAttach, onCancel }: { node: CanvasNode; run?: CanvasRun; media?: CanvasHydratedMedia; onDetail: () => void; onAttach: () => void; onCancel: () => void }) {
  return <div className="canvas-legacy-body">
    {node.asset_id ? <><button type="button" onClick={onDetail}>查看详情</button>{media?.url && canvasNodeMediaKind(node) === 'image' ? <img src={media.url} alt={media.asset.name} /> : media?.url && canvasNodeMediaKind(node) === 'video' ? <video src={media.url} controls playsInline /> : media?.url ? <audio src={media.url} controls /> : <span>正在加载媒体</span>}</> : <p>{String(node.payload?.text ?? node.payload?.title ?? '历史生成节点')}</p>}
    {run ? <span>{run.progress_message || run.progress_stage || run.status}</span> : null}
    {run && activeRunStatuses.has(run.status) ? <button type="button" onClick={onCancel}>取消任务</button> : null}
    {run && ['succeeded', 'unplaced'].includes(run.status) ? <button type="button" onClick={onAttach}>恢复结果</button> : null}
    {run?.error_message ? <small role="alert">{run.error_message}</small> : null}
  </div>
}

const emptyCanvasStore = createCanvasStore({ schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [], edges: [] }, 0)
function toLocalDocument(document: CreativeCanvas['document']): CanvasDocument { return normalizeCanvasDocument(document) as CanvasDocument }
function canonicalCanvasDocument(document: CanvasDocument) {
  return {
    schema_version: 1,
    viewport: { x: document.viewport.x, y: document.viewport.y, zoom: document.viewport.zoom },
    nodes: document.nodes,
    edges: document.edges,
    groups: document.groups?.length ? document.groups : [],
  }
}
function toWireDocument(document: CanvasDocument): CreativeCanvas['document'] { return document as CreativeCanvas['document'] }
function asObject(value: unknown) { return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {} }
function defaultNodePayload(type: CanvasNodeType, imageCapability?: Capability | null, videoCapability?: VideoCapability | null) {
  if (type === 'prompt') return { title: '提示词', text: '' }
  if (type === 'image') return { title: '图片框' }
  if (type === 'note') return { title: '便签', text: '' }
  if (type === 'image_v2') {
    const model = imageCapability?.model_groups.find((item) => item.task_types.includes('text_to_image')) ?? imageCapability?.model_groups[0]
    const taskType = 'text_to_image'
    const options = model?.capabilities_by_task_type?.[taskType] ?? model
    const sizeMode = options?.size_modes?.includes('auto') ? 'auto' : options?.size_modes?.[0] ?? 'auto'
    return { title: '图片', draft: normalizeCanvasImageDraft({ prompt_template: '', variables: {}, route_model_code: model?.code ?? '', task_type: taskType, ...canvasImageSizeDraftPatch(sizeMode, options ?? {}), quality: options?.quality?.[0] ?? '', output_format: options?.output_format?.[0] ?? '', output_image_count: 1 }, imageCapability!) }
  }
  if (type === 'video_v2') {
    const model = videoCapability?.model_groups[0]
    return { title: '视频', draft: normalizeCanvasVideoDraft({ prompt_template: '', variables: {}, route_model_code: model?.code ?? '', task_type: model?.defaults.task_type ?? model?.task_types[0] ?? 'text_to_video', duration_seconds: model?.defaults.duration_seconds, resolution: model?.defaults.resolution, aspect_ratio: model?.defaults.aspect_ratio, audio_mode: model?.defaults.generate_audio ? 'generated' : 'silent', output_count: 1 }, videoCapability!) }
  }
  return {}
}
function addAssetNode(store: ReturnType<typeof createCanvasStore>, asset: MediaAsset, position: { x: number; y: number }) {
  const node = canvasAssetNode(asset, position)
  store.getState().addNode({ ...node, position: { x: position.x - node.size.width / 2, y: position.y - node.size.height / 2 } })
}
function generationInputSummary(node: CanvasNode, document: CanvasDocument): GenerationInputSummary {
  const draft = asObject(node.payload?.draft)
  const edges = document.edges.filter((edge) => edge.target === node.id)
  const errors: string[] = []
  if (isCanvasGenerationNode(node)) {
    if (!String(draft.prompt_template ?? draft.prompt ?? '').trim()) errors.push('缺少提示词')
    if (!String(draft.route_model_code ?? '').trim()) errors.push('请选择模型分组')
    const template = String(draft.prompt_template ?? draft.prompt ?? '')
    const variables = asObject(draft.variables)
    for (const match of template.matchAll(/\$\{([^}]+)\}|\{\{([^}@]+)\}\}/g)) if (!String(variables[(match[1] || match[2]).trim()] ?? '').trim()) errors.push(`请填写变量：${match[1] || match[2]}`)
    const task = String(draft.task_type ?? '')
    if (node.type === 'video_v2' && ['image_to_video', 'first_last_frame_to_video'].includes(task) && !edges.some((edge) => edge.input_role === 'first_frame')) errors.push('缺少首帧图片')
    if (node.type === 'video_v2' && task === 'first_last_frame_to_video' && !edges.some((edge) => edge.input_role === 'last_frame')) errors.push('缺少尾帧图片')
    if (node.type === 'video_v2' && task === 'reference_to_video' && !edges.length) errors.push('缺少参考素材')
    if (node.type === 'video_v2' && ['video_edit', 'video_extend'].includes(task) && !edges.some((edge) => edge.input_role === 'reference_video')) errors.push('缺少参考视频')
  }
  return { prompts: 0, images: edges.filter((edge) => ['reference', 'first_frame', 'last_frame', 'reference_image'].includes(edge.input_role)).length, promptNodes: [], selectedPromptID: '', referenceBindings: [], errors }
}
function canvasEdgeRoleLabel(role?: CanvasEdge['input_role'] | null) {
  switch (role) {
    case 'first_frame': return '首帧'
    case 'last_frame': return '尾帧'
    case 'reference': case 'reference_image': return '参考图'
    case 'reference_video': return '参考视频'
    case 'reference_audio': return '参考音频'
    default: return '输入'
  }
}

function nodeTypeIcon(type: CanvasNodeType) {
  if (type === 'image' || type === 'image_v2') return <Image size={16} />
  if (type === 'video' || type === 'video_v2') return <Film size={16} />
  if (type === 'audio') return <Music2 size={16} />
  if (type === 'note') return <StickyNote size={16} />
  return <Sparkles size={16} />
}
function normalizedRect(start: { x: number; y: number }, end: { x: number; y: number }) { return { x: Math.min(start.x, end.x), y: Math.min(start.y, end.y), width: Math.abs(end.x - start.x), height: Math.abs(end.y - start.y) } }
function rectStyle(rect: { x: number; y: number; width: number; height: number }) { return { left: rect.x, top: rect.y, width: rect.width, height: rect.height } }
function gridStyle(viewport: CanvasViewport) { const size = 32 * viewport.zoom; return { backgroundSize: `${size}px ${size}px`, backgroundPosition: `${viewport.x % size}px ${viewport.y % size}px` } }
function pointerDistance(first: { x: number; y: number }, second: { x: number; y: number }) { return Math.hypot(second.x - first.x, second.y - first.y) }
function pointerCenter(first: { x: number; y: number }, second: { x: number; y: number }) { return { x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 } }
