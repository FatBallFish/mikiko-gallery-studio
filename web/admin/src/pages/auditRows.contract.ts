import type { AuditLog } from '../../../shared/api-types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import {
  auditActionLabel,
  auditActionOptions,
  auditDetailText,
  auditResultBadge,
  auditRowsCSV,
  auditExportFilename,
  auditTimelineRow,
  auditSearchPlaceholder,
  auditSearchText,
  auditSubjectLabel,
} from './auditRows'

const auditPageSource = readFileSync(new URL('./AuditPage.tsx', import.meta.url), 'utf8')

for (const primitive of ['PageHeader', 'FilterToolbar', 'ListPage', 'DataTable', 'Badge']) {
  if (!auditPageSource.includes(`<${primitive}`)) {
    throw new Error(`audit list must use the shared ${primitive} primitive`)
  }
}

for (const behavior of ['adminApi.listAudit', 'auditRowsCSV(visibleRows)', 'auditExportFilename()', 'resultSummary=']) {
  if (!auditPageSource.includes(behavior)) {
    throw new Error(`audit list must preserve ${behavior}`)
  }
}

for (const legacyPattern of ['AuditTimelineItem', '<FilterBar', 'rounded-xl', 'tracking-widest', 'uppercase']) {
  if (auditPageSource.includes(legacyPattern)) {
    throw new Error(`audit list must remove legacy page-local pattern ${legacyPattern}`)
  }
}

const knownAction = auditActionLabel('user.points_adjust')
if (knownAction !== '调整用户积分') {
  throw new Error(`known audit actions should be localized, got ${knownAction}`)
}

const unknownAction = auditActionLabel('custom.future_action')
if (unknownAction !== 'custom.future_action') {
  throw new Error(`unknown audit actions should preserve raw values, got ${unknownAction}`)
}

if (auditActionLabel('   ') !== '未知动作') {
  throw new Error('empty audit actions should show a clear fallback')
}

const actionOptions = auditActionOptions([
  audit({ action: 'user.points_adjust' }),
  audit({ action: 'custom.future_action' }),
  audit({ action: 'user.points_adjust' }),
])
const optionValues = actionOptions.map((option) => option.value).join(',')
const optionLabels = actionOptions.map((option) => option.label).join(',')
if (!optionValues.startsWith('all,') || !optionValues.includes('user.points_adjust') || !optionValues.includes('custom.future_action')) {
  throw new Error(`audit action options should preserve raw values, got ${optionValues}`)
}
if (!optionLabels.includes('全部动作') || !optionLabels.includes('调整用户积分') || !optionLabels.includes('custom.future_action')) {
  throw new Error(`audit action options should expose operator-facing labels, got ${optionLabels}`)
}

const success = auditResultBadge('success')
const failure = auditResultBadge('failure')
const rejected = auditResultBadge('rejected')
const custom = auditResultBadge('timeout')
if (success.label !== '成功' || success.tone !== 'success' || failure.label !== '失败' || failure.tone !== 'danger' || rejected.label !== '已拒绝' || rejected.tone !== 'warning') {
  throw new Error(`audit result badges should be localized, got ${JSON.stringify({ success, failure, rejected })}`)
}
if (custom.label !== 'timeout' || custom.tone !== 'neutral') {
  throw new Error(`unknown audit results should preserve raw value, got ${JSON.stringify(custom)}`)
}

const rich = audit({
  actor: 'admin:7',
  actor_type: 'admin',
  actor_id: '7',
  target: 'user:42',
  target_type: 'user',
  target_id: '42',
  result: 'success',
  ip_addr: '127.0.0.1',
  user_agent: 'Mozilla/5.0',
  detail: 'success',
  metadata: { before: '10.00000', after: '20.00000', reason: 'manual compensation' },
})
const subject = auditSubjectLabel(rich)
if (subject.actor !== '管理员 7' || subject.target !== '用户 42') {
  throw new Error(`audit subject labels should avoid raw debug prefixes, got ${JSON.stringify(subject)}`)
}

const detail = auditDetailText(rich)
if (detail.includes('actor=') || detail.includes('target=')) {
  throw new Error(`audit detail should not expose debug key-value text, got ${detail}`)
}
if (!detail.includes('IP 127.0.0.1') || !detail.includes('before: 10.00000') || !detail.includes('reason: manual compensation')) {
  throw new Error(`audit detail should summarize source and metadata, got ${detail}`)
}

const searchText = auditSearchText(rich)
for (const expected of ['admin:7', '管理员 7', 'user.points_adjust', '调整用户积分', 'user:42', '用户 42', 'manual compensation']) {
  if (!searchText.includes(expected.toLowerCase())) {
    throw new Error(`audit search index should include ${expected}, got ${searchText}`)
  }
}

if (!auditSearchPlaceholder.includes('操作人') || !auditSearchPlaceholder.includes('对象') || !auditSearchPlaceholder.includes('详情')) {
  throw new Error(`audit search placeholder should be operator-facing, got ${auditSearchPlaceholder}`)
}

const timeline = auditTimelineRow(rich)
if (timeline.createdAtLabel !== '2026/06/05 00:00') {
  throw new Error(`audit timeline should format created_at for operators, got ${timeline.createdAtLabel}`)
}
if (timeline.actionLabel !== '调整用户积分' || timeline.targetLabel !== '用户 42' || timeline.actorLabel !== '管理员 7') {
  throw new Error(`audit timeline should expose localized labels, got ${JSON.stringify(timeline)}`)
}
if (timeline.result.label !== '成功' || timeline.result.tone !== 'success') {
  throw new Error(`audit timeline should expose result badge model, got ${JSON.stringify(timeline.result)}`)
}
if (!timeline.detailText.includes('manual compensation') || timeline.raw.id !== rich.id) {
  throw new Error(`audit timeline should preserve detail text and raw row, got ${JSON.stringify(timeline)}`)
}
if (/T|Z$/.test(timeline.createdAtLabel)) {
  throw new Error(`audit timeline should not expose ISO separators, got ${timeline.createdAtLabel}`)
}

const invalidDateTimeline = auditTimelineRow(audit({ created_at: 'not-a-date' }))
if (invalidDateTimeline.createdAtLabel !== 'not-a-date') {
  throw new Error(`audit timeline should preserve invalid dates for troubleshooting, got ${invalidDateTimeline.createdAtLabel}`)
}

const systemTimeline = auditTimelineRow(audit({ actor_type: 'system', actor_id: 'worker', actor: 'system' }))
if (systemTimeline.actorTone !== 'neutral') {
  throw new Error(`system audit rows should use neutral actor tone, got ${systemTimeline.actorTone}`)
}

const csv = auditRowsCSV([
  rich,
  audit({
    id: 'audit-2',
    action: 'custom.future_action',
    actor_type: 'system',
    actor_id: 'worker,1',
    target_type: 'config',
    target_id: 'auth"security',
    result: 'failure',
    detail: 'first line\nsecond line',
    metadata: { reason: 'operator "rollback"', attempts: 2 },
  }),
])
const csvLines = csv.split('\n')
if (csvLines[0] !== '时间,动作,操作人,对象,结果,详情,审计ID') {
  throw new Error(`audit export should include operator-facing CSV headers, got ${csvLines[0]}`)
}
if (!csv.includes('2026/06/05 00:00,调整用户积分,管理员 7,用户 42,成功')) {
  throw new Error(`audit export should use localized row labels, got ${csv}`)
}
if (!csv.includes('custom.future_action')) {
  throw new Error(`audit export should preserve unknown actions for troubleshooting, got ${csv}`)
}
if (!csv.includes('"系统 worker,1"') || !csv.includes('"配置 auth""security"')) {
  throw new Error(`audit export should escape commas and quotes, got ${csv}`)
}
if (!csv.includes('"first line\nsecond line · reason: operator ""rollback"" · attempts: 2"')) {
  throw new Error(`audit export should escape multiline details and metadata, got ${csv}`)
}
if (/2026-06-05T00:00:00Z/.test(csv)) {
  throw new Error(`audit export should not expose raw ISO timestamps, got ${csv}`)
}

const emptyCsv = auditRowsCSV([])
if (emptyCsv !== '时间,动作,操作人,对象,结果,详情,审计ID') {
  throw new Error(`empty audit export should still include headers, got ${emptyCsv}`)
}

const filename = auditExportFilename('2026-06-05T08:09:00Z')
if (filename !== 'audit-logs-20260605-0809.csv') {
  throw new Error(`audit export filename should be stable and sortable, got ${filename}`)
}

function audit(patch: Partial<AuditLog>): AuditLog {
  return {
    id: patch.id ?? '1',
    actor: patch.actor ?? 'admin:1',
    action: patch.action ?? 'user.points_adjust',
    target: patch.target ?? 'user:2',
    detail: patch.detail ?? '',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    actor_type: patch.actor_type,
    actor_id: patch.actor_id,
    target_type: patch.target_type,
    target_id: patch.target_id,
    result: patch.result,
    metadata: patch.metadata,
    ip_addr: patch.ip_addr,
    user_agent: patch.user_agent,
    updated_at: patch.updated_at,
  }
}
