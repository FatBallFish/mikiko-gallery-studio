import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { ArrowUp, Check, ChevronDown, Expand, FolderOpen, ImagePlus, LoaderCircle, MousePointer2, Plus, Shrink, SlidersHorizontal, Sparkles, Undo2, Upload, X } from 'lucide-react'
import type { Capability, PromptOptimizationEstimate, ReferenceAsset, VideoCapability } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Modal } from '../../components'
import { PromptTemplateEditor } from '../../pages/PromptTemplateEditor'
import { PromptVariableForm } from '../../pages/PromptVariableForm'
import { parsePromptTemplate } from '../../pages/promptTemplateParser'
import { VIDEO_TASK_INPUT_ROLES, videoTaskInputLabel, type VideoDraft, type VideoDraftInputRole } from '../creation/videoDraft'
import type { CanvasInputRole } from './core/types'
import { workspaceTaskImageSafetyLimit } from '../../pages/workspaceViewModel'
import { canvasMediaParameterLabel, canvasMediaParameterSummary, canvasConsoleParameterErrors, canvasImageModel, canvasPromptTemplate, canvasPromptVariables, canvasVideoDraft, normalizeCanvasImageDraft, normalizeCanvasVideoDraft } from './canvasMediaV2'
import { canvasMediaRunLabel, type CanvasMediaNodeV2Data, type CanvasMediaRun } from './CanvasMediaNodeV2'
import './canvasMediaV2.css'

export type CanvasConsoleReference = { id: string; alias: string; title: string; asset: ReferenceAsset; role?: CanvasInputRole }
export type CanvasConsoleEstimate = { estimated_points?: string | number; display_points?: string; sufficient?: boolean; valid?: boolean; error?: string }
export type CanvasGenerationConsoleProps = {
  node: CanvasMediaNodeV2Data
  draft: Record<string, unknown>
  imageCapability: Capability | null
  videoCapability: VideoCapability | null
  references: CanvasConsoleReference[]
  estimate?: CanvasConsoleEstimate | null
  run?: CanvasMediaRun | null
  busy?: boolean
  estimating?: boolean
  accessToken?: string | null
  onPatchDraft: (patch: Record<string, unknown>) => void
  onUpload: (role?: CanvasInputRole) => void
  onLibrary: (role?: CanvasInputRole) => void
  onPickCanvasReference: (role?: CanvasInputRole) => void
  onRemoveReference: (id: string) => void
  onGenerate: () => void
  onEstimate?: () => void
  style?: CSSProperties
  className?: string
}

const taskLabels: Record<string, string> = { text_to_image: '文生图', image_edit: '图片编辑', text_to_video: '文生视频', image_to_video: '图生视频', first_last_frame_to_video: '首尾帧视频', reference_to_video: '参考生视频', video_edit: '视频编辑', video_extend: '视频延长' }

export function CanvasGenerationConsole(props: CanvasGenerationConsoleProps) {
  const { node, draft, imageCapability, videoCapability, references, estimate, run, busy = false, estimating = false, accessToken, onPatchDraft, onUpload, onLibrary, onPickCanvasReference, onRemoveReference, onGenerate, onEstimate, style, className = '' } = props
  const [expanded, setExpanded] = useState(false)
  const [menu, setMenu] = useState(false)
  const [parameters, setParameters] = useState(false)
  const [role, setRole] = useState<CanvasInputRole | undefined>()
  const [optimizer, setOptimizer] = useState<{ stage: 'idle' | 'estimating' | 'confirming' | 'running' | 'applied' | 'error'; original: string; result?: string; quote?: PromptOptimizationEstimate; error?: string }>({ stage: 'idle', original: '' })
  const inlineHost = useRef<HTMLDivElement>(null)
  const modalHost = useRef<HTMLDivElement>(null)
  // Keep one portal target across overlay moves: Lexical history, selection and draft survive fullscreen.
  const [editorHost] = useState(() => document.createElement('div'))
  const latest = useRef(props)
  latest.current = props
  const operation = useRef(0)
  const video = node.type === 'video_v2'
  const template = canvasPromptTemplate(String(draft.prompt_template ?? draft.prompt ?? ''))
  const variables = canvasPromptVariables(draft)
  const model = imageCapability && !video ? canvasImageModel(draft, imageCapability) : undefined
  const videoDraft = videoCapability && video ? canvasVideoDraft(draft, videoCapability) : undefined
  const videoModel = videoCapability?.model_groups.find((item) => item.code === String(draft.route_model_code ?? ''))
  const videoOptions = videoDraft ? videoModel?.options_by_task_type[videoDraft.task_type] : undefined
  const roles: CanvasInputRole[] = videoDraft ? VIDEO_TASK_INPUT_ROLES[videoDraft.task_type] ?? [] : ['reference']
  const currentRole = roles.includes(role as CanvasInputRole) ? role : roles[0]
  const locked = busy || optimizer.stage === 'running' || optimizer.stage === 'estimating'
  const validationDraft = video ? { ...draft, inputs: references.filter((reference) => reference.role && reference.role !== 'reference' && reference.role !== 'prompt' && reference.role !== 'result').map((reference, ordinal) => ({ asset_id: reference.asset.id, role: reference.role, ordinal })) } : draft
  const errors = canvasConsoleParameterErrors(validationDraft, imageCapability, videoCapability, video)
  const parsed = parsePromptTemplate(template)
  const missingVariable = parsed.variableNames.some((name) => !variables[name]?.trim())
  const missingReference = parsed.referenceNames.some((name) => !references.some((reference) => reference.alias === name && ['ready', 'ready_original'].includes(reference.asset.status)))
  const canGenerate = !locked && !estimating && Boolean(template.trim()) && !parsed.error && !missingVariable && !missingReference && !errors.length && estimate?.valid === true && estimate.sufficient !== false && !estimate.error
  const assets = references.map((reference) => ({ ...reference.asset, name: reference.alias, display_name: reference.title }))

  useLayoutEffect(() => {
    const target = expanded ? modalHost.current : inlineHost.current
    target?.appendChild(editorHost)
  }, [expanded, editorHost])
  useEffect(() => () => { operation.current += 1; editorHost.remove() }, [editorHost])
  useEffect(() => { operation.current += 1; setOptimizer({ stage: 'idle', original: '' }); setMenu(false); setParameters(false) }, [node.id])
  useEffect(() => {
    operation.current += 1
    setOptimizer((state) => state.stage === 'applied' && state.result === template ? state : { stage: 'idle', original: '' })
  }, [template, draft.route_model_code, JSON.stringify(variables)])

  function patch(value: Record<string, unknown>, field?: keyof VideoDraft) {
    let next = { ...draft, ...value }
    if (!video && imageCapability && typeof value.route_model_code === 'string') {
      const selected = imageCapability.model_groups.find((item) => item.code === value.route_model_code)
      if (selected && !selected.task_types.includes(next.task_type === 'image_edit' ? 'image_edit' : 'text_to_image')) next = { ...next, task_type: selected.task_types[0] }
    }
    onPatchDraft(video && videoCapability ? normalizeCanvasVideoDraft(next, videoCapability, field) : !video && imageCapability ? normalizeCanvasImageDraft(next, imageCapability) : value)
  }
  function patchPrompt(value: string) { onPatchDraft({ prompt_template: canvasPromptTemplate(value) }) }
  async function optimize(confirm = false) {
    const id = ++operation.current
    const snapshot = { nodeId: node.id, template, model: String(draft.route_model_code ?? ''), variables: JSON.stringify(variables) }
    const current = () => id === operation.current && latest.current.node.id === snapshot.nodeId && canvasPromptTemplate(String(latest.current.draft.prompt_template ?? latest.current.draft.prompt ?? '')) === snapshot.template && String(latest.current.draft.route_model_code ?? '') === snapshot.model && JSON.stringify(canvasPromptVariables(latest.current.draft)) === snapshot.variables
    try {
      if (!confirm) {
        setOptimizer({ stage: 'estimating', original: template })
        const quote = await userApi.estimatePromptOptimization(template, video ? 'video' : 'image', snapshot.model)
        if (current()) setOptimizer({ stage: 'confirming', original: template, quote })
        return
      }
      if (!optimizer.quote || optimizer.original !== template || Date.parse(optimizer.quote.expires_at) <= Date.now()) { setOptimizer({ stage: 'error', original: template, error: '提示词或报价已变化，请重新优化' }); return }
      setOptimizer({ ...optimizer, stage: 'running' })
      const result = await userApi.optimizePrompt(template, optimizer.quote.quote, video ? 'video' : 'image', snapshot.model)
      if (current()) { const resultPrompt = canvasPromptTemplate(result.optimized_prompt); patchPrompt(resultPrompt); setOptimizer({ stage: 'applied', original: template, result: resultPrompt }) }
    } catch (reason) { if (current()) setOptimizer({ stage: 'error', original: template, error: reason instanceof Error ? reason.message : '提示词优化失败，请重试' }) }
  }

  function select(label: string, key: string, options: readonly (string | number)[] | undefined) {
    if (!options?.length) return null
    return <label className="canvas-console-v2__field" key={key}><span>{label}</span><select value={String(draft[key] ?? '')} disabled={locked} onChange={(event) => patch({ [key]: typeof options[0] === 'number' ? Number(event.target.value) : event.target.value }, key as keyof VideoDraft)}>{!options.map(String).includes(String(draft[key] ?? '')) ? <option value="">请选择</option> : null}{options.map((value) => <option key={value} value={value}>{taskLabels[value] ?? canvasMediaParameterLabel(value)}</option>)}</select></label>
  }

  const content = <section className={`canvas-console-v2 ${expanded ? 'is-expanded' : ''}`} aria-label={video ? '视频生成控制台' : '图片生成控制台'} aria-busy={locked} onPointerDown={(event) => event.stopPropagation()} onWheel={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
    <header className="canvas-console-v2__top">
      <div className="canvas-console-v2__menu-anchor"><button type="button" className="canvas-console-v2__text-button" disabled={locked} aria-expanded={menu} onClick={() => setMenu(!menu)}><Upload size={16} />上传<ChevronDown size={12} /></button>
        {menu ? <div className="canvas-console-v2__upload-menu">
          {roles.length > 1 ? <label>参考用途<select value={currentRole} onChange={(event) => setRole(event.target.value as CanvasInputRole)}>{roles.map((item) => <option value={item} key={item}>{videoTaskInputLabel(item as VideoDraftInputRole)}</option>)}</select></label> : null}
          <button type="button" onClick={() => { setMenu(false); onUpload(currentRole) }}><Upload size={15} />本地上传</button><button type="button" onClick={() => { setMenu(false); onLibrary(currentRole) }}><FolderOpen size={15} />资产库上传</button>
        </div> : null}
      </div>
      <button type="button" className="canvas-console-v2__text-button" disabled={locked} onClick={() => { setMenu(false); onPickCanvasReference(currentRole) }}><MousePointer2 size={15} />画布参考</button>
      <span className="canvas-console-v2__eyebrow">{video ? '视频创作' : '图片创作'}</span>
      <button className="canvas-console-v2__icon-button" type="button" aria-label={expanded ? '收起编辑器' : '全屏编辑'} onClick={() => { setMenu(false); setParameters(false); setExpanded(!expanded) }}>{expanded ? <Shrink size={16} /> : <Expand size={16} />}</button>
    </header>
    {references.length || (video && roles.length) ? <div className="canvas-console-v2__references" aria-label="参考资产">
      {references.map((reference) => <div key={reference.id} className="canvas-console-v2__reference" title={`${reference.title} · @${reference.alias}`}>
        {reference.asset.preview_url && !reference.asset.mime_type?.startsWith('video/') && !reference.asset.mime_type?.startsWith('audio/') ? <img src={reference.asset.preview_url} alt="" /> : <ImagePlus size={18} />}
        <span>{reference.title}<small>@{reference.alias}{reference.role && reference.role !== 'reference' ? ` · ${videoTaskInputLabel(reference.role as VideoDraftInputRole)}` : ''}</small></span><button type="button" disabled={locked} aria-label={`移除参考 ${reference.title}`} onClick={() => onRemoveReference(reference.id)}><X size={12} /></button>
      </div>)}
      {video ? roles.filter((item) => !references.some((reference) => reference.role === item)).map((item) => <button key={item} className="canvas-console-v2__reference-slot" type="button" disabled={locked} onClick={() => { setRole(item); setMenu(true) }}><Plus size={14} />{videoTaskInputLabel(item as VideoDraftInputRole)}</button>) : null}
    </div> : null}
    <div className="canvas-console-v2__prompt"><PromptTemplateEditor value={template} assets={assets} variables={variables} accessToken={accessToken} disabled={locked} expanded={expanded} onChange={patchPrompt} onAddAsset={() => setMenu(true)} placeholder={video ? '描述镜头、动作与氛围… 输入 @ 引用资产，$ 添加变量' : '描述你想看到的画面… 输入 @ 引用资产，$ 添加变量'} /><PromptVariableForm template={template} values={variables} disabled={locked} onChange={(name, value) => onPatchDraft({ variables: { ...variables, [name]: value } })} /></div>
    {parameters ? <div className="canvas-console-v2__parameters" aria-label="生成参数">
      {select('生成方式', 'task_type', video ? videoModel?.task_types : model?.task_types)}
      {video ? <>{select('时长 / 秒', 'duration_seconds', videoOptions?.durations)}{select('分辨率', 'resolution', videoOptions?.resolutions)}{select('画面比例', 'aspect_ratio', videoOptions?.aspect_ratios)}{select('生成数量', 'output_count', Array.from({ length: videoModel?.max_output_count ?? 1 }, (_, index) => index + 1))}{videoOptions?.audio_generation ? <label className="canvas-console-v2__field"><span>生成音频</span><input type="checkbox" checked={videoDraft?.generate_audio ?? false} disabled={locked} onChange={(event) => patch({ generate_audio: event.target.checked, audio_mode: event.target.checked ? 'generated' : 'silent' }, 'generate_audio')} /></label> : null}</> : <>
        {select('尺寸模式', 'size_mode', model?.size_modes)}{draft.size_mode === 'ratio' ? <>{select('基础分辨率', 'base_resolution', model?.base_resolution)}{select('画面比例', 'aspect_ratio', model?.aspect_ratios)}</> : null}{draft.size_mode === 'pixel' ? select('像素尺寸', 'requested_size', model?.pixel_sizes) : null}{select('画质', 'quality', model?.quality ?? model?.qualities)}{select('格式', 'output_format', model?.output_format)}<label className="canvas-console-v2__field"><span>生成数量</span><input type="number" min={1} max={workspaceTaskImageSafetyLimit} step={1} value={Number(draft.output_image_count ?? 1)} disabled={locked} onChange={(event) => patch({ output_image_count: Number(event.target.value) })} /></label>
      </>}
    </div> : null}
    {optimizer.stage === 'confirming' ? <div className="canvas-console-v2__notice" role="status"><span>优化预计消耗 {optimizer.quote?.estimated_points} 积分</span><button type="button" onClick={() => void optimize(true)}>确认优化</button><button type="button" onClick={() => setOptimizer({ stage: 'idle', original: '' })}>取消</button></div> : null}
    {optimizer.stage === 'applied' ? <div className="canvas-console-v2__notice" role="status"><Check size={14} /><span>提示词已优化</span><button type="button" disabled={template !== optimizer.result || locked} onClick={() => { patchPrompt(optimizer.original); setOptimizer({ stage: 'idle', original: '' }) }}><Undo2 size={13} />撤销</button></div> : null}
    {optimizer.error || estimate?.error ? <p className="canvas-console-v2__error" role="alert">{optimizer.error || estimate?.error}</p> : null}
    <footer className="canvas-console-v2__bottom">
      <select className="canvas-console-v2__model" aria-label="生成模型" value={String(draft.route_model_code ?? '')} disabled={locked} onChange={(event) => patch({ route_model_code: event.target.value })}><option value="" disabled>选择模型</option>{(video ? videoCapability?.model_groups : imageCapability?.model_groups)?.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select>
      <button type="button" className="canvas-console-v2__text-button canvas-console-v2__summary" aria-label="生成参数" aria-expanded={parameters} disabled={locked} onClick={() => setParameters(!parameters)}><SlidersHorizontal size={14} /><span>{canvasMediaParameterSummary(draft, video)}</span></button>
      <button type="button" className="canvas-console-v2__icon-button" aria-label="优化提示词" title="优化提示词" disabled={locked || !template.trim()} onClick={() => void optimize()}>{optimizer.stage === 'estimating' || optimizer.stage === 'running' ? <LoaderCircle size={16} className="canvas-media-v2__spinner" /> : <Sparkles size={16} />}</button>
      <span className="canvas-console-v2__points" aria-live="polite">{estimating ? <LoaderCircle className="canvas-media-v2__spinner" size={13} /> : null}{estimating ? '估算中' : estimate?.valid ? `${estimate.display_points ?? estimate.estimated_points ?? '—'} 积分` : '待估算'}</span>
      {onEstimate && !estimate?.valid && !estimating ? <button type="button" className="canvas-console-v2__text-button" disabled={locked || errors.length > 0} onClick={onEstimate}>估算</button> : null}
      <button type="button" className="canvas-console-v2__generate" disabled={!canGenerate} title={errors[0] || (missingVariable ? '请填写模板变量' : estimate?.sufficient === false ? '积分不足' : !estimate?.valid ? '等待有效报价' : '开始生成')} onClick={onGenerate}>{busy ? <LoaderCircle size={16} className="canvas-media-v2__spinner" /> : <ArrowUp size={16} />}{busy ? canvasMediaRunLabel(run) || '生成中' : '生成'}</button>
    </footer>
    {errors.length ? <p className="canvas-console-v2__hint">{errors[0]}</p> : null}
  </section>

  return <><div className={`canvas-console-v2-anchor ${className}`} style={style} ref={inlineHost} />{expanded ? <Modal title={video ? '视频创作' : '图片创作'} className="canvas-console-v2-modal" onClose={() => setExpanded(false)}><div ref={modalHost} /></Modal> : null}{createPortal(content, editorHost)}</>
}
