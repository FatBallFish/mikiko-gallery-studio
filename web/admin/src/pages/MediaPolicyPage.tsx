import { useEffect, useMemo, useState } from 'react'
import type { MediaPolicy } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, InlineFeedback, LoadingBlock, PageHeader } from '../components'
import { adminPage } from '../ui/classes'
import { FormSaveBar, FormSection, LedgerField, MultiSelectField, SwitchField } from './AdminMediaTabs'
import { settingsValuesEqual, systemSettingsSectionAnchor } from './systemSettingsTabs'

const formatOptions: Record<'image' | 'video' | 'audio', string[]> = {
  image: ['png', 'jpeg', 'webp', 'gif', 'avif'],
  video: ['mp4', 'webm', 'mov', 'mkv'],
  audio: ['mp3', 'wav', 'aac', 'flac'],
}

const deriveToggles = [
  ['video_poster_enabled', '生成视频 Poster'],
  ['video_hover_preview_enabled', '生成视频悬浮预览'],
  ['video_proxy_enabled', '生成视频 Proxy（720p）'],
  ['audio_proxy_enabled', '生成音频 Proxy'],
  ['audio_waveform_enabled', '生成音频波形'],
] as const

type MediaPolicyEditor = {
  policy: MediaPolicy
  baseline: MediaPolicy
}

export function MediaPolicyPage({
  compact = false,
  activeSection,
  onDirtyChange,
  onBusyChange,
}: {
  compact?: boolean
  activeSection?: string
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
} = {}) {
  const [editor, setEditor] = useState<MediaPolicyEditor | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  const policy = editor?.policy ?? null
  const baseline = editor?.baseline ?? null

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const next = clonePolicy(await adminApi.getMediaPolicy())
      // Policy and baseline are captured in one state update so the form can
      // never render an intermediate frame where an untouched draft is dirty.
      setEditor({ policy: next, baseline: clonePolicy(next) })
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '媒体策略载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const dirty = useMemo(() => Boolean(
    editor && !settingsValuesEqual(mediaPolicyEditableSnapshot(editor.policy), mediaPolicyEditableSnapshot(editor.baseline)),
  ), [editor])
  const dirtyCount = useMemo(() => editor ? mediaPolicyDirtyCount(editor.policy, editor.baseline) : 0, [editor])
  const validation = policy ? validateMediaPolicy(policy) : {}
  const errorCount = Object.keys(validation).length

  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])
  useEffect(() => { onBusyChange?.(saving) }, [onBusyChange, saving])
  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  async function save() {
    if (!editor || errorCount) return
    setSaving(true)
    setError(null)
    try {
      const saved = clonePolicy(await adminApi.updateMediaPolicy(editor.policy))
      // Reset the baseline atomically with the saved policy so "有未保存修改"
      // clears immediately after a successful save.
      setEditor({ policy: saved, baseline: clonePolicy(saved) })
      setNotice('媒体策略已保存；只影响新上传对象和后续新建的派生版本。')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '媒体策略保存失败')
    } finally {
      setSaving(false)
    }
  }

  function reset() {
    setEditor((current) => current ? { policy: clonePolicy(current.baseline), baseline: current.baseline } : current)
    setNotice('已恢复为当前生效值。')
    setError(null)
  }

  function updatePolicy(updater: (current: MediaPolicy) => MediaPolicy) {
    setEditor((current) => current ? { ...current, policy: updater(current.policy) } : current)
  }

  function updateFormats(kind: 'image' | 'video' | 'audio', values: string[]) {
    updatePolicy((current) => ({
      ...current,
      allowed_formats: {
        ...current.allowed_formats,
        [kind]: values.map((item) => item.trim().toLowerCase()).filter(Boolean),
      },
    }))
  }

  if (loading && !policy) return <LoadingBlock label="载入媒体策略" />
  if (!policy || !baseline) return <InlineFeedback tone="danger" message={error ?? '媒体策略不可用'} />

  return (
    <section className={adminPage.stack}>
      {!compact ? <PageHeader title="媒体策略" description="统一约束图片、视频和音频上传、派生与保留；版本更新不会静默改变历史资产。" /> : null}
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {notice ? <InlineFeedback tone="success" message={notice} /> : null}

      <section className="a-panel a-panel--flush">
        <div className="a-panel__head">
          <div><h2>媒体策略</h2><p>派生资源、会话与保留期限；配置变更只影响新上传与后续派生。</p></div>
          <div className="a-row-actions">
            <Badge tone="neutral">v{policy.version}</Badge>
            <Badge tone={errorCount ? 'warning' : 'success'}>{errorCount ? `${errorCount} 项校验未通过` : '校验通过'}</Badge>
            <Badge tone={dirty ? 'warning' : 'neutral'}>{dirty ? `${dirtyCount} 项未保存` : '未修改'}</Badge>
          </div>
        </div>

        <form className="a-form" onSubmit={(event) => { event.preventDefault(); void save() }}>
          <div
            id={systemSettingsSectionAnchor('media-policy', 'formats-quota')}
            className={cn('ss-block', activeSection === 'formats-quota' && 'is-active')}
          >
            <FormSection index={1} title="格式与配额" description="单文件上限、视频时长与用户存储配额按新上传对象生效。">
              <MultiSelectField
                label="图片允许格式"
                values={policy.allowed_formats.image}
                options={Array.from(new Set([...formatOptions.image, ...policy.allowed_formats.image]))}
                onChange={(values) => updateFormats('image', values)}
                error={validation.image_formats}
                customPlaceholder="输入格式后添加"
                normalize={(value) => value.trim().toLowerCase()}
              />
              <MultiSelectField
                label="视频允许格式"
                values={policy.allowed_formats.video}
                options={Array.from(new Set([...formatOptions.video, ...policy.allowed_formats.video]))}
                onChange={(values) => updateFormats('video', values)}
                error={validation.video_formats}
                customPlaceholder="输入格式后添加"
                normalize={(value) => value.trim().toLowerCase()}
              />
              <MultiSelectField
                label="音频允许格式"
                values={policy.allowed_formats.audio}
                options={Array.from(new Set([...formatOptions.audio, ...policy.allowed_formats.audio]))}
                onChange={(values) => updateFormats('audio', values)}
                error={validation.audio_formats}
                customPlaceholder="输入格式后添加"
                normalize={(value) => value.trim().toLowerCase()}
              />
              <div className="a-form__pair">
                <LedgerField label="单文件上限（字节）" error={validation.single_file_max_bytes} hint="最大 1GB；图片与视频共用。">
                  <input className="a-input" type="number" min="1" max="1073741824" value={policy.single_file_max_bytes} onChange={(event) => updatePolicy((current) => ({ ...current, single_file_max_bytes: Number(event.target.value) }))} />
                </LedgerField>
                <LedgerField label="视频最长时长（秒）" error={validation.video_max_duration_seconds} hint="0 表示不限制。">
                  <input className="a-input" type="number" min="0" value={policy.video_max_duration_seconds} onChange={(event) => updatePolicy((current) => ({ ...current, video_max_duration_seconds: Number(event.target.value) }))} />
                </LedgerField>
              </div>
              <LedgerField label="用户存储配额（字节）" error={validation.user_quota_bytes}>
                <input className="a-input" type="number" min="1" value={policy.user_quota_bytes} onChange={(event) => updatePolicy((current) => ({ ...current, user_quota_bytes: Number(event.target.value) }))} />
              </LedgerField>
            </FormSection>
          </div>

          <div
            id={systemSettingsSectionAnchor('media-policy', 'derivatives')}
            className={cn('ss-block', activeSection === 'derivatives' && 'is-active')}
          >
            <FormSection index={2} title="派生资源" description="原图/原视频始终保留，缩略图与代理按需生成。">
              <MultiSelectField
                label="图片缩略图档位（像素）"
                values={policy.image_thumbnail_widths.map(String)}
                options={Array.from(new Set(['256', '512', '1024', '2048', ...policy.image_thumbnail_widths.map(String)]))}
                onChange={(values) => updatePolicy((current) => ({ ...current, image_thumbnail_widths: values.map(Number).filter((value) => value > 0) }))}
                error={validation.image_thumbnail_widths}
                customPlaceholder="输入宽度后添加"
                normalize={(value) => String(Number(value.replace(/[^\d]/g, '')) || '')}
                hint="原图仍保留，缩略图按需生成。"
              />
              {deriveToggles.map(([key, label]) => (
                <SwitchField key={key} label={label} checked={policy[key]} onChange={(checked) => updatePolicy((current) => ({ ...current, [key]: checked }))} />
              ))}
            </FormSection>
          </div>

          <div
            id={systemSettingsSectionAnchor('media-policy', 'retention')}
            className={cn('ss-block', activeSection === 'retention' && 'is-active')}
          >
            <FormSection index={3} title="会话与保留" description="修改媒体策略只影响新上传与后续派生，不覆盖历史对象。">
              <div className="a-form__pair">
                <LedgerField label="上传会话有效期（小时）" error={validation.upload_session_ttl_hours}>
                  <input className="a-input" type="number" min="1" max="72" value={policy.upload_session_ttl_hours} onChange={(event) => updatePolicy((current) => ({ ...current, upload_session_ttl_hours: Number(event.target.value) }))} />
                </LedgerField>
                <LedgerField label="失败处理保留（天）" error={validation.failed_processing_retention_days}>
                  <input className="a-input" type="number" min="0" value={policy.failed_processing_retention_days} onChange={(event) => updatePolicy((current) => ({ ...current, failed_processing_retention_days: Number(event.target.value) }))} />
                </LedgerField>
              </div>
              <LedgerField label="软删除保留（天）" error={validation.soft_delete_retention_days}>
                <input className="a-input cfg-narrow-input" type="number" min="0" value={policy.soft_delete_retention_days} onChange={(event) => updatePolicy((current) => ({ ...current, soft_delete_retention_days: Number(event.target.value) }))} />
              </LedgerField>
            </FormSection>
          </div>
        </form>

        <FormSaveBar
          dirtyCount={dirtyCount}
          errorCount={errorCount}
          busy={saving}
          note={`当前版本 v${policy.version} · 变更只影响新上传对象和后续新建的派生版本。`}
          onReset={reset}
          onSave={() => void save()}
          saveLabel="保存媒体策略"
          saveDisabled={!dirty}
        />
      </section>
    </section>
  )
}

/**
 * Baseline-backed dirty comparison ignores server-managed fields
 * (`version`, `applies_to`) and normalises numeric/format representations so
 * an untouched draft never reads as unsaved.
 */
export function mediaPolicyEditableSnapshot(policy: MediaPolicy) {
  return {
    allowed_formats: {
      image: normalizeFormats(policy.allowed_formats?.image),
      video: normalizeFormats(policy.allowed_formats?.video),
      audio: normalizeFormats(policy.allowed_formats?.audio),
    },
    single_file_max_bytes: normalizeNumber(policy.single_file_max_bytes),
    video_max_duration_seconds: normalizeNumber(policy.video_max_duration_seconds),
    user_quota_bytes: normalizeNumber(policy.user_quota_bytes),
    image_thumbnail_widths: (policy.image_thumbnail_widths ?? []).map(normalizeNumber).filter((value) => value > 0).sort((left, right) => left - right),
    video_poster_enabled: Boolean(policy.video_poster_enabled),
    video_hover_preview_enabled: Boolean(policy.video_hover_preview_enabled),
    video_proxy_enabled: Boolean(policy.video_proxy_enabled),
    audio_proxy_enabled: Boolean(policy.audio_proxy_enabled),
    audio_waveform_enabled: Boolean(policy.audio_waveform_enabled),
    upload_session_ttl_hours: normalizeNumber(policy.upload_session_ttl_hours),
    failed_processing_retention_days: normalizeNumber(policy.failed_processing_retention_days),
    soft_delete_retention_days: normalizeNumber(policy.soft_delete_retention_days),
  }
}

function normalizeFormats(values: string[] | undefined) {
  return Array.from(new Set((values ?? []).map((value) => String(value).trim().toLowerCase()).filter(Boolean))).sort()
}

function normalizeNumber(value: number | string | undefined) {
  const next = Number(value)
  return Number.isFinite(next) ? next : 0
}

function clonePolicy(policy: MediaPolicy): MediaPolicy {
  return {
    ...policy,
    allowed_formats: {
      image: [...(policy.allowed_formats?.image ?? [])],
      video: [...(policy.allowed_formats?.video ?? [])],
      audio: [...(policy.allowed_formats?.audio ?? [])],
    },
    image_thumbnail_widths: [...(policy.image_thumbnail_widths ?? [])],
  }
}

function mediaPolicyDirtyCount(policy: MediaPolicy, baseline: MediaPolicy): number {
  const current = mediaPolicyEditableSnapshot(policy)
  const previous = mediaPolicyEditableSnapshot(baseline)
  let count = 0
  if (current.single_file_max_bytes !== previous.single_file_max_bytes) count += 1
  if (current.video_max_duration_seconds !== previous.video_max_duration_seconds) count += 1
  if (current.user_quota_bytes !== previous.user_quota_bytes) count += 1
  if (current.upload_session_ttl_hours !== previous.upload_session_ttl_hours) count += 1
  if (current.failed_processing_retention_days !== previous.failed_processing_retention_days) count += 1
  if (current.soft_delete_retention_days !== previous.soft_delete_retention_days) count += 1
  for (const kind of ['image', 'video', 'audio'] as const) {
    if (current.allowed_formats[kind].join(',') !== previous.allowed_formats[kind].join(',')) count += 1
  }
  if (current.image_thumbnail_widths.join(',') !== previous.image_thumbnail_widths.join(',')) count += 1
  for (const [key] of deriveToggles) {
    if (current[key] !== previous[key]) count += 1
  }
  return count
}

function validateMediaPolicy(policy: MediaPolicy): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const kind of ['image', 'video', 'audio'] as const) {
    if (!policy.allowed_formats[kind].length) errors[`${kind}_formats`] = '至少配置一种允许格式。'
  }
  if (policy.single_file_max_bytes < 1 || policy.single_file_max_bytes > 1073741824) errors.single_file_max_bytes = '单文件上限必须在 1 字节到 1GB 之间。'
  if (policy.video_max_duration_seconds < 0) errors.video_max_duration_seconds = '视频最长时长不能为负数。'
  if (policy.user_quota_bytes < 1) errors.user_quota_bytes = '用户存储配额必须大于 0。'
  if (policy.upload_session_ttl_hours < 1 || policy.upload_session_ttl_hours > 72) errors.upload_session_ttl_hours = '上传会话有效期必须在 1–72 小时之间。'
  if (policy.failed_processing_retention_days < 0) errors.failed_processing_retention_days = '失败处理保留期不能为负数。'
  if (policy.soft_delete_retention_days < 0) errors.soft_delete_retention_days = '软删除保留期不能为负数。'
  return errors
}