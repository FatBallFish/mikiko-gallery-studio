export type MediaRefreshRetry =
  | { kind: 'replace'; src: string }
  | { kind: 'reload' }
  | { kind: 'failed' }

export function mediaRefreshRetry(
  failedSrc: string,
  nextSrc: string | undefined | void,
  activeSrc: string | undefined,
): MediaRefreshRetry {
  if (activeSrc !== failedSrc || typeof nextSrc !== 'string' || !nextSrc.trim()) {
    return { kind: 'failed' }
  }
  if (nextSrc === failedSrc) return { kind: 'reload' }
  return { kind: 'replace', src: nextSrc }
}

export function mediaRefreshDelay(expiresAt: string | undefined, nowMs = Date.now(), marginMs = 30_000) {
  if (!expiresAt) return null
  const expiresAtMs = Date.parse(expiresAt)
  if (!Number.isFinite(expiresAtMs)) return null
  return Math.max(0, expiresAtMs - nowMs - marginMs)
}

export function temporaryMediaExpiryFromURL(value: string | undefined) {
  if (!value) return undefined
  try {
    const target = new URL(value)
    const match = target.searchParams.get('X-Amz-Date')?.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/)
    const expiresSeconds = Number(target.searchParams.get('X-Amz-Expires'))
    if (!match || !Number.isFinite(expiresSeconds) || expiresSeconds <= 0) return undefined
    const signedAt = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]), Number(match[6]))
    return new Date(signedAt + expiresSeconds * 1000).toISOString()
  } catch {
    return undefined
  }
}
