import { calculateImageSizeForQuality } from './image-size'

if (calculateImageSizeForQuality('1K', '16:9') !== '1280x720') {
  throw new Error('1K 16:9 should map to 1280x720')
}

if (calculateImageSizeForQuality('2K', '16:9') !== '2560x1440') {
  throw new Error('2K 16:9 should map to 2560x1440')
}

if (calculateImageSizeForQuality('4K', '1:1') !== '2880x2880') {
  throw new Error('4K 1:1 should map to 2880x2880')
}

if (calculateImageSizeForQuality('auto', '16:9') !== '1536x864') {
  throw new Error('auto 16:9 should keep legacy native estimate sizing')
}
