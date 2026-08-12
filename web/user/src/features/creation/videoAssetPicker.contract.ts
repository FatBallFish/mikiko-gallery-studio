import fs from 'node:fs'

const source = fs.readFileSync(new URL('./VideoCreationPanel.tsx', import.meta.url), 'utf8')
for (const required of ['MediaAssetPicker', "mediaTypes={['image']}", 'userApi.getMediaAsset(initialAssetId)', 'MediaPreviewDialog', 'result_asset_id']) {
  if (!source.includes(required)) throw new Error(`video creation asset flow must include ${required}`)
}
if (source.includes('placeholder="资产 ID"')) throw new Error('ordinary users must not type media UUIDs for video inputs')
