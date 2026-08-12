import { useEffect, useMemo, useRef, useState } from 'react'
import { Ban, Clock3, Film, Image, Info, LoaderCircle, RefreshCw, RotateCcw, Sparkles, Volume2, VolumeX, X } from 'lucide-react'
import type { CapabilityModelGroup, MediaAsset, VideoCapability, VideoEstimateRequest, VideoTask, VideoTaskType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, Modal, useApp } from '../../components'
import { ProjectSelector, useProjects } from '../../ProjectContext'
import { ModelGroupSelect } from '../../pages/ModelGroupSelect'
import { errorMessage } from '../../useApiResource'
import { MediaAssetPicker } from '../media/MediaAssetPicker'
import { MediaPreviewDialog } from '../media/MediaPreviewDialog'
import { buildVideoQuoteBreakdown, buildVideoTaskAccounting } from './videoAccounting'
import { applyVideoCapability, defaultVideoDraft, invalidateVideoQuote, reuseVideoTask, videoDraftKey, videoModelForDraft, type VideoDraft, type VideoQuoteState } from './videoDraft'
import { videoFieldErrors, type VideoFieldErrors } from './videoErrors'

type Props = { initialTaskId?: string; initialAssetId?: string }

const taskTypeLabels: Record<VideoTaskType, string> = {
  text_to_video: '文生视频', image_to_video: '首帧生视频', first_last_frame_to_video: '首尾帧生视频',
}
const stageLabels: Record<string, string> = {
  queued: '排队中', submitting: '正在提交', reconciling: '正在确认', provider_queued: '上游排队', provider_running: '生成中',
  artifact_pending: '正在保存原件', recovery_required: '正在恢复原件', saving: '正在保存', succeeded: '已完成', partial: '部分完成', failed: '失败', cancelled: '已取消',
}

export function VideoCreationPanel({ initialTaskId, initialAssetId }: Props) {
  const app = useApp()
  const projects = useProjects()
  const [capability, setCapability] = useState<VideoCapability | null>(null)
  const [draft, setDraft] = useState<VideoDraft | null>(null)
  const [quote, setQuote] = useState<VideoQuoteState | null>(null)
  const [tasks, setTasks] = useState<VideoTask[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [estimateError, setEstimateError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<VideoFieldErrors>({})
  const [submitting, setSubmitting] = useState(false)
  const [taskAction, setTaskAction] = useState('')
  const [selectedAssets, setSelectedAssets] = useState<Record<string, MediaAsset>>({})
  const [pickerRole, setPickerRole] = useState<'first_frame' | 'last_frame' | null>(null)
  const [previewAsset, setPreviewAsset] = useState<MediaAsset | null>(null)
  const [detailTask, setDetailTask] = useState<VideoTask | null>(null)
  const priorDraftRef = useRef<VideoDraft | null>(null)
  const completedTaskIDsRef = useRef(new Set<string>())

  useEffect(() => {
    let alive = true
    setLoading(true)
    userApi.getVideoCapabilities().then((next) => {
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
		const hasFirstFrame = draft.inputs.some((input) => input.role === 'first_frame' && input.asset_id.trim())
		const hasLastFrame = draft.inputs.some((input) => input.role === 'last_frame' && input.asset_id.trim())
		if ((draft.task_type !== 'text_to_video' && !hasFirstFrame) || (draft.task_type === 'first_last_frame_to_video' && !hasLastFrame)) {
      setQuote(null)
      return undefined
    }
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      const request = estimateRequest(projects.selectedProjectID, draft)
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

  if (loading) return <LoadingState label="正在读取视频能力..." />
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
      const task = await userApi.createVideoTask({ ...estimateRequest(projects.selectedProjectID, activeDraft), quote_token: quote.quote_token })
      setTasks((items) => mergeTasks(items, task))
      setQuote(null)
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
            {model.task_types.map((value) => <button key={value} type="button" aria-pressed={draft.task_type === value} onClick={() => patchDraft({ task_type: value })}>{taskTypeLabels[value]}</button>)}
          </div></div>
        </section>

        {draft.task_type !== 'text_to_video' ? <section className="video-control-section video-frame-inputs">
          <FrameInput label="首帧资产" error={fieldErrorFor(fieldErrors, 'inputs.first_frame')} asset={selectedAssets[draft.inputs.find((item) => item.role === 'first_frame')?.asset_id ?? '']} onSelect={() => setPickerRole('first_frame')} onPreview={setPreviewAsset} onRemove={() => patchDraft({ inputs: replaceInput(draft.inputs, 'first_frame', '') })} />
          {draft.task_type === 'first_last_frame_to_video' ? <FrameInput label="尾帧资产" error={fieldErrorFor(fieldErrors, 'inputs.last_frame')} asset={selectedAssets[draft.inputs.find((item) => item.role === 'last_frame')?.asset_id ?? '']} onSelect={() => setPickerRole('last_frame')} onPreview={setPreviewAsset} onRemove={() => patchDraft({ inputs: replaceInput(draft.inputs, 'last_frame', '') })} /> : null}
        </section> : null}

        <section className="video-control-section">
          <label className="video-field"><span>提示词</span><textarea value={draft.prompt_template} maxLength={4000} rows={6} onChange={(event) => patchDraft({ prompt_template: event.target.value, prompt_variables: reconcileVariables(event.target.value, draft.prompt_variables) })} /><FieldError message={fieldErrors.prompt_template ?? fieldErrors.prompt} /></label>
          {draft.prompt_variables.map((variable) => <label className="video-field video-variable" key={variable.name}><span>{variable.name}</span><input value={variable.value} onChange={(event) => patchDraft({ prompt_variables: draft.prompt_variables.map((item) => item.name === variable.name ? { ...item, value: event.target.value } : item) })} /></label>)}
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
        {tasks.length ? <div className="video-task-list">{tasks.map((task) => <VideoTaskRow key={task.id} task={task} busy={taskAction === task.id} onCancel={() => void cancelTask(task)} onRefresh={() => void refreshTask(task)} onDetail={() => setDetailTask(task)} onResult={(assetID) => void userApi.getMediaAsset(assetID).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught)))} onReuse={() => { setDraft(applyVideoCapability(reuseVideoTask(task), capability).draft); setQuote(null); app.notify('info', '已复用任务参数') }} />)}</div> : <EmptyState title="还没有视频任务" detail="填写参数并确认报价后，任务状态会显示在这里。" icon={<Film size={24} />} />}
      </section>
      {pickerRole ? <MediaAssetPicker projectID={projects.selectedProjectID} mediaTypes={['image']} title={pickerRole === 'first_frame' ? '选择首帧图片' : '选择尾帧图片'} onClose={() => setPickerRole(null)} onConfirm={(assets) => {
        const asset = assets[0]
        if (!asset) return
        setSelectedAssets((current) => ({ ...current, [asset.id]: asset }))
        patchDraft({ inputs: replaceInput(draft.inputs, pickerRole, asset.id) })
        setPickerRole(null)
      }} /> : null}
      {previewAsset ? <MediaPreviewDialog asset={previewAsset} projects={projects.projects} creationActions={[]} onClose={() => setPreviewAsset(null)} onChanged={(asset) => { setPreviewAsset(asset); setSelectedAssets((current) => ({ ...current, [asset.id]: asset })) }} onDeleted={(asset) => { setPreviewAsset(null); setSelectedAssets((current) => { const next = { ...current }; delete next[asset.id]; return next }) }} onContinue={() => undefined} /> : null}
      {detailTask ? <VideoTaskDetailDialog task={detailTask} onClose={() => setDetailTask(null)} onReuse={() => { setDraft(applyVideoCapability(reuseVideoTask(detailTask), capability).draft); setQuote(null); setDetailTask(null); app.notify('info', '已复用任务参数') }} onResult={(assetID) => void userApi.getMediaAsset(assetID).then(setPreviewAsset).catch((caught) => app.notify('error', errorMessage(caught)))} /> : null}
    </main>
  )
}

function estimateRequest(projectId: string, draft: VideoDraft): VideoEstimateRequest {
  return { project_id: projectId, route_model_code: draft.route_model_code, task_type: draft.task_type, prompt_template: draft.prompt_template, prompt_variables: draft.prompt_variables, reference_bindings: [], inputs: draft.inputs.filter((item) => item.asset_id.trim()), duration_seconds: draft.duration_seconds, resolution: draft.resolution, aspect_ratio: draft.aspect_ratio, audio_mode: draft.generate_audio ? 'generated' : 'silent', output_count: draft.output_count }
}

function FrameInput({ label, asset, error, onSelect, onPreview, onRemove }: { label: string; asset?: MediaAsset; error?: string; onSelect: () => void; onPreview: (asset: MediaAsset) => void; onRemove: () => void }) {
  return <div className="video-frame-field"><span>{label}</span>{asset ? <div className="video-frame-asset"><button type="button" className="video-frame-preview" onClick={() => onPreview(asset)}><Image size={22} /><span><strong>{asset.name}</strong><small>{asset.width && asset.height ? `${asset.width} × ${asset.height}` : asset.mime_type}</small></span></button><button type="button" title="更换图片" onClick={onSelect}><RefreshCw size={15} /></button><button type="button" title="移除图片" onClick={onRemove}><X size={15} /></button></div> : <button type="button" className="video-frame-empty" onClick={onSelect}><Image size={20} />选择图片资产</button>}<FieldError message={error} /></div>
}

function Choice<T extends string | number>({ label, value, values, error, format = String, onChange }: { label: string; value: T; values: T[]; error?: string; format?: (value: T) => string; onChange: (value: T) => void }) {
  return <label className="video-field"><span>{label}</span><select value={value} onChange={(event) => { const selected = values.find((item) => String(item) === event.target.value); if (selected !== undefined) onChange(selected) }}>{values.map((item) => <option key={item} value={item}>{format(item)}</option>)}</select><FieldError message={error} /></label>
}

function VideoTaskRow({ task, busy, onCancel, onRefresh, onDetail, onResult, onReuse }: { task: VideoTask; busy: boolean; onCancel: () => void; onRefresh: () => void; onDetail: () => void; onResult: (assetID: string) => void; onReuse: () => void }) {
  const terminal = ['succeeded', 'partial', 'failed', 'cancelled'].includes(task.status)
  const stage = task.progress_stage || task.status
  const charged = task.actual_points ?? '--'
  const reserved = task.reserved_points ?? task.estimated_points ?? '--'
  return <article className="video-task-row" data-status={task.status}>
    <div className="video-task-icon">{terminal ? task.status === 'failed' || task.status === 'cancelled' ? <Ban size={20} /> : <Film size={20} /> : <LoaderCircle className="animate-spin" size={20} />}</div>
    <div className="video-task-copy"><div><strong>{stageLabels[stage] ?? stage}</strong><span>{task.requested_output_count} 个方案 · {task.duration_seconds} 秒 · {task.resolution.toUpperCase()}</span></div><p>{task.progress_message || task.prompt_template}</p><div className="video-task-items">{task.items.map((item) => item.result_asset_id ? <button key={item.id} type="button" data-status={item.status} onClick={() => onResult(item.result_asset_id!)}>#{item.ordinal + 1} 查看结果{item.actual_points ? ` · ${item.actual_points} 积分` : ''}</button> : <span key={item.id} data-status={item.status}>#{item.ordinal + 1} {stageLabels[item.stage || item.status] ?? item.status}{item.actual_points ? ` · ${item.actual_points} 积分` : ''}</span>)}</div></div>
    <div className="video-task-actions"><span><Clock3 size={14} />预留 {reserved}</span><span>实扣 {charged}</span><button type="button" onClick={onDetail}><Info size={15} />详情</button>{!terminal ? <button type="button" disabled={busy} onClick={onCancel}><Ban size={15} />取消</button> : <button type="button" onClick={onReuse}><RotateCcw size={15} />复用参数</button>}<button type="button" title="刷新任务" disabled={busy} onClick={onRefresh}><RefreshCw className={busy ? 'animate-spin' : undefined} size={15} /></button></div>
  </article>
}

function VideoTaskDetailDialog({ task, onClose, onResult, onReuse }: { task: VideoTask; onClose: () => void; onResult: (assetID: string) => void; onReuse: () => void }) {
  const accounting = buildVideoTaskAccounting(task)
  return <Modal title="视频任务详情" onClose={onClose} className="video-task-detail-dialog">
    <div className="video-task-detail-summary">
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

function replaceInput(inputs: VideoDraft['inputs'], role: 'first_frame' | 'last_frame', assetId: string) {
  const filtered = inputs.filter((item) => item.role !== role)
  return assetId ? [...filtered, { asset_id: assetId, role, ordinal: role === 'first_frame' ? 0 : 1 }] : filtered
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
