import { useEffect, useRef, useState } from 'react'
import { Modal, copyText, useApp } from '../components'
import { Copy, Download } from '../ui/icons'
import { confirmCredentialSave, downloadApiKeyCredentials, type ApiKeyCredentials } from './apiKeyCredentials'
import './api-key-credentials.css'

export function ApiKeyCredentialsModal({ credentials, onConfirm }: { credentials: ApiKeyCredentials; onConfirm: () => void }) {
  const app = useApp()
  const [saved, setSaved] = useState(false)
  const downloads = useRef(new Set<() => void>())
  useEffect(() => () => { downloads.current.forEach((cleanup) => cleanup()); downloads.current.clear() }, [])

  async function copy(value: string, field: 'AK' | 'SK') {
    try {
      await copyText(value)
      app.notify('success', `${field} 已复制`)
    } catch {
      app.notify('error', '复制失败，请手动复制并妥善保存。')
    }
  }

  function download() {
    try {
      downloads.current.add(downloadApiKeyCredentials(credentials))
      app.notify('success', '已发起 CSV 下载，请确认文件已保存。')
    } catch {
      app.notify('error', '下载失败，请复制并妥善保存凭据。')
    }
  }

  return (
    <Modal title="保存 API 密钥" onClose={() => {}} dismissible={false} className="acct-modal ak-save-modal">
      <div className="ak-save">
        <p className="ak-save__warning" role="note">完整 SK 仅展示一次，请立即复制或下载并妥善保存。确认关闭后将无法再次查看；请勿将 SK 分享给他人或放入公开代码。</p>
        <p className="u-meta">AK（ak-…）仅用于识别密钥；SK（sk-…）是实际请求使用的 API Key，通过 Authorization: Bearer 认证。轮换后旧 SK 立即失效。</p>
        {([
          { label: 'AK', hint: '密钥标识', value: credentials.accessKey },
          { label: 'SK', hint: 'API Key · Bearer 凭据', value: credentials.secret },
        ] as const).map((field) => (
          <div className="ak-save__field" key={field.label}>
            <div className="ak-save__label"><strong>{field.label}</strong><span className="u-meta">{field.hint}</span></div>
            <div className="ak-save__value">
              <code aria-label={field.label}>{field.value}</code>
              <button className="u-icon-btn" type="button" aria-label={`复制 ${field.label}`} title={`复制 ${field.label}`} onClick={() => void copy(field.value, field.label)}>
                <Copy size={17} strokeWidth={1.6} aria-hidden="true" />
              </button>
            </div>
          </div>
        ))}
        <div><button className="u-btn u-btn--sm" type="button" onClick={download}><Download size={16} aria-hidden="true" />下载 CSV</button></div>
        <p className="u-meta">CSV 包含完整凭据，请保存在安全位置。复制或下载后，仍需由你确认已妥善保存。</p>
        <label className="ak-save__ack"><input type="checkbox" checked={saved} onChange={(event) => setSaved(event.target.checked)} />我已妥善保存</label>
        <div className="ak-save__actions"><button className="u-btn u-btn--primary" type="button" disabled={!saved} onClick={() => confirmCredentialSave(saved, onConfirm)}>确定</button></div>
      </div>
    </Modal>
  )
}
