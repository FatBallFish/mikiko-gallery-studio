import { useCallback, useEffect, useState } from 'react'
import type { BootstrapStatus } from '../../shared/api-types'
import { guardBootstrap } from '../../shared/bootstrap-guard'
import { getDefaultBaseUrl } from '../../shared/http-client'

export type BootstrapGuardState =
  | { phase: 'checking'; retry: () => void }
  | { phase: 'error'; retry: () => void }
  | (BootstrapStatus & { retry: () => void })

export function useBootstrapGuard(): BootstrapGuardState {
  const [attempt, setAttempt] = useState(0)
  const [status, setStatus] = useState<Omit<BootstrapGuardState, 'retry'>>({ phase: 'checking' })
  const retry = useCallback(() => {
    setStatus({ phase: 'checking' })
    setAttempt((value) => value + 1)
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void guardBootstrap({
      apiBaseUrl: getDefaultBaseUrl(),
      frontendOrigin: window.location.origin,
      signal: controller.signal,
    }).then((nextStatus) => {
      if (!controller.signal.aborted) setStatus(nextStatus)
    }).catch(() => {
      if (!controller.signal.aborted) setStatus({ phase: 'error' })
    })
    return () => controller.abort()
  }, [attempt])

  return { ...status, retry } as BootstrapGuardState
}
