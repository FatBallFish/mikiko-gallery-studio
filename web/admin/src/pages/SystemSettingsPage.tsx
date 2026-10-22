import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { Badge, EmptyBlock, PageHeader, RefreshIconButton } from '../components'
import { canAdmin } from '../types'
import { adminPage } from '../ui/classes'
import { ConfigPage } from './ConfigPage'
import { AttachmentPolicyPage } from './AttachmentPolicyPage'
import { SecurityConfigPage } from './SecurityConfigPage'
import { StorageConfigPage } from './StorageConfigPage'
import { MediaPolicyPage } from './MediaPolicyPage'
import { isSystemSettingsHash, systemSettingsTabFromHash, type SystemSettingsTab } from './systemSettingsTabs'

export { isSystemSettingsHash, systemSettingsTabFromHash } from './systemSettingsTabs'

const tabItems = [
  { id: 'general', label: '通用配置', description: '公开内容和低风险运行参数', dangerous: false },
  { id: 'point-conversion', label: '积分换算', description: '每积分对应的人民币金额', dangerous: false },
  { id: 'attachment-policy', label: '附件策略', description: '文件体积与格式限制', dangerous: false },
  { id: 'media-policy', label: '媒体策略', description: '媒体派生、预览与保留策略', dangerous: false },
  { id: 'security', label: '安全配置', description: 'SMTP 与敏感安全项', dangerous: true },
  { id: 'storage', label: '存储配置', description: 'Local / S3 / R2 多实例存储', dangerous: true },
] satisfies Array<{ id: SystemSettingsTab; label: string; description: string; dangerous: boolean }>

export function SystemSettingsPage({
  session,
  onFeedback,
}: {
  session: AdminSession
  onFeedback: (title: string, detail?: string) => void
}) {
  const [activeTab, setActiveTab] = useState<SystemSettingsTab>(() => systemSettingsTabFromHash(window.location.hash))
  const [refreshGeneration, setRefreshGeneration] = useState(0)
  const [dirtyTabs, setDirtyTabs] = useState<Record<SystemSettingsTab, boolean>>({ general: false, 'point-conversion': false, 'attachment-policy': false, 'media-policy': false, security: false, storage: false, 'text-models': false })
  const [busyTabs, setBusyTabs] = useState<Record<SystemSettingsTab, boolean>>({ general: false, 'point-conversion': false, 'attachment-policy': false, 'media-policy': false, security: false, storage: false, 'text-models': false })
  const activeTabRef = useRef(activeTab)
  const dirtyTabsRef = useRef(dirtyTabs)
  const busyTabsRef = useRef(busyTabs)
  const canManageDangerous = canAdmin(session, 'manage:dangerous_config')
  const items = tabItems.map((item) => ({
    ...item,
    disabled: (item.dangerous && !canManageDangerous) || (busyTabs[activeTab] && item.id !== activeTab),
  }))
  const activeTabMeta = tabItems.find((item) => item.id === activeTab) ?? tabItems[0]
  const dirtyCount = Object.values(dirtyTabs).filter(Boolean).length

  useEffect(() => { activeTabRef.current = activeTab }, [activeTab])
  useEffect(() => { dirtyTabsRef.current = dirtyTabs }, [dirtyTabs])
  useEffect(() => { busyTabsRef.current = busyTabs }, [busyTabs])
  useEffect(() => {
    if (activeTab === 'text-models') window.location.hash = '/access-accounts?media=text'
  }, [activeTab])

  useEffect(() => {
    const onHashChange = () => {
      const staysInSystemSettings = isSystemSettingsHash(window.location.hash)
      const nextTab = systemSettingsTabFromHash(window.location.hash)
      const currentTab = activeTabRef.current
      const isLeavingCurrentTab = !staysInSystemSettings || nextTab !== currentTab
      if (isLeavingCurrentTab && busyTabsRef.current[currentTab]) {
        window.history.replaceState(null, '', `#/system-settings?tab=${currentTab}`)
        window.dispatchEvent(new HashChangeEvent('hashchange'))
        return
      }
      if (isLeavingCurrentTab && dirtyTabsRef.current[currentTab] && !window.confirm('当前分区存在未保存修改，确定离开吗？')) {
        window.history.replaceState(null, '', `#/system-settings?tab=${currentTab}`)
        window.dispatchEvent(new HashChangeEvent('hashchange'))
        return
      }
      if (staysInSystemSettings) setActiveTab(nextTab)
    }
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  const switchTab = (tab: SystemSettingsTab) => {
    if (busyTabs[activeTab]) return
    if (tab === activeTab) return
    if (dirtyTabs[activeTab] && !window.confirm('当前分区存在未保存修改，确定离开吗？')) return
    activeTabRef.current = tab
    setActiveTab(tab)
    window.location.hash = `/system-settings?tab=${tab}`
  }

  const updateDirtyTab = useCallback((tab: SystemSettingsTab, dirty: boolean) => {
    setDirtyTabs((current) => current[tab] === dirty ? current : { ...current, [tab]: dirty })
  }, [])
  const updateBusyTab = useCallback((tab: SystemSettingsTab, busy: boolean) => {
    setBusyTabs((current) => current[tab] === busy ? current : { ...current, [tab]: busy })
  }, [])
  const onGeneralDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('general', dirty), [updateDirtyTab])
  const onPointConversionDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('point-conversion', dirty), [updateDirtyTab])
  const onAttachmentPolicyDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('attachment-policy', dirty), [updateDirtyTab])
  const onMediaPolicyDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('media-policy', dirty), [updateDirtyTab])
  const onSecurityDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('security', dirty), [updateDirtyTab])
  const onStorageDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('storage', dirty), [updateDirtyTab])
  const onGeneralBusyChange = useCallback((busy: boolean) => updateBusyTab('general', busy), [updateBusyTab])
  const onPointConversionBusyChange = useCallback((busy: boolean) => updateBusyTab('point-conversion', busy), [updateBusyTab])
  const onAttachmentPolicyBusyChange = useCallback((busy: boolean) => updateBusyTab('attachment-policy', busy), [updateBusyTab])
  const onMediaPolicyBusyChange = useCallback((busy: boolean) => updateBusyTab('media-policy', busy), [updateBusyTab])
  const onSecurityBusyChange = useCallback((busy: boolean) => updateBusyTab('security', busy), [updateBusyTab])
  const onStorageBusyChange = useCallback((busy: boolean) => updateBusyTab('storage', busy), [updateBusyTab])

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="System Settings"
        title="系统设置"
        description="配置字段使用单列账本布局：标签在上、控件全宽、帮助文本在下。左侧分区导航，保存栏固定在内容底部。"
        meta={<span className={dirtyCount ? 'a-badge a-badge--warn' : 'a-badge a-badge--ok'}><i />{dirtyCount ? `${dirtyCount} 个分区有未保存修改` : '所有分区已同步'}</span>}
        secondaryActions={<RefreshIconButton label="刷新当前设置分区" disabled={dirtyTabs[activeTab] || busyTabs[activeTab]} onClick={() => setRefreshGeneration((value) => value + 1)} />}
      />

      <div className="ss-layout">
        <nav className="ss-nav" aria-label="系统设置分区">
          {items.map((item) => (
            <button
              key={item.id}
              className={activeTab === item.id ? 'is-active' : undefined}
              type="button"
              title={item.description}
              disabled={item.disabled}
              aria-current={activeTab === item.id ? 'page' : undefined}
              onClick={() => switchTab(item.id)}
            >
              {item.label}
              <span>{busyTabs[item.id] ? '处理中' : dirtyTabs[item.id] ? '未保存' : item.dangerous ? '敏感' : ''}</span>
            </button>
          ))}
        </nav>

        <div style={{ display: 'grid', gap: 16, minWidth: 0 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
            <div>
              <strong style={{ fontSize: 15 }}>{activeTabMeta.label}</strong>
              <p className="a-hint" style={{ marginTop: 2 }}>{activeTabMeta.description}</p>
            </div>
            <div className="a-row-actions">
              {busyTabs[activeTab] ? <Badge tone="primary">处理中</Badge> : null}
              {dirtyTabs[activeTab] ? <Badge tone="warning">有未保存修改</Badge> : <Badge tone="success">已同步</Badge>}
            </div>
          </div>

          {activeTab === 'general' ? <ConfigPage key={`general:${refreshGeneration}`} session={session} onFeedback={onFeedback} onDirtyChange={onGeneralDirtyChange} onBusyChange={onGeneralBusyChange} compact /> : null}
          {activeTab === 'point-conversion' ? (
            <ConfigPage
              key={`point-conversion:${refreshGeneration}`}
              session={session}
              onFeedback={onFeedback}
              onDirtyChange={onPointConversionDirtyChange}
              onBusyChange={onPointConversionBusyChange}
              categories={['billing_pricing']}
              keys={['cny_per_point']}
              compact
            />
          ) : null}
          {activeTab === 'attachment-policy' ? <AttachmentPolicyPage key={`attachment-policy:${refreshGeneration}`} session={session} onFeedback={onFeedback} onDirtyChange={onAttachmentPolicyDirtyChange} onBusyChange={onAttachmentPolicyBusyChange} compact /> : null}
          {activeTab === 'media-policy' ? <MediaPolicyPage key={`media-policy:${refreshGeneration}`} compact onDirtyChange={onMediaPolicyDirtyChange} onBusyChange={onMediaPolicyBusyChange} /> : null}
          {activeTab === 'security' && canManageDangerous ? <SecurityConfigPage key={`security:${refreshGeneration}`} onFeedback={onFeedback} onDirtyChange={onSecurityDirtyChange} onBusyChange={onSecurityBusyChange} compact /> : null}
          {activeTab === 'storage' && canManageDangerous ? <StorageConfigPage key={`storage:${refreshGeneration}`} onFeedback={onFeedback} onDirtyChange={onStorageDirtyChange} onBusyChange={onStorageBusyChange} compact /> : null}
          {tabItems.find((item) => item.id === activeTab)?.dangerous && !canManageDangerous ? (
            <EmptyBlock title="暂无敏感配置权限" detail="安全和存储配置需要 manage:dangerous_config 权限，请联系超级管理员处理。" />
          ) : null}
        </div>

        <aside style={{ display: 'grid', gap: 12, position: 'sticky', top: 'calc(var(--a-top-h) + 18px)' }}>
          <div className="a-rail-card">
            <h4>当前分区</h4>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12.5 }}><span className="a-hint">分区</span><b>{activeTabMeta.label}</b></div>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12.5 }}><span className="a-hint">未保存</span><b style={{ color: dirtyTabs[activeTab] ? 'var(--a-warn)' : 'var(--a-success)' }}>{dirtyTabs[activeTab] ? '有修改' : '无'}</b></div>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12.5 }}><span className="a-hint">校验</span><b>保存时执行</b></div>
          </div>
          <div className="a-rail-card">
            <h4>保存规则</h4>
            <ul>
              <li><i style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--a-warn)' }} />离开分区前需要保存或放弃修改</li>
              <li><i style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--a-info)' }} />敏感配置需要 manage:dangerous_config 权限</li>
              <li><i style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--a-success)' }} />保存后写入审计日志</li>
            </ul>
          </div>
          <div className="a-rail-card">
            <h4>存储切换</h4>
            <div className="a-hint">切换默认实例不会迁移历史对象；旧对象仍按原实例读取，新上传才写入新的默认实例。</div>
          </div>
        </aside>
      </div>
    </section>
  )
}