import { ChangeEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import type { Capability, CapabilityModelGroup, EstimateResult, GalleryImage, ImageResult, ImageTask, ImageTaskStatus, ImageTaskType, ReferenceAsset } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { toTask, userApi } from '../../../shared/user-api'
import { Button, EmptyState, ImageLightbox, LoadingState, Modal, copyText, useApp, type ImageLightboxPayload } from '../components'
import { userButton, userForm, userState } from '../ui/classes'
import { rdWorkspace } from '../ui/redesign-classes'
import { errorMessage } from '../useApiResource'
import { galleryEditContextKey, parseGalleryEditContext } from './galleryEditContext'
import { displayPoints, publicUnavailableReason, workspaceGenerateReadiness } from './workspaceGenerateReadiness'
import { defaultGalleryImportFilter, filterGalleryImportImages, galleryImportOptions, mergeReferenceAssets, type GalleryImportFilter } from './workspaceGalleryImport'
import { workspaceTaskFailureView } from './workspaceTaskFailure'
import { generationSlots, workspaceQualityLabel } from './workspaceTaskProgress'

type WorkspaceMode = 'reference' | 'text'
type OutputTab = 'current' | 'history'
type RestoreParameters = { routeModelCode?: string; quality?: string; aspectRatio?: string }
type UploadTarget = 'edit' | 'reference'
type DragUploadState = Record<UploadTarget, boolean>

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
  root: rdWorkspace.root,
  panel: rdWorkspace.sidebar,
  panelScroll: rdWorkspace.sidebarScroll,
  panelSection: rdWorkspace.sidebarSection,
  panelSectionFinal: rdWorkspace.actionBar,
  tabs: 'mb-6 grid grid-cols-2 gap-3',
  tab: cn(rdWorkspace.selectItem, 'min-h-11 flex-row text-sm'),
  tabActive: 'border-[var(--accent)] bg-[var(--accent)] text-white shadow-[0_0_20px_rgba(var(--accent-rgb),0.18)] ring-1 ring-[var(--accent)]/50 [&_*]:text-white',
  panelTitle: 'm-0 mb-1.5 text-3xl font-black leading-tight text-[var(--fg)]',
  panelCopy: 'm-0 mb-4 text-sm leading-relaxed text-[var(--muted)]',
  uploadStrip: 'mt-3 grid grid-cols-2 gap-2',
  refThumb: cn(rdWorkspace.uploadBox, 'mt-0 flex h-16 items-center justify-center gap-2 px-3 py-2 text-center text-[11px] font-bold leading-tight text-[var(--muted)] disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:size-4'),
  refThumbDrag: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_10%,transparent)] text-[var(--accent)] shadow-[0_0_0_1px_color-mix(in_oklch,var(--accent)_34%,transparent)]',
  refThumbUpload: 'border-dashed',
  refThumbImport: 'border-dashed',
  hiddenInput: 'hidden',
  refGrid: rdWorkspace.uploadGrid,
  refTile: rdWorkspace.uploadThumb,
  refImage: rdWorkspace.uploadImg,
  refPlaceholder: 'grid size-full place-items-center px-2 text-center text-[11px] leading-snug text-[var(--muted)]',
  refRemove: 'absolute right-1 top-1 grid size-6 place-items-center rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] text-[var(--image-action-text)] opacity-0 backdrop-blur transition hover:bg-[var(--accent-coral)] hover:text-[var(--fg)] group-hover:opacity-100',
  editSourcePanel: 'my-5 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50',
  editSourceTrigger: 'flex w-full cursor-pointer items-center justify-between gap-3 border-0 bg-transparent px-4 py-3 text-left text-[var(--fg)] transition hover:text-[var(--accent)]',
  editSourceTitle: 'flex items-center gap-2 text-[11px] font-bold uppercase tracking-wider',
  editSourceChevron: 'grid size-6 shrink-0 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] transition-transform',
  editSourceBody: 'overflow-hidden px-4 transition-[max-height,padding] duration-300',
  editSourceBodyOpen: 'max-h-[360px] pb-4',
  editSourceBodyClosed: 'max-h-0 pb-0',
  fieldLabel: rdWorkspace.sectionTitle,
  uploadHint: 'mt-2 text-[12px] leading-snug text-[var(--muted)]',
  editUploadRow: 'mt-2.5 flex items-center gap-2',
  editUploadButton: 'inline-flex min-h-9 cursor-pointer items-center justify-center rounded-xl border border-dashed border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_10%,transparent)] px-3 text-[13px] font-bold text-[var(--accent)]',
  importButton: 'inline-flex min-h-9 cursor-pointer items-center justify-center rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3 text-[13px] font-bold text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50',
  fieldBlock: 'mb-5',
  promptBlock: 'mt-0',
  promptBlockReference: 'mt-6',
  details: 'mt-3',
  summary: 'cursor-pointer text-xs text-[var(--muted)]',
  negativeArea: 'mt-2',
  selectGrid: rdWorkspace.grid4,
  selectGridThree: rdWorkspace.grid3,
  selectItem: cn(rdWorkspace.selectItem, 'min-h-14'),
  selectItemActive: 'border-[var(--accent)] bg-[var(--accent)] text-white shadow-[0_0_20px_rgba(var(--accent-rgb),0.18)] ring-1 ring-[var(--accent)]/50 hover:bg-[var(--accent)] [&_*]:text-white',
  modelButton: cn(rdWorkspace.modelItem, 'mb-2 text-left'),
  modelButtonActive: 'border-[var(--accent)] bg-[var(--accent)] text-white ring-1 ring-[var(--accent)]/50 shadow-[0_0_20px_rgba(var(--accent-rgb),0.18)] [&_*]:text-white hover:bg-[var(--accent)]',
  modelMeta: cn('num text-xs', rdWorkspace.modelPoints),
  modelMetaActive: 'bg-white/18 text-white',
  countInputWrap: 'flex items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 p-2',
  countStepper: 'grid size-10 shrink-0 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-lg font-black text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-40',
  countInput: 'h-10 min-w-0 flex-1 rounded-xl border border-transparent bg-transparent px-3 text-center font-vault-mono text-base font-black text-[var(--fg)] focus:border-transparent focus:shadow-none',
  countHint: 'mt-2 text-[11px] text-[var(--muted)]',
  estimateRow: rdWorkspace.priceRow,
  estimateValue: cn('num', rdWorkspace.priceValue),
  formError: 'mb-3 text-[13px] text-[var(--accent-coral)]',
  formActions: 'mt-2.5 flex flex-wrap gap-2 max-[420px]:flex-col max-[420px]:items-stretch',
  generateHint: 'mb-3 rounded-xl border border-[color-mix(in_oklch,var(--accent)_32%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_9%,transparent)] p-3 text-sm text-[var(--accent)]',
  createButton: rdWorkspace.generateBtn,
  canvas: rdWorkspace.canvas,
  feed: cn(rdWorkspace.outputPanel, 'gap-5 overflow-y-auto overflow-x-hidden justify-start min-h-0'),
  outputTabs: 'sticky top-0 z-20 mx-auto flex w-full max-w-5xl shrink-0 rounded-2xl border border-[var(--border)] bg-[var(--canvas-bg)]/92 p-1.5 backdrop-blur',
  outputTab: 'flex-1 rounded-xl px-4 py-2 text-sm font-bold text-[var(--muted)] transition hover:bg-[var(--accent)] hover:text-white',
  outputTabActive: 'bg-[var(--accent)] text-white shadow-[0_8px_24px_rgba(var(--accent-rgb),0.18)]',
  placeholder: cn(rdWorkspace.outputPanel, 'items-center justify-center text-center'),
  placeholderTitle: 'm-0 mb-2 text-3xl font-black leading-tight text-[var(--fg)]',
  placeholderText: 'm-0',
  readyIcon: 'mx-auto grid size-14 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--accent)]/10 text-[var(--accent)]',
  record: 'flex min-h-0 w-full flex-1 flex-col justify-start gap-4',
  recordHead: 'mx-auto grid w-full max-w-5xl grid-cols-[38px_minmax(0,1fr)] items-start gap-3.5 rounded-2xl border border-[var(--border)] bg-[var(--surface)]/55 p-4 max-[760px]:grid-cols-1',
  recordAvatar: 'grid size-[38px] place-items-center rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--accent)_18%,var(--surface))] font-mono text-[11px] font-bold text-[var(--accent)]',
  recordTitle: 'mb-1.5 flex flex-wrap items-center gap-2.5',
  recordTitleText: 'text-[15px]',
  recordDate: 'text-xs text-[var(--muted)]',
  recordPrompt: 'm-0 max-w-[78ch] leading-relaxed text-[var(--muted)] [overflow-wrap:anywhere]',
  recordParams: 'mt-2 flex flex-wrap gap-2.5 font-mono text-xs text-[var(--muted)]',
  recordParam: 'rounded-full bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2 py-1',
  sourceImages: 'mx-auto flex w-full max-w-5xl flex-wrap items-center gap-2 text-xs text-[var(--muted)]',
  sourceImagesTitle: 'font-bold text-[var(--fg)]',
  sourceImageButton: 'h-[54px] w-[72px] cursor-zoom-in overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg)] p-0',
  recordImages: 'mx-auto grid w-full max-w-5xl grid-cols-[repeat(auto-fit,minmax(220px,1fr))] content-start gap-4',
  pending: 'mx-auto grid min-h-[360px] w-full max-w-2xl place-items-center content-center gap-2 rounded-3xl border border-dashed border-[var(--border)] bg-[var(--bg)]/55 text-center text-[var(--muted)]',
  pendingStrong: 'text-[var(--fg)]',
  pendingFailed: 'border-[color-mix(in_oklch,var(--accent-coral)_62%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_16%,var(--surface))]',
  pendingFailedTitle: 'text-[var(--accent-coral)]',
  failureMeta: 'mt-1.5 flex flex-wrap justify-center gap-2',
  failureMetaItem: 'inline-flex max-w-full items-center gap-1.5 rounded-xl border border-[color-mix(in_oklch,var(--accent-coral)_50%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_14%,var(--surface))] px-2 py-1 text-[11px]',
  failureMetaLabel: 'text-[var(--muted)]',
  failureMetaValue: 'm-0 font-mono text-[var(--fg)] [overflow-wrap:anywhere]',
  recordActions: 'mx-auto flex w-full max-w-5xl flex-wrap justify-center gap-2 [&_.pg-public-detail-action]:size-[34px] [&_.pg-public-detail-action]:min-h-[34px] [&_.pg-public-detail-action]:rounded-xl',
  generatedFigure: 'group relative m-0 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--bg)] shadow-xl',
  generatedPreview: 'block w-full cursor-zoom-in border-0 bg-transparent p-0 [aspect-ratio:var(--generated-ratio)] max-h-[calc(100vh-430px)]',
  generatedImage: 'block size-full max-h-[calc(100vh-430px)] object-contain transition duration-500 group-hover:scale-[1.04]',
  generatedCaption: 'absolute right-3 top-3 z-10 flex translate-y-1 justify-end gap-1.5 rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] p-1 opacity-0 shadow-2xl backdrop-blur-2xl transition group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100',
  generatedAction: rdWorkspace.outputBtn,
  generatedIconAction: 'grid size-8 place-items-center rounded-xl text-[var(--image-action-text)] transition hover:bg-[var(--image-action-hover-bg)] hover:text-[var(--image-action-hover-text)] [&_svg]:size-4',
  outputActions: 'absolute left-1/2 top-1/2 z-20 flex -translate-x-1/2 translate-y-8 items-center gap-1.5 rounded-2xl border border-[var(--border)] bg-[var(--surface)]/92 px-2 py-1.5 opacity-0 shadow-2xl backdrop-blur-2xl transition duration-300 group-hover:translate-y-0 group-hover:opacity-100',
  outputProgressFoot: 'mt-2 flex w-full items-center justify-between text-[10px] text-[var(--muted)]',
  outputResultWrap: 'relative flex min-h-0 w-full flex-1 flex-col items-center justify-start gap-3 pb-2 pt-2 animate-in fade-in zoom-in-95 duration-500 group',
  outputResultGrid: 'grid w-full grid-cols-[repeat(auto-fit,minmax(200px,1fr))] content-start gap-3',
  outputMetaRow: 'mt-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-3 border-t border-[var(--border)]/30 pt-4 text-[10px] font-vault-mono text-[var(--muted)]',
  slotState: 'grid min-h-[260px] place-items-center content-center gap-2 rounded-2xl border border-dashed border-[var(--border)] bg-[var(--bg)]/70 p-4 text-center text-[var(--muted)]',
  slotSkeleton: 'grid min-h-[260px] place-items-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--bg)]/70 p-4',
  slotSkeletonFrame: 'relative h-full min-h-[220px] w-full overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)]/80',
  slotSkeletonGlow: 'absolute inset-0 -translate-x-full bg-[linear-gradient(90deg,transparent_0%,rgba(var(--accent-rgb),0.16)_48%,transparent_100%)] animate-[shimmer_1.8s_linear_infinite]',
  slotSkeletonLoader: 'absolute inset-0 grid place-items-center text-[var(--muted)]',
  slotFailed: 'border-[color-mix(in_oklch,var(--accent-coral)_60%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_16%,var(--surface))]',
  slotCode: 'max-w-full overflow-hidden text-ellipsis whitespace-nowrap rounded-full border border-[color-mix(in_oklch,var(--accent-coral)_50%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_12%,var(--surface))] px-2 py-1 font-mono text-[11px] text-[var(--fg)]',
  importGrid: 'mt-5 grid max-h-[54vh] grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-3 overflow-auto pr-1',
  importFilters: 'mt-5 grid grid-cols-[minmax(160px,1fr)_repeat(4,minmax(120px,auto))] gap-2 max-[860px]:grid-cols-2 max-[520px]:grid-cols-1',
  importInput: 'min-h-10 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3 text-sm text-[var(--fg)]',
  importTile: 'relative cursor-pointer overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface)] text-left transition hover:border-[var(--accent)]',
  importTileActive: 'border-[var(--accent)] ring-1 ring-[var(--accent)]',
  importThumb: 'aspect-square w-full bg-[var(--bg)] object-cover',
  importCheck: 'absolute left-2 top-2 grid size-7 place-items-center rounded-xl border border-[var(--image-checkbox-border)] bg-[var(--image-checkbox-bg)] text-sm font-black text-[var(--accent)]',
  importInfo: 'grid gap-1 p-2 text-xs text-[var(--muted)]',
  importTitle: 'overflow-hidden text-ellipsis whitespace-nowrap text-[var(--fg)]',
  importActions: 'sticky bottom-0 mt-5 flex flex-wrap items-center justify-end gap-2 border-t border-[var(--border)] bg-[var(--surface)] pt-4',
  historyGrid: 'grid min-h-0 w-full grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-4 overflow-y-auto pr-1',
  historyCard: 'group relative flex min-w-0 flex-col gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)]/60 p-3 text-left transition hover:-translate-y-1 hover:border-[var(--accent)] hover:shadow-xl hover:shadow-[rgba(var(--accent-rgb),0.14)]',
  historyPreview: 'relative aspect-[4/3] overflow-visible',
  historyLayer: 'absolute inset-0 overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg)] shadow-lg',
  historyLayerBack1: 'translate-x-2 translate-y-2 rotate-2 bg-[var(--surface)]',
  historyLayerBack2: 'translate-x-4 translate-y-4 rotate-[4deg] bg-[var(--bg)]',
  historyImage: 'size-full object-cover transition duration-500 group-hover:scale-105',
  historyState: 'grid size-full place-items-center content-center gap-2 p-3 text-center text-xs text-[var(--muted)]',
  historyFailed: 'border-[color-mix(in_oklch,var(--accent-coral)_55%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_10%,var(--surface))]',
  historyWarn: 'absolute right-2 top-2 z-10 inline-flex min-h-7 items-center rounded-full border border-[color-mix(in_oklch,var(--accent-coral)_52%,transparent)] bg-[color-mix(in_oklch,var(--accent-coral)_22%,var(--canvas))] px-2 text-[10px] font-black tracking-[0.04em] text-[var(--fg)] shadow-lg',
  historyTitle: 'line-clamp-2 min-h-[2.5em] text-sm font-bold leading-snug text-[var(--fg)]',
  historyMeta: 'flex items-center justify-between gap-2 text-[10px] font-vault-mono text-[var(--muted)]',
  historyDialogGrid: 'grid max-h-[65vh] grid-cols-1 gap-3 overflow-y-auto pr-1 sm:grid-cols-2',
  historyDialogTile: 'group overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)]',
  historyDialogButton: 'block w-full border-0 bg-transparent p-0 text-left',
  historyDialogImage: 'aspect-[4/3] w-full object-cover transition duration-500 group-hover:scale-[1.03]',
  historyDialogTileMeta: 'border-t border-[var(--border)] px-3 py-2 text-xs text-[var(--muted)]',
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
  const [editSourceOpen, setEditSourceOpen] = useState(false)
  const [previewImage, setPreviewImage] = useState<ImageLightboxPayload | null>(null)
  const [outputTab, setOutputTab] = useState<OutputTab>('current')
  const [historyTaskDialog, setHistoryTaskDialog] = useState<ImageTask | null>(null)
  const [galleryImportTarget, setGalleryImportTarget] = useState<'reference' | 'edit' | null>(null)
  const [galleryImages, setGalleryImages] = useState<GalleryImage[]>([])
  const [galleryImportLoading, setGalleryImportLoading] = useState(false)
  const [galleryImportBusy, setGalleryImportBusy] = useState(false)
  const [galleryImportFilter, setGalleryImportFilter] = useState<GalleryImportFilter>(defaultGalleryImportFilter)
  const [dragUpload, setDragUpload] = useState<DragUploadState>({ edit: false, reference: false })
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
        app.notify('success', '任务已完成，结果已同步到历史资产')
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
        if (!context) throw new Error('图片上下文读取失败，请从资产重新进入。')
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
    if (editRefs.length) setEditSourceOpen(true)
  }, [editRefs.length])

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
  const maxOutputCount = counts[counts.length - 1] ?? 1

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
    setCount((current) => {
      const restored = current > 0 && counts.includes(current) ? current : counts[0] ?? 0
      return Math.max(0, Math.min(restored, counts[counts.length - 1] ?? restored))
    })
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
  const maxReferenceImages = Number(selectedModel?.max_reference_image_count ?? 5) || 5
  const referenceRemainingLimit = Math.max(0, maxReferenceImages - refs.length)
  const editRemainingLimit = Math.max(0, maxReferenceImages - editRefs.length)
  const remainingGalleryImportLimit = galleryImportTarget === 'edit' ? editRemainingLimit : referenceRemainingLimit
  const latestTask = records[records.length - 1] ?? null
  const historyTasks = useMemo(() => (
    [...records].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  ), [records])

  async function openGalleryImport(target: 'reference' | 'edit') {
    setGalleryImportTarget(target)
    setGalleryImportFilter(defaultGalleryImportFilter)
    setGalleryImportLoading(true)
    try {
      setGalleryImages(await userApi.listGalleryImages())
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setGalleryImportLoading(false)
    }
  }

  async function confirmGalleryImport(selectedIds: string[]) {
    if (!galleryImportTarget || !selectedIds.length) return
    if (selectedIds.length > remainingGalleryImportLimit) {
      app.notify('error', `最多还能选择 ${remainingGalleryImportLimit} 张参考图`)
      return
    }
    setGalleryImportBusy(true)
    try {
      const assets = await userApi.importReferenceAssetsFromGallery(selectedIds)
      if (galleryImportTarget === 'edit') {
        setEditRefs((items) => mergeReferenceAssets(items, assets, maxReferenceImages))
      } else {
        setRefs((items) => mergeReferenceAssets(items, assets, maxReferenceImages))
      }
      setGalleryImportTarget(null)
      app.notify('success', `已从资产导入 ${assets.length} 张参考图`)
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setGalleryImportBusy(false)
    }
  }

  async function uploadReferenceFiles(files: File[], target: UploadTarget) {
    if (!files.length) return
    const accepted = maxReferenceUploadBytes > 0 ? files.filter((file) => file.size <= maxReferenceUploadBytes) : files
    const rejected = maxReferenceUploadBytes > 0 ? files.filter((file) => file.size > maxReferenceUploadBytes) : []
    if (rejected.length) {
      app.notify('error', rejected.length === 1 ? uploadTooLargeMessage(rejected[0], maxReferenceUploadBytes) : `${rejected.length} 个文件超过单张最大 ${maxReferenceUploadLabel}，已跳过。`)
    }
    if (!accepted.length) {
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
        void uploadReferenceFiles(Array.from(event.dataTransfer.files ?? []).filter((file) => file.type.startsWith('image/')), target)
      },
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

  function updateImageCount(value: number) {
    if (!counts.length) return
    const next = Math.max(1, Math.min(maxOutputCount, Math.round(value) || 1))
    setCount(next)
  }

  return (
    <div className={workspaceClasses.root}>
      {/* Left Parameter Panel */}
      <aside className={workspaceClasses.panel}>
        <div className={workspaceClasses.panelScroll}>
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
                <label className={cn(workspaceClasses.refThumb, workspaceClasses.refThumbUpload, dragUpload.reference && workspaceClasses.refThumbDrag)} {...uploadDropBindings('reference')}>
                  <input className={workspaceClasses.hiddenInput} type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'reference')} />
                  <UploadGlyph />
                  <span>本地上传</span>
                </label>
                <button type="button" className={cn(workspaceClasses.refThumb, workspaceClasses.refThumbImport)} disabled={busy || referenceRemainingLimit <= 0} onClick={() => void openGalleryImport('reference')}>
                  <ImportGlyph />
                  <span>从资产导入</span>
                </button>
              </div>
              {maxReferenceUploadLabel ? <p className={workspaceClasses.uploadHint}>单张参考图最大 {maxReferenceUploadLabel}</p> : null}
              {refs.length ? (
                <div className={workspaceClasses.refGrid}>
                  {refs.map((asset) => (
                    <div key={asset.id || asset.preview_url} className={workspaceClasses.refTile}>
                      <ReferenceAssetPreview asset={asset} accessToken={app.session?.token} />
                      <button type="button" className={workspaceClasses.refRemove} title="移除参考图" onClick={() => removeReferenceAsset(asset)}><CloseGlyph /></button>
                    </div>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          {mode === 'text' ? (
            <div className={workspaceClasses.editSourcePanel}>
              <button type="button" className={workspaceClasses.editSourceTrigger} onClick={() => setEditSourceOpen((open) => !open)}>
                <span className={workspaceClasses.editSourceTitle}>
                  <ImageGlyph />
                  图片编辑来源 ({editRefs.length}/{maxReferenceImages})
                </span>
                <span className={cn(workspaceClasses.editSourceChevron, editSourceOpen && 'rotate-180')}><ChevronGlyph /></span>
              </button>
              <div className={cn(workspaceClasses.editSourceBody, editSourceOpen ? workspaceClasses.editSourceBodyOpen : workspaceClasses.editSourceBodyClosed)}>
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  <label className={cn(workspaceClasses.refThumb, workspaceClasses.refThumbUpload, dragUpload.edit && workspaceClasses.refThumbDrag)} {...uploadDropBindings('edit')}>
                    <input className={workspaceClasses.hiddenInput} type="file" accept="image/*" multiple disabled={busy} onChange={(event) => uploadReference(event, 'edit')} />
                    <UploadGlyph />
                    <span>本地上传</span>
                  </label>
                  <button type="button" className={cn(workspaceClasses.refThumb, workspaceClasses.refThumbImport)} disabled={busy || editRemainingLimit <= 0} onClick={() => void openGalleryImport('edit')}>
                    <ImportGlyph />
                    <span>从资产导入</span>
                  </button>
                </div>
                {maxReferenceUploadLabel ? <p className={workspaceClasses.uploadHint}>单张最大 {maxReferenceUploadLabel}</p> : null}
                {editRefs.length ? (
                  <div className={workspaceClasses.refGrid}>
                    {editRefs.map((asset) => (
                      <div key={asset.id || asset.preview_url} className={workspaceClasses.refTile}>
                        <ReferenceAssetPreview asset={asset} accessToken={app.session?.token} />
                        <button type="button" className={workspaceClasses.refRemove} title="移除编辑图片" onClick={() => removeEditAsset(asset)}><CloseGlyph /></button>
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}

          {/* Prompt */}
          <div className={mode === 'reference' ? workspaceClasses.promptBlockReference : workspaceClasses.promptBlock}>
            <label className={workspaceClasses.fieldLabel}>提示词</label>
            <div className={rdWorkspace.promptWrapper}>
              <textarea
                className={cn(rdWorkspace.textarea, 'redesign-prompt-input')}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={5}
                placeholder="描述想要生成的内容..."
              />
            </div>
          </div>

          {/* Negative prompt (collapsed) */}
          <details className={workspaceClasses.details}>
            <summary className={workspaceClasses.summary}>限制词</summary>
            <div className={cn(rdWorkspace.promptWrapper, workspaceClasses.negativeArea)}>
              <textarea
                className={cn(rdWorkspace.textarea, 'redesign-prompt-input')}
                value={negative}
                onChange={(e) => setNegative(e.target.value)}
                rows={2}
              />
            </div>
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
                className={cn(workspaceClasses.modelButton, model === m.code && workspaceClasses.modelButtonActive)}
                onClick={() => setModel(m.code)}
              >
                <span className={rdWorkspace.modelInfo}>
                  <span className={rdWorkspace.itemLabel}>{m.name}</span>
                </span>
                <span className={cn(workspaceClasses.modelMeta, model === m.code && workspaceClasses.modelMetaActive)}>{m.display_points ? `${m.display_points} ◈` : m.effective_multiplier ? `${m.effective_multiplier}x` : ''}</span>
              </button>
            )) : null}
            {!loading && !availableModels.length ? <EmptyState title="平台模型配置中" detail={publicUnavailableReason(capability?.unavailable_reason)} /> : null}
          </div>

          {selectedModel ? (
            <>
              {/* Quality */}
              {qualities.length ? (
                <div className={workspaceClasses.fieldBlock}>
                  <label className={workspaceClasses.fieldLabel}>清晰度</label>
                  <div className={workspaceClasses.selectGrid}>
                    {qualities.map((q) => (
                      <button
                        key={q}
                        type="button"
                        className={cn(workspaceClasses.selectItem, quality === q && workspaceClasses.selectItemActive)}
                        onClick={() => setQuality(q)}
                      >
                        {workspaceQualityLabel(q)}
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}

              {/* Aspect Ratio */}
              {ratios.length ? (
                <div className={workspaceClasses.fieldBlock}>
                  <label className={workspaceClasses.fieldLabel}>比例</label>
                  <div className={workspaceClasses.selectGridThree}>
                    {ratios.map((r) => (
                      <button
                        key={r}
                        type="button"
                        className={cn(workspaceClasses.selectItem, ratio === r && workspaceClasses.selectItemActive)}
                        onClick={() => setRatio(r)}
                      >
                        <AspectRatioIcon ratio={r} active={ratio === r} />
                        <span className={rdWorkspace.itemLabel}>{r}</span>
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}

              {/* Image Count */}
              {counts.length ? (
                <div>
                  <label className={workspaceClasses.fieldLabel}>图片数量</label>
                  <div className={workspaceClasses.countInputWrap}>
                    <button type="button" className={workspaceClasses.countStepper} disabled={count <= 1} onClick={() => updateImageCount(count - 1)}>-</button>
                    <input
                      className={workspaceClasses.countInput}
                      type="number"
                      min={1}
                      max={maxOutputCount}
                      value={count || 1}
                      onChange={(event) => updateImageCount(Number(event.target.value))}
                    />
                    <button type="button" className={workspaceClasses.countStepper} disabled={count >= maxOutputCount} onClick={() => updateImageCount(count + 1)}>+</button>
                  </div>
                  <p className={workspaceClasses.countHint}>后端当前允许最多生成 {maxOutputCount} 张。</p>
                </div>
              ) : null}
            </>
          ) : null}
        </div>
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
            <span className={rdWorkspace.btnGlow} />
            <span className={rdWorkspace.btnText}>
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
            </span>
          </button>
        </div>
      </aside>

      {/* Right Canvas */}
      <section className={workspaceClasses.canvas}>
        {latestTask ? (
          <div className={workspaceClasses.feed}>
            <div className={workspaceClasses.outputTabs} role="tablist" aria-label="创作输出">
              <button type="button" role="tab" aria-selected={outputTab === 'current'} className={cn(workspaceClasses.outputTab, outputTab === 'current' && workspaceClasses.outputTabActive)} onClick={() => setOutputTab('current')}>当前创作</button>
              <button type="button" role="tab" aria-selected={outputTab === 'history'} className={cn(workspaceClasses.outputTab, outputTab === 'history' && workspaceClasses.outputTabActive)} onClick={() => setOutputTab('history')}>历史创作</button>
            </div>
            {outputTab === 'current' ? (
              <GenerationOutput
                key={latestTask.id}
                task={latestTask}
                onCopyPrompt={async () => {
                  await copyText(latestTask.prompt)
                  app.notify('success', '提示词已复制')
                }}
                onUseReference={applyAsEditSource}
                onPreviewImage={setPreviewImage}
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
              />
            ) : (
              <HistoryCreationGrid tasks={historyTasks} accessToken={app.session?.token} onPreviewImage={setPreviewImage} onOpenTask={setHistoryTaskDialog} />
            )}
            <div ref={feedEndRef} />
          </div>
        ) : (
          <div className={workspaceClasses.placeholder}>
            <div className="max-w-sm opacity-60">
              <div className={workspaceClasses.readyIcon}><SparkleGlyph /></div>
              <h2 className={workspaceClasses.placeholderTitle}>准备就绪</h2>
              <p className={workspaceClasses.placeholderText}>在左侧设置参数并输入提示词，点击「开始创作」即可生成画面。</p>
            </div>
          </div>
        )}

        <ImageLightbox image={previewImage} onClose={() => setPreviewImage(null)} />
        {historyTaskDialog ? (
          <HistoryTaskDialog
            task={historyTaskDialog}
            accessToken={app.session?.token}
            onClose={() => setHistoryTaskDialog(null)}
            onPreviewImage={setPreviewImage}
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
            onClose={() => setGalleryImportTarget(null)}
          />
        ) : null}

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

function SparkleGlyph() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 3l1.7 5.2L19 10l-5.3 1.8L12 17l-1.7-5.2L5 10l5.3-1.8L12 3Z" />
      <path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15Z" />
    </svg>
  )
}

function ImageGlyph() {
  return (
    <svg className="size-4 text-[var(--accent)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <path d="m3 16 5-5 4 4 2-2 7 6" />
      <circle cx="8.5" cy="9.5" r="1.5" />
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

function ChevronGlyph() {
  return (
    <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m6 9 6 6 6-6" />
    </svg>
  )
}

function EditGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5Z" />
    </svg>
  )
}

function DownloadGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 3v12" />
      <path d="m7 10 5 5 5-5" />
      <path d="M5 21h14" />
    </svg>
  )
}

function AspectRatioIcon({ ratio, active }: { ratio: string; active?: boolean }) {
  const [rawWidth, rawHeight] = ratio.split(':').map(Number)
  const width = rawWidth > 0 && rawHeight > 0 ? Math.max(10, Math.min(24, 22 * (rawWidth / Math.max(rawWidth, rawHeight)))) : 16
  const height = rawWidth > 0 && rawHeight > 0 ? Math.max(10, Math.min(24, 22 * (rawHeight / Math.max(rawWidth, rawHeight)))) : 16
  return (
    <span className={cn(rdWorkspace.itemIcon, active && rdWorkspace.itemIconActive)}>
      <span
        className="block rounded-xl border-2 border-current"
        style={{ width, height }}
        aria-hidden="true"
      />
    </span>
  )
}

function GalleryImportModal({ images, filter, loading, busy, remainingLimit, accessToken, onFilterChange, onConfirm, onClose }: {
  images: GalleryImage[]
  filter: GalleryImportFilter
  loading: boolean
  busy: boolean
  remainingLimit: number
  accessToken?: string | null
  onFilterChange: (filter: GalleryImportFilter) => void
  onConfirm: (ids: string[]) => void
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
      <div className={workspaceClasses.importFilters}>
        <input className={workspaceClasses.importInput} value={filter.query} onChange={(event) => updateFilter({ query: event.target.value })} placeholder="搜索提示词、模型、分组" />
        <select className={workspaceClasses.importInput} value={filter.group} onChange={(event) => updateFilter({ group: event.target.value })}>
          <option value="all">全部分组</option>
          {options.groups.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className={workspaceClasses.importInput} value={filter.publishStatus} onChange={(event) => updateFilter({ publishStatus: event.target.value })}>
          <option value="all">全部状态</option>
          {options.publishStatuses.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className={workspaceClasses.importInput} value={filter.model} onChange={(event) => updateFilter({ model: event.target.value })}>
          <option value="all">全部模型</option>
          {options.models.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select className={workspaceClasses.importInput} value={filter.ratio} onChange={(event) => updateFilter({ ratio: event.target.value })}>
          <option value="all">全部比例</option>
          {options.ratios.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
      </div>

      {loading ? <LoadingState label="正在读取资产..." /> : null}
      {!loading && !rows.length ? <EmptyState title="没有匹配的资产" detail="换一个搜索词或筛选条件。" /> : null}
      {rows.length ? (
        <div className={workspaceClasses.importGrid}>
          {rows.map((image) => {
            const selected = selectedIds.has(image.id)
            const disabled = !selected && selectedIds.size >= remainingLimit
            const imageUrl = userApi.imageAssetUrl(image.url || image.download_url || '', accessToken)
            return (
              <button
                key={image.id}
                type="button"
                className={cn(workspaceClasses.importTile, selected && workspaceClasses.importTileActive)}
                disabled={disabled}
                onClick={() => toggle(image.id)}
              >
                <span className={workspaceClasses.importCheck}>{selected ? '✓' : ''}</span>
                {imageUrl ? <img className={workspaceClasses.importThumb} src={imageUrl} alt={image.prompt || image.id} /> : null}
                <span className={workspaceClasses.importInfo}>
                  <strong className={workspaceClasses.importTitle}>{image.prompt || image.id}</strong>
                  <span>{image.route_model_code || image.abstract_model || '未知模型'} · {image.aspect_ratio || '未知比例'}</span>
                </span>
              </button>
            )
          })}
        </div>
      ) : null}

      <div className={workspaceClasses.importActions}>
        <span className="mr-auto text-sm text-[var(--muted)]">已选 {selectedIds.size} 张，最多还能选择 {remainingLimit} 张</span>
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

function HistoryCreationGrid({ tasks, accessToken, onPreviewImage, onOpenTask }: {
  tasks: ImageTask[]
  accessToken?: string | null
  onPreviewImage: (image: ImageLightboxPayload) => void
  onOpenTask: (task: ImageTask) => void
}) {
  if (!tasks.length) {
    return <EmptyState title="暂无历史创作" detail="完成一次创作后，任务记录会展示在这里。" />
  }
  return (
    <div className={workspaceClasses.historyGrid}>
      {tasks.map((task) => (
        <HistoryCreationCard
          key={task.id}
          task={task}
          accessToken={accessToken}
          onPreviewImage={onPreviewImage}
          onOpenTask={onOpenTask}
        />
      ))}
    </div>
  )
}

function HistoryCreationCard({ task, accessToken, onPreviewImage, onOpenTask }: {
  task: ImageTask
  accessToken?: string | null
  onPreviewImage: (image: ImageLightboxPayload) => void
  onOpenTask: (task: ImageTask) => void
}) {
  const slots = generationSlots(task)
  const imageSlot = slots.find((slot): slot is Extract<typeof slot, { kind: 'image' }> => slot.kind === 'image')
  const requested = Math.max(Number(task.image_count || 1), task.results.length || 0)
  const running = !isTerminalStatus(task.status)
  const allFailed = isTerminalStatus(task.status) && !task.results.length && task.status !== 'succeeded'
  const partialFailed = task.status === 'partial_failed' || (isTerminalStatus(task.status) && task.results.length > 0 && task.results.length < requested)
  const multi = requested > 1
  const imageUrl = imageSlot ? userApi.imageAssetUrl(imageSlot.image.url, accessToken) : ''
  const downloadUrl = imageSlot ? userApi.imageAssetUrl(imageSlot.image.download_url ?? imageSlot.image.url, accessToken) : ''
  const openPreview = () => {
    if (multi && task.results.length > 1) {
      onOpenTask(task)
      return
    }
    if (!imageSlot || !imageUrl) return
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
    })
  }

  return (
    <button type="button" className={workspaceClasses.historyCard} disabled={!imageSlot} onClick={openPreview}>
      <span className={workspaceClasses.historyPreview}>
        {multi ? <span className={cn(workspaceClasses.historyLayer, workspaceClasses.historyLayerBack2)} aria-hidden="true" /> : null}
        {multi ? <span className={cn(workspaceClasses.historyLayer, workspaceClasses.historyLayerBack1)} aria-hidden="true" /> : null}
        <span className={cn(workspaceClasses.historyLayer, allFailed && workspaceClasses.historyFailed)}>
          {imageSlot && imageUrl ? (
            <img className={workspaceClasses.historyImage} src={imageUrl} alt={task.title} />
          ) : running ? (
            <span className={workspaceClasses.historyState}>
              <span className={userState.spinner} />
              <span>生成中</span>
            </span>
          ) : (
            <span className={workspaceClasses.historyState}>
              <span className="text-xl font-black text-[var(--accent-coral)]">!</span>
              <span>{task.error_message || task.failure_reason || '生成失败'}</span>
            </span>
          )}
        </span>
        {partialFailed ? <span className={workspaceClasses.historyWarn} title="部分失败">部分失败</span> : null}
      </span>
      <span className={workspaceClasses.historyTitle}>{task.prompt || task.title || '未命名创作'}</span>
      <span className={workspaceClasses.historyMeta}>
        <span>{formatHistoryTime(task.created_at)}</span>
        <span>{formatCompactDuration(taskElapsedMs(task))}</span>
        <span>{task.results.length}/{requested} 张</span>
      </span>
    </button>
  )
}

function HistoryTaskDialog({ task, accessToken, onClose, onPreviewImage }: {
  task: ImageTask
  accessToken?: string | null
  onClose: () => void
  onPreviewImage: (image: ImageLightboxPayload) => void
}) {
  return (
    <Modal title="历史创作图片" onClose={onClose}>
      <div className="mb-4 text-sm text-[var(--muted)]">{task.prompt || task.title || '未命名创作'}</div>
      <div className={workspaceClasses.historyDialogGrid}>
        {task.results.map((image, index) => {
          const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
          const downloadUrl = userApi.imageAssetUrl(image.download_url ?? image.url, accessToken)
          return (
            <div className={workspaceClasses.historyDialogTile} key={image.id || `${task.id}-${index}`}>
              <button
                type="button"
                className={workspaceClasses.historyDialogButton}
                onClick={() => onPreviewImage({
                  url: imageUrl,
                  downloadUrl,
                  alt: task.title,
                  prompt: image.prompt || task.prompt,
                  width: image.width,
                  height: image.height,
                  ratio: task.aspect_ratio,
                  model: image.route_model_code || task.route_model_code || task.model_group,
                  source: '历史创作',
                })}
              >
                <img className={workspaceClasses.historyDialogImage} src={imageUrl} alt={task.title} />
              </button>
              <div className={workspaceClasses.historyDialogTileMeta}>
                第 {index + 1} 张 · {image.width || '?'} x {image.height || '?'}
              </div>
            </div>
          )
        })}
      </div>
    </Modal>
  )
}

function GenerationOutput({ task, onCopyPrompt, onUseReference, onPreviewImage, onRetryTask, onDeleteTask, accessToken }: {
  task: ImageTask
  onCopyPrompt: () => Promise<void>
  onUseReference: (url: string) => Promise<void>
  onPreviewImage: (image: ImageLightboxPayload) => void
  onRetryTask: (task: ImageTask) => Promise<void>
  onDeleteTask: (task: ImageTask) => Promise<void>
  accessToken?: string
}) {
  const slots = generationSlots(task)
  const activeStage = task.progress_message || task.progress_stage || '等待后端返回任务进度'
  const successImages = slots.filter((slot): slot is Extract<typeof slot, { kind: 'image' }> => slot.kind === 'image')
  const primaryImage = successImages[0]?.image ?? null
  const [skeletonPhase, setSkeletonPhase] = useState(false)
  const showInitialLoading = !isTerminalStatus(task.status) && !skeletonPhase && !successImages.length
  const showSlots = skeletonPhase || successImages.length > 0 || isTerminalStatus(task.status)

  useEffect(() => {
    if (isTerminalStatus(task.status) || successImages.length > 0) {
      setSkeletonPhase(true)
      return undefined
    }
    setSkeletonPhase(false)
    const timer = window.setTimeout(() => setSkeletonPhase(true), 10000)
    return () => window.clearTimeout(timer)
  }, [task.id, task.status, successImages.length])

  const downloadAll = () => {
    successImages.forEach((slot, index) => {
      const url = userApi.imageAssetUrl(slot.image.download_url ?? slot.image.url, accessToken)
      window.setTimeout(() => window.open(url, '_blank', 'noopener,noreferrer'), index * 120)
    })
  }
  return (
    <article className={workspaceClasses.record}>
      {showInitialLoading ? (
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
      ) : showSlots && slots.length ? (
        <div className={workspaceClasses.outputResultWrap}>
          {task.task_type === 'image_edit' && task.reference_assets.length ? (
            <div className={workspaceClasses.sourceImages}>
              <span className={workspaceClasses.sourceImagesTitle}>原图引用</span>
              {task.reference_assets.map((asset) => (
                <ReferenceAssetPreview
                  key={asset.id || asset.preview_url || asset.download_url}
                  asset={asset}
                  accessToken={accessToken}
                  onClick={(url) => onPreviewImage({ url, alt: asset.name || '原图引用', source: '原图引用' })}
                />
              ))}
            </div>
          ) : null}
          <div className={successImages.length === 1 && slots.length === 1 ? 'w-full max-w-5xl' : workspaceClasses.outputResultGrid}>
            {slots.map((slot) => {
              if (slot.kind === 'image') {
                return (
                  <GeneratedImage
                    key={slot.image.id || `image-${slot.index}`}
                    image={slot.image}
                    alt={task.title}
                    fallbackRatio={task.aspect_ratio}
                    accessToken={accessToken}
                    onUseReference={onUseReference}
                    onPreview={onPreviewImage}
                  />
                )
              }
              if (slot.kind === 'failed') {
                return <GenerationSlotFailure key={`failed-${slot.index}`} title={slot.title} reason={slot.reason} code={slot.code} />
              }
              return <GenerationSlotPending key={`pending-${slot.index}`} label={slot.label} skeleton={skeletonPhase} />
            })}
          </div>
          {successImages.length && primaryImage ? (
            <div className={workspaceClasses.outputActions}>
              <button className={workspaceClasses.generatedAction} type="button" title="下载" onClick={downloadAll}>{successImages.length > 1 ? '全部下载' : '下载'}</button>
              <div className="mx-1 h-4 w-px bg-[var(--border)]" />
              <button className={workspaceClasses.generatedAction} type="button" title="复制提示词" onClick={() => void onCopyPrompt()}>提示词</button>
              <div className="mx-1 h-4 w-px bg-[var(--border)]" />
              <button className={workspaceClasses.generatedAction} type="button" title="再次编辑" onClick={() => {
                if (primaryImage.url) void onUseReference(userApi.imageAssetUrl(primaryImage.url, accessToken))
              }}>编辑</button>
            </div>
          ) : null}
          <div className={workspaceClasses.outputMetaRow}>
            <span>模型: {task.route_model_name || task.route_model_code || task.model_group}</span>
            <span>比例: {task.aspect_ratio}</span>
            <span>数量: {task.image_count}</span>
            <span>耗时: {formatCompactDuration(taskElapsedMs(task))}</span>
            <span>消耗: {displayTaskPoints(task)} ◈</span>
          </div>
        </div>
      ) : isTerminalStatus(task.status) ? (
        <TaskFailureBlock task={task} onRetry={() => onRetryTask(task)} onDelete={() => onDeleteTask(task)} />
      ) : (
        <GenerationSlotPending label="生成中" skeleton={false} />
      )}
    </article>
  )
}

function GenerationSlotPending({ label, skeleton }: { label: string; skeleton: boolean }) {
  if (skeleton) {
    return (
      <div className={workspaceClasses.slotSkeleton}>
        <div className={workspaceClasses.slotSkeletonFrame}>
          <span className={workspaceClasses.slotSkeletonGlow} aria-hidden="true" />
          <span className={workspaceClasses.slotSkeletonLoader}>
            <span className="grid place-items-center gap-2">
              <span className={userState.spinner} />
              <strong className={workspaceClasses.pendingStrong}>{label}</strong>
            </span>
          </span>
        </div>
      </div>
    )
  }
  return (
    <div className={workspaceClasses.slotState}>
      <span className={userState.spinner} />
      <strong className={workspaceClasses.pendingStrong}>{label}</strong>
    </div>
  )
}

function GenerationSlotFailure({ title, reason, code }: { title: string; reason: string; code?: string }) {
  return (
    <div className={cn(workspaceClasses.slotState, workspaceClasses.slotFailed)}>
      <strong className={workspaceClasses.pendingFailedTitle}>{title}</strong>
      <p className={workspaceClasses.placeholderText}>{reason}</p>
      {code ? <span className={workspaceClasses.slotCode}>{code}</span> : null}
    </div>
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
        <button className={cn(userButton.base, userButton.primary, 'min-h-9 rounded-xl px-3 text-sm')} type="button" onClick={() => void onRetry()}>重试</button>
        <button className={cn(userButton.base, userButton.ghost, 'min-h-9 rounded-xl px-3 text-sm')} type="button" onClick={() => void onDelete()}>删除</button>
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

function GeneratedImage({ image, alt, fallbackRatio, accessToken, onUseReference, onPreview }: {
  image: ImageResult
  alt: string
  fallbackRatio?: string
  accessToken?: string
  onUseReference: (url: string) => Promise<void>
  onPreview: (image: ImageLightboxPayload) => void
}) {
  const imageUrl = userApi.imageAssetUrl(image.url, accessToken)
  const downloadUrl = userApi.imageAssetUrl(image.download_url ?? image.url, accessToken)
  const aspectRatio = image.width && image.height ? `${image.width} / ${image.height}` : normalizeAspectRatio(fallbackRatio)
  const ratioStyle = aspectRatio ? { '--generated-ratio': aspectRatio } as CSSProperties : undefined
  const sizeClass = fallbackRatio && Number((fallbackRatio.split(':')[0] || '').trim()) > Number((fallbackRatio.split(':')[1] || '').trim())
    ? 'max-h-[calc(100vh-470px)]'
    : 'max-h-[calc(100vh-520px)]'
  return (
    <figure className={workspaceClasses.generatedFigure} style={ratioStyle}>
      <button
        type="button"
        className={cn(workspaceClasses.generatedPreview, sizeClass)}
        onClick={() => onPreview({
          url: imageUrl,
          downloadUrl,
          alt,
          prompt: image.prompt || alt,
          width: image.width,
          height: image.height,
          ratio: fallbackRatio,
          model: image.route_model_code || image.abstract_model,
          source: '创作输出',
        })}
        aria-label="预览生成图片"
      >
        <img className={cn(workspaceClasses.generatedImage, sizeClass)} src={imageUrl} alt={alt} />
      </button>
      <figcaption className={workspaceClasses.generatedCaption}>
        <button className={workspaceClasses.generatedIconAction} type="button" title="编辑" aria-label="编辑图片" onClick={() => void onUseReference(imageUrl)}><EditGlyph /></button>
        <button className={workspaceClasses.generatedIconAction} type="button" title="下载" aria-label="下载图片" onClick={() => window.open(downloadUrl, '_blank', 'noopener,noreferrer')}><DownloadGlyph /></button>
      </figcaption>
    </figure>
  )
}
