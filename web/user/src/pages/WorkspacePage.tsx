import { ChangeEvent, useEffect, useMemo, useState } from 'react'
import type { Capability, EstimateResult, ImageTask, ImageTaskType, ReferenceAsset } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, StatusPill, copyText, useApp } from '../components'
import { errorMessage } from '../useApiResource'

type WorkspaceMode = 'reference' | 'text'

const promptSeeds = {
  reference: '设计一个带金属编织感的未来艺廊大厅，主色为深蓝、琥珀与孔雀绿，空间中悬浮半透明雕塑与镜面反射，保留参考图中的主体姿态与材质层次。',
  text: 'A cinematic luminous vault for AI art creation, dark blue glass, amber metal threads, emerald fog, editorial lighting, ultra detailed.',
}

export function WorkspacePage() {
  const app = useApp()
  const [mode, setMode] = useState<WorkspaceMode>('text')
  const taskType: ImageTaskType = mode === 'text' ? 'text_to_image' : 'reference_to_image'

  const [capability, setCapability] = useState<Capability | null>(null)
  const [refs, setRefs] = useState<ReferenceAsset[]>([])
  const [prompt, setPrompt] = useState(promptSeeds[mode])
  const [negative, setNegative] = useState('低清晰度、畸形手部、重复主体、文字水印')
  const [model, setModel] = useState('plus-image')
  const [quality, setQuality] = useState('auto')
  const [ratio, setRatio] = useState('16:9')
  const [count, setCount] = useState(2)
  const [estimate, setEstimate] = useState<EstimateResult | null>(null)
  const [activeTask, setActiveTask] = useState<ImageTask | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Load capability and refs on mount only (not on mode change)
  useEffect(() => {
    let mounted = true
    async function load() {
      setLoading(true)
      setError(null)
      try {
        const [nextCapability, nextRefs] = await Promise.all([userApi.getCapabilities(), userApi.listReferenceAssets()])
        if (!mounted) return
        setCapability(nextCapability)
        setRefs(nextRefs)
        setModel(nextCapability.model_groups.find((item) => item.supports_reference || mode === 'text')?.id ?? nextCapability.model_groups[0]?.id ?? 'plus-image')
      } catch (err) {
        if (mounted) setError(errorMessage(err))
      } finally {
        if (mounted) setLoading(false)
      }
    }
    void load()
    return () => { mounted = false }
  }, [])

  // Reset form fields when mode tab switches, but keep canvas (activeTask) intact
  useEffect(() => {
    setPrompt(promptSeeds[mode])
    setError(null)
    // Update model selection if current model doesn't support reference mode
    if (capability) {
      const currentModel = capability.model_groups.find((m) => m.id === model)
      if (mode === 'reference' && currentModel && !currentModel.supports_reference) {
        const refModel = capability.model_groups.find((m) => m.supports_reference)
        if (refModel) setModel(refModel.id)
      }
    }
  }, [mode, capability])

  const estimatePayload = useMemo(() => ({
    task_type: taskType,
    model_group: model,
    quality,
    aspect_ratio: ratio,
    image_count: count,
    reference_asset_ids: mode === 'text' ? [] : refs.map((item) => item.id),
  }), [taskType, model, quality, ratio, count, refs, mode])

  useEffect(() => {
    if (!capability) return undefined
    let cancelled = false
    const timer = window.setTimeout(async () => {
      try {
        setEstimate(await userApi.estimate(estimatePayload))
      } catch (err) {
        if (!cancelled) setError(errorMessage(err))
      }
    }, 180)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [capability, estimatePayload])

  useEffect(() => {
    if (!activeTask || activeTask.status === 'succeeded' || activeTask.status === 'failed' || activeTask.status === 'cancelled') return undefined
    const timer = window.setTimeout(async () => {
      try {
        const next = await userApi.getTask(activeTask.id)
        setActiveTask(next)
        if (next.status === 'succeeded') {
          app.notify('success', '任务已完成，结果已同步到历史图库')
          await app.refreshAccount()
        }
      } catch (err) {
        app.notify('error', errorMessage(err))
      }
    }, 1300)
    return () => window.clearTimeout(timer)
  }, [activeTask, app])

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files ?? [])
    if (!files.length) return
    setBusy(true)
    setError(null)
    try {
      const uploaded = await Promise.all(files.map((file) => userApi.uploadReferenceAsset(file)))
      setRefs((items) => [...uploaded, ...items])
      app.notify('success', `已上传 ${uploaded.length} 张参考图`)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      event.target.value = ''
      setBusy(false)
    }
  }

  async function createTask() {
    setBusy(true)
    setError(null)
    try {
      const task = await userApi.createTask({ ...estimatePayload, prompt, negative_prompt: negative, idempotency_key: crypto.randomUUID() })
      setActiveTask(task)
      app.notify('info', '任务已进入队列，正在轮询进度')
      await app.refreshAccount()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function applyAsReference(url: string) {
    const asset = await userApi.uploadReferenceAsset(`result-${Date.now()}.png`, 1_024_000)
    setRefs((items) => [{ ...asset, preview_url: url }, ...items])
    app.notify('success', '已作为参考素材加入工作台')
  }

  if (loading) return <LoadingState />
  if (error && !capability) return <ErrorState message={error} />

  const qualities = capability?.qualities ?? ['auto', '1K', '2K', '4K']
  const ratios = capability?.aspect_ratios ?? ['1:1', '16:9', '9:16', '4:3']
  const maxCount = capability?.max_image_count ?? 4

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
            {mode === 'text' ? '通过文字描述直接生成图片' : '参考多图元素，生成全新图片'}
          </p>

          {/* Reference upload area (only for reference mode) */}
          {mode === 'reference' ? (
            <>
              <div className="upload-strip">
                <label className="ref-thumb" style={{ cursor: 'pointer' }}>
                  <input type="file" accept="image/*" multiple disabled={busy} onChange={upload} style={{ display: 'none' }} />
                  <span>+ 图片</span>
                </label>
                <label className="ref-thumb" style={{ cursor: 'pointer' }}>
                  <input type="file" accept="image/*" multiple disabled={busy} onChange={upload} style={{ display: 'none' }} />
                  <span>+ 主体</span>
                </label>
              </div>
              {refs.length ? (
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 8, marginTop: 12 }}>
                  {refs.map((asset) => (
                    <div key={asset.id} style={{ aspectRatio: '1', borderRadius: 8, overflow: 'hidden', border: '1px solid var(--border)' }}>
                      <img src={asset.preview_url} alt={asset.name} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                    </div>
                  ))}
                </div>
              ) : null}
            </>
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
            {capability?.model_groups.map((m) => (
              <button
                key={m.id}
                type="button"
                className={model === m.id ? 'select-item active' : 'select-item'}
                style={{ width: '100%', display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}
                onClick={() => setModel(m.id)}
                disabled={mode !== 'text' && !m.supports_reference}
              >
                <span>{m.name}</span>
                <span className="num" style={{ fontSize: 12, color: model === m.id ? 'var(--accent)' : 'var(--muted)' }}>{m.provider}</span>
              </button>
            ))}
          </div>

          {/* Quality */}
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

          {/* Aspect Ratio */}
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

          {/* Image Count */}
          <div>
            <label style={{ fontSize: 13, color: 'var(--muted)', fontWeight: 600, display: 'block', marginBottom: 8 }}>图片数量</label>
            <div className="select-grid">
              {Array.from({ length: maxCount }, (_, i) => i + 1).map((n) => (
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
        </div>

        {/* Estimate & Create */}
        <div className="panel-section" style={{ borderBottom: 'none', marginTop: 'auto' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12, fontSize: 13, color: 'var(--muted)' }}>
            <span>预估消耗</span>
            <span className="num" style={{ color: 'var(--accent)', fontWeight: 700 }}>{estimate?.points ?? '...'} ◈</span>
          </div>
          {estimate && !estimate.sufficient ? (
            <div className="form-error" style={{ marginBottom: 12 }}>积分不足，请降低质量或充值。</div>
          ) : null}
          {error ? <div className="form-error" style={{ marginBottom: 12 }}>{error}</div> : null}
          <button
            className="create-btn btn-primary"
            type="button"
            disabled={!estimate?.sufficient || prompt.trim().length < 8 || busy}
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
      <section className="canvas">
        {activeTask?.results.length ? (
          <div className="results-grid">
            {activeTask.results.map((image) => (
              <article key={image.id} className="result-card">
                <img src={image.url} alt={activeTask.title} />
                <div className="result-card-actions">
                  <Button tone="ghost" onClick={() => window.open(image.url, '_blank')}>下载</Button>
                  <Button tone="ghost" onClick={async () => { await copyText(activeTask.prompt); app.notify('success', '提示词已复制') }}>复制 Prompt</Button>
                  <Button tone="ghost" onClick={() => void applyAsReference(image.url)}>作为参考</Button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="canvas-placeholder">
            {activeTask ? (
              <div>
                <h2>{activeTask.title}</h2>
                <p>{activeTask.status} / {activeTask.progress}% / {activeTask.provider}</p>
                <div style={{ marginTop: 24 }}>
                  <StatusPill status={activeTask.status} />
                </div>
              </div>
            ) : (
              <div>
                <h2>开始创作您的第一个作品吧！</h2>
                <p>在左侧面板设置参数并点击"开始创作"</p>
              </div>
            )}
          </div>
        )}

        {/* Floating Feedback */}
        <div className="floating-feedback">
          <div className="feedback-btn" style={{ color: 'var(--accent)', padding: '10px 18px', width: 'auto', borderRadius: 999 }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" /></svg>
            奖励合计 80 积分
          </div>
          <button type="button" className="feedback-btn" title="确认完成">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 6L9 17l-5-5" /></svg>
          </button>
        </div>

        {/* Progress Band */}
        {activeTask ? (
          <div className="progress-band">
            <div><span>任务状态</span><StatusPill status={activeTask.status} /></div>
            <div><span>进度</span><b>{activeTask.progress}%</b></div>
            <div><span>积分变动</span><b>{activeTask.estimate_points} ◈</b></div>
            <div><span>下一步</span><button type="button" className="btn-ghost" onClick={() => app.navigate('gallery')}>查看历史图库</button></div>
          </div>
        ) : null}
      </section>
    </div>
  )
}
