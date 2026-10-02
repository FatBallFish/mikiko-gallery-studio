import { ApiError } from '../../../../shared/http-client'
import { videoFieldErrors, videoRequestError } from './videoErrors'

const mismatch = new ApiError('no candidate', 422, 'VIDEO_CAPABILITY_MISMATCH', undefined, {
  field_errors: [
    { field: 'duration_seconds', rule: 'unsupported', message: 'video duration is not supported' },
    { field: 'inputs.first_frame.size_bytes', rule: 'too_large', message: 'input exceeds the provider size limit' },
  ],
})
const mismatchErrors = videoFieldErrors(mismatch)
if (mismatchErrors.duration_seconds !== '当前模型不支持所选时长' || mismatchErrors['inputs.first_frame.size_bytes'] !== '首帧文件超过当前模型限制') {
  throw new Error(`capability errors must point to concrete controls in Chinese: ${JSON.stringify(mismatchErrors)}`)
}

const variable = new ApiError('variable missing', 400, 'VIDEO_FIELD_INVALID', undefined, { field: 'prompt_variables', rule: 'required', name: 'scene' })
if (videoFieldErrors(variable).prompt_variables !== '变量“scene”尚未填写') throw new Error('prompt variable errors must name the unresolved variable')

const input = new ApiError('input missing', 400, 'VIDEO_INPUT_INVALID', undefined, { field: 'inputs.first_frame', rule: 'required' })
if (videoFieldErrors(input)['inputs.first_frame'] !== '请选择首帧图片') throw new Error('required input errors must point to the missing frame')

if (Object.keys(videoFieldErrors(new Error('network'))).length !== 0) throw new Error('unstructured errors must remain global instead of being assigned to a random field')

const referenceExtra = new ApiError('提交了模板未使用的资源绑定', 400, 'VIDEO_FIELD_INVALID', undefined, { field: 'reference_bindings', rule: 'reference_extra', name: '参考图' })
const referenceExtraErrors = videoFieldErrors(referenceExtra)
if (!referenceExtraErrors.reference_bindings?.includes('参考图') || !referenceExtraErrors.reference_bindings.includes('@')) {
  throw new Error(`reference binding errors must tell users how to @ reference the asset: ${JSON.stringify(referenceExtraErrors)}`)
}
if (videoRequestError(referenceExtra) !== '提交了模板未使用的资源绑定') {
  throw new Error('Chinese backend validation messages must surface verbatim instead of the generic 400 copy')
}
const internalDiag = videoRequestError(new ApiError('user, project, idempotency key and quote are required', 400, 'BAD_REQUEST'))
if (internalDiag.includes('idempotency key')) {
  throw new Error('English internal diagnostics must keep the friendly generic copy')
}

console.log('video field error contract passed')

const referenceVideoFormat = new ApiError('cap mismatch', 422, 'VIDEO_CAPABILITY_MISMATCH', undefined, { field_errors: [{ field: 'inputs.reference_video.format', rule: 'unsupported' }, { field: 'inputs.reference_audio.media_type', rule: 'unsupported' }] })
const referenceFormatErrors = videoFieldErrors(referenceVideoFormat)
if (referenceFormatErrors['inputs.reference_video.format'] !== '参考视频格式不受当前模型支持' || referenceFormatErrors['inputs.reference_audio.media_type'] !== '参考音频类型不受当前模型支持') {
  throw new Error(`reference role errors must name the matching input row: ${JSON.stringify(referenceFormatErrors)}`)
}
