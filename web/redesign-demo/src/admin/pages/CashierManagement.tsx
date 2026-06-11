import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const CashierManagement: React.FC = () => {
  return (
    <div className="space-y-10">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <div className={rdAdmin.chartContainer}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>支付通道管理 / Payment Providers</h3>
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "text-xs")}>添加通道</button>
          </div>
          <div className="space-y-4">
            <ProviderItem name="Alipay (Direct)" status="online" type="支付宝直连" />
            <ProviderItem name="WeChat Pay (Direct)" status="online" type="微信直连" />
            <ProviderItem name="JeePay Aggregate" status="warning" label="API Key Expiring" type="聚合支付" />
            <ProviderItem name="Stripe (Global)" status="offline" type="境外支付" />
          </div>
        </div>

        <div className={rdAdmin.chartContainer}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>收银台展示设置 / UI Settings</h3>
          </div>
          <div className="space-y-4">
            <ToggleSetting title="允许自定义充值金额" detail="开启后用户可输入任意金额按比例兑换积分" enabled />
            <ToggleSetting title="展示积分赠送标签" detail="在套餐卡片上显示 '+20% Bonus' 等字样" enabled />
            <ToggleSetting title="强制实名认证后支付" detail="未认证用户发起支付时将跳转认证页面" />
            <ToggleSetting title="启用支付倒计时" detail="订单创建后 15 分钟内未支付自动关闭" enabled />
          </div>
        </div>
      </div>

      <div className={rdAdmin.chartContainer}>
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>风控与对账 / Risk & Sync</h3>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <RiskMetric label="订单同步频率" value="Every 5m" />
          <RiskMetric label="单日支付上限" value="¥ 50,000" />
          <RiskMetric label="异常订单阻断" value="Enabled" positive />
        </div>
      </div>
    </div>
  )
}

const ProviderItem = ({ name, status, label, type }: any) => (
  <div className="flex items-center justify-between p-5 rounded-2xl bg-white/5 border border-white/5 hover:border-white/10 transition-all group">
    <div className="flex items-center gap-4">
      <div className={cn(
        "size-2 rounded-full",
        status === 'online' ? "bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.5)]" : 
        status === 'warning' ? "bg-amber-500 shadow-[0_0_10px_rgba(245,158,11,0.5)]" : "bg-white/10"
      )} />
      <div>
        <div className="text-sm font-bold text-white">{name}</div>
        <div className="text-[10px] text-white/30 uppercase tracking-widest">{type}</div>
      </div>
    </div>
    <div className="flex items-center gap-4">
      {label && <span className="text-[10px] text-rose-400 font-bold bg-rose-500/10 px-2 py-0.5 rounded-md border border-rose-500/20">{label}</span>}
      <button className="text-white/20 group-hover:text-[var(--accent)] transition-colors">
        <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.1a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
      </button>
    </div>
  </div>
)

const ToggleSetting = ({ title, detail, enabled }: any) => (
  <div className="flex items-center justify-between p-4 rounded-2xl bg-white/[0.02] border border-white/5">
    <div>
      <div className="text-sm font-bold text-white">{title}</div>
      <div className="text-[10px] text-white/30 mt-0.5">{detail}</div>
    </div>
    <div className={cn(
      "w-10 h-5 rounded-full relative transition-all",
      enabled ? "bg-[var(--accent)]" : "bg-white/10"
    )}>
      <div className={cn(
        "absolute top-1 size-3 rounded-full bg-white transition-all",
        enabled ? "right-1" : "left-1"
      )} />
    </div>
  </div>
)

const RiskMetric = ({ label, value, positive }: any) => (
  <div className="p-5 rounded-2xl bg-white/5 border border-white/5">
    <div className="text-[10px] font-bold text-white/20 uppercase tracking-widest mb-1">{label}</div>
    <div className={cn("text-lg font-black", positive ? "text-emerald-400" : "text-white")}>{value}</div>
  </div>
)
