import { readFileSync } from 'node:fs'
import type { Project } from '../../shared/api-types'
import { createLatestProjectRequestGuard, createProjectSelectionController, projectSelectionStorageKey } from './projectSelection'

class MemoryStorage implements Storage {
  private values = new Map<string, string>()
  get length() { return this.values.size }
  clear() { this.values.clear() }
  getItem(key: string) { return this.values.get(key) ?? null }
  key(index: number) { return [...this.values.keys()][index] ?? null }
  removeItem(key: string) { this.values.delete(key) }
  setItem(key: string, value: string) { this.values.set(key, value) }
}

const project = (id: string, name: string, isDefault = false, status = 'active'): Project => ({
  id, name, is_default: isDefault, status, version: 1, created_at: '', updated_at: '',
})
const defaultProject = project('default-a', '默认', true)
const campaign = project('campaign-a', 'Campaign')
const storage = new MemoryStorage()

const requestGuard = createLatestProjectRequestGuard()
const olderRequest = requestGuard.begin()
const newerRequest = requestGuard.begin()
if (requestGuard.isCurrent(olderRequest) || !requestGuard.isCurrent(newerRequest)) {
  throw new Error('only the latest project refresh may commit its response')
}

if (projectSelectionStorageKey('user-a') === projectSelectionStorageKey('user-b')) {
  throw new Error('project persistence keys must be scoped by authenticated user ID')
}
storage.setItem(projectSelectionStorageKey('user-a'), campaign.id)
const first = createProjectSelectionController({ userID: 'user-a', storage })
first.bootstrap([defaultProject, campaign])
if (first.getSnapshot().selectedProjectID !== campaign.id) throw new Error('remembered owned project should be restored')

storage.setItem(projectSelectionStorageKey('user-b'), campaign.id)
const foreign = createProjectSelectionController({ userID: 'user-b', storage })
foreign.bootstrap([project('default-b', '默认', true)])
if (foreign.getSnapshot().selectedProjectID !== 'default-b' || storage.getItem(projectSelectionStorageKey('user-b')) !== 'default-b') {
  throw new Error('foreign remembered project must fall back to default and repair storage')
}

storage.setItem(projectSelectionStorageKey('user-a'), 'deleted-a')
const deleted = createProjectSelectionController({ userID: 'user-a', storage })
deleted.bootstrap([defaultProject, campaign, project('deleted-a', 'Deleted', false, 'deleted')])
if (deleted.getSnapshot().selectedProjectID !== defaultProject.id) throw new Error('deleted remembered project must fall back to default')

const sameTabA = createProjectSelectionController({ userID: 'same-user', storage })
const sameTabB = createProjectSelectionController({ userID: 'same-user', storage })
sameTabA.bootstrap([defaultProject, campaign])
sameTabB.bootstrap([defaultProject, campaign])
sameTabA.select(campaign.id)
if (sameTabB.getSnapshot().selectedProjectID !== campaign.id) throw new Error('same-tab controllers must synchronize selection')

sameTabB.handleStorageEvent({ key: projectSelectionStorageKey('same-user'), newValue: 'missing' })
if (sameTabB.getSnapshot().selectedProjectID !== defaultProject.id) throw new Error('cross-tab invalid selection must fall back to default')

for (const file of ['./App.tsx', './pages/WorkspacePage.tsx', './pages/GalleryPage.tsx'] as const) {
  const source = readFileSync(new URL(file, import.meta.url), 'utf8')
  if (!source.includes(file === './App.tsx' ? 'ProjectProvider' : 'useProjects')) {
    throw new Error(`${file} must use the shared project selection provider`)
  }
}

const appSource = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
const routeSource = readFileSync(new URL('./routeState.ts', import.meta.url), 'utf8')
const componentsSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const projectsPageSource = readFileSync(new URL('./pages/ProjectsPage.tsx', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('./pages/WorkspacePage.tsx', import.meta.url), 'utf8')
const gallerySource = readFileSync(new URL('./pages/GalleryPage.tsx', import.meta.url), 'utf8')

for (const [source, contract] of [
  [appSource, "case 'projects'"],
  [routeSource, "'projects'"],
  [componentsSource, "route: 'projects'"],
  [projectsPageSource, 'createProject'],
  [projectsPageSource, 'renameProject'],
  [projectsPageSource, 'deleteProject'],
  [projectsPageSource, 'project.is_default'],
  [projectsPageSource, 'targetProjectID'],
  [workspaceSource, 'project_id: selectedProjectID'],
  [gallerySource, 'GALLERY_PAGE_SIZE, selectedProjectID'],
] as const) {
  if (!source.includes(contract)) throw new Error(`project experience must expose ${contract}`)
}

sameTabA.dispose()
sameTabB.dispose()
first.dispose()
foreign.dispose()
deleted.dispose()
