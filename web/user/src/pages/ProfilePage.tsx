import { type FormEvent, useEffect, useId, useState } from 'react'
import type { Balance, LedgerEntry, UserProfile } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { formatPoints } from '../../../shared/pointsDisplay'
import { userApi } from '../../../shared/user-api'
import { EmptyState, ErrorState, Modal, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { balanceBucketLabel, bucketExpiryText, nextExpiringCreditText, normalizeBalanceBuckets, profileLedgerRows } from './profileBalanceModel'
import { RedeemCodeForm } from './RedeemCodeForm'

export function ProfilePage() {
  const app = useApp()
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [balance, setBalance] = useState<Balance | null>(null)
  const [ledger, setLedger] = useState<LedgerEntry[]>([])
  const [showRedeemInput, setShowRedeemInput] = useState(false)
  const [showPasswordDialog, setShowPasswordDialog] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    setLoading(true)
    setLoadError('')
    try {
      const [nextProfile, nextBalance, nextLedger] = await Promise.all([userApi.getProfile(), userApi.getBalance(), userApi.getLedger()])
      setProfile(nextProfile)
      setBalance(nextBalance)
      setLedger(nextLedger)
    } catch (err) {
      const message = errorMessage(err)
      setLoadError(message)
      app.notify('error', message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  async function saveProfile(patch: Partial<UserProfile>) {
    setBusy(true)
    try {
      const next = await userApi.updateProfile(patch)
      setProfile(next)
      await app.refreshAccount()
      app.notify('success', '个人资料已保存')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <ProfileLoadingSkeleton />
  if (loadError && !profile) return <div className="acct-page"><ErrorState message={loadError} onRetry={() => void load()} /></div>

  const available = formatPoints(balance?.available_points ?? app.balance?.available_points ?? '0')
  const frozen = formatPoints(balance?.frozen_points ?? app.balance?.frozen_points ?? '0')
  const trial = formatPoints(balance?.trial_points ?? '0')
  const multiplier = balance?.user_group_multiplier ?? '1.0'
  const ledgerRows = profileLedgerRows(ledger)
  const rawLedger = new Map(ledger.map((entry) => [String(entry.id), entry]))
  const nextExpiry = nextExpiringCreditText(balance)

  return (
    <div className="acct-page">
      <div className="u-settings u-settings--single">
        <div className="u-settings__stack">
          <div className="u-page-head" style={{ margin: 0 }}>
            <div className="u-page-head__copy">
              <span className="u-eyebrow">Account</span>
              <h1 className="u-h1" style={{ fontSize: 'clamp(32px, 3.4vw, 38px)' }}>个人资料</h1>
              <p className="u-sub">账户信息、积分与流水。积分流水按余额桶拆分，到期时间与任务来源都可追溯。</p>
            </div>
          </div>

          <div className="u-split">
            <section className="pf-balance" aria-labelledby="profile-balance-title">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
                <div style={{ display: 'grid', gap: 4, minWidth: 0 }}>
                  <span className="u-eyebrow" id="profile-balance-title">Available Points</span>
                  <div className="pf-balance__value acct-balance-value">{available.text}</div>
                  <div className="acct-tier-row" aria-label="积分等级拆分">
                    {available.parts.map((part) => (
                      <span key={part.tier.key} className="u-chip u-chip--mono">{part.amount} {part.tier.short}</span>
                    ))}
                  </div>
                  <span className="u-meta">冻结 {frozen.text} · 体验额度 {trial.text}</span>
                </div>
                <span className="u-chip u-chip--accent">{profile?.group ?? 'DEFAULT'} 分组 ×{multiplier}</span>
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => app.navigate('checkout')}>充值</button>
                <button className="u-btn u-btn--sm" type="button" onClick={() => setShowRedeemInput((value) => !value)}>{showRedeemInput ? '收起兑换码' : '兑换码'}</button>
              </div>
              {showRedeemInput ? (
                <div className="acct-redeem">
                  <RedeemCodeForm onRedeemed={() => Promise.all([load(), app.refreshAccount()]).then(() => undefined)} />
                </div>
              ) : null}
              <BalanceBuckets balance={balance} />
            </section>

            <div className="u-plate u-plate--pad">
              {profile ? <ProfileEditor profile={profile} busy={busy} onSave={saveProfile} onChangePassword={() => setShowPasswordDialog(true)} /> : <ProfileEditorSkeleton />}
            </div>
          </div>

          <div className="u-stat-row" aria-label="积分概览">
            <div className="u-stat">
              <span className="u-stat__label">可用积分</span>
              <span className="u-stat__value acct-stat-value">{available.text}</span>
              <span className="u-stat__foot">{available.fullText}</span>
            </div>
            <div className="u-stat">
              <span className="u-stat__label">冻结积分</span>
              <span className="u-stat__value acct-stat-value">{frozen.text}</span>
              <span className="u-stat__foot">进行中任务预冻结</span>
            </div>
            <div className="u-stat">
              <span className="u-stat__label">体验额度</span>
              <span className="u-stat__value acct-stat-value">{trial.text}</span>
              <span className="u-stat__foot">{trial.fullText}</span>
            </div>
            <div className="u-stat">
              <span className="u-stat__label">当前套餐</span>
              <span className="u-stat__value acct-stat-value acct-stat-value--text">{balance?.plan_name ?? profile?.tier ?? '免费计划'}</span>
              <span className="u-stat__foot">{profile?.email}</span>
            </div>
          </div>

          <div className="u-plate u-plate--pad">
            <div className="u-panel-title">
              <h3>积分流水</h3>
              <span>最近 {ledgerRows.length} 条</span>
            </div>
            {nextExpiry ? <div className="acct-next-expiry">{nextExpiry}</div> : null}
            {!ledgerRows.length ? <EmptyState title="暂无流水" detail="生成或兑换后会在这里记录。" /> : null}
            {ledgerRows.length ? (
              <div className="u-list acct-ledger">
                {ledgerRows.map((entry) => {
                  const raw = rawLedger.get(String(entry.id))
                  const amount = formatPoints(entry.amount.replace(/^\+/, ''))
                  const afterValue = raw?.bucket_balance_after ?? raw?.balance_after
                  const after = afterValue ? formatPoints(afterValue) : null
                  return (
                    <div className="u-row" key={entry.id}>
                      <div className="u-row__main acct-ledger__main">
                        <div className="u-row__copy acct-ledger__copy">
                          <strong>{entry.title}</strong>
                          <span>{entry.bucketLabel} · {entry.sourceLabel} · {entry.occurredAt} · {entry.expiryText}</span>
                          <span>{entry.detail}</span>
                          {entry.generationDetail ? <span>{entry.generationDetail}</span> : null}
                          {entry.taskId ? <span>任务 {entry.taskId}</span> : null}
                        </div>
                      </div>
                      <div className="acct-ledger__amounts">
                        <span className={cn('pf-ledger__amount', entry.amountTone === 'credit' && 'is-plus')} title={amount.fullText}>{amount.text}</span>
                        {after ? <span className="u-meta" title={after.fullText}>余额 {after.text}</span> : null}
                      </div>
                    </div>
                  )
                })}
              </div>
            ) : null}
            <div style={{ display: 'flex', justifyContent: 'center', marginTop: 14 }}>
              <button className="u-btn u-btn--sm" type="button" onClick={() => void load()}>刷新流水</button>
            </div>
          </div>
        </div>
      </div>
      {profile && showPasswordDialog ? (
        <PasswordChangeModal profile={profile} onClose={() => setShowPasswordDialog(false)} />
      ) : null}
    </div>
  )
}

function ProfileLoadingSkeleton() {
  return (
    <div className="acct-page" aria-hidden="true">
      <div className="u-settings u-settings--single">
        <div className="u-settings__stack">
          <div style={{ display: 'grid', gap: 12 }}>
            <div className="pg-skeleton h-4 w-32 rounded-xl" />
            <div className="pg-skeleton h-12 w-64 rounded-xl" />
            <div className="pg-skeleton h-4 w-full max-w-xl rounded-xl" />
          </div>
          <div className="u-split">
            <div className="pf-balance" style={{ gap: 18 }}>
              <div className="pg-skeleton h-8 w-48 rounded-xl" />
              <div className="pg-skeleton h-4 w-64 rounded-xl" />
              <div className="pg-skeleton h-24 rounded-xl" />
            </div>
            <div className="u-plate u-plate--pad" style={{ display: 'grid', gap: 14 }}>
              <div className="pg-skeleton h-16 w-16 rounded-2xl" />
              <div className="pg-skeleton h-11 rounded-xl" />
              <div className="pg-skeleton h-20 rounded-xl" />
            </div>
          </div>
          <div className="u-plate u-plate--pad" style={{ display: 'grid', gap: 14 }}>
            {Array.from({ length: 5 }).map((_, index) => (
              <div key={index} style={{ display: 'grid', gap: 8, paddingBottom: 12, borderBottom: '1px solid var(--line, var(--border))' }}>
                <div className="pg-skeleton h-4 w-2/3 rounded-xl" />
                <div className="pg-skeleton h-3 w-1/2 rounded-xl" />
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function BalanceBuckets({ balance }: { balance: Balance | null }) {
  const buckets = normalizeBalanceBuckets(balance)
  if (!buckets.length) return null
  return (
    <div className="pf-buckets">
      {buckets.map((bucket) => {
        const points = formatPoints(bucket.available_points)
        return (
          <div key={`${bucket.bucket}-${bucket.expires_at ?? 'never'}`} className="pf-bucket">
            <b>{bucket.label ?? balanceBucketLabel(bucket.bucket)}</b>
            <span>{bucketExpiryText(bucket)}</span>
            <b title={points.fullText}>{points.text}</b>
          </div>
        )
      })}
    </div>
  )
}

function ProfileEditor({ profile, busy, onSave, onChangePassword }: { profile: UserProfile; busy: boolean; onSave: (patch: Partial<UserProfile>) => Promise<void>; onChangePassword: () => void }) {
  const [name, setName] = useState(profile.display_name)
  const [signature, setSignature] = useState(profile.signature)

  return (
    <>
      <div className="u-panel-title">
        <h3>资料</h3>
        <span>可编辑</span>
      </div>
      <div className="pf-profile">
        <div className="pf-avatar" aria-hidden="true">{profile.avatar_initials}</div>
        <div style={{ display: 'grid', gap: 3, minWidth: 0 }}>
          <strong style={{ fontSize: 15 }}>{profile.display_name}</strong>
          <span className="u-meta acct-ellipsis">{profile.email}</span>
          <span className="u-chip u-chip--mono" style={{ alignSelf: 'start', marginTop: 2 }}>UID {profile.id}</span>
        </div>
      </div>
      <div style={{ display: 'grid', gap: 13, marginTop: 16 }}>
        <label className="u-field">
          <span className="u-label">显示昵称</span>
          <input className="u-input" value={name} maxLength={48} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="u-field">
          <span className="u-label">签名</span>
          <textarea className="u-textarea acct-profile-signature" value={signature} maxLength={280} onChange={(event) => setSignature(event.target.value)} />
        </label>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button className="u-btn u-btn--primary u-btn--sm" type="button" disabled={busy} onClick={() => void onSave({ display_name: name.trim(), signature: signature.trim() })}>
            {busy ? '保存中...' : '保存资料'}
          </button>
          <button className="u-btn u-btn--sm" type="button" onClick={onChangePassword}>{profile.has_password ? '修改密码' : '设置密码'}</button>
        </div>
      </div>
    </>
  )
}

function ProfileEditorSkeleton() {
  return (
    <div style={{ display: 'grid', gap: 14 }} aria-hidden="true">
      <div className="pg-skeleton h-16 w-16 rounded-2xl" />
      <div className="pg-skeleton h-11 rounded-xl" />
      <div className="pg-skeleton h-24 rounded-xl" />
      <div className="pg-skeleton h-9 w-32 rounded-xl" />
    </div>
  )
}

function PasswordChangeModal({ profile, onClose }: { profile: UserProfile; onClose: () => void }) {
  const app = useApp()
  const formId = useId()
  const [code, setCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [cooldown, setCooldown] = useState(0)
  const [sending, setSending] = useState(false)
  const [busy, setBusy] = useState(false)
  const [formError, setFormError] = useState('')

  useEffect(() => {
    if (cooldown <= 0) return undefined
    const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])

  async function sendPasswordChangeCode() {
    setSending(true)
    setFormError('')
    try {
      await userApi.sendEmailCode(profile.email, 'password_change')
      setCooldown(60)
      app.notify('success', '验证码已发送，请查看邮箱')
    } catch (err) {
      const message = errorMessage(err)
      setFormError(message)
      app.notify('error', message)
    } finally {
      setSending(false)
    }
  }

  async function submitPasswordChange(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFormError('')
    if (!/^\d{6}$/.test(code.trim())) {
      setFormError('请输入 6 位邮箱验证码')
      return
    }
    if (newPassword.length < 8) {
      setFormError('新密码至少需要 8 个字符')
      return
    }
    if (newPassword !== confirmPassword) {
      setFormError('两次输入的新密码不一致')
      return
    }

    setBusy(true)
    try {
      await userApi.changePassword(code, newPassword)
      app.notify('success', '密码修改成功，请重新登录')
      onClose()
      await app.logout()
    } catch (err) {
      const message = errorMessage(err)
      setFormError(message)
      app.notify('error', message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="修改密码" onClose={onClose} className="acct-modal w-[min(520px,calc(100vw-2rem))]">
      <p className="u-meta" style={{ marginTop: 0 }}>验证码将发送至 {profile.email}。修改成功后需要重新登录。</p>
      <form style={{ display: 'grid', gap: 13 }} onSubmit={(event) => void submitPasswordChange(event)} noValidate>
        <label className="u-field" htmlFor={`${formId}-code`}>
          <span className="u-label">邮箱验证码</span>
          <div className="acct-code-row">
            <input
              id={`${formId}-code`}
              className="u-input"
              value={code}
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              required
              placeholder="6 位数字"
              onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
            />
            <button className="u-btn u-btn--sm" type="button" disabled={sending || cooldown > 0 || busy} onClick={() => void sendPasswordChangeCode()}>
              {sending ? '发送中...' : cooldown > 0 ? `${cooldown}s 后重发` : '发送验证码'}
            </button>
          </div>
        </label>
        <label className="u-field" htmlFor={`${formId}-new-password`}>
          <span className="u-label">新密码</span>
          <input
            id={`${formId}-new-password`}
            className="u-input"
            type="password"
            value={newPassword}
            autoComplete="new-password"
            minLength={8}
            required
            placeholder="至少 8 位"
            onChange={(event) => setNewPassword(event.target.value)}
          />
        </label>
        <label className="u-field" htmlFor={`${formId}-confirm-password`}>
          <span className="u-label">确认新密码</span>
          <input
            id={`${formId}-confirm-password`}
            className="u-input"
            type="password"
            value={confirmPassword}
            autoComplete="new-password"
            minLength={8}
            required
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
        </label>
        {formError ? <p className="u-field__error" role="alert">{formError}</p> : null}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
          <button className="u-btn" type="button" disabled={busy} onClick={onClose}>取消</button>
          <button className="u-btn u-btn--primary" type="submit" disabled={busy || sending}>{busy ? '修改中...' : '确认修改'}</button>
        </div>
      </form>
    </Modal>
  )
}