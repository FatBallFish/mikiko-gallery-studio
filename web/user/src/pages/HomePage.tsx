import { useMemo, useState } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, GalleryImageFrame, ImageLightbox, LocalFeedback, StatusRail, type ImageLightboxPayload, useApp } from '../components'
import { useApiResource } from '../useApiResource'
import { ArrowRight, Image as ImageIcon, RefreshCw, Sparkles } from '../ui/icons'
import { galleryImageAspect } from './galleryExperience'
import { curatedHomeGallery, homeAccountReadinessView, homeContinuationView, homeGalleryCardView, homeModelReadinessView, homeRecentTaskView, newestHomeTask } from './homeGalleryModel'

const homeClasses = {
  content: 'mx-auto w-full max-w-[1440px] flex-1 px-6 pb-28 pt-8 md:px-10 md:pb-16 md:pt-12',
  continuation: 'grid min-h-[300px] grid-cols-[minmax(0,1.2fr)_minmax(18rem,.8fr)] items-end gap-10 border-b border-[var(--border)] pb-12 max-[840px]:grid-cols-1 max-[840px]:gap-8',
  kicker: 'mb-4 inline-flex items-center gap-2 text-xs font-semibold text-[var(--accent)]',
  title: 'm-0 max-w-[18ch] font-vault-display text-[clamp(2.6rem,6vw,5.5rem)] font-medium leading-[.98] text-[var(--fg)]',
  lead: 'mb-0 mt-6 max-w-[58ch] text-sm leading-7 text-[var(--muted)] md:text-base',
  actions: 'mt-7 flex flex-wrap items-center gap-3',
  recent: 'border-l border-[var(--border)] pl-8 max-[840px]:border-l-0 max-[840px]:border-t max-[840px]:pl-0 max-[840px]:pt-6',
  recentLabel: 'mb-3 block text-xs font-semibold text-[var(--muted)]',
  recentTitle: 'm-0 text-xl font-semibold text-[var(--fg)]',
  recentDetail: 'mb-0 mt-3 text-sm leading-6 text-[var(--muted)]',
  section: 'pt-16 md:pt-24',
  sectionHeader: 'mb-7 flex flex-wrap items-end justify-between gap-5 border-b border-[var(--border)] pb-5',
  sectionTitle: 'm-0 font-vault-display text-[clamp(2rem,4vw,3.75rem)] font-medium leading-none text-[var(--fg)]',
  sectionDetail: 'mb-0 mt-3 max-w-[60ch] text-sm leading-6 text-[var(--muted)]',
  gallery: 'grid grid-cols-3 gap-x-5 gap-y-9 max-[980px]:grid-cols-2 max-[620px]:grid-cols-1',
  galleryItem: 'min-w-0',
  galleryTitle: 'mb-0 mt-3 line-clamp-2 text-sm font-semibold leading-6 text-[var(--fg)]',
  galleryMeta: 'mt-1 block truncate font-vault-mono text-[11px] text-[var(--muted)]',
}

const recentActionLabels = {
  create: '开始创作',
  continue: '查看任务进度',
  retry: '查看并重试',
  inspect: '查看任务结果',
} as const

export function HomePage() {
  const app = useApp()
  const balance = useApiResource(() => userApi.getBalance(), [])
  const capabilities = useApiResource(() => userApi.getCapabilities(), [])
  const tasks = useApiResource(() => userApi.listTasks(), [])
  const publicGallery = useApiResource(() => openApi.listPublicGallery(1, 12, { sort: 'hot', accessToken: null }), [])
  const [imagePreview, setImagePreview] = useState<ImageLightboxPayload | null>(null)

  const latestTask = useMemo(() => newestHomeTask(tasks.data ?? []), [tasks.data])
  const continuation = useMemo(() => homeContinuationView(tasks.data ?? []), [tasks.data])
  const recent = homeRecentTaskView(latestTask, tasks.loading)
  const account = homeAccountReadinessView(balance.data)
  const models = homeModelReadinessView(capabilities.data, capabilities.loading)
  const curated = useMemo(() => curatedHomeGallery(publicGallery.data?.items ?? [], 6), [publicGallery.data?.items])

  function openImage(image: ImageResult) {
    const source = image.url || image.download_url
    if (!source) return
    setImagePreview({
      url: userApi.imageAssetUrl(source, null),
      downloadUrl: userApi.imageAssetUrl(image.download_url || source, null),
      alt: image.prompt_excerpt || image.id,
      prompt: image.prompt_excerpt,
      width: image.width,
      height: image.height,
      ratio: image.aspect_ratio,
      model: image.route_model_code || image.abstract_model,
      source: '精选灵感',
    })
  }

  return (
    <main className={homeClasses.content}>
      <section className={homeClasses.continuation} aria-labelledby="home-continuation-title">
        <div>
          <span className={homeClasses.kicker}><Sparkles size={15} strokeWidth={1.5} aria-hidden="true" /> 今天的创作入口</span>
          <h1 id="home-continuation-title" className={homeClasses.title}>回到下一张图像</h1>
          <p className={homeClasses.lead}>继续最近的任务，或者带着一个新想法进入创作台。模型状态、可用积分与历史结果都会留在同一条路径上。</p>
          <div className={homeClasses.actions}>
            <Button onClick={() => app.navigate(continuation.route, { taskId: continuation.taskId })}>{continuation.label}<ArrowRight size={17} strokeWidth={1.5} aria-hidden="true" /></Button>
            <Button tone="ghost" onClick={() => app.navigate('gallery')}>查看历史资产</Button>
          </div>
        </div>
        <div className={homeClasses.recent}>
          <span className={homeClasses.recentLabel}>最近任务</span>
          <h2 className={homeClasses.recentTitle}>{recent.title}</h2>
          <p className={homeClasses.recentDetail}>{recent.detail}</p>
          {recent.state === 'failed' ? <LocalFeedback className="mt-5" tone="error" title="可以调整参数后重试" detail="失败任务不会按成功图片数量扣费。" /> : null}
          {recent.action !== 'none' ? (
            <Button className="mt-5" tone="ghost" onClick={() => app.navigate('genpic', { taskId: latestTask?.id })}>
              {recentActionLabels[recent.action]}
              <ArrowRight size={16} strokeWidth={1.5} aria-hidden="true" />
            </Button>
          ) : null}
        </div>
      </section>

      <StatusRail
        label="创作就绪状态"
        className="px-0 py-5"
        items={[
          { key: 'models', label: '生图能力', value: models.value, tone: models.warning ? 'warning' : 'success' },
          { key: 'balance', label: '可用积分', value: balance.loading ? '读取中' : account.availableValue, tone: Number.parseFloat(balance.data?.available_points ?? '0') > 0 ? 'success' : 'warning' },
          { key: 'trial', label: '体验额度', value: balance.loading ? '读取中' : account.trialValue, tone: account.trialWarning ? 'warning' : 'neutral' },
        ]}
      />

      {(capabilities.error || balance.error || tasks.error) ? (
        <ErrorState message={capabilities.error || balance.error || tasks.error || '首页状态读取失败'} onRetry={() => void Promise.all([capabilities.reload(), balance.reload(), tasks.reload()])} />
      ) : null}

      <section className={homeClasses.section} aria-labelledby="home-inspiration-title">
        <div className={homeClasses.sectionHeader}>
          <div>
            <h2 id="home-inspiration-title" className={homeClasses.sectionTitle}>精选灵感</h2>
            <p className={homeClasses.sectionDetail}>从已公开作品中选取少量代表性结果，聚焦构图、模型与比例，不让信息流淹没下一次创作。</p>
          </div>
          <Button tone="ghost" onClick={() => app.navigate('public-gallery')}>浏览公开广场<ArrowRight size={16} strokeWidth={1.5} aria-hidden="true" /></Button>
        </div>

        {publicGallery.loading && !curated.length ? <HomeGallerySkeleton /> : null}
        {publicGallery.error && !curated.length ? <ErrorState message={publicGallery.error} onRetry={() => void publicGallery.reload()} /> : null}
        {!publicGallery.loading && !publicGallery.error && !curated.length ? <EmptyState icon={<ImageIcon size={26} strokeWidth={1.5} />} title="暂无精选作品" detail="公开广场还没有审核通过的图片。" /> : null}
        {curated.length ? (
          <div className={homeClasses.gallery}>
            {curated.map((image) => {
              const card = homeGalleryCardView(image)
              const src = image.url || image.download_url
              return (
                <article className={homeClasses.galleryItem} key={image.id}>
                  <GalleryImageFrame
                    src={src ? userApi.imageAssetUrl(src, null) : undefined}
                    alt={card.title}
                    width={image.width}
                    height={image.height}
                    aspectRatio={galleryImageAspect({ width: image.width, height: image.height, aspectRatio: image.aspect_ratio })}
                    onOpen={() => openImage(image)}
                  />
                  <h3 className={homeClasses.galleryTitle}>{card.title}</h3>
                  <span className={homeClasses.galleryMeta}>{card.meta}</span>
                </article>
              )
            })}
          </div>
        ) : null}
      </section>
      <ImageLightbox image={imagePreview} onClose={() => setImagePreview(null)} />
    </main>
  )
}

function HomeGallerySkeleton() {
  return (
    <div className={homeClasses.gallery} aria-label="正在读取精选作品">
      {Array.from({ length: 6 }).map((_, index) => <div key={index} className="pg-skeleton aspect-[4/3] rounded-2xl" />)}
    </div>
  )
}
