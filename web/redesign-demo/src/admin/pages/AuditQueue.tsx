import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin } from '../admin-classes'
import { ConfirmModal } from '../components/ui'

export const AuditQueue: React.FC = () => {
  const [items, setItems] = useState([
    { id: '1', title: "Cyberpunk Street", user: "FatBallFish", type: "Public Gallery", context: "Home Feed", time: "2 分钟前", imageUrl: "https://picsum.photos/seed/cyber/400/400" },
    { id: '2', title: "Minimalist Interior", user: "DesignPro", type: "Public Gallery", context: "Architecture Section", time: "15 分钟前", imageUrl: "https://picsum.photos/seed/interior/400/400" },
    { id: '3', title: "Portrait of a Warrior", user: "ArtLover", type: "Reported Content", context: "Inappropriate Content", time: "1 小时前", imageUrl: "https://picsum.photos/seed/warrior/400/400", warning: true }
  ])

  const [actionTarget, setActionTarget] = useState<{ id: string, type: 'approve' | 'reject', title: string } | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)

  const handleAction = () => {
    setIsProcessing(true)
    setTimeout(() => {
      setItems(items.filter(item => item.id !== actionTarget?.id))
      setIsProcessing(false)
      setActionTarget(null)
    }, 1000)
  }

  return (
    <div className="space-y-6">
      <div className="flex gap-4 p-1 bg-white/5 rounded-2xl w-fit">
        <button className="px-6 py-2 rounded-xl text-xs font-bold bg-[var(--accent)] text-white">待处理 ({items.length})</button>
        <button className="px-6 py-2 rounded-xl text-xs font-bold text-white/40 hover:text-white">已通过</button>
        <button className="px-6 py-2 rounded-xl text-xs font-bold text-white/40 hover:text-white">已驳回</button>
      </div>

      <div className="grid grid-cols-1 gap-4">
        {items.map(item => (
          <AuditItem 
            key={item.id}
            {...item}
            onApprove={() => setActionTarget({ id: item.id, type: 'approve', title: item.title })}
            onReject={() => setActionTarget({ id: item.id, type: 'reject', title: item.title })}
          />
        ))}
        {items.length === 0 && (
          <div className="p-12 text-center text-white/30 text-sm border border-dashed border-white/10 rounded-3xl">
            当前队列没有待审核的内容。
          </div>
        )}
      </div>

      <ConfirmModal
        isOpen={!!actionTarget}
        onClose={() => setActionTarget(null)}
        onConfirm={handleAction}
        title={actionTarget?.type === 'approve' ? "审核通过" : "驳回申请"}
        message={
          actionTarget?.type === 'approve' 
            ? `确定要通过 "${actionTarget?.title}" 的申请吗？通过后内容将对所有用户可见。`
            : `确定要驳回 "${actionTarget?.title}" 吗？驳回后该内容将被下架或不予展示。`
        }
        isLoading={isProcessing}
        confirmText={actionTarget?.type === 'approve' ? "通过" : "驳回"}
        confirmTone={actionTarget?.type === 'approve' ? "primary" : "danger"}
      />
    </div>
  )
}

const AuditItem = ({ title, user, type, context, time, imageUrl, warning, onApprove, onReject }: any) => (
  <div className="group flex items-center gap-6 p-6 rounded-3xl border border-white/5 bg-white/[0.02] hover:bg-white/[0.04] transition-all">
    <div className="relative size-24 rounded-2xl overflow-hidden border border-white/10 shrink-0">
      <img src={imageUrl} alt="" className="size-full object-cover transition-transform group-hover:scale-110" />
      {warning && <div className="absolute top-2 left-2 size-3 rounded-full bg-rose-500 shadow-[0_0_10px_rgba(244,63,94,0.5)]" />}
    </div>
    
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-3 mb-1">
        <h4 className="text-lg font-bold text-white truncate">{title}</h4>
        <span className={cn(rdAdmin.badge, "shrink-0", warning ? rdAdmin.badgeError : rdAdmin.badgeInfo)}>{type}</span>
      </div>
      <div className="flex flex-wrap items-center gap-4 text-xs text-white/40">
        <span>用户: <strong className="text-white/60">{user}</strong></span>
        <div className="size-1 rounded-full bg-white/10" />
        <span>位置: <strong className="text-white/60">{context}</strong></span>
        <div className="size-1 rounded-full bg-white/10" />
        <span>提交于: {time}</span>
      </div>
    </div>

    <div className="flex gap-3 shrink-0">
      <button onClick={onApprove} className="px-6 py-2.5 rounded-xl bg-emerald-500 text-white text-xs font-bold hover:scale-105 transition-transform active:scale-95 shadow-lg shadow-emerald-500/20">通过</button>
      <button onClick={onReject} className="px-6 py-2.5 rounded-xl bg-white/5 text-white/60 text-xs font-bold hover:bg-rose-500/10 hover:text-rose-400 transition-all active:scale-95">驳回</button>
    </div>
  </div>
)
