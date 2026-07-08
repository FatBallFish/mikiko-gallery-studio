import { useEffect, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { AdminTabs, EmptyBlock, PageHeader } from '../components'
import { canAdmin } from '../types'
import { adminPage } from '../ui/classes'
import { ConfigPage } from './ConfigPage'
import { SecurityConfigPage } from './SecurityConfigPage'
import { StorageConfigPage } from './StorageConfigPage'

type SystemSettingsTab = 'general' | 'security' | 'storage'

const tabItems = [
  { id: 'general', label: '通用配置', description: '文档、公开内容和低风险运行参数' },
  { id: 'security', label: '安全配置', description: 'SMTP 与敏感安全项' },
  { id: 'storage', label: '存储配置', description: 'Local / S3 / R2 多实例存储' },
] satisfies Array<{ id: SystemSettingsTab; label: string; description: string }>

export function SystemSettingsPage({
  session,
  onFeedback,
}: {
  session: AdminSession
  onFeedback: (title: string, detail?: string) => void
}) {
  const [activeTab, setActiveTab] = useState<SystemSettingsTab>(() => systemSettingsTabFromHash(window.location.hash))
  const canManageDangerous = canAdmin(session, 'manage:dangerous_config')
  const items = tabItems.map((item) => ({
    ...item,
    disabled: item.id !== 'general' && !canManageDangerous,
  }))

  useEffect(() => {
    const onHashChange = () => setActiveTab(systemSettingsTabFromHash(window.location.hash))
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  const switchTab = (tab: SystemSettingsTab) => {
    setActiveTab(tab)
    window.location.hash = `/system-settings?tab=${tab}`
  }

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="系统设置"
        description="通用配置、安全配置和存储配置聚合在一个页面内，通过横向子 Tab 切换。"
      />
      <AdminTabs
        ariaLabel="系统设置分区"
        orientation="horizontal"
        items={items}
        value={activeTab}
        onChange={switchTab}
      />
      {activeTab === 'general' ? <ConfigPage session={session} onFeedback={onFeedback} compact /> : null}
      {activeTab === 'security' && canManageDangerous ? <SecurityConfigPage onFeedback={onFeedback} compact /> : null}
      {activeTab === 'storage' && canManageDangerous ? <StorageConfigPage onFeedback={onFeedback} compact /> : null}
      {activeTab !== 'general' && !canManageDangerous ? (
        <EmptyBlock title="暂无敏感配置权限" detail="安全配置和存储配置需要 manage:dangerous_config 权限，请联系超级管理员处理。" />
      ) : null}
    </section>
  )
}

export function systemSettingsTabFromHash(hash: string): SystemSettingsTab {
  const path = hash.replace(/^#\/?/, '').split('?')[0].replace(/^\/+|\/+$/g, '')
  if (path === 'security-settings' || path === 'security-config') return 'security'
  if (path === 'storage-settings' || path === 'storage-config') return 'storage'

  const query = hash.includes('?') ? hash.slice(hash.indexOf('?') + 1) : ''
  const tab = new URLSearchParams(query).get('tab')
  if (tab === 'security' || tab === 'storage') return tab
  return 'general'
}
