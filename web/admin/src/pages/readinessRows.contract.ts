import type { ReadinessCheck } from '../../../shared/api-types'
import { readinessOverallStatusLabel, readinessRows } from './readinessRows'

const rows = readinessRows([
  {
    key: 'payments',
    label: '支付配置',
    status: 'fail',
    detail: '暂无可用支付渠道实例',
    fix_route: 'cashier',
    fix_action: '去配置',
    blocking: true,
  },
  {
    key: 'docs',
    label: '开发文档',
    status: 'pass',
    detail: 'OpenAPI 与示例文档路由已注册',
  },
] satisfies ReadinessCheck[])

if (rows[0]?.actionHref !== '#/cashier' || rows[0]?.actionLabel !== '去配置') {
  throw new Error(`readiness rows should expose clickable action route, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.blockingLabel !== '阻塞上线' || rows[0]?.statusTone !== 'danger') {
  throw new Error(`readiness rows should mark fail checks as blocking danger, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.status !== '阻塞' || rows[0]?.rawStatus !== 'fail') {
  throw new Error(`readiness fail row should display localized status and preserve raw status, got ${JSON.stringify(rows[0])}`)
}

if (rows[1]?.actionHref !== '#/readiness' || rows[1]?.actionLabel !== '查看') {
  throw new Error(`readiness rows should provide safe fallback action, got ${JSON.stringify(rows[1])}`)
}

if (rows[1]?.status !== '通过' || rows[1]?.rawStatus !== 'pass' || rows[1]?.statusTone !== 'success') {
  throw new Error(`readiness pass row should display localized status and preserve raw status, got ${JSON.stringify(rows[1])}`)
}

if (readinessOverallStatusLabel('fail') !== '存在阻塞' || readinessOverallStatusLabel('warn') !== '存在警告' || readinessOverallStatusLabel('pass') !== '全部通过') {
  throw new Error('readiness overall status should use operator-facing labels')
}

if (readinessOverallStatusLabel('maintenance') !== 'maintenance' || readinessOverallStatusLabel('') !== '未知') {
  throw new Error('readiness overall status should preserve unknown raw values and fallback empty status')
}
