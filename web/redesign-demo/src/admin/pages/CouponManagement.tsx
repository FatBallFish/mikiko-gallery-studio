import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const CouponManagement: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className={rdAdmin.sectionHeader}>
        <div className="flex gap-4">
          <button className={cn(rdForm.button, rdForm.buttonPrimary)}>单码创建</button>
          <button className={cn(rdForm.button, rdForm.buttonSecondary)}>批量生成</button>
        </div>
        <button className={cn(rdForm.button, rdForm.buttonSecondary)}>导出 CSV</button>
      </div>

      <div className={rdAdmin.tableWrapper}>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className={rdAdmin.th}>兑换码</th>
              <th className={rdAdmin.th}>奖励值</th>
              <th className={rdAdmin.th}>有效期</th>
              <th className={rdAdmin.th}>使用情况</th>
              <th className={rdAdmin.th}>状态</th>
              <th className={rdAdmin.th}>操作</th>
            </tr>
          </thead>
          <tbody>
            <CouponRow code="MIKIKO-SUMMER-2026" value="50.00" expiry="2026/08/31" used="124/500" status="available" />
            <CouponRow code="VIP-SPECIAL-99" value="100.00" expiry="2026/12/31" used="10/10" status="exhausted" />
            <CouponRow code="NEW-USER-GIFT" value="10.00" expiry="2027/01/01" used="4,250 / ∞" status="available" />
            <CouponRow code="EXPIRED-CODE-01" value="20.00" expiry="2026/05/01" used="45/100" status="expired" />
          </tbody>
        </table>
      </div>
    </div>
  )
}

const CouponRow = ({ code, value, expiry, used, status }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}><span className="font-mono font-bold text-white">{code}</span></td>
    <td className={rdAdmin.td}>
      <div className="flex items-baseline gap-1">
        <span className="text-emerald-400 font-black">{value}</span>
        <span className="text-[10px] text-white/30">POINTS</span>
      </div>
    </td>
    <td className={rdAdmin.td}><span className="text-white/40">{expiry}</span></td>
    <td className={rdAdmin.td}>
       <div className="flex flex-col gap-1.5">
         <span className="text-xs text-white/60">{used}</span>
         <div className="w-24 h-1 bg-white/5 rounded-full overflow-hidden">
           <div className="h-full bg-[var(--accent)]" style={{ width: '60%' }} />
         </div>
       </div>
    </td>
    <td className={rdAdmin.td}>
      <span className={cn(
        rdAdmin.badge,
        status === 'available' ? rdAdmin.badgeSuccess : 
        status === 'exhausted' ? rdAdmin.badgeWarning : rdAdmin.badgeError
      )}>
        {status}
      </span>
    </td>
    <td className={rdAdmin.td}>
      <button className="text-[var(--accent)] hover:underline text-xs font-bold">查看详情</button>
    </td>
  </tr>
)
