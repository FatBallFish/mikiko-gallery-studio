import { useRef, useState } from 'react'
import type { NativeImageAnnotation } from '../../../../shared/native-image'
import { normalizeNativeImagePoint } from '../../../../shared/native-image'
import { userApi } from '../../../../shared/user-api'
import { useMediaRefreshOnce, type MediaRefreshHandler } from '../../ui/mediaRefresh'
import { Modal } from '../../components'

export type Mark = { kind: 'point' | 'box' | 'brush' | 'arrow'; points: [number, number][] }
export function markAnnotations(marks: Mark[]): NativeImageAnnotation[] {
  return marks.flatMap((mark): NativeImageAnnotation[] => mark.kind === 'point' ? [{ point: mark.points[0] }] : mark.kind === 'box' ? [{ bbox: [Math.min(mark.points[0][0], mark.points[1][0]), Math.min(mark.points[0][1], mark.points[1][1]), Math.max(mark.points[0][0], mark.points[1][0]), Math.max(mark.points[0][1], mark.points[1][1])] }] : [])
}
export function brushSummary(points: [number, number][]) {
  const xs = points.map(([x]) => x), ys = points.map(([, y]) => y)
  return `圈画已记录 · 范围 (${Math.min(...xs)}, ${Math.min(...ys)})–(${Math.max(...xs)}, ${Math.max(...ys)}) · ${points.length} 个点`
}
function path(mark: Mark) {
  const [first, last] = [mark.points[0], mark.points[mark.points.length - 1]]
  const line = mark.points.map((point, index) => `${index ? 'L' : 'M'}${point.join(' ')}`).join(' ')
  if (mark.kind !== 'arrow') return line
  const angle = Math.atan2(last[1] - first[1], last[0] - first[0])
  return `${line} ${[-0.5, 0.5].map((delta) => `M${last.join(' ')} L${last[0] - 30 * Math.cos(angle + delta)} ${last[1] - 30 * Math.sin(angle + delta)}`).join(' ')}`
}
export function ReferenceMarkModal({ asset, initial, accessToken, onDraft, onSave, onClose, onCancel, onMediaRefresh }: {
  onMediaRefresh: MediaRefreshHandler
  asset: { id: string; name: string; preview_url?: string }; initial: Mark[]; accessToken?: string | null
  onDraft: (marks: Mark[]) => void; onSave: (marks: Mark[], file?: File) => Promise<void>; onClose: () => void; onCancel: () => void
}) {
  const [marks, setMarks] = useState(initial)
  const [tool, setTool] = useState<Mark['kind']>('box')
  const [active, setActive] = useState<Mark | null>(null)
  const current = useRef<Mark | null>(null)
  const [ratio, setRatio] = useState(1)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [dirty, setDirty] = useState(false)
  const imageRef = useRef<HTMLImageElement>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [imageError, setImageError] = useState('')
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia, resetMediaRefresh } = useMediaRefreshOnce(userApi.imageAssetUrl(asset.preview_url ?? '', accessToken), onMediaRefresh, undefined, true)
  async function retryImage(manual = false) {
    setLoaded(false); setRefreshing(true); setImageError('')
    if (manual) resetMediaRefresh()
    const refreshed = await refreshFailedMedia()
    if (!refreshed) setImageError('参考图片加载失败。请点击刷新图片重试；标记草稿会保留。')
    setRefreshing(false)
  }
  function change(next: Mark[]) { setMarks(next); onDraft(next); setDirty(true); setError('') }
  async function save() {
    setSaving(true); setError('')
    try {
      let file: File | undefined
      const strokes = marks.filter((mark) => mark.kind === 'brush' || mark.kind === 'arrow')
      if (strokes.length) {
        const image = imageRef.current
        if (!image?.naturalWidth) throw new Error('参考图片尚未加载')
        const output = document.createElement('canvas'); output.width = image.naturalWidth; output.height = image.naturalHeight
        const context = output.getContext('2d'); if (!context) throw new Error('浏览器无法创建标记图片')
        context.drawImage(image, 0, 0)
        context.scale(output.width / 999, output.height / 999)
        context.strokeStyle = '#f43f5e'; context.lineWidth = 6; context.lineCap = 'round'; context.lineJoin = 'round'
        strokes.forEach((mark) => context.stroke(new Path2D(path(mark))))
        const blob = await new Promise<Blob>((resolve, reject) => output.toBlob((value) => value ? resolve(value) : reject(new Error('导出标记图失败')), 'image/png'))
        file = new File([blob], 'marked-reference.png', { type: 'image/png' })
      }
      await onSave(marks, file)
      onClose()
    } catch (reason) { setError(reason instanceof Error ? reason.message : '保存失败，标记草稿已保留') } finally { setSaving(false) }
  }
  const visible = active ? [...marks, active] : marks
  return <Modal title={`标记参考图 · ${asset.name}`} className="reference-mark-modal" onClose={() => { if (!saving) onClose() }}>
    <div className="reference-mark-toolbar" role="group" aria-label="标记工具">{(['box', 'point', 'brush', 'arrow'] as const).map((kind) => <button className="u-btn" type="button" key={kind} aria-pressed={tool === kind} disabled={saving} onClick={() => setTool(kind)}>{{ box: '框选', point: '标点', brush: '画笔 / 圈画', arrow: '箭头' }[kind]}</button>)}</div>
    <div className="reference-mark-stage"><div className="reference-mark-image" style={{ aspectRatio: ratio, width: `min(100%, 600px, ${48 * ratio}dvh)` }}>
      <img key={mediaRetryKey} ref={imageRef} crossOrigin="anonymous" src={currentSrc} alt={asset.name} draggable={false} onLoad={(event) => { setRatio(event.currentTarget.naturalWidth / event.currentTarget.naturalHeight); setLoaded(true); markMediaLoaded(); setImageError('') }} onError={() => { void retryImage() }} />
      <svg viewBox="0 0 999 999" preserveAspectRatio="none" aria-label="参考图标记画布" onPointerDown={(event) => {
        if (saving || !loaded) return
        const r = event.currentTarget.getBoundingClientRect(); const point = normalizeNativeImagePoint(event.clientX - r.left, event.clientY - r.top, r.width, r.height)
        current.current = { kind: tool, points: [point, point] }; setActive(current.current); event.currentTarget.setPointerCapture(event.pointerId)
      }} onPointerMove={(event) => {
        if (!current.current) return
        const r = event.currentTarget.getBoundingClientRect(); const point = normalizeNativeImagePoint(event.clientX - r.left, event.clientY - r.top, r.width, r.height)
        current.current = { ...current.current, points: tool === 'brush' ? [...current.current.points, point] : [current.current.points[0], point] }; setActive(current.current)
      }} onPointerUp={() => { const mark = current.current; current.current = null; setActive(null); if (mark) change([...marks, mark]) }} onPointerCancel={() => { current.current = null; setActive(null) }}>
        {visible.map((mark, index) => mark.kind === 'point' ? <circle key={index} cx={mark.points[0][0]} cy={mark.points[0][1]} r="10" /> : mark.kind === 'box' ? <rect key={index} x={Math.min(mark.points[0][0], mark.points[1][0])} y={Math.min(mark.points[0][1], mark.points[1][1])} width={Math.abs(mark.points[0][0] - mark.points[1][0])} height={Math.abs(mark.points[0][1] - mark.points[1][1])} /> : <path key={index} d={path(mark)} />)}
      </svg>
    </div></div>
    <div className="reference-mark-refresh"><button type="button" className="u-btn" disabled={saving || refreshing} onClick={() => void retryImage(true)}>{refreshing ? '正在刷新图片…' : '刷新图片'}</button>{imageError ? <span role="alert">{imageError}</span> : null}</div>
    <div className="reference-mark-status" role="status">{marks.length} 个标记 · {saving ? '正在上传并等待资产完成…' : error ? '保存失败 · 草稿已保留' : dirty ? '未保存' : '已保存 / 无更改'}</div>
    <ol className="reference-mark-coordinates">{marks.map((mark, index) => <li key={index}>{index + 1}. {{ box: '框', point: '点', brush: '画笔', arrow: '箭头' }[mark.kind]} · {mark.kind === 'brush' ? brushSummary(mark.points) : `${mark.points[0].join(', ')}${mark.kind !== 'point' ? ` → ${mark.points[mark.points.length - 1].join(', ')}` : ''}`}</li>)}</ol>
    <p className="u-meta">坐标范围 0–999。框和点用于编辑指令；画笔与箭头会合成为参考图片，上传完成后替换当前引用。</p>
    {error ? <p role="alert">{error}</p> : null}
    <div className="reference-mark-actions"><button type="button" className="u-btn" disabled={saving || !marks.length} onClick={() => change(marks.slice(0, -1))}>撤销</button><button type="button" className="u-btn" disabled={saving || !marks.length} onClick={() => change([])}>一键清空所有标记</button><button type="button" className="u-btn" disabled={saving} onClick={onCancel}>取消并放弃更改</button><button type="button" className="u-btn u-btn--primary" disabled={saving || !loaded} onClick={() => void save()}>{saving ? '保存中…' : '保存标记'}</button></div>
  </Modal>
}
