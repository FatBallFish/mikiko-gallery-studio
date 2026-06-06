import type { RouteId } from './types'

export type AvatarMenuIcon = 'profile' | 'billing' | 'key' | 'docs'

export type AvatarMenuItem = {
  key: string
  label: string
  route: RouteId
  icon: AvatarMenuIcon
  permission: string
}

export function avatarMenuItems(): AvatarMenuItem[] {
  return [
    { key: 'profile', label: '个人中心', route: 'profile', icon: 'profile', permission: 'account.profile.view' },
    { key: 'checkout', label: '积分充值', route: 'checkout', icon: 'billing', permission: 'billing.checkout.view' },
    { key: 'api-keys', label: 'API 密钥', route: 'api-keys', icon: 'key', permission: 'developer.api_keys.view' },
    { key: 'docs', label: '开发文档', route: 'docs', icon: 'docs', permission: 'developer.docs.view' },
  ]
}
