import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const AuditLog: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4 p-6 rounded-3xl border border-white/5 bg-white/[0.02]">
        <div className="flex flex-1 items-center gap-4">
          <input type="text" placeholder="搜索动作、目标或管理员..." className={rdForm.input + " max-w-md"} />
          <select className={rdForm.input + " w-48"}><option>所有动作</option></select>
        </div>
        <button className={cn(rdForm.button, rdForm.buttonSecondary)}>导出日志</button>
      </div>

      <div className="space-y-2">
        <AuditLogItem 
          admin="Admin" 
          action="UPDATE_ROUTE_MODEL" 
          target="Flux Pro (v-flux-pro)" 
          detail="Enabled route and updated sort order from 10 to 5." 
          time="2 分钟前"
        />
        <AuditLogItem 
          admin="Admin" 
          action="CREATE_REDEEM_CODE" 
          target="Batch: 1718102400" 
          detail="Created 50 codes with 20 points each." 
          time="15 分钟前"
        />
        <AuditLogItem 
          admin="System" 
          action="AUTO_BAN_USER" 
          target="SpamBot_01" 
          detail="Banned due to high frequency task failure pattern." 
          time="1 小时前"
        />
        <AuditLogItem 
          admin="Admin" 
          action="APPROVE_REVIEW" 
          target="Image: img_9921" 
          detail="Approved for public gallery." 
          time="2 小时前"
        />
      </div>
    </div>
  )
}

const AuditLogItem = ({ admin, action, target, detail, time }: any) => (
  <div className="group flex items-center gap-6 p-5 rounded-2xl border border-white/5 bg-white/[0.01] hover:bg-white/[0.03] transition-all">
    <div className="size-10 rounded-xl bg-white/5 flex items-center justify-center font-bold text-white/20 text-xs">
      {admin.slice(0, 1)}
    </div>
    <div className="flex-1">
      <div className="flex items-center gap-3 mb-0.5">
        <span className="text-xs font-black text-[var(--accent)] tracking-widest">{action}</span>
        <div className="size-1 rounded-full bg-white/10" />
        <span className="text-sm font-bold text-white">{target}</span>
      </div>
      <p className="text-xs text-white/40 leading-relaxed">{detail}</p>
    </div>
    <div className="text-right">
      <div className="text-xs font-bold text-white/60">{admin}</div>
      <div className="text-[10px] text-white/20 mt-0.5">{time}</div>
    </div>
  </div>
)
