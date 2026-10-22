import { useEffect, useMemo, useState } from 'react'
import { FileText, Image, Music2, RotateCcw, Save, Video } from 'lucide-react'
import { ADMIN_PERMISSIONS, type AdminSession, type ConfigItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader } from '../components'
import { FormSaveBar, FormSection, LedgerField, MultiSelectField } from './AdminMediaTabs'
import { canAdmin } from '../types'
import { adminButton, adminPage } from '../ui/classes'

export type AttachmentPolicyKey =
  | 'image_max_mb'
  | 'video_max_mb'
  | 'audio_max_mb'
  | 'document_max_mb'
  | 'image_allowed_formats'
  | 'video_allowed_formats'
  | 'audio_allowed_formats'
  | 'document_allowed_formats'

type AttachmentKind = 'image' | 'video' | 'audio' | 'document'
type AttachmentSizeKey = Extract<AttachmentPolicyKey, `${string}_max_mb`>
type AttachmentFormatsKey = Extract<AttachmentPolicyKey, `${string}_allowed_formats`>

export type AttachmentPolicyDraft = Record<AttachmentSizeKey, number> & Record<AttachmentFormatsKey, string[]>

export type AttachmentPolicyFieldDefinition = {
  key: AttachmentPolicyKey
  kind: AttachmentKind
  control: 'number' | 'formats'
  label: string
  hint: string
  reserved: boolean
}

export const attachmentPolicyDefaults: AttachmentPolicyDraft = {
  image_max_mb: 20,
  video_max_mb: 100,
  audio_max_mb: 50,
  document_max_mb: 20,
  image_allowed_formats: ['png', 'jpeg', 'webp', 'gif'],
  video_allowed_formats: ['mp4'],
  audio_allowed_formats: ['mp3'],
  document_allowed_formats: ['pdf'],
}

export const attachmentPolicyFieldDefinitions: AttachmentPolicyFieldDefinition[] = [
  { key: 'image_max_mb', kind: 'image', control: 'number', label: '最大图片附件大小', hint: '单张图片允许上传的最大体积，单位 MB，当前默认 20 MB。', reserved: false },
  { key: 'image_allowed_formats', kind: 'image', control: 'formats', label: '支持的图片格式', hint: '使用逗号分隔扩展名；仅支持 PNG、JPEG、WebP 和 GIF，不允许 SVG。', reserved: false },
  { key: 'video_max_mb', kind: 'video', control: 'number', label: '最大视频附件大小', hint: '为后续视频工作流预留的单文件上限，当前不会开启视频上传入口。', reserved: true },
  { key: 'video_allowed_formats', kind: 'video', control: 'formats', label: '支持的视频格式', hint: '为后续视频工作流预留，使用逗号分隔扩展名。', reserved: true },
  { key: 'audio_max_mb', kind: 'audio', control: 'number', label: '最大音频附件大小', hint: '为后续音频工作流预留的单文件上限，当前不会开启音频上传入口。', reserved: true },
  { key: 'audio_allowed_formats', kind: 'audio', control: 'formats', label: '支持的音频格式', hint: '为后续音频工作流预留，使用逗号分隔扩展名。', reserved: true },
  { key: 'document_max_mb', kind: 'document', control: 'number', label: '最大文档附件大小', hint: '为后续文档工作流预留的单文件上限，当前不会开启文档上传入口。', reserved: true },
  { key: 'document_allowed_formats', kind: 'document', control: 'formats', label: '支持的文档格式', hint: '为后续文档工作流预留，使用逗号分隔扩展名。', reserved: true },
]

const kindMeta: Record<AttachmentKind, { title: string; detail: string; icon: typeof Image }> = {
  image: { title: '图片附件', detail: '仅图片策略当前生效，用户端与 API 会执行相同校验。', icon: Image },
  video: { title: '视频附件', detail: '预留配置，不会在本版本新增视频上传流程。', icon: Video },
  audio: { title: '音频附件', detail: '预留配置，不会在本版本新增音频上传流程。', icon: Music2 },
  document: { title: '文档附件', detail: '预留配置，不会在本版本新增文档上传流程。', icon: FileText },
}

const kinds: AttachmentKind[] = ['image', 'video', 'audio', 'document']

const attachmentClasses = {
  surface: 'overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)]',
  status: 'flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] bg-[var(--surface)] px-4 py-3',
  policyGrid: 'grid grid-cols-2 max-[860px]:grid-cols-1',
  policySection: 'grid min-w-0 content-start gap-4 border-b border-r border-[var(--border)] p-4 even:border-r-0 [&:nth-last-child(-n+2)]:border-b-0 max-[860px]:border-r-0 max-[860px]:[&:nth-last-child(-n+2)]:border-b max-[860px]:last:border-b-0',
  sectionHead: 'flex min-w-0 items-start justify-between gap-3',
  sectionIdentity: 'flex min-w-0 items-start gap-3',
  icon: 'grid size-9 shrink-0 place-items-center rounded-md border border-[var(--border)] bg-[var(--surface)] text-[var(--accent)]',
  title: 'm-0 text-sm font-extrabold text-[var(--text)]',
  detail: 'm-0 mt-1 text-xs leading-5 text-[var(--soft)]',
  fields: 'grid grid-cols-2 items-start gap-3 max-[620px]:grid-cols-1',
  saveRail: 'sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] bg-[var(--surface-solid)]/95 p-3 backdrop-blur',
  actions: 'flex flex-wrap items-center gap-2',
}

export function AttachmentPolicyPage({
  session,
  onFeedback,
  onDirtyChange,
  onBusyChange,
  compact = false,
}: {
  session: AdminSession
  onFeedback?: (title: string, detail?: string) => void
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
  compact?: boolean
}) {
  const [rows, setRows] = useState<ConfigItem[]>([])
  const [draft, setDraft] = useState<AttachmentPolicyDraft>(() => cloneAttachmentPolicyDraft(attachmentPolicyDefaults))
  const [baseline, setBaseline] = useState<AttachmentPolicyDraft>(() => cloneAttachmentPolicyDraft(attachmentPolicyDefaults))
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveFeedback, setSaveFeedback] = useState<{ tone: 'success' | 'danger'; message: string } | null>(null)
  const [showErrors, setShowErrors] = useState(false)

  const canEdit = canAdmin(session, ADMIN_PERMISSIONS.manageConfig)
  const dirty = useMemo(() => attachmentPolicyIsDirty(baseline, draft), [baseline, draft])
  const validationErrors = useMemo(() => validateAttachmentPolicyDraft(draft), [draft])
  const validationCount = Object.keys(validationErrors).length
  const dirtyFieldCount = attachmentPolicyDirtyFieldCount(baseline, draft)

  async function load(initial = false) {
    if (initial) setLoading(true)
    setLoadError(null)
    try {
      const nextRows = (await adminApi.listConfig()).filter((row) => (row.config_category || row.tab) === 'attachment_policy')
      const nextDraft = attachmentPolicyDraftFromRows(nextRows)
      setRows(nextRows)
      setDraft(nextDraft)
      setBaseline(cloneAttachmentPolicyDraft(nextDraft))
      setShowErrors(false)
      return true
    } catch (caught) {
      setLoadError(caught instanceof Error ? caught.message : '附件策略读取失败')
      return false
    } finally {
      if (initial) setLoading(false)
    }
  }

  useEffect(() => { void load(true) }, [])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])

  useEffect(() => {
    onBusyChange?.(saving)
  }, [onBusyChange, saving])

  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty])

  function updateField(field: AttachmentPolicyFieldDefinition, value: string) {
    setSaveFeedback(null)
    setDraft((current) => ({
      ...current,
      [field.key]: field.control === 'number' ? Number(value) : normalizeAttachmentFormats([value]),
    }))
  }

  function updateFormats(field: AttachmentPolicyFieldDefinition, formats: string[]) {
    setSaveFeedback(null)
    setDraft((current) => ({
      ...current,
      [field.key]: normalizeAttachmentFormats(formats),
    }))
  }

  function revert() {
    setDraft(cloneAttachmentPolicyDraft(baseline))
    setShowErrors(false)
    setSaveFeedback(null)
  }

  async function save() {
    setShowErrors(true)
    if (validationCount || !dirty || !canEdit) return
    setSaving(true)
    setSaveFeedback(null)
    try {
      const version = Math.max(1, ...rows.map((row) => row.version || 1))
      await adminApi.updateConfigTab('attachment_policy', {
        version,
        items: attachmentPolicyFieldDefinitions.map((field) => ({
          config_category: 'attachment_policy',
          config_key: field.key,
          config_value: { value: attachmentPolicyValueForSave(draft, field) },
          scope: rows.find((row) => (row.config_key || row.key) === field.key)?.scope || 'global',
        })),
      })
      const loaded = await load()
      if (!loaded) throw new Error('配置已保存，但重新读取最新值失败，请刷新页面确认。')
      setSaveFeedback({ tone: 'success', message: '附件策略已保存并立即对新上传请求生效。' })
      onFeedback?.('附件策略已保存', '图片大小与格式限制已更新')
    } catch (caught) {
      setSaveFeedback({ tone: 'danger', message: caught instanceof Error ? caught.message : '附件策略保存失败' })
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入附件策略" />
  if (loadError && !rows.length) return <ErrorBlock message={loadError} onRetry={() => void load(true)} />

  return (
    <section className={adminPage.stack} aria-busy={saving}>
      {!compact ? <PageHeader title="附件策略" detail="统一管理附件体积和格式限制；当前只有图片策略接入上传链路。" /> : null}
      <section className="a-panel a-panel--flush">
        <header className="a-panel__head">
          <div>
            <h2>上传校验策略</h2>
            <p>图片配置当前生效，视频、音频与文档为预留配置；多值格式使用多选组件维护。</p>
          </div>
          <div className="a-row-actions">
            <Badge tone={validationCount ? 'warning' : 'success'}>{validationCount ? `${validationCount} 项需修正` : '校验通过'}</Badge>
            <Badge tone={dirty ? 'warning' : 'neutral'}>{dirty ? `${dirtyFieldCount} 项未保存` : '已同步'}</Badge>
          </div>
        </header>

        {loadError ? <div style={{ padding: '12px 20px' }}><InlineFeedback tone="warning" message={loadError} /></div> : null}

        <form className="a-form" onSubmit={(event) => { event.preventDefault(); void save() }}>
          {kinds.map((kind, index) => {
            const meta = kindMeta[kind]
            const Icon = meta.icon
            const fields = attachmentPolicyFieldDefinitions.filter((field) => field.kind === kind)
            return (
              <FormSection
                key={kind}
                index={index + 1}
                title={meta.title}
                description={meta.detail}
                aside={<Badge tone={kind === 'image' ? 'success' : 'neutral'}>{kind === 'image' ? '已生效' : '预留配置'}</Badge>}
              >
                <div className="cfg-inline-actions" style={{ color: 'var(--a-ink-3)', fontSize: 12.5 }}><Icon size={16} aria-hidden="true" />{kind === 'image' ? '用户端与 API 共用同一套校验。' : '配置会保存，但在运行时接入前不会开放对应上传入口。'}</div>
                {fields.map((field) => (
                  field.control === 'number' ? (
                    <LedgerField key={field.key} label={field.label} hint={field.hint} error={showErrors || dirty ? validationErrors[field.key] : null}>
                      <input
                        className="a-input cfg-narrow-input"
                        type="number"
                        min="1"
                        max={attachmentPolicyMaxMB(field.kind)}
                        value={String(draft[field.key as AttachmentSizeKey])}
                        disabled={saving || !canEdit}
                        onChange={(event) => updateField(field, event.target.value)}
                      />
                    </LedgerField>
                  ) : (
                    <MultiSelectField
                      key={field.key}
                      label={field.label}
                      hint={field.hint}
                      error={showErrors || dirty ? validationErrors[field.key] : null}
                      values={draft[field.key as AttachmentFormatsKey]}
                      options={Array.from(new Set([...attachmentFormatOptions(field.kind), ...draft[field.key as AttachmentFormatsKey]]))}
                      onChange={(formats) => updateFormats(field, formats)}
                      disabled={saving || !canEdit}
                      customPlaceholder="输入扩展名后添加"
                      normalize={(value) => normalizeAttachmentFormats([value])[0] ?? ''}
                    />
                  )
                ))}
              </FormSection>
            )
          })}
        </form>

        <FormSaveBar
          dirtyCount={dirtyFieldCount}
          errorCount={validationCount}
          busy={saving}
          note={!canEdit ? '当前账号没有 manage:config 权限，附件策略仅可查看。' : saveFeedback?.message ?? '保存后配置解析缓存会失效，新上传请求立即读取最新策略。'}
          onReset={revert}
          onSave={() => void save()}
          saveLabel="保存附件策略"
          saveDisabled={!canEdit || !dirty}
        />
        {saveFeedback?.tone === 'danger' ? <div style={{ padding: '0 20px 16px' }}><InlineFeedback tone="danger" message={saveFeedback.message} /></div> : null}
      </section>
    </section>
  )
}

export function attachmentPolicyDraftFromRows(rows: ConfigItem[]): AttachmentPolicyDraft {
  const draft = cloneAttachmentPolicyDraft(attachmentPolicyDefaults)
  for (const row of rows) {
    const key = (row.config_key || row.key) as AttachmentPolicyKey
    if (!attachmentPolicyFieldDefinitions.some((field) => field.key === key)) continue
    const value = configItemValue(row)
    if (key.endsWith('_max_mb')) {
      const size = Number(value)
      if (Number.isFinite(size)) draft[key as AttachmentSizeKey] = size
    } else {
      const values = Array.isArray(value) ? value.map(String) : String(value ?? '').split(',')
      draft[key as AttachmentFormatsKey] = normalizeAttachmentFormats(values)
    }
  }
  return draft
}

export function normalizeAttachmentFormats(values: readonly string[]) {
  const normalized: string[] = []
  for (const value of values) {
    for (const part of value.split(/[\s,，]+/)) {
      let format = part.trim().toLowerCase().replace(/^\./, '')
      if (!format) continue
      if (format.includes('/')) format = format.slice(format.lastIndexOf('/') + 1)
      if (format === 'jpg') format = 'jpeg'
      if (!normalized.includes(format)) normalized.push(format)
    }
  }
  return normalized
}

export function validateAttachmentPolicyDraft(draft: AttachmentPolicyDraft): Partial<Record<AttachmentPolicyKey, string>> {
  const errors: Partial<Record<AttachmentPolicyKey, string>> = {}
  for (const field of attachmentPolicyFieldDefinitions) {
    if (field.control === 'number') {
      const value = draft[field.key as AttachmentSizeKey]
      const maxMB = attachmentPolicyMaxMB(field.kind)
      if (!Number.isInteger(value) || value < 1 || value > maxMB) errors[field.key] = `请输入 1-${maxMB} 之间的整数 MB。`
      continue
    }
    const formats = normalizeAttachmentFormats(draft[field.key as AttachmentFormatsKey])
    if (!formats.length) {
      errors[field.key] = '至少配置一种文件格式。'
      continue
    }
    if (formats.some((format) => !/^[a-z0-9][a-z0-9.+-]{0,31}$/.test(format))) {
      errors[field.key] = '格式只能包含字母、数字、点、加号和连字符。'
      continue
    }
    if (field.kind === 'image' && formats.some((format) => !['png', 'jpeg', 'webp', 'gif'].includes(format))) {
      errors[field.key] = '图片格式仅支持 PNG、JPEG、WebP 和 GIF，不支持 SVG。'
    }
  }
  return errors
}

export function attachmentPolicyMaxMB(kind: AttachmentKind) {
  return kind === 'image' ? 100 : 10240
}

export function attachmentPolicyDirtyFieldCount(baseline: AttachmentPolicyDraft, draft: AttachmentPolicyDraft) {
  return attachmentPolicyFieldDefinitions.filter((field) => {
    if (field.control === 'number') return baseline[field.key as AttachmentSizeKey] !== draft[field.key as AttachmentSizeKey]
    return normalizeAttachmentFormats(baseline[field.key as AttachmentFormatsKey]).join(',') !== normalizeAttachmentFormats(draft[field.key as AttachmentFormatsKey]).join(',')
  }).length
}

function attachmentFormatOptions(kind: AttachmentKind) {
  if (kind === 'image') return ['png', 'jpeg', 'webp', 'gif']
  if (kind === 'video') return ['mp4', 'webm', 'mov']
  if (kind === 'audio') return ['mp3', 'wav', 'aac']
  return ['pdf', 'txt', 'md', 'docx']
}

export function attachmentPolicyIsDirty(baseline: AttachmentPolicyDraft, draft: AttachmentPolicyDraft) {
  return attachmentPolicyFieldDefinitions.some((field) => {
    if (field.control === 'number') return baseline[field.key as AttachmentSizeKey] !== draft[field.key as AttachmentSizeKey]
    const before = normalizeAttachmentFormats(baseline[field.key as AttachmentFormatsKey])
    const after = normalizeAttachmentFormats(draft[field.key as AttachmentFormatsKey])
    return before.join(',') !== after.join(',')
  })
}

function attachmentPolicyValueForSave(draft: AttachmentPolicyDraft, field: AttachmentPolicyFieldDefinition) {
  if (field.control === 'number') return draft[field.key as AttachmentSizeKey]
  return normalizeAttachmentFormats(draft[field.key as AttachmentFormatsKey])
}

function cloneAttachmentPolicyDraft(source: AttachmentPolicyDraft): AttachmentPolicyDraft {
  return {
    ...source,
    image_allowed_formats: [...source.image_allowed_formats],
    video_allowed_formats: [...source.video_allowed_formats],
    audio_allowed_formats: [...source.audio_allowed_formats],
    document_allowed_formats: [...source.document_allowed_formats],
  }
}

function configItemValue(row: ConfigItem): unknown {
  if (row.config_value && 'value' in row.config_value) return row.config_value.value
  try {
    const parsed = JSON.parse(row.value)
    if (parsed && typeof parsed === 'object' && 'value' in parsed) return parsed.value
    return parsed
  } catch {
    return row.value
  }
}
