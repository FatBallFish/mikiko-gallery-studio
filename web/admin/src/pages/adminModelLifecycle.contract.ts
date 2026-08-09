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

assertIncludes(configurationDependencyMessage({ dependency: 'account_models', count: 2 }), '2 个真实模型')
assertIncludes(configurationDependencyMessage({ dependency: 'route_candidates', count: 1 }), '候选模型')
assertIncludes(configurationDependencyMessage({ dependency: 'route_prices', count: 3 }), '3 条价格')

function assertIncludes(actual: string, expected: string) {
  if (!actual.includes(expected)) throw new Error(`expected ${JSON.stringify(actual)} to include ${JSON.stringify(expected)}`)
}
