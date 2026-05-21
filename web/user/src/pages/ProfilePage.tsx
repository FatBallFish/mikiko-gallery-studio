import { FormEvent, useEffect, useState } from 'react'
import type { Balance, GenerationPreferences, LedgerEntry, UserProfile } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Button, EmptyState, ErrorState, Field, LoadingState, useApp } from '../components'
import { errorMessage } from '../useMockResource'

const layout = {
  content: { padding: 40, maxWidth: 960, marginInline: 'auto', width: '100%' } as const,
  header: { marginBottom: 48 } as const,
  title: { fontSize: 56, margin: 0 } as const,
  grid: { display: 'grid', gridTemplateColumns: '1fr 1.5fr', gap: 40 } as const,
  card: { background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', padding: 32, height: 'fit-content' } as const,
}

export function ProfilePage() {
  const app = useApp()
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [balance, setBalance] = useState<Balance | null>(null)
  const [ledger, setLedger] = useState<LedgerEntry[]>([])
  const [redeem, setRedeem] = useState('WELCOME-2026')
  const [showRedeemInput, setShowRedeemInput] = useState(false)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const [nextProfile, nextBalance, nextLedger] = await Promise.all([mockApi.getProfile(), mockApi.getBalance(), mockApi.getLedger()])
      setProfile(nextProfile)
      setBalance(nextBalance)
      setLedger(nextLedger)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  async function redeemCode(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await mockApi.redeemCode(redeem)
      await Promise.all([load(), app.refreshAccount()])
      app.notify('success', '兑换成功，余额已更新')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function saveProfile(patch: Partial<UserProfile>) {
    setBusy(true)
    setError(null)
    try {
      const next = await mockApi.updateProfile(patch)
      setProfile(next)
      await app.refreshAccount()
      app.notify('success', '账户偏好已保存')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <LoadingState />
  if (error && !profile) return <ErrorState message={error} onRetry={load} />

  return (
    <div className="content" style={layout.content}>
      <div className="header" style={layout.header}>
        <p className="eyebrow">ACCOUNT & CREDITS</p>
        <h1 style={layout.title}>个人中心</h1>
      </div>

      <div className="grid" style={layout.grid}>
        <div className="stack" style={{ display: 'grid', gap: 32 }}>
          <div className="card" style={layout.card}>
            <div className="card-title" style={cardTitleStyle}>◈ 我的积分</div>
            <div className="balance-display" style={{ marginBottom: 32 }}>
              <div className="balance-num num" style={{ fontFamily: 'Cormorant Garamond, serif', fontSize: 64, color: 'var(--vault-gold)', lineHeight: 1, marginBottom: 8 }}>{balance?.available_points ?? app.balance?.available_points ?? '0.00000'}</div>
              <div className="balance-label" style={{ fontSize: 14, color: 'var(--vault-muted)' }}>可用积分余额 (◈) / 冻结 {balance?.frozen_points ?? app.balance?.frozen_points ?? '0.00000'}</div>
            </div>
            <div className="stack" style={{ display: 'grid', gap: 12 }}>
              <button className="btn btn-primary" type="button" onClick={() => app.navigate('api-keys')}>充值积分</button>
              <button className="btn" type="button" onClick={() => setShowRedeemInput((v) => !v)}>{showRedeemInput ? '取消' : '使用兑换码'}</button>
              {showRedeemInput ? (
                <form onSubmit={redeemCode} style={{ display: 'grid', gap: 12 }}>
                  <input value={redeem} onChange={(event) => setRedeem(event.target.value)} placeholder="输入兑换码" />
                  <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? '处理中...' : '确认兑换'}</button>
                </form>
              ) : null}
            </div>
            {error ? <div className="form-error" style={{ marginTop: 14 }}>{error}</div> : null}
            <div style={{ marginTop: 32, paddingTop: 24, borderTop: '1px solid var(--vault-line)' }}>
              <div style={planRowStyle}><span style={{ color: 'var(--vault-muted)' }}>当前套餐</span><span style={{ fontWeight: 700 }}>{balance?.plan_name ?? profile?.tier ?? '免费计划'}</span></div>
              <div style={planRowStyle}><span style={{ color: 'var(--vault-muted)' }}>用户分组</span><span className="num">{profile?.group ?? 'DEFAULT (1.0x)'}</span></div>
            </div>
          </div>

          {profile ? <ProfileEditor profile={profile} busy={busy} onSave={saveProfile} /> : null}
        </div>

        <div className="card" style={layout.card}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, marginBottom: 12 }}>
            <div className="card-title" style={{ ...cardTitleStyle, marginBottom: 0 }}>积分流水 (最近记录)</div>
            <Button tone="ghost" onClick={load}>刷新</Button>
          </div>
          {!ledger.length ? <EmptyState title="暂无流水" detail="生成或兑换后会在这里记录。" /> : null}
          <div className="list" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {ledger.map((entry) => (
              <div key={entry.id} className="list-item" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16, paddingBlock: 12, borderBottom: '1px solid var(--vault-line)' }}>
                <div className="list-item-main" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <div className="list-item-title" style={{ fontSize: 15, fontWeight: 700 }}>{entry.title}</div>
                  <div className="list-item-meta" style={{ fontSize: 12, color: 'var(--vault-muted)', fontFamily: 'JetBrains Mono, monospace' }}>{entry.occurred_at} · {entry.detail}</div>
                </div>
                <div className={`list-item-value ${entry.type === 'credit' ? 'plus' : ''}`} style={{ fontFamily: 'JetBrains Mono, monospace', fontWeight: 700, color: entry.type === 'credit' ? 'var(--vault-gold)' : 'var(--vault-fg)' }}>{entry.amount}</div>
              </div>
            ))}
          </div>
          <div style={{ marginTop: 32, textAlign: 'center' }}>
            <button className="btn btn-ghost" type="button" style={{ color: 'var(--vault-gold)' }} onClick={load}>查看全部流水记录</button>
          </div>
        </div>
      </div>
    </div>
  )
}

const cardTitleStyle = { fontSize: 14, fontWeight: 800, color: 'var(--vault-muted)', textTransform: 'uppercase' as const, letterSpacing: '0.1em', marginBottom: 24, display: 'flex', alignItems: 'center', gap: 8 }
const planRowStyle = { display: 'flex', justifyContent: 'space-between', fontSize: 14, marginBottom: 12, gap: 12 }

function ProfileEditor({ profile, busy, onSave }: { profile: UserProfile; busy: boolean; onSave: (patch: Partial<UserProfile>) => Promise<void> }) {
  const [name, setName] = useState(profile.display_name)
  const [signature, setSignature] = useState(profile.signature)
  const [preferences, setPreferences] = useState<GenerationPreferences>(profile.preferences)

  return (
    <div className="card" style={layout.card}>
      <div className="card-title" style={cardTitleStyle}>基本信息</div>
      <div className="profile-header" style={{ display: 'flex', alignItems: 'center', gap: 24, marginBottom: 32 }}>
        <div className="avatar" style={{ width: 80, height: 80, borderRadius: '50%', background: 'var(--vault-gold)', color: 'var(--vault-bg)', display: 'grid', placeItems: 'center', fontSize: 32, fontWeight: 800, fontFamily: 'Cormorant Garamond, serif' }}>{profile.avatar_initials}</div>
        <div>
          <div style={{ fontSize: 20, fontWeight: 700 }}>{profile.display_name}</div>
          <div style={{ fontSize: 14, color: 'var(--vault-muted)' }}>{profile.email}</div>
        </div>
      </div>
      <Field label="显示昵称"><input className="input" value={name} onChange={(event) => setName(event.target.value)} /></Field>
      <Field label="签名"><textarea className="input" value={signature} onChange={(event) => setSignature(event.target.value)} rows={3} /></Field>
      <div className="pref-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 12 }}>
        <Field label="默认模型"><select value={preferences.model_group} onChange={(event) => setPreferences({ ...preferences, model_group: event.target.value })}><option value="basic-image">Basic Image</option><option value="plus-image">Plus Image</option><option value="pro-image">Pro Studio</option></select></Field>
        <Field label="默认比例"><select value={preferences.aspect_ratio} onChange={(event) => setPreferences({ ...preferences, aspect_ratio: event.target.value })}><option>1:1</option><option>16:9</option><option>9:16</option><option>4:3</option></select></Field>
        <Field label="默认质量"><select value={preferences.quality} onChange={(event) => setPreferences({ ...preferences, quality: event.target.value })}><option>auto</option><option>1K</option><option>2K</option><option>4K</option></select></Field>
      </div>
      <button className="btn" type="button" disabled={busy} onClick={() => void onSave({ display_name: name, signature, preferences })}>{busy ? '保存中...' : '保存修改'}</button>
    </div>
  )
}
