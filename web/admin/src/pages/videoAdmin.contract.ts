// @ts-nocheck
import { readFileSync } from 'node:fs'

const tasks = readFileSync(new URL('./VideoTasksPage.tsx', import.meta.url), 'utf8')
const policy = readFileSync(new URL('./MediaPolicyPage.tsx', import.meta.url), 'utf8')
const navigation = readFileSync(new URL('../layout/admin-navigation.ts', import.meta.url), 'utf8')
const configuration = readFileSync(new URL('./VideoConfigurationWorkspace.tsx', import.meta.url), 'utf8')
const adminAPI = readFileSync(new URL('../../../shared/admin-api.ts', import.meta.url), 'utf8')

for (const expected of ['用户 ID', '平台任务 ID', '厂商任务 ID', '项目 ID', '路由模型', '真实模型', '结算状态', 'usage_normalized', 'provider_cost', '重新转存', '重新处理']) {
  if (!tasks.includes(expected)) throw new Error(`video operations must expose ${expected}`)
}
if (tasks.includes('>重新生成<') || tasks.includes('retryProviderGeneration')) throw new Error('video operations must never expose provider generation retry')
if (!tasks.includes('provider_generation_requested') || !tasks.includes('=== false')) throw new Error('recovery response must prove generation was not requested')

for (const expected of ['允许格式', '单文件上限', '视频最长时长', '用户存储配额', '缩略图档位', '悬浮预览', '波形', '上传会话', '软删除保留期', '只影响新上传对象和后续新建的派生版本']) {
  if (!policy.includes(expected)) throw new Error(`media policy must expose ${expected}`)
}
for (const route of ['video-tasks', 'media-policy']) {
  if (!navigation.includes(`'${route}'`)) throw new Error(`admin navigation must include ${route}`)
}
for (const method of ['saveVideoCapability', 'saveVideoCostRule', 'deleteVideoCostRule', 'createVideoPricingStrategy', 'updateVideoPricingStrategy', 'deleteVideoPricingStrategy', 'simulateVideoPricing', 'recalculateVideoPricing', 'createVideoPriceRule', 'updateVideoPriceRule', 'deleteVideoPriceRule', 'saveRouteVideoConfig', 'deleteRouteVideoConfig', 'getRouteVideoImpact']) {
  if (!adminAPI.includes(method)) throw new Error(`video admin API must expose ${method}`)
}
for (const expected of ['Provider 原生最大 n', '能力 JSON', '成本组合 JSON', '积分商品净收入保护', '目标毛利', '试算安全线', '重新计算价格版本', '可见完整组合', '最大输出数', '缺少候选', '缺少价格', '低于安全线']) {
  if (!configuration.includes(expected)) throw new Error(`video configuration workspace must expose ${expected}`)
}
if (!configuration.includes('1-10') || !configuration.includes('1-4')) throw new Error('video configuration UI must show native n and platform output limits')
