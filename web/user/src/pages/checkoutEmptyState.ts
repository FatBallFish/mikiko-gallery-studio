export type CheckoutUnavailableEmptyState = {
  title: string
  detail: string
  primaryAction: string
  secondaryAction: string
}

export function checkoutUnavailableEmptyState(): CheckoutUnavailableEmptyState {
  return {
    title: '充值配置待完善',
    detail: '请先刷新配置；如果仍无法创建订单，可稍后再试或先查看账户余额。',
    primaryAction: '刷新配置',
    secondaryAction: '查看余额',
  }
}

export function checkoutPlanEmptyState(): CheckoutUnavailableEmptyState {
  return {
    title: '充值配置待完善',
    detail: '当前没有可购买的固定积分包，请先刷新配置；如果仍为空，可稍后再试或先查看账户余额。',
    primaryAction: '刷新配置',
    secondaryAction: '查看余额',
  }
}

export function checkoutPaymentMethodEmptyState(): CheckoutUnavailableEmptyState {
  return {
    title: '支付方式待开启',
    detail: '当前没有可选择的支付方式，请先刷新配置；如果仍为空，可稍后再试或先查看账户余额。',
    primaryAction: '刷新配置',
    secondaryAction: '查看余额',
  }
}
