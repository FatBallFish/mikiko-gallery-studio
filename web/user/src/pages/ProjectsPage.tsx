import { type FormEvent, useMemo, useState } from 'react'
import type { Project } from '../../../shared/api-types'
import { ApiError } from '../../../shared/http-client'
import { EmptyState, ErrorState, Modal, useApp } from '../components'
import { useProjects } from '../ProjectContext'
import { Check, Edit, FolderKanban, Plus, RefreshCw, Trash2 } from '../ui/icons'
import { errorMessage } from '../useApiResource'

type DeleteState = { project: Project; requiresTransfer: boolean; taskCount: number; assetCount: number }

export function ProjectsPage() {
  const app = useApp()
  const projects = useProjects()
  const [createOpen, setCreateOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<Project | null>(null)
  const [deleteState, setDeleteState] = useState<DeleteState | null>(null)
  const [nameDraft, setNameDraft] = useState('')
  const [targetProjectID, setTargetProjectID] = useState('')
  const [busy, setBusy] = useState(false)

  const transferTargets = useMemo(() => projects.projects.filter((project) => (
    project.id !== deleteState?.project.id && project.status === 'active'
  )), [deleteState?.project.id, projects.projects])

  function normalizedName() {
    return nameDraft.trim().replace(/\s+/g, ' ')
  }

  const nameValid = normalizedName().length > 0 && normalizedName().length <= 128

  function openCreate() {
    setNameDraft('')
    setCreateOpen(true)
  }

  function openRename(project: Project) {
    if (project.is_default) return
    setNameDraft(project.name)
    setRenameTarget(project)
  }

  function openDelete(project: Project, counts?: { tasks?: number; assets?: number }) {
    if (project.is_default) return
    const taskCount = Number(counts?.tasks ?? project.task_count ?? 0)
    const assetCount = Number(counts?.assets ?? project.asset_count ?? 0)
    const requiresTransfer = taskCount > 0 || assetCount > 0
    const defaultTarget = projects.projects.find((candidate) => candidate.is_default && candidate.id !== project.id)
      ?? projects.projects.find((candidate) => candidate.id !== project.id)
    setTargetProjectID(requiresTransfer ? defaultTarget?.id ?? '' : '')
    setDeleteState({ project, requiresTransfer, taskCount, assetCount })
  }

  async function create(event: FormEvent) {
    event.preventDefault()
    const name = normalizedName()
    if (!name || name.length > 128) return
    setBusy(true)
    try {
      await projects.createProject(name)
      setCreateOpen(false)
      app.notify('success', '项目已创建')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function rename(event: FormEvent) {
    event.preventDefault()
    if (!renameTarget) return
    const name = normalizedName()
    if (!name || name.length > 128) return
    setBusy(true)
    try {
      await projects.renameProject(renameTarget, name)
      setRenameTarget(null)
      app.notify('success', '项目名称已更新')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
      if (caught instanceof ApiError && caught.code === 'project_changed') await projects.refreshProjects().catch(() => undefined)
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!deleteState || (deleteState.requiresTransfer && !targetProjectID)) return
    setBusy(true)
    try {
      await projects.deleteProject(deleteState.project, deleteState.requiresTransfer ? targetProjectID : undefined)
      setDeleteState(null)
      app.notify('success', deleteState.requiresTransfer ? '资产已转移，项目已删除' : '项目已删除')
    } catch (caught) {
      if (caught instanceof ApiError && caught.code === 'project_not_empty') {
        const counts = caught.details?.counts as { tasks?: number; assets?: number } | undefined
        openDelete(deleteState.project, counts)
      } else {
        app.notify('error', errorMessage(caught))
        if (caught instanceof ApiError && caught.code === 'project_changed') await projects.refreshProjects().catch(() => undefined)
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="acct-page">
      <div className="u-page-head">
        <div className="u-page-head__copy">
          <span className="u-eyebrow">Projects</span>
          <h1 className="u-h1">项目</h1>
          <p className="u-sub">项目是资产、任务与画布的归属边界。切换项目不会丢失其他项目的排队任务。</p>
        </div>
        <div className="u-page-head__actions">
          <button
            className="u-icon-btn"
            type="button"
            title="刷新项目"
            aria-label="刷新项目"
            disabled={projects.loading}
            onClick={() => void projects.refreshProjects().catch(() => undefined)}
          >
            <RefreshCw size={18} strokeWidth={1.6} aria-hidden="true" />
          </button>
          <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={openCreate}>
            <Plus size={16} strokeWidth={1.8} aria-hidden="true" />
            新建项目
          </button>
        </div>
      </div>

      {projects.error && !projects.projects.length ? <ErrorState message={projects.error} onRetry={() => void projects.refreshProjects().catch(() => undefined)} /> : null}
      {!projects.loading && !projects.projects.length ? <EmptyState title="暂无项目" detail="创建第一个项目，开始整理资产与任务。" action={<button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={openCreate}>新建项目</button>} /> : null}

      {projects.projects.length ? (
        <div className="u-plate" aria-busy={projects.loading}>
          <div className="u-list" style={{ padding: '0 18px' }}>
            {projects.projects.map((project) => {
              const current = projects.selectedProjectID === project.id
              return (
                <div className="u-row acct-project-row" key={project.id}>
                  <div className="u-row__main">
                    <span
                      className="u-row__icon"
                      aria-hidden="true"
                      style={current ? { color: 'var(--accent)', background: 'var(--accent-soft)' } : undefined}
                    >
                      <FolderKanban size={18} strokeWidth={1.6} />
                    </span>
                    <div className="u-row__copy acct-project-copy">
                      <strong>
                        {project.name}
                        {current ? <span className="u-chip u-chip--accent" style={{ marginLeft: 8, height: 19, fontSize: 9 }}>当前</span> : null}
                        {project.is_default ? <span className="u-chip" style={{ marginLeft: 6, height: 19, fontSize: 9 }}>默认</span> : null}
                      </strong>
                      <span>{project.asset_count ?? 0} 项资产 · {project.task_count ?? 0} 个任务</span>
                    </div>
                  </div>
                  <div className="u-row__actions acct-project-actions">
                    {current ? (
                      <span className="u-status u-status--ok"><span className="u-dot" aria-hidden="true" />使用中</span>
                    ) : (
                      <button className="u-btn u-btn--sm" type="button" onClick={() => projects.selectProject(project.id)}>
                        <Check size={14} strokeWidth={1.7} aria-hidden="true" />
                        切换
                      </button>
                    )}
                    <button
                      className="u-icon-btn"
                      type="button"
                      title={project.is_default ? '默认项目不可重命名' : '重命名'}
                      aria-label={`重命名项目 ${project.name}`}
                      disabled={project.is_default}
                      onClick={() => openRename(project)}
                    >
                      <Edit size={16} strokeWidth={1.6} aria-hidden="true" />
                    </button>
                    <button
                      className="u-icon-btn u-icon-btn--danger"
                      type="button"
                      title={project.is_default ? '默认项目不可删除' : '删除'}
                      aria-label={`删除项目 ${project.name}`}
                      disabled={project.is_default}
                      onClick={() => openDelete(project)}
                    >
                      <Trash2 size={16} strokeWidth={1.6} aria-hidden="true" />
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      ) : null}

      {projects.loading && !projects.projects.length ? (
        <div className="u-plate" aria-hidden="true">
          <div className="u-list" style={{ padding: '0 18px' }}>
            {Array.from({ length: 3 }).map((_, index) => (
              <div className="u-row" key={index}>
                <div className="u-row__main">
                  <span className="pg-skeleton size-9 rounded-xl" />
                  <div className="u-row__copy" style={{ flex: 1 }}>
                    <span className="pg-skeleton h-4 w-40 rounded-xl" />
                    <span className="pg-skeleton mt-1 h-3 w-56 rounded-xl" />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {projects.projects.length ? (
        <div className="u-empty" style={{ marginTop: 20 }}>
          <span className="u-empty__icon" aria-hidden="true"><Plus size={22} strokeWidth={1.5} /></span>
          <h3>再建一个项目</h3>
          <p>把一个长期主题的所有资产、任务与画布收在同一个边界里，公开与私有设置互不影响。</p>
          <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={openCreate}>新建项目</button>
        </div>
      ) : null}

      {createOpen ? (
        <Modal title="新建项目" onClose={() => setCreateOpen(false)} className="acct-modal">
          <form style={{ display: 'grid', gap: 14 }} onSubmit={create}>
            <label className="u-field">
              <span className="u-label">项目名称 <span className="u-label__hint">最多 128 个字符</span></span>
              <input className="u-input" value={nameDraft} maxLength={128} autoFocus placeholder="例如：品牌视觉 · 秋冬" onChange={(event) => setNameDraft(event.target.value)} />
            </label>
            {!nameValid && nameDraft.length ? <span className="u-field__error" role="alert">项目名称不能为空且不能超过 128 个字符</span> : null}
            <span className="u-meta">创建后会自动切换到此项目。</span>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
              <button className="u-btn" type="button" onClick={() => setCreateOpen(false)}>取消</button>
              <button className="u-btn u-btn--primary" type="submit" disabled={!nameValid || busy}>{busy ? '创建中...' : '创建项目'}</button>
            </div>
          </form>
        </Modal>
      ) : null}

      {renameTarget ? (
        <Modal title="重命名项目" onClose={() => setRenameTarget(null)} className="acct-modal">
          <form style={{ display: 'grid', gap: 14 }} onSubmit={rename}>
            <label className="u-field">
              <span className="u-label">项目名称 <span className="u-label__hint">最多 128 个字符</span></span>
              <input className="u-input" value={nameDraft} maxLength={128} autoFocus onChange={(event) => setNameDraft(event.target.value)} />
            </label>
            {!nameValid && nameDraft.length ? <span className="u-field__error" role="alert">项目名称不能为空且不能超过 128 个字符</span> : null}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
              <button className="u-btn" type="button" onClick={() => setRenameTarget(null)}>取消</button>
              <button className="u-btn u-btn--primary" type="submit" disabled={!nameValid || normalizedName() === renameTarget.name || busy}>{busy ? '保存中...' : '保存'}</button>
            </div>
          </form>
        </Modal>
      ) : null}

      {deleteState ? (
        <Modal title={deleteState.requiresTransfer ? '转移并删除项目' : '删除项目'} onClose={() => setDeleteState(null)} className="acct-modal">
          <div style={{ display: 'grid', gap: 14 }}>
            <p className="u-sub" style={{ margin: 0 }}>确认删除「{deleteState.project.name}」？此操作不可撤销。</p>
            {deleteState.requiresTransfer ? (
              <>
                <div className="u-banner" role="note">项目内有 {deleteState.taskCount} 个任务、{deleteState.assetCount} 个资产，删除前将全部转移。</div>
                <label className="u-field">
                  <span className="u-label">转移到</span>
                  <select className="u-select" value={targetProjectID} onChange={(event) => setTargetProjectID(event.target.value)}>
                    {transferTargets.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
                  </select>
                </label>
              </>
            ) : null}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
              <button className="u-btn" type="button" disabled={busy} onClick={() => setDeleteState(null)}>取消</button>
              <button className="u-btn u-btn--danger" type="button" disabled={busy || (deleteState.requiresTransfer && !targetProjectID)} onClick={() => void remove()}>
                {busy ? '处理中...' : deleteState.requiresTransfer ? '转移并删除' : '删除'}
              </button>
            </div>
          </div>
        </Modal>
      ) : null}
    </div>
  )
}