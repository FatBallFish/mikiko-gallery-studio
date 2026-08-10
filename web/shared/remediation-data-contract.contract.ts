import type {
  ImageResult,
  ImageTask,
  ModelAccountModel,
  ModelAccountModelWriteRequest,
  ObjectDeletionJob,
  PaymentOrder,
  Project,
  ReferenceAsset,
  SubscriptionPlan,
} from './api-types'

const project: Project = {
  id: 'project-1',
  name: '默认',
  is_default: true,
  status: 'active',
  version: 1,
  created_at: '2026-08-08T00:00:00Z',
  updated_at: '2026-08-08T00:00:00Z',
}

const plan = { credit_expiry_enabled: false } satisfies Partial<SubscriptionPlan>
const permanentPlan = {
  credit_expiry_enabled: false,
  duration_days: null,
} satisfies Pick<SubscriptionPlan, 'credit_expiry_enabled' | 'duration_days'>
const permanentPlanWire = JSON.parse(JSON.stringify(permanentPlan)) as Record<string, unknown>
if (permanentPlanWire.credit_expiry_enabled !== false || permanentPlanWire.duration_days !== null) {
  throw new Error(`permanent plan expiry contract was not preserved: ${JSON.stringify(permanentPlanWire)}`)
}
const order = {
  credit_expiry_enabled: true,
  credit_valid_days: 30,
  credited_at: '2026-08-08T00:00:00Z',
  credit_expires_at: '2026-09-07T00:00:00Z',
} satisfies Partial<PaymentOrder>
const model = {
  size_modes: ['auto', 'ratio', 'pixel'],
  supports_custom_ratio: true,
  supported_backgrounds: ['auto', 'transparent'],
  min_width: 512,
  max_width: 4096,
  min_height: 512,
  max_height: 4096,
  max_image_count: 10,
} satisfies Partial<ModelAccountModel>
const modelWrite = model satisfies Partial<ModelAccountModelWriteRequest>
const reference = { source_image_result_id: 'result-1', owns_object: false } satisfies Partial<ReferenceAsset>
const task = { project_id: project.id, project } satisfies Partial<ImageTask>
const result = { project_id: project.id, project } satisfies Partial<ImageResult>
const cleanup = {
  id: 'cleanup-1',
  storage_driver: 's3',
  bucket: 'images',
  object_key: 'users/1/image.png',
  state: 'pending',
  attempt_count: 0,
  created_at: '2026-08-08T00:00:00Z',
  updated_at: '2026-08-08T00:00:00Z',
} satisfies ObjectDeletionJob

void [plan, permanentPlan, order, model, modelWrite, reference, task, result, cleanup]
