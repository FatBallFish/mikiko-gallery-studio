import { InlineFeedback } from '../components'

/**
 * Legacy entry retained for imports outside the routed pages.
 * Video pricing is now the route-edit drawer's quote tab: it shows native
 * rate cards and calls the real quote simulation, never a hand-entered
 * per-second sale price.
 */
export function VideoPricingPanel() {
  return (
    <InlineFeedback
      tone="neutral"
      message="视频报价已并入「路由模型 → 编辑路由 → 视频报价」抽屉；销售价由候选厂商原生费率、最高成本、全局汇率、最低任务积分与取整步长推导。"
    />
  )
}
