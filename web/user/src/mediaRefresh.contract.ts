import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import { mediaRefreshDelay, mediaRefreshRetry, temporaryMediaExpiryFromURL } from './ui/mediaRefreshState'

assert.deepEqual(
  mediaRefreshRetry('https://objects.test/expired', 'https://objects.test/fresh', 'https://objects.test/expired'),
  { kind: 'replace', src: 'https://objects.test/fresh' },
  'a fresh projection should replace the failed URL',
)

assert.equal(
  mediaRefreshDelay('2026-08-06T12:01:00Z', Date.parse('2026-08-06T12:00:00Z')),
  30_000,
  'preview refresh should start thirty seconds before expiry',
)
assert.equal(mediaRefreshDelay('invalid', Date.now()), null, 'invalid expiry metadata must not schedule a refresh loop')
assert.equal(
  temporaryMediaExpiryFromURL('https://objects.test/image?X-Amz-Date=20260806T120000Z&X-Amz-Expires=360'),
  '2026-08-06T12:06:00.000Z',
  'fresh signed URLs must provide the next proactive refresh deadline',
)
assert.deepEqual(
  mediaRefreshRetry('https://objects.test/bucketed', 'https://objects.test/bucketed', 'https://objects.test/bucketed'),
  { kind: 'reload' },
  'the same bucketed URL must force one image reload',
)
assert.deepEqual(
  mediaRefreshRetry('https://objects.test/expired', undefined, 'https://objects.test/expired'),
  { kind: 'failed' },
  'a refresh callback that returns no usable URL must not suppress the image error',
)
assert.deepEqual(
  mediaRefreshRetry('https://objects.test/expired', 'https://objects.test/fresh', 'https://objects.test/new-parent-value'),
  { kind: 'failed' },
  'a stale refresh must not overwrite a newer parent URL',
)

const source = readFileSync(new URL('./ui/mediaRefresh.tsx', import.meta.url), 'utf8')
const componentSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
for (const contract of [
  'const [currentSrc, setCurrentSrc] = useState(src)',
  'const refreshFailedMedia = useCallback(async () =>',
  'const nextSrc = await Promise.resolve(refresh())',
  "retry.kind === 'replace'",
  'setCurrentSrc(retry.src)',
  "retry.kind === 'reload'",
  'setRetryRevision((current) => current + 1)',
  'mediaRetryKey',
  'const resetMediaRefresh = useCallback(() =>',
  'attemptedRef.current = false',
  'mediaRefreshDelay(currentExpiresAt)',
  'window.setTimeout(() => { void refreshFailedMedia() }, delay)',
  'proactiveRefresh = false',
  'if (!proactiveRefresh || currentSrc !== src',
  'return false',
  'src={currentSrc}',
  'return { currentSrc, mediaRetryKey: retryRevision, markMediaLoaded, refreshFailedMedia, resetMediaRefresh }',
]) {
  if (!source.includes(contract)) {
    throw new Error(`media refresh must replace only the failed image URL in place: missing ${contract}`)
  }
}
if (!componentSource.includes('useMediaRefreshOnce(image.url, image.onMediaRefresh, image.mediaExpiresAt, true)')) {
  throw new Error('full-screen image preview must proactively refresh near expiry')
}
if (!componentSource.includes('useMediaRefreshOnce(src, onMediaRefresh, mediaExpiresAt, true)')) {
  throw new Error('active detail image must proactively refresh near expiry')
}
if (componentSource.includes('useMediaRefreshOnce(src, onMediaRefresh, mediaExpiresAt, true)\n\n  useLayoutEffect')) {
  throw new Error('gallery list thumbnails must not schedule proactive refresh timers')
}
if (source.includes('void Promise.resolve(refresh())')) {
  throw new Error('media refresh must propagate refresh success or failure to its caller')
}
