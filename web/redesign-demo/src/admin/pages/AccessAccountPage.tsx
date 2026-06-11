import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const AccessAccountPage: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className={rdAdmin.sectionHeader}>
        <div className="flex gap-4">
          <button className={cn(rdForm.button, rdForm.buttonPrimary)}>添加账号</button>
          <button className={cn(rdForm.button, rdForm.buttonSecondary)}>批量操作</button>
        </div>
        <div className="flex gap-4">
          <input type="text" placeholder="搜索账号名称或适配器..." className={rdForm.input + " w-64"} />
        </div>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className="px-6 py-4 w-10"><input type="checkbox" className="accent-[var(--accent)]" /></th>
              <th className={rdAdmin.th}>账号名称</th>
              <th className={rdAdmin.th}>接入/鉴权方式</th>
              <th className={rdAdmin.th}>配置信息</th>
              <th className={rdAdmin.th}>状态</th>
              <th className={rdAdmin.th}>支持模型</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <AccountTableRow 
              name="SiliconFlow Main" 
              adapter="openai_compatible" 
              auth="api_key"
              baseUrl="https://api.siliconflow.cn/v1"
              priority={1}
              weight={100}
              concurrency={20}
              timeout={120000}
              status="enabled"
              models={[
                { code: "flux-1-pro", name: "Flux 1.0 Pro", tasks: ["T2I", "R2I"], qualities: ["Auto", "1K", "2K"], cost: "0.15", currency: "CNY", status: "enabled" },
                { code: "flux-1-schnell", name: "Flux Schnell", tasks: ["T2I"], qualities: ["Auto"], cost: "0.02", currency: "CNY", status: "enabled" }
              ]}
            />
            <AccountTableRow 
              name="OpenRouter-Express" 
              adapter="openrouter" 
              auth="api_key"
              baseUrl="https://openrouter.ai/api/v1"
              priority={2}
              weight={50}
              concurrency={5}
              timeout={60000}
              status="enabled"
              models={[
                { code: "mj-v6", name: "Midjourney v6", tasks: ["T2I", "R2I", "Edit"], qualities: ["Auto", "1K", "2K", "4K"], cost: "0.25", currency: "USD", status: "enabled" }
              ]}
            />
          </tbody>
        </table>
      </div>
    </div>
  )
}

const AccountTableRow = ({ name, adapter, auth, baseUrl, priority, weight, concurrency, timeout, status, models }: any) => {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <tr className={cn(rdAdmin.tr, expanded && "bg-white/[0.03]")}>
        <td className="px-6 py-4"><input type="checkbox" className="accent-[var(--accent)]" /></td>
        <td className={rdAdmin.td}>
          <div className="flex items-center gap-3">
            <div className="size-8 rounded-lg bg-white/5 flex items-center justify-center text-[var(--accent)]">
              <CloudIcon className="size-4" />
            </div>
            <span className="font-bold text-white">{name}</span>
          </div>
        </td>
        <td className={rdAdmin.td}>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-bold text-white/60">{adapter}</span>
            <span className="text-[10px] text-white/30 uppercase tracking-widest">{auth}</span>
          </div>
        </td>
        <td className={rdAdmin.td}>
          <div className="flex flex-col gap-1 text-[10px]">
            <span className="text-white/60 font-mono truncate max-w-[150px]" title={baseUrl}>{baseUrl}</span>
            <span className="text-white/30">P:{priority} | W:{weight} | C:{concurrency} | {timeout}ms</span>
          </div>
        </td>
        <td className={rdAdmin.td}>
           <span className={cn(rdAdmin.badge, status === 'enabled' ? rdAdmin.badgeSuccess : rdAdmin.badgeError)}>{status}</span>
        </td>
        <td className={rdAdmin.td}>
          <button 
            onClick={() => setExpanded(!expanded)}
            className="flex items-center gap-2 text-xs font-bold text-[var(--accent)] hover:text-white transition-colors"
          >
            {models.length} 个模型 
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
          <td colSpan={7} className="p-6 pl-20">
             <div className="rounded-2xl border border-white/5 bg-white/[0.02] overflow-hidden">
               <div className="flex items-center justify-between p-4 border-b border-white/5">
                 <h5 className="text-[10px] font-bold uppercase tracking-[0.15em] text-white/40">支持模型 / Supported Models</h5>
                 <button className="text-[10px] font-bold text-[var(--accent)] hover:underline">+ 添加模型</button>
               </div>
               <table className="w-full text-left border-collapse">
                 <thead>
                   <tr>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">模型代码 / 名称</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">任务类型</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">质量标准</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">单图成本</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">状态</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">操作</th>
                   </tr>
                 </thead>
                 <tbody>
                   {models.map((m: any, i: number) => (
                     <tr key={i} className="hover:bg-white/[0.02] transition-colors">
                       <td className="px-4 py-3">
                         <div className="flex flex-col">
                           <span className="text-xs font-bold text-white/80">{m.name}</span>
                           <span className="text-[10px] text-white/30 font-mono tracking-tighter">{m.code}</span>
                         </div>
                       </td>
                       <td className="px-4 py-3">
                         <div className="flex gap-1">
                           {m.tasks.map((t: string) => (
                             <span key={t} className="text-[9px] font-black px-1.5 py-0.5 rounded bg-white/5 text-white/40 border border-white/5">{t}</span>
                           ))}
                         </div>
                       </td>
                       <td className="px-4 py-3 text-xs text-white/60">{m.qualities.join(', ')}</td>
                       <td className="px-4 py-3">
                         <div className="flex items-baseline gap-1">
                           <span className="text-xs text-emerald-400 font-bold">{m.cost}</span>
                           <span className="text-[9px] text-white/20 uppercase">{m.currency}</span>
                         </div>
                       </td>
                       <td className="px-4 py-3">
                         <div className={cn("size-2 rounded-full", m.status === 'enabled' ? "bg-emerald-500" : "bg-white/20")} />
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

const CloudIcon = ({ className }: any) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M17.5 19c.1 0 .2 0 .3 0A5.5 5.5 0 0 0 16 8.1l-1.3-.1A7.5 7.5 0 0 0 2 12a7.5 7.5 0 0 0 12.3 5.8l1.2 1.2M17.5 19h.3"/><path d="M12 12l2 2 4-4"/></svg>
const ChevronIcon = ({ className }: any) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m6 9 6 6 6-6"/></svg>
const SettingsIconSmall = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
