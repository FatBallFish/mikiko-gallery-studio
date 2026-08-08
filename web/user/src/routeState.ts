import type { RouteId } from './types'

export type UserRouteState = {
  route: RouteId
  returnTo?: RouteId
  imageId?: string
  taskId?: string
}

export type UserRouteOptions = {
  returnTo?: RouteId
  imageId?: string | null
  taskId?: string | null
}

export const userRouteSet = new Set<RouteId>(['landing', 'login', 'home', 'genpic', 'gallery', 'public-gallery', 'checkout', 'api-keys', 'profile', 'settings'])

export function parseUserHashState(hash: string): UserRouteState {
  const raw = hash.replace(/^#\/?/, '')
  const [path = 'landing', query = ''] = raw.split('?')
  const route = userRouteSet.has(path as RouteId) ? path as RouteId : 'landing'
  const params = new URLSearchParams(query)
  const returnTo = params.get('returnTo')
  const imageId = cleanRouteParam(params.get('image_id'))
  const taskId = cleanRouteParam(params.get('task_id'))
  return {
    route,
    returnTo: returnTo && userRouteSet.has(returnTo as RouteId) ? returnTo as RouteId : undefined,
    imageId,
    taskId,
  }
}

export function userHashForRoute(route: RouteId, options: UserRouteOptions = {}) {
  const params = new URLSearchParams()
  if (options.returnTo) params.set('returnTo', options.returnTo)
  const imageId = cleanRouteParam(options.imageId)
  if (imageId) params.set('image_id', imageId)
  const taskId = cleanRouteParam(options.taskId)
  if (taskId) params.set('task_id', taskId)
  const suffix = params.toString()
  return `/${route}${suffix ? `?${suffix}` : ''}`
}

function cleanRouteParam(value?: string | null) {
  const trimmed = value?.trim()
  return trimmed || undefined
}
