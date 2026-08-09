import type { Project } from '../../shared/api-types'

export type ProjectSelectionSnapshot = {
  projects: Project[]
  selectedProjectID: string
  selectedProject: Project | null
  selectionGeneration: number
}

type StorageEventLike = { key: string | null; newValue: string | null }
type Listener = () => void
type SelectionBroadcast = (projectID: string, source: symbol) => void

const sameTabListeners = new Map<string, Set<SelectionBroadcast>>()

export function createLatestProjectRequestGuard() {
  let latestRequest = 0
  return {
    begin() {
      latestRequest += 1
      return latestRequest
    },
    isCurrent(request: number) {
      return request === latestRequest
    },
    invalidate() {
      latestRequest += 1
    },
  }
}

export function projectSelectionStorageKey(userID: string | number) {
  return `mikiko-studio:selected-project:${encodeURIComponent(String(userID))}`
}

export function resolveSelectedProject(projects: Project[], rememberedID: string | null | undefined) {
  const active = projects.filter((project) => project.status === 'active')
  return active.find((project) => project.id === rememberedID) ?? active.find((project) => project.is_default) ?? active[0] ?? null
}

export function createProjectSelectionController({ userID, storage }: { userID: string | number; storage: Storage }) {
  const key = projectSelectionStorageKey(userID)
  const source = Symbol(key)
  let projects: Project[] = []
  let selectedProjectID = ''
  let selectionGeneration = 0
  const listeners = new Set<Listener>()

  const notify = () => listeners.forEach((listener) => listener())
  const apply = (candidateID: string | null | undefined, persist: boolean) => {
    const selected = resolveSelectedProject(projects, candidateID)
    const nextID = selected?.id ?? ''
    const changed = nextID !== selectedProjectID
    selectedProjectID = nextID
    if (changed) selectionGeneration += 1
    if (persist) {
      try {
        if (nextID) storage.setItem(key, nextID)
        else storage.removeItem(key)
      } catch {
        // Selection remains usable when browser storage is unavailable.
      }
    }
    if (changed) notify()
  }
  const sameTabListener: SelectionBroadcast = (projectID, eventSource) => {
    if (eventSource !== source) apply(projectID, false)
  }
  const channel = sameTabListeners.get(key) ?? new Set<SelectionBroadcast>()
  channel.add(sameTabListener)
  sameTabListeners.set(key, channel)

  return {
    bootstrap(nextProjects: Project[]) {
      projects = nextProjects.filter((project) => project.status === 'active')
      let remembered: string | null = null
      try { remembered = storage.getItem(key) } catch { remembered = null }
      apply(remembered, true)
    },
    replaceProjects(nextProjects: Project[]) {
      projects = nextProjects.filter((project) => project.status === 'active')
      apply(selectedProjectID, true)
    },
    select(projectID: string) {
      const selected = resolveSelectedProject(projects, projectID)
      if (!selected || selected.id !== projectID) return false
      apply(projectID, true)
      sameTabListeners.get(key)?.forEach((broadcast) => broadcast(projectID, source))
      return true
    },
    handleStorageEvent(event: StorageEventLike) {
      if (event.key !== key) return
      apply(event.newValue, true)
    },
    getSnapshot(): ProjectSelectionSnapshot {
      return {
        projects: [...projects],
        selectedProjectID,
        selectedProject: projects.find((project) => project.id === selectedProjectID) ?? null,
        selectionGeneration,
      }
    },
    subscribe(listener: Listener) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    dispose() {
      listeners.clear()
      const current = sameTabListeners.get(key)
      current?.delete(sameTabListener)
      if (current?.size === 0) sameTabListeners.delete(key)
    },
  }
}

export type ProjectSelectionController = ReturnType<typeof createProjectSelectionController>
