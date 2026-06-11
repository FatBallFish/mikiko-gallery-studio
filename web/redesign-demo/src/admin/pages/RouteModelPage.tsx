import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'
import { Modal } from '../components/ui'

export const RouteModelPage: React.FC = () => {
  const [isAddOpen, setIsAddOpen] = useState(false)

  return (
    <div className="space-y-6">
      <div className={rdAdmin.sectionHeader}>
        <div className="flex gap-4">
          <button onClick={() => setIsAddOpen(true)} className={cn(rdForm.button, rdForm.buttonPrimary)}>新增路由</button>
          <button className={cn(rdForm.button, rdForm.buttonSecondary)}>批量操作</button>
        </div>
        <div className="flex gap-4">
          <input type="text" placeholder="搜索路由名称或代码..." className={rdForm.input + " w-64"} />
        </div>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className="px-6 py-4 w-10"><input type="checkbox" className="accent-[var(--accent)]" /></th>
              <th className={rdAdmin.th}>路由模型</th>
              <th className={rdAdmin.th}>可见性</th>
              <th className={rdAdmin.th}>状态</th>
              <th className={rdAdmin.th}>已绑候选账号数</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <RouteTableRow 
              name="Flux Pro (Fast)" 
              code="v-flux-pro" 
              visibility="全员可见" 
              status="enabled"
              candidates={[
                { account: "SiliconFlow-01", model: "flux-pro", priority: 1, weight: 100, active: true },
                { account: "OpenRouter-Main", model: "black-forest-labs/flux-pro", priority: 2, weight: 50, active: true }
              ]}
            />
            <RouteTableRow 
              name="Midjourney v6" 
              code="v-mj-v6" 
              visibility="按分组 (VIP1, Beta)" 
              status="enabled"
              candidates={[
                { account: "MJ-Account-Pool-01", model: "mj-v6", priority: 1, weight: 100, active: true }
              ]}
            />
            <RouteTableRow 
              name="DALL-E 3 (High Res)" 
              code="v-dalle-3" 
              visibility="隐藏" 
              status="disabled"
              candidates={[
                { account: "OpenAI-Direct", model: "dall-e-3", priority: 1, weight: 100, active: false }
              ]}
            />
          </tbody>
        </table>
      </div>

      <Modal 
        isOpen={isAddOpen} 
        onClose={() => setIsAddOpen(false)} 
        title="配置路由模型"
        size="lg"
        footer={
          <>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonPrimary)}>保存路由</button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-6">
          <div className={rdForm.group}>
            <label className={rdForm.label}>路由代码 (Code)</label>
            <input type="text" className={rdForm.input} placeholder="例如: v-mj-v6" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>展示名称 (Name)</label>
            <input type="text" className={rdForm.input} placeholder="例如: Midjourney v6" />
          </div>
          <div className={rdForm.group + " col-span-2"}>
            <label className={rdForm.label}>描述 (Description)</label>
            <textarea className={rdForm.input + " min-h-[80px] py-3"} placeholder="面向用户的模型简介..." />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>可见性 (Visibility)</label>
            <select className={rdForm.input}>
              <option>全员可见 (Public)</option>
              <option>按分组 (Groups)</option>
              <option>隐藏 (Hidden)</option>
            </select>
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>状态 (Status)</label>
            <select className={rdForm.input}>
              <option>启用 (Enabled)</option>
              <option>停用 (Disabled)</option>
            </select>
          </div>
        </div>
      </Modal>
    </div>
  )
}

const RouteTableRow = ({ name, code, visibility, status, candidates }: any) => {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <tr className={cn(rdAdmin.tr, expanded && "bg-white/[0.03]")}>
        <td className="px-6 py-4"><input type="checkbox" className="accent-[var(--accent)]" /></td>
        <td className={rdAdmin.td}>
          <div className="flex flex-col">
            <span className="font-bold text-white">{name}</span>
            <span className="text-[10px] text-white/40 font-mono tracking-tighter">{code}</span>
          </div>
        </td>
        <td className={rdAdmin.td}><span className="text-xs text-white/60">{visibility}</span></td>
        <td className={rdAdmin.td}>
           <span className={cn(rdAdmin.badge, status === 'enabled' ? rdAdmin.badgeSuccess : rdAdmin.badgeError)}>{status}</span>
        </td>
        <td className={rdAdmin.td}>
          <button 
            onClick={() => setExpanded(!expanded)}
            className="flex items-center gap-2 text-xs font-bold text-[var(--accent)] hover:text-white transition-colors"
          >
            {candidates.length} 个候选账号 
            <ChevronIcon className={cn("size-4 transition-transform", expanded && "rotate-180")} />
          </button>
        </td>
        <td className={rdAdmin.td}>
           <div className="flex gap-3">
             <button className="text-[var(--accent)] hover:underline text-xs font-bold">编辑</button>
             <button className="text-rose-400 hover:underline text-xs font-bold">删除</button>
           </div>
        </td>
      </tr>
      {expanded && (
        <tr className="bg-black/40 border-b border-white/[0.02]">
          <td colSpan={6} className="p-6 pl-20">
             <div className="rounded-2xl border border-white/5 bg-white/[0.02] overflow-hidden">
               <table className="w-full text-left border-collapse">
                 <thead>
                   <tr>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">真实账号</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">底层模型</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">优先级</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">权重</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">状态</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">操作</th>
                   </tr>
                 </thead>
                 <tbody>
                   {candidates.map((c: any, i: number) => (
                     <tr key={i} className="hover:bg-white/[0.02] transition-colors">
                       <td className="px-4 py-3 text-xs font-bold text-white/80">{c.account}</td>
                       <td className="px-4 py-3 text-xs text-white/60 font-mono">{c.model}</td>
                       <td className="px-4 py-3 text-xs text-white">{c.priority}</td>
                       <td className="px-4 py-3 text-xs text-white">{c.weight}</td>
                       <td className="px-4 py-3">
                         <div className={cn("size-2 rounded-full", c.active ? "bg-emerald-500" : "bg-white/20")} />
                       </td>
                       <td className="px-4 py-3">
                         <button className="text-white/40 hover:text-white transition-colors"><SettingsIconSmall /></button>
                       </td>
                     </tr>
                   ))}
                 </tbody>
               </table>
             </div>
          </td>
        </tr>
      )}
    </>
  )
}

const ChevronIcon = ({ className }: any) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m6 9 6 6 6-6"/></svg>
const SettingsIconSmall = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
