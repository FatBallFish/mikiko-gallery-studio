import { useEffect, useState } from 'react'
import type { MediaPolicy } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Field, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'

export function MediaPolicyPage() {
  const [policy, setPolicy] = useState<MediaPolicy | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  async function load() { setLoading(true); setError(null); try { setPolicy(await adminApi.getMediaPolicy()) } catch (caught) { setError(caught instanceof Error ? caught.message : '媒体策略载入失败') } finally { setLoading(false) } }
  useEffect(() => { void load() }, [])
  async function save() { if (!policy) return; setSaving(true); setError(null); try { setPolicy(await adminApi.updateMediaPolicy(policy)); setNotice('媒体策略已保存；只影响新上传对象和后续新建的派生版本。') } catch (caught) { setError(caught instanceof Error ? caught.message : '媒体策略保存失败') } finally { setSaving(false) } }
  const formats = (kind: 'image' | 'video' | 'audio') => policy?.allowed_formats[kind].join(', ') ?? ''
  const updateFormats = (kind: 'image' | 'video' | 'audio', value: string) => setPolicy((current) => current ? ({ ...current, allowed_formats: { ...current.allowed_formats, [kind]: value.split(',').map((item) => item.trim().toLowerCase()).filter(Boolean) } }) : current)
  if (loading && !policy) return <LoadingBlock label="载入媒体策略" />
  return <section className={adminPage.stack}>
    <PageHeader title="媒体策略" description="统一约束图片、视频和音频上传、派生与保留；版本更新不会静默改变历史资产。" secondaryActions={<RefreshIconButton label="刷新媒体策略" refreshing={loading} onClick={() => void load()} />} />
    {error ? <InlineFeedback tone="danger" message={error} /> : null}{notice ? <InlineFeedback tone="success" message={notice} /> : null}
    {policy ? <form className="grid gap-6" onSubmit={(event) => { event.preventDefault(); void save() }}>
      <InlineFeedback tone="neutral" message={`当前版本 v${policy.version}。配置变更只影响新上传对象和后续新建的派生版本，不覆盖历史对象。`} />
      <section className="grid gap-4 border-t border-[var(--border)] pt-5"><h2 className="m-0 text-base font-semibold">格式与配额</h2><div className="grid gap-4 lg:grid-cols-3"><Field label="图片允许格式"><input value={formats('image')} onChange={(event) => updateFormats('image', event.target.value)} /></Field><Field label="视频允许格式"><input value={formats('video')} onChange={(event) => updateFormats('video', event.target.value)} /></Field><Field label="音频允许格式"><input value={formats('audio')} onChange={(event) => updateFormats('audio', event.target.value)} /></Field><Field label="单文件上限（字节）"><input type="number" min="1" max="1073741824" value={policy.single_file_max_bytes} onChange={(event) => setPolicy({ ...policy, single_file_max_bytes: Number(event.target.value) })} /></Field><Field label="视频最长时长（秒，0 为不限制）"><input type="number" min="0" value={policy.video_max_duration_seconds} onChange={(event) => setPolicy({ ...policy, video_max_duration_seconds: Number(event.target.value) })} /></Field><Field label="用户存储配额（字节）"><input type="number" min="1" value={policy.user_quota_bytes} onChange={(event) => setPolicy({ ...policy, user_quota_bytes: Number(event.target.value) })} /></Field></div></section>
      <section className="grid gap-4 border-t border-[var(--border)] pt-5"><h2 className="m-0 text-base font-semibold">派生资源</h2><div className="grid gap-4 lg:grid-cols-3"><Field label="图片缩略图档位"><input value={policy.image_thumbnail_widths.join(', ')} onChange={(event) => setPolicy({ ...policy, image_thumbnail_widths: event.target.value.split(',').map(Number).filter((value) => value > 0) })} /></Field>{([['video_poster_enabled', '视频 Poster'], ['video_hover_preview_enabled', '视频悬浮预览'], ['video_proxy_enabled', '视频详情 Proxy'], ['audio_proxy_enabled', '音频 Proxy'], ['audio_waveform_enabled', '音频波形']] as const).map(([key, label]) => <label key={key} className="flex min-h-11 items-center gap-3 text-sm"><input type="checkbox" checked={policy[key]} onChange={(event) => setPolicy({ ...policy, [key]: event.target.checked })} /><span>{label}</span></label>)}</div></section>
      <section className="grid gap-4 border-t border-[var(--border)] pt-5"><h2 className="m-0 text-base font-semibold">会话与保留</h2><div className="grid gap-4 lg:grid-cols-3"><Field label="上传会话（小时）"><input type="number" min="1" max="72" value={policy.upload_session_ttl_hours} onChange={(event) => setPolicy({ ...policy, upload_session_ttl_hours: Number(event.target.value) })} /></Field><Field label="失败处理保留期（天）"><input type="number" min="0" value={policy.failed_processing_retention_days} onChange={(event) => setPolicy({ ...policy, failed_processing_retention_days: Number(event.target.value) })} /></Field><Field label="软删除保留期（天）"><input type="number" min="0" value={policy.soft_delete_retention_days} onChange={(event) => setPolicy({ ...policy, soft_delete_retention_days: Number(event.target.value) })} /></Field></div></section>
      <div><button className={cn(adminButton.base, adminButton.primary)} type="submit" disabled={saving}>{saving ? '保存中...' : '保存媒体策略'}</button></div>
    </form> : null}
  </section>
}
