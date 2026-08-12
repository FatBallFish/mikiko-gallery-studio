import localforage from 'localforage'
import type { CanvasDocument } from '../core/types'

export type CanvasDraftSnapshot = {
  schema_version: 1
  user_id: string
  canvas_id: string
  base_revision: number
  saved_at: string
  document: CanvasDocument
}
export type CanvasDraftRecovery = 'recover_local' | 'conflict' | 'discard_local'

const drafts = localforage.createInstance({ name: 'mikiko-gallery-studio', storeName: 'canvas_drafts', version: 1 })

export function canvasDraftKey(userID: string, canvasID: string) {
  return `mgs:canvas-draft:v1:${encodeURIComponent(userID)}:${encodeURIComponent(canvasID)}`
}

export function decideCanvasDraftRecovery(snapshot: CanvasDraftSnapshot, remoteRevision: number, matchesRemote: boolean): CanvasDraftRecovery {
  if (matchesRemote) return 'discard_local'
  return snapshot.base_revision === remoteRevision ? 'recover_local' : 'conflict'
}

export async function readCanvasDraft(userID: string, canvasID: string) {
  const snapshot = await drafts.getItem<CanvasDraftSnapshot>(canvasDraftKey(userID, canvasID))
  if (!snapshot || snapshot.schema_version !== 1 || snapshot.user_id !== userID || snapshot.canvas_id !== canvasID) return null
  return snapshot
}

export function writeCanvasDraft(snapshot: CanvasDraftSnapshot) {
  return drafts.setItem(canvasDraftKey(snapshot.user_id, snapshot.canvas_id), snapshot)
}

export function removeCanvasDraft(userID: string, canvasID: string) {
  return drafts.removeItem(canvasDraftKey(userID, canvasID))
}

export function createCanvasDraftWriter(delay = 400) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending: CanvasDraftSnapshot | null = null
  async function flush() {
    if (timer) clearTimeout(timer)
    timer = null
    const snapshot = pending
    pending = null
    if (snapshot) await writeCanvasDraft(snapshot)
  }
  return {
    schedule(snapshot: CanvasDraftSnapshot) {
      pending = snapshot
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => { void flush() }, delay)
    },
    flush,
    cancel() {
      if (timer) clearTimeout(timer)
      timer = null
      pending = null
    },
  }
}
