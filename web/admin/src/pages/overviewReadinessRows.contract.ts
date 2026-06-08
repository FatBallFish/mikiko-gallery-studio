import type { ReadinessCheck } from '../../../shared/api-types'
import { overviewReadinessRows } from './overviewReadinessRows'

const rows = overviewReadinessRows([
  {
    key: 'models',
    label: '模型账号',
    status: 'pass',
    detail: '已配置',
    fix_route: 'provider-models',
    fix_action: '去配置',
  },
  {
    key: 'routes',
    label: '路由模型',
    status: 'fail',
    detail: '暂无可见启用路由模型',
    fix_route: 'routing',
    fix_action: '去配置',
  },
  {
    key: 'prices',
    label: '价格策略',
    status: 'warn',
    detail: '',
    summary: '部分价格缺失',
    action_route: 'pricing',
    action_label: '去补齐',
  },
  {
    key: 'docs',
    label: '开发文档',
    status: 'warn',
    detail: '',
  },
  {
    key: 'gallery',
    label: '公开广场',
    status: 'fail',
    detail: '暂无公开作品',
    fix_route: 'reviews',
    fix_action: '去审核',
  },
  {
    key: 'payments',
    label: '支付配置',
    status: 'warn',
    detail: 'Mock 渠道未关闭',
    fix_route: 'cashier',
    fix_action: '去处理',
  },
  {
    key: 'extra',
    label: '额外风险',
    status: 'fail',
    detail: '不应出现在前 5 项之外',
    fix_route: 'config',
    fix_action: '处理',
  },
] satisfies ReadinessCheck[])

if (rows.length !== 5 || rows.some((row) => row.status === 'pass')) {
  throw new Error(`overview readiness should keep only first five fail/warn rows, got ${JSON.stringify(rows)}`)
}

if (rows[0]?.actionHref !== '#/routing' || rows[0]?.actionLabel !== '去配置' || rows[0]?.statusTone !== 'danger') {
  throw new Error(`overview readiness fail row should expose routing action and danger tone, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.status !== '阻塞' || rows[0]?.rawStatus !== 'fail') {
  throw new Error(`overview readiness fail row should display localized status and preserve raw status, got ${JSON.stringify(rows[0])}`)
}

if (rows[1]?.actionHref !== '#/pricing' || rows[1]?.actionLabel !== '去补齐' || rows[1]?.statusTone !== 'warning') {
  throw new Error(`overview readiness warn row should expose pricing action and warning tone, got ${JSON.stringify(rows[1])}`)
}

if (rows[1]?.status !== '警告' || rows[1]?.rawStatus !== 'warn') {
  throw new Error(`overview readiness warn row should display localized status and preserve raw status, got ${JSON.stringify(rows[1])}`)
}

if (rows[2]?.detail !== '-' || rows[2]?.actionHref !== '#/readiness' || rows[2]?.actionLabel !== '处理') {
  throw new Error(`overview readiness row should fallback detail/action safely, got ${JSON.stringify(rows[2])}`)
}

if (rows[rows.length - 1]?.key !== 'payments') {
  throw new Error(`overview readiness should preserve original risk order and cap to five, got ${JSON.stringify(rows)}`)
}
