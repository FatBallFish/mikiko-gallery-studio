import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { publicMediaApi, type PublicMediaItem } from '../../../shared/public-media-api'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { cn } from '../../../shared/classnames'
import { EmptyState, ErrorState, GalleryFilterToolbar, GalleryImageFrame, ImageDetailModal, PublicDetailIcon, SegmentedControl, copyText, useApp } from '../components'
import { PublicVideoCard } from '../features/media/PublicVideoGallery'
import { errorMessage } from '../useApiResource'
import { mediaAccess } from '../mediaAccess'
import { Download, Heart, Image as ImageIcon, RefreshCw, Star } from '../ui/icons'
import { stageWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot } from './workspaceCreationDraft'
import { publicGalleryCardView } from './publicGalleryModel'
import {
  beginPublicGalleryDetailRequest,
  beginPublicGalleryRequest,
  canCommitPublicGalleryDetailRequest,
  canCommitPublicGalleryRequest,
  initialPublicGalleryDetailRequestState,
  initialPublicGalleryRequestState,
  resetPublicGalleryDetailRequest,
} from './publicGalleryRequests'
import '../features/media/media-redesign.css'

const PAGE_SIZE = 24

type GalleryFilter = 'all' | 'liked' | 'favorited'

function downloadFilename(image: Pick<ImageResult, 'id' | 'url' | 'download_url'>) {
  const source = image.download_url ?? image.url ?? ''
  const clean = source.split('?')[0]
  const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
  return `${image.id || 'image'}${ext}`
}

export function PublicGalleryPage({ imageId }: { imageId?: string }) {
  const app = useApp()
  const accessToken = app.session?.token
  const [feed, setFeed] = useState<PublicMediaItem[]>([])
  const [rows, setRows] = useState<ImageResult[]>([])
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<GalleryFilter>('all')
  const [sort, setSort] = useState<'latest' | 'hot'>('latest')
  const [modelFilter, setModelFilter] = useState('')
  const [selected, setSelected] = useState<ImageResult | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [detailError, setDetailError] = useState<{ imageId: string; message: string } | null>(null)
  const [detailRetryVersion, setDetailRetryVersion] = useState(0)
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const requestStateRef = useRef(initialPublicGalleryRequestState())
  const detailRequestStateRef = useRef(initialPublicGalleryDetailRequestState())

  const loadPage = useCallback(async (pageNumber: number, mode: 'replace' | 'append') => {
    const request = beginPublicGalleryRequest(requestStateRef.current, mode)
    requestStateRef.current = request.state
    const requestToken = request.token
    if (mode === 'replace') {
      setLoading(true)
      setLoadingMore(false)
    } else setLoadingMore(true)
    setLoadError(null)
    try {
      const result = await publicMediaApi.list(pageNumber, PAGE_SIZE, {
        accessToken,
        sort,
        model: modelFilter || undefined,
        query: query.trim() || undefined,
        liked: filter === 'liked',
        favorited: filter === 'favorited',
      })
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setFeed((current) => mode === 'replace' ? result.items : [...current, ...result.items.filter((item) => !current.some((known) => known.media_type === item.media_type && known.id === item.id))])
      const images = result.items.flatMap((item) => item.media_type === 'image' ? [item.image] : [])
      setRows((current) => mode === 'replace' ? images : [...current, ...images.filter((image) => !current.some((known) => known.id === image.id))])
      const resultTotal = result.pagination?.total ?? result.total ?? 0
      if (mode === 'replace' || resultTotal > 0) setTotal(resultTotal)
      setHasMore(result.items.length === PAGE_SIZE && (resultTotal <= 0 || pageNumber * PAGE_SIZE < resultTotal))
      setPage(pageNumber)
    } catch (error) {
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setLoadError(errorMessage(error))
    } finally {
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setLoading(false)
      setLoadingMore(false)
    }
  }, [accessToken, filter, query, modelFilter, sort])

  useEffect(() => {
    setRows([])
    setFeed([])
    setPage(1)
    setHasMore(true)
    setTotal(0)
    void loadPage(1, 'replace')
    return () => {
      requestStateRef.current = beginPublicGalleryRequest(requestStateRef.current, 'replace').state
    }
  }, [loadPage])

  useEffect(() => {
    const sentinel = loadMoreRef.current
    if (!sentinel || loading || loadingMore || !hasMore || loadError) return undefined
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void loadPage(page + 1, 'append')
    }, { rootMargin: '280px 0px' })
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasMore, loadError, loadPage, loading, loadingMore, page])

  const models = useMemo(() => Array.from(new Set(rows.map((image) => image.route_model_code || image.abstract_model).filter(Boolean) as string[])).sort(), [rows])
  const visibleRows = feed

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, null)
  }

  function requireLogin(action = '请先登录后再查看完整作品', targetImageId?: string) {
    if (accessToken) return true
    app.notify('warn', action)
    app.navigate('login', { returnTo: 'public-gallery', imageId: targetImageId ?? imageId })
    return false
  }

  async function openDetail(image: ImageResult) {
    if (!requireLogin('请先登录后再查看完整作品', image.id)) return
    setBusyId(`detail:${image.id}`)
    try {
      const detail = await openApi.getPublicGalleryImage(image.id, { accessToken })
      setSelected(detail)
      setRows((items) => items.map((item) => item.id === image.id ? { ...item, ...detail } : item))
    } catch (error) {
      app.notify('error', errorMessage(error))
    } finally {
      setBusyId(null)
    }
  }

  async function toggleReaction(image: ImageResult, kind: 'like' | 'favorite') {
    if (!accessToken) {
      requireLogin(kind === 'like' ? '请先登录后再点赞' : '请先登录后再收藏', image.id)
      return
    }
    setBusyId(`${kind}:${image.id}`)
    try {
      const active = kind === 'like' ? !image.liked_by_viewer : !image.favorited_by_viewer
      const updated = kind === 'like'
        ? await userApi.likePublicImage(image.id, active)
        : await userApi.favoritePublicImage(image.id, active)
      const merged = { ...image, ...updated, publish_status: updated.visibility_status }
      setRows((items) => items.map((item) => item.id === image.id ? merged : item))
      setSelected((current) => current?.id === image.id ? { ...current, ...merged } : current)
    } catch (error) {
      app.notify('error', errorMessage(error))
    } finally {
      setBusyId(null)
    }
  }

  async function copyPrompt(prompt: string) {
    if (!prompt || prompt === '-') {
      requireLogin('请先登录后再查看完整提示词', selected?.id)
      return
    }
    await copyText(prompt)
    app.notify('success', 'Prompt 已复制')
  }

  function reuseConfiguration(image: ImageResult) {
    if (!requireLogin('请先登录后再复用配置', image.id)) return
    if (!image.prompt) {
      app.notify('warn', '请先打开详情获取完整提示词')
      return
    }
    stageWorkspaceCreationDraft(workspaceCreationDraftFromSnapshot(image), window.sessionStorage, window.history)
    app.navigate('genpic')
  }

  useEffect(() => {
    const targetImageId = imageId?.trim()
    if (!targetImageId) {
      detailRequestStateRef.current = beginPublicGalleryDetailRequest(detailRequestStateRef.current, null, null).state
      return
    }
    if (!requireLogin('请先登录后再查看完整作品', targetImageId)) return
    const request = beginPublicGalleryDetailRequest(detailRequestStateRef.current, targetImageId, accessToken)
    detailRequestStateRef.current = request.state
    if (!request.token) return
    const requestToken = request.token
    let cancelled = false
    setSelected((current) => current?.id === targetImageId ? current : null)
    setDetailError(null)
    setBusyId(`detail:${targetImageId}`)
    void openApi.getPublicGalleryImage(targetImageId, { accessToken })
      .then((detail) => {
        if (cancelled || !canCommitPublicGalleryDetailRequest(detailRequestStateRef.current, requestToken)) return
        setSelected(detail)
        setRows((items) => items.some((item) => item.id === detail.id) ? items.map((item) => item.id === detail.id ? { ...item, ...detail } : item) : [detail, ...items])
      })
      .catch((error) => {
        if (cancelled || !canCommitPublicGalleryDetailRequest(detailRequestStateRef.current, requestToken)) return
        const message = errorMessage(error)
        setDetailError({ imageId: targetImageId, message })
        app.notify('error', message)
      })
      .finally(() => {
        if (cancelled || !canCommitPublicGalleryDetailRequest(detailRequestStateRef.current, requestToken)) return
        setBusyId((current) => current === `detail:${targetImageId}` ? null : current)
      })
    return () => { cancelled = true }
  }, [accessToken, detailRetryVersion, imageId])

  function retryDeepLinkDetail() {
    const targetImageId = imageId?.trim()
    if (!targetImageId) return
    detailRequestStateRef.current = resetPublicGalleryDetailRequest(detailRequestStateRef.current, targetImageId, accessToken)
    setDetailError(null)
    setDetailRetryVersion((current) => current + 1)
  }

  async function refreshPublicImage(imageId: string) {
    const projection = await mediaAccess.preview({ kind: 'image', scope: 'public', id: imageId })
    setRows((items) => items.map((item) => item.id === imageId ? { ...item, url: projection.url, download_url: projection.url, preview_expires_at: projection.expires_at } : item))
    setSelected((current) => current?.id === imageId ? { ...current, url: projection.url, download_url: projection.url, preview_expires_at: projection.expires_at } : current)
    return projection.url
  }

  async function downloadImage(image: ImageResult) {
    try {
      const projection = await mediaAccess.download({ kind: 'image', scope: 'public', id: image.id })
      const link = document.createElement('a')
      link.href = projection.url
      link.download = downloadFilename(image)
      link.rel = 'noopener noreferrer'
      document.body.appendChild(link)
      link.click()
      link.remove()
    } catch (error) {
      app.notify('error', errorMessage(error))
    }
  }

  function chooseFilter(next: GalleryFilter) {
    if (next !== 'all' && !requireLogin('请先登录后再查看个人互动内容')) return
    setFilter(next)
  }

  const loadedLabel = total > 0 ? `已显示 ${visibleRows.length} / ${total} 件作品` : `已显示 ${visibleRows.length} 件作品`

  return (
    <main className="public-gallery">
      <header className="u-page-head pg-editorial-head">
        <div className="u-page-head__copy">
          <span className="u-eyebrow">Public Gallery</span>
          <h1 className="u-h1">公开广场</h1>
        </div>
        <p className="u-sub pg-intro">由创作者主动公开的作品。提示词、模型与生成参数完整可见，可以直接复用为下一次创作。</p>
      </header>

      <label>排序 <select value={sort} onChange={(event) => setSort(event.target.value as 'latest' | 'hot')}><option value="latest">最新公开</option><option value="hot">最热</option></select></label>
      <GalleryFilterToolbar
        label="公开广场筛选"
        query={query}
        queryPlaceholder="搜索提示词、标题或作者"
        onQueryChange={setQuery}
        filters={(
          <>
            <SegmentedControl<GalleryFilter>
              label="互动筛选"
              value={filter}
              options={[
                { value: 'all', label: '全部' },
                { value: 'liked', label: '已点赞' },
                { value: 'favorited', label: '已收藏' },
              ]}
              onChange={chooseFilter}
            />
            <select className="u-filter" aria-label="模型筛选" value={modelFilter} onChange={(event) => setModelFilter(event.target.value)}>
              <option value="">全部模型</option>
              {models.map((model) => <option key={model} value={model}>{model}</option>)}
            </select>
            <button className="u-btn u-btn--sm" type="button" disabled={loading} onClick={() => void loadPage(1, 'replace')}>
              <RefreshCw size={15} strokeWidth={1.6} aria-hidden="true" />刷新
            </button>
          </>
        )}
        meta={loading ? '读取中' : loadedLabel}
      />

      {detailError && detailError.imageId === imageId?.trim() ? <ErrorState message={detailError.message} onRetry={retryDeepLinkDetail} /> : null}

      {loading && !feed.length ? <PublicGallerySkeleton /> : null}
      {loadError && !feed.length ? <ErrorState message={loadError} onRetry={() => void loadPage(1, 'replace')} /> : null}
      {!loading && !loadError && !feed.length ? <EmptyState icon={<ImageIcon size={28} strokeWidth={1.5} aria-hidden="true" />} title="暂无公开作品" detail="公开广场未开启，或当前筛选下还没有审核通过的作品。" /> : null}
      {!loading && !loadError && feed.length > 0 && !visibleRows.length ? <EmptyState icon={<ImageIcon size={28} strokeWidth={1.5} aria-hidden="true" />} title="当前模型暂无作品" detail="换一个模型筛选，或刷新查看新公开的作品。" /> : null}

      {visibleRows.length ? (
        <div className="u-grid u-grid--gallery">
          {visibleRows.map((item) => {
            if (item.media_type === 'video') return <PublicVideoCard key={`video:${item.id}`} video={item.video} onChange={(updated) => {
              const removed = filter === 'liked' && !updated.liked_by_viewer || filter === 'favorited' && !updated.favorited_by_viewer
              setFeed((items) => removed ? items.filter((known) => !(known.media_type === 'video' && known.id === updated.id)) : items.map((known) => known.media_type === 'video' && known.id === updated.id ? { ...known, video: updated } : known))
              if (removed) setTotal((value) => Math.max(0, value - 1))
            }} />
            const image = rows.find((row) => row.id === item.id) ?? item.image
            const card = publicGalleryCardView(image)
            const imagePath = image.url || image.download_url
            return (
              <article key={`image:${image.id}`} className="u-card pg-card">
                <div className="u-card__media">
                  <GalleryImageFrame
                    src={imagePath ? assetUrl(imagePath) : undefined}
                    mediaExpiresAt={image.preview_expires_at}
                    alt={card.title || image.id}
                    width={image.width}
                    height={image.height}
                    aspectRatio="4 / 3"
                    onOpen={() => void openDetail(image)}
                    onMediaRefresh={() => refreshPublicImage(image.id)}
                    className="pg-card__frame"
                  />
                  <span className="u-card__tag">图片</span>
                  <div className="u-card__ops pg-card__ops">
                    <button
                      className={cn('u-card__op', image.liked_by_viewer && 'is-on')}
                      type="button"
                      aria-label={`点赞 ${image.like_count ?? 0}`}
                      aria-pressed={Boolean(image.liked_by_viewer)}
                      title={image.liked_by_viewer ? '取消点赞' : '点赞'}
                      disabled={busyId === `like:${image.id}`}
                      onClick={() => void toggleReaction(image, 'like')}
                    >
                      <Heart size={14} strokeWidth={1.8} fill={image.liked_by_viewer ? 'currentColor' : 'none'} aria-hidden="true" />
                    </button>
                    <button
                      className={cn('u-card__op', image.favorited_by_viewer && 'is-on')}
                      type="button"
                      aria-label={`收藏 ${image.favorite_count ?? 0}`}
                      aria-pressed={Boolean(image.favorited_by_viewer)}
                      title={image.favorited_by_viewer ? '取消收藏' : '收藏'}
                      disabled={busyId === `favorite:${image.id}`}
                      onClick={() => void toggleReaction(image, 'favorite')}
                    >
                      <Star size={14} strokeWidth={1.8} fill={image.favorited_by_viewer ? 'currentColor' : 'none'} aria-hidden="true" />
                    </button>
                    <button
                      className="u-card__op"
                      type="button"
                      aria-label="下载作品"
                      title="下载作品"
                      disabled={!imagePath}
                      onClick={() => void downloadImage(image)}
                    >
                      <Download size={14} strokeWidth={1.8} aria-hidden="true" />
                    </button>
                  </div>
                </div>
                <div className="u-card__body">
                  <div className="u-card__title" title={card.title}>{card.title}</div>
                  <div className="u-card__meta">
                    <span className="pg-author"><i>{Array.from(card.author)[0] ?? '匿'}</i>{card.author}</span>
                    <span>{card.date}</span>
                  </div>
                  <div className="u-card__meta pg-card__numbers">
                    <span>{card.taskType} · {card.model}</span>
                    <span>♥ {image.like_count ?? 0} · ★ {image.favorite_count ?? 0}</span>
                  </div>
                </div>
              </article>
            )
          })}
        </div>
      ) : null}

      <div ref={loadMoreRef} className="pg-load-more" aria-live="polite">
        {feed.length ? <span className="u-meta">{loadedLabel}</span> : null}
        {loadingMore ? <span className="u-meta">正在加载更多作品</span> : loadError && feed.length ? <button className="u-btn u-btn--sm" type="button" onClick={() => void loadPage(page + 1, 'append')}>重试加载</button> : loadError ? null : hasMore ? <button className="u-btn u-btn--sm" type="button" onClick={() => void loadPage(page + 1, 'append')}>加载更多</button> : feed.length ? <span className="u-meta">已显示全部作品</span> : null}
      </div>

      <ImageDetailModal
        context="public"
        title="公开图片详情"
        image={selected}
        imageUrl={selected?.url || selected?.download_url ? assetUrl(selected?.url || selected?.download_url || '') : undefined}
        onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
        onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
        onDownload={(image) => void downloadImage(image as ImageResult)}
        onCopyPrompt={(prompt) => void copyPrompt(prompt)}
        actions={selected ? [{ key: 'reuse', label: '复用配置', icon: <PublicDetailIcon name="edit" />, onClick: () => reuseConfiguration(selected), disabled: !selected.prompt }] : []}
        previewSourceLabel="公开广场"
        onMediaRefresh={selected ? () => refreshPublicImage(selected.id) : undefined}
        onClose={() => setSelected(null)}
      />
    </main>
  )
}

function PublicGallerySkeleton() {
  return (
    <div className="u-grid u-grid--gallery" aria-label="正在读取公开广场">
      {Array.from({ length: 9 }).map((_, index) => (
        <div className="u-card media-skeleton-card" key={index}>
          <div className="u-skeleton media-skeleton-media" />
          <div className="u-card__body">
            <div className="u-skeleton media-skeleton-line" />
            <div className="u-skeleton media-skeleton-line is-short" />
          </div>
        </div>
      ))}
    </div>
  )
}