import { checkoutPlanEmptyState, checkoutPaymentMethodEmptyState, checkoutUnavailableEmptyState } from './checkoutEmptyState'

const state = checkoutUnavailableEmptyState()
const visibleCopy = `${state.title}${state.detail}${state.primaryAction}${state.secondaryAction}`

if (!state.title.includes('充值配置')) {
  throw new Error(`checkout unavailable title should explain configuration state, got ${state.title}`)
}

if (!state.detail.includes('刷新') || !state.detail.includes('稍后再试')) {
  throw new Error(`checkout unavailable detail should guide refresh/retry action, got ${state.detail}`)
}

if (state.primaryAction !== '刷新配置' || state.secondaryAction !== '查看余额') {
  throw new Error(`checkout unavailable actions should guide usable next steps, got ${JSON.stringify(state)}`)
}

if (/后台|收银台|管理员|Mock|mock|暂不可用|后续|即将|版本|not available|TODO/i.test(visibleCopy)) {
  throw new Error(`checkout unavailable empty state should avoid admin/test/roadmap wording, got ${visibleCopy}`)
}

const planState = checkoutPlanEmptyState()
const planCopy = `${planState.title}${planState.detail}${planState.primaryAction}${planState.secondaryAction}`

if (planState.title !== '充值配置待完善' || !planState.detail.includes('固定积分包') || !planState.detail.includes('刷新')) {
  throw new Error(`checkout plan empty state should explain package configuration in user-facing language, got ${JSON.stringify(planState)}`)
}

if (planState.primaryAction !== '刷新配置' || planState.secondaryAction !== '查看余额') {
  throw new Error(`checkout plan empty state should keep executable actions, got ${JSON.stringify(planState)}`)
}

const methodState = checkoutPaymentMethodEmptyState()
const methodCopy = `${methodState.title}${methodState.detail}${methodState.primaryAction}${methodState.secondaryAction}`

if (methodState.title !== '支付方式待开启' || !methodState.detail.includes('刷新') || !methodState.detail.includes('稍后再试')) {
  throw new Error(`checkout payment method empty state should explain payment method state without admin jargon, got ${JSON.stringify(methodState)}`)
}

if (methodState.primaryAction !== '刷新配置' || methodState.secondaryAction !== '查看余额') {
  throw new Error(`checkout payment method empty state should keep executable actions, got ${JSON.stringify(methodState)}`)
}

const allLocalCopy = `${planCopy}${methodCopy}`
if (/后台|收银台|管理员|Mock|mock|暂不可用|后续|即将|版本|not available|TODO/i.test(allLocalCopy)) {
  throw new Error(`checkout local empty states should not expose admin/test jargon, got ${allLocalCopy}`)
}
