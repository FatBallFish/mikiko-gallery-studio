import { useId, useRef, useState, type KeyboardEvent } from 'react'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { sharedApiClient } from '../../../shared/http-client'
import { Drawer, ErrorState, copyText, useApp } from '../components'
import { docsSiteUrl, openDocsEntry } from '../docsUrl'
import { errorMessage, useApiResource } from '../useApiResource'
import { developerAPIExamples, developerClientTemplates, endpointDisabledReason } from './developerEndpoints'
import './developer-connect.css'

type ConnectTab = 'api' | 'mcp'
const tabs: { value: ConnectTab; label: string }[] = [{ value: 'api', label: 'API 接入' }, { value: 'mcp', label: 'MCP 接入' }]

export function DeveloperConnectDrawer({ onClose }: { onClose: () => void }) {
  const [tab, setTab] = useState<ConnectTab>('api')
  const id = useId()
  const buttons = useRef<(HTMLButtonElement | null)[]>([])
  const endpoints = useApiResource(() => sharedApiClient.get<DeveloperEndpoints>('/api/v1/public/developer-endpoints', { auth: false }), [])
  const config = endpoints.data
  const disabledReason = config ? endpointDisabledReason(config) : ''

  function navigateTabs(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next: number
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length
    else if (event.key === 'ArrowLeft') next = (index + tabs.length - 1) % tabs.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = tabs.length - 1
    else return
    event.preventDefault()
    setTab(tabs[next].value)
    buttons.current[next]?.focus()
  }

  return <Drawer open title="如何接入" onClose={onClose} className="developer-connect">
    <div className="developer-connect__content">
      <p className="u-sub">使用 API Key 从服务端调用，或通过浏览器授权连接 MCP 客户端。</p>
      <div className="u-tabs developer-connect__tabs" role="tablist" aria-label="接入方式">
        {tabs.map((item, index) => <button
          key={item.value} ref={node => { buttons.current[index] = node }}
          type="button" role="tab" id={`${id}-tab-${item.value}`} aria-controls={`${id}-panel-${item.value}`}
          aria-selected={tab === item.value} tabIndex={tab === item.value ? 0 : -1}
          className={`u-tab${tab === item.value ? ' is-active' : ''}`}
          onClick={() => setTab(item.value)} onKeyDown={event => navigateTabs(event, index)}
        >{item.label}</button>)}
      </div>
      {endpoints.loading && <p role="status">正在读取接入配置…</p>}
      {endpoints.error && <ErrorState message={endpoints.error} onRetry={() => void endpoints.reload()} />}
      {disabledReason && <div className="developer-connect__notice" role="status"><p>{disabledReason}</p><button type="button" className="u-btn u-btn--sm" disabled={endpoints.loading} onClick={() => void endpoints.reload()}>重新读取</button></div>}
      {tabs.map(item => <section key={item.value} role="tabpanel" id={`${id}-panel-${item.value}`} aria-labelledby={`${id}-tab-${item.value}`} hidden={tab !== item.value} tabIndex={0} className="developer-connect__panel">
        {config && !disabledReason && !endpoints.error && (item.value === 'api' ? <>
          {developerAPIExamples(config).map(example => <ConnectExample key={example.name} title={example.name} code={example.code} detail={example.detail} />)}
          <aside className="developer-connect__notice"><h3 className="u-h3">保管 API Key</h3><p>API Key（sk-…）仅在创建或轮换后展示一次，请保存在服务端环境变量中。为不同集成创建独立密钥，按需设置权限、日额度与有效期。</p></aside>
        </> : <>
          <p className="u-sub">MCP 使用浏览器 OAuth 授权，不在配置中填写 API Key 或手动粘贴访问令牌。</p>
          {developerClientTemplates(config).map(template => <ConnectExample key={template.name} title={template.name} code={template.code} detail={template.instructions} format={template.format} />)}
          <aside className="developer-connect__notice"><h3 className="u-h3">连接后如何使用</h3><ol>
            <li>完成浏览器登录与授权，返回客户端检查 studio 连接状态。</li>
            <li>让 agent 调用 list_image_models 和 list_projects，选择可用模型与项目。</li>
            <li>调用 preflight_image_task 查看报价；确认后使用相同参数和返回的 quote_token 调用 create_image_task，并保存任务 ID。</li>
            <li>使用 get_image_task 查询任务，再以 task_id、image_id 调用 get_image_result_access 获取结果链接。链接过期后重新获取。</li>
          </ol><p>只授予需要的权限；可在设置 → 已连接应用中断开连接。</p></aside>
        </>)}
      </section>)}
      <footer className="developer-connect__footer">
        <button type="button" className="u-link acct-inline-link" onClick={() => tab === 'api' ? openDocsEntry('api-keys') : window.open(new URL('#/guide/mcp', docsSiteUrl()).href, '_blank', 'noopener,noreferrer')}>{tab === 'api' ? '查看完整 API 文档' : '查看 OAuth 与客户端教程'}</button>
      </footer>
    </div>
  </Drawer>
}

function ConnectExample({ title, code, detail, format }: { title: string; code: string; detail: string; format?: string }) {
  const app = useApp()
  const [copying, setCopying] = useState(false)
  async function copy() {
    setCopying(true)
    try { await copyText(code); app.notify('success', '已复制到剪贴板') }
    catch (caught) { app.notify('error', errorMessage(caught)) }
    finally { setCopying(false) }
  }
  return <article className="developer-connect__example">
    <div className="developer-connect__example-head"><h3 className="u-h3">{title}</h3><button type="button" className="u-btn u-btn--sm" aria-label={`复制 ${title}`} disabled={copying} onClick={() => void copy()}>{copying ? '正在复制…' : '复制'}</button></div>
    {format && <p className="u-meta">{format}</p>}
    <p className="u-sub">{detail}</p>
    <pre className="acct-code-block" tabIndex={0} aria-label={`${title}代码`}><code>{code}</code></pre>
  </article>
}
