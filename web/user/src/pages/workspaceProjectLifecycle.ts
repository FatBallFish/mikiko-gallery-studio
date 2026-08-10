import type { Project } from '../../../shared/api-types'

type WorkspaceProjectState = {
  loading: boolean
  error: string
  selectedProjectID: string
  selectedProject: Project | null
}

export type WorkspaceProjectSelection = {
  projectID: string
  generation: number
}

export function workspaceProjectReadiness(state: WorkspaceProjectState) {
  if (state.loading) return { ready: false, reason: '项目正在加载，请稍后重试。' }
  if (state.error) return { ready: false, reason: state.error }
  if (!state.selectedProjectID || !state.selectedProject || state.selectedProject.id !== state.selectedProjectID) {
    return { ready: false, reason: '当前项目不可用，请刷新项目列表后重试。' }
  }
  return { ready: true, reason: '' }
}

export function workspaceSubmissionIsCurrent(
  submitted: WorkspaceProjectSelection,
  current: WorkspaceProjectSelection,
) {
  return submitted.projectID === current.projectID && submitted.generation === current.generation
}
