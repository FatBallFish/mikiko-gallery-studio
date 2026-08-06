// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import {
  attachmentPolicyDefaults,
  attachmentPolicyFieldDefinitions,
  attachmentPolicyIsDirty,
  normalizeAttachmentFormats,
  validateAttachmentPolicyDraft,
} from './AttachmentPolicyPage'

if (attachmentPolicyFieldDefinitions.length !== 8) {
  throw new Error(`attachment policy must expose eight settings, got ${attachmentPolicyFieldDefinitions.length}`)
}

const keys = new Set(attachmentPolicyFieldDefinitions.map((field) => field.key))
for (const key of [
  'image_max_mb', 'video_max_mb', 'audio_max_mb', 'document_max_mb',
  'image_allowed_formats', 'video_allowed_formats', 'audio_allowed_formats', 'document_allowed_formats',
]) {
  if (!keys.has(key as never)) throw new Error(`attachment policy is missing ${key}`)
}

if (attachmentPolicyDefaults.image_max_mb !== 20 || attachmentPolicyDefaults.image_allowed_formats.join(',') !== 'png,jpeg,webp,gif') {
  throw new Error('image attachment defaults must remain 20 MB and PNG/JPEG/WebP/GIF')
}

const reserved = attachmentPolicyFieldDefinitions.filter((field) => field.reserved)
if (reserved.length !== 6 || reserved.some((field) => field.kind === 'image')) {
  throw new Error('video, audio, and document settings must be visibly reserved while image settings are active')
}

const normalized = normalizeAttachmentFormats([' .PNG ', 'jpg', 'image/jpeg', 'webp', 'png'])
if (normalized.join(',') !== 'png,jpeg,webp') {
  throw new Error(`format normalization drifted: ${normalized.join(',')}`)
}

const validDraft = { ...attachmentPolicyDefaults }
if (Object.keys(validateAttachmentPolicyDraft(validDraft)).length !== 0) {
  throw new Error('default attachment policy must validate')
}
if (!validateAttachmentPolicyDraft({ ...validDraft, image_allowed_formats: ['png', 'svg'] }).image_allowed_formats) {
  throw new Error('image policy must reject SVG')
}
if (!validateAttachmentPolicyDraft({ ...validDraft, video_max_mb: 0 }).video_max_mb) {
  throw new Error('attachment sizes must reject zero')
}
if (attachmentPolicyIsDirty(validDraft, { ...validDraft })) {
  throw new Error('equal attachment policy drafts must remain pristine')
}
if (!attachmentPolicyIsDirty(validDraft, { ...validDraft, image_max_mb: 21 })) {
  throw new Error('changed attachment policy must become dirty')
}

const source = readFileSync(new URL('./AttachmentPolicyPage.tsx', import.meta.url), 'utf8')
for (const contract of ['onDirtyChange', 'onBusyChange', 'beforeunload', "updateConfigTab('attachment_policy'", '仅图片策略当前生效', '预留配置', '保存附件策略']) {
  if (!source.includes(contract)) throw new Error(`attachment policy editor must implement ${contract}`)
}
