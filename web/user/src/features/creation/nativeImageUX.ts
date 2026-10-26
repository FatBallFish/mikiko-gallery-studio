import type { NativeImageAnnotation, NativeImageProfile, NativeImageRequestFields } from '../../../../shared/native-image'

/** User drafts only carry controls the user can currently see. Platform defaults stay server-side. */
export function userNativeImageFields(value: NativeImageRequestFields, profile: NativeImageProfile | undefined, referenceCount: number): NativeImageRequestFields {
  if (!profile) return { operation: undefined, native_options: undefined, seed: undefined, negative_prompt: undefined }
  const negative = profile.negative_prompt ? value.native_options?.negative_prompt ?? value.negative_prompt : undefined
  const annotations = profile.adapter === 'seedream_image' && referenceCount > 0 ? value.native_options?.annotations : undefined
  return {
    operation: value.operation === 'layer_decomposition' && profile.layer_decomposition && profile.operations.includes('layer_decomposition') ? 'layer_decomposition' : referenceCount ? 'edit' : 'generate',
    seed: undefined, negative_prompt: undefined,
    native_options: negative || annotations?.length ? { ...(negative ? { negative_prompt: negative } : {}), ...(annotations?.length ? { annotations } : {}) } : undefined,
  }
}

/** Bind API positional annotations to stable asset IDs before references can be reordered. */
export function bindReferenceAnnotations(annotations: NativeImageAnnotation[] = [], assetIDs: string[]) {
  return Object.fromEntries(assetIDs.map((id, index) => [id, annotations.filter((item) => (item.reference_index ?? 1) === index + 1).map(({ reference_index: _index, ...item }) => item as NativeImageAnnotation)]))
}
export function remapReferenceAnnotations(bound: Record<string, NativeImageAnnotation[]>, assetIDs: string[]) {
  return assetIDs.flatMap((id, index) => (bound[id] ?? []).map((item) => ({ ...item, reference_index: index + 1 })))
}
