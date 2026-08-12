import { ApiError } from '../../../../shared/http-client'
import { videoFieldErrors } from './videoErrors'

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

console.log('video field error contract passed')
