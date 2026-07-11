import { FormEvent, useEffect, useState } from 'react'
import type { Balance, GenerationPreferences, LedgerEntry, UserProfile } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, Field, useApp } from '../components'
import { button as btn, card, form } from '../ui/redesign-classes'
import { SettingsWorkspace } from '../ui/SettingsWorkspace'
import { errorMessage } from '../useApiResource'
import { balanceBucketLabel, bucketExpiryText, normalizeBalanceBuckets, profileLedgerRows } from './profileBalanceModel'

const profileClasses = {
  content: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-12',
  title: 'm-0 text-4xl font-black leading-none md:text-6xl',
  grid: 'grid gap-8 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]',
  stack: 'grid gap-8',
  compactStack: 'grid gap-3',
  card: cn(card.padded, 'h-fit'),
  cardTitle: 'mb-6 flex items-center gap-2 text-sm font-extrabold text-[var(--muted)]',
  balanceDisplay: 'mb-8',
  balanceNum: 'num mb-2 text-[64px] leading-none text-[var(--accent)]',
  balanceLabel: 'text-sm text-[var(--muted)]',
  redeemForm: 'grid gap-3',
  planBlock: 'mt-8 border-t border-[var(--border)] pt-6',
  planRow: 'mb-3 flex justify-between gap-3 text-sm',
  planLabel: 'text-[var(--muted)]',
  planValue: 'font-bold',
  ledgerHeader: 'mb-3 flex items-center justify-between gap-3',
  ledgerTitle: 'flex items-center gap-2 text-sm font-extrabold uppercase tracking-[.1em] text-[var(--muted)]',
  ledgerList: 'flex flex-col gap-4',
  ledgerItem: 'flex items-center justify-between gap-4 border-b border-[var(--border)] py-3',
  ledgerMain: 'flex min-w-0 flex-col gap-2',
  ledgerItemTitle: 'text-[15px] font-bold',
  ledgerTags: 'flex flex-wrap gap-2',
  ledgerTag: 'rounded-xl border border-[var(--border)] px-[7px] py-[3px] font-mono text-[11px] font-extrabold',
  ledgerBucketTag: 'bg-[color-mix(in_oklch,var(--accent)_10%,transparent)] text-[var(--accent)]',
  ledgerSourceTag: 'bg-[color-mix(in_oklch,var(--fg)_4%,transparent)] text-[var(--muted)]',
  ledgerMeta: 'font-mono text-xs text-[var(--muted)]',
  ledgerAmount: 'font-mono font-bold text-[var(--fg)]',
  ledgerAmountCredit: 'text-[var(--accent)]',
  ledgerFooter: 'mt-8 text-center',
  accentGhost: 'text-[var(--accent)]',
  bucketList: 'mt-6 grid gap-2.5',
  bucketCard: 'rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-3.5',
  bucketTrial: 'bg-[color-mix(in_oklch,var(--accent)_8%,transparent)]',
  bucketWarning: 'border-[color-mix(in_oklch,var(--accent)_55%,var(--border))]',
  bucketHead: 'flex items-baseline justify-between gap-3',
  bucketTitle: 'font-extrabold',
  bucketAmount: 'num font-extrabold',
  bucketHint: 'mt-1.5 text-xs text-[var(--muted)]',
  bucketHintWarning: 'text-[var(--accent)]',
  profileHeader: 'mb-8 flex items-center gap-6',
  avatar: 'grid size-20 place-items-center rounded-2xl bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] text-[32px] font-extrabold text-white shadow-[0_0_30px_rgba(var(--accent-rgb),0.3)]',
  profileName: 'text-xl font-bold',
  profileEmail: 'text-sm text-[var(--muted)]',
  prefGrid: 'grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3',
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

  async function load() {
    setLoading(true)
    try {
      const [nextProfile, nextBalance, nextLedger] = await Promise.all([userApi.getProfile(), userApi.getBalance(), userApi.getLedger()])
      setProfile(nextProfile)
      setBalance(nextBalance)
      setLedger(nextLedger)
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  async function redeemCode(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      await userApi.redeemCode(redeem)
      await Promise.all([load(), app.refreshAccount()])
      app.notify('success', '兑换成功，余额已更新')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function saveProfile(patch: Partial<UserProfile>) {
    setBusy(true)
    try {
      const next = await userApi.updateProfile(patch)
      setProfile(next)
      await app.refreshAccount()
      app.notify('success', '账户偏好已保存')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <ProfileLoadingSkeleton />

  return (
    <SettingsWorkspace
      active="profile"
      title="个人资料"
      detail="管理账户资料、积分余额、兑换记录与默认生成偏好。"
    >
      <div className={profileClasses.grid}>
        <div className={profileClasses.stack}>
          <div className={cn(profileClasses.card, 'pg-enter')}>
            <div className={profileClasses.cardTitle}>◈ 我的积分</div>
            <div className={profileClasses.balanceDisplay}>
              <div className={profileClasses.balanceNum}>{balance?.available_points ?? app.balance?.available_points ?? '0.00000'}</div>
              <div className={profileClasses.balanceLabel}>可用积分余额 (◈) / 冻结 {balance?.frozen_points ?? app.balance?.frozen_points ?? '0.00000'}</div>
            </div>
            <div className={profileClasses.compactStack}>
              <button className={cn(btn.base, btn.primary)} type="button" onClick={() => app.navigate('checkout')}>充值积分</button>
              <button className={btn.base} type="button" onClick={() => setShowRedeemInput((v) => !v)}>{showRedeemInput ? '取消' : '使用兑换码'}</button>
              {showRedeemInput ? (
                <form onSubmit={redeemCode} className={profileClasses.redeemForm}>
                  <input className={form.input} value={redeem} onChange={(event) => setRedeem(event.target.value)} placeholder="输入兑换码" />
                  <button className={cn(btn.base, btn.primary)} type="submit" disabled={busy}>{busy ? '处理中...' : '确认兑换'}</button>
                </form>
              ) : null}
            </div>
            <div className={profileClasses.planBlock}>
              <div className={profileClasses.planRow}><span className={profileClasses.planLabel}>当前套餐</span><span className={profileClasses.planValue}>{balance?.plan_name ?? profile?.tier ?? '免费计划'}</span></div>
              <div className={profileClasses.planRow}><span className={profileClasses.planLabel}>用户分组</span><span className="num">{profile?.group ?? 'DEFAULT (1.0x)'}</span></div>
            </div>
            <BalanceBuckets balance={balance} />
          </div>
        </div>

        <div className={profileClasses.stack}>
          {profile ? <ProfileEditor profile={profile} busy={busy} onSave={saveProfile} /> : null}

          <div className={cn(profileClasses.card, 'pg-enter')}>
            <div className={profileClasses.ledgerHeader}>
              <div className={profileClasses.ledgerTitle}>积分流水 (最近记录)</div>
              <Button tone="ghost" onClick={load}>刷新</Button>
            </div>
            {!ledger.length ? <EmptyState title="暂无流水" detail="生成或兑换后会在这里记录。" /> : null}
            <div className={profileClasses.ledgerList}>
              {profileLedgerRows(ledger).map((entry) => (
                <div key={entry.id} className={profileClasses.ledgerItem}>
                  <div className={profileClasses.ledgerMain}>
                    <div className={profileClasses.ledgerItemTitle}>{entry.title}</div>
                    <div className={profileClasses.ledgerTags}>
                      <span className={cn(profileClasses.ledgerTag, profileClasses.ledgerBucketTag)}>{entry.bucketLabel}</span>
                      <span className={cn(profileClasses.ledgerTag, profileClasses.ledgerSourceTag)}>{entry.ledgerTypeLabel}</span>
                      <span className={cn(profileClasses.ledgerTag, profileClasses.ledgerSourceTag)}>{entry.sourceLabel}</span>
                      <span className={cn(profileClasses.ledgerTag, profileClasses.ledgerSourceTag)}>{entry.expiryText}</span>
                    </div>
                    <div className={profileClasses.ledgerMeta}>{entry.occurredAt} · {entry.detail}</div>
                  </div>
                  <div className={cn(profileClasses.ledgerAmount, entry.amountTone === 'credit' && profileClasses.ledgerAmountCredit)}>{entry.amount}</div>
                </div>
              ))}
            </div>
            <div className={profileClasses.ledgerFooter}>
              <button className={cn(btn.base, btn.ghost, profileClasses.accentGhost)} type="button" onClick={load}>查看全部流水记录</button>
            </div>
          </div>
        </div>
      </div>
    </SettingsWorkspace>
  )
}

function ProfileLoadingSkeleton() {
  return (
    <div className={profileClasses.content} aria-hidden="true">
      <div className="mb-12">
        <div className="pg-skeleton h-12 w-40 rounded-xl md:h-16" />
      </div>
      <div className={profileClasses.grid}>
        <div className={profileClasses.stack}>
          <div className={cn(profileClasses.card, 'grid gap-4')}>
            <div className="pg-skeleton h-4 w-24 rounded-xl" />
            <div className="pg-skeleton h-16 w-56 rounded-xl" />
            <div className="grid gap-3">
              <div className="pg-skeleton h-11 rounded-xl" />
              <div className="pg-skeleton h-11 rounded-xl" />
            </div>
          </div>
        </div>
        <div className={profileClasses.stack}>
          <div className={cn(profileClasses.card, 'grid gap-4')}>
            <div className="pg-skeleton h-24 rounded-2xl" />
            <div className="pg-skeleton h-11 rounded-xl" />
            <div className="pg-skeleton h-28 rounded-2xl" />
            <div className="pg-skeleton h-11 rounded-xl" />
          </div>
          <div className={cn(profileClasses.card, 'grid gap-3')}>
            {Array.from({ length: 5 }).map((_, index) => (
              <div key={index} className="grid gap-2 border-b border-[var(--border)] py-3 last:border-b-0">
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
    <div className={profileClasses.bucketList}>
      {buckets.map((bucket) => (
        <div key={`${bucket.bucket}-${bucket.expires_at ?? 'never'}`} className={cn(profileClasses.bucketCard, bucket.bucket === 'trial' && profileClasses.bucketTrial, bucket.expire_warning && profileClasses.bucketWarning)}>
          <div className={profileClasses.bucketHead}>
            <span className={profileClasses.bucketTitle}>{bucket.label ?? balanceBucketLabel(bucket.bucket)}</span>
            <span className={profileClasses.bucketAmount}>{bucket.available_points}</span>
          </div>
          <div className={cn(profileClasses.bucketHint, bucket.expire_warning && profileClasses.bucketHintWarning)}>
            {bucketExpiryText(bucket)}
          </div>
        </div>
      ))}
    </div>
  )
}

function ProfileEditor({ profile, busy, onSave }: { profile: UserProfile; busy: boolean; onSave: (patch: Partial<UserProfile>) => Promise<void> }) {
  const [name, setName] = useState(profile.display_name)
  const [signature, setSignature] = useState(profile.signature)
  const [preferences, setPreferences] = useState<GenerationPreferences>(profile.preferences)

  return (
    <div className={cn(profileClasses.card, 'pg-enter')}>
      <div className={profileClasses.cardTitle}>基本信息</div>
      <div className={profileClasses.profileHeader}>
        <div className={profileClasses.avatar}>{profile.avatar_initials}</div>
        <div>
          <div className={profileClasses.profileName}>{profile.display_name}</div>
          <div className={profileClasses.profileEmail}>{profile.email}</div>
        </div>
      </div>
      <Field label="显示昵称"><input className={form.input} value={name} onChange={(event) => setName(event.target.value)} /></Field>
      <Field label="签名"><textarea className={form.textarea} value={signature} onChange={(event) => setSignature(event.target.value)} rows={3} /></Field>
      <div className={profileClasses.prefGrid}>
        <Field label="默认模型"><select className={form.input} value={preferences.model_group} onChange={(event) => setPreferences({ ...preferences, model_group: event.target.value })}><option value="basic-image">Basic Image</option><option value="plus-image">Plus Image</option><option value="pro-image">Pro Studio</option></select></Field>
        <Field label="默认比例"><select className={form.input} value={preferences.aspect_ratio} onChange={(event) => setPreferences({ ...preferences, aspect_ratio: event.target.value })}><option>1:1</option><option>16:9</option><option>9:16</option><option>4:3</option></select></Field>
        <Field label="默认基础分辨率"><select className={form.input} value={preferences.base_resolution} onChange={(event) => setPreferences({ ...preferences, base_resolution: event.target.value })}><option>auto</option><option>1K</option><option>2K</option><option>4K</option></select></Field>
      </div>
      <button className={cn(btn.base, btn.primary)} type="button" disabled={busy} onClick={() => void onSave({ display_name: name, signature, preferences })}>{busy ? '保存中...' : '保存修改'}</button>
    </div>
  )
}
