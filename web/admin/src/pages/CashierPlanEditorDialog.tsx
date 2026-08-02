import { cn } from '../../../shared/classnames'
import { Field, InlineFeedback, Modal } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { cashierPlanSectionCopy } from './cashierPlanPurchase'
import { cashierPlanDraftCanSave, type CashierPlanDraft } from './cashierPlanDraft'
import { cashierPlanStatusOptions } from './cashierStatusRows'

export function CashierPlanEditorDialog({ draft, saving, error, onChange, onClose, onSave }: {
  draft: CashierPlanDraft
  saving: boolean
  error?: string | null
  onChange: (draft: CashierPlanDraft) => void
  onClose: () => void
  onSave: () => void
}) {
  return (
    <Modal
      title={draft.row ? '编辑充值套餐' : '新增充值套餐'}
      detail={cashierPlanSectionCopy.dialogDetail}
      onClose={onClose}
      footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={onClose}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !cashierPlanDraftCanSave(draft)} onClick={onSave}>{saving ? '保存中...' : '保存'}</button></>}
    >
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      <div className={adminPage.formGrid}>
        <Field label="套餐代码"><input value={draft.plan_code} disabled={Boolean(draft.row)} onChange={(event) => onChange({ ...draft, plan_code: event.target.value })} placeholder="points-100" /></Field>
        <Field label="套餐名称"><input value={draft.plan_name} onChange={(event) => onChange({ ...draft, plan_name: event.target.value })} placeholder="100 积分包" /></Field>
        <Field label="套餐类型"><select value={draft.plan_type} onChange={(event) => onChange({ ...draft, plan_type: event.target.value, purchase_enabled: event.target.value === 'subscription' ? false : draft.purchase_enabled })}><option value="points_package">积分包</option><option value="subscription">{cashierPlanSectionCopy.subscriptionOptionLabel}</option></select></Field>
        <Field label="状态"><select value={draft.status} onChange={(event) => onChange({ ...draft, status: event.target.value })}>{cashierPlanStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        <Field label="售价 CNY"><input value={draft.price_cny} onChange={(event) => onChange({ ...draft, price_cny: event.target.value })} inputMode="decimal" placeholder="19.90000" /></Field>
        <Field label="基础积分"><input value={draft.points} onChange={(event) => onChange({ ...draft, points: event.target.value })} inputMode="decimal" placeholder="100.00000" /></Field>
        <Field label="赠送积分"><input value={draft.bonus_points} onChange={(event) => onChange({ ...draft, bonus_points: event.target.value })} inputMode="decimal" placeholder="0.00000" /></Field>
        <Field label="有效天数"><input value={draft.duration_days} onChange={(event) => onChange({ ...draft, duration_days: event.target.value })} type="number" min="1" /></Field>
        <Field label="币种"><input value={draft.currency} onChange={(event) => onChange({ ...draft, currency: event.target.value })} /></Field>
        <Field label="排序"><input value={draft.sort_order} onChange={(event) => onChange({ ...draft, sort_order: event.target.value })} type="number" /></Field>
        <label className="flex items-center gap-2 text-sm font-semibold text-[var(--text)]">
          <input type="checkbox" checked={draft.plan_type !== 'subscription' && draft.purchase_enabled} disabled={draft.plan_type === 'subscription'} onChange={(event) => onChange({ ...draft, purchase_enabled: event.target.checked })} />
          <span>允许用户购买</span>
        </label>
        <Field label="描述"><input value={draft.description} onChange={(event) => onChange({ ...draft, description: event.target.value })} placeholder="适合轻量体验" /></Field>
      </div>
    </Modal>
  )
}
