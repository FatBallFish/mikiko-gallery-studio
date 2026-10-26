import type { NativeImagePricing, NativeImageProfile } from '../../../shared/native-image'
import { LedgerField, SwitchField } from './AdminMediaTabs'
import { configObject, patchNativePricing, pricingErrors, pricingIdentityMismatch, resetNativePricing, nativeLayerCostMissing } from './nativeImageConfig'

export function NativeImagePricingFields({ label, value, model, profile, enabled, onChange }: { label: string; value?: NativeImagePricing; model: string; profile?: NativeImageProfile; enabled: boolean; onChange: (value: NativeImagePricing) => void }) {
  const pricing = configObject(value)
  const mismatch = pricingIdentityMismatch(pricing, model)
  const version = profile?.version ?? '2026-10-25'
  const patch = (next: Record<string, unknown>) => {
    const updated = patchNativePricing(pricing, model, version, next)
    if (updated) onChange(updated as NativeImagePricing)
  }
  if (mismatch) return <fieldset className="cfg-native-pricing"><legend>{label} · 价格身份不匹配</legend>
    <p className="a-field__error" role="alert">现有价格型号：{String(pricing.model ?? '未标明')}；币种：{String(pricing.currency ?? '未标明')}。当前型号：{model || '未选择'}，要求 CNY。已阻止编辑，原金额不会被重标或换算。</p>
    <p className="a-hint">重置会清空输入价、免费输入数、普通与拆层档位的已知价格字段及确认状态；保留未知扩展字段。重填完整价格前无法启用。</p>
    <button className="a-btn a-btn--sm" type="button" disabled={!model} onClick={() => { if (window.confirm('确认清空现有已知价格，并为当前型号建立 CNY 草稿？未知扩展字段会保留，不会换算原金额。')) onChange(resetNativePricing(pricing, model, version) as NativeImagePricing) }}>清空已知价格并重新配置</button>
  </fieldset>
  const numeric = (text: string) => text === '' ? undefined : Number(text)
  const layers = profile?.layer_decomposition === true && profile.operations.includes('layer_decomposition')
  return <fieldset className="cfg-native-pricing"><legend>{label}（CNY）</legend>
    <p className="a-hint">此处仅为厂商人民币成本，不是用户销售积分。统一售价在路由维护，积分换算使用现有 FX；不修改运行汇率。禁用模型可保存未完成成本草稿。</p>
    <div className="a-form__pair">
      <LedgerField label={`${label}输入单价（元 / 张）`}><input className="a-input" aria-label={`${label}输入单价`} type="number" min="0" step="any" value={typeof pricing.input_cny === 'number' && Number.isFinite(pricing.input_cny) ? pricing.input_cny : ''} onChange={(event) => patch({ input_cny: numeric(event.target.value) })} /></LedgerField>
      <LedgerField label={`${label}免费输入张数`}><input className="a-input" aria-label={`${label}免费输入张数`} type="number" min="0" step="1" value={typeof pricing.free_input_images === 'number' ? pricing.free_input_images : ''} onChange={(event) => patch({ free_input_images: numeric(event.target.value) })} /></LedgerField>
    </div>
    {(['output_tiers', ...(layers ? ['layer_tiers'] : [])] as Array<'output_tiers' | 'layer_tiers'>).map((field) => {
      const tiers = Array.isArray(pricing[field]) ? (pricing[field] as unknown[]).map(configObject) : []
      const changeTier = (index: number, key: string, value: unknown) => patch({ [field]: tiers.map((tier, i) => i === index ? { ...tier, [key]: value } : tier) })
      return <div key={field} className="cfg-tier-group"><h4>{field === 'output_tiers' ? '普通输出阶梯' : '拆层输出阶梯'}</h4>
        <p className="a-hint">按输出总像素选择首个满足上限的档位，按该档整张计价，不是累进计费。上限含边界，须严格递增；末档覆盖型号最大 {profile?.max_pixels ?? '待选择'} 像素。</p>
        <div className="cfg-tier-scroll"><table className="cfg-tier-table"><thead><tr><th>档名</th><th>总像素上限（含）</th><th>像素区间</th><th>元 / 每张</th><th>操作</th></tr></thead><tbody>{tiers.map((tier, index) => <tr key={index}>
          <td><input className="a-input" aria-label={`${label}${field}${index + 1}档名`} value={String(tier.name ?? '')} onChange={(event) => changeTier(index, 'name', event.target.value)} /></td>
          <td><input className="a-input" aria-label={`${label}${field}${index + 1}总像素上限`} type="number" min="1" step="1" value={typeof tier.max_pixels === 'number' && Number.isFinite(tier.max_pixels) ? tier.max_pixels : ''} onChange={(event) => changeTier(index, 'max_pixels', numeric(event.target.value))} /></td>
          <td className="a-hint">({index ? String(tiers[index - 1].max_pixels ?? '?') : '0'}, {String(tier.max_pixels ?? '?')}]</td>
          <td><input className="a-input" aria-label={`${label}${field}${index + 1}元每张`} type="number" min="0" step="any" value={typeof tier.cny === 'number' && Number.isFinite(tier.cny) ? tier.cny : ''} onChange={(event) => changeTier(index, 'cny', numeric(event.target.value))} /></td>
          <td><button type="button" className="a-row-btn a-row-btn--danger" onClick={() => patch({ [field]: tiers.filter((_, i) => i !== index) })}>删除</button></td>
        </tr>)}</tbody></table></div>
        <button type="button" className="a-btn a-btn--sm" onClick={() => patch({ [field]: [...tiers, { name: '', max_pixels: Math.max(Number(tiers[tiers.length - 1]?.max_pixels ?? 0) + 1, profile?.max_pixels ?? 1) }] })}>添加阶梯</button>
      </div>
    })}
    {layers ? <>
      <LedgerField label="拆层底图单价（元 / 张）" hint="留空为未确认，0 是已明确的零价。"><input className="a-input" aria-label={`${label}拆层底图单价`} type="number" min="0" step="any" value={typeof pricing.layer_base_cny === 'number' ? pricing.layer_base_cny : ''} onChange={(event) => patch({ layer_base_cny: numeric(event.target.value) })} /></LedgerField>
      <SwitchField label={`${label}已核实拆层费口径`} checked={pricing.layer_pricing_confirmed === true} onChange={(confirmed) => patch({ layer_pricing_confirmed: confirmed })} hint="成本和路由销售价格均完整确认、且型号支持时，运行时才能提供拆层。" />
    </> : <p className="a-hint">当前型号未开放拆层；已有扩展元数据保留。</p>}
    {nativeLayerCostMissing(pricing, profile).map((missing) => <p key={missing} className="a-field__error">拆层尚不可用，缺少：{missing}</p>)}
    {pricingErrors(pricing, profile, enabled).map((error) => <p key={error} className="a-field__error" role="alert">{error}</p>)}
  </fieldset>
}
