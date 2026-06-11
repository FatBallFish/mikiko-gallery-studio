import type { RouteId } from './types'

export type UserRouteState = {
  route: RouteId
  returnTo?: RouteId
  imageId?: string
}

export type UserRouteOptions = {
  returnTo?: RouteId
  imageId?: string | null
}

export const userRouteSet = new Set<RouteId>(['landing', 'login', 'home', 'genpic', 'gallery', 'public-gallery', 'checkout', 'api-keys', 'profile', 'docs', 'settings'])

export function parseUserHashState(hash: string): UserRouteState {
  const raw = hash.replace(/^#\/?/, '')
  const [path = 'landing', query = ''] = raw.split('?')
  const route = userRouteSet.has(path as RouteId) ? path as RouteId : 'landing'
  const params = new URLSearchParams(query)
  const returnTo = params.get('returnTo')
  const imageId = cleanRouteParam(params.get('image_id'))
  return {
    route,
    returnTo: returnTo && userRouteSet.has(returnTo as RouteId) ? returnTo as RouteId : undefined,
    imageId,
  }
}

export function userHashForRoute(route: RouteId, options: UserRouteOptions = {}) {
  const params = new URLSearchParams()
  if (options.returnTo) params.set('returnTo', options.returnTo)
  const imageId = cleanRouteParam(options.imageId)
  if (imageId) params.set('image_id', imageId)
  const suffix = params.toString()
  return `/${route}${suffix ? `?${suffix}` : ''}`
}

function cleanRouteParam(value?: string | null) {
  const trimmed = value?.trim()
  return trimmed || undefined
}
