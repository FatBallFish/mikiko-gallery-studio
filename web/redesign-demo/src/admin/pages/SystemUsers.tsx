import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'
import { Modal, ConfirmModal } from '../components/ui'

export const SystemUsers: React.FC = () => {
  const [isAddOpen, setIsAddOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [resetTarget, setResetTarget] = useState<string | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)

  const handleProcess = (callback: () => void) => {
    setIsProcessing(true)
    setTimeout(() => {
      setIsProcessing(false)
      callback()
    }, 1000)
  }

  return (
    <div className="space-y-6">
      <div className={rdAdmin.sectionHeader}>
        <h3 className={rdAdmin.sectionTitle}>系统账户管理 / System Accounts</h3>
        <button onClick={() => setIsAddOpen(true)} className={cn(rdForm.button, rdForm.buttonPrimary)}>添加系统账户</button>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className={rdAdmin.th}>用户信息</th>
              <th className={rdAdmin.th}>角色权限</th>
              <th className={rdAdmin.th}>最后登录</th>
              <th className={rdAdmin.th}>状态</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <SystemUserRow 
              name="Admin" 
              email="admin@mikiko.studio" 
              role="Super Administrator" 
              lastLogin="2026/06/11 10:25:33" 
              status="active" 
              onDelete={() => setDeleteTarget("Admin")}
              onReset={() => setResetTarget("Admin")}
            />
            <SystemUserRow 
              name="Moderator_01" 
              email="mod1@mikiko.studio" 
              role="Content Moderator" 
              lastLogin="2026/06/10 14:12:00" 
              status="active" 
              onDelete={() => setDeleteTarget("Moderator_01")}
              onReset={() => setResetTarget("Moderator_01")}
            />
            <SystemUserRow 
              name="Finance_Reviewer" 
              email="finance@mikiko.studio" 
              role="Financial Auditor" 
              lastLogin="2026/05/20 09:30:15" 
              status="disabled" 
              onDelete={() => setDeleteTarget("Finance_Reviewer")}
              onReset={() => setResetTarget("Finance_Reviewer")}
            />
          </tbody>
        </table>
      </div>

      <Modal 
        isOpen={isAddOpen} 
        onClose={() => setIsAddOpen(false)} 
        title="添加系统账户"
        footer={
          <>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
            <button onClick={() => handleProcess(() => setIsAddOpen(false))} className={cn(rdForm.button, rdForm.buttonPrimary)}>确认添加</button>
          </>
        }
      >
        <div className="space-y-4">
          <div className={rdForm.group}>
            <label className={rdForm.label}>用户名</label>
            <input type="text" className={rdForm.input} placeholder="例如: Editor_01" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>邮箱地址</label>
            <input type="email" className={rdForm.input} placeholder="name@mikiko.studio" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>分配角色</label>
            <select className={rdForm.input}>
              <option>Content Moderator (内容审核员)</option>
              <option>Financial Auditor (财务审计)</option>
              <option>System Administrator (系统管理员)</option>
            </select>
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>初始密码</label>
            <input type="password" className={rdForm.input} placeholder="不填则自动生成并发送邮件" />
          </div>
        </div>
      </Modal>

      <ConfirmModal
        isOpen={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => handleProcess(() => setDeleteTarget(null))}
        title="删除账号确认"
        message={`确定要删除管理员 "${deleteTarget}" 吗？该操作不可逆。`}
        isLoading={isProcessing}
        confirmText="删除"
        confirmTone="danger"
      />

      <ConfirmModal
        isOpen={!!resetTarget}
        onClose={() => setResetTarget(null)}
        onConfirm={() => handleProcess(() => setResetTarget(null))}
        title="重置密码确认"
        message={`确定要重置管理员 "${resetTarget}" 的密码吗？新的随机密码将发送至其注册邮箱。`}
        isLoading={isProcessing}
        confirmText="确认重置"
        confirmTone="primary"
      />
    </div>
  )
}

const SystemUserRow = ({ name, email, role, lastLogin, status, onDelete, onReset }: any) => (
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
      <span className="text-xs text-white/60">{role}</span>
    </td>
    <td className={rdAdmin.td}>
      <span className="text-xs text-white/40 font-mono tracking-tighter">{lastLogin}</span>
    </td>
    <td className={rdAdmin.td}>
      <span className={cn(
        rdAdmin.badge,
        status === 'active' ? rdAdmin.badgeSuccess : rdAdmin.badgeError
      )}>
        {status}
      </span>
    </td>
    <td className={rdAdmin.td}>
      <div className="flex gap-3">
        <button onClick={onReset} className="text-[var(--accent)] hover:underline text-xs font-bold">重置密码</button>
        <button className="text-white/40 hover:text-white transition-colors text-xs font-bold">编辑</button>
        {status === 'active' ? (
           <button className="text-amber-400 hover:underline text-xs font-bold">禁用</button>
        ) : (
           <button className="text-emerald-400 hover:underline text-xs font-bold">启用</button>
        )}
        <button onClick={onDelete} className="text-rose-400 hover:underline text-xs font-bold">删除</button>
      </div>
    </td>
  </tr>
)
