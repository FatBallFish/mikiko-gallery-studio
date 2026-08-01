import type { CashierPlan } from '../../../shared/api-types'
import { cashierPlanSavePayload } from './cashierPlanPurchase'

export type CashierPlanDraft = {
  row?: CashierPlan
  plan_code: string
  plan_name: string
  plan_type: string
  purchase_enabled: boolean
  status: string
  price_cny: string
  points: string
  bonus_points: string
  duration_days: string
  currency: string
  sort_order: string
  description: string
}

export function cashierPlanEmptyDraft(): CashierPlanDraft {
  return {
    plan_code: '',
    plan_name: '',
    plan_type: 'points_package',
    purchase_enabled: true,
    status: 'active',
    price_cny: '19.90000',
    points: '100.00000',
    bonus_points: '0.00000',
    duration_days: '30',
    currency: 'CNY',
    sort_order: '10',
    description: '',
  }
}

export function cashierPlanDraftFromRow(row: CashierPlan): CashierPlanDraft {
  return {
    row,
    plan_code: row.plan_code,
    plan_name: row.plan_name,
    plan_type: row.plan_type ?? 'points_package',
    purchase_enabled: Boolean(row.purchase_enabled),
    status: row.status,
    price_cny: row.price_cny,
    points: row.points,
    bonus_points: row.bonus_points,
    duration_days: String(row.duration_days),
    currency: row.currency,
    sort_order: String(row.sort_order ?? 0),
    description: row.description ?? '',
  }
}

export function cashierPlanPayloadFromDraft(draft: CashierPlanDraft): Partial<CashierPlan> {
  return cashierPlanSavePayload(draft)
}

export function cashierPlanDraftCanSave(draft: CashierPlanDraft): boolean {
  return Boolean(draft.plan_code.trim() && draft.plan_name.trim() && draft.price_cny.trim() && draft.points.trim())
}
