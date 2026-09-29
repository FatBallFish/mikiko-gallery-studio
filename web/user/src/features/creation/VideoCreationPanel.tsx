import { useEffect, useMemo, useRef, useState } from 'react'
import { Ban, Copy, Download, FastForward, Film, Image as ImageIcon, Info, LoaderCircle, Play, RefreshCw, RotateCcw, Sparkles, Volume2, VolumeX, X } from 'lucide-react'
import type { CapabilityModelGroup, MediaAsset, PromptReferenceBinding, ReferenceAsset, VideoCapability, VideoEstimateRequest, VideoTask, VideoTaskType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { cn } from '../../../../shared/classnames'
import { Button, EmptyState, ErrorState, InlineCopyButton, LoadingState, Modal, copyText, useApp } from '../../components'
import { ProjectSelector, useProjects } from '../../ProjectContext'
import { ModelGroupSelect } from '../../pages/ModelGroupSelect'
import { PromptTemplateEditor, type PromptTemplateEditorHandle } from '../../pages/PromptTemplateEditor'
import { PromptEditorActions, PromptEditorDialog, PromptOptimizationPanel } from '../../pages/PromptEditorDialog'
import { PromptVariableForm } from '../../pages/PromptVariableForm'
import { WorkspaceStatusRail } from '../../pages/WorkspaceStatusRail'
import type { WorkspaceProgressNode } from '../../pages/workspaceTaskProgress'
import type { WorkspaceTaskView } from '../../pages/workspaceViewModel'
import { applyOptimizedPrompt, beginPromptOptimization, confirmPromptOptimization, failPromptOptimization, initialPromptOptimizationState, receivePromptEstimate, receivePromptOptimization, undoPromptOptimization } from '../../pages/workspacePromptOptimization'
import { errorMessage } from '../../useApiResource'
import { MediaAssetPicker } from '../media/MediaAssetPicker'
import { MediaPreviewDialog } from '../media/MediaPreviewDialog'
import { QUEUE_MEDIA_UPLOAD_EVENT } from '../media/UploadTray'
import { buildVideoQuoteBreakdown, buildVideoTaskAccounting } from './videoAccounting'
import { applyVideoCapability, defaultVideoDraft, invalidateVideoQuote, reuseVideoTask, videoDraftKey, videoModelForDraft, VIDEO_TASK_INPUT_ROLES, videoTaskInputLabel, videoTaskInputMissing, type VideoDraft, type VideoDraftInputRole, type VideoQuoteState } from './videoDraft'
import { cachedVideoCapability, loadVideoCapability } from './videoCapabilityCache'
import { consoleClasses } from '../../pages/consoleClasses'
import { videoFieldErrors, type VideoFieldErrors } from './videoErrors'

type Props = { initialTaskId?: string; initialAssetId?: string }

const taskTypeLabels: Record<VideoTaskType, string> = {
  text_to_video: '文生视频', image_to_video: '首帧生视频', first_last_frame_to_video: '首尾帧生视频',
  reference_to_video: '参考生视频', video_edit: '视频编辑', video_extend: '视频延长',
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
  const [quote, setQuote] = useState<VideoQuoteState | null>(null)
  const [tasks, setTasks] = useState<VideoTask[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [estimateError, setEstimateError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<VideoFieldErrors>({})
  const [submitting, setSubmitting] = useState(false)
  const [taskAction, setTaskAction] = useState('')
  const [selectedAssets, setSelectedAssets] = useState<Record<string, MediaAsset>>({})
  const [pickerRole, setPickerRole] = useState<VideoDraftInputRole | null>(null)
  const [previewAsset, setPreviewAsset] = useState<MediaAsset | null>(null)
  const [detailTask, setDetailTask] = useState<VideoTask | null>(null)
  const [taskTab, setTaskTab] = useState<'current' | 'history'>('current')
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
        if (normalized.changes.length) app.notify('info', normalized.changes.join('；'))
		return initialAssetId && normalized.draft.task_type === 'image_to_video' && normalized.draft.inputs.length === 0
			? { ...normalized.draft, inputs: [{ asset_id: initialAssetId, role: 'first_frame', ordinal: 0 }] }
          : normalized.draft
      })
      setError('')
    }).catch((reason) => alive && setError(errorMessage(reason))).finally(() => alive && setLoading(false))
    return () => { alive = false }
  }, [app, initialAssetId])

  useEffect(() => {
    if (!initialAssetId) return
    let alive = true
    userApi.getMediaAsset(initialAssetId).then((asset) => { if (alive) setSelectedAssets((current) => ({ ...current, [asset.id]: asset })) }).catch(() => undefined)
    return () => { alive = false }
  }, [initialAssetId])

  useEffect(() => {
	if (!initialTaskId || !capability) return
    let alive = true
    userApi.getVideoTask(initialTaskId).then((task) => {
      if (!alive) return
      setTasks((items) => mergeTasks(items, task))
	  setDraft(applyVideoCapability(reuseVideoTask(task), capability).draft)
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
    if (!draft || !capability || !projects.selectedProjectID || !draft.prompt_template.trim()) {
      setQuote(null)
      return undefined
    }
		if (videoTaskInputMissing(draft.task_type, draft.inputs)) {
      setQuote(null)
      return undefined
    }
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      const request = estimateRequest(projects.selectedProjectID, draft, selectedAssets)
      userApi.estimateVideo(request, controller.signal).then((result) => {
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
        setEstimateError(errorMessage(reason))
        setFieldErrors(videoFieldErrors(reason))
      })
    }, 250)
    return () => { window.clearTimeout(timer); controller.abort() }
  }, [capability, draft, projects.selectedProjectID])

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
    id: item.code, code: item.code, name: item.name, description: item.description, minimum_points: item.minimum_points,
    task_types: ['text_to_image'], base_resolution: [], quality: [], output_format: [], moderation: [], prices: [],
    max_output_image_count: 1, max_reference_image_count: 0, supports_reference: false,
  })) as CapabilityModelGroup[], [capability])
  const promptAssets = useMemo(() => draft
    ? draft.inputs.map((input) => selectedAssets[input.asset_id]).filter((asset): asset is MediaAsset => Boolean(asset) && asset.media_type === 'image')
    : [], [draft, selectedAssets])
  const promptVariables = useMemo(() => Object.fromEntries((draft?.prompt_variables ?? []).map((item) => [item.name, item.value])), [draft])
  const orderedTasks = useMemo(() => [...tasks].sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? '')), [tasks])
  const currentTask = useMemo(() => orderedTasks.find((task) => task.id === selectedTaskID) ?? orderedTasks[0], [orderedTasks, selectedTaskID])

  if (loading && !capability) return <LoadingState label="正在读取视频能力..." />
  if (error && !capability) return <ErrorState message={error} />
  if (!capability || !draft || !model || !options) return <EmptyState title="暂无可用视频模型" detail="当前用户组没有已启用的视频模型分组。" />
  const activeCapability = capability
  const activeDraft = draft
  const resolutionOptions = uniqueVideoValues(options.combinations.filter((item) => item.duration_seconds === draft.duration_seconds).map((item) => item.resolution))
  const aspectRatioOptions = uniqueVideoValues(options.combinations.filter((item) => item.duration_seconds === draft.duration_seconds && item.resolution === draft.resolution).map((item) => item.aspect_ratio))
  const audioAvailable = options.combinations.some((item) => item.duration_seconds === draft.duration_seconds && item.resolution === draft.resolution && item.aspect_ratio === draft.aspect_ratio && item.audio_mode === 'generated')
  const quoteBreakdown = quote ? buildVideoQuoteBreakdown({
    quote_token: quote.quote_token, expires_at: quote.quote_expires_at, capability_version: capability.capability_version,
    config_version: '', price_version: '', unit_points: quote.unit_points, estimated_points: quote.estimated_points,
    max_reserved_points: quote.max_reserved_points, display_points: quote.display_points, pricing_mode: quote.pricing_mode,
    summary: quote.summary, balance: quote.available_points ? { available_points: quote.available_points, sufficient: quote.sufficient !== false } : undefined,
  }) : null

  function patchDraft(patch: Partial<VideoDraft>) {
    const preferredField = Object.keys(patch)[0] as keyof VideoDraft | undefined
    const normalized = applyVideoCapability({ ...activeDraft, ...patch }, activeCapability, preferredField)
    setDraft(normalized.draft)
    setFieldErrors({})
    if (normalized.changes.length) app.notify('info', normalized.changes.join('；'))
  }

  async function startPromptOptimization() {
    const value = activeDraft.prompt_template.trim()
    if (Array.from(value).length < 8) {
      app.notify('error', '提示词至少需要 8 个字符后才能优化')
      return
    }
    const next = beginPromptOptimization(promptOptimization, value)
    if (next === promptOptimization) return
    setPromptOptimization(next)
    try {
      const estimateResult = await userApi.estimatePromptOptimization(value, 'video')
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
      const result = await userApi.optimizePrompt(next.originalPrompt, promptOptimization.estimate.quote, 'video')
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
    if (activeDraft.task_type === 'text_to_video') patchDraft({ task_type: 'image_to_video' })
    setPickerRole(activeDraft.task_type === 'first_last_frame_to_video' && activeDraft.inputs.some((item) => item.role === 'first_frame' && item.asset_id.trim()) ? 'last_frame' : 'first_frame')
  }

  function changePrompt(value: string) {
    if (value === activeDraft.prompt_template) return
    patchDraft({ prompt_template: value, prompt_variables: reconcileVariables(value, activeDraft.prompt_variables) })
  }

  function changePromptVariable(name: string, value: string) {
    patchDraft({ prompt_variables: activeDraft.prompt_variables.map((item) => item.name === name ? { ...item, value } : item) })
  }

  function reuseDraft(task: VideoTask) {
    setDraft(applyVideoCapability(reuseVideoTask(task), activeCapability).draft)
    setQuote(null)
    app.notify('info', '已复用任务参数')
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
      anchor.href = access.url
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
      const access = await userApi.getMediaAssetAccess(assetID, 'preview')
      const frame = await captureVideoFrameBlob(access.url)
      const asset = await uploadSingleMediaAsset(new File([frame], `视频续帧-${assetID.slice(0, 8)}.png`, { type: 'image/png' }), projects.selectedProjectID)
      setSelectedAssets((current) => ({ ...current, [asset.id]: asset }))
      patchDraft({ task_type: 'image_to_video', inputs: replaceInput(activeDraft.inputs, 'first_frame', asset.id) })
      app.notify('success', '已截取视频末帧并设为首帧输入')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setEditingAsset(false)
    }
  }

  async function submit() {
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
      app.notify('success', '视频任务已提交')
      await app.refreshAccount()
    } catch (reason) {
      setError(errorMessage(reason))
      setFieldErrors(videoFieldErrors(reason))
    } finally {
      setSubmitting(false)
    }
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

  return (
    <main className="video-creation-shell">
      <aside className="video-creation-controls" aria-label="视频生成参数">
        <div className="video-control-heading">
          <div><span>快捷视频</span><strong>生成设置</strong></div>
          <ProjectSelector />
        </div>

        <section className="video-control-section">
          <label className="video-field"><span>模型分组</span><ModelGroupSelect options={modelOptions} value={draft.route_model_code} onChange={(route_model_code) => patchDraft({ route_model_code })} /></label>
          <div className="video-field"><span>生成方式</span><div className="video-segmented" role="group" aria-label="生成方式">
            {model.task_types.map((value) => <button key={value} type="button" aria-pressed={draft.task_type === value} onClick={() => { const stale = VIDEO_TASK_INPUT_ROLES[value].length === 0 ? [] : draft.inputs.filter((item) => VIDEO_TASK_INPUT_ROLES[value].includes(item.role)); patchDraft({ task_type: value, inputs: stale }) }}>{taskTypeLabels[value]}</button>)}
          </div></div>
        </section>

        {draft.task_type !== 'text_to_video' ? <section className="video-control-section video-frame-inputs">
          {VIDEO_TASK_INPUT_ROLES[draft.task_type].map((role) => <FrameInput key={role} label={videoTaskInputLabel(role)} error={fieldErrorFor(fieldErrors, `inputs.${role}`)} asset={selectedAssets[draft.inputs.find((item) => item.role === role)?.asset_id ?? '']} onSelect={() => setPickerRole(role)} onPreview={setPreviewAsset} onRemove={() => patchDraft({ inputs: replaceInput(draft.inputs, role, '') })} />)}
        </section> : null}

        <section className="video-control-section">
          <div className="video-prompt-heading">
            <span>提示词</span>
            <PromptEditorActions
              optimizing={promptOptimization.stage === 'estimating' || promptOptimization.stage === 'optimizing'}
              canUndo={promptOptimization.stage === 'applied'}
              onExpand={() => setPromptExpanded(true)}
              onOptimize={() => void startPromptOptimization()}
              onUndo={undoOptimization}
            />
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
            onPasteFiles={(files) => { window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail: { files, projectID: projects.selectedProjectID } })) }}
          />
          <PromptVariableForm template={draft.prompt_template} values={promptVariables} disabled={submitting} onChange={changePromptVariable} />
          <FieldError message={fieldErrors.prompt_template ?? fieldErrors.prompt} />
          <FieldError message={fieldErrors.prompt_variables} />
        </section>

        <section className="video-control-section video-parameter-grid">
          <Choice label="时长" error={fieldErrors.duration_seconds} value={draft.duration_seconds} values={options.durations} format={(value) => `${value} 秒`} onChange={(duration_seconds) => patchDraft({ duration_seconds })} />
          <Choice label="清晰度" error={fieldErrors.resolution} value={draft.resolution} values={resolutionOptions} format={(value) => value.toUpperCase()} onChange={(resolution) => patchDraft({ resolution })} />
          <Choice label="比例" error={fieldErrors.aspect_ratio} value={draft.aspect_ratio} values={aspectRatioOptions} onChange={(aspect_ratio) => patchDraft({ aspect_ratio })} />
          <Choice label="数量" error={fieldErrors.output_count} value={draft.output_count} values={Array.from({ length: Math.max(1, model.max_output_count) }, (_, index) => index + 1)} onChange={(output_count) => patchDraft({ output_count })} />
          {options.audio_generation ? <label className="video-audio-toggle"><span>{draft.generate_audio ? <Volume2 size={17} /> : <VolumeX size={17} />}生成音频</span><input type="checkbox" checked={draft.generate_audio} disabled={!audioAvailable && !draft.generate_audio} onChange={(event) => patchDraft({ generate_audio: event.target.checked })} /><FieldError message={fieldErrors.generate_audio} /></label> : null}
        </section>

        <div className="video-quote-breakdown" aria-label="视频费用预估">
          <div><span>单价</span><strong>{quoteBreakdown ? `${quoteBreakdown.unitPoints} 积分` : '--'}</strong></div>
          <div><span>数量</span><strong>{quoteBreakdown ? `${quoteBreakdown.outputCount} 个` : '--'}</strong></div>
          <div><span>预计总价</span><strong>{quoteBreakdown ? `${quoteBreakdown.estimatedPoints} 积分` : '--'}</strong></div>
          <div><span>最大预留</span><strong>{quoteBreakdown ? `${quoteBreakdown.maxReservedPoints} 积分` : '--'}</strong></div>
          <div><span>可用余额</span><strong>{quoteBreakdown?.availablePoints ? `${quoteBreakdown.availablePoints} 积分` : '--'}</strong></div>
        </div>
        <div className="video-submit-bar">
          <div><span>提交时预留</span><strong>{quote?.display_points ?? quote?.max_reserved_points ?? '--'} 积分</strong>{estimateError ? <small>{estimateError}</small> : quote?.sufficient === false ? <small>积分不足</small> : null}</div>
          <Button busy={submitting} disabled={!quote || quote.sufficient === false || submitting} onClick={() => void submit()}><Sparkles size={17} />生成视频</Button>
        </div>
        {error ? <p className="video-inline-error" role="alert">{error}</p> : null}
      </aside>

      <section className="video-task-region" aria-label="视频任务">
        <header><div><span>任务队列</span><h2>视频生成</h2></div><span>{tasks.length} 个任务</span></header>
        {currentTask ? <WorkspaceStatusRail task={videoTaskRailView(currentTask)} startedAt={currentTask.created_at} finishedAt={currentTask.updated_at} /> : null}
        {orderedTasks.length > 1 ? <VideoRecentStrip tasks={orderedTasks} activeTaskID={currentTask?.id} onSelectTask={(task) => { setSelectedTaskID(task.id); setTaskTab('current') }} /> : null}
        <div className={consoleClasses.feed}>
          <div className={consoleClasses.outputTabs} role="tablist" aria-label="创作输出">
            <button type="button" role="tab" aria-selected={taskTab === 'current'} className={cn(consoleClasses.outputTab, taskTab === 'current' && consoleClasses.outputTabActive)} onClick={() => setTaskTab('current')}>当前创作</button>
            <button type="button" role="tab" aria-selected={taskTab === 'history'} className={cn(consoleClasses.outputTab, taskTab === 'history' && consoleClasses.outputTabActive)} onClick={() => setTaskTab('history')}>历史创作</button>
          </div>
          {taskTab === 'current' ? (
            currentTask ? <VideoCurrentTaskCard task={currentTask} busy={taskAction === currentTask.id} editing={editingAsset} onCancel={() => void cancelTask(currentTask)} onDetail={() => setDetailTask(currentTask)} onReuse={() => reuseDraft(currentTask)} onResult={(assetID) => void openResultPreview(assetID)} onExtend={(assetID) => void editFromResult(assetID)} onDownload={(assetID) => void downloadResult(assetID)} onCopyPrompt={() => { void copyText(currentTask.prompt_template).then(() => app.notify('success', '提示词已复制')).catch(() => app.notify('error', '复制失败')) }} /> : <EmptyState title="还没有视频任务" detail="填写参数并确认报价后，当前任务进度会展示在这里。" icon={<Film size={24} />} />
          ) : (
            orderedTasks.length ? <div className={consoleClasses.historyGrid}>{orderedTasks.map((task) => <VideoHistoryCard key={task.id} task={task} busy={taskAction === task.id} onRefresh={() => void refreshTask(task)} onDetail={() => setDetailTask(task)} onReuse={() => reuseDraft(task)} onResult={(assetID) => void openResultPreview(assetID)} />)}</div> : <EmptyState title="暂无历史创作" detail="完成一次视频创作后，记录会展示在这里。" icon={<Film size={24} />} />
          )}
        </div>
      </section>
      {pickerRole && pickerRole !== 'reference_video' ? <MediaAssetPicker projectID={projects.selectedProjectID} mediaTypes={['image']} title={pickerRole === 'first_frame' ? '选择首帧图片' : '选择参考图片'} onClose={() => setPickerRole(null)} onConfirm={(assets) => {
        const asset = assets[0]
        if (!asset) return
        setSelectedAssets((current) => ({ ...current, [asset.id]: asset }))
        patchDraft({ inputs: replaceInput(draft.inputs, pickerRole, asset.id) })
        setPickerRole(null)
      }} /> : null}
      {pickerRole === 'reference_video' ? <MediaAssetPicker projectID={projects.selectedProjectID} mediaTypes={['video']} title="选择参考视频" onClose={() => setPickerRole(null)} onConfirm={(assets) => {
        const asset = assets[0]
        if (!asset) return
        setSelectedAssets((current) => ({ ...current, [asset.id]: asset }))
        patchDraft({ inputs: replaceInput(draft.inputs, pickerRole, asset.id) })
        setPickerRole(null)
      }} /> : null}
      {previewAsset ? <MediaPreviewDialog asset={previewAsset} projects={projects.projects} creationActions={[]} onClose={() => setPreviewAsset(null)} onChanged={(asset) => { setPreviewAsset(asset); setSelectedAssets((current) => ({ ...current, [asset.id]: asset })) }} onDeleted={(asset) => { setPreviewAsset(null); setSelectedAssets((current) => { const next = { ...current }; delete next[asset.id]; return next }) }} onContinue={() => undefined} /> : null}
      {detailTask ? <VideoTaskDetailDialog task={detailTask} onClose={() => setDetailTask(null)} onReuse={() => { setDraft(applyVideoCapability(reuseVideoTask(detailTask), capability).draft); setQuote(null); setDetailTask(null); app.notify('info', '已复用任务参数') }} onResult={(assetID) => void userApi.getMediaAsset(assetID).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught)))} /> : null}
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
          onPasteFiles={(files) => { window.dispatchEvent(new CustomEvent(QUEUE_MEDIA_UPLOAD_EVENT, { detail: { files, projectID: projects.selectedProjectID } })) }}
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
    </main>
  )
}

function estimateRequest(projectId: string, draft: VideoDraft, assets: Record<string, MediaAsset>): VideoEstimateRequest {
  const bindings = new Map<string, string>()
  for (const input of draft.inputs) {
    const asset = assets[input.asset_id]
    if (!asset || !input.asset_id.trim() || bindings.has(asset.name)) continue
    bindings.set(asset.name, asset.id)
  }
  return { project_id: projectId, route_model_code: draft.route_model_code, task_type: draft.task_type, prompt_template: draft.prompt_template, prompt_variables: draft.prompt_variables, reference_bindings: Array.from(bindings, ([name, asset_id]): PromptReferenceBinding => ({ name, asset_id })), inputs: draft.inputs.filter((item) => item.asset_id.trim()), duration_seconds: draft.duration_seconds, resolution: draft.resolution, aspect_ratio: draft.aspect_ratio, audio_mode: draft.generate_audio ? 'generated' : 'silent', output_count: draft.output_count }
}

function FrameInput({ label, asset, error, onSelect, onPreview, onRemove }: { label: string; asset?: MediaAsset; error?: string; onSelect: () => void; onPreview: (asset: MediaAsset) => void; onRemove: () => void }) {
  return <div className="video-frame-field"><span>{label}</span>{asset ? <div className="video-frame-asset"><button type="button" className="video-frame-preview" onClick={() => onPreview(asset)}><ImageIcon size={22} /><span><strong>{asset.name}</strong><small>{asset.width && asset.height ? `${asset.width} × ${asset.height}` : asset.mime_type}</small></span></button><button type="button" title="更换图片" onClick={onSelect}><RefreshCw size={15} /></button><button type="button" title="移除图片" onClick={onRemove}><X size={15} /></button></div> : <button type="button" className="video-frame-empty" onClick={onSelect}><ImageIcon size={20} />选择图片资产</button>}<FieldError message={error} /></div>
}

function Choice<T extends string | number>({ label, value, values, error, format = String, onChange }: { label: string; value: T; values: T[]; error?: string; format?: (value: T) => string; onChange: (value: T) => void }) {
  return <label className="video-field"><span>{label}</span><select value={value} onChange={(event) => { const selected = values.find((item) => String(item) === event.target.value); if (selected !== undefined) onChange(selected) }}>{values.map((item) => <option key={item} value={item}>{format(item)}</option>)}</select><FieldError message={error} /></label>
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
    <section className={consoleClasses.recentStrip} aria-label="最近创作">
      <div className={consoleClasses.recentScroll}>
        {tasks.slice(0, 8).map((task) => {
          const stage = task.progress_stage || task.status
          const resultAssetID = task.items.find((item) => item.result_asset_id)?.result_asset_id
          return (
            <button key={task.id} type="button" className={cn(consoleClasses.recentItem, task.id === activeTaskID && consoleClasses.recentItemActive)} aria-current={task.id === activeTaskID ? 'true' : undefined} onClick={() => onSelectTask(task)}>
              <span className={consoleClasses.recentThumb}>{resultAssetID ? <StripPoster assetID={resultAssetID} /> : <span>{stageLabels[stage] ?? stage}</span>}</span>
              <span className="video-recent-copy">
                <strong>{taskTypeLabels[task.task_type] ?? task.task_type}</strong>
                <span>{stageLabels[stage] ?? stage} · {formatVideoHistoryTime(task.created_at)}</span>
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
  return posterURL ? <img src={posterURL} alt="" loading="lazy" draggable={false} /> : null
}

function isVideoTaskTerminal(task: VideoTask) {
  return ['succeeded', 'partial', 'failed', 'cancelled'].includes(task.status)
}

// Fresh result assets briefly 409 on poster access while the media pipeline
// generates the derivative, so retry with backoff instead of giving up.
function usePosterURL(assetID: string | undefined) {
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
        if (alive) setPosterURL(access.url)
      }).catch(() => {
        if (!alive || attempt >= 5) return
        attempt += 1
        timer = window.setTimeout(run, attempt * 2000)
      })
    }
    run()
    return () => { alive = false; window.clearTimeout(timer) }
  }, [assetID])
  return posterURL
}

function VideoResultThumb({ assetID, alt }: { assetID: string; alt: string }) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const posterURL = usePosterURL(assetID)
  const [hoverURL, setHoverURL] = useState('')

  const beginHover = () => {
    if (hoverURL) {
      void videoRef.current?.play()
      return
    }
    void userApi.getMediaAssetAccess(assetID, 'hover').then((access) => setHoverURL(access.url)).catch(() => undefined)
  }
  const endHover = () => {
    videoRef.current?.pause()
  }

  return <span className="video-result-thumb" aria-hidden="true">
    {posterURL && !hoverURL ? <img src={posterURL} alt={alt} loading="lazy" draggable={false} /> : null}
    {hoverURL ? <video ref={videoRef} src={hoverURL} muted loop autoPlay playsInline preload="metadata" draggable={false} /> : null}
    {!posterURL && !hoverURL ? <span className="video-result-placeholder"><Film size={22} /></span> : null}
    <span className="video-result-play" aria-hidden="true"><Play size={16} fill="currentColor" /></span>
  </span>
}

// Class strings mirrored from the image workspace result component so both
// surfaces share the same figure frame, hover reveal and meta typography.
// Result figures share the image console's visual language; only the
// aspect-ratio custom property differs (--video-result-ratio).
const resultClasses = {
  figure: consoleClasses.generatedFigure,
  stage: 'block w-full cursor-zoom-in border-0 bg-transparent p-0 [aspect-ratio:var(--video-result-ratio)] max-h-[calc(100vh-430px)]',
  media: consoleClasses.generatedMedia,
  caption: consoleClasses.generatedCaption,
  iconAction: consoleClasses.generatedIconAction,
  metaRow: consoleClasses.outputMetaRow,
}

function videoAspectRatioStyle(ratio: string | undefined) {
  const [w, h] = (ratio ?? '16:9').split(':').map((part) => Number(part.trim()))
  if (!w || !h) return undefined
  return { '--video-result-ratio': `${w} / ${h}` } as React.CSSProperties
}

function VideoCurrentTaskCard({ task, busy, onCancel, onDetail, onReuse, onResult, onExtend, onDownload, onCopyPrompt, editing }: { task: VideoTask; busy: boolean; onCancel: () => void; onDetail: () => void; onReuse: () => void; onResult: (assetID: string) => void; onExtend: (assetID: string) => void; onDownload: (assetID: string) => void; onCopyPrompt: () => void; editing: boolean }) {
  const terminal = isVideoTaskTerminal(task)
  const stage = task.progress_stage || task.status
  const charged = task.actual_points ?? '--'
  const reserved = task.reserved_points ?? task.estimated_points ?? '--'
  const resultItems = task.items.filter((item) => item.result_asset_id)
  const failed = task.status === 'failed' || task.status === 'cancelled'
  return <article className="video-current-task" data-status={task.status}>
    {!terminal ? <header className="video-current-head">
      <div>
        <strong>{stageLabels[stage] ?? stage} · {task.progress_message || '正在处理'}</strong>
        <span className="video-current-badges">{videoTaskBadges(task).map((badge) => <i key={badge}>{badge}</i>)}</span>
      </div>
      <div className="video-current-actions">
        <button type="button" onClick={onDetail}><Info size={15} />详情</button>
        <button type="button" disabled={busy} onClick={onCancel}><Ban size={15} />取消</button>
      </div>
    </header> : null}
    {!terminal ? <div className="video-current-progress" role="status">
      <span className="video-current-progress-track"><span className="video-current-progress-bar" data-stage={stage} /></span>
      <span className="video-current-progress-copy"><LoaderCircle className="animate-spin" size={15} />{stageLabels[stage] ?? stage} · 预留 {reserved} 积分</span>
    </div> : null}
    {failed ? <div className={cn(consoleClasses.pending, consoleClasses.pendingFailed)} role="alert">
      <strong className={consoleClasses.pendingFailedTitle}>{task.status === 'cancelled' ? '任务已取消' : '生成失败'}</strong>
      <p className="m-0">{task.items.find((item) => item.error_message)?.error_message || task.progress_message || (task.status === 'cancelled' ? '预留积分已退回。' : '预留积分已退回。')}</p>
      <div className={consoleClasses.failureMeta}>
        <span className={consoleClasses.failureMetaItem}><span className={consoleClasses.failureMetaLabel}>任务ID</span><span className={consoleClasses.failureMetaValue}>{task.id}</span><InlineCopyButton text={task.id} label="复制任务ID" /></span>
      </div>
      <div className="mt-3 flex flex-wrap justify-center gap-2">
        <button className="min-h-9 rounded-xl px-3 text-sm" type="button" onClick={onDetail}><Info size={15} />详情</button>
        <button className="min-h-9 rounded-xl px-3 text-sm" type="button" onClick={onReuse}><RotateCcw size={15} />复用参数</button>
      </div>
    </div> : null}
    {resultItems.length ? (
      <div className={resultItems.length === 1 ? 'w-full max-w-5xl' : 'grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(260px,1fr))]'}>
        {resultItems.map((item) => <VideoResultFigure key={item.id} assetID={item.result_asset_id!} ordinal={item.ordinal} aspectRatio={task.aspect_ratio} editing={editing} onOpen={() => item.result_asset_id && onResult(item.result_asset_id)} onCopyPrompt={onCopyPrompt} onReuse={onReuse} onExtend={() => item.result_asset_id && onExtend(item.result_asset_id)} onDetail={onDetail} onDownload={() => item.result_asset_id && onDownload(item.result_asset_id)} />)}
      </div>
    ) : null}
    {terminal && !failed ? <>
      <p className="video-current-prompt">{task.prompt_template || '未填写提示词'}</p>
      <div className={resultClasses.metaRow}>
        <span>模型: {task.route_model_code}</span>
        <span>时长: {task.duration_seconds} 秒</span>
        <span>分辨率: {task.resolution.toUpperCase()}</span>
        <span>比例: {task.aspect_ratio}</span>
        <span>{task.audio_mode === 'generated' || task.generate_audio ? '有声' : '静音'}</span>
        <span>数量: {task.requested_output_count}</span>
        <span>任务时间: {formatVideoHistoryTime(task.created_at)}</span>
        <span>消耗: {charged} ◈</span>
        <span className="inline-flex items-center gap-1">任务ID: {task.id}<InlineCopyButton text={task.id} label="复制任务ID" /></span>
      </div>
    </> : null}
  </article>
}

function VideoResultFigure({ assetID, ordinal, aspectRatio, editing, onOpen, onCopyPrompt, onReuse, onExtend, onDetail, onDownload }: { assetID: string; ordinal: number; aspectRatio: string; editing: boolean; onOpen: () => void; onCopyPrompt: () => void; onReuse: () => void; onExtend: () => void; onDetail: () => void; onDownload: () => void }) {
  const posterURL = usePosterURL(assetID)
  const [hoverURL, setHoverURL] = useState('')
  const videoRef = useRef<HTMLVideoElement | null>(null)

  const beginHover = () => {
    if (hoverURL) {
      void videoRef.current?.play()
      return
    }
    void userApi.getMediaAssetAccess(assetID, 'hover').then((access) => setHoverURL(access.url)).catch(() => undefined)
  }
  const endHover = () => {
    videoRef.current?.pause()
  }

  return (
    <figure className={resultClasses.figure} style={videoAspectRatioStyle(aspectRatio)} onMouseEnter={beginHover} onMouseLeave={endHover}>
      <button type="button" className={resultClasses.stage} onClick={onOpen} aria-label="播放生成视频">
        {posterURL && !hoverURL ? <img className={resultClasses.media} src={posterURL} alt={`方案 ${ordinal + 1}`} draggable={false} /> : null}
        {hoverURL ? <video ref={videoRef} className={resultClasses.media} src={hoverURL} muted loop autoPlay playsInline preload="metadata" draggable={false} /> : null}
        {!posterURL && !hoverURL ? <span className="grid size-full place-items-center text-[var(--muted)]"><Film size={28} /></span> : null}
      </button>
      <figcaption className={resultClasses.caption}>
        <button className={resultClasses.iconAction} type="button" title="复制提示词" aria-label="复制提示词" onClick={onCopyPrompt}><Copy size={16} /></button>
        <button className={resultClasses.iconAction} type="button" title="复用参数" aria-label="复用参数" onClick={onReuse}><RotateCcw size={16} /></button>
        <button className={resultClasses.iconAction} type="button" title="续写延长" aria-label="续写延长视频" disabled={editing} onClick={onExtend}><FastForward size={16} /></button>
        <button className={resultClasses.iconAction} type="button" title="详情" aria-label="查看任务详情" onClick={onDetail}><Info size={16} /></button>
        <button className={resultClasses.iconAction} type="button" title="下载" aria-label="下载视频" onClick={onDownload}><Download size={16} /></button>
      </figcaption>
    </figure>
  )
}


function VideoHistoryCard({ task, busy, onRefresh, onDetail, onReuse, onResult }: { task: VideoTask; busy: boolean; onRefresh: () => void; onDetail: () => void; onResult: (assetID: string) => void; onReuse: () => void }) {
  const terminal = isVideoTaskTerminal(task)
  const stage = task.progress_stage || task.status
  const resultItem = task.items.find((item) => item.result_asset_id)
  return <article className={consoleClasses.historyCard} data-status={task.status}>
    <button type="button" className="video-history-stage" onClick={() => resultItem ? onResult(resultItem.result_asset_id!) : onDetail()} title={resultItem ? '播放结果' : '查看任务详情'}>
      {resultItem ? <VideoResultThumb assetID={resultItem.result_asset_id!} alt={task.prompt_template.slice(0, 30)} /> : terminal ? <span className="video-history-state"><Ban size={20} /><span>{task.status === 'cancelled' ? '已取消' : '生成失败'}</span></span> : <span className="video-history-state"><LoaderCircle className="animate-spin" size={20} /><span>{stageLabels[stage] ?? stage}</span></span>}
      <span className="video-history-badges">{videoTaskBadges(task).map((badge) => <i key={badge}>{badge}</i>)}</span>
    </button>
    <p className="video-history-title">{task.prompt_template || '未填写提示词'}</p>
    <div className="video-history-meta">
      <span>{formatVideoHistoryTime(task.created_at)}</span>
      <span>{task.actual_points && task.actual_points !== '0.00000' ? `${task.actual_points} 积分` : `预留 ${task.reserved_points ?? task.estimated_points ?? '--'}`}</span>
      <span className="inline-flex items-center gap-1"><span title={task.id}>任务ID: {task.id.slice(0, 8)}…</span><InlineCopyButton text={task.id} label="复制任务ID" /></span>
    </div>
    <div className="video-history-actions">
      <button type="button" onClick={onDetail}><Info size={14} />详情</button>
      {terminal ? <button type="button" onClick={onReuse}><RotateCcw size={14} />复用</button> : <button type="button" disabled={busy} onClick={onRefresh}><RefreshCw className={busy ? 'animate-spin' : undefined} size={14} /></button>}
    </div>
  </article>
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
  const accounting = buildVideoTaskAccounting(task)
  return <Modal title="视频任务详情" onClose={onClose} className="video-task-detail-dialog">
    <div className="video-task-detail-summary">
      <div><span>任务ID</span><strong className="inline-flex items-center gap-1 break-all font-vault-mono text-xs">{task.id}<InlineCopyButton text={task.id} label="复制任务ID" /></strong></div>
      <div><span>状态</span><strong>{stageLabels[task.progress_stage || task.status] ?? task.status}</strong></div>
      <div><span>预计积分</span><strong>{accounting.estimatedPoints}</strong></div>
      <div><span>预留积分</span><strong>{accounting.reservedPoints}</strong></div>
      <div><span>实际扣除</span><strong>{accounting.actualPoints}</strong></div>
      <div><span>退回积分</span><strong>{accounting.refundPoints}</strong></div>
      <div><span>结算状态</span><strong>{settlementLabels[accounting.settlementStatus] ?? accounting.settlementStatus}</strong></div>
    </div>
    <section className="video-task-detail-section"><h3>生成参数</h3><dl><div><dt>模型分组</dt><dd>{task.route_model_code}</dd></div><div><dt>生成方式</dt><dd>{taskTypeLabels[task.task_type]}</dd></div><div><dt>时长</dt><dd>{task.duration_seconds} 秒</dd></div><div><dt>清晰度</dt><dd>{task.resolution.toUpperCase()}</dd></div><div><dt>比例</dt><dd>{task.aspect_ratio}</dd></div><div><dt>音频</dt><dd>{task.audio_mode === 'generated' || task.generate_audio ? '生成音频' : '静音'}</dd></div><div><dt>方案数量</dt><dd>{task.requested_output_count}</dd></div>{accounting.unitPoints ? <div><dt>单价</dt><dd>{accounting.unitPoints} 积分</dd></div> : null}</dl><p>{task.prompt_template}</p></section>
    {accounting.variables.length ? <section className="video-task-detail-section"><h3>本次变量</h3><dl>{accounting.variables.map((variable) => <div key={variable.name}><dt>{variable.name}</dt><dd>{variable.value}</dd></div>)}</dl></section> : null}
    {accounting.inputs.length ? <section className="video-task-detail-section"><h3>输入素材</h3><ul>{accounting.inputs.map((input) => <li key={`${input.role}-${input.assetID}`}><span>{input.role === 'first_frame' ? '首帧' : '尾帧'}</span><strong>{input.name}</strong></li>)}</ul></section> : null}
    <section className="video-task-detail-section"><h3>时间记录</h3><ul>{accounting.timeline.map((event) => <li key={event.label}><span>{event.label}</span><time dateTime={event.value}>{formatVideoTime(event.value)}</time></li>)}</ul></section>
    <section className="video-task-detail-section"><h3>结果与费用</h3><ul>{accounting.items.map((item) => <li key={item.id}><span>方案 #{item.ordinal + 1}</span><strong>{stageLabels[item.status] ?? item.status} · {item.actualSeconds ? `${item.actualSeconds} 秒 · ` : ''}{item.actualPoints} 积分</strong>{item.error ? <small>{item.error}</small> : null}{item.resultAssetID ? <button type="button" onClick={() => onResult(item.resultAssetID!)}>查看结果</button> : null}</li>)}</ul></section>
    <div className="video-task-detail-actions"><Button tone="ghost" onClick={onReuse}><RotateCcw size={16} />复用参数</Button></div>
  </Modal>
}

function FieldError({ message }: { message?: string }) {
  return message ? <small className="video-field-error" role="alert">{message}</small> : null
}

function fieldErrorFor(errors: VideoFieldErrors, prefix: string) {
  return Object.entries(errors).find(([field]) => field === prefix || field.startsWith(`${prefix}.`))?.[1]
}

const settlementLabels: Record<string, string> = { reserved: '已预留', pending: '待结算', settling: '结算中', settled: '已结算', refunded: '已全额退回', failed: '结算异常' }

function formatVideoTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(date)
}

function replaceInput(inputs: VideoDraft['inputs'], role: VideoDraftInputRole, assetId: string) {
  const filtered = inputs.filter((item) => item.role !== role)
  const ordinal = VIDEO_TASK_INPUT_ROLES_ORDER[role]
  return assetId ? [...filtered, { asset_id: assetId, role, ordinal }] : filtered
}
const VIDEO_TASK_INPUT_ROLES_ORDER: Record<VideoDraftInputRole, number> = { first_frame: 0, last_frame: 1, reference_image: 2, reference_video: 3 }

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
