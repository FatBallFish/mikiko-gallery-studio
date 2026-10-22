import { InlineFeedback } from '../components'

/**
 * Legacy entry retained for imports outside the routed pages.
 * Video route scheduling lives in RoutingPage's route-edit drawer, where the
 * candidate resolution mappings, minimum task points, rounding step and
 * provider max output count are edited together with the route.
 */
export function VideoRouteConfigPanel(_props: {
  route: unknown
  config?: unknown
  candidates?: unknown[]
  onSaved: () => void
}) {
  void _props
  return (
    <InlineFeedback
      tone="neutral"
      message="视频路由参数已并入「路由模型 → 编辑路由 → 视频参数」抽屉；候选分辨率映射、最低任务积分、积分取整步长与平台最大输出数统一在该处维护。"
    />
  )
}
