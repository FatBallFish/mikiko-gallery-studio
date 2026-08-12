import { readFileSync } from 'node:fs'
import { buildMediaAssetQuery } from './mediaExperience'

const query = buildMediaAssetQuery('project-a', {
  mediaType: 'video', sourceType: 'upload', groupName: 'campaign', status: 'ready', keyword: ' launch ', sort: 'updated_at:asc',
}, 'cursor-a')
if (query.project_id !== 'project-a' || query.keyword !== 'launch' || query.cursor !== 'cursor-a' || query.sort_by !== 'updated_at' || query.sort_order !== 'asc') {
  throw new Error(`media filters must map to the server query contract: ${JSON.stringify(query)}`)
}

const card = readFileSync(new URL('./MediaAssetCard.tsx', import.meta.url), 'utf8')
for (const required of ['mediaHoverScheduler.schedule', "getMediaAssetAccess(asset.id, 'preview')", 'canHoverPreview()']) {
  if (!card.includes(required)) throw new Error(`media card must preserve ${required}`)
}
if (card.includes("getMediaAssetAccess(asset.id, 'download')")) throw new Error('asset cards must never request originals')

const picker = readFileSync(new URL('./MediaAssetPicker.tsx', import.meta.url), 'utf8')
if (picker.includes('asset_id') || picker.includes('UUID')) throw new Error('asset picker must not ask users for identifiers')
for (const required of ['keyword', 'next_cursor', 'project_id']) {
  if (!picker.includes(required)) throw new Error(`asset picker must support cross-project search and pagination through ${required}`)
}

for (const purpose of ["'poster'", "'hover'", "'waveform'"]) {
  if (!card.includes(purpose)) throw new Error(`media cards must request the dedicated ${purpose} derivative`)
}
if (!card.includes('AbortController')) throw new Error('hover access requests must be cancellable when the pointer leaves')

const app = readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8')
if (!app.includes('<UploadTray />') || !app.includes('<MediaAssetsPage />')) throw new Error('upload tray and unified media page must be integrated')

console.log('media network contract passed')
