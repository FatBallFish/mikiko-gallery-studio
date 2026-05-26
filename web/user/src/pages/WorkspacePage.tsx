import { ChangeEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import type { Capability, CapabilityModelGroup, EstimateResult, ImageResult, ImageTask, ImageTaskStatus, ImageTaskType, ReferenceAsset } from '../../../shared/api-types'
import { toTask, userApi } from '../../../shared/user-api'
import { EmptyState, ImageLightbox, LoadingState, PublicDetailIcon, StatusPill, copyText, formatDate, publicDetailButton, taskTypeLabel, useApp } from '../components'
import { errorMessage } from '../useApiResource'

type WorkspaceMode = 'reference' | 'text'
const editContextKey = 'pic-gallery-edit-context'

function selectableModels(capability: Capability, taskType: ImageTaskType) {
  return capability.model_groups.filter((item) => item.task_types.includes(taskType))
}

function qualityOptions(model: CapabilityModelGroup | undefined) {
  return model?.qualities?.length ? model.qualities : []
}

function ratioOptions(model: CapabilityModelGroup | undefined, capability: Capability | null) {
  if (!model) return []
  return model.aspect_ratios?.length ? model.aspect_ratios : capability?.aspect_ratios ?? []
}

function countOptions(model: CapabilityModelGroup | undefined, capability: Capability | null) {
  if (!model) return []
  const maxCount = Number(model.max_output_image_count ?? capability?.max_image_count ?? 0)
  return Array.from({ length: Math.max(0, maxCount) }, (_, index) => index + 1)
}

function isTerminalStatus(status: ImageTaskStatus | string) {
  return ['succeeded', 'partial_failed', 'failed', 'cancelled', 'rejected', 'deleted'].includes(status)
}

function mergeGenerationRecord(records: ImageTask[], next: ImageTask) {
  const map = new Map(records.map((item) => [item.id, item]))
  const current = map.get(next.id)
  if (current?.reference_assets?.length && !next.reference_assets?.length) {
    next = { ...next, reference_assets: current.reference_assets }
  }
  map.set(next.id, next)
  return Array.from(map.values())
    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    .slice(-20)
}

function displayTaskPoints(task: ImageTask) {
  const raw = task.actual_points ?? task.estimate_points ?? task.estimated_points ?? '0.00000'
  const value = Number(raw)
  if (!Number.isFinite(value)) return raw
  return value.toFixed(2)
}

export function WorkspacePage() {
  const app = useApp()
  const [mode, setMode] = useState<WorkspaceMode>('text')

  const [capability, setCapability] = useState<Capability | null>(null)
  const [refs, setRefs] = useState<ReferenceAsset[]>([])
  const [editRefs, setEditRefs] = useState<ReferenceAsset[]>([])
  const [prompt, setPrompt] = useState('')
  const [negative, setNegative] = useState('')
  const [model, setModel] = useState('')
  const [quality, setQuality] = useState('')
  const [ratio, setRatio] = useState('')
  const [count, setCount] = useState(0)
  const [estimate, setEstimate] = useState<EstimateResult | null>(null)
  const [records, setRecords] = useState<ImageTask[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [previewImage, setPreviewImage] = useState<{ url: string; alt: string } | null>(null)
  const streamRef = useRef<EventSource | null>(null)
  const completedNoticeRef = useRef<Set<string>>(new Set())
  const feedEndRef = useRef<HTMLDivElement | null>(null)
  const taskType: ImageTaskType = mode === 'reference' ? 'reference_to_image' : editRefs.length ? 'image_edit' : 'text_to_image'

  // Load capability and refs on mount only (not on mode change)
  useEffect(() => {
    let mounted = true
    async function load() {
      setLoading(true)
      try {
        const [nextCapability, nextRefs] = await Promise.all([userApi.getCapabilities(), userApi.listReferenceAssets()])
        if (!mounted) return
        setCapability(nextCapability)
        setRefs(nextRefs)
      } catch (err) {
        if (mounted) app.notify('error', errorMessage(err))
      } finally {
        if (mounted) setLoading(false)
      }
    }
    void load()
    return () => { mounted = false }
  }, [app])

  useEffect(() => {
    const token = app.session?.token
    if (!token) return undefined
    streamRef.current?.close()
    const source = new EventSource(userApi.taskStreamUrl(token))
    source.addEventListener('history', (event) => {
      const tasks = JSON.parse((event as MessageEvent).data).map(toTask) as ImageTask[]
      setRecords(tasks.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()))
    })
    source.addEventListener('task', (event) => {
      const next = toTask(JSON.parse((event as MessageEvent).data))
      setRecords((items) => mergeGenerationRecord(items, next))
      if (isTerminalStatus(next.status) && next.status === 'succeeded' && !completedNoticeRef.current.has(next.id)) {
        completedNoticeRef.current.add(next.id)
        app.notify('success', '任务已完成，结果已同步到历史图库')
        void app.refreshAccount()
      }
    })
    source.addEventListener('error', () => {
      app.notify('error', '任务状态连接已断开，请稍后刷新页面查看最新结果。')
    })
    streamRef.current = source
    return () => {
      source.close()
      if (streamRef.current === source) streamRef.current = null
    }
  }, [app])

  useEffect(() => {
    feedEndRef.current?.scrollIntoView({ block: 'end' })
  }, [records])

  useEffect(() => {
    const raw = window.sessionStorage.getItem(editContextKey)
    if (!raw) return
    window.sessionStorage.removeItem(editContextKey)
    const storedRaw = raw
    let cancelled = false
    async function restoreEditContext() {
      try {
        const context = JSON.parse(storedRaw) as { prompt?: string; sources?: ReferenceAsset[]; fallbackImageUrl?: string }
        setMode('text')
        setPrompt(context.prompt ?? '')
        setNegative('')
        const sources = (context.sources ?? []).filter((item) => item.id || item.preview_url)
        if (sources.length) {
          setEditRefs(sources)
          app.notify('success', '已恢复图片编辑上下文')
          return
        }
        if (context.fallbackImageUrl) {
          setBusy(true)
          const response = await fetch(context.fallbackImageUrl)
          if (!response.ok) throw new Error('图片读取失败，请稍后重试。')
          const blob = await response.blob()
          const file = new File([blob], `gallery-edit-${Date.now()}.png`, { type: blob.type || 'image/png' })
          const asset = await userApi.uploadReferenceAsset(file)
          if (!cancelled) {
            setEditRefs([{ ...asset, preview_url: asset.preview_url || context.fallbackImageUrl }])
            app.notify('success', '已恢复图片编辑上下文')
          }
        }
      } catch (err) {
        if (!cancelled) app.notify('error', errorMessage(err))
      } finally {
        if (!cancelled) setBusy(false)
      }
    }
    void restoreEditContext()
    return () => { cancelled = true }
  }, [app])

  // Reset form fields when mode tab switches, but keep generated records intact.
  useEffect(() => {
    setPrompt('')
    setNegative('')
  }, [mode])

  useEffect(() => {
    if (capability) {
      const nextModels = selectableModels(capability, taskType)
      if (!nextModels.some((item) => item.code === model)) setModel(nextModels[0]?.code ?? '')
    }
  }, [taskType, capability, model])

  const availableModels = useMemo(() => capability ? selectableModels(capability, taskType) : [], [capability, taskType])
  const selectedModel = useMemo(() => availableModels.find((item) => item.code === model), [availableModels, model])
  const qualities = useMemo(() => qualityOptions(selectedModel), [selectedModel])
  const ratios = useMemo(() => ratioOptions(selectedModel, capability), [selectedModel, capability])
  const counts = useMemo(() => countOptions(selectedModel, capability), [selectedModel, capability])

  useEffect(() => {
    setQuality(qualities[0] ?? '')
    setRatio(ratios[0] ?? '')
    setCount(counts[0] ?? 0)
  }, [taskType, selectedModel?.code, qualities, ratios, counts])

  const parametersReady = Boolean(
    selectedModel
    && quality
    && ratio
    && count
    && qualities.includes(quality)
    && ratios.includes(ratio)
    && counts.includes(count),
  )

  const estimatePayload = useMemo(() => ({
    task_type: taskType,
    route_model_code: model,
    quality,
    aspect_ratio: ratio,
    image_count: count,
    reference_asset_ids: taskType === 'image_edit' ? editRefs.map((item) => item.id) : taskType === 'reference_to_image' ? refs.map((item) => item.id) : [],
  }), [taskType, model, quality, ratio, count, refs, editRefs])

  useEffect(() => {
    if (!capability || !model || !parametersReady) {
      setEstimate(null)
      return undefined
    }
    let cancelled = false
    const timer = window.setTimeout(async () => {
      try {
        const nextEstimate = await userApi.estimate(estimatePayload)
        if (!cancelled) setEstimate(nextEstimate)
      } catch (err) {
        if (!cancelled) app.notify('error', errorMessage(err))
      }
    }, 180)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [app, capability, estimatePayload, model, parametersReady])

  async function uploadReference(event: ChangeEvent<HTMLInputElement>, target: 'edit' | 'reference') {
    const files = Array.from(event.target.files ?? [])
    if (!files.length) return
    setBusy(true)
    try {
      const uploaded = await Promise.all(files.map((file) => userApi.uploadReferenceAsset(file)))
      if (target === 'edit') {
        setEditRefs((items) => [...uploaded, ...items])
      } else {
        setRefs((items) => [...uploaded, ...items])
      }
      app.notify('success', `已上传 ${uploaded.length} 张参考图`)
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      event.target.value = ''
      setBusy(false)
    }
  }

  async function createTask() {
    if (!model || !parametersReady) {
      app.notify('error', '暂无可用参数，请先在管理后台启用路由模型及参数。')
      return
    }
    const activeTaskType = taskType
    const editSourceAssets = activeTaskType === 'image_edit' ? [...editRefs] : []
    setBusy(true)
    try {
      const task = await userApi.createTask({ ...estimatePayload, prompt, negative_prompt: negative, idempotency_key: crypto.randomUUID() })
      const nextTask = editSourceAssets.length ? { ...task, reference_assets: editSourceAssets } : task
      setRecords((items) => mergeGenerationRecord(items, nextTask))
      if (activeTaskType === 'image_edit') {
        setPrompt('')
        setNegative('')
        setEditRefs([])
      }
      app.notify('info', '任务已进入队列，正在等待实时状态')
      await app.refreshAccount()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function applyAsEditSource(url: string) {
    setBusy(true)
    try {
      const response = await fetch(url)
      if (!response.ok) throw new Error('图片读取失败，请稍后重试。')
      const blob = await response.blob()
      const file = new File([blob], `generated-reference-${Date.now()}.png`, { type: blob.type || 'image/png' })
      const asset = await userApi.uploadReferenceAsset(file)
      setMode('text')
      setEditRefs((items) => [{ ...asset, preview_url: asset.preview_url || url }, ...items])
      app.notify('success', '已加入图片编辑')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  function removeReferenceAsset(asset: ReferenceAsset) {
    setRefs((items) => items.filter((item) => (
      asset.id ? item.id !== asset.id : item.preview_url !== asset.preview_url
    )))
  }

  function removeEditAsset(asset: ReferenceAsset) {
    setEditRefs((items) => items.filter((item) => (
      asset.id ? item.id !== asset.id : item.preview_url !== asset.preview_url
    )))
  }

  return (
    <div className="workspace">
      {/* Left Parameter Panel */}
      <aside className="panel">
        {/* Tabs */}
        <div className="panel-section">
          <div className="tabs">
            <button type="button" className={mode === 'text' ? 'tab active' : 'tab'} onClick={() => setMode('text')}>文生图片</button>
            <button type="button" className={mode === 'reference' ? 'tab active' : 'tab'} onClick={() => setMode('reference')}>参考生图</button>
          </div>

          <div className="panel-header">
            <h2>{mode === 'text' ? '文生图' : '参考生图'}</h2>
          </div>
          <p style={{ margin: '0 0 16px', color: 'var(--muted)', fontSize: 14 }}>
            {mode === 'text' ? '通过文字描述直接生成图片；添加图片后会进入二次编辑模式。' : '参考多图元素，生成全新图片'}
          </p>

          {/* Reference upload area (only for reference mode) */}
          {mode === 'reference' ? (
            <>
              <div className="upload-strip">
                <label className="ref-thumb" style={{ cursor: 'pointer' }}>
                  <input type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'reference')} style={{ display: 'none' }} />
                  <span>+ 图片</span>
                </label>
                <label className="ref-thumb" style={{ cursor: 'pointer' }}>
                  <input type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'reference')} style={{ display: 'none' }} />
                  <span>+ 主体</span>
                </label>
              </div>
              {refs.length ? (
                <div className="ref-grid">
                  {refs.map((asset) => (
                    <div key={asset.id || asset.preview_url} className="ref-tile">
                      <img src={asset.preview_url} alt={asset.name} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                      <button type="button" className="ref-remove" title="移除参考图" onClick={() => removeReferenceAsset(asset)}>移除</button>
                    </div>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          {mode === 'text' ? (
            <div className="edit-source-panel">
              <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>图片编辑来源</label>
              <div className="edit-upload-row">
                <label className="edit-upload-button" style={{ cursor: 'pointer' }}>
                  <input type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'edit')} style={{ display: 'none' }} />
                  <span>+ 上传原图</span>
                </label>
              </div>
              {editRefs.length ? (
                <div className="ref-grid">
                  {editRefs.map((asset) => (
                    <div key={asset.id || asset.preview_url} className="ref-tile">
                      <img src={asset.preview_url} alt={asset.name} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                      <button type="button" className="ref-remove" title="移除编辑图片" onClick={() => removeEditAsset(asset)}>移除</button>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* Prompt */}
          <div style={{ marginTop: mode === 'reference' ? 24 : 0 }}>
            <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>提示词 (PROMPT)</label>
            <textarea
              className="input-area"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={5}
              placeholder="描述想要生成的内容..."
            />
          </div>

          {/* Negative prompt (collapsed) */}
          <details style={{ marginTop: 12 }}>
            <summary style={{ fontSize: 12, color: 'var(--muted)', cursor: 'pointer' }}>限制词 (NEGATIVE PROMPT)</summary>
            <textarea
              className="input-area"
              value={negative}
              onChange={(e) => setNegative(e.target.value)}
              rows={2}
              style={{ marginTop: 8 }}
            />
          </details>
        </div>

        {/* Parameters */}
        <div className="panel-section">
          {/* Model */}
          <div style={{ marginBottom: 20 }}>
            <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>模型选择</label>
            {loading && !capability ? <LoadingState label="正在加载可用模型..." /> : null}
            {!loading && availableModels.length ? availableModels.map((m) => (
              <button
                key={m.code}
                type="button"
                className={model === m.code ? 'select-item active' : 'select-item'}
                style={{ width: '100%', display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}
                onClick={() => setModel(m.code)}
              >
                <span>{m.name}</span>
                <span className="num" style={{ fontSize: 12, color: model === m.code ? 'var(--accent)' : 'var(--muted)' }}>{m.display_points ? `${m.display_points} ◈` : m.effective_multiplier ? `${m.effective_multiplier}x` : ''}</span>
              </button>
            )) : null}
            {!loading && !availableModels.length ? <EmptyState title="暂无可用模型" detail="请在管理后台启用路由模型，并确认当前用户分组可见。" /> : null}
          </div>

          {selectedModel ? (
            <>
              {/* Quality */}
              {qualities.length ? (
                <div style={{ marginBottom: 20 }}>
                  <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>清晰度 (QUALITY)</label>
                  <div className="select-grid">
                    {qualities.map((q) => (
                      <button
                        key={q}
                        type="button"
                        className={quality === q ? 'select-item active' : 'select-item'}
                        onClick={() => setQuality(q)}
                      >
                        {q === 'auto' ? 'Auto' : q}
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}

              {/* Aspect Ratio */}
              {ratios.length ? (
                <div style={{ marginBottom: 20 }}>
                  <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>比例 (ASPECT RATIO)</label>
                  <div className="select-grid three">
                    {ratios.map((r) => (
                      <button
                        key={r}
                        type="button"
                        className={ratio === r ? 'select-item active' : 'select-item'}
                        onClick={() => setRatio(r)}
                      >
                        {r}
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}

              {/* Image Count */}
              {counts.length ? (
                <div>
                  <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>图片数量</label>
                  <div className="select-grid">
                    {counts.map((n) => (
                      <button
                        key={n}
                        type="button"
                        className={count === n ? 'select-item active' : 'select-item'}
                        onClick={() => setCount(n)}
                      >
                        {n} 张
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}
            </>
          ) : null}
        </div>

        {/* Estimate & Create */}
        <div className="panel-section" style={{ borderBottom: 'none', marginTop: 'auto' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12, fontSize: 13, color: 'var(--muted)' }}>
            <span>预估消耗</span>
            <span className="num" style={{ color: 'var(--accent)', fontWeight: 700 }}>{estimate?.display_points ?? estimate?.points ?? '...'} ◈</span>
          </div>
          {estimate && !estimate.sufficient ? (
            <div className="form-error" style={{ marginBottom: 12 }}>积分不足，请降低质量或充值。</div>
          ) : null}
          <button
            className="create-btn btn-primary"
            type="button"
            disabled={!model || !parametersReady || !estimate?.sufficient || prompt.trim().length < 8 || busy}
            onClick={() => void createTask()}
          >
            {busy ? (
              <>
                <span className="spinner" />
                生成中...
              </>
            ) : (
              <>
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="M5 12h14M12 5l7 7-7 7" /></svg>
                开始创作
              </>
            )}
          </button>
        </div>
      </aside>

      {/* Right Canvas */}
      <section className="canvas generation-canvas">
        {records.length ? (
          <div className="generation-feed">
            {records.map((task) => (
              <GenerationRecord
                key={task.id}
                task={task}
                onCopyPrompt={async () => {
                  await copyText(task.prompt)
                  app.notify('success', '提示词已复制')
                }}
                onUseReference={applyAsEditSource}
                onPublishImage={async (image) => {
                  if (!image.id) {
                    app.notify('error', '图片记录还未同步完成，请稍后再发布。')
                    return
                  }
                  try {
                    await userApi.publishImage(image.id)
                    app.notify('success', '已提交公开审核')
                  } catch (err) {
                    app.notify('error', errorMessage(err))
                  }
                }}
                onOpenGallery={() => app.navigate('gallery')}
                onPreviewImage={(url, alt) => setPreviewImage({ url, alt })}
                accessToken={app.session?.token}
                onUnavailable={() => app.notify('info', '该工具暂不可用')}
              />
            ))}
            <div ref={feedEndRef} />
          </div>
        ) : (
          <div className="canvas-placeholder generation-empty">
            <div>
              <h2>开始创作您的第一个作品吧！</h2>
              <p>在左侧面板设置参数并点击"开始创作"</p>
            </div>
          </div>
        )}

        {/* Floating Feedback */}
        <div className="floating-feedback">
          <button type="button" className="feedback-btn" title="确认完成">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 6L9 17l-5-5" /></svg>
          </button>
        </div>

        <ImageLightbox image={previewImage} onClose={() => setPreviewImage(null)} />

      </section>
    </div>
  )
}

function GenerationRecord({ task, onCopyPrompt, onUseReference, onPublishImage, onOpenGallery, onPreviewImage, accessToken, onUnavailable }: {
  task: ImageTask
  onCopyPrompt: () => Promise<void>
  onUseReference: (url: string) => Promise<void>
  onPublishImage: (image: ImageResult) => Promise<void>
  onOpenGallery: () => void
  onPreviewImage: (url: string, alt: string) => void
  accessToken?: string
  onUnavailable: () => void
}) {
  return (
    <article className="generation-record">
      <header className="generation-record-head">
        <div className="record-avatar" aria-hidden="true">PG</div>
        <div>
          <div className="record-title">
            <b>{taskTypeLabel(task.task_type)}</b>
            <span>{formatDate(task.created_at)}</span>
            <StatusPill status={task.status} />
          </div>
          <p>{task.prompt}</p>
          <div className="record-params">
            <span>模型 {task.model_group}</span>
            <span>清晰度 {task.quality}</span>
            <span>比例 {task.aspect_ratio}</span>
            <span>数量 {task.image_count}</span>
            <span>本次消耗 {displayTaskPoints(task)} 积分</span>
          </div>
        </div>
      </header>

      {task.results.length ? (
        <>
          {task.task_type === 'image_edit' && task.reference_assets.length ? (
            <div className="record-source-images">
              <span>原图引用</span>
              {task.reference_assets.map((asset) => asset.preview_url ? (
                <button key={asset.id || asset.preview_url} type="button" onClick={() => onPreviewImage(userApi.imageAssetUrl(asset.preview_url || '', accessToken), asset.name || '原图引用')}>
                  <img src={userApi.imageAssetUrl(asset.preview_url || '', accessToken)} alt={asset.name || '原图引用'} />
                </button>
              ) : null)}
            </div>
          ) : null}
          <div className="record-images">
            {task.results.map((image) => (
              <GeneratedImage
                key={image.id}
                image={image}
                alt={task.title}
                fallbackRatio={task.aspect_ratio}
                accessToken={accessToken}
                onUseReference={onUseReference}
                onPublish={onPublishImage}
                onPreview={onPreviewImage}
                onUnavailable={onUnavailable}
              />
            ))}
          </div>
        </>
      ) : isTerminalStatus(task.status) ? (
        <div className="record-pending record-pending-failed">
          <strong>{task.status === 'failed' || task.status === 'rejected' ? '生成失败' : '没有可用结果'}</strong>
          <p>{task.failure_reason || '本次任务没有生成图片，请调整提示词、参考图或参数后重试。'}</p>
        </div>
      ) : (
        <div className="record-pending">
          <span className="spinner" />
          <strong>{task.status === 'queued' ? '排队中' : task.status === 'running' ? '生成中' : '等待结果'}</strong>
          <p>任务状态会通过实时连接自动更新，完成后图片会出现在这里。</p>
        </div>
      )}

      <footer className="record-actions">
        {publicDetailButton('复制提示词', <PublicDetailIcon name="copy" />, () => void onCopyPrompt())}
        {publicDetailButton('查看图库', <PublicDetailIcon name="group" />, onOpenGallery)}
      </footer>
    </article>
  )
}

function normalizeAspectRatio(input?: string) {
  if (!input) return undefined
  const colon = input.match(/^(\d+(?:\.\d+)?):(\d+(?:\.\d+)?)$/)
  if (colon) return `${colon[1]} / ${colon[2]}`
  const size = input.match(/^(\d+)x(\d+)$/i)
  if (size) return `${size[1]} / ${size[2]}`
  return undefined
}

function GeneratedImage({ image, alt, fallbackRatio, accessToken, onUseReference, onPublish, onPreview, onUnavailable }: {
  image: ImageResult
  alt: string
  fallbackRatio?: string
  accessToken?: string
  onUseReference: (url: string) => Promise<void>
  onPublish: (image: ImageResult) => Promise<void>
  onPreview: (url: string, alt: string) => void
  onUnavailable: () => void
}) {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  const downloadUrl = userApi.imageAssetUrl(image.download_url ?? image.url, accessToken)
  const aspectRatio = image.width && image.height ? `${image.width} / ${image.height}` : normalizeAspectRatio(fallbackRatio)
  const ratioStyle = aspectRatio ? { '--generated-ratio': aspectRatio } as CSSProperties : undefined
  return (
    <figure className="generated-image" style={ratioStyle}>
      <button type="button" className="generated-image-preview" onClick={() => onPreview(imageUrl, alt)} aria-label="预览生成图片">
        <img src={imageUrl} alt={alt} />
      </button>
      <figcaption>
        <button type="button" title="编辑" onClick={() => void onUseReference(imageUrl)}>编辑</button>
        <button type="button" title="下载" onClick={() => window.open(downloadUrl, '_blank', 'noopener,noreferrer')}>下载</button>
        <button type="button" title="发布" onClick={() => void onPublish(image)}>发布</button>
        <button type="button" title="标记" onClick={onUnavailable}>标记</button>
        <button type="button" title="更多" onClick={onUnavailable}>更多</button>
      </figcaption>
    </figure>
  )
}
