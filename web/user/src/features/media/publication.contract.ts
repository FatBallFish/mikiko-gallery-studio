import assert from 'node:assert/strict'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { ImagePublicationControls, PublicationControls, publicationState } from './PublicationControls'
import type { MediaAsset, GalleryImage } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'

for (const [input, want] of [['approved', 'public'], ['public', 'public'], ['reviewing', 'pending'], ['pending_review', 'pending'], ['unpublished', 'offline']] as const) assert.equal(publicationState(input), want)
const change = async () => ({ status: 'private' })
for (const mediaType of ['image', 'video'] as const) {
  for (const [status, label] of [['private', '申请公开'], ['approved', '撤回公开'], ['pending_review', '取消公开申请'], ['rejected', '申请公开']] as const) {
    const markup = renderToStaticMarkup(createElement(PublicationControls, { mediaType, initialStatus: status, change, onChanged: () => undefined }))
    assert.ok(markup.includes(label), `${mediaType} ${status} must offer ${label}`)
    assert.ok(markup.includes('data-media-selection-control'), 'publication controls must not trigger marquee selection')
  }
}
const asset = { id: 'asset', media_type: 'image', status: 'ready', visibility_status: 'private' } as MediaAsset
const restricted = renderToStaticMarkup(createElement(ImagePublicationControls, { asset, onChanged: () => undefined }))
assert.ok(restricted.includes('尚未关联可公开的历史图片记录'))
assert.ok(!restricted.includes('<button'), 'unsupported image source must not offer an invalid publish request')
const supported = renderToStaticMarkup(createElement(ImagePublicationControls, { asset: { ...asset, legacy_image_id: 'legacy' }, onChanged: () => undefined }))
assert.ok(supported.includes('申请公开'))
const publishOriginal = userApi.publishImage
const cancelOriginal = userApi.cancelImagePublish
const called: string[] = []
try {
  userApi.publishImage = async (id) => { called.push(`publish:${id}`); return { visibility_status: 'pending_review' } as GalleryImage }
  userApi.cancelImagePublish = async (id) => { called.push(`withdraw:${id}`); return { visibility_status: 'private' } as GalleryImage }
  const controls = ImagePublicationControls({ asset: { ...asset, legacy_image_id: 'legacy' }, onChanged: () => undefined })
  assert.equal((await controls.props.change(false)).status, 'pending_review')
  assert.equal((await controls.props.change(true)).status, 'private')
  assert.deepEqual(called, ['publish:legacy', 'withdraw:legacy'], 'image mutations must use the real legacy ID rather than the media asset ID')
} finally {
  userApi.publishImage = publishOriginal
  userApi.cancelImagePublish = cancelOriginal
}
console.log('media publication contract passed')
