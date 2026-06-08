import {
  reviewActionsForStatus,
  reviewDefaultReason,
  reviewRowView,
  reviewStatusLabel,
  reviewStatusTone,
  reviewStatusTabs,
  reviewTerminalActionLabel,
} from './reviewRows'

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
  task_type: 'reference_to_image',
  image_url: '/image.png',
  status: 'pending_review',
  reason: '',
  created_at: '2026-06-05T13:45:30Z',
})

if (pendingRow.createdAtLabel !== '2026/06/05 13:45' || pendingRow.taskTypeLabel !== '参考生图') {
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
