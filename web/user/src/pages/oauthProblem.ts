import { ApiError } from '../../../shared/http-client'

export type OAuthProblem = { kind: 'unconfigured' | 'login' | 'expired' | 'network' | 'fault'; message: string; retryable: boolean }
export function oauthProblem(error: unknown, surface: 'apps' | 'consent'): OAuthProblem {
  const status = error instanceof ApiError ? error.status : 0
  const code = error instanceof ApiError ? error.code.toUpperCase() : ''
  if (status === 503 && code === 'CANONICAL_URL_UNCONFIGURED') return { kind: 'unconfigured', message: '此部署尚未配置 OAuth 公共端点，请联系管理员配置 canonical HTTPS 地址后重试。', retryable: true }
  if (status === 401 && code === 'LOGIN_REQUIRED') return { kind: 'login', message: '请先登录 Studio，再继续授权。', retryable: true }
  if (status === 401) return { kind: 'login', message: '登录状态已失效，请重新登录后重试。', retryable: true }
  if (surface === 'consent' && code === 'AUTHORIZATION_REQUEST_EXPIRED') return { kind: 'expired', message: '授权请求已过期，请返回客户端重新发起连接。', retryable: false }
  if (!status) return { kind: 'network', message: '网络连接失败，请检查网络后重试。', retryable: true }
  if (status === 400 || status === 403 || status === 404) return { kind: 'fault', message: '授权请求无效或不被允许，请返回客户端检查连接配置并重新发起。', retryable: false }
  return { kind: 'fault', message: `OAuth 服务暂时故障（HTTP ${status}），请稍后重试。`, retryable: true }
}
