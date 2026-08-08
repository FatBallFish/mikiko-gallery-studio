import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { Project } from '../../shared/api-types'
import { userApi } from '../../shared/user-api'
import { createLatestProjectRequestGuard, createProjectSelectionController, type ProjectSelectionSnapshot } from './projectSelection'

type ProjectContextValue = ProjectSelectionSnapshot & {
  loading: boolean
  error: string
  selectProject: (projectID: string) => void
  refreshProjects: () => Promise<Project[]>
  createProject: (name: string) => Promise<Project>
  renameProject: (project: Project, name: string) => Promise<Project>
  deleteProject: (project: Project, targetProjectID?: string) => Promise<void>
}

const emptySnapshot: ProjectSelectionSnapshot = { projects: [], selectedProjectID: '', selectedProject: null }
const ProjectContext = createContext<ProjectContextValue | null>(null)

export function ProjectProvider({ userID, children }: { userID: string; children: React.ReactNode }) {
  const controller = useMemo(() => userID ? createProjectSelectionController({ userID, storage: window.localStorage }) : null, [userID])
  const requestGuard = useMemo(() => createLatestProjectRequestGuard(), [controller])
  const [snapshot, setSnapshot] = useState<ProjectSelectionSnapshot>(emptySnapshot)
  const [loading, setLoading] = useState(Boolean(userID))
  const [error, setError] = useState('')

  useEffect(() => {
    if (!controller) {
      setSnapshot(emptySnapshot)
      setLoading(false)
      return undefined
    }
    setSnapshot(controller.getSnapshot())
    return controller.subscribe(() => setSnapshot(controller.getSnapshot()))
  }, [controller])

  useEffect(() => {
    if (!controller) return undefined
    const onStorage = (event: StorageEvent) => controller.handleStorageEvent(event)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener('storage', onStorage)
      requestGuard.invalidate()
      controller.dispose()
    }
  }, [controller, requestGuard])

  const refreshProjects = useCallback(async () => {
    if (!controller) return []
    const request = requestGuard.begin()
    setLoading(true)
    try {
      const projects = await userApi.listProjects()
      if (requestGuard.isCurrent(request)) {
        controller.bootstrap(projects)
        setSnapshot(controller.getSnapshot())
        setError('')
      }
      return projects
    } catch (caught) {
      if (requestGuard.isCurrent(request)) setError(caught instanceof Error ? caught.message : '项目加载失败')
      throw caught
    } finally {
      if (requestGuard.isCurrent(request)) setLoading(false)
    }
  }, [controller, requestGuard])

  useEffect(() => {
    if (controller) void refreshProjects().catch(() => undefined)
  }, [controller, refreshProjects])

  const value = useMemo<ProjectContextValue>(() => ({
    ...snapshot,
    loading,
    error,
    selectProject: (projectID) => {
      controller?.select(projectID)
      if (controller) setSnapshot(controller.getSnapshot())
    },
    refreshProjects,
    createProject: async (name) => {
      const created = await userApi.createProject(name)
      const projects = await refreshProjects()
      controller?.select(created.id)
      if (controller) setSnapshot(controller.getSnapshot())
      return projects.find((project) => project.id === created.id) ?? created
    },
    renameProject: async (project, name) => {
      const updated = await userApi.renameProject(project.id, name, project.version)
      await refreshProjects()
      return updated
    },
    deleteProject: async (project, targetProjectID) => {
      await userApi.deleteProject(project.id, project.version, targetProjectID)
      await refreshProjects()
    },
  }), [controller, error, loading, refreshProjects, snapshot])

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
}

export function useProjects() {
  const value = useContext(ProjectContext)
  if (!value) throw new Error('useProjects must be used within ProjectProvider')
  return value
}

export function ProjectSelector({ className = '' }: { className?: string }) {
  const projects = useProjects()
  return (
    <label className={`grid min-w-0 gap-1 text-xs font-semibold text-[var(--muted)] ${className}`}>
      <span>项目</span>
      <select
        aria-label="当前项目"
        className="h-10 min-w-40 max-w-full rounded-md border border-[var(--border)] bg-[var(--surface)] px-3 text-sm text-[var(--fg)] outline-none focus:border-[var(--accent)]"
        value={projects.selectedProjectID}
        disabled={projects.loading || projects.projects.length === 0}
        onChange={(event) => projects.selectProject(event.target.value)}
      >
        {projects.projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
      </select>
    </label>
  )
}
