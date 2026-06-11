import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const SystemSettings: React.FC = () => {
  return (
    <div className="max-w-4xl space-y-12">
      <section className="space-y-6">
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>通用设置 / General</h3>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <SettingField label="站点名称" value="Mikiko Studio" />
          <SettingField label="运营状态" value="正常运行" status="success" />
          <SettingField label="主域名" value="https://mikiko.studio" />
          <SettingField label="API 基地址" value="https://api.mikiko.studio" />
        </div>
      </section>

      <section className="space-y-6">
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>安全策略 / Security</h3>
        </div>
        <div className="space-y-4">
          <ToggleField title="允许新用户注册" enabled />
          <ToggleField title="强制邮箱验证" enabled />
          <ToggleField title="启用全站内容审核 (Moderation)" enabled />
          <ToggleField title="限制单用户日最大生图数" detail="当前限制: 500 张" enabled />
        </div>
      </section>

      <section className="space-y-6">
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>存储配置 / Storage</h3>
        </div>
        <div className="p-8 rounded-3xl border border-white/5 bg-white/[0.02]">
          <div className="flex items-center justify-between mb-8">
            <div className="flex items-center gap-4">
              <div className="size-12 rounded-2xl bg-white/5 flex items-center justify-center">
                <CloudIcon />
              </div>
              <div>
                <h4 className="text-lg font-bold text-white">Aliyun OSS</h4>
                <p className="text-xs text-white/30 uppercase tracking-widest mt-0.5">Primary Storage</p>
              </div>
            </div>
            <span className={rdAdmin.badgeSuccess}>Connected</span>
          </div>
          <div className="grid grid-cols-2 gap-8">
            <div>
              <div className="text-[10px] font-bold text-white/20 uppercase tracking-widest mb-1">Bucket</div>
              <div className="text-sm font-mono text-white">mikiko-assets-prod</div>
            </div>
            <div>
              <div className="text-[10px] font-bold text-white/20 uppercase tracking-widest mb-1">Region</div>
              <div className="text-sm font-mono text-white">oss-cn-hangzhou</div>
            </div>
          </div>
        </div>
      </section>

      <div className="pt-8 border-t border-white/5 flex justify-end gap-4">
        <button className={cn(rdForm.button, rdForm.buttonSecondary)}>重置修改</button>
        <button className={cn(rdForm.button, rdForm.buttonPrimary, "px-10")}>保存配置</button>
      </div>
    </div>
  )
}

const SettingField = ({ label, value, status }: any) => (
  <div className="space-y-2">
    <label className={rdForm.label}>{label}</label>
    <div className="relative">
      <input type="text" value={value} readOnly className={rdForm.input} />
      {status === 'success' && <div className="absolute right-4 top-1/2 -translate-y-1/2 size-2 rounded-full bg-emerald-500" />}
    </div>
  </div>
)

const ToggleField = ({ title, detail, enabled }: any) => (
  <div className="flex items-center justify-between p-6 rounded-2xl bg-white/[0.02] border border-white/5 hover:bg-white/[0.04] transition-all">
    <div>
      <h4 className="text-sm font-bold text-white">{title}</h4>
      {detail && <p className="text-xs text-white/30 mt-1">{detail}</p>}
    </div>
    <div className={cn(
      "w-12 h-6 rounded-full relative transition-all cursor-pointer",
      enabled ? "bg-[var(--accent)]" : "bg-white/10"
    )}>
      <div className={cn(
        "absolute top-1 size-4 rounded-full bg-white shadow-lg transition-all",
        enabled ? "right-1" : "left-1"
      )} />
    </div>
  </div>
)

const CloudIcon = () => <svg className="size-6 text-[var(--accent)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M17.5 19c.1 0 .2 0 .3 0A5.5 5.5 0 0 0 16 8.1l-1.3-.1A7.5 7.5 0 0 0 2 12a7.5 7.5 0 0 0 12.3 5.8l1.2 1.2M17.5 19h.3"/><path d="M12 12l2 2 4-4"/></svg>
