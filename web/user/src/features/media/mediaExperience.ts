export type BatchResult = { id: string; status: string }
import type { MediaAssetFilters, MediaType } from '../../../../shared/api-types'
import type { UserRouteOptions } from '../../routeState'

export type MediaFilterValues = {
  mediaType: '' | MediaType
  sourceType: string
  groupName: string
  status: string
  keyword: string
  sort: string
}

export type MediaCreationAction = {
  label: '继续生图' | '生成视频' | '复用视频参数'
  options: UserRouteOptions
}

export function mediaCreationActions(asset: Pick<import('../../../../shared/api-types').MediaAsset, 'id' | 'media_type' | 'source_task_kind' | 'source_task_id'>): MediaCreationAction[] {
  if (asset.media_type === 'image') {
    const actions: MediaCreationAction[] = []
    if (asset.source_task_kind === 'image' && asset.source_task_id) {
      actions.push({ label: '继续生图', options: { media: 'image', taskId: asset.source_task_id } })
    }
    actions.push({ label: '生成视频', options: { media: 'video', assetId: asset.id } })
    return actions
  }
  if (asset.media_type === 'video' && asset.source_task_kind === 'video' && asset.source_task_id) {
    return [{ label: '复用视频参数', options: { media: 'video', taskId: asset.source_task_id } }]
  }
  return []
}

export function createMediaProjectScope(initialProjectID: string) {
  let projectID = initialProjectID
  let generation = 0
  return {
    begin(requestProjectID: string) { return { projectID: requestProjectID, generation } },
    switchTo(nextProjectID: string) { projectID = nextProjectID; generation += 1 },
    accepts(request: { projectID: string; generation: number }, responseProjectID: string) {
      return request.generation === generation && request.projectID === projectID && responseProjectID === projectID
    },
    contains(asset: { project_id: string }) { return asset.project_id === projectID },
  }
}

export function buildMediaAssetQuery(projectID: string, filters: MediaFilterValues, cursor?: string): MediaAssetFilters {
  const [sortBy = 'created_at', sortOrder = 'desc'] = filters.sort.split(':')
  return {
    project_id: projectID,
    media_type: filters.mediaType,
    source_type: filters.sourceType,
    group_name: filters.groupName,
    status: filters.status,
    keyword: filters.keyword.trim(),
    sort_by: sortBy,
    sort_order: sortOrder === 'asc' ? 'asc' : 'desc',
    cursor,
    limit: 40,
  }
}

export function reconcileBatchSelection(selected: ReadonlySet<string>, results: BatchResult[]) {
  const next = new Set(selected)
  for (const result of results) {
    if (result.status === 'succeeded') next.delete(result.id)
  }
  return next
}

export async function withSingleAccessRefresh<T>(
  consume: (url: string) => Promise<T>,
  refresh: () => Promise<string>,
  initialURL: string,
) {
  try {
    return await consume(initialURL)
  } catch (error) {
    const status = (error as { status?: number }).status
    if (status !== 401 && status !== 403) throw error
    return consume(await refresh())
  }
}

export function canHoverPreview() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(hover: hover) and (pointer: fine)').matches
    && !window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export function createHoverPreviewScheduler({ delayMs = 200, maxActive = 2 } = {}) {
  type Entry = { key: string; start: () => void; timer?: ReturnType<typeof setTimeout>; active: boolean; cancelled: boolean }
  const queue: Entry[] = []
  let activeCount = 0

  const drain = () => {
    while (activeCount < maxActive) {
      const entry = queue.find((candidate) => !candidate.cancelled && !candidate.active && !candidate.timer)
      if (!entry) break
      entry.timer = setTimeout(() => {
        entry.timer = undefined
        if (entry.cancelled || activeCount >= maxActive) {
          drain()
          return
        }
        entry.active = true
        activeCount += 1
        entry.start()
        drain()
      }, delayMs)
    }
  }

  return {
    schedule(key: string, start: () => void) {
      const entry: Entry = { key, start, active: false, cancelled: false }
      queue.push(entry)
      drain()
      return () => {
        if (entry.cancelled) return
        entry.cancelled = true
        if (entry.timer) clearTimeout(entry.timer)
        if (entry.active) activeCount = Math.max(0, activeCount - 1)
        const index = queue.indexOf(entry)
        if (index >= 0) queue.splice(index, 1)
        drain()
      }
    },
  }
}

export function createAudioPlaybackCoordinator() {
  let active: { id: string; pause: () => void } | null = null
  return {
    activate(id: string, pause: () => void) {
      if (active && active.id !== id) active.pause()
      active = { id, pause }
    },
    release(id: string) {
      if (active?.id === id) active = null
    },
  }
}

export const mediaHoverScheduler = createHoverPreviewScheduler()
export const mediaAudioCoordinator = createAudioPlaybackCoordinator()
