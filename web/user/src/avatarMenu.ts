import type { RouteId } from './types'

export type AvatarMenuIcon = 'profile' | 'billing' | 'key' | 'docs'

type AvatarMenuItemBase = {
  key: string
  label: string
  icon: AvatarMenuIcon
  permission: string
}

export type AvatarMenuItem = AvatarMenuItemBase & (
  | { route: RouteId; external?: false }
  | { route?: never; external: true }
)

export function avatarMenuItems(): AvatarMenuItem[] {
  return [
    { key: 'profile', label: '个人中心', route: 'profile', icon: 'profile', permission: 'account.profile.view' },
    { key: 'checkout', label: '积分充值', route: 'checkout', icon: 'billing', permission: 'billing.checkout.view' },
    { key: 'api-keys', label: 'API 密钥', route: 'api-keys', icon: 'key', permission: 'developer.api_keys.view' },
    { key: 'docs', label: '开发文档', icon: 'docs', permission: 'developer.docs.view', external: true },
  ]
}
