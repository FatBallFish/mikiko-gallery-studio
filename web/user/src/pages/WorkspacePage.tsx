import { ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronUp, Copy, Download, Info, Pencil, RotateCcw } from 'lucide-react'
import type { Capability, CapabilityModelGroup, EstimateRequest, GalleryImage, ImageResult, ImageTask, ImageTaskStatus, ImageTaskType, ReferenceAsset, UserProfile } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { toTask, userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, ImageDetailModal, InlineCopyButton, LoadingState, Modal, PublicDetailIcon, copyText, useApp, type ImagePreviewPayload } from '../components'
import { userState } from '../ui/classes'
import { rdWorkspace } from '../ui/redesign-classes'
import { OverlayPortal } from '../ui/overlayPortal'
import { RefreshableMediaImage } from '../ui/mediaRefresh'
import { pointsText } from '../../../shared/pointsDisplay'
import { errorMessage } from '../useApiResource'
import { mediaAccess, type MediaResource } from '../mediaAccess'
import { consumeWorkspaceCreationDraft, normalizeWorkspaceCreationDraft, stageWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot, type WorkspaceCreationDraft } from './workspaceCreationDraft'
import { displayPoints, publicUnavailableReason, WORKSPACE_REFERENCE_REQUIRED_MESSAGE } from './workspaceGenerateReadiness'
import { currentWorkspaceEstimate, workspaceDisplayedResolvedSize, workspaceEstimateKey, type WorkspaceEstimateSnapshot } from './workspaceEstimate'
import { defaultGalleryImportFilter, filterGalleryImportImages, firstGalleryReferenceReuse, galleryImportOptions, galleryImportSuccessMessage, mergeReferenceAssets, type GalleryImportFilter } from './workspaceGalleryImport'
import { WorkspaceStatusRail } from './WorkspaceStatusRail'
import { workspaceTaskCardView, workspaceTaskFailureView } from './workspaceTaskFailure'
import { generationSlots, workspaceBaseResolutionLabel, type WorkspaceProgressNode } from './workspaceTaskProgress'
import { limitReferenceSelection, remainingReferenceCapacity, singleReferenceAddition, workspaceReferenceMaximum, workspaceRequiredReferencesReady } from './workspaceReferenceLimit'
import { createWorkspaceViewModel, matchWorkspaceCapabilityOption, normalizeWorkspaceImageCount, workspaceTaskImageSafetyLimit } from './workspaceViewModel'
import { useCompactWorkspaceViewport, workspaceParametersHidden } from './workspaceResponsive'
import { workspaceSheetDragOffset, workspaceSheetSnap } from './workspaceSheetGesture'
import { mergeWorkspaceTaskRecords, replaceWorkspaceTaskRecords, workspaceTaskHistoryInteraction } from './workspaceTaskHistory'
import { closeWorkspaceStreamGeneration, createWorkspaceStreamGeneration, markWorkspaceStreamHealthy, nextWorkspaceStreamRetry, workspaceStreamEventIsCurrent, workspaceStreamRecoveryIsCurrent, type WorkspaceStreamGeneration } from './workspaceTaskStream'
import { projectWorkspaceImageDetail } from './workspaceImageDetail'
import { referenceImageAccept, referenceImagePolicy, validateReferenceImageFile } from './referenceImageUpload'
import { chooseWorkspaceSizeMode, normalizeWorkspaceCustomSize, normalizeWorkspaceOutputParameters, workspaceBackgroundForFormat, workspaceBackgroundOptions, workspaceCompressionVisible, workspaceCustomRatioSupported, workspaceCustomRatioValid, workspaceCustomSizeSupported, workspaceModelForTask, workspaceOutputOptions, workspacePixelOptions, workspaceRatioOptions, workspaceRatioPixelEstimate, workspaceSizeModeOptions, workspaceSizeParameterError, type WorkspaceSizeMode } from './workspaceParameters'
import { PromptEditorActions, PromptEditorDialog, PromptOptimizationPanel } from './PromptEditorDialog'
import { PromptTemplateEditor, type PromptTemplateEditorHandle } from './PromptTemplateEditor'
import { PromptVariableForm } from './PromptVariableForm'
import { ModelGroupSelect } from './ModelGroupSelect'
import { buildPromptReferenceBindings, buildPromptVariableInputs, expandedPromptCodePointLength, promptVariableValidation, reconcilePromptVariables, renamePromptReference } from './promptTemplateEditorModel'
import { parsePromptTemplate } from './promptTemplateParser'
import { applyOptimizedPrompt, beginPromptOptimization, confirmPromptOptimization, failPromptOptimization, initialPromptOptimizationState, receivePromptEstimate, receivePromptOptimization, undoPromptOptimization } from './workspacePromptOptimization'
import { useProjects } from '../ProjectContext'
import { MEDIA_ASSETS_CHANGED_EVENT, QUEUE_MEDIA_UPLOAD_EVENT } from '../features/media/UploadTray'
import { workspaceProjectReadiness, workspaceSubmissionIsCurrent, type WorkspaceProjectSelection } from './workspaceProjectLifecycle'

type OutputTab = 'current' | 'history'
type WorkspacePixelSelection = 'preset' | 'custom'
type RestoreParameters = {
  routeModelCode?: string
  sizeMode?: WorkspaceSizeMode
  baseResolution?: string
  aspectRatio?: string
  pixelSize?: string
  quality?: string
	outputFormat?: string
	background?: string
  outputCompression?: number
  moderation?: string
  imageCount?: number
}
type UploadTarget = 'edit'
type DragUploadState = Record<UploadTarget, boolean>
type SheetDragState = { pointerId: number; startY: number; dragged: boolean }

function selectableModels(capability: Capability, taskType: ImageTaskType) {
  return capability.model_groups.filter((item) => item.task_types.includes(taskType))
}

function baseResolutionOptions(model: CapabilityModelGroup | undefined) {
  return model?.base_resolution?.length ? model.base_resolution : []
}

function isTerminalStatus(status: ImageTaskStatus | string) {
  return ['succeeded', 'partial_failed', 'failed', 'cancelled', 'rejected', 'deleted'].includes(status)
}

function displayTaskPoints(task: ImageTask) {
  return pointsText(task.actual_points ?? task.estimate_points ?? task.estimated_points ?? '0')
}

function formatFileSize(bytes?: number) {
  if (!bytes || bytes <= 0) return ''
  const mb = bytes / (1024 * 1024)
  if (mb >= 1) return `${mb.toFixed(mb >= 10 ? 0 : 1)} MB`
  const kb = bytes / 1024
  return `${Math.max(1, Math.round(kb))} KB`
}

function referenceAssetPreviewURL(asset: ReferenceAsset, accessToken?: string | null) {
  const raw = asset.preview_url || asset.download_url || ''
  return raw ? userApi.imageAssetUrl(raw, accessToken) : ''
}

function patchTaskImageAccess(task: ImageTask, imageId: string, url: string, expiresAt?: string) {
  let changed = false
  const results = task.results.map((image) => {
    if (image.id !== imageId) return image
    changed = true
    return { ...image, url, download_url: url, preview_expires_at: expiresAt }
  })
  return changed ? { ...task, results } : task
}

function patchTaskReferenceAccess(task: ImageTask, assetId: string, url: string, expiresAt?: string) {
  let changed = false
  const referenceAssets = task.reference_assets.map((asset) => {
    if (asset.id !== assetId) return asset
    changed = true
    return { ...asset, preview_url: url, download_url: url, preview_expires_at: expiresAt }
  })
  return changed ? { ...task, reference_assets: referenceAssets } : task
}

function workspaceImageFilename(image: Pick<ImageResult, 'id' | 'mime_type'>) {
  const extension = image.mime_type?.split('/')[1]?.replace('jpeg', 'jpg') || 'png'
  return `mikiko-image-${image.id}.${extension}`
}

function startMediaDownload(url: string, filename: string) {
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.rel = 'noopener noreferrer'
  document.body.appendChild(link)
  link.click()
  link.remove()
}

function uploadErrorMessage(error: unknown) {
  if (error instanceof ApiError && error.code === 'IMAGE_REFERENCE_TOO_LARGE') {
    const maxBytes = Number(error.details?.max_size_bytes ?? 0)
    const actualBytes = Number(error.details?.actual_size_bytes ?? 0)
    if (maxBytes > 0 && actualBytes > 0) {
      return `单张参考图最大 ${formatFileSize(maxBytes)}，当前文件 ${formatFileSize(actualBytes)}。`
    }
  }
  return errorMessage(error)
}

function generationParameterErrorMessage(error: unknown) {
  if (error instanceof ApiError && error.code === 'IMAGE_CAPABILITY_MISMATCH') {
    const field = String(error.details?.field ?? '')
    const rule = String(error.details?.rule ?? '')
    const label = field === 'width' ? '宽度' : field === 'height' ? '高度' : field === 'aspect_ratio' ? '比例' : field === 'pixel_size' ? '像素尺寸' : '生成参数'
    const minimum = Number(error.details?.min)
    const maximum = Number(error.details?.max)
    if (rule === 'range' && Number.isFinite(minimum) && Number.isFinite(maximum)) return `${label}必须在 ${minimum} 至 ${maximum} 像素之间。`
    if (rule === 'multiple_of_16') return '宽度和高度必须为 16 的倍数。'
    if (rule === 'format') return field === 'aspect_ratio' ? '比例请使用“宽:高”格式，例如 16:9。' : '像素尺寸格式不合法。'
    if (rule === 'max_ratio') return `${label}必须在 1:3 至 3:1 范围内。`
    if (rule === 'pixel_count') return '像素尺寸的总像素数超出平台限制。'
  }
  return errorMessage(error)
}

const workspaceClasses = {
  generatedImage: 'object-contain transition duration-500 group-hover:scale-[1.04] motion-reduce:transition-none motion-reduce:group-hover:scale-100',
  historyImage: 'size-full object-cover transition duration-500 group-hover:scale-105 motion-reduce:transition-none motion-reduce:group-hover:scale-100',
  outputResultWrap: 'animate-in fade-in zoom-in-95 duration-500 motion-reduce:animate-none',
  slotSkeletonGlow: 'absolute inset-0 -translate-x-full bg-[linear-gradient(90deg,transparent_0%,rgba(var(--accent-rgb),0.16)_48%,transparent_100%)] animate-[shimmer_1.8s_linear_infinite] motion-reduce:animate-none',
  refRemove: 'opacity-100 transition-opacity duration-[var(--motion-fast)] group-focus-within:opacity-100 focus-visible:opacity-100 max-[760px]:opacity-100 [@media(pointer:coarse)]:opacity-100 motion-reduce:transition-none',
  panelSectionFinal: 'max-[760px]:p-3',
}

export function WorkspacePage({ initialTaskId }: { initialTaskId?: string }) {
  const app = useApp()
  const projects = useProjects()
  const selectedProjectID = projects.selectedProjectID
  const compactViewport = useCompactWorkspaceViewport()

  const [capability, setCapability] = useState<Capability | null>(null)
  const [editRefs, setEditRefs] = useState<ReferenceAsset[]>([])
  const [prompt, setPrompt] = useState('')
  const [promptVariables, setPromptVariables] = useState<Record<string, string>>({})
  const [promptExpanded, setPromptExpanded] = useState(false)
  const [promptOptimization, setPromptOptimization] = useState(initialPromptOptimizationState)
  const [model, setModel] = useState('')
  const [sizeMode, setSizeMode] = useState<WorkspaceSizeMode>('auto')
  const [baseResolution, setBaseResolution] = useState('')
  const [ratio, setRatio] = useState('')
  const [customRatio, setCustomRatio] = useState('')
  const [pixelSize, setPixelSize] = useState('')
  const [pixelSelection, setPixelSelection] = useState<WorkspacePixelSelection>('preset')
  const [customWidth, setCustomWidth] = useState('')
  const [customHeight, setCustomHeight] = useState('')
  const [count, setCount] = useState(1)
  const [quality, setQuality] = useState('auto')
	const [outputFormat, setOutputFormat] = useState('png')
	const [background, setBackground] = useState('')
  const [outputCompression, setOutputCompression] = useState(100)
  const [moderation, setModeration] = useState('auto')
  const [estimateSnapshot, setEstimateSnapshot] = useState<WorkspaceEstimateSnapshot>({ key: '', estimate: null, error: '' })
  const [records, setRecords] = useState<ImageTask[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(() => initialTaskId?.trim() || null)
  const selectedTaskIdRef = useRef<string | null>(selectedTaskId)
  const projectSelectionRef = useRef<WorkspaceProjectSelection>({
    projectID: projects.selectedProjectID,
    generation: projects.selectionGeneration,
  })
  const [initialTaskLoading, setInitialTaskLoading] = useState(false)
  const [initialTaskError, setInitialTaskError] = useState('')
  const [initialTaskReloadKey, setInitialTaskReloadKey] = useState(0)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [previewImage, setPreviewImage] = useState<ImagePreviewPayload | null>(null)
  const previewDetailImage: ImageResult | null = previewImage?.detailImage ?? (previewImage ? {
    id: previewImage.alt || 'preview',
    url: previewImage.url,
    download_url: previewImage.downloadUrl,
    width: previewImage.width ?? 0,
    height: previewImage.height ?? 0,
    publish_status: 'private',
    prompt: previewImage.prompt,
    aspect_ratio: previewImage.ratio,
    route_model_code: previewImage.model,
    author_name: app.profile?.display_name,
  } : null)
  const [outputTab, setOutputTab] = useState<OutputTab>('current')
  const [historyTaskDialog, setHistoryTaskDialog] = useState<ImageTask | null>(null)
  const [historyPreviewReturnTarget, setHistoryPreviewReturnTarget] = useState<{ taskId: string; imageId: string } | null>(null)
  const [galleryImportTarget, setGalleryImportTarget] = useState<'edit' | null>(null)
  const [galleryImages, setGalleryImages] = useState<GalleryImage[]>([])
  const [galleryImportLoading, setGalleryImportLoading] = useState(false)
  const [galleryImportBusy, setGalleryImportBusy] = useState(false)
  const [galleryImportFilter, setGalleryImportFilter] = useState<GalleryImportFilter>(defaultGalleryImportFilter)
  const [dragUpload, setDragUpload] = useState<DragUploadState>({ edit: false })
  const [renamingReferenceID, setRenamingReferenceID] = useState<string | null>(null)
  const [renamingReferenceName, setRenamingReferenceName] = useState('')
  const [renamingReferenceBusy, setRenamingReferenceBusy] = useState(false)
  const [parametersExpanded, setParametersExpanded] = useState(false)
  const [sheetDragOffset, setSheetDragOffset] = useState(0)
  const parametersHidden = workspaceParametersHidden(compactViewport, parametersExpanded)
  const [streamRetryKey, setStreamRetryKey] = useState(0)
  const streamRef = useRef<EventSource | null>(null)
  const streamGenerationRef = useRef<WorkspaceStreamGeneration | null>(null)
  const streamTokenRef = useRef<string | null>(null)
  const streamRetryCountRef = useRef(0)
  const streamRecoveryTimerRef = useRef<number | null>(null)
  const streamDisconnectNoticeRef = useRef(false)
  const streamExhaustedNoticeRef = useRef(false)
  const notifyRef = useRef(app.notify)
  const refreshAccountRef = useRef(app.refreshAccount)
  const completedNoticeRef = useRef<Set<string>>(new Set())
  const feedEndRef = useRef<HTMLDivElement | null>(null)
  const compactPromptEditorRef = useRef<PromptTemplateEditorHandle | null>(null)
  const expandedPromptEditorRef = useRef<PromptTemplateEditorHandle | null>(null)
  const pendingPromptAssetInsertRef = useRef<'compact' | 'expanded' | null>(null)
  const restoreParametersRef = useRef<RestoreParameters | null>(null)
  const pendingCreationDraftRef = useRef<WorkspaceCreationDraft | null | undefined>(undefined)
  const sheetDragRef = useRef<SheetDragState | null>(null)
  const suppressSheetClickRef = useRef(false)
  const sheetClickResetRef = useRef<number | null>(null)
  const taskType: ImageTaskType = editRefs.length ? 'image_edit' : 'text_to_image'
  const referenceCount = editRefs.length
  const requiredReferencesReady = workspaceRequiredReferencesReady(taskType, referenceCount)
  const projectReadiness = workspaceProjectReadiness({
    loading: projects.loading,
    error: projects.error,
    selectedProjectID: projects.selectedProjectID,
    selectedProject: projects.selectedProject,
  })
  const projectIDsKey = projects.projects
    .filter((project) => project.status === 'active')
    .map((project) => project.id)
    .sort()
    .join(',')

  notifyRef.current = app.notify

  useEffect(() => {
    setPromptVariables((current) => reconcilePromptVariables(prompt, current))
  }, [prompt])
  refreshAccountRef.current = app.refreshAccount
  projectSelectionRef.current = {
    projectID: projects.selectedProjectID,
    generation: projects.selectionGeneration,
  }

  useEffect(() => {
    selectedTaskIdRef.current = selectedTaskId
  }, [selectedTaskId])

  useEffect(() => {
    setRecords((items) => items.filter((task) => task.project_id === selectedProjectID))
    setHistoryTaskDialog(null)
    setHistoryPreviewReturnTarget(null)
    setPreviewImage(null)
    setGalleryImages([])
    setGalleryImportTarget(null)
  }, [selectedProjectID])

  useEffect(() => {
    if (!historyPreviewReturnTarget || previewImage || historyTaskDialog?.id !== historyPreviewReturnTarget.taskId) return undefined
    const frame = window.requestAnimationFrame(() => {
      const target = Array.from(document.querySelectorAll<HTMLElement>('[data-history-image-id]')).find((element) => (
        element.dataset.historyImageId === historyPreviewReturnTarget.imageId
      ))
      target?.focus()
      setHistoryPreviewReturnTarget((current) => current?.taskId === historyPreviewReturnTarget.taskId && current.imageId === historyPreviewReturnTarget.imageId ? null : current)
    })
    return () => window.cancelAnimationFrame(frame)
  }, [historyPreviewReturnTarget, historyTaskDialog?.id, previewImage])

  // Load capability and refs on mount only (not on mode change)
  useEffect(() => {
    let mounted = true
    async function load() {
      setLoading(true)
      try {
        const nextCapability = await userApi.getCapabilities()
        if (!mounted) return
        setCapability(nextCapability)
      } catch (err) {
        if (mounted) notifyRef.current('error', errorMessage(err))
      } finally {
        if (mounted) setLoading(false)
      }
    }
    void load()
    return () => { mounted = false }
  }, [])

  useEffect(() => {
    const taskId = initialTaskId?.trim() || ''
    setSelectedTaskId(taskId || null)
    setInitialTaskError('')
    if (!taskId) {
      setInitialTaskLoading(false)
      return undefined
    }
    if (projects.loading) {
      setInitialTaskLoading(true)
      return undefined
    }
    if (projects.error) {
      setInitialTaskLoading(false)
      setInitialTaskError(projects.error)
      return undefined
    }
    if (!projectIDsKey) {
      setInitialTaskLoading(false)
      setInitialTaskError('没有可用项目，无法打开该任务。')
      return undefined
    }
    let cancelled = false
    setInitialTaskLoading(true)
    async function loadInitialTask() {
      try {
        const task = await userApi.getTask(taskId)
        if (cancelled) return
        const taskProject = projects.projects.find((project) => project.status === 'active' && project.id === task.project_id)
        if (!task.project_id || !taskProject) {
          setInitialTaskError('任务所属项目不存在或无权访问。')
          return
        }
        if (projects.selectedProjectID !== task.project_id) projects.selectProject(task.project_id)
        setRecords((items) => mergeWorkspaceTaskRecords(items, task, { limit: 20, preserveIds: [taskId] }))
      } catch (err) {
        if (!cancelled) setInitialTaskError(errorMessage(err))
      } finally {
        if (!cancelled) setInitialTaskLoading(false)
      }
    }
    void loadInitialTask()
    return () => { cancelled = true }
  }, [initialTaskId, initialTaskReloadKey, projectIDsKey, projects.error, projects.loading])

  useEffect(() => () => {
    if (sheetClickResetRef.current !== null) window.clearTimeout(sheetClickResetRef.current)
    sheetDragRef.current = null
  }, [])

  useEffect(() => {
    const token = app.session?.token
    streamRef.current?.close()
    if (streamRecoveryTimerRef.current !== null) {
      window.clearTimeout(streamRecoveryTimerRef.current)
      streamRecoveryTimerRef.current = null
    }
    if (!token || !projectReadiness.ready) {
      streamGenerationRef.current = null
      streamTokenRef.current = null
      streamRetryCountRef.current = 0
      streamDisconnectNoticeRef.current = false
      streamExhaustedNoticeRef.current = false
      return undefined
    }
    const tokenChanged = streamTokenRef.current !== token
    if (tokenChanged) {
      streamTokenRef.current = token
      streamRetryCountRef.current = 0
      streamDisconnectNoticeRef.current = false
      streamExhaustedNoticeRef.current = false
    }
    const generation = createWorkspaceStreamGeneration(token, selectedProjectID, streamRetryCountRef.current)
    streamGenerationRef.current = generation
    const source = new EventSource(userApi.taskStreamUrl(token, selectedProjectID))
    function markStreamHealthy() {
      if (!workspaceStreamEventIsCurrent(generation, streamGenerationRef.current)) return
      markWorkspaceStreamHealthy(generation)
      streamRetryCountRef.current = 0
      streamDisconnectNoticeRef.current = false
      streamExhaustedNoticeRef.current = false
    }
    source.addEventListener('open', () => { markStreamHealthy() })
    source.addEventListener('history', (event) => {
      if (!workspaceStreamEventIsCurrent(generation, streamGenerationRef.current)) return
      const tasks = (JSON.parse((event as MessageEvent).data).map(toTask) as ImageTask[])
        .filter((task) => task.project_id === selectedProjectID)
      markStreamHealthy()
      setRecords((current) => replaceWorkspaceTaskRecords(current, tasks, {
        limit: 20,
        preserveIds: selectedTaskIdRef.current ? [selectedTaskIdRef.current] : [],
      }))
    })
    source.addEventListener('task', (event) => {
      if (!workspaceStreamEventIsCurrent(generation, streamGenerationRef.current)) return
      const next = toTask(JSON.parse((event as MessageEvent).data))
      markStreamHealthy()
      if (isTerminalStatus(next.status) && next.status === 'succeeded' && !completedNoticeRef.current.has(next.id)) {
        completedNoticeRef.current.add(next.id)
        if (selectedProjectID && next.project_id === selectedProjectID) notifyRef.current('success', '任务已完成，结果已同步到历史资产')
        void refreshAccountRef.current()
      }
      if (!selectedProjectID || next.project_id !== selectedProjectID) return
      setRecords((items) => mergeWorkspaceTaskRecords(items, next, {
        limit: 20,
        preserveIds: selectedTaskIdRef.current ? [selectedTaskIdRef.current] : [],
      }))
    })
    let recovering = false
    async function recoverStream() {
      if (recovering || streamRecoveryTimerRef.current !== null || !workspaceStreamRecoveryIsCurrent(generation, streamGenerationRef.current)) return
      recovering = true
      closeWorkspaceStreamGeneration(generation)
      source.close()
      if (streamRef.current === source) streamRef.current = null
      const decision = nextWorkspaceStreamRetry(generation)
      streamRetryCountRef.current = generation.retryCount
      if (!decision.retry) {
        if (!streamExhaustedNoticeRef.current) {
          streamExhaustedNoticeRef.current = true
          notifyRef.current('error', '任务状态连接恢复失败，请稍后刷新页面查看最新结果。')
        }
        return
      }
      if (!streamDisconnectNoticeRef.current) {
        streamDisconnectNoticeRef.current = true
        notifyRef.current('info', '任务状态连接已断开，正在自动恢复。')
      }
      try {
        const tasks = await userApi.listTasks({ project_id: selectedProjectID })
        if (!workspaceStreamRecoveryIsCurrent(generation, streamGenerationRef.current)) return
        setRecords((current) => replaceWorkspaceTaskRecords(current, tasks, {
          limit: 20,
          preserveIds: selectedTaskIdRef.current ? [selectedTaskIdRef.current] : [],
        }))
        setStreamRetryKey((value) => value + 1)
      } catch {
        if (!workspaceStreamRecoveryIsCurrent(generation, streamGenerationRef.current)) return
        recovering = false
        streamRecoveryTimerRef.current = window.setTimeout(() => {
          streamRecoveryTimerRef.current = null
          void recoverStream()
        }, 400 * decision.attempt)
      }
    }
    source.addEventListener('error', () => { void recoverStream() })
    streamRef.current = source
    return () => {
      if (streamRecoveryTimerRef.current !== null) {
        window.clearTimeout(streamRecoveryTimerRef.current)
        streamRecoveryTimerRef.current = null
      }
      source.close()
      if (streamRef.current === source) streamRef.current = null
      closeWorkspaceStreamGeneration(generation)
      if (workspaceStreamRecoveryIsCurrent(generation, streamGenerationRef.current)) streamGenerationRef.current = null
    }
  }, [app.session?.token, projectReadiness.ready, selectedProjectID, streamRetryKey])

  useEffect(() => {
    feedEndRef.current?.scrollIntoView({ block: 'end' })
  }, [records])

  const applyCreationDraft = useCallback((draft: WorkspaceCreationDraft) => {
    if (!capability) return undefined
    let cancelled = false
    let normalized: ReturnType<typeof normalizeWorkspaceCreationDraft>
    try {
      normalized = normalizeWorkspaceCreationDraft(draft, capability)
    } catch (err) {
      pendingCreationDraftRef.current = null
      notifyRef.current('error', errorMessage(err))
      return undefined
    }
    const values = normalized.values
    restoreParametersRef.current = {
      routeModelCode: values.route_model_code,
		sizeMode: values.size_mode === 'auto' ? 'auto' : values.size_mode === 'pixel' ? 'pixel' : 'ratio',
      baseResolution: values.base_resolution,
      aspectRatio: values.aspect_ratio,
      pixelSize: values.pixel_size,
      quality: values.quality,
		outputFormat: values.output_format,
		background: values.background,
      outputCompression: values.output_compression,
      moderation: values.moderation,
      imageCount: values.image_count,
    }
    setPrompt(values.prompt)
    setModel(values.route_model_code)
    setSizeMode(values.size_mode === 'auto' ? 'auto' : values.size_mode === 'pixel' ? 'pixel' : 'ratio')
    setBaseResolution(values.base_resolution)
    setRatio(values.aspect_ratio)
    setPixelSize(values.pixel_size)
    setQuality(values.quality)
	setOutputFormat(values.output_format)
	setBackground(values.background)
    setOutputCompression(values.output_compression)
    setModeration(values.moderation)
    setCount(values.image_count)

    async function restoreReferences() {
      const restored = await Promise.all(values.reference_asset_ids.map(async (assetID) => {
        try {
          return await userApi.getReferenceAsset(assetID)
        } catch {
          return null
        }
      }))
      if (cancelled) return
      const accessible = restored.filter((asset): asset is ReferenceAsset => Boolean(asset))
      setEditRefs(accessible)
      const missing = values.reference_asset_ids.length - accessible.length
      const notices = [...normalized.notices]
      if (missing > 0) notices.push(`${missing} 张引用图片已失效或无权访问，未恢复到创作台。`)
      if (values.task_type === 'image_edit' && !accessible.length) notices.push('没有可访问的引用图片，已按文生图配置恢复。')
      notifyRef.current(notices.length ? 'info' : 'success', notices.length ? `已复用配置：${notices.join(' ')}` : '已完整恢复生成配置')
      pendingCreationDraftRef.current = null
    }
    void restoreReferences()
    return () => { cancelled = true }
  }, [capability])

  useEffect(() => {
    if (!capability) return undefined
    if (pendingCreationDraftRef.current === undefined) {
      pendingCreationDraftRef.current = consumeWorkspaceCreationDraft(window.sessionStorage, window.history)
    }
    const draft = pendingCreationDraftRef.current
    if (!draft) return undefined
    return applyCreationDraft(draft)
  }, [applyCreationDraft, capability])

  function reuseTaskConfig(task: ImageTask) {
    try {
      void applyCreationDraft(workspaceCreationDraftFromSnapshot(task))
    } catch (err) {
      app.notify('error', errorMessage(err))
    }
  }

  useEffect(() => {
    if (capability) {
      const nextModels = selectableModels(capability, taskType)
      const preferredModel = restoreParametersRef.current?.routeModelCode
      if (preferredModel && nextModels.some((item) => item.code === preferredModel)) {
        if (model !== preferredModel) setModel(preferredModel)
        return
      }
      if (!nextModels.some((item) => item.code === model)) setModel(nextModels[0]?.code ?? '')
    }
  }, [taskType, capability, model])

  const availableModels = useMemo(() => capability ? selectableModels(capability, taskType) : [], [capability, taskType])
  const rawSelectedModel = useMemo(() => availableModels.find((item) => item.code === model), [availableModels, model])
  const selectedModel = useMemo(() => workspaceModelForTask(rawSelectedModel, taskType), [rawSelectedModel, taskType])
	const sizeModes = useMemo(() => workspaceSizeModeOptions(selectedModel), [selectedModel])
  const baseResolutionOptionsForModel = useMemo(() => baseResolutionOptions(selectedModel), [selectedModel])
	const ratios = useMemo(() => workspaceRatioOptions(selectedModel, capability?.aspect_ratios ?? []), [selectedModel, capability])
	const pixelSizes = useMemo(() => workspacePixelOptions(selectedModel, capability?.pixel_sizes ?? []), [selectedModel, capability])
	const outputOptions = useMemo(() => workspaceOutputOptions(selectedModel), [selectedModel])
	const backgroundOptions = useMemo(() => workspaceBackgroundOptions(selectedModel), [selectedModel])
  const compressionVisible = workspaceCompressionVisible(selectedModel, outputFormat)
  const customSizeSupported = workspaceCustomSizeSupported(selectedModel) && sizeModes.includes('pixel')
	const customSizeNormalization = useMemo(() => normalizeWorkspaceCustomSize(customWidth, customHeight, selectedModel), [customWidth, customHeight, selectedModel])
  const effectivePixelSize = pixelSelection === 'custom' && customSizeSupported
    ? customSizeNormalization.valid ? customSizeNormalization.size : ''
		: pixelSize
  const customRatioSupported = workspaceCustomRatioSupported(selectedModel) && sizeModes.includes('ratio')
  const effectiveRatio = ratio === 'custom' ? customRatio : ratio
  const sizeParameterError = workspaceSizeParameterError({ sizeMode, pixelSelection, customWidth, customHeight, ratio, customRatio, customRatioSupported, model: selectedModel })
  const ratioPixelEstimate = useMemo(
    () => workspaceRatioPixelEstimate(baseResolution, effectiveRatio, selectedModel?.auto_base_resolution_by_task_type?.[taskType]),
    [baseResolution, effectiveRatio, selectedModel, taskType],
  )

  useEffect(() => {
    if (!capability || !selectedModel) return
    const restoreParameters = restoreParametersRef.current
    const waitingForPreferredModel = Boolean(
      restoreParameters?.routeModelCode
      && availableModels.some((item) => item.code === restoreParameters.routeModelCode)
      && selectedModel?.code !== restoreParameters.routeModelCode,
    )
    if (waitingForPreferredModel) return

    setSizeMode((current) => chooseWorkspaceSizeMode(sizeModes, restoreParameters?.sizeMode, current))
    setBaseResolution(matchWorkspaceCapabilityOption(baseResolutionOptionsForModel, restoreParameters?.baseResolution) ?? baseResolutionOptionsForModel[0] ?? '')
	const restoredRatio = restoreParameters?.aspectRatio
	if (customRatioSupported && restoredRatio && !ratios.includes(restoredRatio) && workspaceCustomRatioValid(restoredRatio)) {
		setRatio('custom')
		setCustomRatio(restoredRatio)
	} else {
		setRatio(restoredRatio && ratios.includes(restoredRatio) ? restoredRatio : ratios[0] ?? '')
		setCustomRatio('')
	}
    const restoredPixelSize = restoreParameters?.pixelSize
    const restoredCustomDimensions = customSizeSupported && restoredPixelSize && !pixelSizes.includes(restoredPixelSize)
      ? parsePixelDimensions(restoredPixelSize)
      : null
    if (restoredCustomDimensions) {
      setPixelSelection('custom')
      setCustomWidth(String(restoredCustomDimensions.width))
      setCustomHeight(String(restoredCustomDimensions.height))
    } else {
      setPixelSelection('preset')
      setCustomWidth('')
      setCustomHeight('')
    }
    setPixelSize(restoredPixelSize && pixelSizes.includes(restoredPixelSize) ? restoredPixelSize : pixelSizes[0] ?? '')
	setQuality(matchWorkspaceCapabilityOption(outputOptions.quality, restoreParameters?.quality) ?? outputOptions.quality[0] ?? '')
	setOutputFormat(matchWorkspaceCapabilityOption(outputOptions.outputFormat, restoreParameters?.outputFormat) ?? outputOptions.outputFormat[0] ?? '')
	setBackground(workspaceBackgroundForFormat(selectedModel, restoreParameters?.background ?? background, matchWorkspaceCapabilityOption(outputOptions.outputFormat, restoreParameters?.outputFormat) ?? outputOptions.outputFormat[0] ?? ''))
    setOutputCompression(restoreParameters?.outputCompression ?? 100)
	setModeration(matchWorkspaceCapabilityOption(outputOptions.moderation, restoreParameters?.moderation) ?? outputOptions.moderation[0] ?? '')
    setCount((current) => normalizeWorkspaceImageCount(restoreParameters?.imageCount ?? current))
    restoreParametersRef.current = null
  }, [taskType, capability, selectedModel, availableModels, sizeModes, baseResolutionOptionsForModel, ratios, pixelSizes, outputOptions, customSizeSupported, customRatioSupported])

  useEffect(() => {
    if (!selectedModel) return
    const normalized = normalizeWorkspaceOutputParameters(selectedModel, { quality, outputFormat, outputCompression, moderation })
    if (normalized.quality !== quality) setQuality(normalized.quality)
    if (normalized.outputFormat !== outputFormat) setOutputFormat(normalized.outputFormat)
    if (normalized.outputCompression !== outputCompression) setOutputCompression(normalized.outputCompression)
	if (normalized.moderation !== moderation) setModeration(normalized.moderation)
  }, [selectedModel, quality, outputFormat, outputCompression, moderation])

	useEffect(() => {
		const next = workspaceBackgroundForFormat(selectedModel, background, outputFormat)
		if (next !== background) setBackground(next)
	}, [selectedModel, background, outputFormat])

	const sizeParametersReady = sizeMode === 'auto' ? true : sizeMode === 'pixel'
		? Boolean(effectivePixelSize && (pixelSelection === 'custom' ? customSizeSupported && customSizeNormalization.valid : pixelSizes.includes(pixelSize)))
		: Boolean(baseResolution && effectiveRatio && baseResolutionOptionsForModel.includes(baseResolution) && (ratios.includes(effectiveRatio) || customRatioSupported && workspaceCustomRatioValid(effectiveRatio)))

  const parametersReady = Boolean(
    selectedModel
    && count
	&& sizeModes.includes(sizeMode)
	&& sizeParametersReady
	&& outputOptions.quality.includes(quality)
	&& outputOptions.outputFormat.includes(outputFormat)
	&& outputOptions.moderation.includes(moderation)
	&& (!background || backgroundOptions.includes(background))
	&& (!compressionVisible || (outputCompression >= 1 && outputCompression <= 100))
    && count >= 1
    && requiredReferencesReady,
  )

  const estimatePayload = useMemo<EstimateRequest>(() => ({
		task_type: taskType,
		route_model_code: model,
		size_mode: sizeMode,
		...(sizeMode === 'ratio' ? { base_resolution: baseResolution, aspect_ratio: effectiveRatio } : {}),
		...(sizeMode === 'pixel' ? { pixel_size: effectivePixelSize } : {}),
	quality,
	output_format: outputFormat,
	...(background ? { background } : {}),
	output_compression: compressionVisible ? outputCompression : 100,
	moderation,
    image_count: count,
    reference_asset_ids: taskType === 'image_edit' ? editRefs.map((item) => item.id) : [],
  }), [taskType, model, sizeMode, baseResolution, effectiveRatio, quality, outputFormat, background, compressionVisible, outputCompression, moderation, effectivePixelSize, count, editRefs])
  const estimateKey = useMemo(() => workspaceEstimateKey(estimatePayload), [estimatePayload])
  const currentEstimate = currentWorkspaceEstimate(estimateKey, estimateSnapshot)
  const estimate = currentEstimate.estimate
  const estimateError = currentEstimate.error
  const displayedResolvedSize = workspaceDisplayedResolvedSize(estimateKey, estimateSnapshot, ratioPixelEstimate)

  useEffect(() => {
    if (!capability || !model || !parametersReady) {
      return undefined
    }
    let cancelled = false
    const requestedKey = estimateKey
    const timer = window.setTimeout(async () => {
      try {
        setEstimateSnapshot({ key: requestedKey, estimate: null, error: '' })
        const nextEstimate = await userApi.estimate(estimatePayload)
        if (!cancelled) {
          setEstimateSnapshot({ key: requestedKey, estimate: nextEstimate, error: '' })
        }
      } catch (err) {
        if (!cancelled) {
          const message = generationParameterErrorMessage(err)
          setEstimateSnapshot({ key: requestedKey, estimate: null, error: message })
          notifyRef.current('error', message)
        }
      }
    }, 180)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [capability, estimateKey, estimatePayload, model, parametersReady])

  const referencePolicy = useMemo(() => referenceImagePolicy(capability), [capability])
  const referenceAccept = useMemo(() => referenceImageAccept(referencePolicy), [referencePolicy])
  const maxReferenceUploadBytes = referencePolicy.maxBytes
  const maxReferenceUploadLabel = formatFileSize(maxReferenceUploadBytes)
  const referenceFormatLabel = referencePolicy.allowedFormats.map((format) => format.toUpperCase()).join('、')
  const maxReferenceImages = workspaceReferenceMaximum(selectedModel?.max_reference_image_count)
  const editRemainingLimit = remainingReferenceCapacity(maxReferenceImages, editRefs.length)

  useEffect(() => {
    const onMediaAssetChanged = (event: Event) => {
      const detail = (event as CustomEvent<{ assetID?: string; mediaType?: string }>).detail
      if (!detail?.assetID || detail.mediaType !== 'image') return
      void userApi.importReferenceAssetsFromMedia([detail.assetID]).then((aliases) => {
        if (!aliases.length) return
        setEditRefs((items) => mergeReferenceAssets(items, aliases, maxReferenceImages))
      }).catch(() => undefined)
    }
    window.addEventListener(MEDIA_ASSETS_CHANGED_EVENT, onMediaAssetChanged)
    return () => window.removeEventListener(MEDIA_ASSETS_CHANGED_EVENT, onMediaAssetChanged)
  }, [maxReferenceImages])
  const remainingGalleryImportLimit = editRemainingLimit
  const promptTemplateParse = useMemo(() => parsePromptTemplate(prompt), [prompt])
  const promptReferenceBindings = useMemo(() => buildPromptReferenceBindings(prompt, editRefs), [editRefs, prompt])
  const promptVariableState = useMemo(() => promptVariableValidation(prompt, promptVariables), [prompt, promptVariables])
  const promptExpandedLength = useMemo(() => expandedPromptCodePointLength(prompt, promptVariables, promptReferenceBindings.bindings), [prompt, promptReferenceBindings.bindings, promptVariables])
  const promptTemplateIssue = promptTemplateParse.error?.message
    || (promptReferenceBindings.unresolved.length ? `未关联资产：${promptReferenceBindings.unresolved.join('、')}` : '')
    || (promptVariableState.missing.length ? `请填写变量：${promptVariableState.missing.join('、')}` : '')
    || (promptVariableState.tooLong.length ? `变量内容超过长度限制：${promptVariableState.tooLong.join('、')}` : '')
    || (promptExpandedLength > 4000 ? '变量填充后的提示词超过 4000 个字符。' : '')
  const newestTask = records[records.length - 1] ?? null
  const selectedTask = selectedTaskId ? records.find((task) => task.id === selectedTaskId) ?? null : null
  const latestTask = selectedTask ?? newestTask
  const workspaceView = useMemo(() => createWorkspaceViewModel({
    capability,
    taskType,
    referenceCount,
    requiredReferencesReady,
    selectedModelCode: model,
    parametersReady,
    parameterError: sizeParameterError,
    prompt,
    estimatePending: Boolean(capability && model && parametersReady && currentEstimate.pending),
    estimateError,
    estimate,
    busy,
    task: latestTask,
  }), [capability, taskType, referenceCount, requiredReferencesReady, model, parametersReady, sizeParameterError, prompt, estimateError, estimate, busy, latestTask])
  const baseGenerateReadiness = projectReadiness.ready
    ? workspaceView.generate
    : { disabled: true, reason: projectReadiness.reason, showRechargeAction: false }
  const generateReadiness = promptTemplateIssue
    ? { disabled: true, reason: promptTemplateIssue, showRechargeAction: false }
    : baseGenerateReadiness
  const historyTasks = useMemo(() => (
    [...records].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  ), [records])

  async function refreshWorkspaceImage(imageId: string) {
    const projection = await mediaAccess.preview({ kind: 'image', scope: 'private', id: imageId })
    const refreshedURL = userApi.imageAssetUrl(projection.url, app.session?.token)
    setRecords((tasks) => tasks.map((task) => patchTaskImageAccess(task, imageId, refreshedURL, projection.expires_at)))
    setHistoryTaskDialog((task) => task ? patchTaskImageAccess(task, imageId, refreshedURL, projection.expires_at) : null)
    setPreviewImage((current) => {
      if (current?.mediaResource?.kind !== 'image' || current.mediaResource.id !== imageId) return current
      return {
        ...current,
        url: refreshedURL,
        downloadUrl: refreshedURL,
        mediaExpiresAt: projection.expires_at,
        detailImage: current.detailImage ? { ...current.detailImage, url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at } : current.detailImage,
      }
    })
    return refreshedURL
  }

  async function refreshWorkspaceReference(assetId: string) {
    const projection = await mediaAccess.preview({ kind: 'reference', scope: 'private', id: assetId })
    const refreshedURL = userApi.imageAssetUrl(projection.url, app.session?.token)
    setEditRefs((assets) => assets.map((asset) => asset.id === assetId ? { ...asset, preview_url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at } : asset))
    setRecords((tasks) => tasks.map((task) => patchTaskReferenceAccess(task, assetId, refreshedURL, projection.expires_at)))
    setHistoryTaskDialog((task) => task ? patchTaskReferenceAccess(task, assetId, refreshedURL, projection.expires_at) : null)
    setPreviewImage((current) => current?.mediaResource?.kind === 'reference' && current.mediaResource.id === assetId
      ? { ...current, url: refreshedURL, downloadUrl: refreshedURL, mediaExpiresAt: projection.expires_at }
      : current)
    return refreshedURL
  }

  async function refreshGalleryImportImage(imageId: string) {
    const projection = await mediaAccess.preview({ kind: 'image', scope: 'private', id: imageId })
    const refreshedURL = userApi.imageAssetUrl(projection.url, app.session?.token)
    setGalleryImages((items) => items.map((image) => image.id === imageId
      ? { ...image, url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at }
      : image))
    return refreshedURL
  }

  async function refreshWorkspaceResource(resource: MediaResource) {
    return resource.kind === 'reference'
      ? refreshWorkspaceReference(resource.id)
      : refreshWorkspaceImage(resource.id)
  }

  async function downloadWorkspaceResource(resource: MediaResource, filename: string) {
    try {
      const projection = await mediaAccess.download(resource)
      startMediaDownload(userApi.imageAssetUrl(projection.url, app.session?.token), filename)
    } catch (err) {
      app.notify('error', errorMessage(err))
    }
  }

  async function downloadWorkspaceImage(image: ImageResult) {
    try {
      const projection = await mediaAccess.download({ kind: 'image', scope: 'private', id: image.id })
      startMediaDownload(userApi.imageAssetUrl(projection.url, app.session?.token), workspaceImageFilename(image))
    } catch (err) {
      app.notify('error', errorMessage(err))
    }
  }

  async function openGalleryImport(target: 'edit', promptInsertTarget: 'compact' | 'expanded' | null = null) {
    const remaining = editRemainingLimit
    if (remaining <= 0) {
      pendingPromptAssetInsertRef.current = null
      app.notify('error', '当前模型的参考图数量已达上限，请先移除一张。')
      return
    }
    pendingPromptAssetInsertRef.current = promptInsertTarget
    setGalleryImportTarget(target)
    setGalleryImportFilter(defaultGalleryImportFilter)
    setGalleryImportLoading(true)
    try {
      setGalleryImages(await userApi.listGalleryImages(1, 100, selectedProjectID))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setGalleryImportLoading(false)
    }
  }

  async function confirmGalleryImport(selectedIds: string[]) {
    if (!galleryImportTarget || !selectedIds.length) return
    const limited = limitReferenceSelection(selectedIds, remainingGalleryImportLimit)
    if (limited.rejectedCount > 0) {
      app.notify('error', remainingGalleryImportLimit <= 0
        ? '当前模型的参考图数量已达上限，请先移除一张。'
        : `当前模型最多还能添加 ${remainingGalleryImportLimit} 张，已跳过 ${limited.rejectedCount} 张。`)
    }
    if (!limited.accepted.length) return
    setGalleryImportBusy(true)
    try {
      const assets = await userApi.importReferenceAssetsFromGallery(limited.accepted)
      const promptInsertTarget = pendingPromptAssetInsertRef.current
      const reused = capability ? firstGalleryReferenceReuse(editRefs.length, assets, capability) : null
      setEditRefs((items) => mergeReferenceAssets(items, assets, maxReferenceImages))
      const promptEditorRef = promptInsertTarget === 'expanded' ? expandedPromptEditorRef : promptInsertTarget === 'compact' ? compactPromptEditorRef : null
      for (const asset of assets) {
        const name = asset.name?.trim()
        if (name) promptEditorRef?.current?.insertToken('reference', name)
      }
      if (reused) {
        const values = reused.values
        restoreParametersRef.current = { routeModelCode: values.route_model_code, sizeMode: values.size_mode === 'auto' ? 'auto' : values.size_mode === 'pixel' ? 'pixel' : 'ratio', baseResolution: values.base_resolution, aspectRatio: values.aspect_ratio, pixelSize: values.pixel_size, quality: values.quality, outputFormat: values.output_format, background: values.background, outputCompression: values.output_compression, moderation: values.moderation, imageCount: values.image_count }
        setModel(values.route_model_code)
        setSizeMode(values.size_mode === 'auto' ? 'auto' : values.size_mode === 'pixel' ? 'pixel' : 'ratio')
        setBaseResolution(values.base_resolution)
        setRatio(values.aspect_ratio)
        setPixelSize(values.pixel_size)
        setQuality(values.quality)
        setOutputFormat(values.output_format)
        setBackground(values.background)
        setOutputCompression(values.output_compression)
        setModeration(values.moderation)
        setCount(values.image_count)
      }
      pendingPromptAssetInsertRef.current = null
      setGalleryImportTarget(null)
      app.notify(reused?.notices.length ? 'info' : 'success', galleryImportSuccessMessage(assets.length, reused?.notices))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setGalleryImportBusy(false)
    }
  }

  async function uploadReferenceFiles(files: File[], target: UploadTarget) {
    if (!files.length) return
    const accepted: File[] = []
    const rejected: string[] = []
    for (const file of files) {
      const validation = validateReferenceImageFile(file, referencePolicy)
      if (validation.ok) accepted.push(file)
      else rejected.push(validation.message ?? `${file.name} 不符合图片上传要求。`)
    }
    if (rejected.length) {
      app.notify('error', rejected.length === 1 ? rejected[0] : `${rejected.length} 个文件不符合上传要求，已跳过。${rejected[0] ? ` ${rejected[0]}` : ''}`)
    }
    const remaining = editRemainingLimit
    const limited = limitReferenceSelection(accepted, remaining)
    if (limited.rejectedCount > 0) {
      app.notify('error', remaining <= 0
        ? '当前模型的参考图数量已达上限，请先移除一张。'
        : `当前模型最多还能添加 ${remaining} 张，已跳过 ${limited.rejectedCount} 张。`)
    }
    if (!limited.accepted.length) {
      return
    }
    setBusy(true)
    try {
      const uploaded = await Promise.all(limited.accepted.map((file) => userApi.uploadReferenceAsset(file)))
      setEditRefs((items) => mergeReferenceAssets(items, uploaded, maxReferenceImages))
      app.notify('success', `已上传 ${uploaded.length} 张参考图`)
    } catch (err) {
      app.notify('error', uploadErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function uploadReference(event: ChangeEvent<HTMLInputElement>, target: UploadTarget) {
    const files = Array.from(event.target.files ?? [])
    await uploadReferenceFiles(files, target)
    event.target.value = ''
  }

  function handleUploadDrag(target: UploadTarget, active: boolean) {
    setDragUpload((current) => current[target] === active ? current : { ...current, [target]: active })
  }

  function uploadDropBindings(target: UploadTarget) {
    return {
      onDragEnter: (event: React.DragEvent<HTMLElement>) => {
        event.preventDefault()
        handleUploadDrag(target, true)
      },
      onDragOver: (event: React.DragEvent<HTMLElement>) => {
        event.preventDefault()
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
        handleUploadDrag(target, true)
      },
      onDragLeave: (event: React.DragEvent<HTMLElement>) => {
        event.preventDefault()
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          handleUploadDrag(target, false)
        }
      },
      onDrop: (event: React.DragEvent<HTMLElement>) => {
        event.preventDefault()
        handleUploadDrag(target, false)
        void uploadReferenceFiles(Array.from(event.dataTransfer.files ?? []), target)
      },
    }
  }

  async function createTask() {
    if (!projectReadiness.ready) {
      app.notify('error', projectReadiness.reason)
      return
    }
    if (!requiredReferencesReady) {
      app.notify('error', WORKSPACE_REFERENCE_REQUIRED_MESSAGE)
      return
    }
    if (generateReadiness.disabled) {
      app.notify('error', generateReadiness.reason)
      return
    }
    const activeTaskType = taskType
    const editSourceAssets = activeTaskType === 'image_edit' ? [...editRefs] : []
    const submissionProject = projectSelectionRef.current
    setBusy(true)
    try {
      const task = await userApi.createTask({
        ...estimatePayload,
        project_id: submissionProject.projectID,
        prompt,
        reference_bindings: promptReferenceBindings.bindings,
        prompt_variables: buildPromptVariableInputs(prompt, promptVariables),
        capability_version: estimate?.capability_version,
        idempotency_key: crypto.randomUUID(),
      })
      if (!workspaceSubmissionIsCurrent(submissionProject, projectSelectionRef.current)) {
        void app.refreshAccount()
        return
      }
      const nextTask = editSourceAssets.length ? { ...task, reference_assets: editSourceAssets } : task
      setRecords((items) => mergeWorkspaceTaskRecords(items, nextTask, {
        limit: 20,
        preserveIds: selectedTaskIdRef.current ? [selectedTaskIdRef.current] : [],
      }))
      setSelectedTaskId(nextTask.id)
      app.navigate('genpic', { taskId: nextTask.id })
      setParametersExpanded(false)
      if (activeTaskType === 'image_edit') {
        setPrompt('')
        setPromptVariables({})
        setEditRefs([])
      }
      app.notify('info', '任务已进入队列，正在等待实时状态')
      await app.refreshAccount()
    } catch (err) {
      if (!workspaceSubmissionIsCurrent(submissionProject, projectSelectionRef.current)) return
      if (err instanceof ApiError && err.code === 'PROMPT_TEMPLATE_STALE') {
        const refreshedResults = await Promise.allSettled(editRefs.map((asset) => userApi.getReferenceAsset(asset.id)))
        let refreshFailed = false
        const refreshedAssets = refreshedResults.flatMap((result, index) => {
          if (result.status === 'fulfilled') return [result.value]
          if (result.reason instanceof ApiError && result.reason.status === 404) return []
          refreshFailed = true
          return [editRefs[index]]
        })
        setEditRefs(refreshedAssets)
        app.notify(
          'error',
          refreshFailed
            ? '引用资产名称已变化，部分资产状态刷新失败。请刷新页面并检查红色标记后重新确认创作。'
            : '引用资产名称已变化，已刷新资产状态。请检查红色标记后重新确认创作。',
        )
        return
      }
      if (err instanceof ApiError && err.code === 'capability_changed') {
        try {
          const nextCapability = await userApi.getCapabilities()
          setCapability(nextCapability)
          setEstimateSnapshot({ key: '', estimate: null, error: '' })
        } catch (refreshError) {
          setEstimateSnapshot({ key: '', estimate: null, error: errorMessage(refreshError) })
        }
      }
      app.notify('error', generationParameterErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function startPromptOptimization() {
    const value = prompt.trim()
    if (Array.from(value).length < 8) {
      app.notify('error', '提示词至少需要 8 个字符后才能优化')
      return
    }
    const next = beginPromptOptimization(promptOptimization, value)
    if (next === promptOptimization) return
    setPromptOptimization(next)
    try {
      const estimateResult = await userApi.estimatePromptOptimization(value)
      setPromptOptimization(receivePromptEstimate(next, estimateResult))
    } catch (cause) {
      setPromptOptimization(failPromptOptimization(next, errorMessage(cause)))
    }
  }

  async function confirmOptimization() {
    if (!promptOptimization.estimate) return
    const next = confirmPromptOptimization(promptOptimization)
    setPromptOptimization(next)
    try {
      const result = await userApi.optimizePrompt(next.originalPrompt, promptOptimization.estimate.quote)
      setPromptOptimization(receivePromptOptimization(next, result))
    } catch (cause) {
      setPromptOptimization(failPromptOptimization(next, errorMessage(cause)))
    }
  }

  function applyOptimization() {
    const applied = applyOptimizedPrompt(promptOptimization)
    setPrompt(applied.prompt)
    setPromptOptimization(applied.state)
    app.notify('success', '优化后的提示词已应用')
  }

  function undoOptimization() {
    const undone = undoPromptOptimization(promptOptimization, prompt)
    setPrompt(undone.prompt)
    setPromptOptimization(undone.state)
    app.notify('success', '已恢复优化前的提示词')
  }

  function cancelOptimization() {
    setPromptOptimization(initialPromptOptimizationState())
  }

  async function applyAsEditSource(image: ImageResult) {
    const addition = singleReferenceAddition(image, maxReferenceImages, editRefs.length)
    if (!addition.item) {
      app.notify('error', '当前模型的参考图数量已达上限，请先移除一张。')
      return
    }
    setBusy(true)
    try {
      const imported = await userApi.importReferenceAssetsFromGallery([addition.item.id])
      if (!imported.length) throw new Error('图片导入失败，请稍后重试。')
      setEditRefs((items) => mergeReferenceAssets(items, imported, maxReferenceImages))
      app.notify('success', '已加入图片编辑')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  function removeEditAsset(asset: ReferenceAsset) {
    setEditRefs((items) => items.filter((item) => (
      asset.id ? item.id !== asset.id : item.preview_url !== asset.preview_url
    )))
  }

  function beginRenameReference(asset: ReferenceAsset) {
    setRenamingReferenceID(asset.id)
    setRenamingReferenceName(asset.name)
  }

  async function saveReferenceName(asset: ReferenceAsset) {
    const nextName = renamingReferenceName.trim()
    if (!nextName || nextName === asset.name) {
      setRenamingReferenceID(null)
      return
    }
    setRenamingReferenceBusy(true)
    try {
      const renamed = await userApi.renameReferenceAsset(asset.id, nextName)
      setEditRefs((items) => items.map((item) => item.id === asset.id ? renamed : item))
      setPrompt((current) => renamePromptReference(current, asset.name, renamed.name))
      setRenamingReferenceID(null)
      app.notify('success', '资产名称已更新')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setRenamingReferenceBusy(false)
    }
  }

  function updateImageCount(value: number) {
    setCount(normalizeWorkspaceImageCount(value))
  }

  function selectRecentTask(task: ImageTask) {
    const interaction = workspaceTaskHistoryInteraction({ surface: 'recent', status: task.status, resultCount: task.results.length })
    if (interaction.selectTask) setSelectedTaskId(task.id)
    if (interaction.navigateHash) app.navigate('genpic', { taskId: task.id })
    if (interaction.outputTab === 'current') setOutputTab('current')
    setHistoryTaskDialog(null)
    setHistoryPreviewReturnTarget(null)
  }

  function openHistoryTaskDialog(task: ImageTask) {
    const interaction = workspaceTaskHistoryInteraction({ surface: 'history', status: task.status, resultCount: task.results.length })
    if (interaction.openDialog) {
      setHistoryPreviewReturnTarget(null)
      setHistoryTaskDialog(task)
    }
  }

  function openHistoryPreview(image: ImagePreviewPayload) {
    if (historyTaskDialog && image.mediaResource?.kind === 'image') {
      setHistoryPreviewReturnTarget({ taskId: historyTaskDialog.id, imageId: image.mediaResource.id })
    }
    setPreviewImage(image)
  }

  function closeHistoryTaskDialog() {
    setHistoryPreviewReturnTarget(null)
    setHistoryTaskDialog(null)
  }

  function handleSheetPointerDown(event: React.PointerEvent<HTMLButtonElement>) {
    if (!compactViewport || (event.pointerType === 'mouse' && event.button !== 0)) return
    sheetDragRef.current = { pointerId: event.pointerId, startY: event.clientY, dragged: false }
    event.currentTarget.setPointerCapture(event.pointerId)
    setSheetDragOffset(0)
  }

  function handleSheetPointerMove(event: React.PointerEvent<HTMLButtonElement>) {
    const drag = sheetDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    const deltaY = event.clientY - drag.startY
    if (Math.abs(deltaY) > 4) drag.dragged = true
    setSheetDragOffset(workspaceSheetDragOffset(parametersExpanded, deltaY))
  }

  function releaseSheetPointer(event: React.PointerEvent<HTMLButtonElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    sheetDragRef.current = null
    setSheetDragOffset(0)
  }

  function handleSheetPointerUp(event: React.PointerEvent<HTMLButtonElement>) {
    const drag = sheetDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    const deltaY = event.clientY - drag.startY
    if (drag.dragged) {
      suppressSheetClickRef.current = true
      if (sheetClickResetRef.current !== null) window.clearTimeout(sheetClickResetRef.current)
      sheetClickResetRef.current = window.setTimeout(() => {
        suppressSheetClickRef.current = false
        sheetClickResetRef.current = null
      }, 0)
    }
    setParametersExpanded(workspaceSheetSnap(parametersExpanded, deltaY))
    releaseSheetPointer(event)
  }

  function handleSheetPointerCancel(event: React.PointerEvent<HTMLButtonElement>) {
    const drag = sheetDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    releaseSheetPointer(event)
  }

  function toggleParametersExpanded() {
    if (suppressSheetClickRef.current) {
      suppressSheetClickRef.current = false
      return
    }
    setParametersExpanded((expanded) => !expanded)
  }

  const sheetDragStyle = compactViewport && sheetDragOffset !== 0
    ? { transform: `translate3d(0, ${sheetDragOffset}px, 0)`, transition: 'none' }
    : undefined

  const modelControls = (
    <div className="cr-group">
      <div className="cr-group__head">
        <h4 aria-label="模型分组">模型</h4>
        {availableModels.length ? <span>{availableModels.length} 可用</span> : null}
      </div>
      {loading && !capability ? <LoadingState label="正在加载可用模型..." /> : null}
      {!loading && availableModels.length ? (
        <ModelGroupSelect options={availableModels} groups={capability?.route_model_groups} value={model} onChange={setModel} />
      ) : null}
      {!loading && !availableModels.length ? <EmptyState title="平台模型配置中" detail={publicUnavailableReason(capability?.unavailable_reason)} /> : null}
    </div>
  )

  const referenceControls = (
    <div className="cr-group">
      <div className="cr-group__head">
        <h4>参考图<span className="cr-group__note">图片编辑来源</span></h4>
        <div className="cr-ref-head">
          <span>{editRefs.length} / {maxReferenceImages}</span>
          <button type="button" disabled={busy || editRemainingLimit <= 0} onClick={() => void openGalleryImport('edit')}>从资产导入</button>
        </div>
      </div>
      <div className="u-ref-grid">
        {editRefs.map((asset) => (
          <div key={asset.id || asset.preview_url} className="u-ref-item group">
            <div className="u-ref-thumb">
              <ReferenceAssetPreview
                asset={asset}
                accessToken={app.session?.token}
                onMediaRefresh={() => refreshWorkspaceReference(asset.id)}
              />
              <button type="button" className={workspaceClasses.refRemove} title="移除编辑图片" aria-label={`移除 ${asset.name || '参考图'}`} onClick={() => removeEditAsset(asset)}><CloseGlyph /></button>
            </div>
            <div className="reference-name-row">
              <span title={asset.name}>{asset.name}</span>
              <button type="button" title="重命名资产" aria-label={`重命名资产 ${asset.name}`} onClick={() => beginRenameReference(asset)}><Pencil size={13} /></button>
            </div>
          </div>
        ))}
        <label
          className={cn('u-refdrop cr-refdrop', dragUpload.edit && 'is-drag')}
          role="button"
          tabIndex={0}
          aria-label="上传参考图"
          aria-disabled={busy || editRemainingLimit <= 0}
          onKeyDown={(event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return
            event.preventDefault()
            event.currentTarget.querySelector<HTMLInputElement>('input[type="file"]')?.click()
          }}
          {...uploadDropBindings('edit')}
        >
          <input className="hidden" type="file" accept={referenceAccept} multiple disabled={busy || editRemainingLimit <= 0} onChange={(event) => uploadReference(event, 'edit')} />
          <UploadGlyph />
          <span>上传</span>
        </label>
        <button type="button" className="u-refdrop cr-refdrop" disabled={busy || editRemainingLimit <= 0} onClick={() => void openGalleryImport('edit')}>
          <ImportGlyph />
          <span>从资产</span>
        </button>
      </div>
      <p className="u-meta">{maxReferenceUploadLabel ? `单张最大 ${maxReferenceUploadLabel} · ` : ''}支持 {referenceFormatLabel}</p>
    </div>
  )

  const promptControls = (
    <div className="cr-group">
      <div className="cr-group__head">
        <label className="u-label">提示词</label>
        <span>{promptExpandedLength} / 4000</span>
      </div>
      <div className="u-prompt-box cr-prompt">
        <div className="u-prompt-box__head">
          <span className="u-meta">支持 @资产 与 $变量</span>
          <div className="u-prompt-box__tools">
            <PromptEditorActions
              optimizing={promptOptimization.stage === 'estimating' || promptOptimization.stage === 'optimizing'}
              canUndo={promptOptimization.stage === 'applied'}
              onExpand={() => setPromptExpanded(true)}
              onOptimize={() => void startPromptOptimization()}
              onUndo={undoOptimization}
            />
          </div>
        </div>
        <PromptTemplateEditor
          ref={compactPromptEditorRef}
          value={prompt}
          assets={editRefs}
          variables={promptVariables}
          accessToken={app.session?.token}
          disabled={busy || promptExpanded}
          onChange={setPrompt}
          onAddAsset={() => { void openGalleryImport('edit', 'compact') }}
          onPasteFiles={(files) => { window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail: { files, projectID: projects.selectedProjectID } })) }}
        />
        <div className="u-prompt-box__foot">
          <span>@ 插入资产 · $ 插入变量</span>
          <span>{promptTemplateIssue ? '请检查提示词标记' : '◈ 预估随参数更新'}</span>
        </div>
      </div>
      <PromptVariableForm template={prompt} values={promptVariables} disabled={busy} onChange={(name, value) => setPromptVariables((current) => ({ ...current, [name]: value }))} />
      {promptTemplateIssue ? <span className="cr-field-error" role="status">{promptTemplateIssue}</span> : null}
    </div>
  )

  const sizeControls = selectedModel ? (
    <div className="cr-group">
      <div className="cr-group__head">
        <h4>尺寸</h4>
        {sizeMode === 'ratio' && displayedResolvedSize ? <span>{displayedResolvedSize}</span> : null}
      </div>
      <label className="u-label">尺寸模式</label>
      {sizeModes.length > 1 ? (
        <div className="u-segment" role="group" aria-label="尺寸模式">
          {sizeModes.map((value) => (
            <button
              key={value}
              type="button"
              className={cn(sizeMode === value && 'is-active')}
              aria-pressed={sizeMode === value}
              onClick={() => setSizeMode(value)}
            >
              {value === 'auto' ? '自动' : value === 'pixel' ? '按像素' : '按比例'}
            </button>
          ))}
        </div>
      ) : null}

      {sizeMode === 'ratio' && baseResolutionOptionsForModel.length ? (
        <div className="u-field">
          <label className="u-label" htmlFor="workspace-base-resolution">基础分辨率</label>
          <select id="workspace-base-resolution" className="u-select" value={baseResolution} onChange={(event) => setBaseResolution(event.target.value)}>
            {baseResolutionOptionsForModel.map((option) => <option key={option} value={option}>{workspaceBaseResolutionLabel(option)}</option>)}
          </select>
        </div>
      ) : null}

      {sizeMode === 'ratio' && ratios.length ? (
        <div className="u-field">
          <label className="u-label">比例</label>
          <div className="u-ratio-grid">
            {ratios.map((r) => (
              <button key={r} type="button" className={cn('u-ratio', ratio === r && 'is-active')} aria-pressed={ratio === r} onClick={() => setRatio(r)}>
                <i aria-hidden="true" style={ratioIconDimensions(r)} />
                {r}
              </button>
            ))}
            {customRatioSupported ? (
              <button type="button" className={cn('u-ratio', ratio === 'custom' && 'is-active')} aria-pressed={ratio === 'custom'} onClick={() => setRatio('custom')}>
                <i aria-hidden="true" style={{ width: 18, height: 18 }} />
                自定义比例
              </button>
            ) : null}
          </div>
        </div>
      ) : null}

      {sizeMode === 'ratio' && customRatioSupported && ratio === 'custom' ? (
        <label className="u-field" htmlFor="workspace-custom-ratio">
          <span className="u-label">自定义比例</span>
          <input id="workspace-custom-ratio" className="u-input" value={customRatio} placeholder="例如 7:5" onChange={(event) => setCustomRatio(event.target.value)} />
          <span role="status" className={sizeParameterError ? 'cr-field-error' : 'u-meta'}>{sizeParameterError || `预计比例：${customRatio}`}</span>
        </label>
      ) : null}

      {sizeMode === 'ratio' && displayedResolvedSize ? (
        <p className="u-meta" role="status">预计输出尺寸：<strong className="num">{displayedResolvedSize}</strong></p>
      ) : null}

      {sizeMode === 'pixel' && (pixelSizes.length || customSizeSupported) ? (
        <div className="u-field">
          <label className="u-label" htmlFor="workspace-pixel-size">像素尺寸</label>
          <select
            id="workspace-pixel-size"
            className="u-select"
            value={pixelSelection === 'custom' ? '__custom__' : pixelSize}
            onChange={(event) => {
              if (event.target.value === '__custom__') {
                setPixelSelection('custom')
                return
              }
              setPixelSelection('preset')
              setPixelSize(event.target.value)
            }}
          >
            {pixelSizes.map((value) => <option key={value} value={value}>{value}</option>)}
            {customSizeSupported ? <option value="__custom__">自定义尺寸</option> : null}
          </select>
          {customSizeSupported && pixelSelection === 'custom' ? (
            <div className="cr-custom-size">
              <div className="cr-inline-pair">
                <label className="u-field" htmlFor="workspace-custom-width">
                  <span className="u-label">Width</span>
                  <input id="workspace-custom-width" className="u-input" type="number" inputMode="numeric" min={selectedModel.min_width ?? 16} max={selectedModel.max_width ?? 3840} step="16" value={customWidth} onChange={(event) => setCustomWidth(event.target.value)} />
                </label>
                <label className="u-field" htmlFor="workspace-custom-height">
                  <span className="u-label">Height</span>
                  <input id="workspace-custom-height" className="u-input" type="number" inputMode="numeric" min={selectedModel.min_height ?? 16} max={selectedModel.max_height ?? 3840} step="16" value={customHeight} onChange={(event) => setCustomHeight(event.target.value)} />
                </label>
              </div>
              <p className="u-meta" role="status">
                {customSizeNormalization.valid ? <>输出尺寸：<strong className="num">{customSizeNormalization.size}</strong></> : <span className="cr-field-error">{sizeParameterError}</span>}
              </p>
              <p className="u-meta">宽高必须为 16 的倍数，并满足当前模型配置区间、1:3 至 3:1 比例及平台像素上限。</p>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  ) : null

  const qualityControls = selectedModel ? (
    <div className="cr-group">
      <div className="cr-group__head"><h4>质量</h4><span>输出与审核</span></div>
      <div className="cr-inline">
        {outputOptions.quality.length ? (
          <div className="u-field">
            <label className="u-label" htmlFor="workspace-quality">质量</label>
            <select id="workspace-quality" className="u-select" value={quality} onChange={(event) => setQuality(event.target.value)}>
              {outputOptions.quality.map((value) => <option key={value} value={value}>{value === 'auto' ? '自动' : value}</option>)}
            </select>
          </div>
        ) : null}
        {outputOptions.outputFormat.length ? (
          <div className="u-field">
            <label className="u-label" htmlFor="workspace-output-format">输出格式</label>
            <select id="workspace-output-format" className="u-select" value={outputFormat} onChange={(event) => setOutputFormat(event.target.value)}>
              {outputOptions.outputFormat.map((value) => <option key={value} value={value}>{value.toUpperCase()}</option>)}
            </select>
          </div>
        ) : null}
      </div>
      {backgroundOptions.length ? (
        <div className="u-field">
          <label className="u-label" htmlFor="workspace-background">背景</label>
          <select id="workspace-background" className="u-select" value={background} onChange={(event) => {
            const value = event.target.value
            if (value === 'transparent' && !['png', 'webp'].includes(outputFormat)) {
              setOutputFormat(outputOptions.outputFormat.find((format) => format === 'png' || format === 'webp') ?? outputFormat)
            }
            setBackground(value)
          }}>
            {backgroundOptions.map((value) => <option key={value} value={value}>{value === 'auto' ? '自动' : value === 'opaque' ? '不透明' : '透明'}</option>)}
          </select>
        </div>
      ) : null}
      {outputOptions.moderation.length > 1 ? (
        <div className="u-field">
          <label className="u-label" htmlFor="workspace-moderation">审核等级</label>
          <select id="workspace-moderation" className="u-select" value={moderation} onChange={(event) => setModeration(event.target.value)}>
            {outputOptions.moderation.map((value) => <option key={value} value={value}>{value === 'auto' ? '自动' : value}</option>)}
          </select>
        </div>
      ) : null}
      {compressionVisible ? (
        <div className="u-field">
          <label className="u-label" htmlFor="workspace-output-compression">压缩质量</label>
          <div className="cr-inline-pair" style={{ alignItems: 'center' }}>
            <input id="workspace-output-compression" type="range" min="1" max="100" value={outputCompression} onChange={(event) => setOutputCompression(Number(event.target.value))} />
            <input
              className="u-input num"
              type="number"
              min="1"
              max="100"
              value={outputCompression}
              aria-label="压缩质量数值"
              onChange={(event) => setOutputCompression(Math.max(1, Math.min(100, Number(event.target.value) || 100)))}
            />
          </div>
        </div>
      ) : null}
    </div>
  ) : null

  const countControls = selectedModel ? (
    <div className="cr-group">
      <div className="cr-group__head"><h4>数量</h4><span>上限 {workspaceTaskImageSafetyLimit}</span></div>
      <div className="cr-count-row">
        <div className="u-stepper">
          <button type="button" aria-label="减少数量" disabled={count <= 1} onClick={() => updateImageCount(count - 1)}>−</button>
          <input
            type="number"
            min={1}
            max={workspaceTaskImageSafetyLimit}
            value={count || 1}
            aria-label="生成数量"
            onChange={(event) => updateImageCount(Number(event.target.value))}
          />
          <button type="button" aria-label="增加数量" disabled={count >= workspaceTaskImageSafetyLimit} onClick={() => updateImageCount(count + 1)}>+</button>
        </div>
        <div className="cr-presets">
          {[1, 2, 4, 8].filter((value) => value <= workspaceTaskImageSafetyLimit).map((value) => (
            <button key={value} type="button" className={cn('cr-preset', count === value && 'is-active')} aria-pressed={count === value} onClick={() => updateImageCount(value)}>{value}</button>
          ))}
        </div>
      </div>
    </div>
  ) : null

  const parameterPanel = (
    <aside className={cn('u-controls', parametersExpanded && 'is-open')} aria-label="创作参数" style={sheetDragStyle} data-sheet-dragging={sheetDragOffset !== 0 || undefined}>
      <header className="u-controls__head">
        <button
          type="button"
          className="cr-panel-head__toggle"
          aria-expanded={parametersExpanded}
          aria-controls="workspace-parameter-controls"
          data-workspace-sheet-handle="true"
          onClick={toggleParametersExpanded}
          onPointerDown={handleSheetPointerDown}
          onPointerMove={handleSheetPointerMove}
          onPointerUp={handleSheetPointerUp}
          onPointerCancel={handleSheetPointerCancel}
        >
          <span className="cr-panel-head__title">
            <strong>创作参数</strong>
            <span className="u-meta">图片 · {taskType === 'image_edit' ? '参考图生图' : '文生图'}</span>
          </span>
          <ChevronUp className={cn('cr-panel-head__chevron', !parametersExpanded && 'is-collapsed')} size={16} aria-hidden="true" />
        </button>
        <span className="u-chip u-chip--mono">{workspaceView.estimate.label}</span>
        {compactViewport && !parametersExpanded ? (
          <button
            type="button"
            className="u-btn u-btn--sm u-btn--primary cr-panel-head__cta"
            aria-label="开始创作（移动端）"
            data-workspace-compact-generate="true"
            disabled={generateReadiness.disabled}
            onClick={() => void createTask()}
          >
            开始
          </button>
        ) : null}
      </header>
      <div
        id="workspace-parameter-controls"
        className="u-controls__body"
        aria-hidden={parametersHidden || undefined}
        inert={parametersHidden ? true : undefined}
      >
        {modelControls}
        {promptControls}
        {referenceControls}
        {sizeControls}
        {qualityControls}
        {countControls}
      </div>
      <div className={cn('u-controls__foot', workspaceClasses.panelSectionFinal)} data-workspace-full-actions="true">
        <div className="u-price-row">
          <span>预估消耗 · {count} 张</span>
          <strong>{workspaceView.estimate.label}</strong>
        </div>
        <p className={cn('u-meta', workspaceView.estimate.state === 'error' && 'cr-inline-error')} role="status">{workspaceView.estimate.detail}</p>
        {estimate && !estimate.sufficient ? (
          <div className="u-banner u-banner--danger" role="alert">
            <div>
              积分不足，还差 {displayPoints(estimate.insufficient_points)}。当前可用 {displayPoints(estimate.balance?.available_points)}。
              <div className="cr-estimate-actions">
                <button className="u-btn u-btn--sm u-btn--primary" type="button" onClick={() => app.navigate('checkout')}>去充值</button>
                <button className="u-btn u-btn--sm" type="button" onClick={() => app.navigate('profile')}>兑换积分</button>
              </div>
            </div>
          </div>
        ) : null}
        {generateReadiness.reason && !generateReadiness.showRechargeAction ? (
          <div className={cn('u-banner', !parametersExpanded && 'max-[760px]:hidden')} role="status">{generateReadiness.reason}</div>
        ) : null}
        <button
          className={cn('u-btn u-btn--primary u-btn--lg u-btn--block', !parametersExpanded && 'max-[760px]:hidden')}
          type="button"
          disabled={generateReadiness.disabled}
          onClick={() => void createTask()}
        >
          {busy ? (
            <>
              <span className={userState.spinner} />
              生成中...
            </>
          ) : '开始创作'}
        </button>
      </div>
    </aside>
  )

  return (
    <div className="u-workbench" data-workspace-layout="creative">
      {compactViewport ? <OverlayPortal>{parameterPanel}</OverlayPortal> : parameterPanel}
      {promptExpanded ? (
        <PromptEditorDialog
          promptEditorRef={expandedPromptEditorRef}
          prompt={prompt}
          assets={editRefs}
          variables={promptVariables}
          accessToken={app.session?.token}
          optimization={promptOptimization}
          onPromptChange={setPrompt}
          onVariableChange={(name, value) => setPromptVariables((current) => ({ ...current, [name]: value }))}
          onAddAsset={() => { void openGalleryImport('edit', 'expanded') }}
          onPasteFiles={(files) => { window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail: { files, projectID: projects.selectedProjectID } })) }}
          onClose={() => { setPromptExpanded(false); window.setTimeout(() => compactPromptEditorRef.current?.focus(), 0) }}
          onOptimize={() => void startPromptOptimization()}
          onConfirm={() => void confirmOptimization()}
          onApply={applyOptimization}
          onCancel={cancelOptimization}
          onUndo={undoOptimization}
          onMediaRefresh={refreshWorkspaceReference}
        />
      ) : null}
      {!promptExpanded && promptOptimization.stage !== 'idle' && promptOptimization.stage !== 'applied' ? (
        <Modal title="优化提示词" onClose={cancelOptimization}>
          <PromptOptimizationPanel state={promptOptimization} onConfirm={() => void confirmOptimization()} onApply={applyOptimization} onCancel={cancelOptimization} />
        </Modal>
      ) : null}
      {renamingReferenceID ? (() => {
        const asset = editRefs.find((item) => item.id === renamingReferenceID)
        if (!asset) return null
        return (
          <Modal title="重命名资产" onClose={() => setRenamingReferenceID(null)}>
            <form className="reference-rename-dialog" onSubmit={(event) => { event.preventDefault(); void saveReferenceName(asset) }}>
              <label htmlFor="reference-rename-input">资产名称</label>
              <input id="reference-rename-input" autoFocus className="u-input" value={renamingReferenceName} maxLength={64} onChange={(event) => setRenamingReferenceName(event.target.value)} />
              <div className="reference-rename-dialog-actions">
                <Button type="button" tone="ghost" disabled={renamingReferenceBusy} onClick={() => setRenamingReferenceID(null)}>取消</Button>
                <Button type="submit" busy={renamingReferenceBusy} disabled={!renamingReferenceName.trim() || renamingReferenceName.trim() === asset.name}>保存</Button>
              </div>
            </form>
          </Modal>
        )
      })() : null}

      <section className="u-canvas" aria-label="创作输出">
        <div className="u-canvas__head">
          <div className="u-segment" role="tablist" aria-label="创作输出">
            <button type="button" role="tab" aria-selected={outputTab === 'current'} className={cn(outputTab === 'current' && 'is-active')} onClick={() => setOutputTab('current')}>当前创作</button>
            <button type="button" role="tab" aria-selected={outputTab === 'history'} className={cn(outputTab === 'history' && 'is-active')} onClick={() => setOutputTab('history')}>历史创作</button>
          </div>
          <div className="cr-canvas-actions">
            {latestTask ? (
              <>
                <span className="u-chip u-chip--mono cr-task-id" title={latestTask.id}>task_{latestTask.id.slice(0, 8)}</span>
                <button className="u-btn u-btn--sm" type="button" onClick={() => {
                  void copyText(latestTask.id).then(() => app.notify('success', '任务 ID 已复制')).catch(() => app.notify('error', '复制失败'))
                }}>复制</button>
                <button className="u-btn u-btn--sm" type="button" disabled={!latestTask.results.length} onClick={() => {
                  const image = latestTask.results[0]
                  if (!image) return
                  setPreviewImage(imagePreviewPayload(image, latestTask, app.profile, app.session?.token))
                }}>打开详情</button>
              </>
            ) : (
              <span className="u-meta">{projectReadiness.ready ? '准备就绪' : projectReadiness.reason}</span>
            )}
          </div>
        </div>

        {initialTaskLoading ? <LoadingState label="正在读取指定任务..." /> : null}
        {initialTaskError ? <ErrorState message={initialTaskError} onRetry={() => setInitialTaskReloadKey((key) => key + 1)} /> : null}

        {latestTask ? <div className="cr-sr-only"><WorkspaceStatusRail task={workspaceView.task} startedAt={latestTask.created_at} finishedAt={latestTask.updated_at} /></div> : null}

        {outputTab === 'current' ? (
          <div className="cr-canvas-panel">
            {latestTask ? <StageRail nodes={workspaceView.task.rail} trailing={`已用 ${formatCompactDuration(taskElapsedMs(latestTask))}`} /> : null}
            {latestTask ? (
              <GenerationOutput
                key={latestTask.id}
                task={latestTask}
                profile={app.profile}
                onCopyPrompt={async () => {
                  await copyText(latestTask.prompt)
                  app.notify('success', '提示词已复制')
                }}
                onUseReference={applyAsEditSource}
                onPreviewImage={setPreviewImage}
                onReuseConfig={reuseTaskConfig}
                onOpenDetail={() => setHistoryTaskDialog(latestTask)}
                onRetryTask={async (task) => {
                  setBusy(true)
                  try {
                    const retry = await userApi.retryTask(task.id)
                    setRecords((items) => mergeWorkspaceTaskRecords(items, retry, {
                      limit: 20,
                      preserveIds: selectedTaskIdRef.current ? [selectedTaskIdRef.current] : [],
                    }))
                    setSelectedTaskId(retry.id)
                    app.navigate('genpic', { taskId: retry.id })
                    app.notify('success', '已重新提交生成任务')
                    await app.refreshAccount()
                  } catch (err) {
                    app.notify('error', errorMessage(err))
                  } finally {
                    setBusy(false)
                  }
                }}
                onDeleteTask={async (task) => {
                  try {
                    await userApi.deleteTask(task.id)
                    setRecords((items) => items.filter((item) => item.id !== task.id))
                    if (selectedTaskIdRef.current === task.id) {
                      setSelectedTaskId(null)
                      app.navigate('genpic')
                    }
                    app.notify('success', '失败记录已删除')
                  } catch (err) {
                    app.notify('error', errorMessage(err))
                  }
                }}
                accessToken={app.session?.token}
                onDownloadImage={downloadWorkspaceImage}
                onImageMediaRefresh={refreshWorkspaceImage}
                onReferenceMediaRefresh={refreshWorkspaceReference}
              />
            ) : (
              <div className="u-stage cr-stage--empty">
                <div className="u-empty cr-stage-empty">
                  <span className="u-empty__icon"><SparkleGlyph /></span>
                  <h3>准备就绪</h3>
                  <p>{projectReadiness.ready ? '在左侧设置参数并输入提示词，点击「开始创作」即可生成画面。' : projectReadiness.reason}</p>
                </div>
              </div>
            )}
            {historyTasks.length ? (
              <RecentHistoryStrip
                tasks={historyTasks}
                activeTaskId={latestTask?.id}
                accessToken={app.session?.token}
                onSelectTask={selectRecentTask}
                onMediaRefresh={refreshWorkspaceImage}
              />
            ) : null}
          </div>
        ) : (
          <div className="cr-canvas-panel">
            <HistoryCreationGrid tasks={historyTasks} profile={app.profile} accessToken={app.session?.token} onPreviewImage={setPreviewImage} onOpenTaskDialog={openHistoryTaskDialog} onMediaRefresh={refreshWorkspaceImage} />
          </div>
        )}
        <div ref={feedEndRef} />

        <ImageDetailModal
          title="图片详情"
          image={previewDetailImage}
          imageUrl={previewImage?.url}
          referenceImages={previewImage?.referenceImages}
          showPublicStats={false}
          onDownload={() => {
            if (!previewImage?.mediaResource) return
            const filename = previewImage.detailImage
              ? workspaceImageFilename(previewImage.detailImage)
              : `mikiko-reference-${previewImage.mediaResource.id}.png`
            void downloadWorkspaceResource(previewImage.mediaResource, filename)
          }}
          onCopyPrompt={async (prompt) => {
            await copyText(prompt)
            app.notify('success', 'Prompt 已复制')
          }}
          actions={previewImage?.creationDraft ? [{
            key: 'reuse',
            label: '复用配置',
            icon: <PublicDetailIcon name="edit" />,
            onClick: () => {
              stageWorkspaceCreationDraft(previewImage.creationDraft!, window.sessionStorage, window.history)
              setPreviewImage(null)
              setHistoryTaskDialog(null)
              setHistoryPreviewReturnTarget(null)
              app.navigate('genpic')
            },
          }] : []}
          previewSourceLabel={previewImage?.source || '创作输出'}
          onMediaRefresh={previewImage?.mediaResource ? () => refreshWorkspaceResource(previewImage.mediaResource!) : undefined}
          onClose={() => setPreviewImage(null)}
        />
        {historyTaskDialog && !previewImage ? (
          <HistoryTaskGalleryModal
            task={historyTaskDialog}
            accessToken={app.session?.token}
            onPreviewImage={openHistoryPreview}
            onImageMediaRefresh={refreshWorkspaceImage}
            onReferenceMediaRefresh={refreshWorkspaceReference}
            onClose={closeHistoryTaskDialog}
          />
        ) : null}
        {galleryImportTarget ? (
          <GalleryImportModal
            images={galleryImages}
            filter={galleryImportFilter}
            loading={galleryImportLoading}
            busy={galleryImportBusy}
            remainingLimit={remainingGalleryImportLimit}
            accessToken={app.session?.token}
            onFilterChange={setGalleryImportFilter}
            onConfirm={(ids) => void confirmGalleryImport(ids)}
            onMediaRefresh={refreshGalleryImportImage}
            onClose={() => {
              pendingPromptAssetInsertRef.current = null
              setGalleryImportTarget(null)
            }}
          />
        ) : null}
      </section>
    </div>
  )
}

function StageRail({ nodes, trailing }: { nodes: WorkspaceProgressNode[]; trailing?: string }) {
  if (!nodes.length) return null
  return (
    <div className="u-stage-rail" aria-hidden="true">
      <span>任务阶段</span>
      <div className="u-stage-rail__steps">
        {nodes.map((node) => (
          <span
            key={node.phase}
            className={cn(
              'u-stage-rail__step',
              node.status === 'done' && 'is-done',
              node.status === 'active' && 'is-now',
              node.status === 'failed' && 'is-failed',
            )}
          >
            <i className="u-stage-rail__dot" />
            {node.label}
          </span>
        ))}
      </div>
      {trailing ? <span className="u-meta">{trailing}</span> : null}
    </div>
  )
}

function ratioIconDimensions(ratio: string) {
  const [rawWidth, rawHeight] = ratio.split(':').map(Number)
  if (!(rawWidth > 0) || !(rawHeight > 0)) return { width: 18, height: 18 }
  const longest = Math.max(rawWidth, rawHeight)
  return {
    width: Math.max(10, Math.round((rawWidth / longest) * 26)),
    height: Math.max(10, Math.round((rawHeight / longest) * 26)),
  }
}

function ReferenceAssetPreview({ asset, accessToken, onClick, onMediaRefresh }: { asset: ReferenceAsset; accessToken?: string | null; onClick?: (url: string, asset: ReferenceAsset) => void; onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void> }) {
  const previewURL = referenceAssetPreviewURL(asset, accessToken)
  if (!previewURL) {
    return <div className="cr-ref-placeholder">无法预览</div>
  }
  if (onClick) {
    return (
      <button className="u-detail__ref" type="button" onClick={() => onClick(previewURL, asset)}>
        <RefreshableMediaImage className="size-full object-cover" src={previewURL} mediaExpiresAt={asset.preview_expires_at} alt={asset.name || '参考图'} onMediaRefresh={onMediaRefresh} />
      </button>
    )
  }
  return <RefreshableMediaImage src={previewURL} mediaExpiresAt={asset.preview_expires_at} alt={asset.name || '参考图'} className="size-full object-cover" onMediaRefresh={onMediaRefresh} />
}

function SparkleGlyph() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 3l1.7 5.2L19 10l-5.3 1.8L12 17l-1.7-5.2L5 10l5.3-1.8L12 3Z" />
      <path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15Z" />
    </svg>
  )
}

function UploadGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 16V4" />
      <path d="m7 9 5-5 5 5" />
      <path d="M20 16v3a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-3" />
    </svg>
  )
}

function ImportGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="4" width="7" height="7" rx="1.5" />
      <rect x="14" y="4" width="7" height="7" rx="1.5" />
      <rect x="3" y="15" width="7" height="5" rx="1.5" />
      <path d="M17.5 15v5" />
      <path d="M15 17.5h5" />
    </svg>
  )
}

function CloseGlyph() {
  return (
    <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  )
}

function GalleryImportModal({ images, filter, loading, busy, remainingLimit, accessToken, onFilterChange, onConfirm, onMediaRefresh, onClose }: {
  images: GalleryImage[]
  filter: GalleryImportFilter
  loading: boolean
  busy: boolean
  remainingLimit: number
  accessToken?: string | null
  onFilterChange: (filter: GalleryImportFilter) => void
  onConfirm: (ids: string[]) => void
  onMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
  onClose: () => void
}) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const options = galleryImportOptions(images)
  const rows = filterGalleryImportImages(images, filter)

  function updateFilter(patch: Partial<GalleryImportFilter>) {
    onFilterChange({ ...filter, ...patch })
  }

  function toggle(id: string) {
    setSelectedIds((items) => {
      const next = new Set(items)
      if (next.has(id)) {
        next.delete(id)
        return next
      }
      if (next.size >= remainingLimit) return next
      next.add(id)
      return next
    })
  }

  return (
    <Modal title="从资产导入参考图" onClose={onClose}>
      <div className="u-toolbar cr-import-toolbar">
        <input className="u-input" type="search" value={filter.query} onChange={(event) => updateFilter({ query: event.target.value })} placeholder="搜索提示词、模型、分组" />
        <select className="u-filter" value={filter.group} onChange={(event) => updateFilter({ group: event.target.value })}>
          <option value="all">全部分组</option>
          {options.groups.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className="u-filter" value={filter.publishStatus} onChange={(event) => updateFilter({ publishStatus: event.target.value })}>
          <option value="all">全部状态</option>
          {options.publishStatuses.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className="u-filter" value={filter.model} onChange={(event) => updateFilter({ model: event.target.value })}>
          <option value="all">全部模型</option>
          {options.models.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className="u-filter" value={filter.ratio} onChange={(event) => updateFilter({ ratio: event.target.value })}>
          <option value="all">全部比例</option>
          {options.ratios.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
      </div>

      {loading ? <LoadingState label="正在读取资产..." /> : null}
      {!loading && !rows.length ? <EmptyState title="没有匹配的资产" detail="换一个搜索词或筛选条件。" /> : null}
      {rows.length ? (
        <div className="u-grid cr-import-grid">
          {rows.map((image) => {
            const selected = selectedIds.has(image.id)
            const disabled = !selected && selectedIds.size >= remainingLimit
            const imageUrl = userApi.imageAssetUrl(image.url || image.download_url || '', accessToken)
            return (
              <button
                key={image.id}
                type="button"
                className={cn('u-card cr-import-tile', selected && 'is-selected')}
                disabled={disabled}
                onClick={() => toggle(image.id)}
              >
                <span className="u-card__media">
                  <span className="cr-import-check" aria-hidden="true">{selected ? '✓' : ''}</span>
                  {imageUrl ? (
                    <RefreshableMediaImage
                      src={imageUrl}
                      mediaExpiresAt={image.preview_expires_at}
                      alt={image.prompt || image.id}
                      onMediaRefresh={() => onMediaRefresh(image.id)}
                    />
                  ) : null}
                </span>
                <span className="u-card__body">
                  <span className="u-card__title">{image.prompt || image.id}</span>
                  <span className="u-card__meta">
                    <span>{image.route_model_code || image.abstract_model || '未知模型'}</span>
                    <span>{image.aspect_ratio || '未知比例'}</span>
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      ) : null}

      <div className="cr-modal-actions">
        <span className="cr-modal-actions__meta">已选 {selectedIds.size} 张，最多还能选择 {remainingLimit} 张</span>
        <Button tone="ghost" onClick={onClose}>取消</Button>
        <Button busy={busy} disabled={!selectedIds.size || selectedIds.size > remainingLimit} onClick={() => onConfirm(Array.from(selectedIds))}>确定</Button>
      </div>
    </Modal>
  )
}

function formatHistoryTime(value?: string) {
  if (!value) return '未知时间'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function RecentHistoryStrip({ tasks, activeTaskId, accessToken, onSelectTask, onMediaRefresh }: {
  tasks: ImageTask[]
  activeTaskId?: string
  accessToken?: string
  onSelectTask: (task: ImageTask) => void
  onMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
}) {
  return (
    <section className="cr-history-section" aria-label="最近创作">
      <div className="cr-section-head">
        <h3 className="u-h3">最近创作</h3>
        <span className="u-meta">{tasks.length} 条</span>
      </div>
      <div className="u-history-strip">
        {tasks.slice(0, 8).map((task) => {
          const card = workspaceTaskCardView(task)
          const image = task.results[0]
          return (
            <button
              key={task.id}
              type="button"
              className={cn('u-history-card cr-history-card', task.id === activeTaskId && 'is-active')}
              aria-current={task.id === activeTaskId ? 'true' : undefined}
              onClick={() => onSelectTask(task)}
            >
              <span className="u-history-card__media">
                {image ? (
                  <RefreshableMediaImage className="size-full object-cover" src={userApi.imageAssetUrl(image.url, accessToken)} mediaExpiresAt={image.preview_expires_at} alt="" onMediaRefresh={() => onMediaRefresh(image.id)} />
                ) : (
                  <span className="cr-history-state">{card.statusLabel}</span>
                )}
              </span>
              <span className="u-history-card__body">
                <strong>{card.taskTypeLabel}</strong>
                <span className="u-meta">{card.statusLabel} · {formatHistoryTime(task.created_at)}</span>
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function taskElapsedMs(task: ImageTask) {
  const started = Date.parse(task.created_at || '')
  const ended = Date.parse(task.updated_at || task.created_at || '')
  if (!Number.isFinite(started) || !Number.isFinite(ended)) return 0
  return Math.max(0, ended - started)
}

function formatCompactDuration(ms: number) {
  const totalSeconds = Math.max(0, Math.round(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours}h${minutes > 0 ? `${minutes}m` : ''}`
  if (minutes > 0) return `${minutes}m${seconds}s`
  return `${seconds}s`
}

function HistoryCreationGrid({ tasks, profile, accessToken, onPreviewImage, onOpenTaskDialog, onMediaRefresh }: {
  tasks: ImageTask[]
  profile?: Pick<UserProfile, 'display_name'> | null
  accessToken?: string | null
  onPreviewImage: (image: ImagePreviewPayload) => void
  onOpenTaskDialog: (task: ImageTask) => void
  onMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
}) {
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const rows = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    return tasks.filter((task) => {
      if (status !== 'all') {
        const matchesStatus = status === 'running' ? !isTerminalStatus(task.status) : task.status === status
        if (!matchesStatus) return false
      }
      if (!normalized) return true
      return task.id.toLowerCase().includes(normalized)
        || task.title.toLowerCase().includes(normalized)
        || task.prompt.toLowerCase().includes(normalized)
    })
  }, [query, status, tasks])

  if (!tasks.length) {
    return <EmptyState title="暂无历史创作" detail="完成一次创作后，任务记录会展示在这里。" />
  }
  return (
    <div className="cr-history-panel">
      <div className="u-toolbar" style={{ margin: 0 }}>
        <input className="u-input" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索提示词或任务 ID" aria-label="搜索历史创作" />
        <select className="u-filter" value={status} onChange={(event) => setStatus(event.target.value)} aria-label="按状态筛选">
          <option value="all">全部状态</option>
          <option value="succeeded">成功</option>
          <option value="partial_failed">部分失败</option>
          <option value="failed">失败</option>
          <option value="running">进行中</option>
        </select>
      </div>
      {rows.length ? (
        <div className="cr-hist">
          {rows.map((task) => (
            <HistoryCreationCard
              key={task.id}
              task={task}
              profile={profile}
              accessToken={accessToken}
              onPreviewImage={onPreviewImage}
              onOpenTaskDialog={onOpenTaskDialog}
              onMediaRefresh={onMediaRefresh}
            />
          ))}
        </div>
      ) : <EmptyState title="没有匹配的创作" detail="换一个搜索词或状态筛选。" />}
    </div>
  )
}

function HistoryCreationCard({ task, profile, accessToken, onPreviewImage, onOpenTaskDialog, onMediaRefresh }: {
  task: ImageTask
  profile?: Pick<UserProfile, 'display_name'> | null
  accessToken?: string | null
  onPreviewImage: (image: ImagePreviewPayload) => void
  onOpenTaskDialog: (task: ImageTask) => void
  onMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
}) {
  const slots = generationSlots(task)
  const imageSlot = slots.find((slot): slot is Extract<typeof slot, { kind: 'image' }> => slot.kind === 'image')
  const requested = Math.max(Number(task.image_count || 1), task.results.length || 0)
  const running = !isTerminalStatus(task.status)
  const allFailed = isTerminalStatus(task.status) && !task.results.length && task.status !== 'succeeded'
  const partialFailed = task.status === 'partial_failed' || (isTerminalStatus(task.status) && task.results.length > 0 && task.results.length < requested)
  const interaction = workspaceTaskHistoryInteraction({ surface: 'history', status: task.status, resultCount: task.results.length })
  const imageUrl = imageSlot ? userApi.imageAssetUrl(imageSlot.image.url, accessToken) : ''
  const downloadUrl = imageSlot ? userApi.imageAssetUrl(imageSlot.image.download_url ?? imageSlot.image.url, accessToken) : ''
  const openPreview = () => {
    if (interaction.openDialog) {
      onOpenTaskDialog(task)
      return
    }
    if (!interaction.openLightbox || !imageSlot || !imageUrl) return
    onPreviewImage({
      url: imageUrl,
      downloadUrl,
      alt: task.title,
      prompt: imageSlot.image.prompt || task.prompt,
      width: imageSlot.image.width,
      height: imageSlot.image.height,
      ratio: task.aspect_ratio,
      model: imageSlot.image.route_model_code || task.route_model_code || task.model_group,
      source: '历史创作',
      creationDraft: workspaceCreationDraftFromSnapshot(task),
      detailImage: projectWorkspaceImageDetail(imageSlot.image, task, profile),
      mediaResource: { kind: 'image', scope: 'private', id: imageSlot.image.id },
      mediaExpiresAt: imageSlot.image.preview_expires_at,
    })
  }

  return (
    <button type="button" className="u-card cr-card-button group" disabled={!interaction.openDialog && !interaction.openLightbox} onClick={openPreview} aria-label={task.prompt || task.title || '未命名创作'}>
      <span className="u-card__media">
        {imageSlot && imageUrl ? (
          <RefreshableMediaImage className={workspaceClasses.historyImage} src={imageUrl} mediaExpiresAt={imageSlot.image.preview_expires_at} alt={task.title} onMediaRefresh={() => onMediaRefresh(imageSlot.image.id)} />
        ) : running ? (
          <span className="cr-card-state">
            <span className={userState.spinner} />
            <span>生成中</span>
          </span>
        ) : (
          <span className="cr-card-state">
            <strong className="cr-card-flag" style={{ position: 'static' }}>!</strong>
            <span>{task.error_message || task.failure_reason || '生成失败'}</span>
          </span>
        )}
        <span className="u-card__tag">{task.results.length}/{requested} 张</span>
        {partialFailed ? <span className="cr-card-flag">部分失败</span> : null}
      </span>
      <span className="u-card__body">
        <span className="u-card__title">{task.prompt || task.title || '未命名创作'}</span>
        <span className="u-card__meta">
          <span>{formatHistoryTime(task.created_at)}</span>
          <span>{formatCompactDuration(taskElapsedMs(task))}</span>
        </span>
      </span>
    </button>
  )
}

function HistoryTaskGalleryModal({ task, accessToken, onPreviewImage, onImageMediaRefresh, onReferenceMediaRefresh, onClose }: {
  task: ImageTask
  accessToken?: string | null
  onPreviewImage: (image: ImagePreviewPayload) => void
  onImageMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
  onReferenceMediaRefresh: (assetId: string) => string | undefined | void | Promise<string | undefined | void>
  onClose: () => void
}) {
  const app = useApp()
  if (!task.results.length) return null
  const requested = Math.max(Number(task.image_count || 1), task.results.length)
  const referenceImages = task.reference_assets.map((asset) => ({
    id: asset.id,
    url: referenceAssetPreviewURL(asset, accessToken),
    alt: asset.name || '原图引用',
    mediaExpiresAt: asset.preview_expires_at,
    onMediaRefresh: asset.id ? () => onReferenceMediaRefresh(asset.id) : undefined,
  })).filter((item) => item.url)

  return (
    <Modal title="历史创作总览" onClose={onClose} className="max-h-[90vh] w-[min(960px,calc(100vw-2rem))] max-w-none overflow-y-auto">
      <div className="u-toolbar cr-detail-summary">
        <span className="u-meta" style={{ flex: '1 1 100%', overflowWrap: 'anywhere' }} title={task.prompt || task.title}>{task.prompt || task.title || '未命名创作'}</span>
        <span className="u-chip u-chip--mono">{task.results.length}/{requested} 张</span>
        <span className="u-chip u-chip--mono">{formatHistoryTime(task.created_at)}</span>
        <span className="u-chip u-chip--mono inline-flex items-center gap-1"><span title={task.id}>任务ID: {task.id}</span><InlineCopyButton text={task.id} label="复制任务ID" /></span>
      </div>
      <div className="u-grid cr-detail-grid">
        {task.results.map((image, index) => {
          const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
          return (
            <button
              key={image.id}
              type="button"
              data-history-image-id={image.id}
              className="u-card cr-card-button group"
              aria-label={`查看第 ${index + 1} 张图片详情`}
              onClick={() => onPreviewImage({
                url: imageUrl,
                downloadUrl: userApi.imageAssetUrl(image.download_url ?? image.url, accessToken),
                alt: image.prompt || `${task.title || '历史创作'} ${index + 1}`,
                prompt: image.prompt || task.prompt,
                width: image.width,
                height: image.height,
                ratio: task.aspect_ratio,
                model: image.route_model_code || task.route_model_code || task.model_group,
                source: '历史创作',
                creationDraft: workspaceCreationDraftFromSnapshot(task),
                detailImage: projectWorkspaceImageDetail(image, task, app.profile),
                mediaResource: { kind: 'image', scope: 'private', id: image.id },
                mediaExpiresAt: image.preview_expires_at,
                referenceImages,
              })}
            >
              <span className="u-card__media">
                <RefreshableMediaImage
                  className="size-full object-cover"
                  src={imageUrl}
                  mediaExpiresAt={image.preview_expires_at}
                  alt={image.prompt || `${task.title || '历史创作'} ${index + 1}`}
                  onMediaRefresh={() => onImageMediaRefresh(image.id)}
                />
                <span className="u-card__tag">第 {index + 1} 张</span>
              </span>
            </button>
          )
        })}
      </div>
    </Modal>
  )
}

function GenerationOutput({ task, profile, onCopyPrompt, onUseReference, onPreviewImage, onRetryTask, onDeleteTask, accessToken, onDownloadImage, onImageMediaRefresh, onReferenceMediaRefresh, onReuseConfig, onOpenDetail }: {
  task: ImageTask
  profile?: Pick<UserProfile, 'display_name'> | null
  onCopyPrompt: () => Promise<void>
  onUseReference: (image: ImageResult) => Promise<void>
  onPreviewImage: (image: ImagePreviewPayload) => void
  onRetryTask: (task: ImageTask) => Promise<void>
  onDeleteTask: (task: ImageTask) => Promise<void>
  accessToken?: string
  onDownloadImage: (image: ImageResult) => Promise<void>
  onImageMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
  onReferenceMediaRefresh: (assetId: string) => string | undefined | void | Promise<string | undefined | void>
  onReuseConfig: (task: ImageTask) => void
  onOpenDetail: () => void
}) {
  const slots = generationSlots(task)
  const activeStage = task.progress_message || task.progress_stage || '等待后端返回任务进度'
  const successSlots = slots.filter((slot): slot is Extract<typeof slot, { kind: 'image' }> => slot.kind === 'image')
  const successImages = successSlots.map((slot) => slot.image)
  const primaryImage = successImages[0] ?? null
  const pendingCount = slots.filter((slot) => slot.kind === 'pending').length
  const failedSlots = slots.filter((slot): slot is Extract<typeof slot, { kind: 'failed' }> => slot.kind === 'failed')
  const [skeletonPhase, setSkeletonPhase] = useState(false)
  const showInitialLoading = !isTerminalStatus(task.status) && !skeletonPhase && !successImages.length
  const batchSlots = slots.length > 1 ? slots : []
  const requested = Math.max(Number(task.image_count || 1), slots.length)

  useEffect(() => {
    if (isTerminalStatus(task.status) || successImages.length > 0) {
      setSkeletonPhase(true)
      return undefined
    }
    setSkeletonPhase(false)
    const timer = window.setTimeout(() => setSkeletonPhase(true), 10000)
    return () => window.clearTimeout(timer)
  }, [task.id, task.status, successImages.length])

  return (
    <article className={cn('cr-output', workspaceClasses.outputResultWrap)}>
      <div className="cr-output__head">
        <h3 className="u-h3">本批次输出</h3>
        <span className="u-meta">{requested} 张 · 成功 {successImages.length} · 生成中 {pendingCount}{failedSlots.length ? ` · 失败 ${failedSlots.length}` : ''}</span>
      </div>

      {task.task_type === 'image_edit' && task.reference_assets.length ? (
        <div className="u-detail__refs">
          <span className="u-detail__refs-label">原图引用</span>
          {task.reference_assets.map((asset) => (
            <ReferenceAssetPreview
              key={asset.id || asset.preview_url || asset.download_url}
              asset={asset}
              accessToken={accessToken}
              onMediaRefresh={() => onReferenceMediaRefresh(asset.id)}
              onClick={(url, selectedAsset) => onPreviewImage({
                url,
                downloadUrl: url,
                alt: selectedAsset.name || '原图引用',
                source: '原图引用',
                mediaExpiresAt: selectedAsset.preview_expires_at,
                mediaResource: { kind: 'reference', scope: 'private', id: selectedAsset.id },
              })}
            />
          ))}
        </div>
      ) : null}

      {showInitialLoading ? (
        <div className="u-stage cr-stage--loading">
          <div className={cn(rdWorkspace.outputLoading, 'm-auto')}>
            <div className={rdWorkspace.outputRing}>
              <div className={rdWorkspace.outputRingInner1} />
              <div className={rdWorkspace.outputRingInner2} />
              <div className={rdWorkspace.outputRingCore}>
                <SparkleGlyph />
              </div>
            </div>
            <h4 className={rdWorkspace.outputStage}>Mikiko Studio 引擎解算中</h4>
            <p className={rdWorkspace.outputStageText}>{activeStage}</p>
          </div>
        </div>
      ) : (
        <>
          {primaryImage ? (
            <div className="u-stage">
              <GeneratedImage
                image={primaryImage}
                task={task}
                profile={profile}
                alt={task.title}
                fallbackRatio={task.aspect_ratio}
                accessToken={accessToken}
                onUseReference={onUseReference}
                onPreview={onPreviewImage}
                onDownload={onDownloadImage}
                onMediaRefresh={() => onImageMediaRefresh(primaryImage.id)}
                onCopyPrompt={() => void onCopyPrompt()}
                onReuseConfig={() => onReuseConfig(task)}
                onOpenDetail={onOpenDetail}
              />
            </div>
          ) : failedSlots.length ? (
            <div className="u-stage">
              <div className="cr-stage-message">
                <GenerationSlotFailure title={failedSlots[0].title} reason={failedSlots[0].reason} code={failedSlots[0].code} />
              </div>
            </div>
          ) : isTerminalStatus(task.status) ? (
            <div className="u-stage">
              <div className="cr-stage-message">
                <TaskFailureBlock task={task} onRetry={() => onRetryTask(task)} onDelete={() => onDeleteTask(task)} />
              </div>
            </div>
          ) : (
            <div className="u-stage">
              <GenerationSlotPending label={activeStage} skeleton={skeletonPhase} />
            </div>
          )}

          {batchSlots.length ? (
            <div className="cr-output__batch">
              <div className="u-grid cr-batch-grid">
                {batchSlots.map((slot) => {
                  if (slot.kind === 'image') {
                    return (
                      <GeneratedBatchFigure
                        key={slot.image.id || `image-${slot.index}`}
                        image={slot.image}
                        ordinal={slot.index + 1}
                        task={task}
                        profile={profile}
                        accessToken={accessToken}
                        onPreview={onPreviewImage}
                        onDownload={onDownloadImage}
                        onMediaRefresh={() => onImageMediaRefresh(slot.image.id)}
                      />
                    )
                  }
                  if (slot.kind === 'failed') {
                    return (
                      <div key={`failed-${slot.index}`} className="cr-figure__failure" role="alert">
                        <strong>{slot.title}</strong>
                        <span>{slot.reason}</span>
                      </div>
                    )
                  }
                  return (
                    <div key={`pending-${slot.index}`} className="u-skeleton cr-figure__pending">
                      {skeletonPhase ? <span className={workspaceClasses.slotSkeletonGlow} aria-hidden="true" /> : null}
                      <span className={userState.spinner} />
                      生成中 #{slot.index + 1}
                    </div>
                  )
                })}
              </div>
            </div>
          ) : null}

          <div className="u-meta cr-output__meta">
            <span>模型: {task.route_model_name || task.route_model_code || task.model_group}</span>
            <span>{task.size_mode === 'pixel' ? `尺寸: ${task.requested_size || task.aspect_ratio}` : `比例: ${task.aspect_ratio}`}</span>
            <span>数量: {task.image_count}</span>
            <span>耗时: {formatCompactDuration(taskElapsedMs(task))}</span>
            <span>消耗: {displayTaskPoints(task)}</span>
          </div>
        </>
      )}
    </article>
  )
}

function GenerationSlotPending({ label, skeleton }: { label: string; skeleton: boolean }) {
  if (skeleton) {
    return (
      <div className="cr-working">
        <span className={userState.spinner} />
        <strong>{label}</strong>
      </div>
    )
  }
  return (
    <div className="cr-working">
      <span className={userState.spinner} />
      <strong>{label}</strong>
    </div>
  )
}

function GenerationSlotFailure({ title, reason, code }: { title: string; reason: string; code?: string }) {
  return (
    <div className="cr-slot-failure" role="alert">
      <strong>{title}</strong>
      <p>{reason}</p>
      {code ? <span className="u-chip u-chip--mono">{code}</span> : null}
    </div>
  )
}

function TaskFailureBlock({ task, onRetry, onDelete }: { task: ImageTask; onRetry: () => Promise<void>; onDelete: () => Promise<void> }) {
  const view = workspaceTaskFailureView(task)
  return (
    <div className="cr-slot-failure" role="alert">
      <strong>{view.title}</strong>
      <p>{view.reason}</p>
      {view.meta.length ? (
        <dl className="u-facts">
          {view.meta.map((item) => (
            <div className="u-fact" key={item.label}>
              <dt>{item.label}</dt>
              <dd className="inline-flex items-center gap-1">
                {item.value}
                {item.label === '任务ID' ? <InlineCopyButton text={item.value} label="复制任务ID" /> : null}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      <div className="cr-task-actions">
        <button className="u-btn u-btn--sm u-btn--primary" type="button" onClick={() => void onRetry()}>重试</button>
        <button className="u-btn u-btn--sm" type="button" onClick={() => void onDelete()}>删除</button>
      </div>
    </div>
  )
}

function parsePixelDimensions(input: string) {
  const match = input.match(/^(\d+)x(\d+)$/i)
  if (!match) return null
  return { width: Number(match[1]), height: Number(match[2]) }
}


function imageSizeLabel(image: ImageResult, fallbackRatio?: string) {
  if (image.width && image.height) return `${image.width} × ${image.height}`
  return fallbackRatio || '原始尺寸'
}

function imageQualityLabel(task: ImageTask) {
  return task.quality || task.requested_quality || task.resolved_quality_bucket || 'auto'
}

function imagePreviewPayload(image: ImageResult, task: ImageTask, profile: Pick<UserProfile, 'display_name'> | null | undefined, accessToken?: string): ImagePreviewPayload {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  return {
    url: imageUrl,
    downloadUrl: imageUrl,
    alt: task.title,
    prompt: image.prompt || task.title,
    width: image.width,
    height: image.height,
    ratio: task.aspect_ratio,
    model: image.route_model_code || image.abstract_model,
    source: '创作输出',
    creationDraft: workspaceCreationDraftFromSnapshot(task),
    detailImage: projectWorkspaceImageDetail(image, task, profile),
    mediaResource: { kind: 'image', scope: 'private', id: image.id },
    mediaExpiresAt: image.preview_expires_at,
  }
}

function GeneratedImage({ image, task, profile, alt, fallbackRatio, accessToken, onUseReference, onPreview, onDownload, onMediaRefresh, onCopyPrompt, onReuseConfig, onOpenDetail }: {
  image: ImageResult
  task: ImageTask
  profile?: Pick<UserProfile, 'display_name'> | null
  alt: string
  fallbackRatio?: string
  accessToken?: string
  onUseReference: (image: ImageResult) => Promise<void>
  onPreview: (image: ImagePreviewPayload) => void
  onDownload: (image: ImageResult) => Promise<void>
  onMediaRefresh: () => string | undefined | void | Promise<string | undefined | void>
  onCopyPrompt: () => void
  onReuseConfig: () => void
  onOpenDetail: () => void
}) {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  const caption = `#1 · ${imageSizeLabel(image, fallbackRatio)} · ${imageQualityLabel(task)} · ${formatCompactDuration(taskElapsedMs(task))}`
  return (
    <figure className="u-result cr-result cr-result--stage group">
      <button
        type="button"
        className="cr-result__media"
        onClick={() => onPreview(imagePreviewPayload(image, task, profile, accessToken))}
        aria-label="预览生成图片"
      >
        <RefreshableMediaImage className={cn('cr-result__img', workspaceClasses.generatedImage)} src={imageUrl} mediaExpiresAt={image.preview_expires_at} alt={alt} onMediaRefresh={onMediaRefresh} />
      </button>
      <div className="u-result__bar">
        <div className="u-result__ops">
          <button className="u-result__op" type="button" title="复制提示词" aria-label="复制提示词" onClick={onCopyPrompt}><Copy size={15} /></button>
          <button className="u-result__op" type="button" title="复用参数" aria-label="复用参数" onClick={onReuseConfig}><RotateCcw size={15} /></button>
          <button className="u-result__op" type="button" title="编辑" aria-label="编辑图片" onClick={() => void onUseReference(image)}><Pencil size={15} /></button>
          <button className="u-result__op" type="button" title="详情" aria-label="查看任务详情" onClick={onOpenDetail}><Info size={15} /></button>
          <button className="u-result__op" type="button" title="下载" aria-label="下载图片" onClick={() => void onDownload(image)}><Download size={15} /></button>
        </div>
        <span className="u-result__caption">{caption}</span>
      </div>
    </figure>
  )
}

function GeneratedBatchFigure({ image, ordinal, task, profile, accessToken, onPreview, onDownload, onMediaRefresh }: {
  image: ImageResult
  ordinal: number
  task: ImageTask
  profile?: Pick<UserProfile, 'display_name'> | null
  accessToken?: string
  onPreview: (image: ImagePreviewPayload) => void
  onDownload: (image: ImageResult) => Promise<void>
  onMediaRefresh: () => string | undefined | void | Promise<string | undefined | void>
}) {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  const preview = () => onPreview(imagePreviewPayload(image, task, profile, accessToken))
  return (
    <figure className="cr-figure group">
      <div className="cr-figure__frame">
        <button type="button" className="cr-figure__media" onClick={preview} aria-label={`预览第 ${ordinal} 张图片`}>
          <RefreshableMediaImage className={workspaceClasses.generatedImage} src={imageUrl} mediaExpiresAt={image.preview_expires_at} alt={`${task.title} ${ordinal}`} onMediaRefresh={onMediaRefresh} />
        </button>
        <div className="cr-figure__ops">
          <button className="u-card__op" type="button" title="详情" aria-label="查看图片详情" onClick={preview}><Info size={15} /></button>
          <button className="u-card__op" type="button" title="下载" aria-label="下载图片" onClick={() => void onDownload(image)}><Download size={15} /></button>
        </div>
      </div>
      <figcaption className="cr-figure__cap">
        <span>#{ordinal} · {imageSizeLabel(image, task.aspect_ratio)}</span>
        <span>{formatCompactDuration(taskElapsedMs(task))}</span>
      </figcaption>
    </figure>
  )
}
