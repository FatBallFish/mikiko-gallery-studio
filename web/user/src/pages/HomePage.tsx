import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import heroImage from '../../../../docs/template/PicGallery/mpdhezm8-image.png'
import { Modal, PublicImageDetail, copyText, formatDate, taskTypeLabel, useApp } from '../components'
import { useApiResource } from '../useApiResource'

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

const styles = {
  content: {
    width: '100%',
    maxWidth: 1200,
    marginInline: 'auto',
    padding: 40,
  },
  carousel: {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    minHeight: 320,
    aspectRatio: '21 / 9',
    marginBottom: 40,
    borderRadius: 20,
    border: '1px solid var(--vault-line)',
    background: 'var(--vault-surface)',
    cursor: 'pointer',
  },
  heroImage: {
    width: '100%',
    height: '100%',
    objectFit: 'cover',
    opacity: 0.82,
  },
  carouselOverlay: {
    position: 'absolute',
    inset: 0,
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'flex-end',
    alignItems: 'flex-start',
    padding: 40,
    background: 'linear-gradient(to top, var(--vault-bg) 0%, rgba(8, 10, 16, 0.58) 36%, transparent 70%)',
  },
  heroKicker: {
    margin: '0 0 8px',
    color: 'var(--vault-gold)',
    fontFamily: 'var(--font-mono)',
    fontSize: 12,
    letterSpacing: '0.1em',
    textTransform: 'uppercase',
  },
  heroTitle: {
    margin: 0,
    color: '#fff',
    fontFamily: 'var(--font-display)',
    fontSize: 'clamp(2.4rem, 5vw, 4rem)',
    lineHeight: 0.95,
  },
  heroText: {
    maxWidth: '60ch',
    margin: '14px 0 20px',
    color: 'var(--vault-muted)',
    fontSize: 14,
  },
  actionRow: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: 12,
  },
  sectionHead: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'flex-end',
    gap: 16,
    marginBottom: 24,
  },
  sectionTitle: {
    margin: 0,
    fontFamily: 'var(--font-display)',
    fontSize: 28,
  },
  sectionDetail: {
    display: 'block',
    marginTop: 6,
    color: 'var(--vault-muted)',
    fontSize: 13,
  },
  filterGroup: {
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    gap: 12,
  },
  masonry: {
    columns: '280px 3',
    columnGap: 24,
  },
  masonryItem: {
    display: 'inline-block',
    width: '100%',
    overflow: 'hidden',
    breakInside: 'avoid',
    marginBottom: 24,
    borderRadius: 'var(--radius-lg)',
    border: '1px solid var(--vault-line)',
    background: 'var(--vault-surface)',
    color: 'var(--vault-fg)',
    textAlign: 'left',
    cursor: 'pointer',
  },
  masonryImage: {
    width: '100%',
    height: 'auto',
    opacity: 0.9,
  },
  cardBody: {
    padding: 16,
  },
  cardTitle: {
    display: 'block',
    marginBottom: 4,
    fontSize: 13,
    fontWeight: 700,
  },
  cardMeta: {
    color: 'var(--vault-muted)',
    fontFamily: 'var(--font-mono)',
    fontSize: 11,
    textTransform: 'uppercase',
  },
  placeholder: {
    display: 'grid',
    placeItems: 'center',
    minHeight: 220,
    color: 'var(--vault-muted)',
    background: 'rgba(255, 255, 255, 0.04)',
  },
  smallNote: {
    margin: '10px 0 0',
    color: 'var(--vault-muted)',
    fontSize: 13,
  },
} satisfies Record<string, CSSProperties>

export function HomePage() {
  const app = useApp()
  const [filter, setFilter] = useState<FilterMode>('hot')
  const [selected, setSelected] = useState<ImageResult | null>(null)
  const gallery = useApiResource(() => openApi.listPublicGallery(1, 30, { sort: filter, accessToken: app.session?.token }), [filter, app.session?.token])

  const cards = useMemo(() => {
    return (gallery.data?.items ?? []).map((image) => ({
      id: image.id,
      title: image.prompt || image.id,
      meta: `${taskTypeLabel(image.task_type ?? 'text_to_image')} · ${image.route_model_code || image.abstract_model || '-'} · ${image.quality || '-'} · ${formatDate(image.created_at ?? '')}`,
      imageUrl: image.url ? userApi.imageAssetUrl(image.url, app.session?.token) : undefined,
      createdAt: image.created_at ?? '',
      hotScore: (image.like_count ?? 0) * 2 + (image.favorite_count ?? 0) * 3 + (image.comment_count ?? 0),
      image,
      onClick: () => setSelected(image),
    }))
  }, [app.session?.token, gallery.data])

  async function toggleReaction(image: ImageResult, kind: 'like' | 'favorite') {
    const active = kind === 'like' ? !image.liked_by_viewer : !image.favorited_by_viewer
    try {
      const updated = kind === 'like'
        ? await userApi.likePublicImage(image.id, active)
        : await userApi.favoritePublicImage(image.id, active)
      setSelected((current) => current?.id === image.id ? { ...current, ...updated, publish_status: updated.visibility_status } : current)
      await gallery.reload()
    } catch {
      app.notify('error', '操作失败，请稍后重试')
    }
  }

  function imageUrl(image: ImageResult) {
    const url = image.url || image.download_url
    return url ? userApi.imageAssetUrl(url, null) : undefined
  }

  function downloadImage(image: ImageResult) {
    const url = image.url || image.download_url
    if (!url) return
    const link = document.createElement('a')
    link.href = userApi.imageAssetUrl(url, null)
    link.download = downloadFilename(image)
    link.rel = 'noopener noreferrer'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  function downloadFilename(image: ImageResult) {
    const source = image.download_url ?? image.url ?? ''
    const clean = source.split('?')[0]
    const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
    return `${image.id || 'image'}${ext}`
  }

  async function copyPrompt(prompt: string) {
    await copyText(prompt)
    app.notify('success', 'Prompt 已复制')
  }

  return (
    <div className="content" style={styles.content}>
      <section
        className="carousel"
        style={styles.carousel}
        role="button"
        tabIndex={0}
        aria-label="套用 Cinematic Luxury Watch 主题"
        onClick={() => app.navigate('genpic')}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') app.navigate('genpic')
        }}
      >
        <img src={heroImage} alt="Carousel Banner" style={styles.heroImage} />
        <div className="carousel-overlay" style={styles.carouselOverlay}>
          <p style={styles.heroKicker}>FEATURED SHOWCASE</p>
          <h2 style={styles.heroTitle}>Cinematic Luxury Watch</h2>
          <p style={styles.heroText}>探索如何通过精确的提示词与全能 PRO 模型，生成令人惊叹的高奢产品视觉资产。</p>
          <div style={styles.actionRow}>
            <HomeButton isActive onClick={(event) => navigateFromButton(event, () => app.navigate('genpic'))}>从参考图开始</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('public-gallery'))}>查看公开图库</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('api-keys'))}>API 开放平台</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('profile'))}>{app.profile?.avatar_initials ?? '账户'}</HomeButton>
          </div>
        </div>
      </section>

      <section aria-labelledby="inspiration-title">
        <div style={styles.sectionHead}>
          <div>
            <h3 id="inspiration-title" style={styles.sectionTitle}>灵感瀑布流</h3>
            <span style={styles.sectionDetail}>
              {gallery.loading ? '正在刷新公开图库...' : `已同步 ${gallery.data?.items.length ?? 0} 张公开图片`}
            </span>
          </div>
          <div style={styles.filterGroup} aria-label="灵感筛选">
            <FilterButton active={filter === 'latest'} onClick={() => setFilter('latest')}>最新</FilterButton>
            <FilterButton active={filter === 'hot'} onClick={() => setFilter('hot')}>最热</FilterButton>
            <HomeButton onClick={() => void gallery.reload()}>刷新</HomeButton>
          </div>
        </div>

        <div className="masonry" style={styles.masonry}>
          {cards.map((card) => (
            <button type="button" className="masonry-item" style={styles.masonryItem} key={card.id} onClick={card.onClick}>
              {card.imageUrl ? <img src={card.imageUrl} alt={card.title} style={styles.masonryImage} /> : <ImagePlaceholder />}
              <div style={styles.cardBody}>
                <strong style={styles.cardTitle}>{card.title}</strong>
                <span style={styles.cardMeta}>{card.meta}</span>
              </div>
            </button>
          ))}
        </div>
      </section>
      {selected ? (
        <Modal title="公开图片详情" onClose={() => setSelected(null)}>
          <PublicImageDetail
            image={selected}
            imageUrl={imageUrl(selected)}
            onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
            onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
            onDownload={(image) => downloadImage(image as ImageResult)}
            onCopyPrompt={(prompt) => void copyPrompt(prompt)}
          />
        </Modal>
      ) : null}
    </div>
  )
}

function navigateFromButton(event: React.MouseEvent<HTMLButtonElement>, navigate: () => void) {
  event.stopPropagation()
  navigate()
}

function HomeButton({ children, isActive = false, onClick }: { children: React.ReactNode; isActive?: boolean; onClick: React.MouseEventHandler<HTMLButtonElement> }) {
  return (
    <button type="button" style={buttonStyle(isActive)} onClick={onClick}>
      {children}
    </button>
  )
}

function FilterButton({ children, active, onClick }: { children: React.ReactNode; active: boolean; onClick: () => void }) {
  return (
    <button type="button" aria-pressed={active} style={buttonStyle(active)} onClick={onClick}>
      {children}
    </button>
  )
}

function buttonStyle(active: boolean): CSSProperties {
  return {
    padding: '6px 16px',
    borderRadius: 20,
    border: active ? '1px solid var(--vault-gold)' : '1px solid var(--vault-line)',
    background: active ? 'var(--vault-gold)' : 'var(--vault-surface)',
    color: active ? 'var(--vault-bg)' : 'var(--vault-fg)',
    fontSize: 13,
    fontWeight: active ? 800 : 600,
  }
}

function ImagePlaceholder() {
  return (
    <div style={styles.placeholder}>
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" aria-hidden="true">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <circle cx="8.5" cy="8.5" r="1.5" />
        <path d="M21 15l-5-5L5 21" />
      </svg>
    </div>
  )
}
