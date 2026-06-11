import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'
import { Modal, ConfirmModal } from '../components/ui'

export const UserManagement: React.FC = () => {
  const [isAddOpen, setIsAddOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = () => {
    setIsDeleting(true)
    setTimeout(() => {
      setIsDeleting(false)
      setDeleteTarget(null)
    }, 1500)
  }

  return (
    <div className="space-y-6">
      {/* Search & Filter */}
      <div className="flex items-center justify-between gap-4 p-6 rounded-3xl border border-white/5 bg-white/[0.02]">
        <div className="flex flex-1 items-center gap-4">
          <div className="relative flex-1 max-w-md">
            <SearchIcon className="absolute left-4 top-1/2 -translate-y-1/2 size-4 text-white/20" />
            <input 
              type="text" 
              placeholder="搜索用户名、邮箱或 ID..." 
              className={rdForm.input + " pl-12"}
            />
          </div>
          <select className={rdForm.input + " w-40"}>
            <option>所有分组</option>
            <option>普通用户</option>
            <option>VIP</option>
            <option>企业用户</option>
          </select>
        </div>
        <button onClick={() => setIsAddOpen(true)} className={cn(rdForm.button, rdForm.buttonPrimary)}>
          <PlusIcon className="mr-2 size-4" /> 新增用户
        </button>
      </div>

      {/* Table */}
      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className={rdAdmin.th}>用户信息</th>
              <th className={rdAdmin.th}>所属分组</th>
              <th className={rdAdmin.th}>账户余额</th>
              <th className={rdAdmin.th}>最后活跃</th>
              <th className={rdAdmin.th}>状态</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <UserRow 
              name="FatBallFish" email="fat@ball.fish" group="超级管理员" balance="8,420" active="2026/06/11 14:30" status="active" 
              onDelete={() => setDeleteTarget("FatBallFish")}
            />
            <UserRow 
              name="CyberNinja" email="ninja@cyber.io" group="VIP 成员" balance="1,200" active="2026/06/10 09:12" status="active" 
              onDelete={() => setDeleteTarget("CyberNinja")}
            />
            <UserRow 
              name="SpamBot_01" email="spam@toxic.com" group="普通用户" balance="0" active="2026/06/05 23:58" status="banned" 
              onDelete={() => setDeleteTarget("SpamBot_01")}
            />
          </tbody>
        </table>
        
        {/* Pagination */}
        <div className="p-6 border-t border-white/5 flex items-center justify-between">
          <span className="text-xs text-white/30">显示 1 到 3 共 12,840 条记录</span>
          <div className="flex gap-2">
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-4 py-1.5")}>上一页</button>
            <button className={cn(rdForm.button, rdForm.buttonPrimary, "px-4 py-1.5")}>1</button>
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-4 py-1.5")}>下一页</button>
          </div>
        </div>
      </div>

      {/* Modals */}
      <Modal 
        isOpen={isAddOpen} 
        onClose={() => setIsAddOpen(false)} 
        title="新增用户"
        footer={
          <>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonPrimary)}>保存配置</button>
          </>
        }
      >
        <div className="space-y-4">
          <div className={rdForm.group}>
            <label className={rdForm.label}>用户名</label>
            <input type="text" className={rdForm.input} placeholder="例如: Neo" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>邮箱地址</label>
            <input type="email" className={rdForm.input} placeholder="neo@matrix.io" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>初始积分 (可选)</label>
            <input type="number" className={rdForm.input} placeholder="0" defaultValue={50} />
          </div>
        </div>
      </Modal>

      <ConfirmModal
        isOpen={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="删除用户确认"
        message={`确定要永久删除用户 "${deleteTarget}" 吗？此操作将清除其所有生图记录及余额且无法恢复。`}
        isLoading={isDeleting}
        confirmText="删除"
        confirmTone="danger"
      />
    </div>
  )
}

const UserRow = ({ name, email, group, balance, active, status, onDelete }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}>
      <div className="flex items-center gap-3">
        <div className="size-10 rounded-xl bg-white/5 flex items-center justify-center font-bold text-white/20">
          {name.slice(0, 1)}
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-white">{name}</span>
          <span className="text-[10px] text-white/30">{email}</span>
        </div>
      </div>
    </td>
    <td className={rdAdmin.td}>
      <span className="text-xs text-white/60">{group}</span>
    </td>
    <td className={rdAdmin.td}>
      <div className="flex items-baseline gap-1">
        <span className="font-mono text-white">{balance}</span>
        <span className="text-[10px] text-white/30">◈</span>
      </div>
    </td>
    <td className={rdAdmin.td}>
      <span className="text-xs text-white/40">{active}</span>
    </td>
    <td className={rdAdmin.td}>
      <span className={cn(
        rdAdmin.badge,
        status === 'active' ? rdAdmin.badgeSuccess : 
        status === 'banned' ? rdAdmin.badgeError : rdAdmin.badgeWarning
      )}>
        {status}
      </span>
    </td>
    <td className={rdAdmin.td}>
      <div className="flex gap-2">
        <button className="p-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/40 hover:text-white transition-all">
          <EditIcon className="size-4" />
        </button>
        <button onClick={onDelete} className="p-2 rounded-lg bg-white/5 hover:bg-rose-500/10 text-white/40 hover:text-rose-400 transition-all">
          <TrashIcon className="size-4" />
        </button>
      </div>
    </td>
  </tr>
)

const SearchIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
const PlusIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M5 12h14M12 5v14"/></svg>
const EditIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>
const TrashIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M10 11v6M14 11v6"/></svg>
