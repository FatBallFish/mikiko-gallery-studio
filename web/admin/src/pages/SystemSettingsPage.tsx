import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, EmptyBlock, PageHeader, RefreshIconButton } from '../components'
import { canAdmin } from '../types'
import { adminPage } from '../ui/classes'
import { ConfigPage } from './ConfigPage'
import { AttachmentPolicyPage } from './AttachmentPolicyPage'
import { SecurityConfigPage } from './SecurityConfigPage'
import { StorageConfigPage } from './StorageConfigPage'
import { MediaPolicyPage } from './MediaPolicyPage'
import {
  isSystemSettingsHash,
  scrollToSystemSettingsSection,
  systemSettingsSections,
  systemSettingsTabFromHash,
  type SystemSettingsSection,
  type SystemSettingsTab,
} from './systemSettingsTabs'
import './system-settings.css'

export { isSystemSettingsHash, systemSettingsTabFromHash } from './systemSettingsTabs'

const tabItems = [
  { id: 'general', label: '通用配置', description: '公开内容和低风险运行参数', dangerous: false },
  { id: 'point-conversion', label: '积分换算', description: '每积分对应的人民币金额', dangerous: false },
  { id: 'attachment-policy', label: '附件策略', description: '文件体积与格式限制', dangerous: false },
  { id: 'media-policy', label: '媒体策略', description: '媒体派生、预览与保留策略', dangerous: false },
  { id: 'security', label: '安全配置', description: 'SMTP 与敏感安全项', dangerous: true },
  { id: 'storage', label: '存储配置', description: 'Local / S3 / R2 多实例存储', dangerous: true },
] satisfies Array<{ id: SystemSettingsTab; label: string; description: string; dangerous: boolean }>

type SectionMap = Record<SystemSettingsTab, string>

const initialSectionMap: SectionMap = {
  general: '',
  'point-conversion': systemSettingsSections['point-conversion'][0]?.id ?? '',
  'attachment-policy': systemSettingsSections['attachment-policy'][0]?.id ?? '',
  'media-policy': systemSettingsSections['media-policy'][0]?.id ?? '',
  security: systemSettingsSections.security[0]?.id ?? '',
  storage: systemSettingsSections.storage[0]?.id ?? '',
  'text-models': '',
}

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
  const [sectionByTab, setSectionByTab] = useState<SectionMap>(initialSectionMap)
  const [generalSections, setGeneralSections] = useState<SystemSettingsSection[]>([])
  const [generalSectionRequest, setGeneralSectionRequest] = useState<{ id: string; nonce: number } | null>(null)
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

  const sections = useMemo(() => {
    if (activeTab === 'general') return generalSections
    if ((activeTab === 'security' || activeTab === 'storage') && !canManageDangerous) return []
    return systemSettingsSections[activeTab]
  }, [activeTab, canManageDangerous, generalSections])
  const activeSectionId = sectionByTab[activeTab]
  const activeSectionMeta = sections.find((section) => section.id === activeSectionId) ?? sections[0]
  const showSubnav = sections.length > 1
  const activeEditable = activeTab === 'general'
    || activeTab === 'point-conversion'
    || activeTab === 'attachment-policy'
    || activeTab === 'media-policy'
    || (activeTab === 'security' && canManageDangerous)

  useEffect(() => { activeTabRef.current = activeTab }, [activeTab])
  useEffect(() => { dirtyTabsRef.current = dirtyTabs }, [dirtyTabs])
  useEffect(() => { busyTabsRef.current = busyTabs }, [busyTabs])
  useEffect(() => {
    if (activeTab === 'text-models') window.location.hash = '/access-accounts?media=text'
  }, [activeTab])

  useEffect(() => {
    if (!sections.length) return
    if (sections.some((section) => section.id === sectionByTab[activeTab])) return
    setSectionByTab((current) => ({ ...current, [activeTab]: sections[0].id }))
  }, [activeTab, sectionByTab, sections])

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
    setGeneralSectionRequest(null)
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

  const onGeneralSectionsChange = useCallback((nextSections: SystemSettingsSection[]) => {
    setGeneralSections(nextSections)
    setSectionByTab((current) => {
      if (current.general && nextSections.some((section) => section.id === current.general)) return current
      const first = nextSections[0]?.id ?? ''
      return first ? { ...current, general: first } : current
    })
  }, [])

  const onGeneralActiveSectionChange = useCallback((section: string) => {
    setSectionByTab((current) => current.general === section ? current : { ...current, general: section })
  }, [])

  const selectSection = (section: string) => {
    if (busyTabs[activeTab]) return
    if (activeTab === 'general') {
      // ConfigPage owns the unsaved-change confirmation for its categories.
      setGeneralSectionRequest((current) => ({ id: section, nonce: (current?.nonce ?? 0) + 1 }))
      return
    }
    setSectionByTab((current) => ({ ...current, [activeTab]: section }))
    scrollToSystemSettingsSection(activeTab, section)
  }

  const tabNavItems = items.map((item) => ({
    id: item.id,
    label: item.label,
    disabled: item.disabled,
    badge: busyTabs[item.id] ? '处理中' : dirtyTabs[item.id] ? '未保存' : undefined,
  }))

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="System Settings"
        title="系统设置"
        description="顶部横向切换配置分区；包含多个区块的分区使用左侧竖向导航定位，右侧固定展示当前分区的保存状态与规则。"
        meta={<span className={dirtyCount ? 'a-badge a-badge--warn' : 'a-badge a-badge--ok'}><i />{dirtyCount ? `${dirtyCount} 个分区有未保存修改` : '所有分区已同步'}</span>}
        secondaryActions={<RefreshIconButton label="刷新当前设置分区" disabled={dirtyTabs[activeTab] || busyTabs[activeTab]} onClick={() => setRefreshGeneration((value) => value + 1)} />}
      />

      <AdminTabs<SystemSettingsTab>
        ariaLabel="系统设置分区"
        orientation="horizontal"
        items={tabNavItems}
        value={activeTab}
        onChange={switchTab}
      />

      <div className={cn('ss-workspace', showSubnav && 'ss-workspace--with-subnav')}>
        {showSubnav ? (
          <nav className="ss-subnav" aria-label={`${activeTabMeta.label}区块`}>
            {sections.map((section) => (
              <button
                key={section.id}
                className={cn('ss-subnav__item', activeSectionMeta?.id === section.id && 'is-active')}
                type="button"
                disabled={busyTabs[activeTab] || section.disabled}
                aria-current={activeSectionMeta?.id === section.id ? 'true' : undefined}
                title={section.description}
                onClick={() => selectSection(section.id)}
              >
                <span>{section.label}</span>
                {typeof section.count === 'number' ? <em>{section.count}</em> : null}
              </button>
            ))}
          </nav>
        ) : null}

        <div className="ss-content">
          <div className="ss-content-head">
            <div>
              <strong>{activeTabMeta.label}</strong>
              <p className="a-hint">{activeSectionMeta ? `${activeSectionMeta.label} · ${activeTabMeta.description}` : activeTabMeta.description}</p>
            </div>
            <div className="a-row-actions">
              {busyTabs[activeTab] ? <Badge tone="primary">处理中</Badge> : null}
              {dirtyTabs[activeTab] ? <Badge tone="warning">有未保存修改</Badge> : <Badge tone="success">已同步</Badge>}
            </div>
          </div>

          {activeTab === 'general' ? (
            <ConfigPage
              key={`general:${refreshGeneration}`}
              session={session}
              onFeedback={onFeedback}
              onDirtyChange={onGeneralDirtyChange}
              onBusyChange={onGeneralBusyChange}
              sectionRequest={generalSectionRequest}
              onSectionsChange={onGeneralSectionsChange}
              onActiveSectionChange={onGeneralActiveSectionChange}
              compact
            />
          ) : null}
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
          {activeTab === 'attachment-policy' ? <AttachmentPolicyPage key={`attachment-policy:${refreshGeneration}`} session={session} onFeedback={onFeedback} onDirtyChange={onAttachmentPolicyDirtyChange} onBusyChange={onAttachmentPolicyBusyChange} activeSection={activeSectionMeta?.id} compact /> : null}
          {activeTab === 'media-policy' ? <MediaPolicyPage key={`media-policy:${refreshGeneration}`} compact activeSection={activeSectionMeta?.id} onDirtyChange={onMediaPolicyDirtyChange} onBusyChange={onMediaPolicyBusyChange} /> : null}
          {activeTab === 'security' && canManageDangerous ? <SecurityConfigPage key={`security:${refreshGeneration}`} onFeedback={onFeedback} onDirtyChange={onSecurityDirtyChange} onBusyChange={onSecurityBusyChange} activeSection={activeSectionMeta?.id} compact /> : null}
          {activeTab === 'storage' && canManageDangerous ? <StorageConfigPage key={`storage:${refreshGeneration}`} onFeedback={onFeedback} onDirtyChange={onStorageDirtyChange} onBusyChange={onStorageBusyChange} compact /> : null}
          {tabItems.find((item) => item.id === activeTab)?.dangerous && !canManageDangerous ? (
            <EmptyBlock title="暂无敏感配置权限" detail="安全和存储配置需要 manage:dangerous_config 权限，请联系超级管理员处理。" />
          ) : null}
        </div>

        <aside className="ss-rail" aria-label="系统设置侧栏">
          <div className="a-rail-card">
            <h4>当前分区</h4>
            <div className="ss-rail-row"><span className="a-hint">分区</span><b>{activeTabMeta.label}</b></div>
            {activeSectionMeta ? <div className="ss-rail-row"><span className="a-hint">区块</span><b>{activeSectionMeta.label}</b></div> : null}
            <div className="ss-rail-row"><span className="a-hint">未保存</span><b className={dirtyTabs[activeTab] ? 'is-warn' : 'is-ok'}>{dirtyTabs[activeTab] ? '有修改' : '无'}</b></div>
            <div className="ss-rail-row"><span className="a-hint">校验</span><b>{busyTabs[activeTab] ? '处理中' : dirtyTabs[activeTab] ? '待保存' : '已同步'}</b></div>
          </div>

          {activeEditable ? (
            <div className="a-rail-card">
              <h4>保存规则</h4>
              <ul>
                <li><i className="ss-dot ss-dot--warn" />离开分区前需要保存或放弃修改</li>
                {activeTab === 'security'
                  ? <li><i className="ss-dot ss-dot--info" />敏感配置需要 manage:dangerous_config 权限</li>
                  : <li><i className="ss-dot ss-dot--info" />保存后配置解析缓存会失效</li>}
                <li><i className="ss-dot ss-dot--success" />保存后写入审计日志</li>
              </ul>
            </div>
          ) : null}

          {activeTab === 'storage' ? (
            <>
              <div className="a-rail-card">
                <h4>存储切换</h4>
                <div className="a-hint">切换默认实例不会迁移历史对象；旧对象仍按原实例读取，新上传才写入新的默认实例。</div>
              </div>
              <div className="a-rail-card">
                <h4>凭据</h4>
                <div className="a-hint">Access Key 与 Secret 只写不读；保存前先在抽屉或列表操作列探测连接。</div>
              </div>
            </>
          ) : null}

          {sectionRailHint(activeTab, activeSectionMeta?.id) ? (
            <div className="a-rail-card">
              <h4>区块提示</h4>
              <div className="a-hint">{sectionRailHint(activeTab, activeSectionMeta?.id)}</div>
            </div>
          ) : null}
        </aside>
      </div>
    </section>
  )
}

function sectionRailHint(tab: SystemSettingsTab, section?: string) {
  if (tab === 'general') {
    if (section === 'features') return '功能入口关闭后，历史资产仍会保留，只停止新建入口。'
    if (section === 'public_gallery') return '公开内容开关同时影响广场展示与用户公开申请。'
    if (section === 'site') return '站点基础信息会用于页面标题、邮件模板与 Open API 文档。'
    return ''
  }
  if (tab === 'point-conversion') return '汇率变更只影响新任务报价，历史订单按创建时快照结算。'
  if (tab === 'attachment-policy') {
    if (section === 'image') return '图片策略当前生效，用户端与 API 共用同一套校验；SVG 不允许上传。'
    if (section) return '视频、音频与文档为预留配置，运行时接入前不会开放对应上传入口。'
    return ''
  }
  if (tab === 'media-policy') {
    if (section === 'formats-quota') return '格式与配额只影响新上传对象，不改变历史对象。'
    if (section === 'derivatives') return '派生资源按需生成，原图与原视频始终保留。'
    if (section === 'retention') return '保留与清理策略只作用于后续清理任务。'
    return ''
  }
  if (tab === 'security') {
    if (section === 'smtp') return 'SMTP 密码只写不读，保存后不会明文回显。'
    if (section === 'oidc') return 'OIDC 回调地址必须与身份提供方登记值完全一致。'
    return ''
  }
  return ''
}