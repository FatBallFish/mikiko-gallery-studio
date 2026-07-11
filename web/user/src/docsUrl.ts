export const localDocsUrl = 'http://localhost:5175/'

export type DocsUrlEnv = {
  readonly [key: string]: unknown
  VITE_DOCS_URL?: unknown
}

type DocsWindowOpen = (url?: string | URL, target?: string, features?: string) => WindowProxy | null

export const docsEntryPoints = ['home', 'api-keys', 'account-menu', 'footer', 'legacy-route'] as const
export type DocsEntryPoint = typeof docsEntryPoints[number]

export type OpenDocsOptions = {
  runtimeEnv?: DocsUrlEnv
  buildEnv?: DocsUrlEnv
  open?: DocsWindowOpen
}

export function docsUrl(env: DocsUrlEnv): string {
  const configured = env.VITE_DOCS_URL
  return typeof configured === 'string' && configured.trim() ? configured : localDocsUrl
}

export function resolveDocsUrl(runtimeEnv?: DocsUrlEnv, buildEnv?: DocsUrlEnv): string {
  const runtimeUrl = runtimeEnv?.VITE_DOCS_URL
  if (typeof runtimeUrl === 'string' && runtimeUrl.trim()) return runtimeUrl
  return docsUrl(buildEnv ?? {})
}

export function docsSiteUrl(): string {
  return resolveDocsUrl(globalThis.window?.__PIC_GALLERY_ENV__, import.meta.env)
}

export function openDocsSite(options: OpenDocsOptions = {}): WindowProxy | null {
  const url = resolveDocsUrl(
    options.runtimeEnv ?? globalThis.window?.__PIC_GALLERY_ENV__,
    options.buildEnv ?? import.meta.env,
  )
  const open = options.open ?? globalThis.window?.open.bind(globalThis.window)
  return open?.(url, '_blank', 'noopener,noreferrer') ?? null
}

export function openDocsEntry(_entryPoint: DocsEntryPoint, options: OpenDocsOptions = {}): WindowProxy | null {
  return openDocsSite(options)
}
