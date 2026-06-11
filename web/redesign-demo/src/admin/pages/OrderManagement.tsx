import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const OrderManagement: React.FC = () => {
  const [tab, setTab] = useState<'overview' | 'records'>('overview')

  return (
    <div className="space-y-8">
      <div className="flex gap-8 border-b border-white/5 pb-4">
        <TabButton active={tab === 'overview'} onClick={() => setTab('overview')} label="订单概览" />
        <TabButton active={tab === 'records'} onClick={() => setTab('records')} label="订单记录" />
      </div>

      {tab === 'overview' ? <OrderOverview /> : <OrderRecords />}
    </div>
  )
}

const TabButton = ({ active, onClick, label }: any) => (
  <button 
    onClick={onClick}
    className={cn(
      "text-sm font-bold transition-all relative pb-4",
      active ? "text-[var(--accent)]" : "text-white/30 hover:text-white"
    )}
  >
    {label}
    {active && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent)] shadow-[0_0_10px_var(--accent)]" />}
  </button>
)

const OrderOverview = () => (
  <div className="space-y-10">
    {/* Financial Stats */}
    <div className={rdAdmin.statGrid}>
      <FinancialStatCard label="今日收入" value="¥ 4,820.00" trend="+12.5%" positive />
      <FinancialStatCard label="今日订单数" value="128" trend="+8.2%" positive />
      <FinancialStatCard label="平均订单金额" value="¥ 37.65" trend="+4.1%" positive />
      <FinancialStatCard label="总营收" value="¥ 124,500.00" trend="+15%" positive />
    </div>

    {/* Trends */}
    <div className={rdAdmin.chartContainer}>
      <div className={rdAdmin.sectionHeader}>
        <h3 className={rdAdmin.sectionTitle}>近 30 天营收趋势 / 30-Day Revenue Trend</h3>
      </div>
      <div className="h-[300px] w-full flex items-end gap-1 px-4">
        {[...Array(30)].map((_, i) => (
          <div 
            key={i} 
            className="flex-1 bg-[var(--accent)]/20 hover:bg-[var(--accent)] transition-all rounded-t-sm relative group"
            style={{ height: `${Math.random() * 80 + 10}%` }}
          >
            <div className="absolute -top-10 left-1/2 -translate-x-1/2 bg-black/80 text-[10px] text-white px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap z-10">
              ¥ {(Math.random() * 500 + 100).toFixed(2)}
            </div>
          </div>
        ))}
      </div>
      <div className="flex justify-between mt-4 text-[10px] text-white/20 font-bold uppercase tracking-widest px-4">
        <span>30 Days Ago</span>
        <span>Today</span>
      </div>
    </div>

    <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
      {/* Payment Distribution */}
      <div className={rdAdmin.chartContainer}>
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>支付方式分布 / Payment Methods</h3>
        </div>
        <div className="space-y-4">
          <DistributionRow label="支付宝 (Alipay)" value="¥ 84,200" percentage={68} color="bg-blue-500" />
          <DistributionRow label="微信支付 (WeChat Pay)" value="¥ 32,100" percentage={26} color="bg-emerald-500" />
          <DistributionRow label="Stripe / 其他" value="¥ 8,200" percentage={6} color="bg-purple-500" />
        </div>
      </div>

      {/* User Spending Ranking */}
      <div className={rdAdmin.chartContainer}>
        <div className={rdAdmin.sectionHeader}>
          <h3 className={rdAdmin.sectionTitle}>用户消费排行 / Top Spenders</h3>
        </div>
        <div className="space-y-4">
          <UserSpendingRow name="FatBallFish" amount="¥ 4,250.00" orders={12} />
          <UserSpendingRow name="CyberNinja" amount="¥ 2,840.00" orders={8} />
          <UserSpendingRow name="CreativeSoul" amount="¥ 1,920.00" orders={15} />
          <UserSpendingRow name="PixelMaster" amount="¥ 1,200.00" orders={4} />
        </div>
      </div>
    </div>
  </div>
)

const FinancialStatCard = ({ label, value, trend, positive }: any) => (
  <div className={rdAdmin.statCard}>
    <div className={rdAdmin.statLabel}>{label}</div>
    <div className="text-3xl font-black text-white tracking-tighter mb-2">{value}</div>
    <div className={cn("text-xs font-bold flex items-center gap-1", positive ? "text-emerald-400" : "text-rose-400")}>
      {positive ? '↑' : '↓'} {trend}
      <span className="text-white/20 font-normal ml-1">vs yesterday</span>
    </div>
  </div>
)

const DistributionRow = ({ label, value, percentage, color }: any) => (
  <div className="space-y-2">
    <div className="flex justify-between text-xs font-bold">
      <span className="text-white/60">{label}</span>
      <span className="text-white">{value} ({percentage}%)</span>
    </div>
    <div className="w-full h-1.5 bg-white/5 rounded-full overflow-hidden">
      <div className={cn("h-full", color)} style={{ width: `${percentage}%` }} />
    </div>
  </div>
)

const UserSpendingRow = ({ name, amount, orders }: any) => (
  <div className="flex items-center justify-between p-3 rounded-2xl hover:bg-white/5 transition-all">
    <div className="flex items-center gap-3">
      <div className="size-8 rounded-lg bg-white/5 flex items-center justify-center font-bold text-white/20 text-xs">
        {name.slice(0, 1)}
      </div>
      <div className="flex flex-col">
        <span className="text-sm font-bold text-white">{name}</span>
        <span className="text-[10px] text-white/30">{orders} 笔订单</span>
      </div>
    </div>
    <div className="text-right">
      <div className="text-sm font-black text-emerald-400">{amount}</div>
    </div>
  </div>
)

const OrderRecords = () => (
  <div className="space-y-6">
    <div className="flex justify-between items-center">
      <div className="flex gap-4 flex-1 max-w-2xl">
        <input type="text" placeholder="订单号 / 用户 ID..." className={rdForm.input} />
        <select className={rdForm.input + " w-40"}><option>所有状态</option></select>
      </div>
    </div>
    <div className={rdAdmin.tableWrapper}>
      <table className={rdAdmin.table}>
        <thead>
          <tr>
            <th className={rdAdmin.th}>订单号</th>
            <th className={rdAdmin.th}>用户信息</th>
            <th className={rdAdmin.th}>支付金额</th>
            <th className={rdAdmin.th}>购买内容</th>
            <th className={rdAdmin.th}>状态</th>
            <th className={rdAdmin.th}>时间</th>
          </tr>
        </thead>
        <tbody>
          <OrderRow id="ORD-2026-0611-001" user="FatBallFish" amount="¥ 99.00" item="1000 积分叠加包" status="paid" time="2026/06/11 15:45" />
          <OrderRow id="ORD-2026-0611-002" user="CyberNinja" amount="¥ 19.90" item="VIP 月度会员" status="paid" time="2026/06/11 14:20" />
          <OrderRow id="ORD-2026-0611-003" user="NewUser_88" amount="¥ 299.00" item="5000 积分专业包" status="pending" time="2026/06/11 13:05" />
        </tbody>
      </table>
    </div>
  </div>
)

const OrderRow = ({ id, user, amount, item, status, time }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}><span className="font-mono text-white/60">{id}</span></td>
    <td className={rdAdmin.td}><span className="font-bold text-white">{user}</span></td>
    <td className={rdAdmin.td}><span className="text-emerald-400 font-black">{amount}</span></td>
    <td className={rdAdmin.td}><span className="text-white/70">{item}</span></td>
    <td className={rdAdmin.td}><span className={cn(rdAdmin.badge, status === 'paid' ? rdAdmin.badgeSuccess : rdAdmin.badgeWarning)}>{status}</span></td>
    <td className={rdAdmin.td}><span className="text-white/30">{time}</span></td>
  </tr>
)
