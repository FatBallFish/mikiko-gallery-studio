// @ts-ignore contract scripts run in Node without project Node typings.
import { readdirSync, readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in Node without project Node typings.
import { join } from 'node:path'
// @ts-ignore contract scripts run in Node without project Node typings.
import { fileURLToPath } from 'node:url'

const sharedRoot = fileURLToPath(new URL('.', import.meta.url))
const repositoryRoot = fileURLToPath(new URL('../..', import.meta.url))
const canonicalTokens = readFileSync(join(sharedRoot, 'admin-design-tokens.css'), 'utf8')
const adminStyles = readFileSync(join(repositoryRoot, 'web/admin/src/styles.css'), 'utf8')
const setupSources = readdirSync(join(repositoryRoot, 'internal/http/setupui'))
  .filter((name: string) => name.endsWith('.go') && !name.endsWith('_test.go'))
  .map((name: string) => readFileSync(join(repositoryRoot, 'internal/http/setupui', name), 'utf8'))
  .join('\n')

if (!adminStyles.startsWith("@import 'tailwindcss';\n@import '../../shared/base.css';\n@import '../../shared/admin-design-tokens.css';")) {
  throw new Error('admin stylesheet must import the canonical shared admin design tokens')
}

for (const token of [
  '--admin-font-ui:', '--admin-font-mono:', '--admin-type-label: 11px', '--admin-type-body: 14px',
  '--admin-type-page: 24px', '--admin-motion-fast: 120ms', '--bg:', '--surface:', '--fg:', '--border:',
  '--accent:', '--green:', '--amber:', '--red:', '--pg-radius-xs: 6px', '--pg-radius-sm: 8px',
  '--pg-radius-md: 12px', '--pg-topbar-height: 64px', '--pg-sidebar-admin-width: 216px',
]) {
  if (!canonicalTokens.includes(token)) throw new Error(`canonical admin tokens missing ${token}`)
}

if (!setupSources.includes('Code generated from web/shared/admin-design-tokens.css; DO NOT EDIT.')) {
  throw new Error('setup UI must embed a generated artifact from the canonical admin token source')
}

const generatedMatch = setupSources.match(/const adminDesignTokensCSS = `([\s\S]*?)`/)
if (!generatedMatch || `${generatedMatch[1]}\n` !== canonicalTokens) {
  throw new Error('embedded setup admin design tokens have drifted from the canonical shared source')
}

for (const endpoint of [
  '/api/setup/v1/session', '/api/setup/v1/probes/database', '/api/setup/v1/probes/redis',
  '/api/setup/v1/probes/storage', '/api/setup/v1/apply', '/api/setup/v1/progress/',
  '/api/system/v1/bootstrap-status',
]) {
  if (!setupSources.includes(endpoint)) throw new Error(`embedded setup UI missing relative endpoint ${endpoint}`)
}

for (const required of [
  'deployctl setup token show', 'deployctl setup token reset', 'history.back()', 'history.length',
  'document.referrer', 'function returnURLFromHash()', 'decodeURIComponent', 'location.assign(returnURL)',
  'aria-live="polite"', 'role="status"', '<progress', ':focus-visible',
  'aria-label="初始化进度 / Setup progress"',
  '@media (max-width: 720px)', '@media (prefers-reduced-motion: reduce)', 'overflow-x: hidden',
  'credentials: \'same-origin\'', 'crypto.randomUUID()',
  'crypto.getRandomValues', 'function createOperationID()',
  'function resetAuthentication(code) {\n    clearSecretInputs();',
  "const pendingApply = requestJSON('/api/setup/v1/apply', { method: 'POST', body, timeout: 300000 });\n      clearApplyPayload(body);\n      const view = await pendingApply;",
  'function clearApplyPayload(body) {',
  'for (let attempt = 0; attempt < 60; attempt += 1)',
  'const readinessDeadline = Date.now() + 120000;',
  'new AbortController()', 'controller.abort()',
  'const timeout = options.timeout || 15000;',
  'if (controller.signal.aborted) throw error;',
  "if (controller.signal.aborted) throw { code: 'SETUP_REQUEST_TIMEOUT' };",
  "if (typeof error?.code === 'string') throw error;",
  "requestJSON('/api/setup/v1/apply', { method: 'POST', body, timeout: 300000 })",
  'deployctl status', 'deployctl logs', 'deployctl doctor', 'deployctl restart',
  'function invalidateProbe(kind) {',
  'probeVersions[kind] += 1;',
  'if (version !== probeVersions[kind]) return;',
  "if (field && ['database', 'redis', 'storage'].includes(field.group)) invalidateProbe(field.group);",
	  'async function recoverApplyOperation() {',
	  'operationId = session?.operation_id || operationId;',
  'let preserveOperationID = false;',
  'preserveOperationID = true;',
  'if (!applying && !preserveOperationID) operationId = \'\';',
  'const recoveryDeadline = Date.now() + 300000;',
  "bootstrap?.phase === 'setup_required' && error.code === 'SETUP_OPERATION_NOT_FOUND'",
  "error.code === 'SETUP_NETWORK_ERROR' || error.code === 'SETUP_REQUEST_TIMEOUT' || error.code === 'SETUP_REQUEST_CANCELLED'",
  'await recoverApplyOperation();',
  'id="workspace" tabindex="-1" hidden',
  'function focusSetupWorkspace() {',
  'input.offsetParent !== null',
]) {
  if (!setupSources.includes(required)) throw new Error(`embedded setup UI missing contract ${required}`)
}

if (setupSources.includes("bootstrap?.phase === 'setup_required' && error.code === 'SETUP_OPERATION_NOT_FOUND') {\n          operationId = '';")) {
  throw new Error('a durable setup attempt must retain its original operation id after restart')
}

if (!setupSources.includes("error.code === 'SETUP_SESSION_INVALID') {\n        applying = false;\n        applyButton.disabled = false;\n        preserveOperationID = true;")) {
  throw new Error('apply session expiry must unlock the form while preserving the operation id')
}

if (setupSources.includes('async function pollReadiness() {\n    while (true)')) {
  throw new Error('setup readiness polling must stop at a bounded recovery state')
}

if (setupSources.includes('while (operationId)')) {
  throw new Error('setup operation recovery must be bounded and phase-aware')
}

if (setupSources.includes('const controller = options.timeout ?')) {
  throw new Error('all setup requests must have a bounded default timeout')
}

if (setupSources.includes('if (error?.code) throw error;')) {
  throw new Error('numeric DOMException codes must not escape as setup business errors')
}

for (const forbidden of [
  'localStorage', 'sessionStorage', 'URLSearchParams', 'location.search', 'unsafe-inline', 'unsafe-eval',
  'fonts.googleapis.com', '<script src=', '<link rel="stylesheet"', 'window.location.host',
]) {
  if (setupSources.includes(forbidden)) throw new Error(`embedded setup UI contains forbidden ${forbidden}`)
}

if (!setupSources.includes('DescriptionZH') || !setupSources.includes('DescriptionEN') || !setupSources.includes('FieldOwnerSetup')) {
  throw new Error('setup field descriptions must derive from the sanitized runtime schema projection')
}
