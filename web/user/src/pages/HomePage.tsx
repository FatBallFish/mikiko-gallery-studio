import { useEffect, useMemo, useRef, useState } from 'react'
import type { ImageResult, PageResult } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import heroImage from '../../../../docs/template/PicGallery/mpdhezm8-image.png'
import { EmptyState, ImageLightbox, type ImageLightboxPayload, useApp } from '../components'
import { publicEngagementScore } from '../publicEngagementModel'
import { button as btn } from '../ui/redesign-classes'
import { rdGallery, rdHome } from '../ui/redesign-classes'
import { errorMessage } from '../useApiResource'
import { homeGalleryCardView } from './homeGalleryModel'
import { Image as ImageIcon, RefreshCw } from '../ui/icons'

type FilterMode = 'latest' | 'hot'

type MasonryCard = {
  id: string
  title: string
  meta: string
  imageUrl?: string
  createdAt: string
  hotScore: number
  image: ImageResult
  onClick: () => void
}

const homeClasses = {
  content: 'w-full flex-1 p-6 md:p-10',
  carousel: cn(rdHome.hero, 'mb-16 min-h-[420px] cursor-pointer border border-[var(--border)] p-10 md:p-16 max-[760px]:min-h-[360px] max-[760px]:p-6'),
  heroImage: rdHome.heroImg,
  carouselOverlay: rdHome.heroOverlay,
  heroTitle: rdHome.heroTitle,
  heroContent: rdHome.heroContent,
  heroText: rdHome.heroText,
  actionRow: rdHome.heroActions,
  sectionHead: 'mb-6 flex items-end justify-between gap-4 max-[760px]:items-start max-[760px]:flex-col',
  sectionTitle: 'm-0 text-3xl font-black md:text-4xl',
  sectionDetail: 'mt-1.5 block text-[13px] text-[var(--muted)]',
  filterGroup: 'flex flex-wrap justify-end gap-3',
  masonry: rdGallery.masonry,
  masonryItem: cn(rdGallery.itemShell, 'mb-8 block w-full break-inside-avoid overflow-hidden border-0 p-1 text-left text-[var(--fg)] active:scale-[0.98]'),
  masonryImage: cn(rdGallery.itemImg, 'h-auto opacity-90 transition duration-700 hover:scale-110 hover:opacity-100'),
  cardBody: 'p-5',
  cardTitle: 'mb-1 block text-sm font-bold',
  cardMeta: 'font-mono text-[11px] uppercase text-[var(--muted)]',
  placeholder: 'grid min-h-[220px] place-items-center bg-[color-mix(in_oklch,var(--fg)_4%,transparent)] text-[var(--muted)]',
  smallNote: 'm-0 mt-2.5 text-[13px] text-[var(--muted)]',
  button: cn(btn.base, 'min-h-0 px-4 py-1.5 text-[13px] font-semibold'),
  buttonActive: cn(btn.primary, 'font-extrabold'),
  buttonGhost: cn(btn.base, btn.ghost, 'min-h-0 px-4 py-1.5 text-[13px] font-semibold'),
  loadSentinel: 'mt-8 flex min-h-12 items-center justify-center gap-2 text-sm text-[var(--muted)]',
  skeletonCard: 'mb-8 break-inside-avoid overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface)]',
  skeletonImage: 'pg-skeleton aspect-[4/3] w-full',
  skeletonBody: 'grid gap-3 p-5',
}

const HOME_PAGE_SIZE = 16

export function HomePage() {
  const app = useApp()
  const [filter, setFilter] = useState<FilterMode>('hot')
  const [imagePreview, setImagePreview] = useState<ImageLightboxPayload | null>(null)
  const [cardsData, setCardsData] = useState<ImageResult[]>([])
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const sentinelRef = useRef<HTMLDivElement | null>(null)

  const cards = useMemo(() => {
    return cardsData.map((image) => {
      const card = homeGalleryCardView(image)
      return {
        id: image.id,
        title: card.title,
        meta: card.meta,
        imageUrl: image.url ? userApi.imageAssetUrl(image.url, app.session?.token) : undefined,
        createdAt: image.created_at ?? '',
        hotScore: publicEngagementScore(image),
        image,
        onClick: () => openImageLightbox(image),
      }
    })
  }, [app.session?.token, cardsData])

  useEffect(() => {
    let cancelled = false

    async function loadPublic(pageNo: number, mode: 'replace' | 'append') {
      if (mode === 'replace') setLoading(true)
      else setLoadingMore(true)
      try {
        const result: PageResult<ImageResult> = await openApi.listPublicGallery(pageNo, HOME_PAGE_SIZE, { sort: filter, accessToken: app.session?.token })
        if (cancelled) return
        setCardsData((current) => mode === 'replace' ? result.items : [...current, ...result.items])
        const total = result.pagination?.total ?? result.total ?? 0
        const loaded = result.items.length
        setHasMore(result.items.length === HOME_PAGE_SIZE && (total <= 0 || loaded < total))
        setPage(pageNo)
      } catch (err) {
        if (!cancelled) app.notify('error', errorMessage(err))
      } finally {
        if (!cancelled) {
          setLoading(false)
          setLoadingMore(false)
        }
      }
    }

    setCardsData([])
    setHasMore(true)
    setPage(1)
    void loadPublic(1, 'replace')
    return () => { cancelled = true }
  }, [app, app.session?.token, filter, reloadKey])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || !hasMore || loading || loadingMore) return undefined
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      void (async () => {
        setLoadingMore(true)
        try {
          const nextPage = page + 1
          const result = await openApi.listPublicGallery(nextPage, HOME_PAGE_SIZE, { sort: filter, accessToken: app.session?.token })
          let loaded = result.items.length
          setCardsData((current) => {
            const next = [...current, ...result.items]
            loaded = next.length
            return next
          })
          const total = result.pagination?.total ?? result.total ?? 0
          setHasMore(result.items.length === HOME_PAGE_SIZE && (total <= 0 || loaded < total))
          setPage(nextPage)
        } catch (err) {
          app.notify('error', errorMessage(err))
        } finally {
          setLoadingMore(false)
        }
      })()
    }, { rootMargin: '240px 0px' })
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [app, app.session?.token, filter, hasMore, loading, loadingMore, page])

  function openImageLightbox(image: ImageResult) {
    const url = image.url || image.download_url
    if (!url) return
    setImagePreview({
      url: userApi.imageAssetUrl(url, app.session?.token),
      downloadUrl: userApi.imageAssetUrl(image.download_url || url, app.session?.token),
      alt: image.prompt || image.prompt_excerpt || image.id,
      prompt: image.prompt || image.prompt_excerpt,
      width: image.width,
      height: image.height,
      ratio: image.aspect_ratio,
      model: image.route_model_code || image.abstract_model,
      source: '灵感发现',
    })
  }

  return (
    <div className={homeClasses.content}>
      <section
        className={homeClasses.carousel}
        role="button"
        tabIndex={0}
        aria-label="套用高奢腕表影棚主题"
        onClick={() => app.navigate('genpic')}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') app.navigate('genpic')
        }}
      >
        <img src={heroImage} alt="Carousel Banner" className={homeClasses.heroImage} />
        <div className={homeClasses.carouselOverlay} />
        <div className={homeClasses.heroContent}>
          <h1 className={homeClasses.heroTitle}>高奢视觉生成工坊</h1>
          <p className={homeClasses.heroText}>用统一模型、清晰参数和可追溯历史，快速完成商品图、概念稿和视觉提案。</p>
          <div className={homeClasses.actionRow}>
            <HomeButton isActive onClick={(event) => navigateFromButton(event, () => app.navigate('genpic'))}>开始创作</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('docs'))}>查看文档</HomeButton>
          </div>
        </div>
      </section>

      <section aria-labelledby="inspiration-title">
        <div className={homeClasses.sectionHead}>
          <div>
            <h2 id="inspiration-title" className={homeClasses.sectionTitle}>灵感发现</h2>
            <span className={homeClasses.sectionDetail}>
              {loading ? '正在刷新公开广场...' : '查看公开作品的模型、比例与提示词方向。'}
            </span>
          </div>
          <div className={homeClasses.filterGroup} aria-label="灵感筛选">
            <FilterButton active={filter === 'latest'} onClick={() => setFilter('latest')}>最新</FilterButton>
            <FilterButton active={filter === 'hot'} onClick={() => setFilter('hot')}>最热</FilterButton>
            <button type="button" className={cn(homeClasses.buttonGhost, 'inline-flex items-center gap-1.5')} onClick={() => setReloadKey((current) => current + 1)} aria-label="刷新">
              <RefreshCw size={14} strokeWidth={1.5} />
              刷新
            </button>
          </div>
        </div>

        {loading && !cards.length ? <HomeMasonrySkeleton /> : cards.length ? (
          <div className={cn(homeClasses.masonry, 'pg-enter')}>
            {cards.map((card) => (
              <button type="button" className={homeClasses.masonryItem} key={card.id} onClick={card.onClick}>
                {card.imageUrl ? <img src={card.imageUrl} alt={card.title} className={homeClasses.masonryImage} /> : <ImagePlaceholder />}
                <div className={homeClasses.cardBody}>
                  <strong className={homeClasses.cardTitle}>{card.title}</strong>
                  <span className={homeClasses.cardMeta}>{card.meta}</span>
                </div>
              </button>
            ))}
          </div>
        ) : (
          <EmptyState title="暂无公开灵感" detail="当前公开广场还没有可展示的作品。" />
        )}
        <div ref={sentinelRef} className={homeClasses.loadSentinel}>
          {loadingMore ? (
            <div className="grid w-full max-w-sm gap-2">
              <div className="pg-skeleton h-4 rounded-xl" />
              <div className="pg-skeleton h-4 w-2/3 rounded-xl" />
            </div>
          ) : hasMore ? '继续下滑加载更多' : cards.length ? '已经到底了' : ''}
        </div>
      </section>
      <ImageLightbox image={imagePreview} onClose={() => setImagePreview(null)} />
    </div>
  )
}

function navigateFromButton(event: React.MouseEvent<HTMLButtonElement>, navigate: () => void) {
  event.stopPropagation()
  navigate()
}

function HomeButton({ children, isActive = false, onClick }: { children: React.ReactNode; isActive?: boolean; onClick: React.MouseEventHandler<HTMLButtonElement> }) {
  return (
    <button type="button" className={cn(homeClasses.button, isActive && homeClasses.buttonActive)} onClick={onClick}>
      {children}
    </button>
  )
}

function FilterButton({ children, active, onClick }: { children: React.ReactNode; active: boolean; onClick: () => void }) {
  return (
    <button type="button" aria-pressed={active} className={cn(homeClasses.button, active && homeClasses.buttonActive)} onClick={onClick}>
      {children}
    </button>
  )
}

function ImagePlaceholder() {
  return (
    <div className={homeClasses.placeholder}>
      <ImageIcon size={48} strokeWidth={1} aria-hidden="true" />
    </div>
  )
}

function HomeMasonrySkeleton() {
  return (
    <div className={homeClasses.masonry} aria-hidden="true">
      {Array.from({ length: 8 }).map((_, index) => (
        <div key={index} className={homeClasses.skeletonCard}>
          <div className={homeClasses.skeletonImage} />
          <div className={homeClasses.skeletonBody}>
            <div className="pg-skeleton h-4 rounded-xl" />
            <div className="pg-skeleton h-3 w-2/3 rounded-xl" />
          </div>
        </div>
      ))}
    </div>
  )
}
