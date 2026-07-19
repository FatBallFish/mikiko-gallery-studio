import {
  accountDraftFromView,
  emptyTextModelAccountDraft,
  modelDraftFromView,
  textModelAccountRequest,
  textModelModelRequest,
  validateTextModelAccountDraft,
  validateTextModelDraft,
} from './textModelRows'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('./TextModelsPage.tsx', import.meta.url), 'utf8')
if (!pageSource.includes('className="flex min-w-0 flex-1 overflow-hidden"')) {
  throw new Error('text model account labels must shrink and clip inside the account sidebar')
}
if (!pageSource.includes("cn('flex min-h-12 min-w-0 w-full")) {
  throw new Error('text model account buttons must allow the sidebar grid track to shrink')
}

const empty = emptyTextModelAccountDraft()
if (empty.platformType !== 'openai_compatible' || empty.apiStyle !== 'responses' || empty.enabled) throw new Error('new account defaults drifted')

const account = accountDraftFromView({
  id: 1, name: 'Primary', platform_type: 'openai_compatible', api_style: 'chat_completions', base_url: 'https://text.example.com/v1', enabled: true,
  secret_status: { has_secret: true, fingerprint: 'abcd1234' }, version: 3, created_at: '', updated_at: '',
})
if (validateTextModelAccountDraft(account)) throw new Error('valid account draft rejected')
const preserved = textModelAccountRequest(account)
if (preserved.secrets || preserved.clear_secrets) throw new Error('unchanged secret must be omitted')
const replaced = textModelAccountRequest({ ...account, secretMode: 'replace', apiKey: 'sk-new' })
if (replaced.secrets?.api_key !== 'sk-new') throw new Error('replacement secret missing')
const cleared = textModelAccountRequest({ ...account, enabled: false, secretMode: 'clear' })
if (cleared.clear_secrets?.[0] !== 'api_key') throw new Error('clear secret intent missing')
if (!validateTextModelAccountDraft({ ...account, baseUrl: 'file:///tmp/key' })) throw new Error('unsafe URL must be rejected')

const model = modelDraftFromView({
  id: 2, account_id: 1, model_code: 'gpt-test', display_name: 'GPT Test', input_price_per_million_tokens: '1.25',
  output_price_per_million_tokens: '10', currency: 'usd', enabled: true, is_default: false, version: 4, created_at: '', updated_at: '',
})
if (validateTextModelDraft(model)) throw new Error('valid model draft rejected')
const request = textModelModelRequest(model)
if (request.input_price_per_million_tokens !== '1.250000' || request.output_price_per_million_tokens !== '10.000000' || request.currency !== 'USD') throw new Error('model prices were not normalized')
if (!validateTextModelDraft({ ...model, inputPrice: '-1' })) throw new Error('negative model price must be rejected')
