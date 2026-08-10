import { ApiError } from '../../../shared/http-client'

export type ConfigurationDependency = { dependency?: unknown; count?: unknown }

export function configurationDependencyMessage(details: ConfigurationDependency = {}) {
  const count = Number(details.count)
  const amount = Number.isFinite(count) && count > 0 ? count : 1
  switch (details.dependency) {
    case 'account_models':
      return `当前账号仍有 ${amount} 个真实模型。请先删除这些真实模型，再删除账号。`
    case 'route_candidates':
      return `当前配置仍被 ${amount} 个候选模型引用。请先在路由模型中删除候选，再重试。`
    case 'route_prices':
      return `当前路由仍有 ${amount} 条价格配置。请先在价格策略中删除这些价格，再删除路由。`
    default:
      return '当前配置仍被其他生效配置引用。请先删除依赖项，再重试。'
  }
}

export function modelLifecycleErrorMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError && error.status === 409 && error.code === 'configuration_in_use') {
    return configurationDependencyMessage(error.details)
  }
  return error instanceof Error ? error.message : fallback
}
