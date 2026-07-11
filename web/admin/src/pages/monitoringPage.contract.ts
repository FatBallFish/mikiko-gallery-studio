// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./MonitoringPage.tsx', import.meta.url), 'utf8')

for (const contract of [
  'getMonitoringSnapshot',
  'monitoringWindows',
  'SegmentedControl',
  'TimeSeriesChart',
  'visibilitychange',
  'document.visibilityState',
  'window.setInterval',
  '5000',
  'autoRefresh',
  'initialLoading || refreshing',
  'pollInFlight',
  'lastSuccessfulAt',
  'lastDiagnosticsAt',
  '当前仍显示上一次成功数据',
  '请求负载',
  '响应质量',
  '资源压力',
  'ResourceTrend',
  '热点接口',
  '状态码分布',
  '依赖与诊断',
  'data-admin-monitoring-runtime',
]) {
  if (!source.includes(contract)) {
    throw new Error(`runtime monitoring page should preserve ${contract}`)
  }
}

for (const primitive of ['MetricStrip', 'DataTable', 'Badge', 'InlineFeedback', 'PageHeader']) {
  if (!source.includes(primitive)) {
    throw new Error(`runtime monitoring page should reuse ${primitive}`)
  }
}

for (const obsolete of ['完整上线检查', '上游探针', 'data-admin-monitoring-blockers', 'healthScore', 'Math.random']) {
  if (source.includes(obsolete)) {
    throw new Error(`runtime monitoring page should remove obsolete pattern ${obsolete}`)
  }
}
