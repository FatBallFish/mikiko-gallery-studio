import { calculateImageSizeForBaseResolution } from './image-size'

if (calculateImageSizeForBaseResolution('1K', '16:9') !== '1280x720') {
  throw new Error('1K 16:9 should map to 1280x720')
}

if (calculateImageSizeForBaseResolution('2K', '16:9') !== '2560x1440') {
  throw new Error('2K 16:9 should map to 2560x1440')
}

if (calculateImageSizeForBaseResolution('4K', '1:1') !== '2880x2880') {
  throw new Error('4K 1:1 should map to 2880x2880')
}

if (calculateImageSizeForBaseResolution('auto', '16:9') !== '1536x864') {
  throw new Error('auto 16:9 should keep legacy native estimate sizing')
}
