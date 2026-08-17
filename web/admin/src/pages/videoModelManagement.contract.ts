// @ts-nocheck
import fs from 'node:fs'

const provider = fs.readFileSync(new URL('./VideoProviderAccountsPanel.tsx', import.meta.url), 'utf8')
for (const required of ['seedance', 'minimax', 'listModelAccountModels', 'saveVideoCapability', 'saveVideoRateCard', 'seedance_token_v1', 'minimax_h3_second_v1', 'provider_native_max_n', 'prompt_max_runes', 'first_frame', 'last_frame', 'inputFormats', 'inputMaxMB']) {
  if (!provider.includes(required)) throw new Error(`video provider hierarchy must include ${required}`)
}
if (provider.includes('真实模型 ID')) throw new Error('video provider management must not require a database model ID')

const routing = fs.readFileSync(new URL('./VideoRouteConfigPanel.tsx', import.meta.url), 'utf8')
for (const required of ['candidate_parameter_mappings', 'minimum_task_points', 'rounding_step_points', 'max_output_count', 'saveRouteVideoConfig', 'deleteRouteVideoConfig', '删除视频配置']) {
  if (!routing.includes(required)) throw new Error(`video route panel must include ${required}`)
}
for (const removed of ['用户可见参数组合', 'visible_combinations']) if (routing.includes(removed)) throw new Error(`video route panel must derive capabilities instead of exposing ${removed}`)
if (routing.includes('路由模型 ID')) throw new Error('video route management must derive the selected route ID')
if (routing.includes('真实模型 ID')) throw new Error('video route mappings must display account and model names instead of database IDs')

const pricing = fs.readFileSync(new URL('./VideoPricingPanel.tsx', import.meta.url), 'utf8')
for (const required of ['simulateVideoRouteQuote', 'result.candidates', 'minimum_task_points', 'rounding_step_points', 'cny_per_point']) {
  if (!pricing.includes(required)) throw new Error(`video pricing panel must include ${required}`)
}
for (const forbidden of ['pricing_bindings', 'simulateVideoPricing', 'createVideoPriceRule', 'provider_cost_buffer_rate', 'reserve_markup']) if (pricing.includes(forbidden)) throw new Error(`video pricing panel must not include ${forbidden}`)
