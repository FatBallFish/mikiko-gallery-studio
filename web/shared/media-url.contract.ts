// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import { readFileSync } from 'node:fs'
import { isAbsoluteHTTPMediaURL, mediaAssetURL } from './media-url'

const signed = 'https://objects.example.test/generated/image.png?X-Amz-Signature=secret&X-Amz-Expires=300'
assert.equal(isAbsoluteHTTPMediaURL(signed), true, 'signed HTTPS URLs should be classified as absolute media')
assert.equal(isAbsoluteHTTPMediaURL('HTTP://objects.example.test/image.png'), true, 'HTTP schemes should be case-insensitive through URL parsing')
assert.equal(isAbsoluteHTTPMediaURL('/api/agent/image/v1/images/image-1'), false, 'application fallback paths should remain relative')
assert.equal(mediaAssetURL(signed, 'application-token', 'https://app.example.test'), signed, 'signed URLs must remain byte-for-byte unchanged')
assert.equal(
  mediaAssetURL('/api/agent/image/v1/images/image-1?download=1', 'application-token', 'https://app.example.test'),
  'https://app.example.test/api/agent/image/v1/images/image-1?download=1&access_token=application-token',
  'relative fallback paths should receive the application token',
)

for (const file of ['../user/src/ui/mediaRefresh.tsx', '../admin/src/ui/mediaRefresh.tsx']) {
  const refreshSource = readFileSync(new URL(file, import.meta.url), 'utf8')
  for (const contract of [
    'const attemptedRef = useRef(false)',
    'if (attemptedRef.current || !src || !isAbsoluteHTTPMediaURL(src)) return false',
    'attemptedRef.current = true',
    'attemptedRef.current = false',
  ]) {
    assert.ok(refreshSource.includes(contract), `${file} must bound media refresh and reset only after a successful load: ${contract}`)
  }
}

for (const [file, contract] of [
  ['../user/src/pages/HomePage.tsx', 'onMediaRefresh={() => void publicGallery.reload()}'],
  ['../user/src/pages/GalleryPage.tsx', 'onMediaRefresh={() => void reloadLoadedPages()}'],
  ['../user/src/pages/PublicGalleryPage.tsx', "onMediaRefresh={() => void loadPage(1, 'replace')}"],
  ['../user/src/pages/WorkspacePage.tsx', 'onMediaRefresh={() => void refreshWorkspaceMedia()}'],
  ['../admin/src/pages/ReviewPage.tsx', 'onMediaRefresh={() => void load()}'],
] as const) {
  const source = readFileSync(new URL(file, import.meta.url), 'utf8')
  assert.ok(source.includes(contract), `${file} must refetch its media resource once after an expired signed URL fails`)
}

const reviewSource = readFileSync(new URL('../admin/src/pages/ReviewPage.tsx', import.meta.url), 'utf8')
assert.ok(reviewSource.includes('selectedRow.imageURL'), 'admin review must consume the projected image URL from its list response')

console.log('OK: direct media URL and bounded refresh contract passed')
