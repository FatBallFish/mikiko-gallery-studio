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
