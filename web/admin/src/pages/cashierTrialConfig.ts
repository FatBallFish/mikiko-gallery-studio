import type { ConfigTab } from '../../../shared/api-types'

export type CashierTrialConfigSummary = {
  enabled: boolean
  points: string
  validDays: number
  expiryReminderDays: number
  grantOncePerUser: boolean
  statusLabel: string
  detail: string
  tabKey: string
  version: number
  configCategory: string
  configKey: string
  scope: string
}

export type CashierTrialConfigDraft = {
  enabled: boolean
  points: string
  valid_days: string
  expiry_reminder_days: string
  grant_once_per_user: boolean
}

export function cashierTrialConfigSummary(tabs: ConfigTab[]): CashierTrialConfigSummary {
  const tab = tabs.find((row) => row.tab_key === 'trial_credits')
  const item = tab?.items.find((row) => row.config_key === 'signup_trial')
  const value = configValueObject(item?.config_value)
  const enabled = booleanValue(value.enabled, false)
  const points = stringValue(value.points, '0.00000')
  const validDays = numberValue(value.valid_days, 0)
  const expiryReminderDays = numberValue(value.expiry_reminder_days, 0)
  const grantOncePerUser = booleanValue(value.grant_once_per_user, true)
  return {
    enabled,
    points,
    validDays,
    expiryReminderDays,
    grantOncePerUser,
    statusLabel: enabled ? '已启用' : '未启用',
    detail: `注册赠送 ${points} 积分，有效期 ${validDays} 天，提前 ${expiryReminderDays} 天提醒${grantOncePerUser ? '，每个用户仅领取一次' : ''}。`,
    tabKey: tab?.tab_key ?? 'trial_credits',
    version: tab?.version ?? item?.version ?? 1,
    configCategory: item?.config_category ?? 'billing_trial',
    configKey: item?.config_key ?? 'signup_trial',
    scope: item?.scope ?? 'global',
  }
}

export function cashierTrialConfigDraft(summary: CashierTrialConfigSummary): CashierTrialConfigDraft {
  return {
    enabled: summary.enabled,
    points: summary.points,
    valid_days: summary.validDays > 0 ? String(summary.validDays) : '7',
    expiry_reminder_days: summary.expiryReminderDays > 0 ? String(summary.expiryReminderDays) : '2',
    grant_once_per_user: summary.grantOncePerUser,
  }
}

export function cashierTrialConfigPayload(summary: CashierTrialConfigSummary, draft: CashierTrialConfigDraft): {
  version: number
  items: Array<{ config_category: string; config_key: string; config_value: Record<string, unknown>; scope: string }>
} {
  return {
    version: summary.version,
    items: [{
      config_category: summary.configCategory,
      config_key: summary.configKey,
      config_value: {
        value: {
          enabled: draft.enabled,
          points: stringValue(draft.points, '0.00000'),
          valid_days: positiveIntValue(draft.valid_days, 7),
          expiry_reminder_days: nonNegativeIntValue(draft.expiry_reminder_days, 2),
          grant_once_per_user: draft.grant_once_per_user,
        },
      },
      scope: summary.scope,
    }],
  }
}

export function cashierTrialConfigDraftDetail(draft: CashierTrialConfigDraft) {
  return `注册赠送 ${stringValue(draft.points, '0.00000')} 积分，有效期 ${positiveIntValue(draft.valid_days, 7)} 天，提前 ${nonNegativeIntValue(draft.expiry_reminder_days, 2)} 天提醒${draft.grant_once_per_user ? '，每个用户仅领取一次' : ''}。`
}

function configValueObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const record = value as Record<string, unknown>
  const nested = record.value
  if (nested && typeof nested === 'object' && !Array.isArray(nested)) return nested as Record<string, unknown>
  return record
}

function stringValue(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

function numberValue(value: unknown, fallback: number) {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

function booleanValue(value: unknown, fallback: boolean) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string' && value.trim()) {
    if (value.trim().toLowerCase() === 'true') return true
    if (value.trim().toLowerCase() === 'false') return false
  }
  return fallback
}

function positiveIntValue(value: unknown, fallback: number) {
  const parsed = Math.trunc(numberValue(value, fallback))
  return parsed > 0 ? parsed : fallback
}

function nonNegativeIntValue(value: unknown, fallback: number) {
  const parsed = Math.trunc(numberValue(value, fallback))
  return parsed >= 0 ? parsed : fallback
}
