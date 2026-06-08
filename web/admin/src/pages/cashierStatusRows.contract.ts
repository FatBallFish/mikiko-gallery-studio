import {
  cashierBooleanVisibilityLabel,
  cashierEnabledBadge,
  cashierOrderStatusBadge,
  cashierPlanStatusBadge,
  cashierPlanStatusOptions,
  cashierPlanTypeLabel,
  cashierSyncStatusLabel,
  cashierVisibleFlagLabel,
  cashierWebhookStatusBadge,
} from './cashierStatusRows'

const enabled = cashierEnabledBadge(true)
const disabled = cashierEnabledBadge(false)
if (enabled.label !== '启用' || enabled.tone !== 'success' || disabled.label !== '停用' || disabled.tone !== 'warning') {
  throw new Error(`cashier enabled flags should be localized, got ${JSON.stringify({ enabled, disabled })}`)
}

if (cashierVisibleFlagLabel(true) !== '启用' || cashierVisibleFlagLabel(false) !== '停用') {
  throw new Error('visible method checkboxes should keep localized enabled labels')
}

if (cashierBooleanVisibilityLabel(true) !== '已启用' || cashierBooleanVisibilityLabel(false) !== '已隐藏') {
  throw new Error('boolean visibility labels should be operator-facing')
}

const activePlan = cashierPlanStatusBadge('active')
const disabledPlan = cashierPlanStatusBadge('disabled')
const archivedPlan = cashierPlanStatusBadge('archived')
if (activePlan.label !== '启用' || activePlan.tone !== 'success' || disabledPlan.label !== '停用' || archivedPlan.label !== '已归档') {
  throw new Error(`plan status labels should localize known states, got ${JSON.stringify({ activePlan, disabledPlan, archivedPlan })}`)
}

if (cashierPlanTypeLabel('points_package') !== '积分包' || cashierPlanTypeLabel('subscription') !== '订阅套餐') {
  throw new Error('plan type labels should localize points packages and subscription plans')
}

const planOptionText = cashierPlanStatusOptions.map((option) => option.label).join(',')
if (planOptionText.includes('active') || planOptionText.includes('disabled') || planOptionText.includes('archived')) {
  throw new Error(`plan status options should not expose raw status labels, got ${planOptionText}`)
}

for (const value of ['active', 'disabled', 'archived']) {
  if (!cashierPlanStatusOptions.some((option) => option.value === value)) {
    throw new Error(`plan status options must preserve raw value ${value}`)
  }
}

const orderExpectations = [
  ['pending', '待支付', 'warning'],
  ['paid', '已支付', 'success'],
  ['completed', '已完成', 'success'],
  ['partially_refunded', '部分退款', 'warning'],
  ['refunded', '已退款', 'neutral'],
  ['failed', '失败', 'danger'],
] as const
for (const [status, label, tone] of orderExpectations) {
  const badge = cashierOrderStatusBadge(status)
  if (badge.label !== label || badge.tone !== tone) {
    throw new Error(`order ${status} should be ${label}/${tone}, got ${JSON.stringify(badge)}`)
  }
}

const webhookFailed = cashierWebhookStatusBadge('failed')
const webhookProcessed = cashierWebhookStatusBadge('processed')
if (webhookFailed.label !== '失败' || webhookFailed.tone !== 'danger' || webhookProcessed.label !== '已处理' || webhookProcessed.tone !== 'success') {
  throw new Error(`webhook statuses should be localized, got ${JSON.stringify({ webhookFailed, webhookProcessed })}`)
}

if (cashierSyncStatusLabel('paid') !== '已支付' || cashierSyncStatusLabel('failed') !== '查询失败') {
  throw new Error('sync query statuses should be localized in feedback copy')
}

const unknown = cashierOrderStatusBadge('provider_hold')
if (unknown.label !== 'provider_hold' || unknown.tone !== 'neutral') {
  throw new Error(`unknown cashier states should preserve raw value for troubleshooting, got ${JSON.stringify(unknown)}`)
}
