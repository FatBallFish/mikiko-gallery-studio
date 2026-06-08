import { cashierTrialConfigDraft, cashierTrialConfigDraftDetail, cashierTrialConfigPayload, cashierTrialConfigSummary } from './cashierTrialConfig'

const summary = cashierTrialConfigSummary([{
  tab_key: 'trial_credits',
  tab_name: '体验额度',
  version: 1,
  items: [{
    tab: 'trial_credits',
    key: 'signup_trial',
    value: '',
    draft_value: '',
    state: 'active',
    version: 1,
    description: '注册送体验额度',
    config_category: 'billing_trial',
    config_key: 'signup_trial',
    config_value: {
      value: {
        enabled: true,
        points: '18.00000',
        valid_days: 9,
        expiry_reminder_days: 4,
        grant_once_per_user: true,
      },
    },
    scope: 'global',
  }],
}])

if (!summary.enabled || summary.statusLabel !== '已启用') {
  throw new Error(`cashier trial config should expose enabled status, got ${JSON.stringify(summary)}`)
}
for (const fragment of ['18.00000', '9 天', '提前 4 天', '仅领取一次']) {
  if (!summary.detail.includes(fragment)) {
    throw new Error(`cashier trial config detail should include ${fragment}, got ${summary.detail}`)
  }
}
if (summary.tabKey !== 'trial_credits' || summary.configCategory !== 'billing_trial' || summary.configKey !== 'signup_trial') {
  throw new Error(`cashier trial config should expose editable config contract, got ${JSON.stringify(summary)}`)
}

const draft = cashierTrialConfigDraft(summary)
if (draft.points !== '18.00000' || draft.valid_days !== '9' || draft.expiry_reminder_days !== '4' || !draft.grant_once_per_user) {
  throw new Error(`cashier trial config draft should preserve editable values, got ${JSON.stringify(draft)}`)
}
if (!cashierTrialConfigDraftDetail({ ...draft, enabled: false }).includes('18.00000')) {
  throw new Error('cashier trial config draft detail should describe the editable values')
}

const payload = cashierTrialConfigPayload(summary, {
  enabled: false,
  points: '25.00000',
  valid_days: '14',
  expiry_reminder_days: '3',
  grant_once_per_user: false,
})
const value = payload.items[0]?.config_value?.value as Record<string, unknown> | undefined
if (payload.version !== 1 || payload.items[0]?.config_category !== 'billing_trial' || payload.items[0]?.config_key !== 'signup_trial') {
  throw new Error(`cashier trial config payload should target trial_credits signup_trial, got ${JSON.stringify(payload)}`)
}
if (value?.enabled !== false || value.points !== '25.00000' || value.valid_days !== 14 || value.expiry_reminder_days !== 3 || value.grant_once_per_user !== false) {
  throw new Error(`cashier trial config payload should normalize editable values, got ${JSON.stringify(payload)}`)
}
