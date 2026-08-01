import { cashierPlanDraftFromRow, cashierPlanEmptyDraft, cashierPlanPayloadFromDraft } from './cashierPlanDraft'

const empty = cashierPlanEmptyDraft()
if (empty.plan_code !== '' || empty.plan_type !== 'points_package' || !empty.purchase_enabled || empty.currency !== 'CNY') {
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
  duration_days: 60,
  currency: 'CNY',
  sort_order: 20,
  description: 'fixture',
  created_at: '2026-08-02T00:00:00Z',
  updated_at: '2026-08-02T00:00:00Z',
})
if (draft.row?.id !== 7 || draft.duration_days !== '60' || draft.sort_order !== '20') {
  throw new Error(`cashier plan row did not map to an editable draft: ${JSON.stringify(draft)}`)
}

const payload = cashierPlanPayloadFromDraft({ ...draft, plan_type: 'subscription', purchase_enabled: true })
if (payload.purchase_enabled !== false || payload.duration_days !== 60 || payload.sort_order !== 20) {
  throw new Error(`cashier plan draft did not map to the write payload: ${JSON.stringify(payload)}`)
}
