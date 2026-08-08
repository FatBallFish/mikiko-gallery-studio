import { type FormEvent, useMemo, useState } from 'react'
import type { Project } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { Button, EmptyState, ErrorState, Modal, useApp } from '../components'
import { useProjects } from '../ProjectContext'
import { Check, Edit, FolderKanban, LockKeyhole, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { errorMessage } from '../useApiResource'

const projectClasses = {
  page: 'w-full flex-1 px-4 py-6 sm:px-6 md:px-10 md:py-8',
  header: 'mb-8 flex flex-col gap-5 border-b border-[var(--border)] pb-7 sm:flex-row sm:items-end sm:justify-between',
  title: 'm-0 text-4xl font-black leading-none md:text-5xl',
  actions: 'flex flex-wrap gap-2',
  list: 'grid gap-3',
  row: 'grid min-w-0 gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center',
  identity: 'flex min-w-0 items-center gap-3',
  icon: 'grid size-10 shrink-0 place-items-center rounded-lg border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)]',
  name: 'flex min-w-0 flex-wrap items-center gap-2 text-base font-bold text-[var(--fg)]',
  defaultBadge: 'inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--bg)] px-2 py-0.5 text-[11px] font-semibold text-[var(--muted)]',
  meta: 'mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--muted)]',
  rowActions: 'flex shrink-0 gap-2',
  iconButton: 'grid size-10 place-items-center rounded-lg border border-[var(--border)] bg-[var(--bg)] text-[var(--fg)] transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-40',
  dangerButton: 'hover:border-[var(--accent-coral)] hover:text-[var(--accent-coral)]',
  form: 'grid gap-4',
  label: 'grid gap-2 text-sm font-semibold text-[var(--fg)]',
  input: 'h-11 w-full rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 text-sm text-[var(--fg)] outline-none focus:border-[var(--accent)]',
  dialogActions: 'flex justify-end gap-2 max-[420px]:flex-col-reverse',
  warning: 'rounded-lg border border-[color-mix(in_oklch,var(--accent-coral)_38%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_8%,transparent)] p-3 text-sm leading-6 text-[var(--fg)]',
}

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
    <div className={projectClasses.page}>
      <header className={projectClasses.header}>
        <h1 className={projectClasses.title}>项目</h1>
        <div className={projectClasses.actions}>
          <button className={projectClasses.iconButton} type="button" title="刷新项目" aria-label="刷新项目" disabled={projects.loading} onClick={() => void projects.refreshProjects().catch(() => undefined)}>
            <RefreshCw size={18} strokeWidth={1.6} aria-hidden="true" />
          </button>
          <Button type="button" onClick={openCreate}><Plus size={17} strokeWidth={1.8} aria-hidden="true" />新建项目</Button>
        </div>
      </header>

      {projects.error && !projects.projects.length ? <ErrorState message={projects.error} onRetry={() => void projects.refreshProjects().catch(() => undefined)} /> : null}
      {!projects.loading && !projects.projects.length ? <EmptyState title="暂无项目" detail="" action={<Button onClick={openCreate}>新建项目</Button>} /> : null}
      <div className={projectClasses.list} aria-busy={projects.loading}>
        {projects.projects.map((project) => (
          <section className={projectClasses.row} key={project.id}>
            <div className={projectClasses.identity}>
              <span className={projectClasses.icon} aria-hidden="true"><FolderKanban size={19} strokeWidth={1.6} /></span>
              <div className="min-w-0">
                <div className={projectClasses.name}>
                  <span className="truncate">{project.name}</span>
                  {project.is_default ? <span className={projectClasses.defaultBadge}><LockKeyhole size={12} />默认项目</span> : null}
                  {projects.selectedProjectID === project.id ? <span className={projectClasses.defaultBadge}><Check size={12} />当前</span> : null}
                </div>
                <div className={projectClasses.meta}>
                  <span>{project.task_count ?? 0} 个任务</span>
                  <span>{project.asset_count ?? 0} 个资产</span>
                </div>
              </div>
            </div>
            <div className={projectClasses.rowActions}>
              <button className={projectClasses.iconButton} type="button" title="切换到此项目" aria-label={`切换到项目 ${project.name}`} disabled={projects.selectedProjectID === project.id} onClick={() => projects.selectProject(project.id)}>
                <Check size={17} strokeWidth={1.7} aria-hidden="true" />
              </button>
              <button className={projectClasses.iconButton} type="button" title={project.is_default ? '默认项目不可更名' : '修改名称'} aria-label={`修改项目 ${project.name}`} disabled={project.is_default} onClick={() => openRename(project)}>
                <Edit size={17} strokeWidth={1.7} aria-hidden="true" />
              </button>
              <button className={cn(projectClasses.iconButton, projectClasses.dangerButton)} type="button" title={project.is_default ? '默认项目不可删除' : '删除项目'} aria-label={`删除项目 ${project.name}`} disabled={project.is_default} onClick={() => openDelete(project)}>
                <Trash2 size={17} strokeWidth={1.7} aria-hidden="true" />
              </button>
            </div>
          </section>
        ))}
      </div>

      {createOpen ? (
        <Modal title="新建项目" onClose={() => setCreateOpen(false)}>
          <form className={projectClasses.form} onSubmit={create}>
            <label className={projectClasses.label}>名称<input className={projectClasses.input} value={nameDraft} maxLength={128} autoFocus onChange={(event) => setNameDraft(event.target.value)} /></label>
            <div className={projectClasses.dialogActions}><Button tone="ghost" type="button" onClick={() => setCreateOpen(false)}>取消</Button><Button type="submit" busy={busy} disabled={!normalizedName()}>创建</Button></div>
          </form>
        </Modal>
      ) : null}

      {renameTarget ? (
        <Modal title="修改项目名称" onClose={() => setRenameTarget(null)}>
          <form className={projectClasses.form} onSubmit={rename}>
            <label className={projectClasses.label}>名称<input className={projectClasses.input} value={nameDraft} maxLength={128} autoFocus onChange={(event) => setNameDraft(event.target.value)} /></label>
            <div className={projectClasses.dialogActions}><Button tone="ghost" type="button" onClick={() => setRenameTarget(null)}>取消</Button><Button type="submit" busy={busy} disabled={!normalizedName() || normalizedName() === renameTarget.name}>保存</Button></div>
          </form>
        </Modal>
      ) : null}

      {deleteState ? (
        <Modal title={deleteState.requiresTransfer ? '转移并删除项目' : '删除项目'} onClose={() => setDeleteState(null)}>
          <div className={projectClasses.form}>
            <p className="m-0 text-sm leading-6 text-[var(--muted)]">确认删除「{deleteState.project.name}」？</p>
            {deleteState.requiresTransfer ? (
              <>
                <div className={projectClasses.warning}>项目内有 {deleteState.taskCount} 个任务、{deleteState.assetCount} 个资产，删除前将全部转移。</div>
                <label className={projectClasses.label}>转移到<select className={projectClasses.input} value={targetProjectID} onChange={(event) => setTargetProjectID(event.target.value)}>{transferTargets.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
              </>
            ) : null}
            <div className={projectClasses.dialogActions}><Button tone="ghost" type="button" onClick={() => setDeleteState(null)}>取消</Button><Button tone="danger" type="button" busy={busy} disabled={deleteState.requiresTransfer && !targetProjectID} onClick={() => void remove()}>{deleteState.requiresTransfer ? '转移并删除' : '删除'}</Button></div>
          </div>
        </Modal>
      ) : null}
    </div>
  )
}
