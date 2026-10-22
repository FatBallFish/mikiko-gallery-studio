import { useMemo, useRef, useState } from 'react'
import type { ImageResult, MediaAccessProjection } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, GalleryImageFrame, ImageDetailModal, copyText, useApp } from '../components'
import { useApiResource } from '../useApiResource'
import { mediaAccess } from '../mediaAccess'
import { ArrowRight, Image as ImageIcon, Sparkles } from '../ui/icons'
import { galleryImageAspect } from './galleryExperience'
import { curatedHomeGallery, homeAccountReadinessView, homeContinuationView, homeGalleryCardView, homeModelReadinessView, homePublicDetailImage, homeRecentTaskView, newestHomeTask } from './homeGalleryModel'

const homeClasses = {
  gallery: 'u-grid u-grid--gallery',
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
  const [selectedImage, setSelectedImage] = useState<ImageResult | null>(null)
  const [publicImageAccess, setPublicImageAccess] = useState<Record<string, MediaAccessProjection>>({})
  const publicDetailRequestRef = useRef(0)

  const latestTask = useMemo(() => newestHomeTask(tasks.data ?? []), [tasks.data])
  const continuation = useMemo(() => homeContinuationView(tasks.data ?? []), [tasks.data])
  const recent = homeRecentTaskView(latestTask, tasks.loading)
  const account = homeAccountReadinessView(balance.data)
  const models = homeModelReadinessView(capabilities.data, capabilities.loading)
  const curated = useMemo(() => curatedHomeGallery(publicGallery.data?.items ?? [], 6), [publicGallery.data?.items])

  async function openImage(image: ImageResult) {
    const access = publicImageAccess[image.id]
    const resolvedImage = access ? { ...image, url: access.url, download_url: access.url, preview_expires_at: access.expires_at } : image
    const source = resolvedImage.url || resolvedImage.download_url
    if (!source) return
    const request = publicDetailRequestRef.current + 1
    publicDetailRequestRef.current = request
    const summary = homePublicDetailImage(resolvedImage)
    setSelectedImage({ ...summary, prompt: undefined })
    try {
      const detail = await openApi.getPublicGalleryImage(image.id, { accessToken: app.session?.token })
      if (publicDetailRequestRef.current !== request) return
      if (!detail.prompt?.trim()) throw new Error('完整 Prompt 暂时无法读取，请稍后再试。')
      setSelectedImage({
        ...summary,
        ...detail,
        url: access?.url || detail.url || summary.url,
        download_url: access?.url || detail.download_url || summary.download_url,
        preview_expires_at: access?.expires_at || detail.preview_expires_at || summary.preview_expires_at,
        prompt: detail.prompt,
      })
    } catch (error) {
      if (publicDetailRequestRef.current !== request) return
      app.notify('error', error instanceof Error ? error.message : '完整 Prompt 读取失败')
    }
  }

  async function refreshPublicImage(imageId: string) {
    const projection = await mediaAccess.preview({ kind: 'image', scope: 'public', id: imageId })
    setPublicImageAccess((current) => ({ ...current, [imageId]: projection }))
    setSelectedImage((current) => current?.id === imageId ? { ...current, url: projection.url, download_url: projection.url, preview_expires_at: projection.expires_at } : current)
    return projection.url
  }

  async function downloadPublicImage(image: ImageResult) {
    try {
      const projection = await mediaAccess.download({ kind: 'image', scope: 'public', id: image.id })
      const link = document.createElement('a')
      link.href = projection.url
      link.download = `mikiko-${image.id}`
      link.rel = 'noopener noreferrer'
      document.body.appendChild(link)
      link.click()
      link.remove()
    } catch (error) {
      app.notify('error', error instanceof Error ? error.message : '下载地址刷新失败')
    }
  }

  const taskProgress = latestTask && (latestTask.status === 'queued' || latestTask.status === 'running')
    ? Math.max(0, Math.min(100, Math.round(latestTask.progress || 0)))
    : 0

  return (
    <main>
      <div className="hm-return">
        <div className="hm-return__copy">
          <div style={{ display: 'grid', gap: 14 }}>
            <span className="u-eyebrow">Continue where you left</span>
            <h1 id="home-continuation-title">回到<em>下一张</em>图像</h1>
            <p className="u-sub">继续最近的任务，或者带着一个新想法进入创作台。模型状态、可用积分与历史结果都会留在同一条路径上。</p>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
            <Button onClick={() => app.navigate(continuation.route, { taskId: continuation.taskId })}>{continuation.label}<ArrowRight size={17} strokeWidth={1.5} aria-hidden="true" /></Button>
            <Button tone="ghost" onClick={() => app.navigate('gallery')}>查看全部资产</Button>
            <Button tone="ghost" onClick={() => app.navigate('public-gallery')}>寻找灵感</Button>
          </div>
        </div>
        <aside className="hm-task">
          <div className="u-panel-title" style={{ margin: 0 }}><h3>最近任务</h3><span>{recent.state === 'active' ? `${taskProgress}%` : recent.state === 'complete' ? '已完成' : recent.state === 'failed' ? '失败' : '等待开始'}</span></div>
          <div className="hm-task__stage">
            <div style={{ display: 'grid', placeItems: 'center', height: '100%', gap: 8, color: 'var(--ink-3)' }}>
              <Sparkles size={28} strokeWidth={1.2} aria-hidden="true" />
              <span className="u-meta">{recent.title}</span>
            </div>
            <span className="u-chip u-chip--accent hm-task__badge"><span className="u-dot" aria-hidden="true" />{recent.state === 'active' ? `生成中 · ${taskProgress}%` : recent.state === 'failed' ? '需要重试' : recent.state === 'complete' ? '已完成' : '待开始'}</span>
          </div>
          <div style={{ display: 'grid', gap: 6 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10 }}>
              <strong style={{ fontSize: 13 }}>{recent.detail}</strong>
              <span className="u-meta">{latestTask?.route_model_code || latestTask?.abstract_model || '—'}</span>
            </div>
            <div className="u-progress"><span style={{ width: `${recent.state === 'active' ? taskProgress : recent.state === 'complete' ? 100 : 0}%` }} /></div>
            <div className="u-meta">{latestTask ? `${latestTask.aspect_ratio || '-'} · ${latestTask.image_count || '—'} 张` : '还没有生成记录'}</div>
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            {recent.action !== 'none' ? (
              <Button tone="ghost" onClick={() => app.navigate('genpic', { taskId: latestTask?.id })}>{recentActionLabels[recent.action]}</Button>
            ) : null}
            <Button tone="ghost" onClick={() => app.navigate('genpic')}>新任务</Button>
          </div>
        </aside>
      </div>

      <div className="u-stat-row" style={{ marginBottom: 34 }}>
        <div className="u-stat"><span className="u-stat__label">生图能力</span><span className="u-stat__value" style={{ fontSize: 19, lineHeight: 1.35 }}>{models.value}</span><span className="u-stat__foot">{models.warning ? '部分模型不可用' : '模型通道可用'}</span></div>
        <div className="u-stat"><span className="u-stat__label">可用积分</span><span className="u-stat__value" style={{ fontSize: 19, lineHeight: 1.35 }}>{balance.loading ? '读取中' : account.availableValue}</span><span className="u-stat__foot">冻结与体验额度独立计算</span></div>
        <div className="u-stat"><span className="u-stat__label">体验额度</span><span className="u-stat__value" style={{ fontSize: 19, lineHeight: 1.35 }}>{balance.loading ? '读取中' : account.trialValue}</span><span className="u-stat__foot">{account.trialWarning ? '体验额度即将用尽' : '可用于首次生成'}</span></div>
        <div className="u-stat"><span className="u-stat__label">最近任务</span><span className="u-stat__value">{tasks.loading ? '…' : tasks.data?.length ?? 0}</span><span className="u-stat__foot">全部历史任务</span></div>
      </div>

      {(capabilities.error || balance.error || tasks.error) ? (
        <ErrorState message={capabilities.error || balance.error || tasks.error || '首页状态读取失败'} onRetry={() => void Promise.all([capabilities.reload(), balance.reload(), tasks.reload()])} />
      ) : null}

      <section style={{ marginTop: 36 }} aria-labelledby="home-inspiration-title">
        <div className="u-section-head">
          <div style={{ display: 'grid', gap: 6 }}>
            <span className="u-eyebrow">Curated Inspiration</span>
            <h2 id="home-inspiration-title" className="u-h2">精选灵感</h2>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span className="u-meta">今天从公开广场挑选</span>
            <Button tone="ghost" onClick={() => app.navigate('public-gallery')}>浏览广场</Button>
          </div>
        </div>

        {publicGallery.loading && !curated.length ? <HomeGallerySkeleton /> : null}
        {publicGallery.error && !curated.length ? <ErrorState message={publicGallery.error} onRetry={() => void publicGallery.reload()} /> : null}
        {!publicGallery.loading && !publicGallery.error && !curated.length ? <EmptyState icon={<ImageIcon size={26} strokeWidth={1.5} />} title="暂无精选作品" detail="公开广场还没有审核通过的图片。" /> : null}
        {curated.length ? (
          <div className={homeClasses.gallery}>
            {curated.map((image) => {
              const card = homeGalleryCardView(image)
              const access = publicImageAccess[image.id]
              const src = access?.url || image.url || image.download_url
              return (
                <article className="u-card" key={image.id}>
                  <div className="u-card__media">
                    <GalleryImageFrame
                      src={src ? userApi.imageAssetUrl(src, null) : undefined}
                      mediaExpiresAt={publicImageAccess[image.id]?.expires_at ?? image.preview_expires_at}
                      alt={card.title}
                      width={image.width}
                      height={image.height}
                      aspectRatio={galleryImageAspect({ width: image.width, height: image.height, aspectRatio: image.aspect_ratio })}
                      onOpen={() => void openImage(image)}
                      onMediaRefresh={() => refreshPublicImage(image.id)}
                      className="h-full w-full rounded-none border-0"
                    />
                    <span className="u-card__tag"><ImageIcon size={11} strokeWidth={1.6} aria-hidden="true" />图片</span>
                  </div>
                  <div className="u-card__body">
                    <div className="u-card__title">{card.title}</div>
                    <div className="u-card__meta"><span>{card.meta}</span><span>{image.width && image.height ? `${image.width}×${image.height}` : ''}</span></div>
                  </div>
                </article>
              )
            })}
          </div>
        ) : null}
      </section>
      <ImageDetailModal
        title="精选作品详情"
        image={selectedImage}
        imageUrl={selectedImage?.url || selectedImage?.download_url ? userApi.imageAssetUrl(selectedImage.url || selectedImage.download_url || '', null) : undefined}
        onDownload={(image) => void downloadPublicImage(image as ImageResult)}
        onCopyPrompt={async (prompt) => {
          await copyText(prompt)
          app.notify('success', 'Prompt 已复制')
        }}
        previewSourceLabel="精选灵感"
        onMediaRefresh={selectedImage ? () => refreshPublicImage(selectedImage.id) : undefined}
        onClose={() => { publicDetailRequestRef.current += 1; setSelectedImage(null) }}
      />
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
