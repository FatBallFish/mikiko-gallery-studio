import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const PackageManagement: React.FC = () => {
  return (
    <div className="space-y-8">
      <div className={rdAdmin.sectionHeader}>
        <h3 className={rdAdmin.sectionTitle}>套餐配置 / Package Config</h3>
        <button className={cn(rdForm.button, rdForm.buttonPrimary)}>新增套餐</button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <PlanCard name="基础叠加包" points="500" price="49.00" active />
        <PlanCard name="专业叠加包" points="2000" price="149.00" active />
        <PlanCard name="至尊叠加包" points="5000" price="299.00" active />
        <PlanCard name="早鸟测试包" points="100" price="9.90" />
      </div>
    </div>
  )
}

const PlanCard = ({ name, points, price, active }: any) => (
  <div className={cn(
    "p-8 rounded-[2.5rem] border transition-all hover:scale-[1.02] group",
    active ? "bg-white/[0.04] border-white/10 shadow-2xl" : "bg-white/[0.01] border-white/5 opacity-50"
  )}>
    <div className="flex justify-between items-start mb-8">
      <div>
        <h4 className="text-xl font-bold text-white group-hover:text-[var(--accent)] transition-colors">{name}</h4>
        <p className="text-xs text-white/30 mt-1 uppercase tracking-widest">Points Credit Package</p>
      </div>
      {active && <span className="bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded-lg text-[10px] font-bold">ACTIVE</span>}
    </div>
    <div className="mb-10">
      <div className="text-4xl font-black text-white tracking-tighter">{points} <span className="text-sm font-normal text-white/20 ml-1">POINTS</span></div>
      <div className="text-xl font-mono text-[var(--accent)] mt-2">¥ {price}</div>
    </div>
    <div className="flex gap-3">
      <button className={cn(rdForm.button, rdForm.buttonSecondary, "flex-1")}>编辑</button>
      <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-3")}>
        <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
      </button>
    </div>
  </div>
)
