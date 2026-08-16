import { useEffect, useState } from 'react'
import type { AdminVideoConfiguration } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { InlineFeedback } from '../components'
import { adminButton } from '../ui/classes'

export function VideoConfigurationImpact({ context }: { context: 'models' | 'pricing' | 'routing' }) {
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => { void adminApi.getVideoConfiguration().then(setSnapshot).catch((caught) => setError(caught instanceof Error ? caught.message : '视频配置摘要载入失败')) }, [])
  if (error) return <InlineFeedback tone="warning" message={error} />
  if (!snapshot) return null
  const blocking = snapshot.impacts.filter((impact) => impact.blocking)
  const links = context === 'models' ? [['pricing', '查看视频报价'], ['routing', '查看视频路由']] : context === 'pricing' ? [['access-accounts', '查看真实模型费率'], ['routing', '查看受影响路由']] : [['access-accounts', '查看真实模型能力与费率'], ['pricing', '查看报价总览']]
  return <section className="grid gap-3 border-y border-[var(--border)] py-4" data-video-configuration-impact>
    <div className="flex flex-wrap items-center justify-between gap-3"><div><strong className="text-sm text-[var(--text)]">视频配置影响摘要</strong><p className="m-0 mt-1 text-xs text-[var(--soft)]">能力 {snapshot.capabilities.length} 版 · 销售费率 {snapshot.rate_cards.length} 版 · 路由 {snapshot.routes.length} 版。</p></div><div className="flex flex-wrap gap-2">{links.map(([href, label]) => <a key={href} className={cn(adminButton.base, adminButton.ghost, adminButton.small)} href={`#/${href}`}>{label}</a>)}</div></div>
    {blocking.length ? <InlineFeedback tone="danger" message={`${blocking.length} 项阻断视频上线：${blocking.slice(0, 3).map((item) => item.summary).join('；')}`} /> : <InlineFeedback tone="success" message="当前未发现候选、能力或销售费率缺失的启用视频配置。" />}
  </section>
}
