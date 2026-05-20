export type UserPageId = 'recommend' | 'reference' | 'text' | 'history' | 'access'

const trendingWorks = [
  { title: 'Cloud Museum', meta: '本周热门 / Plus / 2K', skin: 'one' },
  { title: 'Amber Cathedral', meta: '参考图生成 / 已公开', skin: 'two' },
  { title: 'Mercury Figure', meta: '局部编辑 / 蒙版任务', skin: 'three' },
  { title: 'Emerald Script', meta: '风格迁移 / 4K', skin: 'four' },
]

const historyRows = [
  { title: '镀铬花园', status: '已完成', meta: '参考图生成 / Plus / auto -> 2K / 2 张', publicState: '公开申请中' },
  { title: '静电肖像', status: '处理中', meta: '局部编辑 / Basic / 1 张', publicState: '私有' },
  { title: '矿石剧院', status: '已完成', meta: '风格迁移 / Plus / 4K / 1 张', publicState: '已公开' },
  { title: '流光手稿', status: '失败重试', meta: '参考图生成 / Plus / 2 张', publicState: '私有' },
]

const compatGroups = [
  {
    title: 'OpenAI Compat',
    detail: '兼容 generate 与 edit，适配 OpenAI / OpenRouter 双上游。',
    rows: [
      'POST /v1/images/generations - OpenAI 兼容生图接口',
      'POST /v1/images/edits - OpenAI 兼容编辑接口',
      'GET /v1/models - 返回可用图片模型列表',
    ],
  },
  {
    title: 'Web API',
    detail: '面向用户站点的业务接口，含余额、任务、历史与参考图资产。',
    rows: [
      'POST /api/tasks/image - 创建生图任务',
      'GET /api/tasks/:id - 查询任务与阶段进度',
      'GET /api/history/images - 获取历史图片及公开状态',
    ],
  },
  {
    title: 'Admin API',
    detail: '面向后台的配置、审核、价格与错误治理接口。',
    rows: [
      'GET /admin/config/items - 配置中心分页查询',
      'POST /admin/public-images/:id/review - 审核公开图片',
      'GET /admin/provider-error-policies - 错误策略列表',
    ],
  },
]

export function RecommendPage() {
  return (
    <section className="page-stack">
      <section className="surface-band recommend-stage">
        <div className="recommend-hero">
          <div className="section-title-row">
            <div>
              <label>今日推荐</label>
              <strong>首页聚焦开始创作、热门审美方向与最近结果。</strong>
            </div>
            <span>让第一次进入平台的用户快速理解这里是一个艺术生成工作台。</span>
          </div>

          <div className="hero-layout">
            <div className="hero-artwork">
              <div className="hero-copy">
                <span className="live-chip">Featured Theme</span>
                <strong>金属织幕、剧院级体积光与富色调材质，是今天的推荐方向。</strong>
                <p>推荐内容不做信息堆叠，只留一条主入口、一个主题方向和几个可以立刻点击开始的创作入口。</p>
                <div className="action-row">
                  <button type="button">从推荐主题开始</button>
                  <button type="button" className="ghost">上传参考图</button>
                </div>
              </div>
            </div>

            <div className="hero-side-feed">
              <div className="feed-line">
                <label>快速入口</label>
                <strong>参考图生成</strong>
                <span>适合角色延展、单图二创、参考风格生成。</span>
              </div>
              <div className="feed-line">
                <label>今日灵感</label>
                <strong>光泽体块 + 深色背景</strong>
                <span>更适合搭配 Plus 模型与 2K 输出。</span>
              </div>
              <div className="feed-line">
                <label>最近结果</label>
                <strong>上次生成成功率 98.7%</strong>
                <span>点击可直接回到你的最近任务链路。</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="surface-band strip-section">
        <div className="section-title-row compact">
          <div>
            <label>热门作品</label>
            <strong>推荐流不做卡片墙，改成连续画带。</strong>
          </div>
          <a href="#history">查看完整图片广场</a>
        </div>
        <div className="visual-strip">
          {trendingWorks.map((item) => (
            <article key={item.title} className={`visual-tile ${item.skin}`}>
              <div>
                <strong>{item.title}</strong>
                <span>{item.meta}</span>
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="surface-band action-matrix">
        <div className="matrix-line">
          <div>
            <label>流程一</label>
            <strong>上传参考图</strong>
          </div>
          <span>支持单图编辑、角色图 + 风格图组合输入。</span>
        </div>
        <div className="matrix-line">
          <div>
            <label>流程二</label>
            <strong>选择模型与分辨率</strong>
          </div>
          <span>auto 会解析到 1K / 2K / 4K，并实时重算积分。</span>
        </div>
        <div className="matrix-line">
          <div>
            <label>流程三</label>
            <strong>同步看结果与历史</strong>
          </div>
          <span>生成完成后可再次编辑、申请公开或复用为参考图。</span>
        </div>
      </section>
    </section>
  )
}

export function ReferenceStudioPage() {
  const materialChips = ['角色图已锁定', '风格图已锁定', '支持继续添加参考图']
  const controlRows = [
    ['模型分组', 'Plus Image'],
    ['分辨率', 'auto 解析为 2K'],
    ['宽高比', '16:9 影院屏'],
    ['图片数量', '2 张'],
  ]

  return (
    <section className="page-stack">
      <section className="surface-band reference-motherboard">
        <section className="reference-command-rail pg-glass-panel">
          <div className="reference-band flow-band">
            <label>创作流程</label>
            <div className="flow-steps">
              <div className="flow-step active"><span>01</span><strong>上传参考图</strong></div>
              <div className="flow-step"><span>02</span><strong>补全描述</strong></div>
              <div className="flow-step"><span>03</span><strong>确认参数</strong></div>
              <div className="flow-step"><span>04</span><strong>同步看结果</strong></div>
            </div>
          </div>

          <div className="reference-band material-band">
            <div className="material-preview" />
            <div className="material-copy">
              <label>参考素材</label>
              <strong>角色、场景、风格素材在这里合流，不再拆成多个分散小卡块。</strong>
              <p>优先处理素材上传、素材状态与继续添加动作，让用户在一个连续区域内完成图生图前置准备。</p>
              <div className="action-row compact-row">
                <button type="button">上传图片</button>
                <button type="button" className="ghost">添加主体图</button>
              </div>
            </div>
            <div className="material-chip-strip">
              {materialChips.map((item) => (
                <span key={item}>{item}</span>
              ))}
            </div>
          </div>

          <div className="reference-band prompt-band">
            <div className="prompt-band-head">
              <div className="prompt-tab active">正向描述</div>
              <div className="prompt-tab">限制词</div>
            </div>
            <textarea
              readOnly
              value="设计一个带金属编织感的未来艺廊大厅，主色为深蓝、琥珀与孔雀绿，空间中悬浮半透明雕塑与镜面反射，保留参考图中的主体姿态与材质层次，整体氛围偏高级时尚展陈。"
            />
          </div>

          <div className="reference-band control-band">
            <label>生成参数</label>
            <div className="control-sheet">
              {controlRows.map(([name, value]) => (
                <div key={name} className="control-sheet-row">
                  <span>{name}</span>
                  <strong>{value}</strong>
                </div>
              ))}
            </div>
          </div>

          <div className="reference-band billing-band">
            <div>
              <label>费用预估</label>
              <strong>12.50000 积分</strong>
            </div>
            <p>Plus × auto {'>'} 2K × 图生图 × 2 张 × 用户倍率 1.00</p>
          </div>
        </section>

        <section className="reference-output-zone">
          <div className="output-topline">
            <div className="output-topline-left">
              <span className="live-chip">Live Preview</span>
              <strong>结果画布、任务状态与操作入口收在一个主视区。</strong>
            </div>
            <div className="output-topline-right">
              <span>OpenAI 主路由</span>
              <span>同步回显</span>
              <span>队列 02</span>
            </div>
          </div>

          <div className="output-canvas-shell canvas-area">
            <div className="output-canvas-art" />
            <div className="output-floating-panel">
              <label>结果预览区</label>
              <strong>大画布仍然是主角，局部状态只做贴边反馈，不拆独立公告块。</strong>
              <p>生成前显示构图占位与安全边界；生成中展示阶段进度；完成后支持放大、下载、公开申请与再次编辑。</p>
              <div className="action-row">
                <button type="button">开始生成</button>
                <button type="button" className="ghost">再次编辑</button>
              </div>
            </div>
          </div>

          <div className="output-feedback-band">
            <div className="feedback-cell">
              <label>参考图状态</label>
              <strong>角色图 1 / 风格图 1</strong>
              <span>素材校验完成，可继续补图。</span>
            </div>
            <div className="feedback-cell">
              <label>积分变动</label>
              <strong>预扣 12.50000</strong>
              <span>任务失败自动返还，部分成功按成功张数结算。</span>
            </div>
            <div className="feedback-cell">
              <label>任务回显</label>
              <strong>排队中 {'>'} 处理中 {'>'} 成功</strong>
              <span>失败时会显示重试原因与 provider 路由信息。</span>
            </div>
          </div>
        </section>
      </section>

      <section className="surface-band reference-footstrip">
        <div className="footstrip-heading">
          <label>最近结果</label>
          <strong>历史结果仅作为延续创作的素材条，不抢当前工作台焦点。</strong>
        </div>
        <div className="footstrip-film">
          <article className="film-cell film-one"><strong>镀铬花园</strong><span>参考图生成 / Plus / 2K</span></article>
          <article className="film-cell film-two"><strong>静电肖像</strong><span>局部编辑 / 蒙版 / 1 张</span></article>
          <article className="film-cell film-three"><strong>矿石剧院</strong><span>风格迁移 / 4K / 已公开</span></article>
        </div>
      </section>
    </section>
  )
}

export function TextStudioPage() {
  return (
    <section className="page-stack">
      <section className="surface-band text-workbench">
        <section className="text-left-rail pg-glass-panel">
          <div className="rail-section prompt-section dense">
            <div className="section-title-row compact">
              <div>
                <label>Prompt</label>
                <strong>文生图输入区</strong>
              </div>
              <span>更强调文本引导与风格设定。</span>
            </div>
            <textarea
              readOnly
              value="生成一组具有高奢装置艺术感的商业大片，场景为雾化金属大厅，漂浮织物与琥珀色体积光交织，镜面地面带轻微水波反射，主体位于画面中心偏左。"
            />
          </div>

          <div className="rail-section parameter-list">
            <div className="parameter-row">
              <span>模型分组</span>
              <strong>Basic Image</strong>
            </div>
            <div className="parameter-row">
              <span>风格模板</span>
              <strong>Editorial Luxe</strong>
            </div>
            <div className="parameter-row">
              <span>分辨率</span>
              <strong>1K</strong>
            </div>
            <div className="parameter-row">
              <span>图片数量</span>
              <strong>1 张</strong>
            </div>
          </div>

          <div className="rail-section estimate-section">
            <div>
              <label>费用预估</label>
              <strong>2.00000 积分</strong>
            </div>
            <p>Basic / 文生图 / 1K / 1 张 / 用户倍率 1.00</p>
          </div>
        </section>

        <section className="text-right-stage">
          <div className="stage-head text-head-split">
            <div>
              <label>风格导航</label>
              <strong>让风格模板和结果预览紧挨在一起。</strong>
            </div>
            <span>避免左边配一堆卡片、右边再塞一堆卡片的旧式后台感布局。</span>
          </div>

          <div className="style-ribbon">
            <span>金属剧场</span>
            <span>彩色烟幕</span>
            <span>高级时装</span>
            <span>杂志构图</span>
            <span>镜面材质</span>
          </div>

          <div className="stage-canvas text-canvas canvas-area">
            <div className="canvas-overlay narrow">
              <span className="live-chip">Text Driven</span>
              <strong>大画布仍然是主角，只是输入从参考图换成了文本与风格模板。</strong>
              <p>这页与参考生图区分开，但仍共享同一套反馈语言、动作按钮和价格逻辑。</p>
              <div className="action-row">
                <button type="button">用当前 Prompt 生成</button>
                <button type="button" className="ghost">保存为模板</button>
              </div>
            </div>
          </div>
        </section>
      </section>
    </section>
  )
}

export function HistoryPage() {
  return (
    <section className="page-stack">
      <section className="surface-band filter-band">
        <div className="filter-row">
          <strong>历史图片库</strong>
          <span>筛选：全部任务 / 已公开 / 待审核 / 私有 / 失败重试</span>
          <button type="button" className="ghost">批量公开设置</button>
        </div>
      </section>

      <section className="surface-band history-table-band">
        <div className="table-head-lite history-grid-head">
          <span>作品</span>
          <span>任务状态</span>
          <span>生成参数</span>
          <span>公开状态</span>
          <span>动作</span>
        </div>
        {historyRows.map((row) => (
          <div key={row.title} className="table-line history-grid-head">
            <strong>{row.title}</strong>
            <span>{row.status}</span>
            <span>{row.meta}</span>
            <span>{row.publicState}</span>
            <div className="line-actions">
              <a href="#reference">再次编辑</a>
              <a href="#">下载</a>
            </div>
          </div>
        ))}
      </section>

      <section className="surface-band strip-section">
        <div className="section-title-row compact">
          <div>
            <label>公开候选</label>
            <strong>公开图会先进入人工审核，再进入图片广场。</strong>
          </div>
          <span>这里弱提示规则，不把审核说明放成大段公告。</span>
        </div>
        <div className="visual-strip tall-strip">
          {trendingWorks.map((item) => (
            <article key={item.title} className={`visual-tile ${item.skin}`}>
              <div>
                <strong>{item.title}</strong>
                <span>{item.meta}</span>
              </div>
            </article>
          ))}
        </div>
      </section>
    </section>
  )
}

export function AccessPage() {
  return (
    <section className="page-stack">
      <section className="surface-band access-layout">
        <section className="access-keys-lane">
          <div className="section-title-row compact">
            <div>
              <label>API Keys</label>
              <strong>把密钥与文档放在同一接入场景下。</strong>
            </div>
            <span>减少“工具页”和“文档页”分裂带来的跳转感。</span>
          </div>

          <div className="key-line">
            <div>
              <label>默认 Key</label>
              <strong>pg_live_xxxxxxxx9ab2</strong>
            </div>
            <span>最近使用：2026-05-19 10:42</span>
          </div>
          <div className="key-line">
            <div>
              <label>权限范围</label>
              <strong>images.generate / images.edit / history.read</strong>
            </div>
            <span>支持按 Key 维度查看调用量和错误率。</span>
          </div>
          <div className="action-row compact-row">
            <button type="button">新建 Key</button>
            <button type="button" className="ghost">查看调用日志</button>
          </div>
        </section>

        <section className="access-docs-lane">
          <div className="section-title-row compact">
            <div>
              <label>开发文档</label>
              <strong>OpenAPI 接口按用途分组展示。</strong>
            </div>
            <a href="#">导出 OpenAPI</a>
          </div>

          <div className="doc-groups">
            {compatGroups.map((group) => (
              <div key={group.title} className="doc-group">
                <div className="doc-group-head">
                  <strong>{group.title}</strong>
                  <span>{group.detail}</span>
                </div>
                {group.rows.map((row) => (
                  <div key={row} className="doc-row">
                    <code>{row}</code>
                  </div>
                ))}
              </div>
            ))}
          </div>
        </section>
      </section>

      <section className="surface-band code-sample-band">
        <div className="section-title-row compact">
          <div>
            <label>示例代码</label>
            <strong>示例区保持品牌风格，但不做 plain docs。</strong>
          </div>
          <span>展示最关键的 generate / edit 接入姿势。</span>
        </div>
        <pre>{`curl -X POST /v1/images/edits \\
  -H "Authorization: Bearer $API_KEY" \\
  -F model=gpt-image-2 \\
  -F image=@reference.png \\
  -F prompt="Refine the metallic installation hall with emerald light"`}</pre>
      </section>
    </section>
  )
}
