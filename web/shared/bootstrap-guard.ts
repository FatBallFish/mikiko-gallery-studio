import type { BootstrapStatus } from './api-types'
import { fetchBootstrapStatus, setupURLWithReturnTarget } from './bootstrap-status'

type GuardBootstrapOptions = {
  apiBaseUrl: string
  frontendOrigin: string
  signal?: AbortSignal
}

export async function guardBootstrap(options: GuardBootstrapOptions): Promise<BootstrapStatus> {
  const status = await fetchBootstrapStatus({
    apiBaseUrl: options.apiBaseUrl,
    frontendOrigin: options.frontendOrigin,
    credentials: 'omit',
    signal: options.signal,
  })
  if (status.phase === 'setup_required' || status.phase === 'initializing' || status.phase === 'restart_pending') {
    window.location.assign(setupURLWithReturnTarget(status.setup_url, window.location.href))
  }
  return status
}
