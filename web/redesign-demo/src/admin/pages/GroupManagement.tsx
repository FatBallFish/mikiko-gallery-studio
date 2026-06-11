import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'
import { Modal, ConfirmModal } from '../components/ui'

export const GroupManagement: React.FC = () => {
  const [isAddOpen, setIsAddOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
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
        <h3 className={rdAdmin.sectionTitle}>用户分组 / User Groups</h3>
        <button onClick={() => setIsAddOpen(true)} className={cn(rdForm.button, rdForm.buttonPrimary)}>添加用户分组</button>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className={rdAdmin.th}>分组代码</th>
              <th className={rdAdmin.th}>分组名称</th>
              <th className={rdAdmin.th}>描述</th>
              <th className={rdAdmin.th}>关联路由模型数</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <GroupRow 
              code="grp_vip" 
              name="VIP 用户组" 
              desc="购买过包月或高级套餐的用户，享有优先队列及受限模型访问权。" 
              routesCount={5} 
              onDelete={() => setDeleteTarget("VIP 用户组")}
            />
            <GroupRow 
              code="grp_beta" 
              name="内测用户组" 
              desc="受邀体验实验性功能和模型的小规模群体。" 
              routesCount={8} 
              onDelete={() => setDeleteTarget("内测用户组")}
            />
            <GroupRow 
              code="grp_enterprise" 
              name="企业用户组" 
              desc="企业批量采购客户，专享独立高速通道及 API 接入权。" 
              routesCount={3} 
              onDelete={() => setDeleteTarget("企业用户组")}
            />
          </tbody>
        </table>
      </div>

      <Modal 
        isOpen={isAddOpen} 
        onClose={() => setIsAddOpen(false)} 
        title="添加用户分组"
        footer={
          <>
            <button onClick={() => setIsAddOpen(false)} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
            <button onClick={() => handleProcess(() => setIsAddOpen(false))} className={cn(rdForm.button, rdForm.buttonPrimary)}>保存</button>
          </>
        }
      >
        <div className="space-y-4">
          <div className={rdForm.group}>
            <label className={rdForm.label}>分组代码 (Code)</label>
            <input type="text" className={rdForm.input} placeholder="如: grp_vip" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>分组名称 (Name)</label>
            <input type="text" className={rdForm.input} placeholder="如: VIP 用户组" />
          </div>
          <div className={rdForm.group}>
            <label className={rdForm.label}>分组描述 (Description)</label>
            <textarea className={rdForm.input + " min-h-[100px]"} placeholder="分组用途描述..." />
          </div>
        </div>
      </Modal>

      <ConfirmModal
        isOpen={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => handleProcess(() => setDeleteTarget(null))}
        title="删除分组确认"
        message={`确定要删除 "${deleteTarget}" 吗？该组内用户将失去对组绑定的私有路由模型的访问权限。`}
        isLoading={isProcessing}
        confirmText="删除分组"
        confirmTone="danger"
      />
    </div>
  )
}

const GroupRow = ({ code, name, desc, routesCount, onDelete }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}><span className="text-xs font-mono font-bold text-[var(--accent)]">{code}</span></td>
    <td className={rdAdmin.td}><span className="font-bold text-white">{name}</span></td>
    <td className={rdAdmin.td}><span className="text-xs text-white/50">{desc}</span></td>
    <td className={rdAdmin.td}><span className="text-xs font-bold text-white/80">{routesCount} 个路由模型</span></td>
    <td className={rdAdmin.td}>
      <div className="flex gap-3">
        <button className="text-white/40 hover:text-white transition-colors text-xs font-bold">配置模型可见性</button>
        <button className="text-white/40 hover:text-white transition-colors text-xs font-bold">编辑</button>
        <button onClick={onDelete} className="text-rose-400 hover:underline text-xs font-bold">删除</button>
      </div>
    </td>
  </tr>
)
