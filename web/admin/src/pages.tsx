export type AdminPageId = 'overview' | 'config' | 'routing' | 'pricing' | 'review' | 'audit'

const configRows = [
  { tab: '生成限制', key: 'generation_limits.max_image_count', value: '5', state: '已生效' },
  { tab: '积分价格', key: 'billing_pricing.cny_per_point', value: '0.31250', state: '已生效' },
  { tab: '模型价格', key: 'billing_pricing.plus.2k', value: '8.00000', state: '待发布' },
  { tab: '用户分组', key: 'user_groups.default_multiplier', value: '1.00000', state: '已生效' },
  { tab: '认证安全', key: 'auth_security.access_token_ttl_sec', value: '600', state: '已生效' },
  { tab: '开发文档', key: 'docs.portal_enabled', value: 'true', state: '已生效' },
]

const routeRows = [
  { scene: 'Compat /images/generations', provider: 'OpenAI', policy: '主路由', note: '失败按错误码决定重试或切换' },
  { scene: 'Compat /images/edits', provider: 'OpenAI', policy: '图输入优先', note: 'mask 与 image 校验前置' },
  { scene: '参考图生成', provider: 'OpenRouter', policy: '能力优先', note: '仅命中支持 image input 的模型' },
  { scene: 'Provider 异常', provider: 'Internal', policy: '包装输出', note: '屏蔽上游无意义错误文案' },
]

const queueRows = [
  { item: '公开图审核', count: '12', detail: '人工审核后决定是否进入图片广场' },
  { item: '价格策略待复核', count: '03', detail: '涉及图生图差价与用户倍率' },
  { item: '错误策略待确认', count: '04', detail: 'OpenAI / OpenRouter 错误码映射' },
]

const auditRows = [
  '10:24 更新 generation_limits.max_image_count：3 -> 5',
  '10:11 确认积分精度支持到小数点后五位',
  '09:52 发布 OpenAI Compat generate/edit 路由表',
  '09:38 增加公开图片人工审核开关',
]

const providerPolicyRows = [
  { code: '429 / rate_limit', action: '自动重试', detail: '指数退避后优先在原 provider 重试，再评估路由切换' },
  { code: '5xx / upstream_unavailable', action: '自动切换', detail: '在能力矩阵允许时切到备用 provider' },
  { code: '400 / invalid_image', action: '包装透出', detail: '转成平台可读错误，避免暴露上游原始字段' },
  { code: '401 / invalid_api_key', action: '内部告警', detail: '不直接透给用户，标记为平台侧 provider 配置异常' },
]

const reviewRows = [
  { title: 'Amber Cathedral', owner: 'u_1024', type: '公开申请', detail: '参考图生成 / Plus / 2K / 1 张', reason: '构图稳定，可进入广场' },
  { title: 'Mercury Figure', owner: 'u_2468', type: '公开申请', detail: '局部编辑 / Plus / 1K / 1 张', reason: '需检查是否含敏感人物元素' },
  { title: 'Glass Sonata', owner: 'u_3141', type: '再次审核', detail: '风格迁移 / Basic / 4K / 1 张', reason: '用户补充了修改说明' },
]

const pricingRows = [
  { group: 'Basic', q1k: '2.00000', q2k: '4.00000', q4k: '8.00000' },
  { group: 'Plus', q1k: '5.00000', q2k: '8.00000', q4k: '16.00000' },
]

export function OverviewPage() {
  return (
    <section className="page-stack">
      <section className="pg-admin-card overview-surface">
        <section className="main-lane">
          <div className="lane-head">
            <div>
              <label>系统健康</label>
              <strong>用一眼可扫读的行列结构承载核心运维信息。</strong>
            </div>
            <div className="inline-tags">
              <span>任务成功率 98.7%</span>
              <span>平均生成时长 18s</span>
              <span>自动重试命中 2.4%</span>
            </div>
          </div>

          <div className="overview-band">
            <div className="band-cell">
              <label>OpenAI</label>
              <strong>主 Provider 健康</strong>
              <span>近 1 小时 5xx 占比 0.3%</span>
            </div>
            <div className="band-cell">
              <label>OpenRouter</label>
              <strong>备用路由可切换</strong>
              <span>支持 image input 模型 4 个</span>
            </div>
            <div className="band-cell">
              <label>任务队列</label>
              <strong>执行中 18 / 等待中 06</strong>
              <span>worker lease 续约正常</span>
            </div>
          </div>

          <div className="lane-divider" />

          <div className="lane-head compact">
            <div>
              <label>集群部署</label>
              <strong>集群支持被体现在运维页，而不是藏在文档里。</strong>
            </div>
          </div>
          <div className="cluster-grid">
            <div className="cluster-row">
              <strong>Ingress / Nginx</strong>
              <span>统一入口、限流、静态资源与 TLS 终止</span>
            </div>
            <div className="cluster-row">
              <strong>API Pods x 3</strong>
              <span>处理 Web API、OpenAPI、OpenAI Compat 与 token 刷新</span>
            </div>
            <div className="cluster-row">
              <strong>Worker Pods x 4</strong>
              <span>抢占任务 lease，负责 OpenAI / OpenRouter 生成与重试</span>
            </div>
            <div className="cluster-row">
              <strong>Redis + Postgres + Object Storage</strong>
              <span>共享状态、队列协调、资产存储与审计数据</span>
            </div>
          </div>
        </section>

        <aside className="signal-rail">
          <section className="signal-section">
            <div className="section-head compact">
              <strong>待处理事项</strong>
            </div>
            <div className="queue-list">
              {queueRows.map((row) => (
                <div key={row.item} className="queue-row">
                  <strong>{row.count}</strong>
                  <div>
                    <span>{row.item}</span>
                    <p>{row.detail}</p>
                  </div>
                </div>
              ))}
            </div>
          </section>

          <section className="signal-section">
            <div className="section-head compact">
              <strong>最近变更</strong>
            </div>
            <div className="audit-list">
              {auditRows.map((row) => (
                <div key={row} className="audit-row">
                  <span>{row}</span>
                </div>
              ))}
            </div>
          </section>
        </aside>
      </section>
    </section>
  )
}

export function ConfigCenterPage() {
  return (
    <section className="page-stack">
      <div className="status-strip pg-admin-card">
        <div className="status-cell">
          <label>草稿状态</label>
          <strong>2 项待发布变更</strong>
        </div>
        <div className="status-cell">
          <label>发布轨迹</label>
          <strong>v2026.05.19.3 (可回滚)</strong>
        </div>
        <div className="status-cell">
          <label>生效范围</label>
          <strong>全量 API 节点 (3)</strong>
        </div>
        <div className="status-cell">
          <label>同步冲突</label>
          <strong>无冲突检测</strong>
        </div>
      </div>

      <section className="pg-admin-card config-motherboard">
        <section className="config-sheet-lane">
          <div className="config-toolbar-band">
            <div className="config-mode-tabs">
              <span className="active">生成限制</span>
              <span>价格策略</span>
              <span>模型路由</span>
              <span>认证安全</span>
              <span>开发文档</span>
            </div>
            <div className="config-toolbar-meta">
              <span>草稿 2</span>
              <span>可回滚</span>
              <span>预发布中</span>
            </div>
          </div>

          <div className="config-intro-line">
            <div>
              <label>配置主表</label>
              <strong>把高频配置收进同一张连续主表，让操作、校对和发布反馈都沿同一条阅读路径发生。</strong>
            </div>
            <p>主表负责承载配置本身；发布、冲突、回滚和变更摘要统一压到右侧窄反馈区，避免出现多块并列大卡片抢视线。</p>
          </div>

          <div className="config-sheet-head config-board-grid">
            <span>分类</span>
            <span>配置项</span>
            <span>当前值</span>
            <span>状态</span>
          </div>
          {configRows.map((row) => (
            <div key={row.key} className="config-sheet-row config-board-grid">
              <strong>{row.tab}</strong>
              <code>{row.key}</code>
              <span>{row.value}</span>
              <em className={row.state === '待发布' ? 'warning' : 'success'}>{row.state}</em>
            </div>
          ))}

          <div className="config-summary-band">
            <div className="summary-cell">
              <label>变更摘要</label>
              <strong>当前提交涉及 2 个价格项、1 个限额项</strong>
            </div>
            <div className="summary-cell">
              <label>影响范围</label>
              <strong>用户端生图计费、后台审核上限与开发文档展示</strong>
            </div>
            <div className="summary-cell">
              <label>发布策略</label>
              <strong>先预发布，再人工确认后全量生效</strong>
            </div>
          </div>
        </section>

        <aside className="config-side-rail">
          <section className="side-strip">
            <label>发布反馈</label>
            <div className="side-strip-list">
              <div className="side-line">
                <strong>保存中</strong>
                <span>2 项配置已进入预发布态</span>
              </div>
              <div className="side-line">
                <strong>可回滚</strong>
                <span>最近一次发布版本 v2026.05.19.3</span>
              </div>
              <div className="side-line">
                <strong>冲突提醒</strong>
                <span>1 项路由配置与价格策略修改同窗发生</span>
              </div>
            </div>
          </section>

          <section className="side-strip">
            <label>最近变更</label>
            <div className="side-strip-list">
              <div className="side-line"><strong>10:24</strong><span>更新 max_image_count：3 {'>'} 5</span></div>
              <div className="side-line"><strong>10:11</strong><span>确认积分精度支持到小数点后五位</span></div>
              <div className="side-line"><strong>09:52</strong><span>发布 OpenAI Compat generate/edit 路由表</span></div>
            </div>
          </section>
        </aside>
      </section>
    </section>
  )
}

export function RoutingPage() {
  return (
    <section className="page-stack">
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="lane-head compact">
            <div>
              <label>模型路由</label>
              <strong>OpenAI Compat 与 Provider 路由策略</strong>
            </div>
          </div>
          <div className="table-head route-grid">
            <span>场景</span>
            <span>Provider</span>
            <span>策略</span>
            <span>备注</span>
          </div>
          {routeRows.map((row) => (
            <div key={row.scene} className="table-row route-grid">
              <strong>{row.scene}</strong>
              <span>{row.provider}</span>
              <span>{row.policy}</span>
              <span>{row.note}</span>
            </div>
          ))}

          <div className="lane-divider" />

          <div className="lane-head compact">
            <div>
              <label>错误策略</label>
              <strong>收到上游错误后，先按策略分类再决定是否透出。</strong>
            </div>
          </div>
          <div className="table-head policy-grid">
            <span>上游错误</span>
            <span>处理动作</span>
            <span>策略说明</span>
          </div>
          {providerPolicyRows.map((row) => (
            <div key={row.code} className="table-row policy-grid">
              <strong>{row.code}</strong>
              <span>{row.action}</span>
              <span>{row.detail}</span>
            </div>
          ))}
        </section>
      </section>
    </section>
  )
}

export function PricingPage() {
  return (
    <section className="page-stack">
      <section className="pg-admin-card overview-surface">
        <section className="main-lane pricing-lane">
          <div className="lane-head">
            <div>
              <label>价格矩阵</label>
              <strong>基础价格、倍率与图生图附加统一在一页确认。</strong>
            </div>
          </div>

          <div className="table-head price-grid">
            <span>模型分组</span>
            <span>1K</span>
            <span>2K</span>
            <span>4K</span>
          </div>
          {pricingRows.map((row) => (
            <div key={row.group} className="table-row price-grid">
              <strong>{row.group}</strong>
              <span>{row.q1k}</span>
              <span>{row.q2k}</span>
              <span>{row.q4k}</span>
            </div>
          ))}

          <div className="lane-divider" />

          <div className="formula-strip">
            <div className="formula-line">
              <label>汇率</label>
              <strong>1 积分 = 0.31250 元</strong>
            </div>
            <div className="formula-line">
              <label>图生图附加</label>
              <strong>基础模型价格 x 参考图系数 x 图片数量</strong>
            </div>
            <div className="formula-line wide">
              <label>完整伪公式</label>
              <strong>总积分 = 基础单价(模型分组, 分辨率桶) x 输出张数 x 图生图系数(任务类型, 参考图数量) x 用户分组倍率</strong>
            </div>
          </div>
        </section>

        <aside className="signal-rail">
          <section className="signal-section">
            <div className="section-head compact">
              <strong>倍率规则</strong>
            </div>
            <div className="compact-item"><span>默认用户组</span><p>1.00000</p></div>
            <div className="compact-item"><span>渠道合作组</span><p>0.85000</p></div>
            <div className="compact-item"><span>高级会员组</span><p>0.92000</p></div>
          </section>
        </aside>
      </section>
    </section>
  )
}

export function ReviewQueuePage() {
  return (
    <section className="page-stack">
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="lane-head">
            <div>
              <label>审核队列</label>
              <strong>公开图片人工审核</strong>
            </div>
            <div className="micro-tabs">
              <span>待审核</span>
              <span>已通过</span>
              <span>已驳回</span>
            </div>
          </div>

          <div className="table-head review-grid">
            <span>图片</span>
            <span>用户</span>
            <span>类型</span>
            <span>上下文</span>
            <span>审核备注</span>
            <span>动作</span>
          </div>
          {reviewRows.map((row) => (
            <div key={row.title} className="table-row review-grid">
              <strong>{row.title}</strong>
              <span>{row.owner}</span>
              <span>{row.type}</span>
              <span>{row.detail}</span>
              <span>{row.reason}</span>
              <div className="row-actions">
                <a href="#">通过</a>
                <a href="#">驳回</a>
              </div>
            </div>
          ))}
        </section>
      </section>
    </section>
  )
}

export function AuditLogPage() {
  return (
    <section className="page-stack">
      <section className="pg-admin-card filter-band">
        <div className="filter-row">
          <strong>审计过滤</strong>
          <span>筛选：配置变更 / 价格更新 / 审核决策 / Provider 策略 / 身份认证</span>
          <button type="button" className="ghost">导出日志</button>
        </div>
      </section>

      <section className="pg-admin-card timeline-surface">
        {auditRows.concat(providerPolicyRows.map((row) => `${row.code} -> ${row.action}`)).map((row) => (
          <div key={row} className="timeline-item">
            <strong>{row}</strong>
            <span>记录操作者、变更前后值、request id 与回滚版本号。</span>
          </div>
        ))}
      </section>
    </section>
  )
}
