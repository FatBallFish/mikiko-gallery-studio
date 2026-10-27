import type { ApiKey } from '../../../shared/api-types'

export type ApiKeyCredentials = { accessKey: string; secret: string }

export function apiKeyCredentials(key: ApiKey): ApiKeyCredentials | null {
  return key.secret_preview ? { accessKey: key.access_key, secret: key.secret_preview } : null
}

function csvCell(value: string): string {
  // Prefix formula-like input defensively; normal server-generated ak-/sk- values are unchanged.
  const safe = /^[\s\uFEFF]*[=+\-@]/.test(value) ? `'${value}` : value
  return `"${safe.replace(/"/g, '""')}"`
}

export function apiKeyCredentialsCsv(credentials: ApiKeyCredentials): string {
  return `\uFEFFAK,SK\r\n${csvCell(credentials.accessKey)},${csvCell(credentials.secret)}\r\n`
}

export function confirmCredentialSave(saved: boolean, onConfirm: () => void): void {
  if (saved) onConfirm()
}

export function downloadApiKeyCredentials(credentials: ApiKeyCredentials): () => void {
  const url = URL.createObjectURL(new Blob([apiKeyCredentialsCsv(credentials)], { type: 'text/csv;charset=utf-8' }))
  const anchor = document.createElement('a')
  let timer: ReturnType<typeof setTimeout> | undefined
  let revoked = false
  const cleanup = () => {
    if (timer !== undefined) clearTimeout(timer)
    if (!revoked) URL.revokeObjectURL(url)
    revoked = true
  }
  try {
    anchor.href = url
    anchor.download = 'api-key-credentials.csv'
    anchor.hidden = true
    document.body.appendChild(anchor)
    anchor.click()
    // Let the browser start the local download before releasing the URL.
    timer = setTimeout(cleanup, 1000)
  } catch (error) {
    cleanup()
    throw error
  } finally {
    anchor.remove()
  }
  return cleanup
}
