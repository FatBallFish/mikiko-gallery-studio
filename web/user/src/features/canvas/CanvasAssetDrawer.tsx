import type { MediaAsset } from '../../../../shared/api-types'
import { MediaAssetPicker } from '../media/MediaAssetPicker'

export function CanvasAssetDrawer({ projectID, onSelect, onClose }: { projectID: string; onSelect: (asset: MediaAsset) => void; onClose: () => void }) {
  return <MediaAssetPicker projectID={projectID} mediaTypes={['image', 'video', 'audio']} title="添加资产到画布" onConfirm={(assets) => { if (assets[0]) onSelect(assets[0]) }} onClose={onClose} />
}
