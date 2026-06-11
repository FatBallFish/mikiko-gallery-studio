import React from 'react'
import { cn } from '../../../shared/classnames'
import { rdAdmin } from '../admin-classes'

export const Monitoring: React.FC = () => {
  return (
    <div className="space-y-10">
      {/* Infrastructure Health */}
      <div className={rdAdmin.sectionHeader}>
        <h3 className={rdAdmin.sectionTitle}>基础信息 / Infrastructure</h3>
      </div>
      <div className={rdAdmin.healthGrid}>
        <HealthCard label="CPU 使用率" value="24.5%" status="success" icon={<CpuIcon />} />
        <HealthCard label="内存使用率" value="6.2 / 16 GB" status="success" icon={<MemoryIcon />} />
        <HealthCard label="数据库连接池" value="12 / 100" status="success" icon={<DbIcon />} />
        <HealthCard label="Redis 连接池" value="5 / 50" status="success" icon={<ZapIcon />} />
        <HealthCard label="Worker 健康情况" value="8 / 8 Online" status="success" icon={<WorkerIcon />} />
        <HealthCard label="带宽情况" value="12.4 Mbps" status="success" icon={<NetworkIcon />} />
      </div>

      {/* SLA Metrics */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <div className={rdAdmin.chartContainer}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>SLA 指标 / Metrics</h3>
            <div className="flex gap-2">
              <TimePill label="1m" />
              <TimePill label="5m" active />
              <TimePill label="1h" />
            </div>
          </div>
          <div className="flex items-center justify-between mb-8">
            <div>
              <div className={rdAdmin.slaValue}>99.98%</div>
              <div className={rdAdmin.slaLabel}>健康总评分 / Health Score</div>
            </div>
            <div className="text-right">
              <div className="text-xl font-bold text-white">12.4 req/s</div>
              <div className={rdAdmin.slaLabel}>实时 QPS</div>
            </div>
          </div>
          <div className="space-y-4">
            <MetricRow label="请求错误率" value="0.02%" status="success" />
            <MetricRow label="上游错误率" value="0.05%" status="success" />
            <MetricRow label="峰值 QPS" value="48.5 req/s" status="info" />
          </div>
        </div>

        <div className={rdAdmin.chartContainer}>
          <div className={rdAdmin.sectionHeader}>
            <h3 className={rdAdmin.sectionTitle}>生图耗时 / Latency</h3>
          </div>
          <div className="grid grid-cols-2 gap-6">
            <LatencyCard label="P95" value="12.4s" />
            <LatencyCard label="P90" value="8.2s" />
            <LatencyCard label="P50" value="4.5s" />
            <LatencyCard label="Avg" value="5.1s" />
          </div>
          <div className="mt-8 pt-8 border-t border-white/5">
             <div className="flex justify-between items-center text-[10px] font-bold text-white/30 uppercase tracking-widest">
               <span>Max Latency</span>
               <span className="text-rose-400">45.2s</span>
             </div>
          </div>
        </div>
      </div>

      {/* Worker & Account Concurrency */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <div className={rdAdmin.tableWrapper}>
          <div className="p-6 border-b border-white/5 bg-white/[0.02]">
            <h3 className="text-sm font-bold text-white">Worker 节点并发 / Worker Nodes</h3>
          </div>
          <table className={rdAdmin.table}>
            <thead>
              <tr>
                <th className={rdAdmin.th}>节点名称</th>
                <th className={rdAdmin.th}>当前并发</th>
                <th className={rdAdmin.th}>可用容量</th>
                <th className={rdAdmin.th}>使用率</th>
                <th className={rdAdmin.th}>错误数</th>
              </tr>
            </thead>
            <tbody>
              <WorkerRow name="worker-node-01" current="12" total="20" usage={60} errors="0" />
              <WorkerRow name="worker-node-02" current="8" total="20" usage={40} errors="2" />
              <WorkerRow name="worker-node-03" current="18" total="20" usage={90} errors="5" warning />
            </tbody>
          </table>
        </div>

        <div className={rdAdmin.tableWrapper}>
           <div className="p-6 border-b border-white/5 bg-white/[0.02]">
            <h3 className="text-sm font-bold text-white">底层账号并发 / Accounts</h3>
          </div>
          <table className={rdAdmin.table}>
            <thead>
              <tr>
                <th className={rdAdmin.th}>账号 ID</th>
                <th className={rdAdmin.th}>当前并发</th>
                <th className={rdAdmin.th}>可用容量</th>
                <th className={rdAdmin.th}>使用率</th>
                <th className={rdAdmin.th}>状态</th>
              </tr>
            </thead>
            <tbody>
              <AccountRow id="acc_mj_001" current="3" total="3" usage={100} status="full" />
              <AccountRow id="acc_mj_002" current="1" total="3" usage={33} status="active" />
              <AccountRow id="acc_flux_v1" current="5" total="10" usage={50} status="active" />
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

const HealthCard = ({ label, value, status, icon }: { label: string; value: string; status: string; icon: React.ReactNode }) => (
  <div className={rdAdmin.healthCard}>
    <div className={rdAdmin.healthIcon}>{icon}</div>
    <div className={rdAdmin.healthContent}>
      <div className={rdAdmin.healthLabel}>{label}</div>
      <div className={rdAdmin.healthValue}>{value}</div>
    </div>
    <div className={cn(rdAdmin.healthStatus, status === 'success' ? 'text-emerald-500' : 'text-rose-500')} />
  </div>
)

const TimePill = ({ label, active }: { label: string; active?: boolean }) => (
  <button className={cn(
    "px-3 py-1 rounded-full text-[10px] font-bold transition-all",
    active ? "bg-[var(--accent)] text-white" : "bg-white/5 text-white/40 hover:bg-white/10"
  )}>
    {label}
  </button>
)

const MetricRow = ({ label, value, status }: { label: string; value: string; status: string }) => (
  <div className="flex items-center justify-between p-4 rounded-2xl bg-white/[0.02] border border-white/5">
    <span className="text-xs font-medium text-white/50">{label}</span>
    <span className={cn(
      "text-sm font-bold",
      status === 'success' ? 'text-emerald-400' : status === 'info' ? 'text-[var(--accent)]' : 'text-rose-400'
    )}>{value}</span>
  </div>
)

const LatencyCard = ({ label, value }: { label: string; value: string }) => (
  <div className="p-6 rounded-2xl bg-white/[0.03] border border-white/5">
    <div className="text-[10px] font-bold text-white/30 uppercase tracking-widest mb-1">{label}</div>
    <div className="text-2xl font-black text-white">{value}</div>
  </div>
)

const WorkerRow = ({ name, current, total, usage, errors, warning }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}>
      <div className="flex items-center gap-2">
        <div className={cn("size-2 rounded-full", warning ? "bg-amber-500" : "bg-emerald-500")} />
        <span className="font-mono">{name}</span>
      </div>
    </td>
    <td className={rdAdmin.td}>{current}</td>
    <td className={rdAdmin.td}>{total}</td>
    <td className={rdAdmin.td}>
      <div className="flex items-center gap-3">
        <div className="flex-1 h-1.5 rounded-full bg-white/5 overflow-hidden max-w-[60px]">
          <div className={cn("h-full", warning ? "bg-amber-500" : "bg-[var(--accent)]")} style={{ width: `${usage}%` }} />
        </div>
        <span className="text-[10px] font-bold">{usage}%</span>
      </div>
    </td>
    <td className={rdAdmin.td}>
      <span className={cn(errors > 0 ? "text-rose-400" : "text-white/20")}>{errors}</span>
    </td>
  </tr>
)

const AccountRow = ({ id, current, total, usage, status }: any) => (
  <tr className={rdAdmin.tr}>
    <td className={rdAdmin.td}><span className="font-mono">{id}</span></td>
    <td className={rdAdmin.td}>{current}</td>
    <td className={rdAdmin.td}>{total}</td>
    <td className={rdAdmin.td}>
      <div className="flex items-center gap-3">
        <div className="flex-1 h-1.5 rounded-full bg-white/5 overflow-hidden max-w-[60px]">
          <div className={cn("h-full", status === 'full' ? "bg-rose-500" : "bg-emerald-500")} style={{ width: `${usage}%` }} />
        </div>
        <span className="text-[10px] font-bold">{usage}%</span>
      </div>
    </td>
    <td className={rdAdmin.td}>
       <span className={cn(rdAdmin.badge, status === 'full' ? rdAdmin.badgeWarning : rdAdmin.badgeSuccess)}>
         {status}
       </span>
    </td>
  </tr>
)

// Icons
const CpuIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M9 9h6v6H9zM15 2v2M9 2v2M20 15h2M20 9h2M15 20v2M9 20v2M2 15h2M2 9h2"/></svg>
const MemoryIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 18a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-3V4a2 2 0 0 0-2-2H9a2 2 0 0 0-2 2v2H4a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2"/><path d="M2 10h20M2 14h20"/></svg>
const DbIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>
const ZapIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
const WorkerIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><path d="M6 6h.01M6 18h.01"/></svg>
const NetworkIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="16" y="16" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="9" y="2" width="6" height="6" rx="1"/><path d="M5 16v-3a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v3M12 16v-5"/></svg>
