import { homePublicGalleryAccess } from './homeGalleryModel'

const guest = homePublicGalleryAccess(null, 'img_1')
if (guest.action !== 'login' || guest.returnTo !== 'home' || guest.imageId !== 'img_1') {
  throw new Error(`guest home gallery click should require login, got ${JSON.stringify(guest)}`)
}

const loggedIn = homePublicGalleryAccess('token', 'img_2')
if (loggedIn.action !== 'detail' || loggedIn.imageId !== 'img_2') {
  throw new Error(`logged-in home gallery click should load detail, got ${JSON.stringify(loggedIn)}`)
}
