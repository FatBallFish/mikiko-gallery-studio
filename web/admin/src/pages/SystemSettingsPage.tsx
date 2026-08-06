import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { AdminTabs, EmptyBlock, PageHeader } from '../components'
import { canAdmin } from '../types'
import { adminPage } from '../ui/classes'
import { ConfigPage } from './ConfigPage'
import { AttachmentPolicyPage } from './AttachmentPolicyPage'
import { SecurityConfigPage } from './SecurityConfigPage'
import { StorageConfigPage } from './StorageConfigPage'
import { TextModelsPage } from './TextModelsPage'
import { isSystemSettingsHash, systemSettingsTabFromHash, type SystemSettingsTab } from './systemSettingsTabs'

export { isSystemSettingsHash, systemSettingsTabFromHash } from './systemSettingsTabs'

const tabItems = [
  { id: 'general', label: '通用配置', description: '文档、公开内容和低风险运行参数', dangerous: false },
  { id: 'point-conversion', label: '积分换算', description: '每积分对应的人民币金额', dangerous: false },
  { id: 'attachment-policy', label: '附件策略', description: '文件体积与格式限制', dangerous: false },
  { id: 'security', label: '安全配置', description: 'SMTP 与敏感安全项', dangerous: true },
  { id: 'storage', label: '存储配置', description: 'Local / S3 / R2 多实例存储', dangerous: true },
  { id: 'text-models', label: '文本模型', description: '提示词优化账号、模型与价格', dangerous: true },
] satisfies Array<{ id: SystemSettingsTab; label: string; description: string; dangerous: boolean }>

export function SystemSettingsPage({
  session,
  onFeedback,
}: {
  session: AdminSession
  onFeedback: (title: string, detail?: string) => void
}) {
  const [activeTab, setActiveTab] = useState<SystemSettingsTab>(() => systemSettingsTabFromHash(window.location.hash))
  const [dirtyTabs, setDirtyTabs] = useState<Record<SystemSettingsTab, boolean>>({ general: false, 'point-conversion': false, 'attachment-policy': false, security: false, storage: false, 'text-models': false })
  const [busyTabs, setBusyTabs] = useState<Record<SystemSettingsTab, boolean>>({ general: false, 'point-conversion': false, 'attachment-policy': false, security: false, storage: false, 'text-models': false })
  const activeTabRef = useRef(activeTab)
  const dirtyTabsRef = useRef(dirtyTabs)
  const busyTabsRef = useRef(busyTabs)
  const canManageDangerous = canAdmin(session, 'manage:dangerous_config')
  const items = tabItems.map((item) => ({
    ...item,
    disabled: (item.dangerous && !canManageDangerous) || (busyTabs[activeTab] && item.id !== activeTab),
  }))

  useEffect(() => { activeTabRef.current = activeTab }, [activeTab])
  useEffect(() => { dirtyTabsRef.current = dirtyTabs }, [dirtyTabs])
  useEffect(() => { busyTabsRef.current = busyTabs }, [busyTabs])

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
  const onSecurityDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('security', dirty), [updateDirtyTab])
  const onStorageDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('storage', dirty), [updateDirtyTab])
  const onGeneralBusyChange = useCallback((busy: boolean) => updateBusyTab('general', busy), [updateBusyTab])
  const onPointConversionBusyChange = useCallback((busy: boolean) => updateBusyTab('point-conversion', busy), [updateBusyTab])
  const onAttachmentPolicyBusyChange = useCallback((busy: boolean) => updateBusyTab('attachment-policy', busy), [updateBusyTab])
  const onSecurityBusyChange = useCallback((busy: boolean) => updateBusyTab('security', busy), [updateBusyTab])
  const onStorageBusyChange = useCallback((busy: boolean) => updateBusyTab('storage', busy), [updateBusyTab])
  const onTextModelsDirtyChange = useCallback((dirty: boolean) => updateDirtyTab('text-models', dirty), [updateDirtyTab])
  const onTextModelsBusyChange = useCallback((busy: boolean) => updateBusyTab('text-models', busy), [updateBusyTab])

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="系统设置"
        description="通用、安全、存储和文本模型配置聚合在一个页面内，通过横向子 Tab 切换。"
      />
      <AdminTabs
        ariaLabel="系统设置分区"
        orientation="horizontal"
        items={items}
        value={activeTab}
        onChange={switchTab}
      />
      {activeTab === 'general' ? <ConfigPage session={session} onFeedback={onFeedback} onDirtyChange={onGeneralDirtyChange} onBusyChange={onGeneralBusyChange} compact /> : null}
      {activeTab === 'point-conversion' ? (
        <ConfigPage
          session={session}
          onFeedback={onFeedback}
          onDirtyChange={onPointConversionDirtyChange}
          onBusyChange={onPointConversionBusyChange}
          categories={['billing_pricing']}
          keys={['cny_per_point']}
          compact
        />
      ) : null}
      {activeTab === 'attachment-policy' ? <AttachmentPolicyPage session={session} onFeedback={onFeedback} onDirtyChange={onAttachmentPolicyDirtyChange} onBusyChange={onAttachmentPolicyBusyChange} compact /> : null}
      {activeTab === 'security' && canManageDangerous ? <SecurityConfigPage onFeedback={onFeedback} onDirtyChange={onSecurityDirtyChange} onBusyChange={onSecurityBusyChange} compact /> : null}
      {activeTab === 'storage' && canManageDangerous ? <StorageConfigPage onFeedback={onFeedback} onDirtyChange={onStorageDirtyChange} onBusyChange={onStorageBusyChange} compact /> : null}
      {activeTab === 'text-models' && canManageDangerous ? <TextModelsPage onFeedback={onFeedback} onDirtyChange={onTextModelsDirtyChange} onBusyChange={onTextModelsBusyChange} /> : null}
      {tabItems.find((item) => item.id === activeTab)?.dangerous && !canManageDangerous ? (
        <EmptyBlock title="暂无敏感配置权限" detail="安全、存储和文本模型配置需要 manage:dangerous_config 权限，请联系超级管理员处理。" />
      ) : null}
    </section>
  )
}
