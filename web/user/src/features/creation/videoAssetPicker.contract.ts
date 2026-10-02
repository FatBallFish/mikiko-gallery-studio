import fs from 'node:fs'

const source = fs.readFileSync(new URL('./VideoCreationPanel.tsx', import.meta.url), 'utf8')
for (const required of ['MediaAssetPicker', 'PICKER_MEDIA_TYPES', "reference_audio: 'audio'", "reference_video: 'video'", 'userApi.getMediaAsset(initialAssetId)', 'MediaPreviewDialog', 'result_asset_id']) {
  if (!source.includes(required)) throw new Error(`video creation asset flow must include ${required}`)
}
if (source.includes('placeholder="资产 ID"')) throw new Error('ordinary users must not type media UUIDs for video inputs')

const pickerSource = fs.readFileSync(new URL('../media/MediaAssetPicker.tsx', import.meta.url), 'utf8')
if (pickerSource.includes("status: 'ready'")) throw new Error('media asset picker must not hide usable ready_original assets behind a status=ready query')
if (!pickerSource.includes('ready_original')) throw new Error('media asset picker must accept ready_original assets as generation inputs')
