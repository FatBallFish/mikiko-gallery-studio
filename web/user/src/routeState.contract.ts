import { parseUserHashState, userHashForRoute } from './routeState'

const publicDetail = parseUserHashState('#/public-gallery?image_id=img_123')
if (publicDetail.route !== 'public-gallery' || publicDetail.imageId !== 'img_123') {
  throw new Error(`public gallery deep link should preserve image_id, got ${JSON.stringify(publicDetail)}`)
}

const loginReturn = parseUserHashState('#/login?returnTo=public-gallery&image_id=img_456')
if (loginReturn.route !== 'login' || loginReturn.returnTo !== 'public-gallery' || loginReturn.imageId !== 'img_456') {
  throw new Error(`login return hash should preserve public gallery image_id, got ${JSON.stringify(loginReturn)}`)
}

const loginHash = userHashForRoute('login', { returnTo: 'public-gallery', imageId: 'img_789' })
if (loginHash !== '/login?returnTo=public-gallery&image_id=img_789') {
  throw new Error(`login hash should include image_id return parameter, got ${loginHash}`)
}

const detailHash = userHashForRoute('public-gallery', { imageId: 'img_999' })
if (detailHash !== '/public-gallery?image_id=img_999') {
  throw new Error(`public gallery hash should include image_id, got ${detailHash}`)
}

const workspaceTask = parseUserHashState('#/genpic?task_id=task_failed_123')
if (workspaceTask.route !== 'genpic' || workspaceTask.taskId !== 'task_failed_123') {
  throw new Error(`workspace deep link should preserve task_id, got ${JSON.stringify(workspaceTask)}`)
}

const workspaceHash = userHashForRoute('genpic', { taskId: ' task_running_456 ' })
if (workspaceHash !== '/genpic?task_id=task_running_456') {
  throw new Error(`workspace hash should include a trimmed task_id, got ${workspaceHash}`)
}

const loginTaskReturn = userHashForRoute('login', { returnTo: 'genpic', taskId: 'task_retry_789' })
if (loginTaskReturn !== '/login?returnTo=genpic&task_id=task_retry_789') {
  throw new Error(`login hash should preserve the workspace task return context, got ${loginTaskReturn}`)
}

const combinedContext = parseUserHashState('#/login?returnTo=public-gallery&image_id=img_keep&task_id=task_keep')
if (combinedContext.imageId !== 'img_keep' || combinedContext.taskId !== 'task_keep') {
  throw new Error(`route parsing must preserve public image and workspace task params independently, got ${JSON.stringify(combinedContext)}`)
}

const settings = parseUserHashState('#/settings')
if (settings.route !== 'settings') {
  throw new Error(`settings route should be parseable, got ${JSON.stringify(settings)}`)
}

const settingsHash = userHashForRoute('settings')
if (settingsHash !== '/settings') {
  throw new Error(`settings route should have a stable hash, got ${settingsHash}`)
}

const projects = parseUserHashState('#/projects')
if (projects.route !== 'projects' || userHashForRoute('projects') !== '/projects') {
  throw new Error(`projects route should have a stable hash, got ${JSON.stringify(projects)}`)
}

const retiredDocsRoute = parseUserHashState('#/docs')
if (retiredDocsRoute.route !== 'landing') {
  throw new Error(`retired docs route must not redirect or remain registered, got ${JSON.stringify(retiredDocsRoute)}`)
}

const retiredDemoRoute = parseUserHashState('#/redesign-demo')
if (retiredDemoRoute.route !== 'landing') {
  throw new Error(`redesign demo route should not be part of the production user router, got ${JSON.stringify(retiredDemoRoute)}`)
}

const canvasDetail = parseUserHashState('#/creative-canvas?canvas_id=canvas_123')
if (canvasDetail.route !== 'creative-canvas' || canvasDetail.canvasId !== 'canvas_123') {
  throw new Error(`creative canvas detail route must preserve canvas_id: ${JSON.stringify(canvasDetail)}`)
}
if (userHashForRoute('login', { returnTo: 'creative-canvas', canvasId: ' canvas_456 ' }) !== '/login?returnTo=creative-canvas&canvas_id=canvas_456') {
  throw new Error('canvas return route must survive login redirects')
}
