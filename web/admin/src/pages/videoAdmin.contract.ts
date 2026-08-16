// @ts-nocheck
import { readFileSync } from 'node:fs'

const tasks = readFileSync(new URL('./VideoTasksPage.tsx', import.meta.url), 'utf8')
const policy = readFileSync(new URL('./MediaPolicyPage.tsx', import.meta.url), 'utf8')
const navigation = readFileSync(new URL('../layout/admin-navigation.ts', import.meta.url), 'utf8')
const pricing = readFileSync(new URL('./VideoPricingPanel.tsx', import.meta.url), 'utf8')
const provider = readFileSync(new URL('./VideoProviderAccountsPanel.tsx', import.meta.url), 'utf8')
const adminAPI = readFileSync(new URL('../../../shared/admin-api.ts', import.meta.url), 'utf8')

for (const expected of ['用户 ID', '平台任务 ID', '厂商任务 ID', '项目 ID', '路由模型', '真实模型', '结算状态', 'usage_normalized', 'provider_cost', '重新转存', '重新处理', '重新结算', 'retryAdminVideoSettlement']) {
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
for (const method of ['saveVideoCapability', 'saveVideoRateCard', 'deleteVideoRateCard', 'simulateVideoRouteQuote', 'saveRouteVideoConfig', 'deleteRouteVideoConfig', 'getRouteVideoImpact']) {
  if (!adminAPI.includes(method)) throw new Error(`video admin API must expose ${method}`)
}
if (!tasks.includes("settlement_status !== 'finalized'") || !tasks.includes("['succeeded', 'failed', 'partial', 'cancelled']")) throw new Error('settlement recovery must only appear for terminal unfinalized tasks, including cancelled releases')
for (const expected of ['CNY/百万 Token', '输出视频 CNY/秒', '输入视频 CNY/秒', '免费输入图片数', '超额图片 CNY/张']) if (!provider.includes(expected)) throw new Error(`native rate editor must expose ${expected}`)
if (!provider.includes('with_input_video_million_tokens_cny:')) throw new Error('Seedance native rate editor must submit the input-video token rate')
for (const expected of ['最低任务积分', '积分取整步长', '最高销售价', '全局汇率', '映射分辨率', '最高价来源']) if (!pricing.includes(expected)) throw new Error(`video quote overview must expose ${expected}`)
for (const forbidden of ['payment_fee_rate', 'target_margin_rate', 'provider_cost_buffer_rate', 'reserve_markup', 'simulateVideoPricing', 'createVideoPriceRule']) if (pricing.includes(forbidden) || provider.includes(forbidden)) throw new Error(`retired video pricing concept leaked into active UI: ${forbidden}`)
for (const forbidden of ['saveVideoCostRule', 'createVideoPricingStrategy', 'updateVideoPricingStrategy', 'deleteVideoPricingStrategy', 'createVideoPriceRule', 'updateVideoPriceRule', 'deleteVideoPriceRule']) if (adminAPI.includes(forbidden)) throw new Error(`retired video pricing client method remains reachable: ${forbidden}`)
