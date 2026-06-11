import React, { useState, useEffect } from 'react'
import { rdAdmin } from '../admin-classes'
import { FullPageLoader } from '../components/ui'

export const Dashboard: React.FC = () => {
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    // Simulate initial data fetching
    const timer = setTimeout(() => setIsLoading(false), 800)
    return () => clearTimeout(timer)
  }, [])

  if (isLoading) return <FullPageLoader text="Loading Dashboard Data" />

  return (
    <div className="space-y-10 animate-in fade-in duration-500">
      {/* Stat Cards */}
      <div className={rdAdmin.statGrid}>
        <StatCard label="总用户数" value="12,840" trend="+12%" positive />
        <StatCard label="今日生图总数" value="2,450" trend="+5.2%" positive />
        <StatCard label="今日消耗积分" value="48,200" trend="-2.1%" positive={false} />
        <StatCard label="总消耗积分" value="1.2M" trend="+18%" positive />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        {/* Model Distribution */}
        <div className={rdAdmin.chartContainer + " lg:col-span-2"}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>模型调用分布 / Model Distribution</h3>
          </div>
          <div className="h-[300px] flex items-center justify-center border border-dashed border-white/10 rounded-2xl bg-white/[0.01]">
            <span className="text-white/20 font-bold uppercase tracking-widest">Pie Chart Visualization</span>
          </div>
          <div className="grid grid-cols-3 gap-4 mt-8">
            <ModelStat name="Flux Pro" calls="1,240" cost="24,800" />
            <ModelStat name="Midjourney v6" calls="850" cost="12,750" />
            <ModelStat name="DALL-E 3" calls="360" cost="3,600" />
          </div>
        </div>

        {/* User Ranking */}
        <div className={rdAdmin.chartContainer}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>用户消费榜 / Rankings</h3>
          </div>
          <div className="space-y-4">
            <UserRank name="FatBallFish" tasks="120" cost="2,400" />
            <UserRank name="CyberNinja" tasks="98" cost="1,960" />
            <UserRank name="DesignMaster" tasks="85" cost="1,700" />
            <UserRank name="CreativeSoul" tasks="72" cost="1,440" />
            <UserRank name="PixelArt" tasks="64" cost="1,280" />
          </div>
        </div>
      </div>
    </div>
  )
}

const StatCard = ({ label, value, trend, positive }: { label: string; value: string; trend: string; positive: boolean }) => (
  <div className={rdAdmin.statCard}>
    <div className={rdAdmin.statLabel}>{label}</div>
    <div className={rdAdmin.statValue}>{value}</div>
    <div className={rdAdmin.statTrend + (positive ? ` ${rdAdmin.statPositive}` : ` ${rdAdmin.statNegative}`)}>
      {positive ? <ArrowUpIcon /> : <ArrowDownIcon />}
      {trend}
      <span className="text-white/20 font-normal ml-1">vs last week</span>
    </div>
    <div className={rdAdmin.statChart}>
      <svg className="w-full h-full" preserveAspectRatio="none">
        <path 
          d={`M0 64 Q 25 20, 50 40 T 100 30 T 150 50 T 200 10 T 250 40 T 300 20 T 400 64 L 400 64 L 0 64 Z`} 
          fill="url(#gradient)" 
          className={positive ? 'text-emerald-500' : 'text-rose-500'}
        />
        <defs>
          <linearGradient id="gradient" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="currentColor" stopOpacity="0.2" />
            <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
          </linearGradient>
        </defs>
      </svg>
    </div>
  </div>
)

const ModelStat = ({ name, calls, cost }: { name: string; calls: string; cost: string }) => (
  <div className="p-4 rounded-2xl bg-white/[0.03] border border-white/5">
    <div className="text-[10px] font-bold text-white/30 uppercase tracking-widest mb-1">{name}</div>
    <div className="flex items-baseline gap-2">
      <span className="text-lg font-black text-white">{calls}</span>
      <span className="text-[10px] text-white/40">calls</span>
    </div>
    <div className="text-[10px] font-mono text-[var(--accent)] mt-1">{cost} points consumed</div>
  </div>
)

const UserRank = ({ name, tasks, cost }: { name: string; tasks: string; cost: string }) => (
  <div className="flex items-center justify-between p-4 rounded-2xl hover:bg-white/[0.03] transition-colors border border-transparent hover:border-white/5 group">
    <div className="flex items-center gap-3">
      <div className="size-8 rounded-lg bg-gradient-to-br from-white/10 to-white/5 flex items-center justify-center text-xs font-bold text-white/40">
        {name.slice(0, 1)}
      </div>
      <div className="flex flex-col">
        <span className="text-sm font-bold text-white group-hover:text-[var(--accent)] transition-colors">{name}</span>
        <span className="text-[10px] text-white/30">{tasks} tasks completed</span>
      </div>
    </div>
    <div className="text-right">
      <div className="text-sm font-black text-white">{cost}</div>
      <div className="text-[10px] text-white/30 uppercase tracking-tighter">points</div>
    </div>
  </div>
)

const ArrowUpIcon = () => <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="m5 12 7-7 7 7M12 19V5"/></svg>
const ArrowDownIcon = () => <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="m19 12-7 7-7-7M12 5v14"/></svg>
