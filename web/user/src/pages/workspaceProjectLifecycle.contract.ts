import { readFileSync } from 'node:fs'
import {
  workspaceProjectReadiness,
  workspaceSubmissionIsCurrent,
} from './workspaceProjectLifecycle'

const selected = { id: 'project-a', name: 'A', is_default: false, status: 'active', version: 1, created_at: '', updated_at: '' }
for (const [name, input] of [
  ['loading', { loading: true, error: '', selectedProjectID: '', selectedProject: null }],
  ['error', { loading: false, error: 'failed', selectedProjectID: '', selectedProject: null }],
  ['missing', { loading: false, error: '', selectedProjectID: '', selectedProject: null }],
  ['mismatch', { loading: false, error: '', selectedProjectID: 'project-b', selectedProject: selected }],
] as const) {
  if (workspaceProjectReadiness(input).ready) throw new Error(`${name} project state must block generation`)
}
if (!workspaceProjectReadiness({ loading: false, error: '', selectedProjectID: 'project-a', selectedProject: selected }).ready) {
  throw new Error('an owned selected project must enable project-dependent workspace actions')
}

const submitted = { projectID: 'project-a', generation: 2 }
if (!workspaceSubmissionIsCurrent(submitted, { projectID: 'project-a', generation: 2 })) {
  throw new Error('the unchanged project selection must accept its submit response')
}
if (workspaceSubmissionIsCurrent(submitted, { projectID: 'project-b', generation: 3 })) {
  throw new Error('a response from the previous project must not commit after switching projects')
}
if (workspaceSubmissionIsCurrent(submitted, { projectID: 'project-a', generation: 4 })) {
  throw new Error('A to B to A must still reject the stale A submit response by generation')
}

const workspace = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
for (const required of [
  'workspaceProjectReadiness',
  'projects.loading',
  'projects.error',
  'projects.selectedProject',
  'selectionGeneration',
  'project_id: submissionProject.projectID',
  'workspaceSubmissionIsCurrent(submissionProject, projectSelectionRef.current)',
  'projects.selectProject(task.project_id)',
  'if (!token || !projectReadiness.ready)',
]) {
  if (!workspace.includes(required)) throw new Error(`Workspace project lifecycle must include ${required}`)
}
