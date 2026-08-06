import { normalizeCapabilities } from '../../../shared/user-api'
import { readFileSync } from 'node:fs'
import { referenceImageAccept, referenceImagePolicy, validateReferenceImageFile } from './referenceImageUpload'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

const capability = normalizeCapabilities({
  ReferenceImageMaxBytes: 20 * 1024 * 1024,
  ReferenceImageAllowedFormats: ['png', 'jpeg', 'webp'],
  ReferenceImageAllowedMIMETypes: ['image/png', 'image/jpeg', 'image/webp'],
})
assert(capability.reference_image_allowed_formats?.join(',') === 'png,jpeg,webp', 'capability formats must survive normalization')
assert(capability.reference_image_allowed_mime_types?.includes('image/webp'), 'capability MIME types must survive normalization')

const policy = referenceImagePolicy(capability)
assert(validateReferenceImageFile({ name: 'photo.jpg', type: 'image/jpeg', size: 20 * 1024 * 1024 }, policy).ok, 'configured JPEG at the limit should pass')
assert(!validateReferenceImageFile({ name: 'large.png', type: 'image/png', size: 20 * 1024 * 1024 + 1 }, policy).ok, 'over-limit image should fail')
assert(!validateReferenceImageFile({ name: 'vector.svg', type: 'image/svg+xml', size: 1024 }, policy).ok, 'SVG should fail when absent from policy')
assert(!validateReferenceImageFile({ name: 'fake.png', type: 'image/jpeg', size: 1024 }, policy).ok, 'declared extension/MIME mismatch should fail client validation')
const accept = referenceImageAccept(policy)
for (const item of ['.png', '.jpg', '.jpeg', '.webp', 'image/png', 'image/jpeg', 'image/webp']) {
  assert(accept.includes(item), `file input accept is missing ${item}: ${accept}`)
}

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
for (const contract of [
  'referenceImagePolicy(capability)',
  'validateReferenceImageFile(file, referencePolicy)',
  'referenceImageAccept(referencePolicy)',
  'accept={referenceAccept}',
]) {
  assert(workspaceSource.includes(contract), `workspace uploads must enforce the dynamic image policy: missing ${contract}`)
}
assert(!workspaceSource.includes("file.type.startsWith('image/')"), 'drag-and-drop must not discard extension-valid files before policy validation')
