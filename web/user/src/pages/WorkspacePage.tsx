import { ChangeEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import type { Capability, CapabilityModelGroup, EstimateResult, ImageResult, ImageTask, ImageTaskStatus, ImageTaskType, ReferenceAsset } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { toTask, userApi } from '../../../shared/user-api'
import { EmptyState, ImageLightbox, LoadingState, PublicDetailIcon, copyText, publicDetailButton, useApp } from '../components'
import { userButton, userForm, userPill, userState } from '../ui/classes'
import { errorMessage } from '../useApiResource'
import { galleryEditContextKey, parseGalleryEditContext } from './galleryEditContext'
import { displayPoints, publicUnavailableReason, workspaceGenerateReadiness } from './workspaceGenerateReadiness'
import { workspaceUnavailableImageActionNotice, type WorkspaceImageAction } from './workspaceImageActions'
import { workspaceTaskCardView, workspaceTaskFailureView, workspaceTaskPendingView } from './workspaceTaskFailure'

type WorkspaceMode = 'reference' | 'text'
type RestoreParameters = { routeModelCode?: string; quality?: string; aspectRatio?: string }

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

function formatFileSize(bytes?: number) {
  if (!bytes || bytes <= 0) return ''
  const mb = bytes / (1024 * 1024)
  if (mb >= 1) return `${mb.toFixed(mb >= 10 ? 0 : 1)} MB`
  const kb = bytes / 1024
  return `${Math.max(1, Math.round(kb))} KB`
}

function referenceUploadMaxBytes(capability: Capability | null) {
  const bytes = Number(capability?.reference_image_max_bytes ?? 0)
  if (bytes > 0) return bytes
  const mb = Number(capability?.reference_image_max_mb ?? 0)
  return mb > 0 ? mb * 1024 * 1024 : 0
}

function referenceAssetPreviewURL(asset: ReferenceAsset, accessToken?: string | null) {
  const raw = asset.preview_url || asset.download_url || ''
  return raw ? userApi.imageAssetUrl(raw, accessToken) : ''
}

function uploadTooLargeMessage(file: File, maxBytes: number) {
  return `单张参考图最大 ${formatFileSize(maxBytes)}，当前文件 ${formatFileSize(file.size)}。`
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

const workspaceClasses = {
  root: 'grid h-[calc(100vh_-_var(--topbar-h))] grid-cols-[380px_minmax(0,1fr)] max-[1024px]:grid-cols-[340px_minmax(0,1fr)] max-[760px]:block max-[760px]:min-h-0',
  panel: 'flex flex-col overflow-y-auto border-r border-[var(--border)] bg-[var(--surface)] max-[760px]:border-b max-[760px]:border-r-0',
  panelSection: 'border-b border-[var(--border)] p-6',
  panelSectionFinal: 'mt-auto border-b-0 p-6',
  tabs: 'grid grid-cols-2 gap-2 mb-6',
  tab: 'cursor-pointer rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_78%,transparent)] px-3 py-2.5 text-center text-[var(--muted)]',
  tabActive: 'border-[var(--accent)] bg-[var(--accent)] font-extrabold text-[var(--bg)]',
  panelTitle: 'm-0 mb-1.5 font-[var(--font-display)] text-[34px] leading-tight',
  panelCopy: 'm-0 mb-4 text-sm text-[var(--muted)]',
  uploadStrip: 'mt-3.5 grid grid-cols-2 gap-2.5',
  refThumb: 'grid min-h-[76px] cursor-pointer place-items-center overflow-hidden rounded-[10px] border border-dashed border-[var(--border)] text-[var(--muted)]',
  hiddenInput: 'hidden',
  refGrid: 'mt-3 grid grid-cols-3 gap-2',
  refTile: 'relative aspect-square overflow-hidden rounded-lg border border-[var(--border)] bg-[#05070d] group',
  refImage: 'size-full object-cover',
  refPlaceholder: 'grid size-full place-items-center px-2 text-center text-[11px] leading-snug text-[var(--muted)]',
  refRemove: 'absolute bottom-1.5 right-1.5 min-h-[26px] translate-y-[3px] rounded-md border border-white/20 bg-[#05070dc2] px-2 text-xs text-[var(--fg)] opacity-0 backdrop-blur transition group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100',
  editSourcePanel: 'my-5 rounded-lg border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_86%,#05070d)] p-3',
  fieldLabel: 'mb-2 block text-[13px] font-semibold text-[var(--muted)]',
  uploadHint: 'mt-2 text-[12px] leading-snug text-[var(--muted)]',
  editUploadRow: 'mt-2.5 flex items-center gap-2',
  editUploadButton: 'inline-flex min-h-9 cursor-pointer items-center justify-center rounded-lg border border-dashed border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_10%,transparent)] px-3 text-[13px] font-bold text-[var(--accent)]',
  fieldBlock: 'mb-5',
  promptBlock: 'mt-0',
  promptBlockReference: 'mt-6',
  details: 'mt-3',
  summary: 'cursor-pointer text-xs text-[var(--muted)]',
  negativeArea: 'mt-2',
  selectGrid: 'grid grid-cols-2 gap-2.5',
  selectGridThree: 'grid grid-cols-3 gap-2.5',
  selectItem: 'cursor-pointer rounded-lg border border-[var(--border)] bg-[var(--bg)] p-2.5 text-center font-mono text-sm text-[var(--fg)]',
  selectItemActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_11%,transparent)] text-[var(--accent)]',
  modelButton: 'flex w-full items-center justify-between gap-3 text-left mb-2',
  modelMeta: 'num text-xs',
  estimateRow: 'mb-3 flex items-center justify-between text-[13px] text-[var(--muted)]',
  estimateValue: 'num font-bold text-[var(--accent)]',
  formError: 'mb-3 text-[13px] text-[oklch(76%_.14_35)]',
  formActions: 'mt-2.5 flex flex-wrap gap-2 max-[420px]:flex-col max-[420px]:items-stretch',
  generateHint: 'mb-3 rounded-lg border border-[color-mix(in_oklch,var(--accent)_32%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_9%,transparent)] p-3 text-sm text-[var(--accent)]',
  createButton: cn(userButton.base, userButton.primary, 'w-full min-h-[54px] rounded-[14px] text-lg'),
  canvas: 'relative flex min-w-0 flex-col gap-4 overflow-y-auto p-7 pb-24 max-[760px]:p-[18px]',
  feed: 'grid w-full max-w-[980px] gap-6',
  placeholder: 'grid min-h-[420px] flex-1 place-items-center rounded-3xl border border-[var(--border)] bg-[radial-gradient(circle_at_35%_36%,rgba(212,157,94,.22),transparent_18%),radial-gradient(circle_at_70%_30%,rgba(131,118,255,.18),transparent_24%),#0d1320] p-7 text-center text-[var(--muted)]',
  placeholderTitle: 'm-0 mb-2 font-[var(--font-display)] text-[42px] leading-tight text-[var(--fg)]',
  placeholderText: 'm-0',
  floatingFeedback: 'absolute bottom-11 right-11 flex gap-2.5 rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_88%,transparent)] p-2.5 backdrop-blur-2xl max-[760px]:static max-[760px]:ml-auto max-[760px]:w-fit',
  feedbackButton: cn(userButton.base, 'size-11 rounded-full p-0'),
  record: 'grid gap-3.5 border-b border-[var(--border)] pb-6',
  recordHead: 'grid grid-cols-[38px_minmax(0,1fr)] items-start gap-3.5 max-[760px]:grid-cols-1',
  recordAvatar: 'grid size-[38px] place-items-center rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--accent)_18%,var(--surface))] font-mono text-[11px] font-bold text-[var(--accent)]',
  recordTitle: 'mb-1.5 flex flex-wrap items-center gap-2.5',
  recordTitleText: 'text-[15px]',
  recordDate: 'text-xs text-[var(--muted)]',
  recordPrompt: 'm-0 max-w-[78ch] leading-relaxed text-[var(--muted)] [overflow-wrap:anywhere]',
  recordParams: 'mt-2 flex flex-wrap gap-2.5 font-mono text-xs text-[var(--muted)]',
  recordParam: 'rounded-full bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2 py-1',
  sourceImages: 'ml-[52px] flex flex-wrap items-center gap-2 text-xs text-[var(--muted)] max-[760px]:ml-0',
  sourceImagesTitle: 'font-bold text-[var(--fg)]',
  sourceImageButton: 'h-[54px] w-[72px] cursor-zoom-in overflow-hidden rounded-lg border border-[var(--border)] bg-[#05070d] p-0',
  recordImages: 'grid max-w-[1040px] grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3.5 pl-[52px] max-[760px]:pl-0',
  pending: 'ml-[52px] grid min-h-[260px] place-items-center content-center gap-2 rounded-lg border border-dashed border-[var(--border)] bg-[#05070d] text-center text-[var(--muted)] max-[760px]:ml-0',
  pendingStrong: 'text-[var(--fg)]',
  pendingFailed: 'border-[color-mix(in_oklch,var(--accent-coral)_50%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_9%,#05070d)]',
  pendingFailedTitle: 'text-[oklch(78%_.14_35)]',
  failureMeta: 'mt-1.5 flex flex-wrap justify-center gap-2',
  failureMetaItem: 'inline-flex max-w-full items-center gap-1.5 rounded-lg border border-[color-mix(in_oklch,var(--accent-coral)_32%,var(--border))] bg-[color-mix(in_oklch,var(--fg)_6%,transparent)] px-2 py-1 text-[11px]',
  failureMetaLabel: 'text-[var(--muted)]',
  failureMetaValue: 'm-0 font-mono text-[var(--fg)] [overflow-wrap:anywhere]',
  recordActions: 'flex flex-wrap gap-2 pl-[52px] max-[760px]:pl-0 [&_.pg-public-detail-action]:size-[34px] [&_.pg-public-detail-action]:min-h-[34px] [&_.pg-public-detail-action]:rounded-lg',
  generatedFigure: 'group relative m-0 overflow-hidden rounded-lg border border-[var(--border)] bg-[#05070d]',
  generatedPreview: 'block w-full cursor-zoom-in border-0 bg-transparent p-0 [aspect-ratio:var(--generated-ratio)]',
  generatedImage: 'block size-full object-contain transition duration-200 group-hover:scale-[1.015]',
  generatedCaption: 'absolute bottom-2.5 left-2.5 right-2.5 flex translate-y-1 justify-end gap-1.5 opacity-0 transition group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 max-[760px]:static max-[760px]:translate-y-0 max-[760px]:flex-wrap max-[760px]:justify-start max-[760px]:bg-[#05070d] max-[760px]:p-2 max-[760px]:opacity-100',
  generatedAction: 'min-h-[30px] rounded-md border border-white/20 bg-[#05070dc7] px-2.5 text-xs text-[var(--fg)] backdrop-blur',
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
  const skipNextModeResetRef = useRef(false)
  const restoreParametersRef = useRef<RestoreParameters | null>(null)
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
    const raw = window.sessionStorage.getItem(galleryEditContextKey)
    if (!raw) return
    window.sessionStorage.removeItem(galleryEditContextKey)
    const storedRaw = raw
    let cancelled = false
    async function restoreEditContext() {
      try {
        const context = parseGalleryEditContext(storedRaw)
        if (!context) throw new Error('图片上下文读取失败，请从图库重新进入。')
        skipNextModeResetRef.current = true
        restoreParametersRef.current = {
          routeModelCode: context.route_model_code,
          quality: context.quality,
          aspectRatio: context.aspect_ratio,
        }
        setMode(context.task_type === 'reference_to_image' ? 'reference' : 'text')
        setPrompt(context.prompt)
        setNegative('')
        const sources = (context.sources ?? []).filter((item) => item.id || item.preview_url)
        if (sources.length) {
          if (context.task_type === 'reference_to_image') {
            setRefs((items) => [...sources, ...items])
          } else {
            setEditRefs(sources)
          }
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
    if (skipNextModeResetRef.current) {
      skipNextModeResetRef.current = false
      return
    }
    setPrompt('')
    setNegative('')
  }, [mode])

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
  const selectedModel = useMemo(() => availableModels.find((item) => item.code === model), [availableModels, model])
  const qualities = useMemo(() => qualityOptions(selectedModel), [selectedModel])
  const ratios = useMemo(() => ratioOptions(selectedModel, capability), [selectedModel, capability])
  const counts = useMemo(() => countOptions(selectedModel, capability), [selectedModel, capability])

  useEffect(() => {
    if (!capability || !selectedModel) return
    const restoreParameters = restoreParametersRef.current
    const waitingForPreferredModel = Boolean(
      restoreParameters?.routeModelCode
      && availableModels.some((item) => item.code === restoreParameters.routeModelCode)
      && selectedModel?.code !== restoreParameters.routeModelCode,
    )
    if (waitingForPreferredModel) return

    setQuality(restoreParameters?.quality && qualities.includes(restoreParameters.quality) ? restoreParameters.quality : qualities[0] ?? '')
    setRatio(restoreParameters?.aspectRatio && ratios.includes(restoreParameters.aspectRatio) ? restoreParameters.aspectRatio : ratios[0] ?? '')
    setCount(counts[0] ?? 0)
    restoreParametersRef.current = null
  }, [taskType, capability, selectedModel, availableModels, qualities, ratios, counts])

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

  const generateReadiness = workspaceGenerateReadiness({
    busy,
    hasModel: Boolean(model && selectedModel),
    unavailableReason: capability?.unavailable_reason,
    parametersReady,
    prompt,
    estimate,
  })
  const maxReferenceUploadBytes = referenceUploadMaxBytes(capability)
  const maxReferenceUploadLabel = formatFileSize(maxReferenceUploadBytes)

  async function uploadReference(event: ChangeEvent<HTMLInputElement>, target: 'edit' | 'reference') {
    const files = Array.from(event.target.files ?? [])
    if (!files.length) return
    const accepted = maxReferenceUploadBytes > 0 ? files.filter((file) => file.size <= maxReferenceUploadBytes) : files
    const rejected = maxReferenceUploadBytes > 0 ? files.filter((file) => file.size > maxReferenceUploadBytes) : []
    if (rejected.length) {
      app.notify('error', rejected.length === 1 ? uploadTooLargeMessage(rejected[0], maxReferenceUploadBytes) : `${rejected.length} 个文件超过单张最大 ${maxReferenceUploadLabel}，已跳过。`)
    }
    if (!accepted.length) {
      event.target.value = ''
      return
    }
    setBusy(true)
    try {
      const uploaded = await Promise.all(accepted.map((file) => userApi.uploadReferenceAsset(file)))
      if (target === 'edit') {
        setEditRefs((items) => [...uploaded, ...items])
      } else {
        setRefs((items) => [...uploaded, ...items])
      }
      app.notify('success', `已上传 ${uploaded.length} 张参考图`)
    } catch (err) {
      app.notify('error', uploadErrorMessage(err))
    } finally {
      event.target.value = ''
      setBusy(false)
    }
  }

  async function createTask() {
    if (generateReadiness.disabled) {
      app.notify('error', generateReadiness.reason)
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
    <div className={workspaceClasses.root}>
      {/* Left Parameter Panel */}
      <aside className={workspaceClasses.panel}>
        {/* Tabs */}
        <div className={workspaceClasses.panelSection}>
          <div className={workspaceClasses.tabs}>
            <button type="button" className={cn(workspaceClasses.tab, mode === 'text' && workspaceClasses.tabActive)} onClick={() => setMode('text')}>文生图片</button>
            <button type="button" className={cn(workspaceClasses.tab, mode === 'reference' && workspaceClasses.tabActive)} onClick={() => setMode('reference')}>参考生图</button>
          </div>

          <div>
            <h2 className={workspaceClasses.panelTitle}>{mode === 'text' ? '文生图' : '参考生图'}</h2>
          </div>
          <p className={workspaceClasses.panelCopy}>
            {mode === 'text' ? '通过文字描述直接生成图片；添加图片后会进入二次编辑模式。' : '参考多图元素，生成全新图片'}
          </p>

          {/* Reference upload area (only for reference mode) */}
          {mode === 'reference' ? (
            <>
              <div className={workspaceClasses.uploadStrip}>
                <label className={workspaceClasses.refThumb}>
                  <input className={workspaceClasses.hiddenInput} type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'reference')} />
                  <span>+ 图片</span>
                </label>
                <label className={workspaceClasses.refThumb}>
                  <input className={workspaceClasses.hiddenInput} type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'reference')} />
                  <span>+ 主体</span>
                </label>
              </div>
              {maxReferenceUploadLabel ? <p className={workspaceClasses.uploadHint}>单张参考图最大 {maxReferenceUploadLabel}</p> : null}
              {refs.length ? (
                <div className={workspaceClasses.refGrid}>
                  {refs.map((asset) => (
                    <div key={asset.id || asset.preview_url} className={workspaceClasses.refTile}>
                      <ReferenceAssetPreview asset={asset} accessToken={app.session?.token} />
                      <button type="button" className={workspaceClasses.refRemove} title="移除参考图" onClick={() => removeReferenceAsset(asset)}>移除</button>
                    </div>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          {mode === 'text' ? (
            <div className={workspaceClasses.editSourcePanel}>
              <label className={workspaceClasses.fieldLabel}>图片编辑来源</label>
              <div className={workspaceClasses.editUploadRow}>
                <label className={workspaceClasses.editUploadButton}>
                  <input className={workspaceClasses.hiddenInput} type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'edit')} />
                  <span>+ 上传原图</span>
                </label>
              </div>
              {maxReferenceUploadLabel ? <p className={workspaceClasses.uploadHint}>单张最大 {maxReferenceUploadLabel}</p> : null}
              {editRefs.length ? (
                <div className={workspaceClasses.refGrid}>
                  {editRefs.map((asset) => (
                    <div key={asset.id || asset.preview_url} className={workspaceClasses.refTile}>
                      <ReferenceAssetPreview asset={asset} accessToken={app.session?.token} />
                      <button type="button" className={workspaceClasses.refRemove} title="移除编辑图片" onClick={() => removeEditAsset(asset)}>移除</button>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* Prompt */}
          <div className={mode === 'reference' ? workspaceClasses.promptBlockReference : workspaceClasses.promptBlock}>
            <label className={workspaceClasses.fieldLabel}>提示词 (PROMPT)</label>
            <textarea
              className={userForm.textarea}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={5}
              placeholder="描述想要生成的内容..."
            />
          </div>

          {/* Negative prompt (collapsed) */}
          <details className={workspaceClasses.details}>
            <summary className={workspaceClasses.summary}>限制词 (NEGATIVE PROMPT)</summary>
            <textarea
              className={cn(userForm.textarea, workspaceClasses.negativeArea)}
              value={negative}
              onChange={(e) => setNegative(e.target.value)}
              rows={2}
            />
          </details>
        </div>

        {/* Parameters */}
        <div className={workspaceClasses.panelSection}>
          {/* Model */}
          <div className={workspaceClasses.fieldBlock}>
            <label className={workspaceClasses.fieldLabel}>模型选择</label>
            {loading && !capability ? <LoadingState label="正在加载可用模型..." /> : null}
            {!loading && availableModels.length ? availableModels.map((m) => (
              <button
                key={m.code}
                type="button"
                className={cn(workspaceClasses.selectItem, workspaceClasses.modelButton, model === m.code && workspaceClasses.selectItemActive)}
                onClick={() => setModel(m.code)}
              >
                <span>{m.name}</span>
                <span className={cn(workspaceClasses.modelMeta, model === m.code ? 'text-[var(--accent)]' : 'text-[var(--muted)]')}>{m.display_points ? `${m.display_points} ◈` : m.effective_multiplier ? `${m.effective_multiplier}x` : ''}</span>
              </button>
            )) : null}
            {!loading && !availableModels.length ? <EmptyState title="平台模型配置中" detail={publicUnavailableReason(capability?.unavailable_reason)} /> : null}
          </div>

          {selectedModel ? (
            <>
              {/* Quality */}
              {qualities.length ? (
                <div className={workspaceClasses.fieldBlock}>
                  <label className={workspaceClasses.fieldLabel}>清晰度 (QUALITY)</label>
                  <div className={workspaceClasses.selectGrid}>
                    {qualities.map((q) => (
                      <button
                        key={q}
                        type="button"
                        className={cn(workspaceClasses.selectItem, quality === q && workspaceClasses.selectItemActive)}
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
                <div className={workspaceClasses.fieldBlock}>
                  <label className={workspaceClasses.fieldLabel}>比例 (ASPECT RATIO)</label>
                  <div className={workspaceClasses.selectGridThree}>
                    {ratios.map((r) => (
                      <button
                        key={r}
                        type="button"
                        className={cn(workspaceClasses.selectItem, ratio === r && workspaceClasses.selectItemActive)}
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
                  <label className={workspaceClasses.fieldLabel}>图片数量</label>
                  <div className={workspaceClasses.selectGrid}>
                    {counts.map((n) => (
                      <button
                        key={n}
                        type="button"
                        className={cn(workspaceClasses.selectItem, count === n && workspaceClasses.selectItemActive)}
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
        <div className={workspaceClasses.panelSectionFinal}>
          <div className={workspaceClasses.estimateRow}>
            <span>预估消耗</span>
            <span className={workspaceClasses.estimateValue}>{estimate?.display_points ?? estimate?.points ?? '...'} ◈</span>
          </div>
          {estimate && !estimate.sufficient ? (
            <div className={workspaceClasses.formError}>
              <div>
                积分不足，还差 {displayPoints(estimate.insufficient_points)} 积分。
                当前可用 {displayPoints(estimate.balance?.available_points)} 积分。
              </div>
              <div className={workspaceClasses.formActions}>
                <button className={cn(userButton.base, userButton.primary)} type="button" onClick={() => app.navigate('checkout')}>去充值</button>
                <button className={cn(userButton.base, userButton.ghost)} type="button" onClick={() => app.navigate('profile')}>兑换积分</button>
              </div>
            </div>
          ) : null}
          {generateReadiness.reason && !generateReadiness.showRechargeAction ? (
            <div className={workspaceClasses.generateHint}>
              {generateReadiness.reason}
            </div>
          ) : null}
          <button
            className={workspaceClasses.createButton}
            type="button"
            disabled={generateReadiness.disabled}
            onClick={() => void createTask()}
          >
            {busy ? (
              <>
                <span className={userState.spinner} />
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
      <section className={workspaceClasses.canvas}>
        {records.length ? (
          <div className={workspaceClasses.feed}>
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
                onRetryTask={async (task) => {
                  setBusy(true)
                  try {
                    const retry = await userApi.retryTask(task.id)
                    setRecords((items) => mergeGenerationRecord(items, retry))
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
                    app.notify('success', '失败记录已删除')
                  } catch (err) {
                    app.notify('error', errorMessage(err))
                  }
                }}
                accessToken={app.session?.token}
                onUnavailable={(action) => {
                  const notice = workspaceUnavailableImageActionNotice(action)
                  app.notify('info', `${notice.title}：${notice.detail}`)
                }}
              />
            ))}
            <div ref={feedEndRef} />
          </div>
        ) : (
          <div className={workspaceClasses.placeholder}>
            <div>
              <h2 className={workspaceClasses.placeholderTitle}>开始创作您的第一个作品吧！</h2>
              <p className={workspaceClasses.placeholderText}>在左侧面板设置参数并点击"开始创作"</p>
            </div>
          </div>
        )}

        {/* Floating Feedback */}
        <div className={workspaceClasses.floatingFeedback}>
          <button type="button" className={workspaceClasses.feedbackButton} title="确认完成">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 6L9 17l-5-5" /></svg>
          </button>
        </div>

        <ImageLightbox image={previewImage} onClose={() => setPreviewImage(null)} />

      </section>
    </div>
  )
}

function ReferenceAssetPreview({ asset, accessToken, onClick }: { asset: ReferenceAsset; accessToken?: string | null; onClick?: (url: string) => void }) {
  const previewURL = referenceAssetPreviewURL(asset, accessToken)
  if (!previewURL) {
    return <div className={workspaceClasses.refPlaceholder}>无法预览</div>
  }
  if (onClick) {
    return (
      <button className={workspaceClasses.sourceImageButton} type="button" onClick={() => onClick(previewURL)}>
        <img className={workspaceClasses.refImage} src={previewURL} alt={asset.name || '参考图'} />
      </button>
    )
  }
  return <img src={previewURL} alt={asset.name || '参考图'} className={workspaceClasses.refImage} />
}

function GenerationRecord({ task, onCopyPrompt, onUseReference, onPublishImage, onOpenGallery, onPreviewImage, onRetryTask, onDeleteTask, accessToken, onUnavailable }: {
  task: ImageTask
  onCopyPrompt: () => Promise<void>
  onUseReference: (url: string) => Promise<void>
  onPublishImage: (image: ImageResult) => Promise<void>
  onOpenGallery: () => void
  onPreviewImage: (url: string, alt: string) => void
  onRetryTask: (task: ImageTask) => Promise<void>
  onDeleteTask: (task: ImageTask) => Promise<void>
  accessToken?: string
  onUnavailable: (action: WorkspaceImageAction) => void
}) {
  const card = workspaceTaskCardView(task)
  const pending = workspaceTaskPendingView(task)
  return (
    <article className={workspaceClasses.record}>
      <header className={workspaceClasses.recordHead}>
        <div className={workspaceClasses.recordAvatar} aria-hidden="true">PG</div>
        <div>
          <div className={workspaceClasses.recordTitle}>
            <b className={workspaceClasses.recordTitleText}>{card.taskTypeLabel}</b>
            <span className={workspaceClasses.recordDate}>{card.createdAtLabel}</span>
            <span className={cn(userPill.base, userPill[card.statusTone] ?? userPill.neutral)}>{card.statusLabel}</span>
          </div>
          <p className={workspaceClasses.recordPrompt}>{task.prompt}</p>
          <div className={workspaceClasses.recordParams}>
            <span className={workspaceClasses.recordParam}>模型 {task.model_group}</span>
            <span className={workspaceClasses.recordParam}>清晰度 {task.quality}</span>
            <span className={workspaceClasses.recordParam}>比例 {task.aspect_ratio}</span>
            <span className={workspaceClasses.recordParam}>数量 {task.image_count}</span>
            <span className={workspaceClasses.recordParam}>本次消耗 {displayTaskPoints(task)} 积分</span>
          </div>
        </div>
      </header>

      {task.results.length ? (
        <>
          {task.task_type === 'image_edit' && task.reference_assets.length ? (
            <div className={workspaceClasses.sourceImages}>
              <span className={workspaceClasses.sourceImagesTitle}>原图引用</span>
              {task.reference_assets.map((asset) => (
                <ReferenceAssetPreview
                  key={asset.id || asset.preview_url || asset.download_url}
                  asset={asset}
                  accessToken={accessToken}
                  onClick={(url) => onPreviewImage(url, asset.name || '原图引用')}
                />
              ))}
            </div>
          ) : null}
          <div className={workspaceClasses.recordImages}>
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
        <TaskFailureBlock task={task} onRetry={() => onRetryTask(task)} onDelete={() => onDeleteTask(task)} />
      ) : (
        <div className={workspaceClasses.pending}>
          <span className={userState.spinner} />
          <strong className={workspaceClasses.pendingStrong}>{pending.title}</strong>
          <p className={workspaceClasses.placeholderText}>{pending.detail}</p>
        </div>
      )}

      <footer className={workspaceClasses.recordActions}>
        {publicDetailButton('复制提示词', <PublicDetailIcon name="copy" />, () => void onCopyPrompt())}
        {publicDetailButton('查看图库', <PublicDetailIcon name="group" />, onOpenGallery)}
      </footer>
    </article>
  )
}

function TaskFailureBlock({ task, onRetry, onDelete }: { task: ImageTask; onRetry: () => Promise<void>; onDelete: () => Promise<void> }) {
  const view = workspaceTaskFailureView(task)
  return (
    <div className={cn(workspaceClasses.pending, workspaceClasses.pendingFailed)}>
      <strong className={workspaceClasses.pendingFailedTitle}>{view.title}</strong>
      <p className={workspaceClasses.placeholderText}>{view.reason}</p>
      {view.meta.length ? (
        <dl className={workspaceClasses.failureMeta}>
          {view.meta.map((item) => (
            <div className={workspaceClasses.failureMetaItem} key={item.label}>
              <dt className={workspaceClasses.failureMetaLabel}>{item.label}</dt>
              <dd className={workspaceClasses.failureMetaValue}>{item.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      <div className="mt-3 flex flex-wrap justify-center gap-2">
        <button className={cn(userButton.base, userButton.primary, 'min-h-9 rounded-lg px-3 text-sm')} type="button" onClick={() => void onRetry()}>重试</button>
        <button className={cn(userButton.base, userButton.ghost, 'min-h-9 rounded-lg px-3 text-sm')} type="button" onClick={() => void onDelete()}>删除</button>
      </div>
    </div>
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
  onUnavailable: (action: WorkspaceImageAction) => void
}) {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  const downloadUrl = userApi.imageAssetUrl(image.download_url ?? image.url, accessToken)
  const aspectRatio = image.width && image.height ? `${image.width} / ${image.height}` : normalizeAspectRatio(fallbackRatio)
  const ratioStyle = aspectRatio ? { '--generated-ratio': aspectRatio } as CSSProperties : undefined
  return (
    <figure className={workspaceClasses.generatedFigure} style={ratioStyle}>
      <button type="button" className={workspaceClasses.generatedPreview} onClick={() => onPreview(imageUrl, alt)} aria-label="预览生成图片">
        <img className={workspaceClasses.generatedImage} src={imageUrl} alt={alt} />
      </button>
      <figcaption className={workspaceClasses.generatedCaption}>
        <button className={workspaceClasses.generatedAction} type="button" title="编辑" onClick={() => void onUseReference(imageUrl)}>编辑</button>
        <button className={workspaceClasses.generatedAction} type="button" title="下载" onClick={() => window.open(downloadUrl, '_blank', 'noopener,noreferrer')}>下载</button>
        <button className={workspaceClasses.generatedAction} type="button" title="发布" onClick={() => void onPublish(image)}>发布</button>
        <button className={workspaceClasses.generatedAction} type="button" title="标记" onClick={() => onUnavailable('标记')}>标记</button>
        <button className={workspaceClasses.generatedAction} type="button" title="更多" onClick={() => onUnavailable('更多')}>更多</button>
      </figcaption>
    </figure>
  )
}
