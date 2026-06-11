import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin, rdForm } from '../admin-classes'

export const CallRecords: React.FC = () => {
  return (
    <div className="space-y-10">
      {/* Filters and Search */}
      <div className="flex flex-col gap-4 p-6 rounded-3xl border border-white/5 bg-white/[0.02]">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-bold text-white uppercase tracking-widest">调用记录查询 / Call Records</h3>
          <div className="flex gap-2">
            <TimePill label="今天" active />
            <TimePill label="昨天" />
            <TimePill label="近 7 天" />
            <TimePill label="近 30 天" />
            <TimePill label="自定义区间" />
          </div>
        </div>
        <div className="flex flex-wrap gap-4 pt-4 border-t border-white/5">
          <input type="text" placeholder="用户 ID / 用户名" className={cn(rdForm.input, "w-48")} />
          <select className={cn(rdForm.input, "w-48")}><option>所有路由模型</option></select>
          <select className={cn(rdForm.input, "w-48")}><option>所有底层账号</option></select>
          <select className={cn(rdForm.input, "w-48")}><option>所有执行状态</option></select>
          <button className={cn(rdForm.button, rdForm.buttonPrimary, "px-8")}>查询</button>
        </div>
      </div>

      {/* Aggregate Stats */}
      <div className={rdAdmin.statGrid}>
        <StatCard label="区间总任务数" value="12,450" />
        <StatCard label="区间生图数" value="28,920" />
        <StatCard label="区间消耗积分" value="84,500.00" />
        <StatCard label="平均生图耗时" value="8.4s" />
      </div>

      {/* Charts / Distributions */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <DistributionCard title="路由模型调用量 (Route Models)">
          <DistributionRow label="Flux Pro (Fast)" value="6,240" percentage={50} color="bg-blue-500" />
          <DistributionRow label="Midjourney v6" value="4,850" percentage={39} color="bg-emerald-500" />
          <DistributionRow label="DALL-E 3" value="1,360" percentage={11} color="bg-purple-500" />
        </DistributionCard>
        
        <DistributionCard title="底层账号调用量 (Actual Accounts)">
          <DistributionRow label="SiliconFlow-Main" value="4,500" percentage={36} color="bg-orange-500" />
          <DistributionRow label="OpenRouter-Exp" value="3,200" percentage={26} color="bg-pink-500" />
          <DistributionRow label="MJ-Pool-01" value="4,750" percentage={38} color="bg-indigo-500" />
        </DistributionCard>

        <DistributionCard title="底层模型调用量 (Actual Models)">
          <DistributionRow label="flux-1-pro" value="4,200" percentage={34} color="bg-cyan-500" />
          <DistributionRow label="mj-v6-turbo" value="4,850" percentage={39} color="bg-teal-500" />
          <DistributionRow label="dall-e-3-hd" value="1,360" percentage={11} color="bg-yellow-500" />
          <DistributionRow label="flux-1-schnell" value="2,040" percentage={16} color="bg-rose-500" />
        </DistributionCard>
      </div>

      {/* Detailed Records Table */}
      <div className={rdAdmin.tableWrapper}>
        <div className="flex items-center justify-between p-6 border-b border-white/5 bg-white/[0.01]">
          <h3 className="text-sm font-bold text-white">调用记录明细 (Detailed Logs)</h3>
          <button className={cn(rdForm.button, rdForm.buttonSecondary, "text-xs px-4 py-1.5")}>导出 CSV</button>
        </div>
        <table className={rdAdmin.table}>
          <thead>
            <tr>
              <th className={rdAdmin.th}>任务 ID / 时间</th>
              <th className={rdAdmin.th}>用户信息</th>
              <th className={rdAdmin.th}>模型路由链路</th>
              <th className={rdAdmin.th}>配置 / 提示词</th>
              <th className={rdAdmin.th}>消耗积分</th>
              <th className={rdAdmin.th}>耗时</th>
              <th className={rdAdmin.th}>执行状态</th>
            </tr>
          </thead>
          <tbody>
            <RecordRow 
              id="tsk_99218a" time="2026/06/11 14:32:10" 
              user="FatBallFish" 
              route="v-flux-pro" account="SiliconFlow-Main" actualModel="flux-1-pro"
              prompt="A cyber ninja holding a glowing katana..." quality="Auto" 
              points="1.25" latency="4.2s" status="success"
            />
            <RecordRow 
              id="tsk_99218b" time="2026/06/11 14:30:45" 
              user="DesignMaster" 
              route="v-mj-v6" account="OpenRouter-Exp" actualModel="mj-v6"
              prompt="Minimalist interior design, natural light..." quality="High" 
              points="2.50" latency="18.5s" status="success"
            />
            <RecordRow 
              id="tsk_99218c" time="2026/06/11 14:28:12" 
              user="SpamBot_01" 
              route="v-dalle-3" account="OpenAI-Direct" actualModel="dall-e-3"
              prompt="Violent content..." quality="Ultra" 
              points="0" latency="0.8s" status="failed" error="Content Policy Violation"
            />
          </tbody>
        </table>
        {/* Pagination */}
        <div className="p-6 border-t border-white/5 flex items-center justify-between">
          <span className="text-xs text-white/30">显示 1 到 3 共 12,450 条记录</span>
          <div className="flex gap-2">
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-4 py-1.5")}>上一页</button>
            <button className={cn(rdForm.button, rdForm.buttonPrimary, "px-4 py-1.5")}>1</button>
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-4 py-1.5")}>2</button>
            <button className={cn(rdForm.button, rdForm.buttonSecondary, "px-4 py-1.5")}>下一页</button>
          </div>
        </div>
      </div>
    </div>
  )
}

const TimePill = ({ label, active }: any) => (
  <button className={cn(
    "px-4 py-1.5 rounded-xl text-xs font-bold transition-all",
    active ? "bg-[var(--accent)] text-white" : "bg-white/5 text-white/40 hover:bg-white/10 hover:text-white"
  )}>
    {label}
  </button>
)

const StatCard = ({ label, value }: any) => (
  <div className={cn(rdAdmin.statCard, "py-8")}>
    <div className={rdAdmin.statLabel}>{label}</div>
    <div className="text-3xl font-black text-white tracking-tighter mt-2">{value}</div>
  </div>
)

const DistributionCard = ({ title, children }: any) => (
  <div className={rdAdmin.chartContainer}>
    <div className={rdAdmin.sectionHeader}>
      <h3 className="text-xs font-bold uppercase tracking-widest text-white/40">{title}</h3>
    </div>
    <div className="space-y-4">
      {children}
    </div>
  </div>
)

const DistributionRow = ({ label, value, percentage, color }: any) => (
  <div className="space-y-2">
    <div className="flex justify-between text-[10px] font-bold uppercase tracking-wider">
      <span className="text-white/60">{label}</span>
      <span className="text-white">{value} ({percentage}%)</span>
    </div>
    <div className="w-full h-1 bg-white/5 rounded-full overflow-hidden">
      <div className={cn("h-full", color)} style={{ width: `${percentage}%` }} />
    </div>
  </div>
)

const RecordRow = ({ id, time, user, route, account, actualModel, prompt, quality, points, latency, status, error }: any) => (
  <tr className={cn(rdAdmin.tr, status === 'failed' && "bg-rose-500/[0.02]")}>
    <td className={rdAdmin.td}>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-mono font-bold text-white/80">{id}</span>
        <span className="text-[10px] text-white/30">{time}</span>
      </div>
    </td>
    <td className={rdAdmin.td}><span className="font-bold text-white text-sm">{user}</span></td>
    <td className={rdAdmin.td}>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-bold text-[var(--accent)]">{route}</span>
        <span className="text-[10px] text-white/40">↳ {account} <span className="text-white/20 px-1">/</span> {actualModel}</span>
      </div>
    </td>
    <td className={rdAdmin.td}>
      <div className="flex flex-col gap-1 max-w-[200px]">
        <span className="text-[10px] bg-white/5 w-fit px-1.5 py-0.5 rounded text-white/60 font-mono">Q: {quality}</span>
        <span className="text-xs text-white/50 truncate" title={prompt}>{prompt}</span>
      </div>
    </td>
    <td className={rdAdmin.td}>
      {status === 'failed' ? (
        <span className="text-xs text-white/20">-</span>
      ) : (
        <div className="flex items-baseline gap-1">
          <span className="font-bold text-emerald-400">{points}</span>
          <span className="text-[10px] text-white/30">◈</span>
        </div>
      )}
    </td>
    <td className={rdAdmin.td}><span className="text-xs font-mono text-white/60">{latency}</span></td>
    <td className={rdAdmin.td}>
      {status === 'success' ? (
        <span className={rdAdmin.badgeSuccess + " " + rdAdmin.badge}>SUCCESS</span>
      ) : (
        <div className="flex flex-col gap-1">
          <span className={rdAdmin.badgeError + " " + rdAdmin.badge}>FAILED</span>
          <span className="text-[9px] text-rose-400 max-w-[120px] truncate" title={error}>{error}</span>
        </div>
      )}
    </td>
  </tr>
)
