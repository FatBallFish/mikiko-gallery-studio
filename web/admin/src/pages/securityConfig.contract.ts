import { API_PATHS } from '../../../shared/api-types'
import { protectedRoutes } from '../layout/admin-navigation'
import { ADMIN_ROUTE_PERMISSION_MAP } from '../types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const securitySource = readFileSync(new URL('./SecurityConfigPage.tsx', import.meta.url), 'utf8')

if (API_PATHS.ops.securitySMTP !== '/api/ops/admin/v1/security/smtp') {
  throw new Error(`security smtp API path should be stable, got ${API_PATHS.ops.securitySMTP}`)
}

if (API_PATHS.ops.securitySMTPTest !== '/api/ops/admin/v1/security/smtp/test') {
  throw new Error(`security smtp test API path should be stable, got ${API_PATHS.ops.securitySMTPTest}`)
}

if (!protectedRoutes.includes('system-settings')) {
  throw new Error('system settings route should be protected and navigable')
}

if (ADMIN_ROUTE_PERMISSION_MAP['system-settings'] !== 'manage:config') {
  throw new Error(`system settings route should require manage:config, got ${ADMIN_ROUTE_PERMISSION_MAP['system-settings']}`)
}

if (!securitySource.includes('export function SecurityConfigPage')) {
  throw new Error('SecurityConfigPage should be exported as a React page component')
}

for (const phase of ['pristine', 'dirty', 'validating', 'saving', 'saved', 'failed']) {
  if (!securitySource.includes(`'${phase}'`)) {
    throw new Error(`security configuration editor must expose ${phase} state`)
  }
}

for (const dirtyContract of [
  'onDirtyChange?: (dirty: boolean) => void',
  'onDirtyChange?.(dirty)',
  "window.addEventListener('beforeunload'",
  "window.removeEventListener('beforeunload'",
  'event.preventDefault()',
  "window.confirm('当前 SMTP 配置有未保存修改，确认放弃并刷新吗？')",
  'onBusyChange?: (busy: boolean) => void',
  'onBusyChange?.(editorLocked)',
]) {
  if (!securitySource.includes(dirtyContract)) {
    throw new Error(`security configuration must protect dirty edits: ${dirtyContract}`)
  }
}

for (const editorContract of [
  'data-security-editor-group="connection"',
  'data-security-editor-group="delivery"',
  'data-security-editor-group="secret"',
  'sticky bottom-0',
  'validateSMTPDraft',
  'error={fieldErrors.host}',
  'error={fieldErrors.port}',
  'error={fieldErrors.from}',
  '<InlineFeedback tone="danger"',
  'if (error && !config)',
  'const editorLocked = saving || testing',
  'disabled={editorLocked}',
  'aria-busy={editorLocked}',
]) {
  if (!securitySource.includes(editorContract)) {
    throw new Error(`security configuration must use grouped local feedback: ${editorContract}`)
  }
}

for (const apiContract of ['adminApi.getSMTPConfig', 'adminApi.updateSMTPConfig', 'adminApi.testSMTPConfig', 'smtpPayloadFromDraft(draft, config)']) {
  if (!securitySource.includes(apiContract)) {
    throw new Error(`security redesign must preserve ${apiContract}`)
  }
}

for (const tlsGuidance of ['465 端口会自动使用隐式 TLS', '587 端口通常需要启用 STARTTLS']) {
  if (!securitySource.includes(tlsGuidance)) {
    throw new Error(`security configuration must explain SMTP TLS modes: ${tlsGuidance}`)
  }
}

for (const visualDrift of ['rounded-2xl', 'rounded-3xl', 'shadow-lg', 'tracking-[']) {
  if (securitySource.includes(visualDrift)) {
    throw new Error(`security configuration must remove visual drift: ${visualDrift}`)
  }
}
