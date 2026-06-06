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
