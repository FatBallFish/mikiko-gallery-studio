import { useLayoutEffect, useRef, useState } from 'react'

/** Keep a notification's remaining lifetime across renders and reading pauses. */
export function useToastLifetime(durationMs: number, onExpire: () => void) {
  const expireRef = useRef(onExpire)
  expireRef.current = onExpire
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [hidden, setHidden] = useState(() => document.hidden)
  const remaining = useRef(durationMs)
  const paused = hovered || focused || hidden

  useLayoutEffect(() => {
    const update = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', update)
    return () => document.removeEventListener('visibilitychange', update)
  }, [])

  useLayoutEffect(() => {
    remaining.current = durationMs
  }, [durationMs])

  useLayoutEffect(() => {
    if (paused) return
    const started = Date.now()
    const timer = window.setTimeout(() => expireRef.current(), remaining.current)
    return () => {
      window.clearTimeout(timer)
      remaining.current = Math.max(0, remaining.current - (Date.now() - started))
    }
  }, [durationMs, paused])

  return {
    onMouseEnter: () => setHovered(true),
    onMouseLeave: () => setHovered(false),
    onFocusCapture: () => setFocused(true),
    onBlurCapture: (event: React.FocusEvent<HTMLElement>) => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setFocused(false)
    },
  }
}
