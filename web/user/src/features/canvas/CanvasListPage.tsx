import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { Copy, Edit, FilePlus2, Film, Image as ImageIcon, MoreHorizontal, RefreshCw, Search, Sparkles, Trash2 } from 'lucide-react'
import type { CreativeCanvas } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { EmptyState, ErrorState, Modal, useApp } from '../../components'
import { ProjectSelector, useProjects } from '../../ProjectContext'
import { errorMessage } from '../../useApiResource'
import './canvas.css'

type Template = 'blank' | 'image_exploration' | 'image_to_video'
const templates: Array<{ template: Template; title: string; detail: string; icon: React.ReactNode }> = [
  { template: 'blank', title: '空白画布', detail: '添加图片、视频或便签开始创作', icon: <FilePlus2 size={18} /> },
  { template: 'image_exploration', title: '图片探索', detail: '4 个独立图片节点，多方向探索', icon: <ImageIcon size={18} /> },
  { template: 'image_to_video', title: '图片转视频', detail: '图片参考与视频生成工作区', icon: <Film size={18} /> },
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
  const [renameTarget, setRenameTarget] = useState<CreativeCanvas | null>(null)
  const [renameName, setRenameName] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<CreativeCanvas | null>(null)

  const load = useCallback(async () => {
    if (!projects.selectedProjectID) return
    setLoading(true)
    setError('')
    try {
      setItems(await userApi.listCanvases({ project_id: projects.selectedProjectID, search: query.trim() || undefined }))
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setLoading(false)
    }
  }, [projects.selectedProjectID, query])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 180)
    return () => window.clearTimeout(timer)
  }, [load])

  const activeProject = useMemo(() => projects.projects.find((project) => project.id === projects.selectedProjectID), [projects.projects, projects.selectedProjectID])

  async function create(template: Template) {
    if (!projects.selectedProjectID) return
    setBusy(`create:${template}`)
    try {
      const created = await userApi.createCanvas({ project_id: projects.selectedProjectID, name: `${templates.find((item) => item.template === template)?.title ?? '创意画布'} ${new Date().toLocaleDateString('zh-CN')}`, template })
      onOpen(created.id)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy('')
    }
  }

  function openRename(item: CreativeCanvas) {
    setMenuID('')
    setRenameTarget(item)
    setRenameName(item.name)
  }

  async function rename(event: FormEvent) {
    event.preventDefault()
    if (!renameTarget) return
    const name = renameName.trim()
    if (!name || name === renameTarget.name) return
    setBusy(renameTarget.id)
    try {
      const updated = await userApi.renameCanvas(renameTarget.id, name, renameTarget.metadata_version)
      setItems((current) => current.map((entry) => entry.id === renameTarget.id ? updated : entry))
      app.notify('success', '画布已重命名')
      setRenameTarget(null)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
      await load()
    } finally {
      setBusy('')
      setMenuID('')
    }
  }

  async function duplicate(item: CreativeCanvas) {
    setBusy(item.id)
    try {
      const copy = await userApi.duplicateCanvas(item.id, { name: `${item.name} 副本` })
      setItems((current) => [copy, ...current])
      app.notify('success', '画布副本已创建')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy('')
      setMenuID('')
    }
  }

  async function transfer(item: CreativeCanvas) {
    if (item.running_task_count > 0) {
      app.notify('error', '画布有运行中的任务，暂时不能转移')
      return
    }
    const candidates = projects.projects.filter((project) => project.id !== item.project_id && project.status === 'active')
    const targetName = window.prompt(`输入目标项目名称：${candidates.map((project) => project.name).join('、')}`)?.trim()
    const target = candidates.find((project) => project.name === targetName)
    if (!target) return
    setBusy(item.id)
    try {
      await userApi.transferCanvas(item.id, target.id, item.metadata_version)
      setItems((current) => current.filter((entry) => entry.id !== item.id))
      app.notify('success', `已转移到 ${target.name}`)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
      await load()
    } finally {
      setBusy('')
      setMenuID('')
    }
  }

  async function remove() {
    if (!deleteTarget) return
    if (deleteTarget.running_task_count > 0) {
      app.notify('error', '画布有运行中的任务，暂时不能删除')
      setDeleteTarget(null)
      return
    }
    setBusy(deleteTarget.id)
    try {
      await userApi.deleteCanvas(deleteTarget.id, deleteTarget.metadata_version)
      setItems((current) => current.filter((entry) => entry.id !== deleteTarget.id))
      app.notify('success', '画布已删除')
      setDeleteTarget(null)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
      await load()
    } finally {
      setBusy('')
      setMenuID('')
    }
  }

  return (
    <main className="canvas-list-page acct-page">
      <div className="u-page-head">
        <div className="u-page-head__copy">
          <span className="u-eyebrow">Creative Canvas</span>
          <h1 className="u-h1">创意画布</h1>
          <p className="u-sub">把图片、视频与参考素材放在同一张无限画布上。选中空节点即可展开生成操作台，生成结果直接留在节点中。</p>
        </div>
        <div className="u-page-head__actions">
          <ProjectSelector />
          <button className="u-icon-btn" type="button" title="刷新画布" aria-label="刷新画布" disabled={loading} onClick={() => void load()}>
            <RefreshCw size={17} strokeWidth={1.6} className={loading ? 'animate-spin' : undefined} aria-hidden="true" />
          </button>
          <button className="u-btn u-btn--primary u-btn--sm" type="button" disabled={!projects.selectedProjectID || busy.startsWith('create:')} onClick={() => void create('blank')}>
            新建画布
          </button>
        </div>
      </div>

      <div className="cv-templates" aria-label="新建画布">
        {templates.map((item) => (
          <button key={item.template} className="cv-template" type="button" disabled={!projects.selectedProjectID || busy.startsWith('create:')} onClick={() => void create(item.template)}>
            <i aria-hidden="true">{item.icon}</i>
            <div><strong>{item.title}</strong><span>{item.detail}</span></div>
            <Sparkles size={14} strokeWidth={1.5} aria-hidden="true" />
          </button>
        ))}
      </div>

      <div className="u-section-head">
        <div style={{ display: 'grid', gap: 5, minWidth: 0 }}>
          <h2 className="u-h3">我的画布</h2>
          <span className="u-meta">{items.length} 张画布 · 当前项目：{activeProject?.name ?? '未选择'}</span>
        </div>
        <div className="u-search" style={{ maxWidth: 260, flex: '0 1 260px' }}>
          <Search size={15} strokeWidth={1.6} aria-hidden="true" />
          <input value={query} placeholder="搜索画布" aria-label="搜索画布" onChange={(event) => setQuery(event.target.value)} />
        </div>
      </div>

      {error ? <ErrorState message={error} onRetry={() => void load()} /> : null}
      {loading && !items.length ? <CanvasGridSkeleton /> : null}
      {!loading && !error && !items.length ? (
        <EmptyState
          title={projects.selectedProjectID ? '当前项目还没有画布' : '请先选择项目'}
          detail={projects.selectedProjectID ? '从模板创建画布，或新建一张空白画布。' : '画布需要归属到一个项目，请先在顶栏切换项目。'}
          action={projects.selectedProjectID ? <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => void create('blank')}>新建空白画布</button> : <ProjectSelector />}
        />
      ) : null}

      {items.length ? (
        <div className="u-grid u-grid--projects" aria-busy={loading}>
          {items.map((item) => (
            <div className="acct-canvas-card" key={item.id}>
              <article className="u-card">
                <button className="cv-card__preview canvas-list-preview" type="button" onClick={() => onOpen(item.id)} aria-label={`打开 ${item.name}`}>
                  <CanvasPreview item={item} />
                </button>
                <div className="cv-card__foot">
                  <button className="acct-canvas-title" type="button" onClick={() => onOpen(item.id)}>
                    <strong>{item.name}</strong>
                    <span className="u-meta">
                      {item.node_count} 个节点 · {item.running_task_count ? `${item.running_task_count} 个任务运行中` : formatUpdated(item.updated_at)}
                    </span>
                  </button>
                  <div style={{ display: 'flex', gap: 5, flex: 'none' }}>
                    <button className="u-icon-btn" type="button" aria-label="重命名" title="重命名" disabled={busy === item.id} onClick={() => openRename(item)}>
                      <Edit size={15} strokeWidth={1.6} aria-hidden="true" />
                    </button>
                    <button className="u-icon-btn" type="button" aria-label="更多操作" title="更多操作" aria-haspopup="menu" aria-expanded={menuID === item.id} onClick={() => setMenuID((current) => current === item.id ? '' : item.id)}>
                      <MoreHorizontal size={16} strokeWidth={1.6} aria-hidden="true" />
                    </button>
                  </div>
                </div>
              </article>
              {menuID === item.id ? (
                <>
                  <div className="acct-canvas-menu-backdrop" aria-hidden="true" onClick={() => setMenuID('')} />
                  <div className="canvas-list-menu" role="menu" aria-label={`${item.name} 操作`}>
                    <button type="button" role="menuitem" disabled={busy === item.id} onClick={() => openRename(item)}><Edit size={15} />重命名</button>
                    <button type="button" role="menuitem" disabled={busy === item.id} onClick={() => void duplicate(item)}><Copy size={15} />创建副本</button>
                    <button type="button" role="menuitem" disabled={busy === item.id || item.running_task_count > 0} onClick={() => void transfer(item)}>转移项目</button>
                    <button type="button" role="menuitem" className="is-danger" disabled={busy === item.id || item.running_task_count > 0} onClick={() => { setMenuID(''); setDeleteTarget(item) }}><Trash2 size={15} />删除</button>
                  </div>
                </>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}

      {renameTarget ? (
        <Modal title="重命名画布" onClose={() => setRenameTarget(null)} className="acct-modal">
          <form style={{ display: 'grid', gap: 14 }} onSubmit={rename}>
            <label className="u-field">
              <span className="u-label">画布名称</span>
              <input className="u-input" value={renameName} maxLength={128} autoFocus onChange={(event) => setRenameName(event.target.value)} />
            </label>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
              <button className="u-btn" type="button" onClick={() => setRenameTarget(null)}>取消</button>
              <button className="u-btn u-btn--primary" type="submit" disabled={!renameName.trim() || renameName.trim() === renameTarget.name || busy === renameTarget.id}>
                {busy === renameTarget.id ? '保存中...' : '保存'}
              </button>
            </div>
          </form>
        </Modal>
      ) : null}

      {deleteTarget ? (
        <Modal title="删除画布" onClose={() => setDeleteTarget(null)} className="acct-modal">
          <div style={{ display: 'grid', gap: 14 }}>
            <p className="u-sub" style={{ margin: 0 }}>删除画布「{deleteTarget.name}」？画布中的资产不会被删除。</p>
            {deleteTarget.running_task_count > 0 ? <div className="u-banner" role="note">画布有运行中的任务，完成或取消后才能删除。</div> : null}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
              <button className="u-btn" type="button" onClick={() => setDeleteTarget(null)}>取消</button>
              <button className="u-btn u-btn--danger" type="button" disabled={busy === deleteTarget.id || deleteTarget.running_task_count > 0} onClick={() => void remove()}>
                {busy === deleteTarget.id ? '删除中...' : '删除画布'}
              </button>
            </div>
          </div>
        </Modal>
      ) : null}
    </main>
  )
}

function CanvasGridSkeleton() {
  return (
    <div className="u-grid u-grid--projects" aria-hidden="true">
      {Array.from({ length: 3 }).map((_, index) => (
        <div className="u-card" key={index}>
          <div className="pg-skeleton" style={{ aspectRatio: '16 / 10' }} />
          <div style={{ display: 'grid', gap: 6, padding: 13 }}>
            <div className="pg-skeleton h-4 w-2/3 rounded-xl" />
            <div className="pg-skeleton h-3 w-1/2 rounded-xl" />
          </div>
        </div>
      ))}
    </div>
  )
}

function CanvasPreview({ item }: { item: CreativeCanvas }) {
  const nodes = (Array.isArray(item.document.nodes) ? item.document.nodes : []).slice(0, 40)
  if (!nodes.length) return <span className="acct-canvas-empty"><FilePlus2 size={24} strokeWidth={1.4} aria-hidden="true" />空白画布</span>
  const minX = Math.min(...nodes.map((node) => node.position.x))
  const minY = Math.min(...nodes.map((node) => node.position.y))
  const maxX = Math.max(...nodes.map((node) => node.position.x + node.size.width))
  const maxY = Math.max(...nodes.map((node) => node.position.y + node.size.height))
  const width = Math.max(1, maxX - minX)
  const height = Math.max(1, maxY - minY)
  return (
    <span className="acct-canvas-preview-world">
      {nodes.map((node) => (
        <i
          key={node.id}
          className={canvasNodeClass(node.type)}
          style={{
            left: `${((node.position.x - minX) / width) * 82 + 9}%`,
            top: `${((node.position.y - minY) / height) * 72 + 14}%`,
            width: `${Math.max(4, (node.size.width / width) * 82)}%`,
            height: `${Math.max(4, (node.size.height / height) * 72)}%`,
          }}
        />
      ))}
    </span>
  )
}

function canvasNodeClass(type: string) {
  if (type === 'image_v2' || type === 'image' || type === 'image_generation') return 'cv-card__node cv-card__node--img'
  if (type === 'video_v2' || type === 'video' || type === 'video_generation') return 'cv-card__node cv-card__node--vid'
  if (type === 'note') return 'cv-card__node cv-card__node--note'
  return 'cv-card__node cv-card__node--txt'
}

function formatUpdated(value?: string) {
  if (!value) return '尚未保存'
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}