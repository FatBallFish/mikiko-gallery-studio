import {
  reviewActionsForStatus,
  reviewDefaultReason,
  reviewListQuery,
  reviewRowView,
  reviewStatusLabel,
  reviewStatusTone,
  reviewStatusTabs,
  reviewTerminalActionLabel,
} from './reviewRows'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const reviewPageSource = readFileSync(new URL('./ReviewPage.tsx', import.meta.url), 'utf8')

const combinedQuery = reviewListQuery({
  user: 'alice',
  prompt: 'warm light',
  model: 'studio-v2',
  taskType: 'image_edit',
  baseResolution: '2k',
  requestedSize: '1536x1024',
  width: '1536',
  height: '1024',
  aspectRatio: '3:2',
  createdFrom: '2026-08-01T00:00:00Z',
  createdTo: '2026-08-05T23:59:59Z',
  publishedFrom: '2026-08-02T00:00:00Z',
  publishedTo: '2026-08-04T23:59:59Z',
}, 'approved', 2, 50)
for (const [key, expected] of Object.entries({
  user: 'alice', prompt: 'warm light', model: 'studio-v2', task_type: 'image_edit', base_resolution: '2k',
  requested_size: '1536x1024', width: 1536, height: 1024, aspect_ratio: '3:2', status: 'approved', page: 2, page_size: 50,
  created_from: '2026-08-01T00:00:00Z', created_to: '2026-08-05T23:59:59Z', published_from: '2026-08-02T00:00:00Z', published_to: '2026-08-04T23:59:59Z',
})) {
  if (combinedQuery[key] !== expected) throw new Error(`review server query should map ${key}, got ${JSON.stringify(combinedQuery)}`)
}
if (reviewListQuery({ user: '', prompt: '', model: '', taskType: '', baseResolution: '', requestedSize: '', width: '', height: '', aspectRatio: '', createdFrom: '', createdTo: '', publishedFrom: '', publishedTo: '' }, 'all', 1, 20).status !== undefined) {
  throw new Error('all review tab should omit the status query')
}

const pendingActions = reviewActionsForStatus('pending_review')
if (pendingActions.map((action) => action.decision).join(',') !== 'approve,reject') {
  throw new Error(`pending review items should only allow approve/reject, got ${JSON.stringify(pendingActions)}`)
}

if (pendingActions.some((action) => action.decision === 'unpublish')) {
  throw new Error(`pending review items must not expose unpublish before approval, got ${JSON.stringify(pendingActions)}`)
}

const approvedActions = reviewActionsForStatus('approved')
if (approvedActions.length !== 1 || approvedActions[0]?.decision !== 'unpublish') {
  throw new Error(`approved review items should only allow unpublish, got ${JSON.stringify(approvedActions)}`)
}

for (const terminalStatus of ['rejected', 'unpublished'] as const) {
  const actions = reviewActionsForStatus(terminalStatus)
  if (actions.length) {
    throw new Error(`${terminalStatus} review items should not expose more write actions, got ${JSON.stringify(actions)}`)
  }
}

if (reviewTerminalActionLabel({ status: 'rejected' }) !== '已驳回' || reviewTerminalActionLabel({ status: 'unpublished' }) !== '已下架') {
  throw new Error('terminal review rows should show clear non-clickable labels')
}

if (reviewStatusLabel('pending') !== '待审核' || reviewStatusLabel('approved') !== '已通过' || reviewStatusLabel('public') !== '已通过') {
  throw new Error('review status labels should localize backend visibility states')
}

if (reviewStatusTone('pending_review') !== 'warning' || reviewStatusTone('approved') !== 'success' || reviewStatusTone('rejected') !== 'danger') {
  throw new Error('review status tones should match operational severity')
}

if (!reviewStatusTabs.includes('pending_review') || !reviewStatusTabs.includes('all')) {
  throw new Error(`review status tabs should keep pending queue and all view, got ${reviewStatusTabs.join(',')}`)
}

for (const decision of ['approve', 'reject', 'unpublish'] as const) {
  if (!reviewDefaultReason(decision).trim()) {
    throw new Error(`review decision ${decision} should have a default reason`)
  }
}

const pendingRow = reviewRowView({
  id: 'review_1',
  image_id: 'img_1',
  title: '公开申请',
  owner: 'user@example.com',
  task_type: 'image_edit',
  image_url: '/image.png',
  status: 'pending_review',
  reason: '',
  created_at: '2026-06-05T13:45:30Z',
})

if (pendingRow.createdAtLabel !== '2026/06/05 13:45' || pendingRow.taskTypeLabel !== '图片编辑') {
  throw new Error(`review row should format created_at and task type for operators, got ${JSON.stringify(pendingRow)}`)
}

if (pendingRow.statusLabel !== '待审核' || pendingRow.statusTone !== 'warning') {
  throw new Error(`review row should expose localized status model, got ${JSON.stringify(pendingRow)}`)
}

if (pendingRow.actions.map((action) => action.decision).join(',') !== 'approve,reject' || pendingRow.terminalActionLabel !== '') {
  throw new Error(`pending review row should expose approve/reject actions only, got ${JSON.stringify(pendingRow)}`)
}

if (/T|Z$/.test(`${pendingRow.createdAtLabel}${pendingRow.statusLabel}`)) {
  throw new Error(`review row should not expose ISO separators, got ${JSON.stringify(pendingRow)}`)
}

const invalidDateRow = reviewRowView({ ...pendingRow.raw, created_at: 'not-a-date' })
if (invalidDateRow.createdAtLabel !== 'not-a-date') {
  throw new Error(`review row should preserve invalid created_at for troubleshooting, got ${invalidDateRow.createdAtLabel}`)
}

const rejectedRow = reviewRowView({ ...pendingRow.raw, status: 'rejected' })
if (rejectedRow.actions.length || rejectedRow.terminalActionLabel !== '已驳回') {
  throw new Error(`rejected review row should expose terminal label without write actions, got ${JSON.stringify(rejectedRow)}`)
}

for (const region of ['queue', 'preview', 'action']) {
  if (!reviewPageSource.includes(`data-review-region="${region}"`)) {
    throw new Error(`review workbench must keep a stable ${region} region`)
  }
}

for (const keyboardContract of [
  'role="listbox"',
  'onKeyDown={handleQueueKeyDown}',
  "event.key !== 'ArrowDown' && event.key !== 'ArrowUp'",
  'tabIndex={active ? 0 : -1}',
  'aria-selected={active}',
  'focus-visible:outline-[var(--focus-ring)]',
]) {
  if (!reviewPageSource.includes(keyboardContract)) {
    throw new Error(`review queue needs keyboard selection and visible focus: ${keyboardContract}`)
  }
}

for (const reasonContract of [
  'reasonPresets.map((preset)',
  'setDraftReason(preset)',
  '补充说明（可选）',
  'value={draftReason}',
]) {
  if (!reviewPageSource.includes(reasonContract)) {
    throw new Error(`review rejection reasons must be interactive: ${reasonContract}`)
  }
}

for (const behaviorContract of [
  'adminApi.decideReview',
  'adminApi.listReviews(reviewListQuery(appliedFilters, filter, page, pageSize))',
  'setRows(result.items)',
  'setTotal(result.total)',
  '<FilterToolbar',
  '<Pager',
  'setMutationError(',
  '<InlineFeedback tone="danger" message={mutationError}',
  'useAdminPreviewMotion',
  'refreshing',
  'requestGenerationRef',
  'loading && !rows.length',
  'error && rows.length',
  'decisionTriggerRef',
  "querySelector<HTMLElement>('textarea, button')",
  'decisionTriggerRef.current?.focus()',
  '<RefreshIconButton label="刷新审核队列"',
  'refreshing={refreshing}',
  'disabled={Boolean(drawer) || busy}',
  'interactionLocked={refreshing}',
  'requestGenerationRef.current += 1',
]) {
  if (!reviewPageSource.includes(behaviorContract)) {
    throw new Error(`review workbench must preserve bounded local behavior: ${behaviorContract}`)
  }
}

for (const visualDrift of ['rounded-2xl', 'rounded-3xl', 'hover:scale-105', 'shadow-lg']) {
  if (reviewPageSource.includes(visualDrift)) {
    throw new Error(`review workbench must remove visual drift: ${visualDrift}`)
  }
}
