// @ts-nocheck
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { nativeImageProfile } from '../../../shared/native-image'
import { routeImageCapabilities, routeImagePricingErrors, routeTierErrors, newRouteImageRule, legacyRoutePriceErrors, routeImagePricingAvailability, confirmPricingTransition } from './routeImagePricing'
import { nativeLayerCostMissing, switchNativePreset } from './nativeImageConfig'
const account = { id: 1, status: 'enabled', adapter_type: 'seedream_image', extra: {} }
const profile = nativeImageProfile('doubao-seedream-5-0-pro-260628')!
const model = { id: 11, account_id: 1, enabled: true, model_code: profile.model, task_types: ['text_to_image', 'image_edit'], base_resolution: ['1K', '2K', '4K'], extra: {} }
const candidate = { account_model_id: 11, enabled: true }
const caps = routeImageCapabilities([candidate], [model], [account])
assert.deepEqual(caps.operations, ['generate', 'edit', 'layer_decomposition'])
assert.deepEqual(caps.resolutions, ['1K', '1.5K', '2K'])
assert.equal(caps.resolutions.includes('4K'), false, 'native profile overrides stale model 4K claim')
assert.equal(caps.maxPixels, profile.max_pixels)
assert.equal(caps.maxLayers, 16)
const tier = { name: 'all', max_pixels: profile.max_pixels, price: '0.32' }
const generate = { ...newRouteImageRule('generate'), version: 'sale-1', input: { per_image: '0.01', free_images: 1 }, output_tiers: [tier] }
const edit = { ...generate, operation: 'edit' }
const config = { schema_version: 1, rules: [generate, edit] }
assert.deepEqual(routeImagePricingErrors(config, caps), [])
assert.equal(config.rules[0].currency, 'CNY')
assert.equal(config.rules[0].input.free_images, 1)
assert.ok(routeImagePricingErrors({ ...config, rules: [{ ...generate, output_tiers: [{ ...tier, max_pixels: 1048576 }] }, edit] }, caps).some((error) => error.includes('覆盖')))
assert.ok(routeTierErrors([{ name: '4K', base_resolution: '4K', price: '1' }], caps, '输出').some((error) => error.includes('不受')))
assert.ok(routeTierErrors([{ ...tier, base_resolution: '2K' }], caps, '输出').some((error) => error.includes('混用')))
assert.ok(legacyRoutePriceErrors([{ enabled: true, task_type: 'text_to_image', base_resolution: '1K', base_points: '10' }], caps).length)
const token = { operation: 'generate', currency: 'points', version: 'token-1', mode: 'pixel_token', input: { free_images: 0 }, pixel_token: { definition: 'platform_pixels_v1', pixels_per_token: 256, input_per_million: '10', output_per_million: '20' } }
assert.deepEqual(routeImagePricingErrors({ schema_version: 1, rules: [token, edit] }, caps), [])
assert.ok(routeImagePricingErrors({ schema_version: 1, rules: [{ ...token, input: { free_images: 0, per_image: '0' } }, edit] }, caps).some((error) => error.includes('混用')))
for (const invalid of ['', '-1', 'NaN', 'Infinity', '1e3']) assert.ok(routeImagePricingErrors({ ...config, rules: [{ ...generate, input: { per_image: invalid, free_images: 0 } }, edit] }, caps).length)
assert.ok(Number.isNaN(newRouteImageRule('generate').input.free_images), 'draft does not invent free count')
const layers = { ...newRouteImageRule('layer_decomposition'), version: 'layer-1', input: { free_images: 0, per_image: '0' }, layers: { confirmed: false, tiers: [] } }
const layerErrors = routeImagePricingErrors({ schema_version: 1, rules: [...config.rules, layers] }, caps)
assert.ok(layerErrors.some((error) => error.includes('尚未确认')))
assert.ok(layerErrors.some((error) => error.includes('底图')))
assert.ok(layerErrors.some((error) => error.includes('缺少档位')))
const costMissing = nativeLayerCostMissing({}, profile)
assert.ok(costMissing.includes('拆层成本口径确认'))
assert.ok(costMissing.some((item) => item.includes('layer_base_cny')))
assert.ok(costMissing.some((item) => item.includes('layer_tiers')))
assert.deepEqual(routeImageCapabilities([candidate], [model], [{ ...account, status: 'disabled' }]).operations, [])
const qwen = { ...model, id: 12, account_id: 2, model_code: 'qwen-image-3.0', extra: {} }
const mixed = routeImageCapabilities([candidate, { ...candidate, account_model_id: 12 }], [model, qwen], [account, { ...account, id: 2, adapter_type: 'qwen_image' }])
assert.deepEqual(mixed.operations, ['generate', 'edit'])
assert.deepEqual(mixed.resolutions, [], 'no synthetic official 1K label for Qwen dimensions')
assert.ok(mixed.warnings.some((warning) => warning.includes('不兼容')))
const oldSale = { model: 'old', currency: 'USD', input_cny: 5, future: { keep: true } }
assert.deepEqual(switchNativePreset({ native_image: { model: 'old', sale_pricing: oldSale } }, profile).native_image.sale_pricing, oldSale, 'retired sale metadata stays untouched')
const source = readFileSync(new URL('./RouteImagePricingFields.tsx', import.meta.url), 'utf8')
for (const text of ['FormSection', 'LedgerField', 'SwitchField', 'platform_pixels_v1', 'ceil(宽 × 高 / pixels_per_token)', '1,000,000', '此表单不修改 FX']) assert.ok(source.includes(text), text)
assert.ok(!source.includes('JSON.parse'))
console.log('route image pricing admin contracts passed')

// Review regressions: saving a generate-only rule does not require edit, even on disabled routes.
const generateOnly = { schema_version: 1, rules: [generate] }
assert.deepEqual(routeImagePricingErrors(generateOnly, caps), [])
let status = routeImagePricingAvailability(generateOnly, caps)
assert.deepEqual(status.available, ['generate'])
assert.ok(status.warnings.some((message) => message.includes('参考图编辑')))
status = routeImagePricingAvailability({ schema_version: 1, rules: [generate, layers] }, caps)
assert.equal(status.ready, true, 'layer draft must not block a valid ordinary sale')
assert.deepEqual(status.available, ['generate'])
assert.ok(status.warnings.some((message) => message.includes('底图')))
const confirmedLayers = { ...layers, layers: { confirmed: true, base_price: '0.1', tiers: [tier] } }
status = routeImagePricingAvailability({ schema_version: 1, rules: [generate, confirmedLayers] }, { ...caps, maxLayers: undefined, layerSizeModes: [] })
assert.deepEqual(status.available, ['generate'])
assert.ok(status.warnings.some((message) => message.includes('max_layers')))
assert.ok(status.warnings.some((message) => message.includes('layer_size_modes')))
assert.ok(status.warnings.some((message) => message.includes('成本缺少')))
assert.ok(routeTierErrors([{ name: 'auto', base_resolution: 'auto', price: '1' }], caps, '拆层', true).length)
assert.ok(routeTierErrors(['1K', '1.5K', '2K'].map((mode) => ({ name: mode, base_resolution: mode, price: '1' })), caps, '拆层', true).some((message) => message.includes('auto')))
assert.deepEqual(routeTierErrors([tier], caps, '拆层', true), [])
const legacyAccount = { ...account, adapter_type: 'openai' }
const legacyModel = { ...model, model_code: 'gpt-image-2', base_resolution: ['1K'], supported_pixel_sizes: [], supports_custom_size: false }
const legacyCaps = routeImageCapabilities([candidate], [legacyModel], [legacyAccount])
assert.equal(legacyCaps.maxPixels, 1048576)
assert.ok(routeTierErrors([{ ...tier, max_pixels: 16777216 }], legacyCaps, '普通').some((message) => message.includes('超过')))
const presetCaps = routeImageCapabilities([candidate], [{ ...legacyModel, supported_pixel_sizes: ['1536x1024'] }], [legacyAccount])
assert.equal(presetCaps.maxPixels, 1572864)
const customCaps = routeImageCapabilities([candidate], [{ ...legacyModel, supports_custom_size: true, max_width: 2048, max_height: 2048 }], [legacyAccount])
assert.equal(customCaps.maxPixels, 4194304)
let confirmations = 0
assert.equal(confirmPricingTransition(false, () => { confirmations++; return false }, 'leave'), true)
assert.equal(confirmPricingTransition(true, () => { confirmations++; return false }, 'discard'), false)
assert.equal(confirmPricingTransition(true, () => { confirmations++; return true }, 'discard'), true)
assert.equal(confirmations, 2, 'dirty transitions explicitly require user confirmation')
assert.ok(source.includes('confirmPricingTransition(true, window.confirm'), 'mode switch must gate clearing draft fields')
const routing = readFileSync(new URL('./RoutingPage.tsx', import.meta.url), 'utf8')
assert.ok(routing.includes('confirmPricingTransition(Boolean(routeEditorDirtyCount(routeEditor))'), 'candidate navigation guards unsaved editor')
console.log('six admin pricing review regressions passed')
