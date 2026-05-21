import { useEffect, useMemo, useState } from 'react'
import type { ConfigItem } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, useFilteredTabs } from '../components'

type DraftMap = Record<string, string>

function detectConfigConflict(row: ConfigItem, nextValue: string) {
  if (!nextValue.trim()) return '值不能为空'
  if (row.key.includes('max') && Number(nextValue) > 8) return '超过当前集群安全上限 8，需先扩容 Worker。'
  if (row.key.includes('ttl') && Number(nextValue) < 300) return 'Token TTL 低于 300 秒会触发频繁刷新。'
  if (row.key.includes('exchange') && Number(nextValue) <= 0) return '兑换比例必须大于 0。'
  return null
}

export function ConfigPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ConfigItem[]>([])
  const [drafts, setDrafts] = useState<DraftMap>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingKey, setSavingKey] = useState<string | null>(null)
  const [publishing, setPublishing] = useState(false)
  const [notice, setNotice] = useState<string>('配置主表已连接 Mock API。')

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextRows = await mockApi.listConfig()
      setRows(nextRows)
      setDrafts(Object.fromEntries(nextRows.map((row) => [row.key, row.draft_value])))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '配置载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const { tabs, activeTab, setActiveTab, visibleRows } = useFilteredTabs(rows)
  const dirtyCount = rows.filter((row) => row.state !== 'active' || drafts[row.key] !== row.value).length
  const conflicts = useMemo(() => rows.flatMap((row) => {
    const conflict = detectConfigConflict(row, drafts[row.key] ?? row.draft_value)
    return conflict ? [{ key: row.key, conflict }] : []
  }), [drafts, rows])

  const saveRow = async (row: ConfigItem) => {
    const draftValue = drafts[row.key] ?? row.draft_value
    const conflict = detectConfigConflict(row, draftValue)
    if (conflict) {
      setNotice(`${row.key}: ${conflict}`)
      return
    }
    setSavingKey(row.key)
    try {
      const updated = await mockApi.editConfig(row.key, draftValue)
      setRows((current) => current.map((item) => item.key === row.key ? updated : item))
      setNotice(`${row.key} 已保存为草稿。`)
      onFeedback('配置草稿已保存', row.key)
    } catch (caught) {
      setNotice(caught instanceof Error ? caught.message : '保存失败')
    } finally {
      setSavingKey(null)
    }
  }

  const publish = async () => {
    if (conflicts.length) {
      setNotice(`仍有 ${conflicts.length} 项冲突，发布已阻止。`)
      return
    }
    setPublishing(true)
    try {
      const nextRows = await mockApi.publishConfig()
      setRows(nextRows)
      setDrafts(Object.fromEntries(nextRows.map((row) => [row.key, row.draft_value])))
      setNotice('配置已发布，全量 API 节点将在 1 分钟内生效。')
      onFeedback('配置发布成功', '审计日志已记录 PUBLISH_CONFIG')
    } finally {
      setPublishing(false)
    }
  }

  const revert = async () => {
    setPublishing(true)
    try {
      const nextRows = await mockApi.revertConfig()
      setRows(nextRows)
      setDrafts(Object.fromEntries(nextRows.map((row) => [row.key, row.value])))
      setNotice('所有配置草稿已回滚到当前生效值。')
      onFeedback('配置已回滚', '草稿和冲突状态已清空')
    } finally {
      setPublishing(false)
    }
  }

  if (loading) return <LoadingBlock label="载入配置中心" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无配置项" detail="配置中心尚未返回可编辑条目。" />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Config"
        title="配置中心"
        detail="内联编辑、冲突检测、发布与回滚都贴近配置主表，避免操作反馈丢失。"
        actions={
          <>
            <button type="button" className="ghost" onClick={revert} disabled={publishing || !dirtyCount}>回滚草稿</button>
            <button type="button" className="btn primary" onClick={publish} disabled={publishing || !dirtyCount || Boolean(conflicts.length)}>{publishing ? '发布中...' : `发布 ${dirtyCount}`}</button>
          </>
        }
      />

      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>草稿</label><strong>{dirtyCount} 项待处理</strong></div>
        <div className="status-cell"><label>冲突</label><strong>{conflicts.length ? `${conflicts.length} 项阻塞` : '无冲突'}</strong></div>
        <div className="status-cell"><label>发布轨迹</label><strong>v2026.05.21 可回滚</strong></div>
        <div className="status-cell"><label>生效范围</label><strong>全量 API 节点</strong></div>
      </section>

      <section className="pg-admin-card config-motherboard">
        <section className="config-sheet-lane">
          <div className="config-toolbar-band">
            <div className="config-mode-tabs">
              {tabs.map((tab) => <button key={tab} className={activeTab === tab ? 'active' : ''} type="button" onClick={() => setActiveTab(tab)}>{tab}</button>)}
            </div>
            <div className="config-toolbar-meta"><span>Draft {dirtyCount}</span><span>{conflicts.length ? 'Conflict' : 'Clean'}</span><span>Mock API</span></div>
          </div>

          <div className="config-sheet-head config-board-grid"><span>分类</span><span>配置项</span><span>草稿值</span><span>状态</span><span>操作</span></div>
          {visibleRows.map((row) => {
            const draftValue = drafts[row.key] ?? row.draft_value
            const conflict = detectConfigConflict(row, draftValue)
            const dirty = draftValue !== row.draft_value || row.state !== 'active'
            return (
              <div key={row.key} className={`config-sheet-row config-board-grid ${conflict ? 'has-conflict' : ''}`}>
                <strong>{row.tab}</strong>
                <div><code>{row.key}</code><p>{row.description} · 当前 {row.value} · v{row.version}</p></div>
                <input value={draftValue} onChange={(event) => setDrafts((current) => ({ ...current, [row.key]: event.target.value }))} aria-label={`${row.key} 草稿值`} />
                <Badge tone={conflict ? 'warning' : row.state === 'active' && !dirty ? 'success' : 'warning'}>{conflict ? '冲突' : row.state === 'active' && !dirty ? '已生效' : '待发布'}</Badge>
                <button type="button" className="btn small" onClick={() => saveRow(row)} disabled={savingKey === row.key || Boolean(conflict) || !dirty}>{savingKey === row.key ? '保存中' : '保存'}</button>
                {conflict ? <InlineFeedback tone="warning" message={conflict} /> : null}
              </div>
            )
          })}
        </section>

        <aside className="config-side-rail">
          <section className="side-strip"><label>发布反馈</label><strong>{notice}</strong></section>
          <section className="side-strip"><label>冲突检测</label>{conflicts.length ? conflicts.map((item) => <p key={item.key}>{item.key}: {item.conflict}</p>) : <p>所有草稿均可进入发布流程。</p>}</section>
          <section className="side-strip"><label>发布策略</label><p>预发布写入草稿，点击发布后 Mock API 统一升级版本并写审计。</p></section>
        </aside>
      </section>
    </section>
  )
}
