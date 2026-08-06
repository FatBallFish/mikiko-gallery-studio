import type { GalleryImage } from '../../../shared/api-types'
import {
  filterGalleryImages,
  galleryImageCard,
  galleryImageSearchText,
  galleryPublishActionPresentation,
  galleryPublishLabel,
  galleryPublishMatches,
  galleryPublishStatus,
  patchGalleryItems,
  removeGalleryItems,
} from './galleryRows'

if (galleryPublishLabel('public') !== '已公开' || galleryPublishLabel('approved') !== '已公开') {
  throw new Error('public and approved publish statuses should share visible label')
}
if (galleryPublishLabel('reviewing') !== '审核中' || galleryPublishLabel('pending_review') !== '审核中') {
  throw new Error('reviewing and pending_review publish statuses should share visible label')
}
if (galleryPublishLabel('unknown_status') !== 'unknown_status') {
  throw new Error('unknown publish statuses should preserve raw values for troubleshooting')
}

if (galleryPublishStatus(image({ visibility_status: 'public' })) !== 'approved') {
  throw new Error('legacy public status should normalize to approved for filtering')
}
if (galleryPublishStatus(image({ visibility_status: 'reviewing' })) !== 'pending_review') {
  throw new Error('legacy reviewing status should normalize to pending_review for filtering')
}

if (!galleryPublishMatches('public', 'approved') || !galleryPublishMatches('approved', 'approved')) {
  throw new Error('approved filter should include both public and approved rows')
}
if (!galleryPublishMatches('reviewing', 'pending_review') || !galleryPublishMatches('pending_review', 'pending_review')) {
  throw new Error('pending_review filter should include both reviewing and pending_review rows')
}
if (galleryPublishMatches('private', 'approved')) {
  throw new Error('approved filter should not include private rows')
}

const rows = [
  image({ id: '1', prompt: 'blue city skyline', visibility_status: 'public', task_type: 'text_to_image', image_group: '城市' }),
  image({ id: '2', prompt: 'green forest', visibility_status: 'reviewing', task_type: 'image_edit', image_group: '自然' }),
  image({ id: '3', prompt: 'red icon', visibility_status: 'private', task_type: 'image_edit', image_group: '' }),
]

const approvedRows = filterGalleryImages(rows, {
  type: 'all',
  status: 'all',
  publishStatus: 'approved',
  imageGroup: 'all',
  query: '',
})
if (approvedRows.length !== 1 || approvedRows[0]?.id !== '1') {
  throw new Error(`approved filter should include legacy public rows, got ${JSON.stringify(approvedRows)}`)
}

const reviewingRows = filterGalleryImages(rows, {
  type: 'all',
  status: 'all',
  publishStatus: 'pending_review',
  imageGroup: 'all',
  query: '',
})
if (reviewingRows.length !== 1 || reviewingRows[0]?.id !== '2') {
  throw new Error(`pending_review filter should include legacy reviewing rows, got ${JSON.stringify(reviewingRows)}`)
}

const multiTermRows = filterGalleryImages(rows, {
  type: 'all',
  status: 'all',
  publishStatus: 'all',
  imageGroup: 'all',
  query: '公开 city',
})
if (multiTermRows.length !== 1 || multiTermRows[0]?.id !== '1') {
  throw new Error(`gallery search should include localized publish status and split query terms, got ${JSON.stringify(multiTermRows)}`)
}

const search = galleryImageSearchText(rows[1])
for (const expected of ['green forest', '图片编辑', '审核中', '自然']) {
  if (!search.includes(expected.toLowerCase())) {
    throw new Error(`gallery search text should include ${expected}, got ${search}`)
  }
}

const downloadable = galleryImageCard(image({
  id: 'img_download',
  prompt: 'download from signed url',
  url: '',
  download_url: '/api/open/image/v1/gallery/images/img_download/image',
  created_at: '2026-06-05T13:45:30Z',
}))
if (!downloadable.canDownload) {
  throw new Error(`gallery card should allow download when download_url exists, got ${JSON.stringify(downloadable)}`)
}
if (!downloadable.canPreview || !downloadable.canEdit || downloadable.imageUrl !== '/api/open/image/v1/gallery/images/img_download/image') {
  throw new Error(`gallery card should expose asset card preview/edit fields, got ${JSON.stringify(downloadable)}`)
}
if (downloadable.modelLabel !== 'gpt-image' || downloadable.ratioLabel !== '1:1') {
  throw new Error(`gallery card should expose model and ratio labels, got ${JSON.stringify(downloadable)}`)
}
if (downloadable.createdAtLabel !== '2026/06/05 13:45') {
  throw new Error(`gallery card should format created_at without raw T/Z date, got ${downloadable.createdAtLabel}`)
}
if (downloadable.modelLine !== '文生图 · gpt-image') {
  throw new Error(`gallery card should expose localized model line, got ${downloadable.modelLine}`)
}

const reviewing = galleryImageCard(image({ visibility_status: 'pending_review', url: '/image.png' }))
if (!reviewing.canPublish || reviewing.publishAction !== 'cancel' || reviewing.publishActionLabel !== '取消申请') {
  throw new Error(`gallery card should allow cancel while reviewing, got ${JSON.stringify(reviewing)}`)
}

const approved = galleryImageCard(image({ visibility_status: 'approved', url: '/image.png' }))
if (!approved.canPublish || approved.publishAction !== 'cancel' || approved.publishActionLabel !== '取消公开') {
  throw new Error(`gallery card should allow cancel for approved images, got ${JSON.stringify(approved)}`)
}

const requestPresentation = galleryPublishActionPresentation('private', true)
const withdrawPresentation = galleryPublishActionPresentation('pending_review', true)
const unpublishPresentation = galleryPublishActionPresentation('approved', true)
if (requestPresentation.icon === withdrawPresentation.icon || withdrawPresentation.icon === unpublishPresentation.icon || requestPresentation.icon === unpublishPresentation.icon) {
  throw new Error('申请公开、取消申请、取消公开必须使用三种不同图标')
}
if (requestPresentation.tone !== 'positive' || withdrawPresentation.tone !== 'warning' || unpublishPresentation.tone !== 'danger') {
  throw new Error(`publish action tones drifted: ${JSON.stringify({ requestPresentation, withdrawPresentation, unpublishPresentation })}`)
}

const mutationRows = [
  image({ id: 'patch-1', image_group: '保留' }),
  image({ id: 'patch-2', image_group: '旧分组' }),
  image({ id: 'patch-3', image_group: '保留' }),
]
const patchedRows = patchGalleryItems(mutationRows, [image({ id: 'patch-2', image_group: '新分组', visibility_status: 'pending_review' })])
if (patchedRows === mutationRows || patchedRows[0] !== mutationRows[0] || patchedRows[2] !== mutationRows[2]) {
  throw new Error('gallery patch must replace the collection while preserving untouched item identity')
}
if (patchedRows[1] === mutationRows[1] || patchedRows[1]?.image_group !== '新分组' || patchedRows[1]?.visibility_status !== 'pending_review') {
  throw new Error(`gallery patch did not merge the affected item: ${JSON.stringify(patchedRows[1])}`)
}
const unchangedRows = patchGalleryItems(mutationRows, [image({ id: 'missing' })])
if (unchangedRows !== mutationRows) {
  throw new Error('gallery patch must preserve collection identity when no loaded item matches')
}
const removedRows = removeGalleryItems(mutationRows, new Set(['patch-2']))
if (removedRows.length !== 2 || removedRows[0] !== mutationRows[0] || removedRows[1] !== mutationRows[2]) {
  throw new Error('gallery removal must preserve untouched items and remove only successful IDs')
}

const rejected = galleryImageCard(image({ visibility_status: 'rejected', url: '/image.png' }))
if (!rejected.canPublish || rejected.publishAction !== 'request' || rejected.publishActionLabel !== '重新申请') {
  throw new Error(`rejected image should allow reapply, got ${JSON.stringify(rejected)}`)
}

const privateWithoutAsset = galleryImageCard(image({ visibility_status: 'private', url: '', download_url: '' }))
if (privateWithoutAsset.canPublish || privateWithoutAsset.publishAction !== null || privateWithoutAsset.canDownload || privateWithoutAsset.publishActionLabel !== '无图片文件') {
  throw new Error(`gallery card should block publish/download without image asset, got ${JSON.stringify(privateWithoutAsset)}`)
}
if (privateWithoutAsset.canPreview || privateWithoutAsset.canEdit) {
  throw new Error(`gallery card should block preview/edit without image asset, got ${JSON.stringify(privateWithoutAsset)}`)
}

const invalidDate = galleryImageCard(image({ created_at: 'not-a-date' }))
if (invalidDate.createdAtLabel !== 'not-a-date') {
  throw new Error(`gallery card should preserve invalid date for troubleshooting, got ${invalidDate.createdAtLabel}`)
}
if (/T|Z$/.test(downloadable.createdAtLabel)) {
  throw new Error(`gallery card date should not expose ISO separators, got ${downloadable.createdAtLabel}`)
}

function image(patch: Partial<GalleryImage>): GalleryImage {
  return {
    id: patch.id ?? 'img_1',
    task_id: patch.task_id ?? 'task_1',
    prompt: patch.prompt ?? 'test prompt',
    task_type: patch.task_type ?? 'text_to_image',
    task_status: patch.task_status ?? 'succeeded',
    route_model_code: patch.route_model_code ?? 'gpt-image',
    base_resolution: patch.base_resolution ?? '1K',
    quality: patch.quality ?? 'auto',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    url: patch.url,
    download_url: patch.download_url,
    file_size_bytes: patch.file_size_bytes ?? 0,
    width: patch.width ?? 1024,
    height: patch.height ?? 1024,
    image_group: patch.image_group,
    visibility_status: patch.visibility_status ?? 'private',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
  }
}
