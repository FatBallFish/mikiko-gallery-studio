import type { Capability } from '../../../shared/api-types'

const DEFAULT_FORMATS = ['png', 'jpeg', 'webp', 'gif']
const FORMAT_ALIASES: Record<string, string> = {
  jpg: 'jpeg',
  jpeg: 'jpeg',
  png: 'png',
  webp: 'webp',
  gif: 'gif',
}
const FORMAT_MIME: Record<string, string> = {
  png: 'image/png',
  jpeg: 'image/jpeg',
  webp: 'image/webp',
  gif: 'image/gif',
}
const FORMAT_EXTENSIONS: Record<string, string[]> = {
  png: ['.png'],
  jpeg: ['.jpg', '.jpeg'],
  webp: ['.webp'],
  gif: ['.gif'],
}

export type ReferenceImagePolicy = {
  maxBytes: number
  allowedFormats: string[]
  allowedMIMETypes: string[]
}

export type ReferenceImageFileLike = {
  name: string
  type: string
  size: number
}

export type ReferenceImageValidation = {
  ok: boolean
  message?: string
}

export function referenceImagePolicy(capability: Capability | null | undefined): ReferenceImagePolicy {
  const maxBytes = positiveNumber(capability?.reference_image_max_bytes)
    || positiveNumber(capability?.reference_image_max_mb) * 1024 * 1024
  const allowedFormats = normalizeFormats(capability?.reference_image_allowed_formats)
  const formats = allowedFormats.length ? allowedFormats : [...DEFAULT_FORMATS]
  const configuredMIMEs = (capability?.reference_image_allowed_mime_types ?? [])
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean)
  return {
    maxBytes,
    allowedFormats: formats,
    allowedMIMETypes: configuredMIMEs.length ? Array.from(new Set(configuredMIMEs)) : formats.map((format) => FORMAT_MIME[format]).filter(Boolean),
  }
}

export function validateReferenceImageFile(file: ReferenceImageFileLike, policy: ReferenceImagePolicy): ReferenceImageValidation {
  if (policy.maxBytes > 0 && file.size > policy.maxBytes) {
    return { ok: false, message: `${file.name} 超过单张最大 ${formatBytes(policy.maxBytes)}。` }
  }
  const extension = file.name.trim().toLowerCase().match(/\.([a-z0-9]+)$/)?.[1] ?? ''
  const extensionFormat = FORMAT_ALIASES[extension] ?? ''
  const mime = file.type.trim().toLowerCase()
  const mimeFormat = Object.entries(FORMAT_MIME).find(([, value]) => value === mime)?.[0] ?? ''
  const allowed = new Set(policy.allowedFormats)
  if (!extensionFormat || !allowed.has(extensionFormat) || (mime && !policy.allowedMIMETypes.includes(mime))) {
    return { ok: false, message: `${file.name} 格式不受支持；允许格式：${policy.allowedFormats.map((item) => item.toUpperCase()).join('、')}。` }
  }
  if (mimeFormat && mimeFormat !== extensionFormat) {
    return { ok: false, message: `${file.name} 的扩展名与文件类型不一致，请选择有效图片。` }
  }
  return { ok: true }
}

export function referenceImageAccept(policy: ReferenceImagePolicy) {
  const values = policy.allowedFormats.flatMap((format) => [
    ...(FORMAT_EXTENSIONS[format] ?? []),
    FORMAT_MIME[format],
  ]).filter(Boolean)
  return Array.from(new Set(values)).join(',')
}

function normalizeFormats(values?: string[]) {
  return Array.from(new Set((values ?? [])
    .map((item) => FORMAT_ALIASES[item.trim().toLowerCase()] ?? '')
    .filter(Boolean)))
}

function positiveNumber(value: unknown) {
  const numeric = Number(value ?? 0)
  return Number.isFinite(numeric) && numeric > 0 ? numeric : 0
}

function formatBytes(bytes: number) {
  const mb = bytes / (1024 * 1024)
  return `${mb >= 10 ? mb.toFixed(0) : mb.toFixed(1)} MB`
}
