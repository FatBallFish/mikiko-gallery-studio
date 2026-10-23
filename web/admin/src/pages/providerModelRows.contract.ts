import {
  credentialsStatusLabel,
  modelAccountStatusLabel,
  modelAccountStatusTone,
  modelCapabilitySummary,
  modelEnabledLabel,
  modelEnabledTone,
  providerAccountDialogDetail,
  providerAdapterLabel,
  providerAuthLabel,
} from './providerModelRows'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const providerModelsSource = readFileSync(new URL('./ProviderModelsPage.tsx', import.meta.url), 'utf8')

assertEqual(providerAdapterLabel('openai_compatible'), 'OpenAI 兼容', 'openai compatible adapter label')
assertEqual(providerAdapterLabel('openrouter'), 'OpenRouter', 'openrouter adapter label')
assertEqual(providerAdapterLabel('gemini'), 'gemini', 'unknown adapter fallback')
assertEqual(providerAdapterLabel(''), '未知接入', 'empty adapter fallback')

assertEqual(providerAuthLabel('api_key'), 'API Key', 'api key auth label')
assertEqual(providerAuthLabel('oauth'), 'oauth', 'unknown auth fallback')
assertEqual(providerAuthLabel(''), '未知鉴权', 'empty auth fallback')

const accountDialogDetail = providerAccountDialogDetail()
if (!accountDialogDetail.includes('API Key')) {
  throw new Error(`model account dialog detail should guide the current API Key setup path, got ${accountDialogDetail}`)
}
if (/后续|暂未|即将|版本|预留/.test(accountDialogDetail)) {
  throw new Error(`model account dialog detail should not expose roadmap wording, got ${accountDialogDetail}`)
}

assertEqual(modelAccountStatusLabel('enabled'), '启用', 'enabled account status label')
assertEqual(modelAccountStatusTone('enabled'), 'success', 'enabled account status tone')
assertEqual(modelAccountStatusLabel('disabled'), '停用', 'disabled account status label')
assertEqual(modelAccountStatusTone('disabled'), 'warning', 'disabled account status tone')
assertEqual(modelAccountStatusLabel('error'), '异常', 'error account status label')
assertEqual(modelAccountStatusTone('error'), 'danger', 'error account status tone')
assertEqual(modelAccountStatusLabel('cooldown'), 'cooldown', 'unknown account status fallback')
assertEqual(modelAccountStatusTone('cooldown'), 'neutral', 'unknown account status tone')

assertEqual(credentialsStatusLabel(true), 'API Key 已配置', 'configured credential label')
assertEqual(credentialsStatusLabel(false), '未配置密钥', 'missing credential label')
assertEqual(modelEnabledLabel(true), '启用', 'enabled model label')
assertEqual(modelEnabledTone(true), 'success', 'enabled model tone')
assertEqual(modelEnabledLabel(false), '停用', 'disabled model label')
assertEqual(modelEnabledTone(false), 'warning', 'disabled model tone')

assertEqual(
  modelCapabilitySummary({
    task_types: ['text_to_image', 'image_edit'],
    base_resolution: ['auto', '1K', '2K'],
    cost_per_image: '0.12000',
    currency: 'USD',
  }),
  '文生图/图片编辑 · auto/1K/2K · 0.12000 USD',
  'model capability summary',
)

for (const primitive of ['FilterToolbar', 'DataTable', 'Drawer', 'Modal', 'Badge']) {
  if (!providerModelsSource.includes(`<${primitive}`)) {
    throw new Error(`provider model operations must use the shared ${primitive} primitive`)
  }
}

for (const operationContract of [
  'adminApi.listModelAccounts',
  'adminApi.listModelAccountModels',
  'adminApi.createModelAccount',
  'adminApi.updateModelAccount',
  'adminApi.createModelAccountModel',
  'adminApi.updateModelAccountModel',
  'adminApi.testModelAccountImage',
]) {
  if (!providerModelsSource.includes(operationContract)) {
    throw new Error(`provider model redesign must preserve ${operationContract}`)
  }
}

if (!providerModelsSource.includes("accountPrimaryActionLabel = '模型配置'")) {
  throw new Error('account rows must expose one persistent primary action')
}

if (!providerModelsSource.includes('setModelAccountId((current) =>')) {
  throw new Error('provider model drawer should preserve the current account or clear it after loading')
}

if (!providerModelsSource.includes('const filteredAccounts = useMemo')) {
  throw new Error('provider list must stay inside the currently visible filtered account set')
}

if (!providerModelsSource.includes('message={mutationError}')) {
  throw new Error('provider account and model mutations should expose local failure feedback')
}

for (const createContinuation of ['const saved =', 'await load(String(saved.id))', 'setModelAccountId(String(saved.id))']) {
  if (!providerModelsSource.includes(createContinuation)) {
    throw new Error(`new provider accounts should continue into model setup with ${createContinuation}`)
  }
}

for (const rowAction of ['onConfigureModels(account)', 'onEdit(account)', 'onTest(account)']) {
  if (!providerModelsSource.includes(rowAction)) {
    throw new Error(`account row must expose a direct row action: ${rowAction}`)
  }
}

if (!providerModelsSource.includes("<Drawer\n          stacked={Boolean(modelAccount)}")) {
  throw new Error('the long real-model editor must use a stacked Drawer')
}

if (!providerModelsSource.includes("title={accountDialog.id ? '编辑图片接入账号' : '新增图片接入账号'}")) {
  throw new Error('the multi-field provider account editor must use a Drawer')
}

if (!providerModelsSource.includes('title="测试图片模型"')) {
  throw new Error('the model test request must remain a real capability-driven form')
}

if (providerModelsSource.includes('测试连接')) {
  throw new Error('the image account drawer must not expose the unusable API connection test button')
}

for (const marker of [
  'counts={counts}',
  'className="cfg-account-table"',
  'className="a-skeleton cfg-test-media__frame"',
  'className="cfg-test-progress"',
  'className="cfg-test-media__image"',
]) {
  if (!providerModelsSource.includes(marker)) {
    throw new Error(`access account pages must keep the structural UI contract: ${marker}`)
  }
}

if (providerModelsSource.includes('JSON.stringify(testDialog.result.actual_params')) {
  throw new Error('image test results must render media and metadata instead of raw JSON')
}

for (const drift of ['rounded-3xl', 'uppercase tracking-']) {
  if (providerModelsSource.includes(drift)) {
    throw new Error(`provider model workspace must remove visual drift: ${drift}`)
  }
}

assertEqual(
  modelCapabilitySummary({
    task_types: [],
    base_resolution: [],
    cost_per_image: '0.00000',
    currency: 'USD',
  }),
  '未配置任务类型 · 未配置基础分辨率 · 0.00000 USD',
  'empty capability summary',
)

function assertEqual(actual: string, expected: string, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${expected}, got ${actual}`)
  }
}
