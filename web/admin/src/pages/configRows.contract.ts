import { forbiddenGeneralConfigCategories, generalConfigCategories } from './configRows'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

assert(generalConfigCategories.includes('site'), '通用配置必须预留站点基础类目')
assert(generalConfigCategories.includes('docs'), '通用配置必须包含开发文档类目')
assert(generalConfigCategories.includes('public_gallery'), '通用配置必须包含公开内容类目')
assert(!generalConfigCategories.includes('auth_security' as never), '认证安全不能出现在通用配置')
assert(!generalConfigCategories.includes('generation_limits' as never), '生成限制不能出现在通用配置')
assert(!generalConfigCategories.includes('moderation' as never), '内容审核不能出现在通用配置')
assert(!generalConfigCategories.includes('payments' as never), '支付配置不能出现在通用配置')
assert(forbiddenGeneralConfigCategories.includes('auth_security'), '高风险类目禁止清单必须包含认证安全')
