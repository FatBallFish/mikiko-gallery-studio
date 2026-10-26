import { canHideHistoryRecord } from '../../pages/historyRecordActions'
import { useHistoryRecordActions } from '../../pages/HistoryRecordControls'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { OverlayPortal } from '../../ui/overlayPortal'
import { Ban, ChevronUp, Copy, Download, FastForward, Film, Image as ImageIcon, Info, LoaderCircle, Play, RefreshCw, RotateCcw, Sparkles, Volume2, VolumeX, X } from 'lucide-react'
import type { CapabilityModelGroup, MediaAsset, PromptReferenceBinding, ReferenceAsset, VideoCapability, VideoEstimateRequest, VideoTask, VideoTaskType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { cn } from '../../../../shared/classnames'
import { Button, Drawer, EmptyState, ErrorState, InlineCopyButton, LoadingState, Modal, copyText, useApp } from '../../components'
import { useProjects } from '../../ProjectContext'
import { ModelGroupSelect } from '../../pages/ModelGroupSelect'
import { PromptTemplateEditor, type PromptTemplateEditorHandle } from '../../pages/PromptTemplateEditor'
import { PromptEditorActions, PromptEditorDialog, PromptOptimizationPanel } from '../../pages/PromptEditorDialog'
import { PromptVariableForm } from '../../pages/PromptVariableForm'
import { WorkspaceStatusRail } from '../../pages/WorkspaceStatusRail'
import { useCompactWorkspaceViewport } from '../../pages/workspaceResponsive'
import type { WorkspaceProgressNode } from '../../pages/workspaceTaskProgress'
import type { WorkspaceTaskView } from '../../pages/workspaceViewModel'
import { applyOptimizedPrompt, beginPromptOptimization, confirmPromptOptimization, failPromptOptimization, initialPromptOptimizationState, receivePromptEstimate, receivePromptOptimization, undoPromptOptimization } from '../../pages/workspacePromptOptimization'
import { errorMessage } from '../../useApiResource'
import { MediaAssetPicker } from '../media/MediaAssetPicker'
import { MediaPreviewDialog } from '../media/MediaPreviewDialog'
import { createReferenceReservations, referenceAdmission } from '../../../../shared/reference-admission'
import { buildVideoQuoteBreakdown, buildVideoTaskAccounting } from './videoAccounting'
import { videoDraftInvalidReason, videoInputAssetReason, videoReferencePolicy, resolveInputCapabilities, applyVideoCapability, defaultVideoDraft, invalidateVideoQuote, promptReferenceNames, reuseVideoTask, videoDraftKey, videoModelForDraft, VIDEO_TASK_INPUT_ROLES, videoTaskInputLabel, videoTaskInputMissing, type VideoDraft, type VideoDraftInputRole, type VideoQuoteState } from './videoDraft'
import { cachedVideoCapability, loadVideoCapability } from './videoCapabilityCache'
import { pointsText } from '../../../../shared/pointsDisplay'
import { userState } from '../../ui/classes'
import { videoFieldErrors, videoRequestError, type VideoFieldErrors } from './videoErrors'

type Props = { initialTaskId?: string; initialAssetId?: string }

const taskTypeLabels: Record<VideoTaskType, string> = {
  text_to_video: '文生视频', image_to_video: '首帧生视频', first_last_frame_to_video: '首尾帧生视频',
  reference_to_video: '参考生视频', video_edit: '视频编辑', video_extend: '视频延长',
}
const taskTypeHints: Record<VideoTaskType, string> = {
  text_to_video: '纯文本描述', image_to_video: '一张起始画面', first_last_frame_to_video: '起始 + 结束',
  reference_to_video: '主体 / 风格参考', video_edit: '修改已有片段', video_extend: '续写已有镜头',
}
const stageLabels: Record<string, string> = {
  queued: '排队中', submitting: '正在提交', reconciling: '正在确认', provider_queued: '上游排队', provider_running: '生成中',
  artifact_pending: '正在保存原件', recovery_required: '正在恢复原件', saving: '正在保存', succeeded: '已完成', partial: '部分完成', failed: '失败', cancelled: '已取消',
}

export function VideoCreationPanel({ initialTaskId, initialAssetId }: Props) {
  const app = useApp()
  const projects = useProjects()
  const [capability, setCapability] = useState<VideoCapability | null>(() => cachedVideoCapability())
  const [draft, setDraft] = useState<VideoDraft | null>(() => {
    const snapshot = cachedVideoCapability()
    return snapshot ? defaultVideoDraft(snapshot, initialAssetId ? 'image_to_video' : undefined) : null
  })
  const live = useRef({ draft, capability, projectID: projects.selectedProjectID })
  live.current = { draft, capability, projectID: projects.selectedProjectID }
  const reservations = useRef(createReferenceReservations<VideoDraftInputRole>())
  const [pendingUploads, setPendingUploads] = useState(0)
  const [quote, setQuote] = useState<VideoQuoteState | null>(null)
  const [tasks, setTasks] = useState<VideoTask[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [estimateError, setEstimateError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<VideoFieldErrors>({})
  const [submitting, setSubmitting] = useState(false)
  const [taskAction, setTaskAction] = useState('')
  const historyActions = useHistoryRecordActions()
  const currentProjectRef = useRef(projects.selectedProjectID)
  currentProjectRef.current = projects.selectedProjectID
  const [selectedAssets, setSelectedAssets] = useState<Record<string, MediaAsset>>({})
  const [pickerRole, setPickerRole] = useState<VideoDraftInputRole | null>(null)
  const [previewAsset, setPreviewAsset] = useState<MediaAsset | null>(null)
  const [detailTask, setDetailTask] = useState<VideoTask | null>(null)
  const [taskTab, setTaskTab] = useState<'current' | 'history'>('current')
  const [controlsOpen, setControlsOpen] = useState(false)
  const [historyQuery, setHistoryQuery] = useState('')
  const [historyStatus, setHistoryStatus] = useState('all')
  const compactViewport = useCompactWorkspaceViewport()
  const [selectedTaskID, setSelectedTaskID] = useState<string | undefined>(undefined)
  const [editingAsset, setEditingAsset] = useState(false)
  const priorDraftRef = useRef<VideoDraft | null>(null)
  const completedTaskIDsRef = useRef(new Set<string>())
  const promptEditorRef = useRef<PromptTemplateEditorHandle | null>(null)
  const [promptExpanded, setPromptExpanded] = useState(false)
  const [promptOptimization, setPromptOptimization] = useState(initialPromptOptimizationState)

  useEffect(() => {
    let alive = true
    // Serve the cached snapshot immediately: tab switches must not blank the
    // console behind the capability spinner. Only the very first visit (no
    // cache yet) shows 正在读取视频能力.
    const snapshot = cachedVideoCapability()
    if (snapshot && !capability) {
      setCapability(snapshot)
      setDraft((current) => current ?? defaultVideoDraft(snapshot, initialAssetId ? 'image_to_video' : undefined))
    }
    if (!snapshot) setLoading(true)
    loadVideoCapability().then((next) => {
      if (!alive) return
      setCapability(next)
      setDraft((current) => {
        const base = current ?? defaultVideoDraft(next, initialAssetId ? 'image_to_video' : undefined)
        const normalized = applyVideoCapability(base, next)
        if (normalized.changes.length) app.notify('warn', normalized.changes.join('；'))
        return normalized.draft
      })
      setError('')
    }).catch((reason) => alive && setError(errorMessage(reason))).finally(() => alive && setLoading(false))
    return () => { alive = false }
  }, [app, initialAssetId])

  useEffect(() => {
    if (!initialAssetId || !capability) return
    let alive = true
    userApi.getMediaAsset(initialAssetId).then((asset) => { if (alive) admitAssets([asset], 'first_frame') }).catch(() => { if (alive) app.notify('error', '首帧素材不可用或无权访问') })
    return () => { alive = false }
  }, [initialAssetId, capability])

  useEffect(() => {
	if (!initialTaskId || !capability) return
    let alive = true
    userApi.getVideoTask(initialTaskId).then((task) => {
      if (!alive) return
      setTasks((items) => mergeTasks(items, task))
	  const reused = applyVideoCapability(reuseVideoTask(task), capability).draft
	  void hydrateInputAssets(reused)
	  setDraft(reused)
	  setQuote(null)
    }).catch(() => undefined)
    return () => { alive = false }
  }, [capability, initialTaskId])

  useEffect(() => {
    if (!projects.selectedProjectID) return
    let alive = true
    let source: EventSource | null = null
    let retryTimer: number | null = null
    let retryCount = 0
    const token = app.session?.token
    const refresh = () => userApi.listVideoTasks({ project_id: projects.selectedProjectID, limit: 20 }).then((page) => {
      if (alive) setTasks(page.items)
    }).catch((reason) => alive && setError(errorMessage(reason)))
    async function refreshTask(taskID: string) {
      try {
        const task = await userApi.getVideoTask(taskID)
        if (!alive) return
        setTasks((items) => mergeTasks(items, task))
        if (['succeeded', 'partial', 'failed', 'cancelled'].includes(task.status) && !completedTaskIDsRef.current.has(task.id)) {
          completedTaskIDsRef.current.add(task.id)
          await app.refreshAccount()
        }
      } catch (reason) {
        if (alive) setError(errorMessage(reason))
      }
    }
    function connect() {
      if (!alive || !token) return
      source = new EventSource(userApi.videoTaskStreamUrl(token, projects.selectedProjectID))
      source.addEventListener('open', () => { retryCount = 0 })
      source.addEventListener('task', (event) => {
        try {
          const projection = JSON.parse((event as MessageEvent).data) as { id?: string }
          const taskID = projection.id?.trim()
          if (taskID) void refreshTask(taskID)
        } catch {
          if (alive) setError('任务状态数据无效，已保留当前列表。')
        }
      })
      source.addEventListener('error', () => {
        source?.close()
        source = null
        void refresh()
        retryCount += 1
        if (retryCount > 5 || !alive) return
        retryTimer = window.setTimeout(connect, Math.min(500 * (2 ** retryCount), 8000))
      })
    }
    void refresh()
    connect()
    return () => {
      alive = false
      source?.close()
      if (retryTimer !== null) window.clearTimeout(retryTimer)
    }
  }, [app, app.session?.token, projects.selectedProjectID])

  useEffect(() => {
    if (pendingUploads > 0 || Object.values(reservations.current.pending()).some((count) => (count ?? 0) > 0) || !draft || !capability || !projects.selectedProjectID || !draft.prompt_template.trim()) {
      setQuote(null)
      return undefined
    }
		if (videoDraftInvalidReason(draft, capability, selectedAssets) || videoTaskInputMissing(draft.task_type, draft.inputs)) {
      setQuote(null)
      return undefined
    }
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      const request = estimateRequest(projects.selectedProjectID, draft, selectedAssets)
      userApi.estimateVideo(request, controller.signal).then((result) => {
        if (controller.signal.aborted || Object.values(reservations.current.pending()).some((count) => (count ?? 0) > 0)) return
        setQuote({
          key: videoDraftKey(draft), quote_token: result.quote_token, quote_expires_at: result.expires_at,
          unit_points: result.unit_points, estimated_points: result.estimated_points, max_reserved_points: result.max_reserved_points,
          available_points: result.balance?.available_points, pricing_mode: result.pricing_mode, summary: result.summary,
          display_points: result.display_points, sufficient: result.balance?.sufficient,
        })
        setEstimateError('')
        setFieldErrors({})
      }).catch((reason) => {
        if (controller.signal.aborted) return
        setQuote(null)
        setEstimateError(videoRequestError(reason))
        setFieldErrors(videoFieldErrors(reason))
      })
    }, 250)
    return () => { window.clearTimeout(timer); controller.abort() }
  }, [capability, draft, projects.selectedProjectID, selectedAssets, pendingUploads])

  useEffect(() => {
    if (draft && priorDraftRef.current) setQuote((current) => invalidateVideoQuote(current, priorDraftRef.current!, draft))
    priorDraftRef.current = draft
  }, [draft])

  useEffect(() => {
    if (!quote) return undefined
    const delay = new Date(quote.quote_expires_at).getTime() - Date.now()
    if (delay <= 0) {
      setQuote(null)
      return undefined
    }
    const timer = window.setTimeout(() => setQuote((current) => current === quote ? null : current), delay)
    return () => window.clearTimeout(timer)
  }, [quote])

  const model = useMemo(() => capability && draft ? videoModelForDraft(capability, draft) : undefined, [capability, draft])
  const options = model && draft ? model.options_by_task_type[draft.task_type] : undefined
  const modelOptions = useMemo(() => (capability?.model_groups ?? []).map((item) => ({
    ...item,
    id: item.code,
    task_types: ['text_to_image'], base_resolution: [], quality: [], output_format: [], moderation: [], prices: [],
    max_output_image_count: 1, max_reference_image_count: 0, supports_reference: false,
  })) as CapabilityModelGroup[], [capability])
  const promptAssets = useMemo(() => draft
    ? draft.inputs.map((input) => selectedAssets[input.asset_id]).filter((asset): asset is MediaAsset => Boolean(asset) && asset.media_type === 'image')
    : [], [draft, selectedAssets])
  const promptVariables = useMemo(() => Object.fromEntries((draft?.prompt_variables ?? []).map((item) => [item.name, item.value])), [draft])
  const orderedTasks = useMemo(() => [...tasks].sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? '')), [tasks])
  const currentTask = useMemo(() => orderedTasks.find((task) => task.id === selectedTaskID) ?? orderedTasks[0], [orderedTasks, selectedTaskID])
  const historyTasks = useMemo(() => {
    const normalized = historyQuery.trim().toLowerCase()
    return orderedTasks.filter((task) => {
      if (historyStatus !== 'all' && task.status !== historyStatus) return false
      if (!normalized) return true
      return task.id.toLowerCase().includes(normalized) || task.prompt_template.toLowerCase().includes(normalized)
    })
  }, [historyQuery, historyStatus, orderedTasks])
  const runningCount = orderedTasks.filter((task) => !isVideoTaskTerminal(task)).length

  if (loading && !capability) return <LoadingState label="正在读取视频能力..." />
  if (error && !capability) return <ErrorState message={error} />
  if (!capability || !draft) return <EmptyState title="暂无可用视频模型" detail="当前用户组没有已启用的视频模型分组。" />
  const invalidReason = videoDraftInvalidReason(draft, capability, selectedAssets)
  const displayModel = model ?? { task_types: [] as VideoTaskType[], max_output_count: draft.output_count }
  const displayOptions = options ?? { durations: [draft.duration_seconds], combinations: [], audio_generation: false }
  const activeCapability = capability
  const activeDraft = draft
  const resolutionOptions = uniqueVideoValues(displayOptions.combinations.filter((item) => item.duration_seconds === draft.duration_seconds).map((item) => item.resolution))
  const aspectRatioOptions = uniqueVideoValues(displayOptions.combinations.filter((item) => item.duration_seconds === draft.duration_seconds && item.resolution === draft.resolution).map((item) => item.aspect_ratio))
  const audioAvailable = displayOptions.combinations.some((item) => item.duration_seconds === draft.duration_seconds && item.resolution === draft.resolution && item.aspect_ratio === draft.aspect_ratio && item.audio_mode === 'generated')
  const quoteBreakdown = quote ? buildVideoQuoteBreakdown({
    quote_token: quote.quote_token, expires_at: quote.quote_expires_at, capability_version: capability.capability_version,
    config_version: '', price_version: '', unit_points: quote.unit_points, estimated_points: quote.estimated_points,
    max_reserved_points: quote.max_reserved_points, display_points: quote.display_points, pricing_mode: quote.pricing_mode,
    summary: quote.summary, balance: quote.available_points ? { available_points: quote.available_points, sufficient: quote.sufficient !== false } : undefined,
  }) : null

  function admitAssets(assets: MediaAsset[], role: VideoDraftInputRole, excludeToken?: number) {
    const state = live.current
    if (!state.draft || !state.capability) return false
    const model = videoModelForDraft(state.capability, state.draft)
    for (const asset of assets) {
      const reason = videoInputAssetReason(model, state.draft.task_type, role, asset)
      if (reason) { app.notify('warn', reason); return false }
    }
    const current = role === 'first_frame' || role === 'last_frame' ? state.draft.inputs.filter((input) => input.role !== role) : state.draft.inputs
    const incoming = assets.map((asset, ordinal) => ({ asset_id: asset.id, role, ordinal }))
    const result = referenceAdmission({ policy: videoReferencePolicy(model, state.draft.task_type), current, incoming, reserved: reservations.current.pending(excludeToken), kind: (input) => input.role, key: (input) => `${input.role}:${input.asset_id}` })
    if (!result.ok) { app.notify('warn', result.reason); return false }
    const next = { ...state.draft, inputs: result.merged }
    live.current = { ...state, draft: next }
    setDraft(next)
    setSelectedAssets((previous) => ({ ...previous, ...Object.fromEntries(assets.map((asset) => [asset.id, asset])) }))
    setQuote(null)
    return true
  }

  async function uploadReferences(files: File[]) {
    if (!files.length) return
    const state = live.current
    if (!state.draft || !state.capability) return
    const model = videoModelForDraft(state.capability, state.draft)
    const policy = videoReferencePolicy(model, state.draft.task_type)
    if (policy.reason) { app.notify('warn', policy.reason); return }
    const entries = files.map((file, ordinal) => {
      const mediaType = file.type.split('/')[0]
      const role = VIDEO_TASK_INPUT_ROLES[state.draft!.task_type].find((role) => resolveInputCapabilities(model, state.draft!.task_type)?.[role]?.media_types.includes(mediaType))
      return { file, role, asset_id: `pending:${ordinal}:${file.name}`, ordinal }
    })
    for (const entry of entries) {
      if (!entry.role) { app.notify('warn', '当前模型与生成方式不支持该引用类型'); return }
      const reason = videoInputAssetReason(model, state.draft.task_type, entry.role, { media_type: entry.file.type.split('/')[0] as MediaAsset['media_type'], mime_type: entry.file.type, name: entry.file.name, file_size_bytes: entry.file.size })
      if (reason) { app.notify('warn', reason); return }
    }
    const incoming = entries.map((entry) => ({ asset_id: entry.asset_id, role: entry.role!, ordinal: entry.ordinal }))
    const replacing = new Set(incoming.filter((input) => input.role === 'first_frame' || input.role === 'last_frame').map((input) => input.role))
    const current = state.draft.inputs.filter((input) => !replacing.has(input.role))
    const reservation = reservations.current.reserve(policy, current, incoming, (input) => input.role)
    if (!reservation.ok) { app.notify('warn', reservation.reason); return }
    setQuote(null)
    setPendingUploads((count) => count + 1)
    try {
      const uploaded = await Promise.all(entries.map(async (entry) => ({ asset: await uploadSingleMediaAsset(entry.file, state.projectID), role: entry.role! })))
      const now = live.current
      if (!now.draft || !now.capability || now.projectID !== state.projectID || now.draft.route_model_code !== state.draft.route_model_code || now.draft.task_type !== state.draft.task_type) {
        app.notify('warn', '模型、生成方式或项目已变化，上传素材保留在资产库，请重新选择引用'); return
      }
      const currentModel = videoModelForDraft(now.capability, now.draft)
      for (const { asset, role } of uploaded) {
        const reason = videoInputAssetReason(currentModel, now.draft.task_type, role, asset)
        if (reason) { app.notify('warn', reason); return }
      }
      const result = referenceAdmission({ policy: videoReferencePolicy(currentModel, now.draft.task_type), current: now.draft.inputs.filter((input) => !replacing.has(input.role)), incoming: uploaded.map(({ asset, role }, ordinal) => ({ asset_id: asset.id, role, ordinal })), reserved: reservations.current.pending(reservation.token), kind: (input) => input.role, key: (input) => `${input.role}:${input.asset_id}` })
      if (!result.ok) { app.notify('warn', result.reason); return }
      const next = { ...now.draft, inputs: result.merged }
      live.current = { ...now, draft: next }
      setDraft(next)
      setSelectedAssets((previous) => ({ ...previous, ...Object.fromEntries(uploaded.map(({ asset }) => [asset.id, asset])) }))
      setQuote(null)
    } catch (caught) { app.notify('error', errorMessage(caught)) }
    finally {
      if (reservation.token !== undefined) reservations.current.release(reservation.token)
      setPendingUploads((count) => Math.max(0, count - 1))
    }
  }

  function patchDraft(patch: Partial<VideoDraft>) {
    const preferredField = Object.keys(patch)[0] as keyof VideoDraft | undefined
    const normalized = applyVideoCapability({ ...(live.current.draft ?? activeDraft), ...patch }, live.current.capability ?? activeCapability, preferredField)
    live.current = { ...live.current, draft: normalized.draft }
    setDraft(normalized.draft)
    setFieldErrors({})
    if (normalized.changes.length) app.notify('warn', normalized.changes.join('；'))
  }

  async function startPromptOptimization() {
    const value = activeDraft.prompt_template.trim()
    if (Array.from(value).length < 8) {
      app.notify('warn', '提示词至少需要 8 个字符后才能优化')
      return
    }
    const next = beginPromptOptimization(promptOptimization, value)
    if (next === promptOptimization) return
    setPromptOptimization(next)
    try {
      const estimateResult = await userApi.estimatePromptOptimization(value, 'video', activeDraft.route_model_code)
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
      const result = await userApi.optimizePrompt(next.originalPrompt, promptOptimization.estimate.quote, 'video', activeDraft.route_model_code)
      setPromptOptimization(receivePromptOptimization(next, result))
    } catch (cause) {
      setPromptOptimization(failPromptOptimization(next, errorMessage(cause)))
    }
  }

  function applyOptimization() {
    const applied = applyOptimizedPrompt(promptOptimization)
    patchDraft({ prompt_template: applied.prompt, prompt_variables: reconcileVariables(applied.prompt, activeDraft.prompt_variables) })
    setPromptOptimization(applied.state)
    app.notify('success', '优化后的提示词已应用')
  }

  function undoOptimization() {
    const undone = undoPromptOptimization(promptOptimization, activeDraft.prompt_template)
    patchDraft({ prompt_template: undone.prompt, prompt_variables: reconcileVariables(undone.prompt, activeDraft.prompt_variables) })
    setPromptOptimization(undone.state)
    app.notify('success', '已恢复优化前的提示词')
  }

  function cancelOptimization() {
    setPromptOptimization(initialPromptOptimizationState())
  }

  function openFramePicker() {
    const policy = videoReferencePolicy(model, activeDraft.task_type)
    if (policy.reason) { app.notify('warn', policy.reason); return }
    const role = VIDEO_TASK_INPUT_ROLES[activeDraft.task_type].find((role) => resolveInputCapabilities(model, activeDraft.task_type)?.[role]?.max_count !== 0 && resolveInputCapabilities(model, activeDraft.task_type)?.[role])
    if (role) setPickerRole(role)
  }

  function changePrompt(value: string) {
    if (value === activeDraft.prompt_template) return
    patchDraft({ prompt_template: value, prompt_variables: reconcileVariables(value, activeDraft.prompt_variables) })
  }

  function changePromptVariable(name: string, value: string) {
    patchDraft({ prompt_variables: activeDraft.prompt_variables.map((item) => item.name === name ? { ...item, value } : item) })
  }

  function reuseDraft(task: VideoTask) {
    const reused = applyVideoCapability(reuseVideoTask(task), activeCapability).draft
    void hydrateInputAssets(reused)
    setDraft(reused)
    setQuote(null)
    app.notify('success', '已复用提示词、兼容参数与引用资源')
  }

  // Reused tasks carry their input asset ids; fetch the asset records so the
  // input rows can show names and previews instead of looking empty.
  async function hydrateInputAssets(target: VideoDraft) {
    await Promise.all(target.inputs.filter((input) => input.asset_id.trim()).map(async (input) => {
      if (selectedAssets[input.asset_id]) return
      try {
        const asset = await userApi.getMediaAsset(input.asset_id)
        setSelectedAssets((current) => ({ ...current, [asset.id]: asset }))
      } catch {
        app.notify('warn', '部分引用已失效或无权访问，已保留引用，请移除或重新选择')
      }
    }))
  }

  async function openResultPreview(assetID: string) {
    try {
      setPreviewAsset(await userApi.getMediaAsset(assetID))
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    }
  }

  // Download goes through the presigned storage URL so the bytes stream
  // directly from object storage instead of the API service.
  async function downloadResult(assetID: string) {
    try {
      const access = await userApi.getMediaAssetAccess(assetID, 'download')
      const anchor = document.createElement('a')
      anchor.href = userApi.mediaAssetUrl(access.url, app.session?.token)
      anchor.download = ''
      anchor.rel = 'noopener'
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      app.notify('success', '已开始下载视频')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    }
  }

  async function editFromResult(assetID: string) {
    setEditingAsset(true)
    try {
      const policy = videoReferencePolicy(model, activeDraft.task_type)
      if (policy.reason || !resolveInputCapabilities(model, activeDraft.task_type)?.first_frame) { app.notify('warn', policy.reason || '当前生成方式不支持首帧引用'); return }
      const access = await userApi.getMediaAssetAccess(assetID, 'preview')
      const frame = await captureVideoFrameBlob(userApi.mediaAssetUrl(access.url, app.session?.token))
      await uploadReferences([new File([frame], `视频续帧-${assetID.slice(0, 8)}.png`, { type: 'image/png' })])
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setEditingAsset(false)
    }
  }

  async function submit() {
    if (Object.values(reservations.current.pending()).some((count) => (count ?? 0) > 0)) { app.notify('warn', '引用素材正在上传，请等待上传完成后重新估价'); return }
    const current = live.current
    const reason = current.draft && current.capability ? videoDraftInvalidReason(current.draft, current.capability, selectedAssets) : '视频能力不可用'
    if (reason) { app.notify('warn', reason); return }
    if (!quote || quote.key !== videoDraftKey(activeDraft) || new Date(quote.quote_expires_at).getTime() <= Date.now()) {
      setEstimateError('报价已失效，正在重新估价。')
      setQuote(null)
      return
    }
    setSubmitting(true)
    setError('')
    setFieldErrors({})
    try {
      const task = await userApi.createVideoTask({ ...estimateRequest(projects.selectedProjectID, activeDraft, selectedAssets), quote_token: quote.quote_token })
      setTasks((items) => mergeTasks(items, task))
      setQuote(null)
      setSelectedTaskID(task.id)
      setTaskTab('current')
      setControlsOpen(false)
      app.notify('success', '视频任务已提交')
      await app.refreshAccount()
    } catch (reason) {
      setError(videoRequestError(reason))
      setFieldErrors(videoFieldErrors(reason))
    } finally {
      setSubmitting(false)
    }
  }

  async function retryHistoryTask(task: VideoTask) {
    if (task.status !== 'failed') return
    const projectID = projects.selectedProjectID
    await historyActions.run(async () => {
      const created = await userApi.retryVideoTask(task.id)
      if (currentProjectRef.current !== projectID) return
      setTasks((items) => [created, ...items.filter((item) => item.id !== created.id)])
      setSelectedTaskID(created.id)
      setTaskTab('current')
      app.notify('success', '已创建新的重试任务，原记录已保留')
      await app.refreshAccount()
    })
  }

  function confirmHideHistoryTask(task: VideoTask) {
    if (!canHideHistoryRecord(task.status)) return
    historyActions.confirm(false, async () => {
      await userApi.hideVideoTaskHistory(task.id)
      setTasks((items) => items.filter((item) => item.id !== task.id))
      setDetailTask((current) => current?.id === task.id ? null : current)
      setSelectedTaskID((id) => id === task.id ? undefined : id)
      app.notify('success', '历史记录已删除，素材已保留')
    })
  }

  function confirmClearFailedHistory() {
    const projectID = projects.selectedProjectID
    historyActions.confirm(true, async () => {
      const result = await userApi.clearFailedVideoTaskHistory({ project_id: projectID })
      if (currentProjectRef.current === projectID) {
        const page = await userApi.listVideoTasks({ project_id: projectID, limit: 20 })
        if (currentProjectRef.current === projectID) setTasks(page.items)
        setDetailTask(null)
      }
      app.notify('success', `已清空 ${result.hidden_count} 条失败记录`)
    })
  }

  async function cancelTask(task: VideoTask) {
    setTaskAction(task.id)
    try {
      const updated = await userApi.cancelVideoTask(task.id)
      setTasks((items) => mergeTasks(items, updated))
      app.notify('info', '已提交取消请求')
    } catch (reason) {
      app.notify('error', errorMessage(reason))
    } finally {
      setTaskAction('')
    }
  }

  async function refreshTask(task: VideoTask) {
    setTaskAction(task.id)
    try {
      const updated = await userApi.getVideoTask(task.id)
      setTasks((items) => mergeTasks(items, updated))
    } catch (reason) {
      app.notify('error', errorMessage(reason))
    } finally {
      setTaskAction('')
    }
  }

  const ParameterHost = compactViewport ? OverlayPortal : Fragment
  return (
    <div className="u-workbench">
      <ParameterHost><aside className={cn('u-controls', controlsOpen && 'is-open')} aria-label="视频生成参数">
        <header className="u-controls__head">
          <button
            type="button"
            className="cr-panel-head__toggle"
            aria-expanded={controlsOpen}
            aria-controls="video-parameter-controls"
            onClick={() => { if (compactViewport) setControlsOpen((open) => !open) }}
          >
            <span className="cr-panel-head__title">
              <strong>生成设置</strong>
              <span className="u-meta">视频 · {taskTypeLabels[draft.task_type]}</span>
            </span>
            {compactViewport ? <ChevronUp className={cn('cr-panel-head__chevron', compactViewport && !controlsOpen && 'is-collapsed')} size={16} aria-hidden="true" /> : null}
          </button>
          <span className="u-chip u-chip--mono">{quote ? pointsText(quote.display_points ?? quote.max_reserved_points) : '--'}</span>
        </header>

        <div id="video-parameter-controls" className="u-controls__body" aria-hidden={compactViewport && !controlsOpen || undefined} inert={compactViewport && !controlsOpen ? true : undefined}>
          <div className="cr-group">
            <div className="cr-group__head"><h4>模型</h4><span>{modelOptions.length} 可用</span></div>
            <ModelGroupSelect options={modelOptions} groups={capability?.route_model_groups} value={draft.route_model_code} onChange={(route_model_code) => patchDraft({ route_model_code })} />
            <div className="vd-modes" role="group" aria-label="生成方式">
              {displayModel.task_types.map((value) => (
                <button
                  key={value}
                  type="button"
                  className={cn('vd-mode', draft.task_type === value && 'is-active')}
                  aria-pressed={draft.task_type === value}
                  onClick={() => {
                    patchDraft({ task_type: value })
                  }}
                >
                  <b>{taskTypeLabels[value]}</b>
                  <span>{taskTypeHints[value]}</span>
                </button>
              ))}
            </div>
          </div>

          {draft.task_type !== 'text_to_video' || draft.inputs.length ? (
            <div className="cr-group">
              <div className="cr-group__head"><h4>输入素材</h4><span>{VIDEO_TASK_INPUT_ROLES[draft.task_type].length} 个槽位</span></div>
              <div className="vd-frames">
                {Array.from(new Set([...VIDEO_TASK_INPUT_ROLES[draft.task_type], ...draft.inputs.map((input) => input.role)])).flatMap((role) => {
                  const inputs = draft.inputs.filter((input) => input.role === role)
                  return [...inputs, { asset_id: '', role, ordinal: inputs.length }].map((input, index) => <FrameInput
                    key={`${role}-${input.asset_id || 'add'}`} role={role}
                    label={`${videoTaskInputLabel(role)}${inputs.length > 1 ? ` ${index + 1}` : ''}${input.asset_id && !selectedAssets[input.asset_id] ? ` · ${input.asset_id}` : ''}`}
                    error={fieldErrorFor(fieldErrors, `inputs.${role}`)} asset={selectedAssets[input.asset_id]}
                    onSelect={() => setPickerRole(role)} onPreview={setPreviewAsset}
                    onRemove={() => patchDraft({ inputs: draft.inputs.filter((item) => item !== input && item.asset_id !== input.asset_id) })} />)
                })}
              </div>
            </div>
          ) : null}

          <div className="cr-group">
            <div className="cr-group__head">
              <label className="u-label">提示词</label>
              <span>{draft.prompt_template.length} / 2000</span>
            </div>
            <div className="u-prompt-box cr-prompt">
              <div className="u-prompt-box__head">
                <span className="u-meta">描述镜头运动与光线</span>
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
                ref={promptEditorRef}
                value={draft.prompt_template}
                assets={promptAssets}
                variables={promptVariables}
                accessToken={app.session?.token}
                disabled={submitting}
                placeholder="描述想要生成的画面...，输入 @ 引用首帧/尾帧资产，输入 $ 添加变量"
                onChange={changePrompt}
                onAddAsset={openFramePicker}
                onPasteFiles={(files) => { void uploadReferences(files) }}
              />
              <div className="u-prompt-box__foot">
                <span>@ 引用首帧/尾帧 · $ 插入变量</span>
                <span>建议 ≤ 2000 字</span>
              </div>
            </div>
            <PromptVariableForm template={draft.prompt_template} values={promptVariables} disabled={submitting} onChange={changePromptVariable} />
            <FieldError message={fieldErrors.prompt_template ?? fieldErrors.prompt} />
            <FieldError message={fieldErrors.reference_bindings} />
            <FieldError message={fieldErrors.prompt_variables} />
          </div>

          <div className="cr-group">
            <div className="cr-group__head"><h4>参数</h4></div>
            <div className="vd-grid2">
              <Choice label="时长" error={fieldErrors.duration_seconds} value={draft.duration_seconds} values={displayOptions.durations} format={(value) => `${value} 秒`} onChange={(duration_seconds) => patchDraft({ duration_seconds })} />
              <Choice label="清晰度" error={fieldErrors.resolution} value={draft.resolution} values={resolutionOptions} format={(value) => value.toUpperCase()} onChange={(resolution) => patchDraft({ resolution })} />
              <Choice label="比例" error={fieldErrors.aspect_ratio} value={draft.aspect_ratio} values={aspectRatioOptions} onChange={(aspect_ratio) => patchDraft({ aspect_ratio })} />
              <Choice label="数量" error={fieldErrors.output_count} value={draft.output_count} values={Array.from({ length: Math.max(1, displayModel.max_output_count) }, (_, index) => index + 1)} onChange={(output_count) => patchDraft({ output_count })} />
            </div>
            {displayOptions.audio_generation ? (
              <label className="u-switch">
                <input type="checkbox" checked={draft.generate_audio} disabled={!audioAvailable && !draft.generate_audio} onChange={(event) => patchDraft({ generate_audio: event.target.checked })} />
                <span className="u-switch__track" />
                {draft.generate_audio ? <Volume2 size={16} /> : <VolumeX size={16} />}
                生成音频
              </label>
            ) : null}
            <FieldError message={fieldErrors.generate_audio} />
          </div>

          <div className="vd-quote" aria-label="视频费用预估">
            <div className="u-estimate__row"><span>单价 · {draft.duration_seconds} 秒</span><b>{quoteBreakdown ? pointsText(quoteBreakdown.unitPoints) : '--'}</b></div>
            <div className="u-estimate__row"><span>预计总价 · {quoteBreakdown?.outputCount ?? draft.output_count} 个</span><b>{quoteBreakdown ? pointsText(quoteBreakdown.estimatedPoints) : '--'}</b></div>
            <div className="u-estimate__row"><span>最大预留</span><b>{quoteBreakdown ? pointsText(quoteBreakdown.maxReservedPoints) : '--'}</b></div>
            <div className="u-estimate__row"><span>可用余额</span><b>{quoteBreakdown?.availablePoints ? pointsText(quoteBreakdown.availablePoints) : '--'}</b></div>
          </div>
          {error ? <p className="cr-inline-error" role="alert">{error}</p> : null}
        </div>

        <div className="u-controls__foot">
          <div className="u-price-row">
            <span>提交时预留</span>
            <strong>{quote ? pointsText(quote.display_points ?? quote.max_reserved_points) : '--'}</strong>
          </div>
          <div className="u-meta">提交后按实际生成秒数结算，失败自动退回预留</div>
          {pendingUploads > 0 ? <p className="u-meta" role="status">引用素材正在上传，完成后重新估价</p> : null}
          {invalidReason ? <p className="cr-inline-error" role="alert">{invalidReason}</p> : null}
          {estimateError ? <p className="cr-inline-error" role="status">{estimateError}</p> : null}
          {quote?.sufficient === false ? <p className="cr-inline-error" role="status">积分不足</p> : null}
          <button className="u-btn u-btn--primary u-btn--lg u-btn--block" type="button" disabled={pendingUploads > 0 || Boolean(invalidReason) || !quote || quote.sufficient === false || submitting} onClick={() => void submit()}>
            {submitting ? <><span className={userState.spinner} />提交中...</> : <><Sparkles size={17} />生成视频</>}
          </button>
        </div>
      </aside></ParameterHost>

      <section className="u-canvas" aria-label="视频任务">
        <div className="u-canvas__head">
          <div className="u-segment" role="tablist" aria-label="创作输出">
            <button type="button" role="tab" aria-selected={taskTab === 'current'} className={cn(taskTab === 'current' && 'is-active')} onClick={() => setTaskTab('current')}>当前创作</button>
            <button type="button" role="tab" aria-selected={taskTab === 'history'} className={cn(taskTab === 'history' && 'is-active')} onClick={() => setTaskTab('history')}>历史创作</button>
          </div>
          <span className="u-meta">队列 {orderedTasks.length} · {runningCount} 个任务生成中</span>
        </div>

        {taskTab === 'current' && currentTask ? <StageRail nodes={videoTaskRailView(currentTask).rail} trailing={stageLabels[currentTask.progress_stage || currentTask.status] ?? currentTask.status} /> : null}
        {currentTask ? <div className="cr-sr-only"><WorkspaceStatusRail task={videoTaskRailView(currentTask)} startedAt={currentTask.created_at} finishedAt={currentTask.updated_at} /></div> : null}
        {taskTab === 'current' && orderedTasks.length > 1 ? <VideoRecentStrip tasks={orderedTasks} activeTaskID={currentTask?.id} onSelectTask={(task) => { setSelectedTaskID(task.id); setTaskTab('current') }} /> : null}

        <div className="cr-canvas-panel">
          {taskTab === 'current' ? (
            currentTask ? (
              <VideoCurrentTaskCard task={currentTask} busy={historyActions.busy || taskAction === currentTask.id} onRetry={() => void retryHistoryTask(currentTask)} editing={editingAsset} onCancel={() => void cancelTask(currentTask)} onDetail={() => setDetailTask(currentTask)} onReuse={() => reuseDraft(currentTask)} onResult={(assetID) => void openResultPreview(assetID)} onExtend={(assetID) => void editFromResult(assetID)} onDownload={(assetID) => void downloadResult(assetID)} onCopyPrompt={() => { void copyText(currentTask.prompt_template).then(() => app.notify('success', '提示词已复制')).catch(() => app.notify('error', '复制失败')) }} />
            ) : (
              <div className="u-stage cr-stage--empty">
                <div className="u-empty cr-stage-empty">
                  <span className="u-empty__icon"><Film size={24} /></span>
                  <h3>还没有视频任务</h3>
                  <p>填写参数并确认报价后，当前任务进度会展示在这里。</p>
                </div>
              </div>
            )
          ) : (
            <>
              <div className="u-toolbar" style={{ margin: 0 }}>
                <input className="u-input" type="search" value={historyQuery} onChange={(event) => setHistoryQuery(event.target.value)} placeholder="搜索提示词或任务 ID" aria-label="搜索历史任务" />
                <select className="u-filter" value={historyStatus} onChange={(event) => setHistoryStatus(event.target.value)} aria-label="按状态筛选">
                  <option value="all">全部状态</option>
                  <option value="succeeded">成功</option>
                  <option value="partial">部分成功</option>
                  <option value="failed">失败</option>
                  <option value="cancelled">已取消</option>
                </select>
                <button className="u-btn u-btn--sm" type="button" disabled={historyActions.busy || !projects.selectedProjectID} onClick={confirmClearFailedHistory}>{historyActions.busy ? '处理中...' : '清空全部失败记录'}</button>
              </div>
              {historyTasks.length ? (
                <div className="vd-history">
                  {historyTasks.map((task) => <VideoHistoryCard key={task.id} task={task} busy={historyActions.busy || taskAction === task.id} onRetry={() => void retryHistoryTask(task)} onHide={() => confirmHideHistoryTask(task)} onRefresh={() => void refreshTask(task)} onDetail={() => setDetailTask(task)} onReuse={() => reuseDraft(task)} onResult={(assetID) => void openResultPreview(assetID)} />)}
                </div>
              ) : <EmptyState title="暂无历史创作" detail="完成一次视频创作后，记录会展示在这里。" icon={<Film size={24} />} />}
            </>
          )}
        </div>
      </section>

      {historyActions.feedback}
      {pickerRole ? <MediaAssetPicker projectID={projects.selectedProjectID} mediaTypes={[PICKER_MEDIA_TYPES[pickerRole]]} multiple={pickerRole.startsWith('reference_')} title={PICKER_TITLES[pickerRole]} onClose={() => setPickerRole(null)} onConfirm={(assets) => {
        if (admitAssets(assets, pickerRole)) setPickerRole(null)
      }} /> : null}
      {previewAsset ? <MediaPreviewDialog asset={previewAsset} projects={projects.projects} creationActions={undefined} onClose={() => setPreviewAsset(null)} onChanged={(asset) => { setPreviewAsset(asset); setSelectedAssets((current) => ({ ...current, [asset.id]: asset })) }} onDeleted={(asset) => { setPreviewAsset(null); setSelectedAssets((current) => { const next = { ...current }; delete next[asset.id]; return next }) }} /> : null}
      {detailTask ? <VideoTaskDetailDialog task={detailTask} onClose={() => setDetailTask(null)} onReuse={() => { reuseDraft(detailTask); setDetailTask(null) }} onResult={(assetID) => void userApi.getMediaAsset(assetID).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught)))} /> : null}
      {promptExpanded ? (
        <PromptEditorDialog
          prompt={draft.prompt_template}
          assets={promptAssets}
          variables={promptVariables}
          accessToken={app.session?.token}
          optimization={promptOptimization}
          onPromptChange={changePrompt}
          onVariableChange={changePromptVariable}
          onAddAsset={() => { setPromptExpanded(false); openFramePicker() }}
          onPasteFiles={(files) => { void uploadReferences(files) }}
          onClose={() => { setPromptExpanded(false); window.setTimeout(() => promptEditorRef.current?.focus(), 0) }}
          onOptimize={() => void startPromptOptimization()}
          onConfirm={() => void confirmOptimization()}
          onApply={applyOptimization}
          onCancel={cancelOptimization}
          onUndo={undoOptimization}
        />
      ) : null}
      {!promptExpanded && promptOptimization.stage !== 'idle' && promptOptimization.stage !== 'applied' ? (
        <Modal title="优化提示词" onClose={cancelOptimization}>
          <PromptOptimizationPanel state={promptOptimization} onConfirm={() => void confirmOptimization()} onApply={applyOptimization} onCancel={cancelOptimization} />
        </Modal>
      ) : null}
    </div>
  )
}

function estimateRequest(projectId: string, draft: VideoDraft, assets: Record<string, MediaAsset>): VideoEstimateRequest {
  // Bindings must mirror the {{@名称}} references in the template exactly —
  // the backend rejects extra bindings (reference_extra). Frame inputs ride
  // the structured inputs list, so only bind assets the prompt mentions.
  const referenced = promptReferenceNames(draft.prompt_template)
  const bindings = new Map<string, string>()
  for (const binding of draft.reference_bindings ?? []) {
    const name = binding.name.trim().normalize('NFC')
    if (referenced.has(name) && draft.inputs.some((input) => input.asset_id === binding.asset_id) && assets[binding.asset_id]) bindings.set(name, binding.asset_id)
  }
  for (const input of draft.inputs) {
    const asset = assets[input.asset_id]
    if (!asset || !input.asset_id.trim()) continue
    const name = asset.name.trim().normalize('NFC')
    if (!name || !referenced.has(name) || bindings.has(name)) continue
    bindings.set(name, asset.id)
  }
  return { project_id: projectId, route_model_code: draft.route_model_code, task_type: draft.task_type, prompt_template: draft.prompt_template, prompt_variables: draft.prompt_variables, reference_bindings: Array.from(bindings, ([name, asset_id]): PromptReferenceBinding => ({ name, asset_id })), inputs: draft.inputs.filter((item) => item.asset_id.trim()), duration_seconds: draft.duration_seconds, resolution: draft.resolution, aspect_ratio: draft.aspect_ratio, audio_mode: draft.generate_audio ? 'generated' : 'silent', output_count: draft.output_count }
}

const PICKER_MEDIA_TYPES: Record<VideoDraftInputRole, 'image' | 'video' | 'audio'> = { first_frame: 'image', last_frame: 'image', reference_image: 'image', reference_video: 'video', reference_audio: 'audio' }
const PICKER_TITLES: Record<VideoDraftInputRole, string> = { first_frame: '选择首帧图片', last_frame: '选择尾帧图片', reference_image: '选择参考图片', reference_video: '选择参考视频', reference_audio: '选择参考音频' }

function inputRoleIcon(role: VideoDraftInputRole, size: number) {
  if (role === 'reference_video') return <Film size={size} />
  if (role === 'reference_audio') return <Volume2 size={size} />
  return <ImageIcon size={size} />
}

function assetMetaLine(asset: MediaAsset) {
  if (asset.width && asset.height) return `${asset.width} × ${asset.height}`
  if (asset.duration_ms) return `${Math.round(asset.duration_ms / 1000)} 秒`
  return asset.mime_type
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

function FrameInput({ role, label, asset, error, onSelect, onPreview, onRemove }: { role: VideoDraftInputRole; label: string; asset?: MediaAsset; error?: string; onSelect: () => void; onPreview: (asset: MediaAsset) => void; onRemove: () => void }) {
  return (
    <div className="vd-frame">
      <span>{label}</span>
      {asset ? (
        <div className="cr-frame-asset">
          <button type="button" className="vd-frame__slot" onClick={() => onPreview(asset)}>
            {inputRoleIcon(role, 18)}
            <strong>{asset.name}</strong>
            <small>{assetMetaLine(asset)}</small>
          </button>
          <div className="cr-frame-actions">
            <button type="button" className="u-btn u-btn--sm" title={`更换${label}`} onClick={onSelect}><RefreshCw size={14} /></button>
            <button type="button" className="u-btn u-btn--sm" title={`移除${label}`} onClick={onRemove}><X size={14} /></button>
          </div>
        </div>
      ) : (
        <button type="button" className="vd-frame__slot" onClick={onSelect}>
          {inputRoleIcon(role, 20)}
          <span>选择{PICKER_TITLES[role].replace('选择', '')}</span>
        </button>
      )}
      <FieldError message={error} />
    </div>
  )
}

function Choice<T extends string | number>({ label, value, values, error, format = String, onChange }: { label: string; value: T; values: T[]; error?: string; format?: (value: T) => string; onChange: (value: T) => void }) {
  return (
    <label className="u-field">
      <span className="u-label">{label}</span>
      <select className="u-select" value={value} onChange={(event) => { const selected = values.find((item) => String(item) === event.target.value); if (selected !== undefined) onChange(selected) }}>
        {values.map((item) => <option key={item} value={item}>{format(item)}</option>)}
      </select>
      <FieldError message={error} />
    </label>
  )
}

function videoTaskBadges(task: VideoTask) {
  return [
    `${task.duration_seconds} 秒`,
    task.resolution.toUpperCase(),
    task.aspect_ratio,
    task.audio_mode === 'generated' || task.generate_audio ? '有声' : '静音',
    `${task.requested_output_count} 个`,
  ]
}

const videoProgressContract: Array<{ phase: WorkspaceProgressNode['phase']; label: string }> = [
  { phase: 'validating', label: '参数校验' },
  { phase: 'routing', label: '模型路由' },
  { phase: 'queued', label: '队列调度' },
  { phase: 'generating', label: '视频生成' },
  { phase: 'storing', label: '结果入库' },
  { phase: 'settling', label: '积分结算' },
]

const videoStagePhase: Record<string, WorkspaceProgressNode['phase']> = {
  queued: 'validating',
  submitting: 'routing',
  reconciling: 'routing',
  provider_queued: 'queued',
  provider_running: 'generating',
  artifact_pending: 'storing',
  recovery_required: 'storing',
  saving: 'storing',
  succeeded: 'settling',
  partial: 'settling',
  failed: 'generating',
  cancelled: 'generating',
}

function videoProgressNodes(task: VideoTask): WorkspaceProgressNode[] {
  const failed = ['failed', 'cancelled'].includes(task.status)
  const succeeded = ['succeeded', 'partial'].includes(task.status)
  const stagePhase = videoStagePhase[(task.progress_stage ?? task.status ?? '').trim().toLowerCase()]
  const fallbackPhase: WorkspaceProgressNode['phase'] = task.status === 'queued' ? 'validating' : 'generating'
  const currentIndex = Math.max(0, videoProgressContract.findIndex((item) => item.phase === (stagePhase ?? fallbackPhase)))
  return videoProgressContract.map((item, index) => {
    if (failed) {
      return { phase: item.phase, label: item.label, status: index < currentIndex ? 'done' : index === currentIndex ? 'failed' : 'idle' }
    }
    if (succeeded) return { phase: item.phase, label: item.label, status: 'done' }
    if (index < currentIndex) return { phase: item.phase, label: item.label, status: 'done' }
    if (index === currentIndex) return { phase: item.phase, label: item.label, status: 'active' }
    return { phase: item.phase, label: item.label, status: 'idle' }
  })
}

function videoTaskRailView(task: VideoTask): WorkspaceTaskView {
  const terminal = isVideoTaskTerminal(task)
  const state: WorkspaceTaskView['state'] = terminal
    ? (task.status === 'succeeded' ? 'success' : task.status === 'partial' ? 'partial' : 'failure')
    : (task.status === 'queued' || task.progress_stage === 'queued' ? 'queued' : 'running')
  return {
    state,
    title: taskTypeLabels[task.task_type] ?? task.task_type,
    detail: task.progress_message || task.prompt_template.slice(0, 80) || '视频任务处理中',
    rail: videoProgressNodes(task),
    resultCount: task.items.filter((item) => item.result_asset_id).length,
    requestedCount: task.requested_output_count,
  }
}

function VideoRecentStrip({ tasks, activeTaskID, onSelectTask }: { tasks: VideoTask[]; activeTaskID?: string; onSelectTask: (task: VideoTask) => void }) {
  return (
    <section className="cr-history-section" aria-label="最近创作">
      <div className="cr-section-head">
        <h3 className="u-h3">最近任务</h3>
        <span className="u-meta">{tasks.length} 条</span>
      </div>
      <div className="u-history-strip">
        {tasks.slice(0, 8).map((task) => {
          const stage = task.progress_stage || task.status
          const resultAssetID = task.items.find((item) => item.result_asset_id)?.result_asset_id
          return (
            <button
              key={task.id}
              type="button"
              className={cn('u-history-card cr-history-card', task.id === activeTaskID && 'is-active')}
              aria-current={task.id === activeTaskID ? 'true' : undefined}
              onClick={() => onSelectTask(task)}
            >
              <span className="u-history-card__media">
                {resultAssetID ? <StripPoster assetID={resultAssetID} /> : <span className="cr-history-state">{stageLabels[stage] ?? stage}</span>}
              </span>
              <span className="u-history-card__body">
                <strong>{taskTypeLabels[task.task_type] ?? task.task_type}</strong>
                <span className="u-meta">{stageLabels[stage] ?? stage} · {formatVideoHistoryTime(task.created_at)}</span>
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function StripPoster({ assetID }: { assetID: string }) {
  const posterURL = usePosterURL(assetID)
  return posterURL ? <img className="size-full object-cover" src={posterURL} alt="" loading="lazy" draggable={false} /> : null
}

function isVideoTaskTerminal(task: VideoTask) {
  return ['succeeded', 'partial', 'failed', 'cancelled'].includes(task.status)
}

// Fresh result assets briefly 409 on poster access while the media pipeline
// generates the derivative, so retry with backoff instead of giving up.
function usePosterURL(assetID: string | undefined) {
  const app = useApp()
  const [posterURL, setPosterURL] = useState('')
  useEffect(() => {
    if (!assetID) {
      setPosterURL('')
      return undefined
    }
    let alive = true
    let attempt = 0
    let timer = 0
    setPosterURL('')
    const run = () => {
      void userApi.getMediaAssetAccess(assetID, 'poster').then((access) => {
        if (alive) setPosterURL(userApi.mediaAssetUrl(access.url, app.session?.token))
      }).catch(() => {
        if (!alive || attempt >= 5) return
        attempt += 1
        timer = window.setTimeout(run, attempt * 2000)
      })
    }
    run()
    return () => { alive = false; window.clearTimeout(timer) }
  }, [assetID, app.session?.token])
  return posterURL
}

function VideoResultThumb({ assetID, alt }: { assetID: string; alt: string }) {
  const app = useApp()
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const posterURL = usePosterURL(assetID)
  const [hoverURL, setHoverURL] = useState('')

  const beginHover = () => {
    if (hoverURL) {
      void videoRef.current?.play()
      return
    }
    void userApi.getMediaAssetAccess(assetID, 'hover').then((access) => setHoverURL(userApi.mediaAssetUrl(access.url, app.session?.token))).catch(() => undefined)
  }
  const endHover = () => {
    videoRef.current?.pause()
  }

  return (
    <span className="cr-history-media" aria-hidden="true" onMouseEnter={beginHover} onMouseLeave={endHover}>
      {posterURL && !hoverURL ? <img className="size-full object-cover" src={posterURL} alt={alt} loading="lazy" draggable={false} /> : null}
      {hoverURL ? <video ref={videoRef} className="size-full object-cover" src={hoverURL} muted loop autoPlay playsInline preload="metadata" draggable={false} /> : null}
      {!posterURL && !hoverURL ? <span className="cr-card-state"><Film size={22} /></span> : null}
      <span className="u-play"><span><Play size={16} fill="currentColor" /></span></span>
    </span>
  )
}

function videoAspectRatioStyle(ratio: string | undefined) {
  const [w, h] = (ratio ?? '16:9').split(':').map((part) => Number(part.trim()))
  if (!w || !h) return undefined
  return { '--video-result-ratio': `${w} / ${h}` } as React.CSSProperties
}

function VideoCurrentTaskCard({ task, onRetry, busy, onCancel, onDetail, onReuse, onResult, onExtend, onDownload, onCopyPrompt, editing }: { task: VideoTask; onRetry: () => void; busy: boolean; onCancel: () => void; onDetail: () => void; onReuse: () => void; onResult: (assetID: string) => void; onExtend: (assetID: string) => void; onDownload: (assetID: string) => void; onCopyPrompt: () => void; editing: boolean }) {
  const terminal = isVideoTaskTerminal(task)
  const stage = task.progress_stage || task.status
  const statusLabel = stageLabels[stage] ?? stage
  const reserved = task.reserved_points ?? task.estimated_points ?? '--'
  const charged = task.actual_points && task.actual_points !== '0.00000' ? pointsText(task.actual_points) : `预留 ${pointsText(reserved)}`
  const resultItems = task.items.filter((item) => item.result_asset_id)
  const failed = task.status === 'failed' || task.status === 'cancelled'
  const statusTone = task.status === 'succeeded' ? 'cr-chip--ok' : task.status === 'partial' ? 'cr-chip--warn' : failed ? 'cr-chip--bad' : 'u-chip--accent'
  const audioLabel = task.audio_mode === 'generated' || task.generate_audio ? '有声' : '静音'
  return (
    <article className="cr-video-task" data-status={task.status}>
      <div className="cr-task-head">
        <div className="cr-task-badges">
          <span className={cn('u-chip', statusTone)}>{!terminal ? <span className="u-dot" /> : null}{statusLabel}</span>
          {videoTaskBadges(task).map((badge) => <span key={badge} className="u-chip u-chip--mono">{badge}</span>)}
        </div>
        <div className="cr-task-actions">
          <button className="u-btn u-btn--sm" type="button" onClick={onDetail}>详情</button>
          {!terminal ? <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={onCancel}>取消</button> : null}
        </div>
      </div>

      {!terminal ? (
        <div className="cr-task-progress" role="status">
          <div className="u-progress"><span /></div>
          <span className="u-meta inline-flex items-center gap-2"><LoaderCircle className="animate-spin" size={14} />{statusLabel} · 预留 {pointsText(reserved)}</span>
        </div>
      ) : null}

      {failed ? (
        <div className="u-banner u-banner--danger" role="alert">
          <div>
            <strong>{task.status === 'cancelled' ? '任务已取消' : '生成失败'}</strong>
            <p style={{ margin: 0 }}>{task.items.find((item) => item.error_message)?.error_message || task.progress_message || '预留积分已退回。'}</p>
            <span className="u-meta inline-flex items-center gap-1">任务ID: {task.id}<InlineCopyButton text={task.id} label="复制任务ID" /></span>
            <div className="cr-task-actions">
              <button className="u-btn u-btn--sm u-btn--primary" type="button" onClick={onDetail}>详情</button>
              {task.status === 'failed' ? <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={onRetry}>{busy ? '提交中...' : '重试（新任务）'}</button> : null}
              <button className="u-btn u-btn--sm" type="button" onClick={onReuse}>复用参数</button>
            </div>
          </div>
        </div>
      ) : null}

      {resultItems.length ? (
        resultItems.length === 1 ? (
          <div className="u-stage">
            <VideoResultFigure
              assetID={resultItems[0].result_asset_id!}
              ordinal={resultItems[0].ordinal}
              aspectRatio={task.aspect_ratio}
              caption={`#${resultItems[0].ordinal + 1} · ${Number(task.duration_seconds).toFixed(1)}s · ${task.resolution.toUpperCase()} · ${task.aspect_ratio} · ${audioLabel} · ${charged} · ${settlementLabels[task.settlement_status ?? ''] ?? '待结算'}`}
              editing={editing}
              onOpen={() => resultItems[0].result_asset_id && onResult(resultItems[0].result_asset_id)}
              onCopyPrompt={onCopyPrompt}
              onReuse={onReuse}
              onExtend={() => resultItems[0].result_asset_id && onExtend(resultItems[0].result_asset_id)}
              onDetail={onDetail}
              onDownload={() => resultItems[0].result_asset_id && onDownload(resultItems[0].result_asset_id)}
            />
          </div>
        ) : (
          <div className="u-grid cr-video-results">
            {resultItems.map((item) => (
              <div className="u-stage cr-stage--compact" key={item.id}>
                <VideoResultFigure
                  assetID={item.result_asset_id!}
                  ordinal={item.ordinal}
                  aspectRatio={task.aspect_ratio}
                  caption={`#${item.ordinal + 1} · ${Number(task.duration_seconds).toFixed(1)}s · ${task.resolution.toUpperCase()} · ${task.aspect_ratio} · ${audioLabel} · ${charged} · ${settlementLabels[task.settlement_status ?? ''] ?? '待结算'}`}
                  editing={editing}
                  onOpen={() => item.result_asset_id && onResult(item.result_asset_id)}
                  onCopyPrompt={onCopyPrompt}
                  onReuse={onReuse}
                  onExtend={() => item.result_asset_id && onExtend(item.result_asset_id)}
                  onDetail={onDetail}
                  onDownload={() => item.result_asset_id && onDownload(item.result_asset_id)}
                />
              </div>
            ))}
          </div>
        )
      ) : null}

      {!terminal && !resultItems.length ? (
        <div className="u-stage cr-stage--working">
          <div className="cr-working">
            <LoaderCircle className="animate-spin" size={20} />
            <strong>{statusLabel}</strong>
            <span className="u-meta">{task.progress_message || '正在处理'}</span>
          </div>
        </div>
      ) : null}

      {terminal && !failed ? (
        <div className="cr-task-meta">
          <p className="cr-task-prompt">{task.prompt_template || '未填写提示词'}</p>
          <div className="u-meta cr-task-meta__row">
            <span>模型: {task.route_model_code}</span>
            <span>时长: {task.duration_seconds} 秒</span>
            <span>分辨率: {task.resolution.toUpperCase()}</span>
            <span>比例: {task.aspect_ratio}</span>
            <span>{audioLabel}</span>
            <span>数量: {task.requested_output_count}</span>
            <span>任务时间: {formatVideoHistoryTime(task.created_at)}</span>
            <span>消耗: {charged}</span>
            <span className="inline-flex items-center gap-1">任务ID: {task.id}<InlineCopyButton text={task.id} label="复制任务ID" /></span>
          </div>
        </div>
      ) : null}
    </article>
  )
}

function VideoResultFigure({ assetID, ordinal, aspectRatio, caption, editing, onOpen, onCopyPrompt, onReuse, onExtend, onDetail, onDownload }: { assetID: string; ordinal: number; aspectRatio: string; caption: string; editing: boolean; onOpen: () => void; onCopyPrompt: () => void; onReuse: () => void; onExtend: () => void; onDetail: () => void; onDownload: () => void }) {
  const app = useApp()
  const posterURL = usePosterURL(assetID)
  const [hoverURL, setHoverURL] = useState('')
  const videoRef = useRef<HTMLVideoElement | null>(null)

  const beginHover = () => {
    if (hoverURL) {
      void videoRef.current?.play()
      return
    }
    void userApi.getMediaAssetAccess(assetID, 'hover').then((access) => setHoverURL(userApi.mediaAssetUrl(access.url, app.session?.token))).catch(() => undefined)
  }
  const endHover = () => {
    videoRef.current?.pause()
  }

  return (
    <figure className="u-result u-result--video cr-result cr-result--video" style={videoAspectRatioStyle(aspectRatio)} onMouseEnter={beginHover} onMouseLeave={endHover}>
      <button type="button" className="cr-result__media" onClick={onOpen} aria-label="播放生成视频">
        {posterURL && !hoverURL ? <img className="cr-result__img" src={posterURL} alt={`方案 ${ordinal + 1}`} draggable={false} /> : null}
        {hoverURL ? <video ref={videoRef} className="cr-result__img" src={hoverURL} muted loop autoPlay playsInline preload="metadata" draggable={false} /> : null}
        {!posterURL && !hoverURL ? <span className="cr-result__placeholder"><Film size={28} /></span> : null}
      </button>
      <button type="button" className="cr-result__play" onClick={onOpen} aria-label="播放视频"><Play size={20} fill="currentColor" /></button>
      <div className="u-result__bar">
        <div className="u-result__ops">
          <button className="u-result__op" type="button" title="复制提示词" aria-label="复制提示词" onClick={onCopyPrompt}><Copy size={15} /></button>
          <button className="u-result__op" type="button" title="复用参数" aria-label="复用参数" onClick={onReuse}><RotateCcw size={15} /></button>
          <button className="u-result__op" type="button" title="续写延长" aria-label="续写延长视频" disabled={editing} onClick={onExtend}><FastForward size={15} /></button>
          <button className="u-result__op" type="button" title="详情" aria-label="查看任务详情" onClick={onDetail}><Info size={15} /></button>
          <button className="u-result__op" type="button" title="下载" aria-label="下载视频" onClick={onDownload}><Download size={15} /></button>
        </div>
        <span className="u-result__caption">{caption}</span>
      </div>
    </figure>
  )
}

function VideoHistoryCard({ task, busy, onRetry, onHide, onRefresh, onDetail, onReuse, onResult }: { task: VideoTask; busy: boolean; onRetry: () => void; onHide: () => void; onRefresh: () => void; onDetail: () => void; onResult: (assetID: string) => void; onReuse: () => void }) {
  const terminal = isVideoTaskTerminal(task)
  const stage = task.progress_stage || task.status
  const resultItem = task.items.find((item) => item.result_asset_id)
  return (
    <article className="u-card cr-history-video" data-status={task.status}>
      <button type="button" className="u-card__media cr-card-media-button" onClick={() => resultItem ? onResult(resultItem.result_asset_id!) : onDetail()} title={resultItem ? '播放结果' : '查看任务详情'}>
        {resultItem ? <VideoResultThumb assetID={resultItem.result_asset_id!} alt={task.prompt_template.slice(0, 30)} /> : terminal ? <span className="cr-card-state"><Ban size={20} /><span>{task.status === 'cancelled' ? '已取消' : '生成失败'}</span></span> : <span className="cr-card-state"><LoaderCircle className="animate-spin" size={20} /><span>{stageLabels[stage] ?? stage}</span></span>}
        <span className="u-card__tag">{taskTypeLabels[task.task_type] ?? task.task_type}</span>
      </button>
      <div className="u-card__body">
        <div className="u-card__title">{task.prompt_template || '未填写提示词'}</div>
        <div className="u-card__meta">
          <span>{formatVideoHistoryTime(task.created_at)}</span>
          <span>{task.actual_points && task.actual_points !== '0.00000' ? pointsText(task.actual_points) : `预留 ${pointsText(task.reserved_points ?? task.estimated_points ?? '0')}`}</span>
        </div>
        <div className="cr-task-actions">
          <button className="u-btn u-btn--sm" type="button" onClick={onDetail}>详情</button>
          {task.status === 'failed' ? <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={onRetry}>{busy ? '处理中...' : '重试（新任务）'}</button> : null}
          {terminal ? <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={onHide}>删除记录</button> : null}
          {terminal ? <button className="u-btn u-btn--sm" type="button" onClick={onReuse}>复用参数</button> : <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={onRefresh}><RefreshCw className={busy ? 'animate-spin' : undefined} size={14} />刷新</button>}
        </div>
        <div className="u-meta inline-flex items-center gap-1"><span title={task.id}>任务ID: {task.id.slice(0, 8)}…</span><InlineCopyButton text={task.id} label="复制任务ID" /></div>
      </div>
    </article>
  )
}

function formatVideoHistoryTime(value?: string) {
  if (!value) return '未知时间'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

// Captures the last decodable frame of the video as a PNG blob so a finished
// result can seed the next generation as a first-frame input. The presigned
// storage URL is CORS-enabled, so the canvas stays untainted.
function captureVideoFrameBlob(url: string): Promise<Blob> {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video')
    video.crossOrigin = 'anonymous'
    video.muted = true
    video.preload = 'auto'
    video.src = url
    const fail = (message: string) => reject(new Error(message))
    video.onerror = () => fail('视频加载失败，无法截取帧')
    video.onloadedmetadata = () => {
      const seekTo = Math.max(0, (video.duration || 0) - 0.15)
      video.onseeked = () => {
        try {
          const canvas = document.createElement('canvas')
          canvas.width = video.videoWidth
          canvas.height = video.videoHeight
          const context = canvas.getContext('2d')
          if (!context || !canvas.width || !canvas.height) {
            fail('画布初始化失败')
            return
          }
          context.drawImage(video, 0, 0)
          canvas.toBlob((blob) => blob ? resolve(blob) : fail('截帧导出失败'), 'image/png')
        } catch {
          fail('截帧失败，存储可能未开放跨域')
        }
      }
      video.currentTime = seekTo
    }
  })
}

async function sha256Hex(buffer: ArrayBuffer) {
  const digest = await crypto.subtle.digest('SHA-256', buffer)
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

// Single-shot upload through the storage presign flow (data plane PUT).
async function uploadSingleMediaAsset(file: File, projectID: string): Promise<MediaAsset> {
  const session = await userApi.initMediaUpload({
    project_id: projectID, filename: file.name, media_type: 'image',
    mime_type: file.type, size_bytes: file.size,
  }, crypto.randomUUID())
  const buffer = await file.arrayBuffer()
  const checksum = await sha256Hex(buffer)
  const target = await userApi.signMediaUploadPart(session.id, 1, checksum)
  const response = await fetch(target.url, { method: 'PUT', body: file, headers: target.headers })
  if (!response.ok) throw new Error(`对象存储上传失败（${response.status}）`)
  const etag = response.headers.get('etag')?.replace(/^"|"$/g, '')
  if (!etag) throw new Error('对象存储未返回 ETag')
  return userApi.completeMediaUpload(session.id, [{ part_number: 1, etag, checksum, size_bytes: file.size }])
}

function VideoTaskDetailDialog({ task, onClose, onResult, onReuse }: { task: VideoTask; onClose: () => void; onResult: (assetID: string) => void; onReuse: () => void }) {
  const app = useApp()
  const accounting = buildVideoTaskAccounting(task)
  const resultAssetID = task.items.find((item) => item.result_asset_id)?.result_asset_id
  const [previewURL, setPreviewURL] = useState('')
  const [previewError, setPreviewError] = useState('')
  const terminal = task.status === 'succeeded' || task.status === 'partial' || task.status === 'failed' || task.status === 'cancelled'
  const running = !terminal
  const summary = [
    ['任务ID', task.id],
    ['状态', stageLabels[task.progress_stage || task.status] ?? task.status],
    ['预计积分', pointsText(accounting.estimatedPoints)],
    ['预留积分', pointsText(accounting.reservedPoints)],
    ['实际扣除', pointsText(accounting.actualPoints)],
    ['退回积分', pointsText(accounting.refundPoints)],
    ['结算状态', settlementLabels[accounting.settlementStatus] ?? accounting.settlementStatus],
  ]

  useEffect(() => {
    if (!resultAssetID) {
      setPreviewURL('')
      return undefined
    }
    let cancelled = false
    setPreviewError('')
    void userApi.getMediaAssetAccess(resultAssetID, 'preview').then((projection) => {
      if (!cancelled) setPreviewURL(userApi.mediaAssetUrl(projection.url, app.session?.token))
    }).catch((caught) => {
      if (!cancelled) setPreviewError(caught instanceof Error ? caught.message : '结果视频暂时无法预览')
    })
    return () => { cancelled = true }
  }, [resultAssetID, app.session?.token])

  return (
    <Drawer open wide title="视频任务详情" onClose={onClose}>
      <div className="u-detail">
        <section className="u-detail__stage-col">
          <div className="u-detail__stage">
            {previewURL ? (
              <video src={previewURL} controls playsInline preload="metadata" aria-label="视频生成结果" />
            ) : (
              <div className="cr-history-empty">
                {running ? <span className={userState.spinner} aria-hidden="true" /> : <strong className="cr-card-flag" style={{ position: 'static' }}>!</strong>}
                <h3>{running ? '视频生成中' : resultAssetID ? '结果视频加载中' : '暂无结果视频'}</h3>
                <p>{running ? (task.progress_message || '任务正在上游生成，完成后结果会出现在这里。') : previewError || (task.items.find((item) => item.error_message)?.error_message || '本次任务没有可播放的结果。')}</p>
                {running ? <div className="u-progress" style={{ width: 'min(320px, 100%)' }}><span /></div> : null}
              </div>
            )}
          </div>
          <div className="u-actions" style={{ justifyContent: 'space-between' }}>
            <div className="u-actions">
              <span className="u-chip u-chip--accent">{taskTypeLabels[task.task_type]}</span>
              <span className="u-chip u-chip--mono">{stageLabels[task.progress_stage || task.status] ?? task.status}</span>
              <span className="u-chip u-chip--mono">{pointsText(accounting.actualPoints)}</span>
            </div>
            <span className="u-chip u-chip--mono">{task.duration_seconds}s · {task.resolution?.toUpperCase() || '-'} · {task.aspect_ratio || '-'}</span>
          </div>
        </section>

        <section className="u-detail__info">
          <div className="u-detail__info-scroll">
            <div style={{ display: 'grid', gap: 7 }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                <span className="u-chip u-chip--accent">视频</span>
                <span className="u-chip u-chip--mono">{task.route_model_code || '未知模型'}</span>
                <span className="u-chip u-chip--mono">{settlementLabels[accounting.settlementStatus] ?? accounting.settlementStatus}</span>
              </div>
              <h2 className="u-detail__title">{task.prompt_template.split('\n')[0] || '视频任务'}</h2>
              <div className="u-meta">{task.created_at ? formatVideoTime(task.created_at) : '-'} · {task.id}</div>
            </div>

            <div className="u-detail__section">
              <h4>Prompt</h4>
              <div className="u-prompt" tabIndex={0}>{task.prompt_template || '未记录 Prompt'}</div>
            </div>

            <div className="u-detail__section">
              <h4>任务与结算</h4>
              <dl className="u-facts">
                {summary.map(([label, value]) => (
                  <div className="u-fact" key={label}>
                    <dt>{label}</dt>
                    <dd title={value}>{value}</dd>
                  </div>
                ))}
              </dl>
            </div>

            <div className="u-detail__section">
              <h4>生成参数</h4>
              <dl className="u-facts">
                <div className="u-fact"><dt>生成方式</dt><dd>{taskTypeLabels[task.task_type]}</dd></div>
                <div className="u-fact"><dt>时长</dt><dd>{task.duration_seconds} 秒</dd></div>
                <div className="u-fact"><dt>清晰度</dt><dd>{task.resolution?.toUpperCase() || '-'}</dd></div>
                <div className="u-fact"><dt>比例</dt><dd>{task.aspect_ratio || '-'}</dd></div>
                <div className="u-fact"><dt>音频</dt><dd>{task.audio_mode === 'generated' || task.generate_audio ? '生成音频' : '静音'}</dd></div>
                <div className="u-fact"><dt>方案数量</dt><dd>{task.requested_output_count}</dd></div>
                {accounting.unitPoints ? <div className="u-fact"><dt>单价</dt><dd>{pointsText(accounting.unitPoints)}</dd></div> : null}
              </dl>
            </div>

            {accounting.variables.length ? (
              <div className="u-detail__section">
                <h4>本次变量</h4>
                <dl className="u-facts">
                  {accounting.variables.map((variable) => (
                    <div className="u-fact" key={variable.name}><dt>{variable.name}</dt><dd>{variable.value}</dd></div>
                  ))}
                </dl>
              </div>
            ) : null}

            {accounting.inputs.length ? (
              <div className="u-detail__section">
                <h4>输入素材</h4>
                <ul className="cr-detail-list">
                  {accounting.inputs.map((input) => (
                    <li key={`${input.role}-${input.assetID}`}><span>{INPUT_ROLE_LABELS[input.role] ?? input.role}</span><strong>{input.name}</strong></li>
                  ))}
                </ul>
              </div>
            ) : null}

            <div className="u-detail__section">
              <h4>时间记录</h4>
              <ul className="cr-detail-list">
                {accounting.timeline.map((event) => (
                  <li key={event.label}><span>{event.label}</span><time dateTime={event.value}>{formatVideoTime(event.value)}</time></li>
                ))}
              </ul>
            </div>

            <div className="u-detail__section">
              <h4>结果与费用</h4>
              <ul className="cr-detail-list">
                {accounting.items.map((item) => (
                  <li key={item.id}>
                    <span>方案 #{item.ordinal + 1}</span>
                    <strong>{stageLabels[item.status] ?? item.status} · {item.actualSeconds ? `${item.actualSeconds} 秒 · ` : ''}{pointsText(item.actualPoints)}</strong>
                    {item.error ? <small className="cr-field-error">{item.error}</small> : null}
                    {item.resultAssetID ? <button className="u-btn u-btn--sm" type="button" onClick={() => onResult(item.resultAssetID!)}>查看结果</button> : null}
                  </li>
                ))}
              </ul>
            </div>
          </div>
          <div className="u-detail__foot">
            <Button tone="ghost" onClick={onReuse}><RotateCcw size={16} />复用参数</Button>
            <Button tone="ghost" onClick={onClose}>关闭</Button>
          </div>
        </section>
      </div>
    </Drawer>
  )
}

function FieldError({ message }: { message?: string }) {
  return message ? <small className="cr-field-error" role="alert">{message}</small> : null
}

function fieldErrorFor(errors: VideoFieldErrors, prefix: string) {
  return Object.entries(errors).find(([field]) => field === prefix || field.startsWith(`${prefix}.`))?.[1]
}

const INPUT_ROLE_LABELS: Record<string, string> = { first_frame: '首帧', last_frame: '尾帧', reference_image: '参考图片', reference_video: '参考视频', reference_audio: '参考音频' }

const settlementLabels: Record<string, string> = { reserved: '已预留', pending: '待结算', settling: '结算中', settled: '已结算', refunded: '已全额退回', failed: '结算异常' }

function formatVideoTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(date)
}

function reconcileVariables(template: string, current: VideoDraft['prompt_variables']) {
  const names = Array.from(new Set(Array.from(template.matchAll(/\{\{\s*([a-zA-Z][\w-]{0,63})\s*\}\}/g), (match) => match[1])))
  return names.map((name) => current.find((item) => item.name === name) ?? { name, value: '' })
}

function mergeTasks(tasks: VideoTask[], task: VideoTask) {
  return [task, ...tasks.filter((item) => item.id !== task.id)].slice(0, 20)
}

function uniqueVideoValues<T>(values: T[]) {
  return Array.from(new Set(values))
}
