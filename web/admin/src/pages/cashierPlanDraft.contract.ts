import { cashierPlanDraftFromRow, cashierPlanEmptyDraft, cashierPlanPayloadFromDraft } from './cashierPlanDraft'

const empty = cashierPlanEmptyDraft()
if (empty.plan_code !== '' || empty.plan_type !== 'points_package' || !empty.purchase_enabled || !empty.credit_expiry_enabled || 'currency' in empty) {
  throw new Error(`unexpected empty cashier plan draft: ${JSON.stringify(empty)}`)
}

const draft = cashierPlanDraftFromRow({
  id: 7,
  plan_code: 'points-200',
  plan_name: '200 积分包',
  plan_type: 'points_package',
  purchase_enabled: true,
  status: 'active',
  price_cny: '39.90000',
  points: '200.00000',
  bonus_points: '20.00000',
  credit_expiry_enabled: false,
  duration_days: 60,
  currency: 'CNY',
  sort_order: 20,
  description: 'fixture',
  created_at: '2026-08-02T00:00:00Z',
  updated_at: '2026-08-02T00:00:00Z',
})
if (draft.row?.id !== 7 || draft.credit_expiry_enabled || draft.duration_days !== '' || draft.sort_order !== '20' || 'currency' in draft) {
  throw new Error(`cashier plan row did not map to an editable draft: ${JSON.stringify(draft)}`)
}

const payload = cashierPlanPayloadFromDraft({ ...draft, plan_type: 'subscription', purchase_enabled: true })
if (payload.purchase_enabled !== false || payload.credit_expiry_enabled !== false || payload.duration_days !== null || payload.sort_order !== 20 || 'currency' in payload) {
  throw new Error(`cashier plan draft did not map to the write payload: ${JSON.stringify(payload)}`)
}
