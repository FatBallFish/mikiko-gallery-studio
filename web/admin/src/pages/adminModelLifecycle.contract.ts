import { configurationDependencyMessage } from './adminModelLifecycle'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const providerSource = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')
const routingSource = readFileSync(new URL('./RoutingPage.tsx', import.meta.url), 'utf8')
const pricingSource = readFileSync(new URL('./PricingPage.tsx', import.meta.url), 'utf8')

for (const [source, required] of [
  [providerSource, ['adminApi.deleteModelAccount', 'adminApi.deleteModelAccountModel']],
  [routingSource, ['adminApi.deleteRouteModel', 'adminApi.deleteRouteModelCandidate']],
  [pricingSource, ['adminApi.deleteRouteModelPrice']],
] as const) {
  for (const operation of required) {
    if (!source.includes(operation)) throw new Error(`missing lifecycle operation ${operation}`)
  }
  if (!source.includes('<Modal')) throw new Error('destructive lifecycle actions require a confirmation Modal')
  if (!source.includes('disabled={deleting')) throw new Error('destructive lifecycle actions must disable submission while deleting')
}

for (const contract of [
  { name: '接入账号', source: providerSource, pattern: /<TooltipIconButton label=\{`删除账号 \$\{account\.name\}`\}[\s\S]{0,200}<Trash2 \/><\/TooltipIconButton>/ },
  { name: '真实模型', source: providerSource, pattern: /<TooltipIconButton label=\{`删除真实模型 \$\{model\.model_code\}`\}[\s\S]{0,200}<Trash2 \/><\/TooltipIconButton>/ },
  { name: '路由模型', source: routingSource, pattern: /<TooltipIconButton label=\{`删除路由 \$\{route\.code\}`\}[\s\S]{0,200}<Trash2 \/><\/TooltipIconButton>/ },
  { name: '模型能力', source: routingSource, pattern: /<TooltipIconButton label=\{`删除候选 \$\{candidate\.model_code \|\| candidate\.id\}`\}[\s\S]{0,200}<Trash2 \/><\/TooltipIconButton>/ },
  { name: '分辨率价格', source: pricingSource, pattern: /<TooltipIconButton label=\{`删除 \$\{pricingBaseResolutionLabel\(row\.base_resolution\)\} 价格`\}[\s\S]{0,200}<Trash2 \/><\/TooltipIconButton>/ },
]) {
  if (!contract.pattern.test(contract.source)) {
    throw new Error(`${contract.name}删除操作必须使用带 Trash2 图标和 tooltip 的可见 TooltipIconButton`)
  }
}

assertIncludes(configurationDependencyMessage({ dependency: 'account_models', count: 2 }), '2 个真实模型')
assertIncludes(configurationDependencyMessage({ dependency: 'route_candidates', count: 1 }), '候选模型')
assertIncludes(configurationDependencyMessage({ dependency: 'route_prices', count: 3 }), '3 条价格')

function assertIncludes(actual: string, expected: string) {
  if (!actual.includes(expected)) throw new Error(`expected ${JSON.stringify(actual)} to include ${JSON.stringify(expected)}`)
}
