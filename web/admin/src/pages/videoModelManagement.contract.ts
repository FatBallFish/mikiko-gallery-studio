// @ts-nocheck
import fs from 'node:fs'

const provider = fs.readFileSync(new URL('./VideoProviderAccountsPanel.tsx', import.meta.url), 'utf8')
for (const required of ['seedance', 'minimax', 'listModelAccountModels', 'saveVideoCapability', 'saveVideoCostRule', 'provider_native_max_n', 'prompt_max_runes', 'first_frame', 'last_frame', 'inputFormats', 'inputMaxMB']) {
  if (!provider.includes(required)) throw new Error(`video provider hierarchy must include ${required}`)
}
if (provider.includes('真实模型 ID')) throw new Error('video provider management must not require a database model ID')

const routing = fs.readFileSync(new URL('./VideoRouteConfigPanel.tsx', import.meta.url), 'utf8')
for (const required of ['visible_combinations', 'pricing_strategy_id', 'max_output_count', 'saveRouteVideoConfig']) {
  if (!routing.includes(required)) throw new Error(`video route panel must include ${required}`)
}
if (routing.includes('路由模型 ID')) throw new Error('video route management must derive the selected route ID')

const pricing = fs.readFileSync(new URL('./VideoPricingPanel.tsx', import.meta.url), 'utf8')
for (const required of ['pricing_bindings', 'simulateVideoPricing', 'createVideoPriceRule', 'provider_cost_buffer_rate', 'reserve_markup']) {
  if (!pricing.includes(required)) throw new Error(`video pricing panel must include ${required}`)
}
