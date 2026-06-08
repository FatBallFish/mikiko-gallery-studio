import { ADMIN_PERMISSIONS, type ConfigItem } from '../../../shared/api-types'
import {
  configFieldMeta,
  configLockedDetail,
  configPermission,
  configTabMeta,
  configTabSummary,
  configValidateValue,
  extractConfigValue,
  isSameConfigValue,
} from './configRows'

if (configPermission('auth_security') !== ADMIN_PERMISSIONS.manageDangerousConfig || configPermission('payments') !== ADMIN_PERMISSIONS.manageDangerousConfig) {
  throw new Error('auth and payments config tabs should require dangerous config permission')
}

if (configPermission('generation_limits') !== ADMIN_PERMISSIONS.manageConfig || configPermission('billing_pricing') !== ADMIN_PERMISSIONS.manageConfig) {
  throw new Error('ordinary config tabs should require manage config permission')
}

const dangerousCopy = configLockedDetail(ADMIN_PERMISSIONS.manageDangerousConfig)
if (!dangerousCopy.includes('支付') || !dangerousCopy.includes('认证') || !dangerousCopy.includes('密钥') || !dangerousCopy.includes('超级管理员')) {
  throw new Error(`dangerous config locked copy should explain risk and required role, got ${dangerousCopy}`)
}

const ordinaryCopy = configLockedDetail(ADMIN_PERMISSIONS.manageConfig)
if (!ordinaryCopy.includes('保存') || !ordinaryCopy.includes('权限')) {
  throw new Error(`ordinary config locked copy should be operator-facing, got ${ordinaryCopy}`)
}

const paymentTab = configTabMeta('payments')
if (paymentTab.label !== '支付配置' || !paymentTab.detail.includes('底层支付配置') || !paymentTab.detail.includes('收银台')) {
  throw new Error(`payments tab should explain cashier boundary, got ${JSON.stringify(paymentTab)}`)
}

const unknownTab = configTabMeta('future_tab')
if (unknownTab.label !== 'future_tab' || !unknownTab.detail.includes('后端配置中心')) {
  throw new Error(`unknown tabs should preserve raw key for troubleshooting, got ${JSON.stringify(unknownTab)}`)
}

const signupTrial = configFieldMeta('signup_trial')
if (signupTrial.label !== '注册送体验额度' || signupTrial.type !== 'map' || !signupTrial.hint.includes('金额') || !signupTrial.hint.includes('有效期')) {
  throw new Error(`signup trial config should be operator-facing, got ${JSON.stringify(signupTrial)}`)
}

const visibleMethods = configFieldMeta('visible_methods')
if (visibleMethods.label !== '可见支付方式' || visibleMethods.type !== 'list' || !visibleMethods.hint.includes('收银台')) {
  throw new Error(`visible methods config should explain cashier payment entrance, got ${JSON.stringify(visibleMethods)}`)
}

const customAmount = configFieldMeta('custom_amount_min_cny')
if (!customAmount.hint.includes('自定义金额') || !customAmount.hint.includes('最低')) {
  throw new Error(`custom amount hint should be product-facing, got ${JSON.stringify(customAmount)}`)
}

const unknownField = configFieldMeta('future_key', 'backend description')
if (unknownField.label !== 'future_key' || unknownField.hint !== 'backend description') {
  throw new Error(`unknown config keys should preserve raw key and backend description, got ${JSON.stringify(unknownField)}`)
}

const rawOnlyField = configFieldMeta('raw_key')
if (rawOnlyField.label !== 'raw_key' || !rawOnlyField.hint.includes('后端配置中心')) {
  throw new Error(`unknown config keys without description should have clear fallback, got ${JSON.stringify(rawOnlyField)}`)
}

const extracted = extractConfigValue(configItem({ config_value: { value: { enabled: true, points: '20.00000' } } }))
if (JSON.stringify(extracted) !== JSON.stringify({ enabled: true, points: '20.00000' })) {
  throw new Error(`extractConfigValue should unwrap config_value.value, got ${JSON.stringify(extracted)}`)
}

const parsed = extractConfigValue(configItem({ value: '{"value":["mock","alipay"]}' }))
if (JSON.stringify(parsed) !== JSON.stringify(['mock', 'alipay'])) {
  throw new Error(`extractConfigValue should parse legacy string JSON values, got ${JSON.stringify(parsed)}`)
}

if (!isSameConfigValue({ a: 1 }, { a: 1 }) || isSameConfigValue({ a: 1 }, { a: 2 })) {
  throw new Error('isSameConfigValue should compare structured config values')
}

if (configValidateValue('max_image_count', 9) !== '数量超过当前安全上限 8，请降低数量或先扩容任务处理能力。') {
  throw new Error('max image count validation should keep safety guard')
}

if (configValidateValue('custom_amount_min_cny', 0) !== '金额必须大于 0。') {
  throw new Error('custom recharge minimum should be positive')
}

if (configValidateValue('signup_trial', { enabled: true, points: '20.00000', valid_days: 0 }) !== '体验额度有效期必须大于 0 天。') {
  throw new Error('signup trial valid_days should be validated')
}

if (configValidateValue('refresh_token_ttl_sec', 120) !== 'Token TTL 低于 300 秒会触发频繁刷新。') {
  throw new Error('token ttl validation should keep security guard')
}

const summary = configTabSummary([
  configItem({ config_category: 'payments', config_key: 'enabled' }),
  configItem({ config_category: 'payments', config_key: 'visible_methods' }),
  configItem({ config_category: 'generation_limits', config_key: 'max_image_count' }),
])
if (summary.tabCount !== 2 || summary.fieldCount !== 3 || summary.dangerousTabCount !== 1) {
  throw new Error(`config summary should count tabs, fields and dangerous tabs, got ${JSON.stringify(summary)}`)
}

const allVisibleCopy = [
  paymentTab.detail,
  signupTrial.hint,
  visibleMethods.hint,
  customAmount.hint,
].join(' ')
if (/custom_amount_min_cny|visible_methods|signup_trial|raw/i.test(allVisibleCopy)) {
  throw new Error(`config page copy should not expose raw key names in known field hints, got ${allVisibleCopy}`)
}

function configItem(patch: Partial<ConfigItem>): ConfigItem {
  return {
    config_category: patch.config_category ?? 'payments',
    config_key: patch.config_key ?? 'enabled',
    config_value: patch.config_value,
    value: patch.value ?? '',
    scope: patch.scope ?? 'global',
    version: patch.version ?? 1,
    tab: patch.tab ?? patch.config_category ?? 'payments',
    key: patch.key ?? patch.config_key ?? 'enabled',
    description: patch.description ?? '',
    draft_value: patch.draft_value ?? '',
    state: patch.state ?? 'active',
  }
}
