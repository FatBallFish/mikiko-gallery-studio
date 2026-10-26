// Route sale pricing. Decimal amounts remain strings; billing owns currency conversion.
export type ImagePricingOperation = 'generate' | 'edit' | 'layer_decomposition'
export type ImagePricingTier = { name: string; price: string } & (
  | { max_pixels: number; base_resolution?: never }
  | { base_resolution: string; max_pixels?: never }
)
export type ImagePricingInput = { free_images: number; per_image?: string }
export type ImagePricingPixelToken = {
  definition: 'platform_pixels_v1'
  pixels_per_token: number
  input_per_million: string
  output_per_million: string
}
export type ImagePricingRule = {
  operation: ImagePricingOperation
  mode: 'per_image' | 'pixel_token'
  currency: 'CNY' | 'points'
  version: string
  input: ImagePricingInput
  output_tiers?: ImagePricingTier[]
  pixel_token?: ImagePricingPixelToken
  layers?: { confirmed: boolean; base_price?: string; tiers: ImagePricingTier[] }
}
export type ImagePricingConfig = { schema_version: 1; rules: ImagePricingRule[] }
