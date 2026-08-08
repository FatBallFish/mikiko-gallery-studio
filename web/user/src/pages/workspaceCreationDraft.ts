import type { Capability, CapabilityModelGroup, ImageTaskType } from '../../../shared/api-types'
import { workspaceBackgroundForFormat, workspaceCustomRatioSupported, workspaceCustomRatioValid, workspaceCustomSizeSupported, workspaceModelForTask, normalizeWorkspaceCustomSize } from './workspaceParameters'

export const workspaceCreationDraftStorageKey = 'pic-gallery-workspace-creation-draft-v1'
const workspaceCreationDraftHistoryKey = '__picGalleryWorkspaceCreationDraft'

export type WorkspaceCreationDraft = {
  version: 1
  prompt: string
  task_type: ImageTaskType
  route_model_code?: string
	size_mode?: 'auto' | 'ratio' | 'pixel' | string
  base_resolution?: string
  aspect_ratio?: string
  pixel_size?: string
  quality?: string
	output_format?: string
	background?: string
  output_compression?: number
  moderation?: string
  image_count?: number
  reference_asset_ids?: string[]
}

export type NormalizedWorkspaceCreationDraft = {
  values: Required<Pick<WorkspaceCreationDraft,
    'version' | 'prompt' | 'task_type' | 'route_model_code' | 'size_mode' | 'base_resolution' |
		'aspect_ratio' | 'pixel_size' | 'quality' | 'output_format' | 'background' | 'output_compression' |
    'moderation' | 'image_count' | 'reference_asset_ids'>>
  notices: string[]
}

export type WorkspaceCreationSnapshot = {
  prompt?: string | null
  task_type?: ImageTaskType | null
  route_model_code?: string | null
  abstract_model?: string | null
  size_mode?: string | null
  requested_size?: string | null
  base_resolution?: string | null
  quality?: string | null
  aspect_ratio?: string | null
	output_format?: string | null
	background?: string | null
  output_compression?: number | null
  moderation?: string | null
  requested_output_image_count?: number | null
  image_count?: number | null
  reference_asset_ids?: string[] | null
}

type SessionStorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>
type HistoryLike = Pick<History, 'state' | 'replaceState'>

export function workspaceCreationDraftFromSnapshot(snapshot: WorkspaceCreationSnapshot): WorkspaceCreationDraft {
  const sizeMode = clean(snapshot.size_mode) ?? 'ratio'
  return {
    version: 1,
    prompt: snapshot.prompt ?? '',
    task_type: snapshot.task_type === 'image_edit' ? 'image_edit' : 'text_to_image',
    route_model_code: clean(snapshot.route_model_code ?? snapshot.abstract_model),
    size_mode: sizeMode,
    base_resolution: clean(snapshot.base_resolution),
    aspect_ratio: clean(snapshot.aspect_ratio),
    pixel_size: sizeMode === 'pixel' ? clean(snapshot.requested_size ?? snapshot.aspect_ratio) : undefined,
    quality: clean(snapshot.quality),
		output_format: clean(snapshot.output_format),
		background: clean(snapshot.background),
    output_compression: typeof snapshot.output_compression === 'number' ? snapshot.output_compression : undefined,
    moderation: clean(snapshot.moderation),
    image_count: snapshot.requested_output_image_count ?? snapshot.image_count ?? 1,
    reference_asset_ids: snapshot.reference_asset_ids ?? [],
  }
}

export function stageWorkspaceCreationDraft(
  draft: WorkspaceCreationDraft,
  session: SessionStorageLike,
  history: HistoryLike,
) {
  const normalized = parseWorkspaceCreationDraft(draft)
  if (!normalized) throw new Error('无法暂存不受支持的创作配置。')
  session.setItem(workspaceCreationDraftStorageKey, JSON.stringify(normalized))
  history.replaceState({ ...historyRecord(history.state), [workspaceCreationDraftHistoryKey]: normalized }, '')
}

export function consumeWorkspaceCreationDraft(
  session: SessionStorageLike,
  history: HistoryLike,
): WorkspaceCreationDraft | null {
  const state = historyRecord(history.state)
  const fromHistory = parseWorkspaceCreationDraft(state[workspaceCreationDraftHistoryKey])
  const fromSession = parseStoredDraft(session.getItem(workspaceCreationDraftStorageKey))
  session.removeItem(workspaceCreationDraftStorageKey)
  if (workspaceCreationDraftHistoryKey in state) {
    const nextState = { ...state }
    delete nextState[workspaceCreationDraftHistoryKey]
    history.replaceState(nextState, '')
  }
  return fromHistory ?? fromSession
}

export function normalizeWorkspaceCreationDraft(
  draft: WorkspaceCreationDraft,
  capability: Capability,
): NormalizedWorkspaceCreationDraft {
  const notices: string[] = []
  const requestedReferences = unique((draft.reference_asset_ids ?? []).map(clean).filter(Boolean) as string[])
  const requestedTaskType = draft.task_type === 'image_edit' && requestedReferences.length === 0
    ? 'text_to_image'
    : capability.task_types.includes(draft.task_type) ? draft.task_type : 'text_to_image'
  const taskType: ImageTaskType = capability.task_types.includes(requestedTaskType)
    ? requestedTaskType
    : capability.task_types[0] ?? 'text_to_image'
  if (taskType !== draft.task_type) notices.push(`任务类型 ${draft.task_type} 当前不可用，已切换为 ${taskType}。`)

  const models = capability.model_groups.filter((item) => item.task_types.includes(taskType))
  const rawModel = models.find((item) => item.code === clean(draft.route_model_code)) ?? models[0]
  const model = workspaceModelForTask(rawModel, taskType)
  if (!model) throw new Error('当前没有可用于该任务类型的模型。')
  if (model.code !== clean(draft.route_model_code)) notices.push(`模型 ${clean(draft.route_model_code) || '未指定'} 当前不可用，已切换为 ${model.name || model.code}。`)

	const sizeModes = unique(model.size_modes?.filter((item) => item === 'auto' || item === 'ratio' || item === 'pixel') ?? [])
	const sizeMode = chooseOption(sizeModes.length ? sizeModes : ['ratio'], draft.size_mode, '尺寸模式', notices)
	const baseResolution = sizeMode === 'ratio' ? chooseOption(model.base_resolution ?? capability.base_resolution ?? [], draft.base_resolution, '基础分辨率', notices) : ''
  const aspectRatios = model.aspect_ratios?.length ? model.aspect_ratios : capability.aspect_ratios
  const pixelSizes = model.pixel_sizes?.length ? model.pixel_sizes : capability.pixel_sizes ?? []
	const requestedRatio = clean(draft.aspect_ratio)
	const customRatio = sizeMode === 'ratio' && workspaceCustomRatioSupported(model) && requestedRatio && workspaceCustomRatioValid(requestedRatio)
	const aspectRatio = sizeMode === 'ratio' ? customRatio ? requestedRatio : chooseOption(aspectRatios, draft.aspect_ratio, '画面比例', notices) : ''
  const pixelSize = sizeMode === 'pixel' ? normalizeDraftPixelSize(model, pixelSizes, draft.pixel_size, notices) : ''
  const quality = chooseOption(model.quality ?? model.qualities ?? capability.quality ?? capability.qualities ?? ['auto'], draft.quality, '质量', notices)
	const outputFormat = chooseOption(model.output_format ?? capability.output_format ?? ['png'], draft.output_format, '输出格式', notices)
	const background = workspaceBackgroundForFormat(model, clean(draft.background) ?? '', outputFormat)
	if (clean(draft.background) && background !== clean(draft.background)) notices.push(`背景 ${clean(draft.background)} 与当前输出格式不兼容，已调整为 ${background || '不传'}。`)
  const moderation = chooseOption(model.moderation ?? capability.moderation ?? ['auto'], draft.moderation, '内容审核', notices)

  const requestedCount = finiteInteger(draft.image_count, 1)
  const maxCount = Math.max(1, Math.min(capability.max_image_count || 1, model.max_output_image_count || capability.max_image_count || 1))
  const imageCount = Math.min(maxCount, Math.max(1, requestedCount))
  if (imageCount !== requestedCount) notices.push(`生成数量 ${requestedCount} 超出当前模型范围，已调整为 ${imageCount}。`)

  const compressionSupported = Boolean(model.supports_output_compression && ['jpeg', 'jpg', 'webp'].includes(outputFormat.toLowerCase()))
  const requestedCompression = finiteInteger(draft.output_compression, 100)
  const outputCompression = compressionSupported ? Math.min(100, Math.max(1, requestedCompression)) : 100
  if (draft.output_compression !== undefined && outputCompression !== requestedCompression) notices.push(`输出压缩值 ${requestedCompression} 当前不可用，已调整为 ${outputCompression}。`)
  if (draft.output_compression !== undefined && !compressionSupported && requestedCompression !== 100) notices.push('当前输出格式不支持自定义压缩，已恢复为默认值。')

  const referenceLimit = taskType === 'image_edit' ? Math.max(0, model.max_reference_image_count || 0) : 0
  const referenceAssetIds = requestedReferences.slice(0, referenceLimit)
  if (referenceAssetIds.length !== requestedReferences.length) notices.push(`引用图片超过当前模型上限，已保留前 ${referenceAssetIds.length} 张。`)

  return {
    values: {
      version: 1,
      prompt: draft.prompt,
      task_type: taskType,
      route_model_code: model.code,
      size_mode: sizeMode,
      base_resolution: baseResolution,
      aspect_ratio: aspectRatio,
      pixel_size: pixelSize,
      quality,
			output_format: outputFormat,
			background,
      output_compression: outputCompression,
      moderation,
      image_count: imageCount,
      reference_asset_ids: referenceAssetIds,
    },
    notices,
  }
}

function parseStoredDraft(raw: string | null) {
  if (!raw) return null
  try {
    return parseWorkspaceCreationDraft(JSON.parse(raw))
  } catch {
    return null
  }
}

function parseWorkspaceCreationDraft(value: unknown): WorkspaceCreationDraft | null {
  if (!value || typeof value !== 'object') return null
  const draft = value as Partial<WorkspaceCreationDraft>
  if (draft.version !== 1 || typeof draft.prompt !== 'string') return null
  if (draft.task_type !== 'text_to_image' && draft.task_type !== 'image_edit') return null
	const optionalStrings = [draft.route_model_code, draft.size_mode, draft.base_resolution, draft.aspect_ratio, draft.pixel_size, draft.quality, draft.output_format, draft.background, draft.moderation]
  if (optionalStrings.some((item) => item !== undefined && typeof item !== 'string')) return null
  if (draft.output_compression !== undefined && typeof draft.output_compression !== 'number') return null
  if (draft.image_count !== undefined && typeof draft.image_count !== 'number') return null
  if (draft.reference_asset_ids !== undefined && (!Array.isArray(draft.reference_asset_ids) || draft.reference_asset_ids.some((item) => typeof item !== 'string'))) return null
  return {
    version: 1,
    prompt: draft.prompt,
    task_type: draft.task_type,
    route_model_code: clean(draft.route_model_code),
    size_mode: clean(draft.size_mode),
    base_resolution: clean(draft.base_resolution),
    aspect_ratio: clean(draft.aspect_ratio),
    pixel_size: clean(draft.pixel_size),
    quality: clean(draft.quality),
		output_format: clean(draft.output_format),
		background: clean(draft.background),
    output_compression: typeof draft.output_compression === 'number' ? draft.output_compression : undefined,
    moderation: clean(draft.moderation),
    image_count: typeof draft.image_count === 'number' ? draft.image_count : undefined,
    reference_asset_ids: draft.reference_asset_ids ?? [],
  }
}

function historyRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function clean(value: unknown) {
  const trimmed = typeof value === 'string' ? value.trim() : ''
  return trimmed || undefined
}

function chooseOption(options: string[], requested: string | undefined, label: string, notices: string[]) {
  const candidates = unique(options.map((item) => item.trim()).filter(Boolean))
  const selected = candidates.find((item) => item.toLowerCase() === clean(requested)?.toLowerCase()) ?? candidates[0] ?? ''
  if (selected !== clean(requested)) notices.push(`${label} ${clean(requested) || '未指定'} 当前不可用，已调整为 ${selected || '未设置'}。`)
  return selected
}

function normalizeDraftPixelSize(model: CapabilityModelGroup, presets: string[], requested: string | undefined, notices: string[]) {
  const requestedSize = clean(requested)
  const preset = presets.find((item) => item.toLowerCase() === requestedSize?.toLowerCase())
  if (preset) return preset

  if (workspaceCustomSizeSupported(model) && requestedSize) {
    const match = requestedSize.match(/^\s*(\d+)\s*[xX×]\s*(\d+)\s*$/)
    if (match) {
		const normalized = normalizeWorkspaceCustomSize(match[1], match[2], model)
		if (normalized.valid) {
			return normalized.size
      }
    }
  }

  return chooseOption(presets, requested, '像素尺寸', notices)
}

function unique<T>(values: T[]) {
  return Array.from(new Set(values))
}

function finiteInteger(value: number | undefined, fallback: number) {
  return Number.isFinite(value) ? Math.round(value as number) : fallback
}
