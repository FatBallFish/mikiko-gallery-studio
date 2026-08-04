import { getDefaultBaseUrl, withQuery } from './http-client'

export function isAbsoluteHTTPMediaURL(value: string) {
  try {
    const parsed = new URL(value)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && Boolean(parsed.host) && !parsed.username && !parsed.password
  } catch {
    return false
  }
}

export function mediaAssetURL(path: string, accessToken?: string | null, baseURL = getDefaultBaseUrl() || globalThis.location?.origin || '') {
  if (!path) return ''
  if (isAbsoluteHTTPMediaURL(path)) return path
  return `${baseURL}${withQuery(path, { access_token: accessToken })}`
}
