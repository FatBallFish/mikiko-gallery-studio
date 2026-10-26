import { PageHeader } from '../components'

export function PricingPage({ onFeedback }: { onFeedback?: (title: string, detail?: string) => void }) {
  void onFeedback
  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Pricing · Moved"
        title="价格策略已并入路由模型"
        description="价格不再作为独立菜单维护。每个路由在编辑抽屉的价格页签内统一配置图片与视频售价。"
        primaryAction={<a className="a-btn a-btn--primary" href="#/routing">前往路由模型</a>}
      />

      <div className="a-banner a-banner--info">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
        <div>
          <strong style={{ fontSize: 13 }}>统一入口</strong>
          <div className="a-hint">路由模型 → 编辑路由 → 价格页签。图片价格按任务类型 × 基础分辨率 × 参考图倍率维护；视频售价由候选厂商原生费率与最高成本推导。</div>
        </div>
      </div>

      <section className="a-panel a-panel--pad">
        <div className="a-section-title">
          <div>
            <h2>价格配置的新位置</h2>
            <p>价格与路由、候选、视频参数在同一个抽屉中，缺价或未映射会在提交前阻断。</p>
          </div>
        </div>
        <div className="a-grid a-grid--3">
          <div className="a-panel" style={{ boxShadow: 'none', padding: 15 }}>
            <div className="a-hint">图片价格</div>
            <strong style={{ fontSize: 15 }}>按基础分辨率</strong>
            <div className="a-hint" style={{ marginTop: 5 }}>文生图 / 图片编辑 × 1K / 2K / 4K 单张积分与参考图倍率。</div>
          </div>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 15 }}>
            <div className="a-hint">视频价格</div>
            <strong style={{ fontSize: 15 }}>按原生费率推导</strong>
            <div className="a-hint" style={{ marginTop: 5 }}>候选厂商原生费率 + 最高成本 + 全局汇率 + 最低积分 / 取整步长。</div>
          </div>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 15 }}>
            <div className="a-hint">缺价阻断</div>
            <strong style={{ fontSize: 15 }}>提交前校验</strong>
            <div className="a-hint" style={{ marginTop: 5 }}>缺少组合价格或分辨率映射时，用户端不展示该能力。</div>
          </div>
        </div>
        <div className="a-row-actions" style={{ marginTop: 16 }}>
          <a className="a-btn a-btn--sm" href="#/routing">返回路由模型</a>
          <a className="a-btn a-btn--sm" href="#/access-accounts">查看接入账号与模型能力</a>
        </div>
      </section>
    </section>
  )
}