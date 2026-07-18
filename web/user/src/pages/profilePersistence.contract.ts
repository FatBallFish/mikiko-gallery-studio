import { readFileSync } from 'node:fs'
import type { GenerationPreferences } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'

const profileSource = readFileSync(new URL('./ProfilePage.tsx', import.meta.url), 'utf8')

for (const unsupportedControl of ['默认模型', '默认比例', '默认基础分辨率', '默认质量']) {
  if (profileSource.includes(unsupportedControl)) {
    throw new Error(`Profile must not promise unsupported generation preference persistence: ${unsupportedControl}`)
  }
}

if (profileSource.includes('账户偏好已保存') || profileSource.includes('生成偏好已保存')) {
  throw new Error('Profile save feedback must describe personal profile persistence only')
}

if (!profileSource.includes('个人资料已保存')) {
  throw new Error('Profile save feedback must explicitly confirm personal profile persistence')
}

let capturedBody: Record<string, unknown> | null = null
const originalFetch = globalThis.fetch
globalThis.fetch = async (_input, init) => {
  capturedBody = JSON.parse(String(init?.body)) as Record<string, unknown>
  return new Response(JSON.stringify({
    data: {
      id: 'user_1',
      email: 'user@example.com',
      nickname: '新昵称',
      bio: '新签名',
    },
  }), { status: 200, headers: { 'content-type': 'application/json' } })
}

const unsupportedPreferences: GenerationPreferences = {
  model_group: 'pro-image',
  base_resolution: '4K',
  quality: '4K',
  aspect_ratio: '16:9',
  image_count: 3,
}

try {
  await userApi.updateProfile({
    display_name: '新昵称',
    signature: '新签名',
    preferences: unsupportedPreferences,
  })
} finally {
  globalThis.fetch = originalFetch
}

if (JSON.stringify(capturedBody) !== JSON.stringify({ nickname: '新昵称', bio: '新签名' })) {
  throw new Error(`Profile update payload must contain only persisted profile fields, got ${JSON.stringify(capturedBody)}`)
}

if (JSON.stringify(capturedBody).includes('pro-image')) {
  throw new Error('model_group must never be written into default_locale')
}
