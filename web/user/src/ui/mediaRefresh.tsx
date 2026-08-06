import { useCallback, useEffect, useRef, useState } from 'react'
import type { ImgHTMLAttributes } from 'react'
import { isAbsoluteHTTPMediaURL } from '../../../shared/media-url'
import { mediaRefreshDelay, mediaRefreshRetry, temporaryMediaExpiryFromURL } from './mediaRefreshState'

export type MediaRefreshHandler = () => string | undefined | void | Promise<string | undefined | void>

export function useMediaRefreshOnce(src: string | undefined, onMediaRefresh?: MediaRefreshHandler, expiresAt?: string, proactiveRefresh = false) {
  const [currentSrc, setCurrentSrc] = useState(src)
  const [currentExpiresAt, setCurrentExpiresAt] = useState(expiresAt ?? temporaryMediaExpiryFromURL(src))
  const [retryRevision, setRetryRevision] = useState(0)
  const attemptedRef = useRef(false)
  const currentSrcRef = useRef(currentSrc)
  const refreshRef = useRef(onMediaRefresh)
  currentSrcRef.current = currentSrc
  refreshRef.current = onMediaRefresh

  useEffect(() => {
    setCurrentSrc(src)
    setCurrentExpiresAt(expiresAt ?? temporaryMediaExpiryFromURL(src))
    setRetryRevision(0)
    attemptedRef.current = false
  }, [expiresAt, src])

  const markMediaLoaded = useCallback(() => {
    attemptedRef.current = false
  }, [])

  const resetMediaRefresh = useCallback(() => {
    attemptedRef.current = false
  }, [])

  const refreshFailedMedia = useCallback(async () => {
    const failedSrc = currentSrcRef.current
    const refresh = refreshRef.current
    if (attemptedRef.current || !failedSrc || !isAbsoluteHTTPMediaURL(failedSrc) || !refresh) return false
    attemptedRef.current = true
    try {
      const nextSrc = await Promise.resolve(refresh())
      const retry = mediaRefreshRetry(failedSrc, nextSrc, currentSrcRef.current)
      if (retry.kind === 'replace') {
        setCurrentSrc(retry.src)
        setCurrentExpiresAt(temporaryMediaExpiryFromURL(retry.src))
      } else if (retry.kind === 'reload') {
        setRetryRevision((current) => current + 1)
      } else {
        attemptedRef.current = false
        return false
      }
      return true
    } catch {
      attemptedRef.current = false
      return false
    }
  }, [])

  useEffect(() => {
    if (!proactiveRefresh || currentSrc !== src || !refreshRef.current || !currentSrc || !isAbsoluteHTTPMediaURL(currentSrc)) return undefined
    const delay = mediaRefreshDelay(currentExpiresAt)
    if (delay === null) return undefined
    const timer = window.setTimeout(() => { void refreshFailedMedia() }, delay)
    return () => window.clearTimeout(timer)
  }, [currentExpiresAt, currentSrc, expiresAt, proactiveRefresh, refreshFailedMedia, src])

  return { currentSrc, mediaRetryKey: retryRevision, markMediaLoaded, refreshFailedMedia, resetMediaRefresh }
}

export function RefreshableMediaImage({ src, mediaExpiresAt, onMediaRefresh, onLoad, onError, ...props }: ImgHTMLAttributes<HTMLImageElement> & {
  mediaExpiresAt?: string
  onMediaRefresh?: MediaRefreshHandler
}) {
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia } = useMediaRefreshOnce(src, onMediaRefresh, mediaExpiresAt)
  return (
    <img
      key={mediaRetryKey}
      {...props}
      src={currentSrc}
      onLoad={(event) => {
        markMediaLoaded()
        onLoad?.(event)
      }}
      onError={(event) => {
        void refreshFailedMedia().then((refreshed) => {
          if (!refreshed) onError?.(event)
        })
      }}
    />
  )
}
