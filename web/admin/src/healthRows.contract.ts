import type { ProviderHealth } from '../../shared/api-types'
import { healthProviderRows, healthReadinessRows, healthRefreshTimeLabel, healthStatusLabel, healthStatusTone, providerHealthValue, providerHealthWarn, refreshPolicyLabel, taskQueuePressure } from './healthRows'

assertEqual(healthStatusLabel('healthy'), '健康', 'healthy status label')
assertEqual(healthStatusTone('healthy'), 'success', 'healthy status tone')
assertEqual(healthStatusLabel('degraded'), '降级', 'degraded status label')
assertEqual(healthStatusTone('degraded'), 'warning', 'degraded status tone')
assertEqual(healthStatusLabel('down'), '不可用', 'down status label')
assertEqual(healthStatusTone('down'), 'danger', 'down status tone')
assertEqual(healthStatusLabel('pass'), '通过', 'pass status label')
assertEqual(healthStatusTone('pass'), 'success', 'pass status tone')
assertEqual(healthStatusLabel(' throttled '), 'throttled', 'unknown status fallback')
assertEqual(healthStatusLabel(''), '未知状态', 'empty status fallback')

const healthyProvider: ProviderHealth = {
  provider: 'OpenAI',
  status: 'healthy',
  latency_ms: 88,
  error_rate: '0%',
  note: '主路由健康',
}

const degradedProvider: ProviderHealth = {
  provider: 'Task Worker',
  status: 'degraded',
  latency_ms: 1200,
  error_rate: '2%',
  note: '队列等待 12',
}

assertEqual(providerHealthValue(healthyProvider), '健康', 'healthy provider shell value')
assertEqual(String(providerHealthWarn(healthyProvider)), 'false', 'healthy provider shell warning')
assertEqual(providerHealthValue(degradedProvider), '队列等待 12', 'degraded provider shell value')
assertEqual(String(providerHealthWarn(degradedProvider)), 'true', 'degraded provider shell warning')
assertEqual(taskQueuePressure([healthyProvider]), '正常', 'missing task worker pressure fallback')
assertEqual(taskQueuePressure([healthyProvider, degradedProvider]), '队列等待 12', 'task worker pressure note')
assertEqual(refreshPolicyLabel('30s interval'), '每 30 秒巡检', 'refresh policy label')
assertEqual(refreshPolicyLabel(''), '按需刷新', 'empty refresh policy fallback')

const providerRows = healthProviderRows([healthyProvider, degradedProvider])
assertEqual(providerRows[0]?.name ?? '', 'OpenAI', 'provider row name')
assertEqual(providerRows[0]?.statusLabel ?? '', '健康', 'provider row localized healthy status')
assertEqual(providerRows[1]?.statusLabel ?? '', '降级', 'provider row localized degraded status')
assertEqual(providerRows[1]?.note ?? '', '队列等待 12', 'provider row note')

const readinessRows = healthReadinessRows([
  { key: 'model_accounts', label: '模型账号', status: 'fail', detail: '暂无启用账号', fix_route: 'provider-models', fix_action: '去配置', blocking: true },
  { key: 'payments', label: '支付配置', status: 'pass', detail: 'Mock 渠道已启用', fix_route: 'cashier', fix_action: '查看收银台' },
])
assertEqual(readinessRows[0]?.name ?? '', '模型账号', 'readiness row name')
assertEqual(readinessRows[0]?.statusLabel ?? '', '失败', 'readiness row localized fail status')
assertEqual(readinessRows[0]?.probeLabel ?? '', '阻塞上线', 'readiness blocking probe label')
assertEqual(readinessRows[0]?.actionHref ?? '', '#/provider-models', 'readiness action route')
assertEqual(readinessRows[1]?.statusLabel ?? '', '通过', 'readiness pass status')
assertEqual(readinessRows[1]?.probeLabel ?? '', '上线检查', 'readiness non-blocking probe label')

const fakeProbeNames = ['API Gateway', 'Postgres', 'Redis Queue', 'Object Storage']
for (const name of fakeProbeNames) {
  if (readinessRows.some((row) => row.name === name)) {
    throw new Error(`health page readiness rows must not include hard-coded fake probe ${name}`)
  }
}

assertEqual(healthRefreshTimeLabel('2026-06-05T13:45:30Z'), '2026/06/05 13:45', 'refresh time readable label')
assertEqual(healthRefreshTimeLabel('bad-time'), 'bad-time', 'refresh time preserves invalid value')
assertEqual(healthRefreshTimeLabel(''), '-', 'refresh time empty fallback')

function assertEqual(actual: string, expected: string, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${expected}, got ${actual}`)
  }
}
