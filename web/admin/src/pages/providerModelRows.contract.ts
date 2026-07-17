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
    task_types: ['text_to_image', 'reference_to_image', 'image_edit'],
    base_resolution: ['auto', '1K', '2K'],
    cost_per_image: '0.12000',
    currency: 'USD',
  }),
  '文生图/参考生图/图片编辑 · auto/1K/2K · 0.12000 USD',
  'model capability summary',
)

for (const primitive of ['FilterToolbar', 'DataTable', 'ActionMenu', 'Drawer', 'Modal', 'Badge']) {
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

if (!providerModelsSource.includes("accountPrimaryActionLabel = '查看模型'")) {
  throw new Error('account rows must expose one persistent primary action')
}

if (!providerModelsSource.includes('setExpandedAccountId((current) =>')) {
  throw new Error('provider master-detail should preserve the current account or select the first account after loading')
}

if (!providerModelsSource.includes('filteredAccounts.find')) {
  throw new Error('provider detail must stay inside the currently visible filtered account set')
}

if (!providerModelsSource.includes('message={mutationError}')) {
  throw new Error('provider account and model mutations should expose local failure feedback')
}

for (const createContinuation of ['const saved =', 'await load(String(saved.id))', 'setModelDialog(newModelDraft(saved))']) {
  if (!providerModelsSource.includes(createContinuation)) {
    throw new Error(`new provider accounts should continue into model setup with ${createContinuation}`)
  }
}

for (const menuAction of ["id: 'edit-account'", "id: 'add-model'", "id: 'test-account'"]) {
  if (!providerModelsSource.includes(menuAction)) {
    throw new Error(`secondary account operation must move into ActionMenu: ${menuAction}`)
  }
}

if (!providerModelsSource.includes("<Drawer\n          title={modelDialog.row ? '编辑真实模型' : '新增真实模型'}")) {
  throw new Error('the long real-model editor must use a Drawer')
}

if (!providerModelsSource.includes("<Drawer\n          title={accountDialog.id ? '编辑模型账号' : '新增模型账号'}")) {
  throw new Error('the multi-field provider account editor must use a Drawer')
}

if (!providerModelsSource.includes('<Modal title="测试模型账号"')) {
  throw new Error('the bounded model test request must remain in a Modal')
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
