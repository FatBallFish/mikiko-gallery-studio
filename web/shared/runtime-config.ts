export type RuntimeConfig = {
  apiBaseUrl?: string
  apiPort?: string
  directFrontendPort?: string
}

declare global {
  interface Window {
    __PIC_GALLERY_CONFIG__?: RuntimeConfig
  }
}

function parseBrowserURL(value: string) {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    throw new TypeError('Browser URL is invalid')
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new TypeError('Browser URL must use HTTP or HTTPS')
  }
  return parsed
}

function normalizeExplicitAPIBase(value: string, browser: URL) {
  const base = value.trim()
  if (!base) return ''
  if (base.startsWith('//')) throw new TypeError('Runtime API URL must not be protocol-relative')
  if (base.startsWith('/')) return base.replace(/\/+$/, '')

  let parsed: URL
  try {
    parsed = new URL(base, browser.origin)
  } catch {
    throw new TypeError('Runtime API URL is invalid')
  }
  if ((parsed.protocol !== 'http:' && parsed.protocol !== 'https:') || parsed.username || parsed.password) {
    throw new TypeError('Runtime API URL must use HTTP or HTTPS without credentials')
  }
  return parsed.href.replace(/\/+$/, '')
}

function normalizePort(value: string | undefined, name: string) {
  const port = value?.trim() ?? ''
  if (!port) return ''
  const numeric = Number(port)
  if (!/^\d+$/.test(port) || numeric < 1 || numeric > 65535) {
    throw new TypeError(`${name} must be a valid TCP port`)
  }
  return String(numeric)
}

export function resolveRuntimeAPIBase(config: RuntimeConfig, browserURL: string) {
  const browser = parseBrowserURL(browserURL)
  const explicit = normalizeExplicitAPIBase(config.apiBaseUrl ?? '', browser)
  if (explicit) return explicit

  const apiPort = normalizePort(config.apiPort, 'API port')
  const directFrontendPort = normalizePort(config.directFrontendPort, 'Direct frontend port')
  if (apiPort && directFrontendPort && browser.port === directFrontendPort) {
    const api = new URL(browser.origin)
    api.port = apiPort
    return api.origin
  }
  return ''
}

export function getRuntimeConfig(): RuntimeConfig {
  return globalThis.window?.__PIC_GALLERY_CONFIG__ ?? {}
}
