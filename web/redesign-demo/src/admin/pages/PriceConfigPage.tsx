import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'
import { Modal } from '../components/ui'

export const PriceConfigPage: React.FC = () => {
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<any>(null)

  const handleEdit = (route: string, type: string, quality: any) => {
    setEditTarget({ route, type, ...quality })
    setIsEditOpen(true)
  }

  return (
    <div className="space-y-10">
      <div className={rdAdmin.sectionHeader}>
        <h3 className={rdAdmin.sectionTitle}>积分价格配置 / Price Strategy</h3>
        <button className={cn(rdForm.button, rdForm.buttonPrimary)}>新增配置</button>
      </div>

      {/* Rules Notice */}
      <div className="p-8 rounded-[2rem] bg-[var(--accent)]/5 border border-[var(--accent)]/10">
        <div className="flex items-start gap-4">
          <div className="size-10 rounded-xl bg-[var(--accent)]/10 flex items-center justify-center text-[var(--accent)]">
            <InfoIcon />
          </div>
          <div className="flex-1">
            <h4 className="text-lg font-bold text-white mb-2">计费规则说明</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
              <div className="space-y-2">
                <p className="text-sm text-white/50 leading-relaxed">
                  1. <strong className="text-white/80">扣费公式</strong>: <code className="bg-white/5 px-1.5 py-0.5 rounded text-[var(--accent)]">最终积分 = 基础消耗 * (参考图倍率 if 包含参考图)</code>
                </p>
                <p className="text-sm text-white/50 leading-relaxed">
                  2. <strong className="text-white/80">精度说明</strong>: 后端扣费保留 5 位小数，前端展示四舍五入保留 2 位。
                </p>
              </div>
              <div className="space-y-2">
                <p className="text-sm text-white/50 leading-relaxed">
                  3. <strong className="text-white/80">兜底逻辑</strong>: 若路由模型未配对应任务类型的价格，系统将返回配置错误。
                </p>
                <p className="text-sm text-white/50 leading-relaxed">
                  4. <strong className="text-white/80">展示规则</strong>: 列表按照路由模型和任务类型进行聚合，点击行展开具体质量的价格配置。
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className="px-6 py-4 w-10"></th>
              <th className={rdAdmin.th}>路由模型</th>
              <th className={rdAdmin.th}>任务类型</th>
              <th className={rdAdmin.th}>已配置质量数</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <AggregatedPriceRow 
              route="Flux Pro" 
              type="文生图 (T2I)" 
              base_resolution={[
                { quality: "Auto", base: "1.25000", multiplier: "1.0", status: "enabled" },
                { quality: "1K", base: "2.00000", multiplier: "1.0", status: "enabled" }
              ]}
              onEdit={handleEdit}
            />
            <AggregatedPriceRow 
              route="Flux Pro" 
              type="参考生图 (R2I)" 
              base_resolution={[
                { quality: "Auto", base: "1.25000", multiplier: "1.5", status: "enabled" }
              ]}
              onEdit={handleEdit}
            />
            <AggregatedPriceRow 
              route="Midjourney v6" 
              type="文生图 (T2I)" 
              base_resolution={[
                { quality: "Standard", base: "1.50000", multiplier: "1.0", status: "enabled" },
                { quality: "High", base: "2.50000", multiplier: "1.0", status: "enabled" },
                { quality: "Ultra", base: "4.00000", multiplier: "1.0", status: "disabled" }
              ]}
              onEdit={handleEdit}
            />
            <AggregatedPriceRow 
              route="Midjourney v6" 
              type="图片编辑 (Edit)" 
              base_resolution={[
                { quality: "Auto", base: "1.00000", multiplier: "1.0", status: "enabled" }
              ]}
              onEdit={handleEdit}
            />
          </tbody>
        </table>
      </div>

      <Modal 
        isOpen={isEditOpen} 
        onClose={() => setIsEditOpen(false)} 
        title="编辑价格配置"
        footer={
          <>
            <button onClick={() => setIsEditOpen(false)} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
            <button onClick={() => setIsEditOpen(false)} className={cn(rdForm.button, rdForm.buttonPrimary)}>保存价格</button>
          </>
        }
      >
        <div className="space-y-6">
          <div className="p-4 rounded-xl bg-white/5 border border-white/5 flex flex-col gap-1">
            <span className="text-xs text-white/50 uppercase tracking-widest">Target</span>
            <span className="text-sm font-bold text-white">{editTarget?.route} <span className="text-white/30 mx-2">/</span> {editTarget?.type} <span className="text-white/30 mx-2">/</span> <span className="text-[var(--accent)]">{editTarget?.quality}</span></span>
          </div>

          <div className="grid grid-cols-2 gap-6">
            <div className={rdForm.group}>
              <label className={rdForm.label}>基础消耗积分 (Base Points)</label>
              <div className="relative">
                <input type="number" step="0.01" className={rdForm.input} defaultValue={editTarget?.base || ''} />
                <span className="absolute right-4 top-1/2 -translate-y-1/2 text-xs font-bold text-white/30">◈</span>
              </div>
            </div>
            <div className={rdForm.group}>
              <label className={rdForm.label}>参考图倍率 (Ref Multiplier)</label>
              <div className="relative">
                <input type="number" step="0.1" className={rdForm.input} defaultValue={editTarget?.multiplier || ''} />
                <span className="absolute right-4 top-1/2 -translate-y-1/2 text-xs font-bold text-white/30">x</span>
              </div>
            </div>
          </div>
          
          <div className={rdForm.group}>
            <label className={rdForm.label}>启用状态 (Status)</label>
            <select className={rdForm.input} defaultValue={editTarget?.status}>
              <option value="enabled">启用 (Enabled)</option>
              <option value="disabled">停用 (Disabled)</option>
            </select>
          </div>
        </div>
      </Modal>
    </div>
  )
}

const AggregatedPriceRow = ({ route, type, base_resolution, onEdit }: any) => {
  const [expanded, setExpanded] = useState(false)

  return (
    <>
      <tr className={cn(rdAdmin.tr, "cursor-pointer group", expanded && "bg-white/[0.03]")} onClick={() => setExpanded(!expanded)}>
        <td className="px-6 py-4">
          <ChevronIcon className={cn("size-4 text-white/30 group-hover:text-white transition-all", expanded && "rotate-180")} />
        </td>
        <td className={rdAdmin.td}><span className="font-bold text-white group-hover:text-[var(--accent)] transition-colors">{route}</span></td>
        <td className={rdAdmin.td}>
          <span className="inline-flex px-2 py-1 rounded bg-white/5 border border-white/5 text-xs text-white/80 font-bold">{type}</span>
        </td>
        <td className={rdAdmin.td}>
          <span className="text-xs font-bold text-white/60">{base_resolution.length} 个质量配置</span>
        </td>
        <td className={rdAdmin.td}>
          <button className="text-[var(--accent)] hover:underline text-xs font-bold" onClick={(e) => e.stopPropagation()}>快速添加质量</button>
        </td>
      </tr>
      {expanded && (
        <tr className="bg-black/40 border-b border-white/[0.02]">
          <td colSpan={5} className="p-6 pl-20">
             <div className="rounded-2xl border border-white/5 bg-white/[0.02] overflow-hidden">
               <table className="w-full text-left border-collapse">
                 <thead>
                   <tr>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">生成质量 (Quality)</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">基础消耗 (Base)</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">参考图倍率 (Multiplier)</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">状态</th>
                     <th className="px-4 py-3 text-[10px] font-bold uppercase tracking-wider text-white/30 border-b border-white/5">操作</th>
                   </tr>
                 </thead>
                 <tbody>
                   {base_resolution.map((q: any, i: number) => (
                     <tr key={i} className="hover:bg-white/[0.02] transition-colors">
                       <td className="px-4 py-3 text-xs font-bold text-white/80">{q.quality}</td>
                       <td className="px-4 py-3">
                         <div className="flex items-baseline gap-1">
                           <span className="text-sm font-black text-[var(--accent)]">{q.base}</span>
                           <span className="text-[10px] text-white/20">◈</span>
                         </div>
                       </td>
                       <td className="px-4 py-3 text-xs font-bold text-white/60">x {q.multiplier}</td>
                       <td className="px-4 py-3">
                         <span className={cn(rdAdmin.badge, q.status === 'enabled' ? rdAdmin.badgeSuccess : rdAdmin.badgeError)}>{q.status}</span>
                       </td>
                       <td className="px-4 py-3">
                         <button onClick={() => onEdit(route, type, q)} className="text-white/40 hover:text-white transition-colors"><SettingsIconSmall /></button>
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

const InfoIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
const ChevronIcon = ({ className }: any) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m6 9 6 6 6-6"/></svg>
const SettingsIconSmall = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
