// @ts-nocheck
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { nativeImageProfile } from '../../../shared/native-image'
import { configValue, patchConfig, switchNativePreset, pricingErrors, nativeConfigurationErrors, patchNativePricing, resetNativePricing, pricingIdentityMismatch } from './nativeImageConfig'

const profile = nativeImageProfile('qwen-image-3.0')!
const raw = { future: { nested: [false, null, 0] }, native_image: { model: profile.model, profile: { ...profile, vendor_flag: { keep: true } }, defaults: { qwen: { future: 0, enable_thinking: null } }, pricing: { vendor: { opaque: true }, output_tiers: [{ name: 'old', max_pixels: 99, cny: 2, custom: { keep: false } }] } } }
const patched = patchConfig(raw, ['native_image', 'defaults', 'qwen', 'prompt_extend'], false)
assert.equal(configValue(patched, ['native_image', 'defaults', 'qwen', 'prompt_extend']), false)
assert.equal(configValue(patched, ['native_image', 'defaults', 'qwen', 'enable_thinking']), null)
assert.equal(configValue(patched, ['native_image', 'defaults', 'qwen', 'future']), 0)
assert.deepEqual(patched.future, raw.future)
assert.equal(configValue(raw, ['native_image', 'defaults', 'qwen', 'prompt_extend']), undefined, 'patch must not mutate source')
const seed = patchConfig(patched, ['native_image', 'defaults', 'seed'], 0)
assert.equal(configValue(seed, ['native_image', 'defaults', 'seed']), 0)
assert.equal(configValue(patchConfig(seed, ['native_image', 'defaults', 'seed'], undefined), ['native_image', 'defaults', 'seed']), undefined)
assert.deepEqual(switchNativePreset(raw, profile), raw, 'same model does not reset prices/profile')
const switched = switchNativePreset(raw, nativeImageProfile('qwen-image-2.1-pro')!)
assert.deepEqual(switched.native_image.profile.vendor_flag, { keep: true })
assert.deepEqual(switched.native_image.pricing.vendor, { opaque: true })
assert.deepEqual(switched.native_image.pricing.output_tiers, [{ custom: { keep: false } }])
assert.equal(switched.native_image.pricing.input_cny, undefined)
assert.deepEqual(switched.future, raw.future)
const rate = { currency: 'CNY', model: profile.model, version: profile.version, input_cny: 0, free_input_images: 0, output_tiers: [{ name: 'small', max_pixels: 1048576, cny: 0 }, { name: 'large', max_pixels: profile.max_pixels, cny: 0.3 }] }
assert.deepEqual(pricingErrors(rate, profile, true), [])
assert.deepEqual(pricingErrors(undefined, profile, false), [], 'disabled drafts may omit prices')
assert.ok(pricingErrors(undefined, profile, true).length)
for (const bad of [NaN, Infinity, -1, undefined, null]) assert.ok(pricingErrors({ ...rate, input_cny: bad }, profile, true).length)
assert.ok(pricingErrors({ ...rate, output_tiers: [{ name: 'a', max_pixels: profile.max_pixels, cny: 0 }, { name: 'b', max_pixels: profile.max_pixels, cny: 0 }] }, profile, true).some((error) => error.includes('递增')))
assert.ok(pricingErrors({ ...rate, output_tiers: [{ name: 'short', max_pixels: profile.max_pixels - 1, cny: 0 }] }, profile, true).some((error) => error.includes('覆盖')))
assert.ok(pricingErrors({ ...rate, layer_pricing_confirmed: true, layer_base_cny: 0, layer_tiers: rate.output_tiers }, profile, true).some((error) => error.includes('不支持拆层')))
assert.deepEqual(nativeConfigurationErrors({ native_image: { model: profile.model, defaults: { seed: 0 } } }, { native_image: { pricing: rate, sale_pricing: rate } }, profile.model, true), [])
assert.deepEqual(nativeConfigurationErrors({ native_image: { model: profile.model, pricing: rate } }, {}, profile.model, true), [], 'provider models validate costs only')
assert.ok(!pageSaleEditor())
function pageSaleEditor() { return readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8').includes("['pricing', 'sale_pricing']") }
const page = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')
assert.ok(!page.includes('模型扩展配置 JSON'))
assert.ok(!page.includes('JSON.parse(event.target.value)'))
const fields = readFileSync(new URL('./ImageConfigurationFields.tsx', import.meta.url), 'utf8')
assert.ok(fields.includes("['gpt_image_response_format']"))
assert.ok(!fields.includes("['send_response_format']"))
assert.ok(fields.includes('watermark 默认 false'))
assert.ok(fields.includes('收费资料未知'))
assert.ok(nativeConfigurationErrors({ native_image: { model: profile.model, profile: { ...profile, max_reference_images: profile.max_reference_images + 1 } } }, {}, profile.model, false).some((error) => error.includes('max_reference_images')))
assert.ok(nativeConfigurationErrors({ native_image: { model: profile.model, profile: { ...profile, output_formats: ['unsupported'] } } }, {}, profile.model, false).some((error) => error.includes('output_formats')))
assert.deepEqual(nativeConfigurationErrors({ native_image: { model: profile.model, profile: { ...profile, max_reference_images: 0, future: { keep: true } } } }, {}, profile.model, false), [])
const otherProfile = nativeImageProfile('qwen-image-2.1-pro')!
for (const inherited of [rate, { ...rate, currency: 'USD' }, { ...rate, model: otherProfile.model, currency: 'USD' }]) {
  const current = inherited === rate ? otherProfile : profile
  const renamed = { output_tiers: inherited.output_tiers.map((tier) => ({ ...tier, name: 'renamed' })) }
  assert.equal(pricingIdentityMismatch(inherited, current.model), true)
  assert.equal(patchNativePricing(inherited, current.model, current.version, renamed), undefined, 'renaming a tier must not relabel inherited model/currency')
  assert.ok(pricingErrors(inherited, current, true).length)
  const local = { native_image: { model: current.model } }
  assert.ok(nativeConfigurationErrors(local, { native_image: { pricing: inherited, sale_pricing: inherited } }, current.model, true).length, 'inherited A → B and foreign currency stay invalid')
}
const opaqueRate = { ...rate, currency: 'USD', vendor: { retained: false }, output_tiers: [{ ...rate.output_tiers[0], custom: { zero: 0 } }], layer_pricing_confirmed: true, layer_base_cny: 7 }
const reset = resetNativePricing(opaqueRate, otherProfile.model, otherProfile.version)
assert.deepEqual(reset.vendor, { retained: false })
assert.deepEqual(reset.output_tiers, [{ custom: { zero: 0 } }])
assert.equal(reset.input_cny, undefined)
assert.equal(reset.layer_base_cny, undefined)
assert.equal(reset.layer_pricing_confirmed, undefined)
assert.equal(reset.model, otherProfile.model)
assert.equal(reset.currency, 'CNY')
assert.ok(pricingErrors(reset, otherProfile, true).length, 'explicit reset still requires newly configured prices')
assert.equal(patchNativePricing(rate, profile.model, profile.version, { input_cny: 0.2 })?.input_cny, 0.2)
console.log('native image admin configuration contracts passed')
