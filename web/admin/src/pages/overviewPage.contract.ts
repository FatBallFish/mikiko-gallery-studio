// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'

const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'OverviewPage.tsx'), 'utf8')

for (const forbidden of ['No providers', 'Model Distribution', 'Rankings', 'Operations Detail', 'providers tracked', 'generation guardrail blocks', 'public gallery scans']) {
  if (source.includes(forbidden)) {
    throw new Error(`overview page should not expose noisy mixed-language dashboard copy: ${forbidden}`)
  }
}

if (!source.includes('暂无模型调用')) {
  throw new Error('overview page should render an empty state instead of a fake provider progress bar')
}

for (const inlineEmpty of ['暂无模型调用', '暂无待处理事项', '暂无用户排行', '暂无上线风险']) {
  if (!source.includes(`<EmptyBlock variant="inline" title="${inlineEmpty}"`)) {
    throw new Error(`nested overview empty state must use the unframed inline variant: ${inlineEmpty}`)
  }
}

for (const overviewContract of [
  'MetricStrip',
  'DataTable',
  'function overviewReadinessColumns',
  'grid-flow-dense grid-cols-12',
  'lg:col-span-8',
  'lg:col-span-4',
  'function OperationalAlertRail',
  '生成成功率',
  'call_distribution',
  '真实上游调用',
  '24 小时窗口',
  'preflight_failure_count',
]) {
  if (!source.includes(overviewContract)) throw new Error(`overview must implement ${overviewContract}`)
}

for (const forbiddenFakeDistribution of ['providerDistributionRows', 'providerWeight']) {
  if (source.includes(forbiddenFakeDistribution)) {
    throw new Error(`overview must not derive call distribution from provider health: ${forbiddenFakeDistribution}`)
  }
}

for (const forbidden of ['<details', 'rounded-2xl', 'rounded-3xl', '.trend?.match']) {
  if (source.includes(forbidden)) throw new Error(`overview must remove ${forbidden}`)
}

for (const brokenReadinessGrid of ['readinessGrid:', 'overviewClasses.readinessGrid', 'overviewClasses.dataGrid']) {
  if (source.includes(brokenReadinessGrid)) {
    throw new Error(`overview readiness risks must not reuse nested grid contract ${brokenReadinessGrid}`)
  }
}
