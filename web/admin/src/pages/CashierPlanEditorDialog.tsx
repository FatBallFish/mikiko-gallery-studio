import { Button, Drawer, InlineFeedback } from '../components'
import { FormSection, LedgerField, SwitchField } from './AdminMediaTabs'
import { cashierPlanSectionCopy } from './cashierPlanPurchase'
import { cashierPlanDraftCanSave, type CashierPlanDraft } from './cashierPlanDraft'

export function CashierPlanEditorDialog({ draft, saving, error, onChange, onClose, onSave }: {
  draft: CashierPlanDraft
  saving: boolean
  error?: string | null
  onChange: (draft: CashierPlanDraft) => void
  onClose: () => void
  onSave: () => void
}) {
  const canSave = cashierPlanDraftCanSave(draft)
  return (
    <Drawer
      title={draft.row ? '编辑充值套餐' : '新增充值套餐'}
      description={cashierPlanSectionCopy.dialogDetail}
      headerAside={<span className={canSave ? 'a-badge a-badge--ok' : 'a-badge a-badge--warn'}><i />{canSave ? '可保存' : '待完善'}</span>}
      onClose={onClose}
      footer={<>
        <span className={canSave ? 'a-badge a-badge--ok' : 'a-badge a-badge--warn'}><i />{canSave ? '校验通过' : '存在未完成字段'}</span>
        <div style={{ flex: 1 }} />
        <Button variant="ghost" size="sm" disabled={saving} onClick={onClose}>取消</Button>
        <Button variant="primary" size="sm" disabled={saving || !canSave} onClick={onSave}>{saving ? '保存中...' : '保存套餐'}</Button>
      </>}
    >
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      <form className="a-form" onSubmit={(event) => event.preventDefault()}>
        <FormSection index={1} title="标识">
          <LedgerField label="套餐代码" required hint={draft.row ? '套餐代码创建后不可修改。' : '用于订单与账本快照。'}>
            <input className="a-input" value={draft.plan_code} disabled={Boolean(draft.row)} onChange={(event) => onChange({ ...draft, plan_code: event.target.value })} placeholder="points-100" style={{ fontFamily: 'var(--font-admin-mono)' }} />
          </LedgerField>
          <LedgerField label="套餐名称" required><input className="a-input" value={draft.plan_name} onChange={(event) => onChange({ ...draft, plan_name: event.target.value })} placeholder="100 积分包" /></LedgerField>
          <LedgerField label="套餐类型"><select className="a-select" value={draft.plan_type} onChange={(event) => onChange({ ...draft, plan_type: event.target.value, purchase_enabled: event.target.value === 'subscription' ? false : draft.purchase_enabled })}><option value="points_package">积分包</option><option value="subscription">{cashierPlanSectionCopy.subscriptionOptionLabel}</option></select></LedgerField>
          <LedgerField label="描述"><input className="a-input" value={draft.description} onChange={(event) => onChange({ ...draft, description: event.target.value })} placeholder="适合轻量体验" /></LedgerField>
        </FormSection>

        <FormSection index={2} title="价格与积分">
          <div className="a-form__pair">
            <LedgerField label="售价 CNY" required><input className="a-input" value={draft.price_cny} onChange={(event) => onChange({ ...draft, price_cny: event.target.value })} inputMode="decimal" placeholder="19.90000" /></LedgerField>
            <LedgerField label="基础积分" required><input className="a-input" value={draft.points} onChange={(event) => onChange({ ...draft, points: event.target.value })} inputMode="decimal" placeholder="100.00000" /></LedgerField>
          </div>
          <LedgerField label="赠送积分"><input className="a-input cfg-narrow-input" value={draft.bonus_points} onChange={(event) => onChange({ ...draft, bonus_points: event.target.value })} inputMode="decimal" placeholder="0.00000" /></LedgerField>
        </FormSection>

        <FormSection index={3} title="有效期与排序">
          <SwitchField label="按有效天数过期" checked={draft.credit_expiry_enabled} onChange={(credit_expiry_enabled) => onChange({ ...draft, credit_expiry_enabled, duration_days: credit_expiry_enabled ? draft.duration_days || '30' : '' })} hint={draft.credit_expiry_enabled ? undefined : '关闭后积分长期有效。'} />
          {draft.credit_expiry_enabled ? <LedgerField label="有效天数" required><input className="a-input cfg-narrow-input" value={draft.duration_days} onChange={(event) => onChange({ ...draft, duration_days: event.target.value })} type="number" min="1" required /></LedgerField> : null}
          <LedgerField label="排序"><input className="a-input cfg-narrow-input" value={draft.sort_order} onChange={(event) => onChange({ ...draft, sort_order: event.target.value })} type="number" /></LedgerField>
        </FormSection>
      </form>
    </Drawer>
  )
}