import { useCallback, useEffect, useMemo, useState } from 'react'
import { Copy, FilePlus2, Film, Image, MoreHorizontal, Pencil, RefreshCw, Search, Sparkles, Trash2 } from 'lucide-react'
import type { CreativeCanvas } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Button, EmptyState, useApp } from '../../components'
import { ProjectSelector, useProjects } from '../../ProjectContext'
import { errorMessage } from '../../useApiResource'
import './canvas.css'

type Template = 'blank' | 'image_exploration' | 'image_to_video'
const templates: Array<{ template: Template; title: string; detail: string; icon: React.ReactNode }> = [
  { template: 'blank', title: '空白画布', detail: '从任意节点开始', icon: <FilePlus2 size={18} /> },
  { template: 'image_exploration', title: '图片探索', detail: '提示词到图片生成', icon: <Image size={18} /> },
  { template: 'image_to_video', title: '图片转视频', detail: '首帧与视频生成', icon: <Film size={18} /> },
]

export function CanvasListPage({ onOpen }: { onOpen: (canvasID: string) => void }) {
  const app = useApp()
  const projects = useProjects()
  const [items, setItems] = useState<CreativeCanvas[]>([])
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [menuID, setMenuID] = useState('')

  const load = useCallback(async () => {
    if (!projects.selectedProjectID) return
    setLoading(true); setError('')
    try { setItems(await userApi.listCanvases({ project_id: projects.selectedProjectID, search: query.trim() || undefined })) }
    catch (caught) { setError(errorMessage(caught)) }
    finally { setLoading(false) }
  }, [projects.selectedProjectID, query])
  useEffect(() => { const timer = window.setTimeout(() => void load(), 180); return () => window.clearTimeout(timer) }, [load])

  const activeProject = useMemo(() => projects.projects.find((project) => project.id === projects.selectedProjectID), [projects.projects, projects.selectedProjectID])
  async function create(template: Template) {
    if (!projects.selectedProjectID) return
    setBusy(`create:${template}`)
    try {
      const created = await userApi.createCanvas({ project_id: projects.selectedProjectID, name: `${templates.find((item) => item.template === template)?.title ?? '创意画布'} ${new Date().toLocaleDateString('zh-CN')}`, template })
      onOpen(created.id)
    } catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusy('') }
  }
  async function rename(item: CreativeCanvas) {
    const name = window.prompt('画布名称', item.name)?.trim()
    if (!name || name === item.name) return
    setBusy(item.id)
    try { const updated = await userApi.renameCanvas(item.id, name, item.metadata_version); setItems((current) => current.map((entry) => entry.id === item.id ? updated : entry)) }
    catch (caught) { app.notify('error', errorMessage(caught)); await load() } finally { setBusy(''); setMenuID('') }
  }
  async function duplicate(item: CreativeCanvas) {
    setBusy(item.id)
    try { const copy = await userApi.duplicateCanvas(item.id, { name: `${item.name} 副本` }); setItems((current) => [copy, ...current]) }
    catch (caught) { app.notify('error', errorMessage(caught)) } finally { setBusy(''); setMenuID('') }
  }
  async function transfer(item: CreativeCanvas) {
    if (item.running_task_count > 0) { app.notify('error', '画布有运行中的任务，暂时不能转移'); return }
    const candidates = projects.projects.filter((project) => project.id !== item.project_id && project.status === 'active')
    const targetName = window.prompt(`输入目标项目名称：${candidates.map((project) => project.name).join('、')}`)?.trim()
    const target = candidates.find((project) => project.name === targetName)
    if (!target) return
    setBusy(item.id)
    try { await userApi.transferCanvas(item.id, target.id, item.metadata_version); setItems((current) => current.filter((entry) => entry.id !== item.id)); app.notify('success', `已转移到 ${target.name}`) }
    catch (caught) { app.notify('error', errorMessage(caught)); await load() } finally { setBusy(''); setMenuID('') }
  }
  async function remove(item: CreativeCanvas) {
    if (item.running_task_count > 0) { app.notify('error', '画布有运行中的任务，暂时不能删除'); return }
    if (!window.confirm(`删除画布“${item.name}”？画布中的资产不会被删除。`)) return
    setBusy(item.id)
    try { await userApi.deleteCanvas(item.id, item.metadata_version); setItems((current) => current.filter((entry) => entry.id !== item.id)) }
    catch (caught) { app.notify('error', errorMessage(caught)); await load() } finally { setBusy(''); setMenuID('') }
  }

  return <main className="canvas-list-page">
    <header className="canvas-list-header"><div><span>CREATIVE CANVAS</span><h1>创意画布</h1><p>{activeProject?.name ?? '当前项目'}</p></div><div><ProjectSelector /><button type="button" title="刷新画布" aria-label="刷新画布" disabled={loading} onClick={() => void load()}><RefreshCw className={loading ? 'animate-spin' : undefined} size={18} /></button></div></header>
    <section className="canvas-template-row" aria-label="新建画布">{templates.map((item) => <button key={item.template} type="button" disabled={busy.startsWith('create:')} onClick={() => void create(item.template)}>{item.icon}<span><strong>{item.title}</strong><small>{item.detail}</small></span><Sparkles size={14} /></button>)}</section>
    <div className="canvas-list-toolbar"><label><Search size={16} /><input value={query} placeholder="搜索画布" onChange={(event) => setQuery(event.target.value)} /></label><span>{items.length} 个画布</span></div>
    {error ? <div className="canvas-list-error" role="alert"><span>{error}</span><Button tone="ghost" onClick={() => void load()}>重试</Button></div> : null}
    {!loading && !error && !items.length ? <EmptyState title="当前项目还没有画布" detail="" action={<Button onClick={() => void create('blank')}>新建空白画布</Button>} /> : null}
    <section className="canvas-list-grid" aria-busy={loading}>{items.map((item) => <article key={item.id} className="canvas-list-card">
      <button className="canvas-list-preview" type="button" onClick={() => onOpen(item.id)}><CanvasPreview item={item} /></button>
      <div className="canvas-list-meta"><button type="button" onClick={() => onOpen(item.id)}><strong>{item.name}</strong><span>{item.node_count} 节点 · {item.running_task_count ? `${item.running_task_count} 个任务运行中` : formatUpdated(item.updated_at)}</span></button><button type="button" title="更多操作" onClick={() => setMenuID((current) => current === item.id ? '' : item.id)}><MoreHorizontal size={17} /></button></div>
      {menuID === item.id ? <div className="canvas-list-menu"><button type="button" disabled={busy === item.id} onClick={() => void rename(item)}><Pencil size={15} />重命名</button><button type="button" disabled={busy === item.id} onClick={() => void duplicate(item)}><Copy size={15} />创建副本</button><button type="button" disabled={busy === item.id || item.running_task_count > 0} onClick={() => void transfer(item)}>转移项目</button><button type="button" className="is-danger" disabled={busy === item.id || item.running_task_count > 0} onClick={() => void remove(item)}><Trash2 size={15} />删除</button></div> : null}
    </article>)}</section>
  </main>
}

function CanvasPreview({ item }: { item: CreativeCanvas }) {
  const nodes = item.document.nodes.slice(0, 40)
  if (!nodes.length) return <span className="canvas-empty-preview"><FilePlus2 size={26} />空白画布</span>
  const minX = Math.min(...nodes.map((node) => node.position.x)); const minY = Math.min(...nodes.map((node) => node.position.y))
  const maxX = Math.max(...nodes.map((node) => node.position.x + node.size.width)); const maxY = Math.max(...nodes.map((node) => node.position.y + node.size.height))
  const width = Math.max(1, maxX - minX); const height = Math.max(1, maxY - minY)
  return <span className="canvas-preview-world">{nodes.map((node) => <i key={node.id} data-type={node.type} style={{ left: `${((node.position.x - minX) / width) * 82 + 9}%`, top: `${((node.position.y - minY) / height) * 72 + 14}%`, width: `${Math.max(4, (node.size.width / width) * 82)}%`, height: `${Math.max(4, (node.size.height / height) * 72)}%` }} />)}</span>
}
function formatUpdated(value?: string) { if (!value) return '尚未保存'; return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) }
