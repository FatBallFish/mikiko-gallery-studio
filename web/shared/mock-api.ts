import type { AdminMetric, AdminSession, AdminUser, ApiKey, AuditLog, Balance, Capability, ConfigItem, CreateApiKeyRequest, CreateTaskRequest, EndpointDoc, EstimateRequest, EstimateResult, ImageTask, LedgerEntry, ModelRoute, PriceRow, ProviderHealth, ReferenceAsset, ReviewItem, UserProfile } from './api-types'
import { resolveGenerationResolution } from './generation-resolution'
import { adminMetrics, demoCapability, demoImages, demoProfile, endpointDocs, initialAudit, initialConfig, initialKeys, initialLedger, initialPrices, initialReviews, initialRoutes, initialTasks, initialUsers, providerHealth } from './mock-data'

const wait = (ms = 320) => new Promise((resolve) => setTimeout(resolve, ms))
const now = () => new Date().toLocaleString('sv-SE', { hour12: false }).replace('T', ' ')
const id = (prefix: string) => `${prefix}_${Math.random().toString(36).slice(2, 9)}`
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

function toNumber(points: string): number {
  return Number.parseFloat(points || '0')
}

function formatPoints(value: number): string {
  return value.toFixed(5)
}

class MockPicGalleryApi {
  private profile: UserProfile = clone(demoProfile)
  private balanceValue = 182.5
  private ledger: LedgerEntry[] = clone(initialLedger)
  private tasks: ImageTask[] = clone(initialTasks)
  private keys: ApiKey[] = clone(initialKeys)
  private refs: ReferenceAsset[] = []
  private sentCodes = new Map<string, string>()
  private config: ConfigItem[] = clone(initialConfig)
  private routes: ModelRoute[] = clone(initialRoutes)
  private prices: PriceRow[] = clone(initialPrices)
  private reviews: ReviewItem[] = clone(initialReviews)
  private users: AdminUser[] = clone(initialUsers)
  private audit: AuditLog[] = clone(initialAudit)

  reset() {
    this.profile = clone(demoProfile)
    this.balanceValue = 182.5
    this.ledger = clone(initialLedger)
    this.tasks = clone(initialTasks)
    this.keys = clone(initialKeys)
    this.refs = []
    this.config = clone(initialConfig)
    this.routes = clone(initialRoutes)
    this.prices = clone(initialPrices)
    this.reviews = clone(initialReviews)
    this.users = clone(initialUsers)
    this.audit = clone(initialAudit)
  }

  private addAudit(action: string, target: string, detail: string, actor = 'Admin Liu') {
    this.audit.unshift({ id: id('aud'), actor, action, target, detail, created_at: now() })
  }

  async sendEmailCode(email: string, scene: 'login' | 'register' = 'login') {
    await wait()
    if (!/^\S+@\S+\.\S+$/.test(email)) throw new Error('请输入有效邮箱地址')
    this.sentCodes.set(email, '123456')
    return { cooldown_seconds: 60, scene }
  }

  async loginWithEmailCode(email: string, code: string) {
    await wait(420)
    if (this.sentCodes.get(email) !== code && code !== '123456') throw new Error('验证码错误或已过期')
    this.profile.email = email
    return { access_token: 'mock_access_token', expires_in: 600, profile: clone(this.profile) }
  }

  async loginWithPassword(email: string, password: string) {
    await wait(420)
    if (!/^\S+@\S+\.\S+$/.test(email)) throw new Error('请输入有效邮箱地址')
    if (password.length < 6) throw new Error('密码至少 6 位；Mock 可使用任意 6 位以上密码')
    this.profile.email = email
    return { access_token: 'mock_access_token', expires_in: 600, profile: clone(this.profile) }
  }

  async refreshSession() {
    await wait(180)
    return { access_token: 'mock_access_token_refreshed', expires_in: 600 }
  }

  async logout() {
    await wait(160)
    return { ok: true }
  }

  async getProfile(): Promise<UserProfile> {
    await wait(180)
    return clone(this.profile)
  }

  async updateProfile(patch: Partial<UserProfile>): Promise<UserProfile> {
    await wait()
    this.profile = { ...this.profile, ...patch, preferences: { ...this.profile.preferences, ...(patch.preferences ?? {}) } }
    return clone(this.profile)
  }

  async getBalance(): Promise<Balance> {
    await wait(180)
    return { available_points: formatPoints(this.balanceValue), frozen_points: '0.00000', plan_name: this.profile.tier, first_purchase_bonus: true }
  }

  async getLedger(): Promise<LedgerEntry[]> {
    await wait(200)
    return clone(this.ledger)
  }

  async redeemCode(code: string) {
    await wait(360)
    if (!['WELCOME-2026', 'PIC-TRIAL', 'DEV-BOOST'].includes(code.trim().toUpperCase())) throw new Error('兑换码无效或已使用')
    this.balanceValue += 20
    const entry: LedgerEntry = { id: id('led'), title: `兑换码: ${code.toUpperCase()}`, occurred_at: now(), amount: '+20.00000', type: 'credit', detail: 'Mock 活动兑换' }
    this.ledger.unshift(entry)
    return { balance: await this.getBalance(), ledger_entry: clone(entry) }
  }

  async getCapabilities(): Promise<Capability> {
    await wait(180)
    return clone(demoCapability)
  }

  async estimate(req: EstimateRequest): Promise<EstimateResult> {
    await wait(220)
    const routeCode = req.route_model_code ?? req.model_group ?? 'basic'
    const { requested_quality: requestedQuality } = resolveGenerationResolution(req)
    const resolvedQuality = requestedQuality === 'auto' ? '2k' : requestedQuality
    const modelBase = routeCode.includes('pro') ? 8 : routeCode.includes('plus') ? 5.125 : 2
    const qualityMulti = requestedQuality === '4k' ? 2 : requestedQuality === '2k' ? 1.45 : requestedQuality === 'auto' ? 1.25 : 1
    const refMulti = req.task_type === 'reference_to_image' || req.task_type === 'image_edit' ? 1.2 : 1
    const points = modelBase * qualityMulti * refMulti * req.image_count
    return {
      points: points.toFixed(2),
      charged_points: formatPoints(points),
      display_points: points.toFixed(2),
      formula: `${routeCode} x ${requestedQuality} x ${req.task_type} x ${req.image_count}`,
      resolved_quality: resolvedQuality,
      base_resolution: resolvedQuality,
      sufficient: this.balanceValue >= points,
    }
  }

  async uploadReferenceAsset(fileName: string, sizeBytes = 1_280_000): Promise<ReferenceAsset> {
    await wait(500)
    const asset: ReferenceAsset = { id: id('refasset'), name: fileName, preview_url: demoImages[this.refs.length % demoImages.length], status: 'ready', size_bytes: sizeBytes, created_at: now() }
    this.refs.unshift(asset)
    return clone(asset)
  }

  async listReferenceAssets(): Promise<ReferenceAsset[]> {
    await wait(160)
    return clone(this.refs)
  }

  async createTask(req: CreateTaskRequest): Promise<ImageTask> {
    await wait(420)
    if (req.prompt.trim().length < 8) throw new Error('请至少输入 8 个字符的提示词')
    const estimate = await this.estimate(req)
    if (!estimate.sufficient) throw new Error('积分余额不足，请充值或降低输出质量')
    this.balanceValue -= toNumber(estimate.charged_points ?? estimate.points)
    const routeCode = req.route_model_code ?? req.model_group ?? 'basic'
    const resolvedQuality = estimate.base_resolution ?? estimate.resolved_quality ?? resolveGenerationResolution(req).requested_quality
    this.ledger.unshift({ id: id('led'), title: `生图任务: ${req.prompt.slice(0, 28)}...`, occurred_at: now(), amount: `-${estimate.charged_points ?? estimate.points}`, type: 'debit', detail: `${routeCode} / ${estimate.resolved_quality} / ${req.image_count} 张` })
    const task: ImageTask = {
      id: id('task'),
      title: req.prompt.split(/[，,。.]/)[0].slice(0, 54) || 'Untitled generation',
      prompt: req.prompt,
      task_type: req.task_type,
      status: 'queued',
      route_model_code: routeCode,
      model_group: routeCode,
      base_resolution: resolvedQuality,
      quality: resolvedQuality,
      aspect_ratio: req.aspect_ratio,
      image_count: req.image_count,
      estimate_points: estimate.display_points ?? estimate.points,
      progress: 8,
      progress_stage: 'queued',
      progress_message: '任务已进入生成队列',
      provider: routeCode.includes('basic') ? 'OpenRouter' : 'OpenAI',
      route: req.task_type === 'text_to_image' ? 'primary' : 'capability-matrix',
      created_at: now(),
      updated_at: now(),
      reference_assets: this.refs.filter((asset) => req.reference_asset_ids?.includes(asset.id)),
      results: [],
    }
    this.tasks.unshift(task)
    return clone(task)
  }

  async getTask(taskId: string): Promise<ImageTask> {
    await wait(360)
    const task = this.tasks.find((item) => item.id === taskId)
    if (!task) throw new Error('任务不存在')
    if (task.status === 'queued') {
      task.status = 'running'
      task.progress = 42
      task.progress_stage = 'provider'
      task.progress_message = `正在调用 ${task.route_model_code || task.model_group} 生成图片`
      task.updated_at = now()
    } else if (task.status === 'running') {
      task.status = 'succeeded'
      task.progress = 100
      task.progress_stage = 'completed'
      task.progress_message = '生成完成，结果已同步到资产'
      task.updated_at = now()
      task.results = Array.from({ length: task.image_count }, (_, index) => ({
        id: id('img'),
        url: demoImages[(this.tasks.length + index) % demoImages.length],
        width: task.aspect_ratio === '16:9' ? 1536 : 1024,
        height: task.aspect_ratio === '9:16' ? 1536 : 1024,
        publish_status: 'private' as const,
      }))
    }
    return clone(task)
  }

  async listTasks(filters?: { query?: string; status?: string; type?: string }): Promise<ImageTask[]> {
    await wait(240)
    let rows = this.tasks
    if (filters?.query) rows = rows.filter((row) => `${row.title} ${row.prompt}`.toLowerCase().includes(filters.query!.toLowerCase()))
    if (filters?.status && filters.status !== 'all') rows = rows.filter((row) => row.status === filters.status)
    if (filters?.type && filters.type !== 'all') rows = rows.filter((row) => row.task_type === filters.type)
    return clone(rows)
  }

  async updatePublishStatus(imageId: string, status: 'reviewing' | 'private' | 'public') {
    await wait(260)
    for (const task of this.tasks) {
      const image = task.results.find((item) => item.id === imageId)
      if (image) image.publish_status = status
    }
    return { ok: true }
  }

  async deleteTask(taskId: string) {
    await wait(260)
    this.tasks = this.tasks.filter((task) => task.id !== taskId)
    return { ok: true }
  }

  async listApiKeys(): Promise<ApiKey[]> {
    await wait(220)
    return clone(this.keys)
  }

  async createApiKey(input: CreateApiKeyRequest & { rpm_limit: number; expires_at: string | null }): Promise<ApiKey> {
    await wait(380)
    if (input.name.trim().length < 3) throw new Error('密钥名称至少 3 个字符')
    const key: ApiKey = { id: id('key'), name: input.name, access_key: `pk_live_${Math.random().toString(36).slice(2, 14)}`, secret_preview: `sk_once_${Math.random().toString(36).slice(2, 18)}`, status: 'active', scopes: input.scopes ?? ['images:write', 'images:read'], total_quota_points: input.total_quota_points ?? null, daily_quota_points: input.daily_quota_points ?? null, total_quota_used_points: '0.00000', daily_quota_used_points: '0.00000', rpm_limit: input.rpm_limit, expires_at: input.expires_at, created_at: now().slice(0, 10), last_used_at: null }
    this.keys.unshift(key)
    return clone(key)
  }

  async updateApiKey(idValue: string, patch: Partial<ApiKey>) {
    await wait(240)
    this.keys = this.keys.map((key) => key.id === idValue ? { ...key, ...patch, secret_preview: undefined } : key)
    return clone(this.keys.find((key) => key.id === idValue)!)
  }

  async deleteApiKey(idValue: string) {
    await wait(240)
    this.keys = this.keys.filter((key) => key.id !== idValue)
    return { ok: true }
  }

  async listEndpointDocs(): Promise<EndpointDoc[]> {
    await wait(140)
    return clone(endpointDocs)
  }

  async adminLogin(email: string, password: string): Promise<AdminSession> {
    await wait(360)
    if (!email.includes('@') || password.length < 6) throw new Error('管理员邮箱或密码无效')
    return { token: 'mock_admin_token', admin_name: 'Admin Liu', role: 'super_admin' }
  }

  async getAdminDashboard(): Promise<{ metrics: AdminMetric[]; providers: ProviderHealth[]; queue: Array<{ item: string; count: string; detail: string }>; audit: AuditLog[] }> {
    await wait(220)
    return { metrics: clone(adminMetrics), providers: clone(providerHealth), queue: [
      { item: '公开图审核', count: String(this.reviews.filter((item) => item.status === 'pending').length).padStart(2, '0'), detail: '人工审核后决定是否进入图片广场' },
      { item: '价格策略待复核', count: String(this.prices.filter((item) => item.state === 'draft').length).padStart(2, '0'), detail: '涉及图生图差价与用户倍率' },
      { item: '配置待发布', count: String(this.config.filter((item) => item.state !== 'active').length).padStart(2, '0'), detail: '发布后 1 分钟内全节点生效' },
    ], audit: clone(this.audit.slice(0, 6)) }
  }

  async listConfig(): Promise<ConfigItem[]> {
    await wait(200)
    return clone(this.config)
  }

  async editConfig(key: string, draftValue: string): Promise<ConfigItem> {
    await wait(220)
    const item = this.config.find((row) => row.key === key)
    if (!item) throw new Error('配置项不存在')
    item.draft_value = draftValue
    item.state = draftValue === item.value ? 'active' : 'draft'
    this.addAudit('EDIT_CONFIG_DRAFT', key, `${item.value} -> ${draftValue}`)
    return clone(item)
  }

  async publishConfig(): Promise<ConfigItem[]> {
    await wait(420)
    this.config = this.config.map((item) => item.state === 'draft' ? { ...item, value: item.draft_value, state: 'active', version: item.version + 1 } : item)
    this.addAudit('PUBLISH_CONFIG', 'config-tabs', '发布配置草稿')
    return clone(this.config)
  }

  async revertConfig(): Promise<ConfigItem[]> {
    await wait(280)
    this.config = this.config.map((item) => ({ ...item, draft_value: item.value, state: 'active' }))
    this.addAudit('REVERT_CONFIG', 'config-tabs', '丢弃配置草稿')
    return clone(this.config)
  }

  async listRoutes(): Promise<ModelRoute[]> {
    await wait(200)
    return clone(this.routes)
  }

  async updateRoute(routeId: string, patch: Partial<ModelRoute>) {
    await wait(260)
    this.routes = this.routes.map((route) => route.id === routeId ? { ...route, ...patch } : route)
    this.addAudit('UPDATE_ROUTE', routeId, JSON.stringify(patch))
    return clone(this.routes.find((route) => route.id === routeId)!)
  }

  async listPrices(): Promise<PriceRow[]> {
    await wait(200)
    return clone(this.prices)
  }

  async updatePrice(priceId: string, patch: Partial<PriceRow>) {
    await wait(260)
    this.prices = this.prices.map((row) => row.id === priceId ? { ...row, ...patch, state: 'draft' } : row)
    this.addAudit('EDIT_PRICE', priceId, JSON.stringify(patch))
    return clone(this.prices.find((row) => row.id === priceId)!)
  }

  async publishPrices() {
    await wait(360)
    this.prices = this.prices.map((row) => row.state === 'draft' ? { ...row, state: 'active', version: row.version + 1 } : row)
    this.addAudit('PUBLISH_PRICE', 'billing-pricing', '发布价格矩阵')
    return clone(this.prices)
  }

  async listReviews(): Promise<ReviewItem[]> {
    await wait(220)
    return clone(this.reviews)
  }

  async decideReview(reviewId: string, decision: 'approved' | 'rejected' | 'unpublished', reason: string) {
    await wait(320)
    this.reviews = this.reviews.map((item) => item.id === reviewId ? { ...item, status: decision, reason } : item)
    this.addAudit(`REVIEW_${decision.toUpperCase()}`, reviewId, reason)
    return clone(this.reviews.find((item) => item.id === reviewId)!)
  }

  async listUsers(query = ''): Promise<AdminUser[]> {
    await wait(220)
    const q = query.toLowerCase()
    return clone(this.users.filter((user) => !q || `${user.email} ${user.display_name} ${user.id}`.toLowerCase().includes(q)))
  }

  async updateUser(userId: string, patch: Partial<AdminUser>) {
    await wait(260)
    this.users = this.users.map((user) => user.id === userId ? { ...user, ...patch } : user)
    this.addAudit('UPDATE_USER', userId, JSON.stringify(patch))
    return clone(this.users.find((user) => user.id === userId)!)
  }

  async listAudit(): Promise<AuditLog[]> {
    await wait(180)
    return clone(this.audit)
  }

  async getHealth(): Promise<ProviderHealth[]> {
    await wait(200)
    return clone(providerHealth)
  }
}

export const mockApi = new MockPicGalleryApi()
export type { MockPicGalleryApi }
