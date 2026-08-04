import { useCallback, useRef } from 'react'
import type { ImgHTMLAttributes } from 'react'
import { isAbsoluteHTTPMediaURL } from '../../../shared/media-url'

export function useMediaRefreshOnce(src: string | undefined, onMediaRefresh?: () => void | Promise<void>) {
  const attemptedRef = useRef(false)
  const refreshRef = useRef(onMediaRefresh)
  refreshRef.current = onMediaRefresh

  const markMediaLoaded = useCallback(() => {
    attemptedRef.current = false
  }, [])

  const refreshFailedMedia = useCallback(() => {
    if (attemptedRef.current || !src || !isAbsoluteHTTPMediaURL(src)) return false
    attemptedRef.current = true
    const refresh = refreshRef.current
    if (refresh) void Promise.resolve(refresh()).catch(() => undefined)
    return Boolean(refresh)
  }, [src])

  return { markMediaLoaded, refreshFailedMedia }
}

export function RefreshableMediaImage({ src, onMediaRefresh, onLoad, onError, ...props }: ImgHTMLAttributes<HTMLImageElement> & {
  onMediaRefresh?: () => void | Promise<void>
}) {
  const { markMediaLoaded, refreshFailedMedia } = useMediaRefreshOnce(src, onMediaRefresh)
  return (
    <img
      {...props}
      src={src}
      onLoad={(event) => {
        markMediaLoaded()
        onLoad?.(event)
      }}
      onError={(event) => {
        refreshFailedMedia()
        onError?.(event)
      }}
    />
  )
}
