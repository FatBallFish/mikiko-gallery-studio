export const WORKSPACE_STREAM_MAX_RETRIES = 3

let nextGenerationId = 1

export type WorkspaceStreamGeneration = {
  id: number
  token: string
  projectID: string
  retryCount: number
  acceptsEvents: boolean
}

export function createWorkspaceStreamGeneration(token: string, projectID = '', retryCount = 0): WorkspaceStreamGeneration {
  return {
    id: nextGenerationId++,
    token,
    projectID,
    retryCount: Math.max(0, Math.min(WORKSPACE_STREAM_MAX_RETRIES, Math.floor(retryCount))),
    acceptsEvents: true,
  }
}

export function nextWorkspaceStreamRetry(generation: WorkspaceStreamGeneration) {
  if (generation.retryCount >= WORKSPACE_STREAM_MAX_RETRIES) {
    return { retry: false, attempt: generation.retryCount }
  }
  generation.retryCount += 1
  return { retry: true, attempt: generation.retryCount }
}

export function markWorkspaceStreamHealthy(generation: WorkspaceStreamGeneration) {
  generation.retryCount = 0
}

export function workspaceStreamEventIsCurrent(
  source: WorkspaceStreamGeneration,
  current: WorkspaceStreamGeneration | null,
) {
  return source.acceptsEvents && workspaceStreamRecoveryIsCurrent(source, current)
}

export function closeWorkspaceStreamGeneration(generation: WorkspaceStreamGeneration) {
  generation.acceptsEvents = false
}

export function workspaceStreamRecoveryIsCurrent(
  source: WorkspaceStreamGeneration,
  current: WorkspaceStreamGeneration | null,
) {
  return current?.id === source.id && current.token === source.token && current.projectID === source.projectID
}
