import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { apiKeyCredentials, apiKeyCredentialsCsv, confirmCredentialSave, downloadApiKeyCredentials } from './apiKeyCredentials'
import type { ApiKey } from '../../../shared/api-types'

const credentials = { accessKey: 'ak-contract-example', secret: 'sk-contract-example' }
assert.deepEqual(apiKeyCredentials({ access_key: credentials.accessKey, secret_preview: credentials.secret } as ApiKey), credentials)
assert.equal(apiKeyCredentials({ access_key: credentials.accessKey } as ApiKey), null)
assert.equal(apiKeyCredentialsCsv(credentials), '\uFEFFAK,SK\r\n"ak-contract-example","sk-contract-example"\r\n')
assert.equal(apiKeyCredentialsCsv({ accessKey: 'ak-a,"b', secret: 'sk-a\r\nb' }), '\uFEFFAK,SK\r\n"ak-a,""b","sk-a\r\nb"\r\n')
for (const value of ['=CMD()', '+CMD()', '-CMD()', '@CMD()', ' \t=CMD()']) {
  assert.ok(apiKeyCredentialsCsv({ accessKey: value, secret: 'sk-safe' }).includes(`"'${value}"`))
}
let closed = 0
confirmCredentialSave(false, () => closed++)
assert.equal(closed, 0)
confirmCredentialSave(true, () => closed++)
assert.equal(closed, 1)

const originalDocument = globalThis.document
const createObjectURL = URL.createObjectURL
const revokeObjectURL = URL.revokeObjectURL
let downloadedBlob: Blob | undefined
let revoked = 0
let removed = 0
let clicked = 0
let failClick = false
const anchor = { href: '', download: '', hidden: false, click() { clicked++; if (failClick) throw new Error('download failed') }, remove() { removed++ } }
try {
  Object.defineProperty(globalThis, 'document', { configurable: true, value: { createElement: (tag: string) => { assert.equal(tag, 'a'); return anchor }, body: { appendChild: (node: unknown) => assert.equal(node, anchor) } } })
  URL.createObjectURL = (blob) => { downloadedBlob = blob as Blob; return 'blob:contract-download' }
  URL.revokeObjectURL = (url) => { assert.equal(url, 'blob:contract-download'); revoked++ }
  const cleanup = downloadApiKeyCredentials(credentials)
  assert.equal(clicked, 1)
  assert.equal(removed, 1)
  assert.equal(anchor.download, 'api-key-credentials.csv')
  assert.equal(anchor.href, 'blob:contract-download')
  assert.equal(downloadedBlob?.type, 'text/csv;charset=utf-8')
  assert.deepEqual(Array.from(new Uint8Array(await downloadedBlob!.arrayBuffer()).slice(0, 3)), [239, 187, 191])
  cleanup()
  cleanup()
  assert.equal(revoked, 1, 'cleanup must be idempotent')
  failClick = true
  assert.throws(() => downloadApiKeyCredentials(credentials))
  assert.equal(revoked, 2, 'download errors release the object URL')
  assert.equal(removed, 2, 'download errors remove the anchor')
} finally {
  Object.defineProperty(globalThis, 'document', { configurable: true, value: originalDocument })
  URL.createObjectURL = createObjectURL
  URL.revokeObjectURL = revokeObjectURL
}

const page = readFileSync(new URL('./ApiKeysPage.tsx', import.meta.url), 'utf8')
const modal = readFileSync(new URL('./ApiKeyCredentialsModal.tsx', import.meta.url), 'utf8')
const components = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
const css = readFileSync(new URL('./api-key-credentials.css', import.meta.url), 'utf8')
assert.ok(!page.includes('oneTimeSecret'))
assert.match(page, /setCredentials\(apiKeyCredentials\(updated\)\)/)
assert.match(page, /setCreating\(false\)[\s\S]*?setCredentials\(apiKeyCredentials\(key\)\)[\s\S]*?await keys.reload/)
assert.match(page, /credentials \? <ApiKeyCredentialsModal credentials=\{credentials\} onConfirm=\{\(\) => setCredentials\(null\)\}/)
assert.match(modal, /useState\(false\)/)
assert.match(modal, /dismissible=\{false\}/)
assert.match(modal, /onClose=\{\(\) => \{\}\}/)
assert.match(modal, /disabled=\{!saved\}[\s\S]*?confirmCredentialSave\(saved, onConfirm\)/)
assert.match(modal, /我已妥善保存/)
assert.match(modal, /aria-label=\{`复制 \$\{field.label\}`\}/)
assert.match(modal, /downloads.current.forEach\(\(cleanup\) => cleanup\(\)\)/)
assert.equal((modal.match(/setSaved\(/g) ?? []).length, 1, 'only the checkbox can change acknowledgement')
for (const forbidden of ['localStorage', 'sessionStorage', 'console.', 'errorMessage(']) assert.ok(!modal.includes(forbidden))
const modalSource = components.slice(components.indexOf('export function Modal('), components.indexOf('export function Drawer('))
assert.match(modalSource, /dismissible = true/)
assert.match(modalSource, /const dismiss = \(\) => \{ if \(dismissible\) onClose\(\) \}/)
assert.match(modalSource, /useDismissableLayer\(true, dismiss, dialogRef\)/)
assert.match(modalSource, /data-focus-layer onMouseDown=\{dismiss\}/)
assert.match(modalSource, /dismissible \? <IconButton/)
assert.match(components, /window.addEventListener\('keydown', handleKeyDown, true\)/)
assert.match(components, /event.key === 'Escape'[\s\S]*?event.preventDefault\(\)[\s\S]*?event.stopPropagation\(\)[\s\S]*?onCloseRef.current\(\)/)
assert.match(css, /overflow-wrap: anywhere/)
assert.match(css, /overflow-y: auto/)
assert.match(css, /@media \(max-width: 600px\)/)
console.log('OK: one-time credential mapping, acknowledgement guards, modal dismissal, CSV bytes/escaping and download cleanup')
