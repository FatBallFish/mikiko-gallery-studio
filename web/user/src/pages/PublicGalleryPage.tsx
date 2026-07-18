import { useCallback, useEffect, useRef, useState } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, GalleryFilterToolbar, GalleryImageFrame, ImageDetailModal, ImageLightbox, PublicDetailIcon, copyText, publicDetailButton, type ImageLightboxPayload, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { ArrowRight, Image as ImageIcon, RefreshCw } from '../ui/icons'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { galleryImageAspect } from './galleryExperience'
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

const PAGE_SIZE = 24

const publicGalleryClasses = {
  content: 'mx-auto w-full max-w-[1440px] px-6 pb-28 pt-8 md:px-10 md:pb-16 md:pt-12',
  header: 'mb-9 grid grid-cols-[minmax(0,1fr)_minmax(18rem,.5fr)] items-end gap-8 border-b border-[var(--border)] pb-8 max-[760px]:grid-cols-1',
  title: 'm-0 font-vault-display text-[clamp(2.6rem,6vw,5.5rem)] font-medium leading-none text-[var(--fg)]',
  intro: 'mb-0 max-w-[58ch] text-sm leading-7 text-[var(--muted)]',
  filterButton: 'min-h-10 rounded-xl border border-[var(--border)] bg-transparent px-3 text-sm font-semibold text-[var(--muted)] transition-colors hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:text-[var(--fg)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)] motion-reduce:transition-none max-[620px]:min-w-[6rem] max-[620px]:flex-1',
  filterButtonActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] text-[var(--accent)]',
  grid: 'mt-8 grid grid-cols-3 gap-x-5 gap-y-10 max-[980px]:grid-cols-2 max-[620px]:grid-cols-1',
  card: 'min-w-0',
  info: 'grid gap-1.5 pt-3',
  titleLine: 'line-clamp-2 text-sm font-semibold leading-6 text-[var(--fg)]',
  metaLine: 'truncate font-vault-mono text-[11px] text-[var(--muted)]',
  iconActions: 'flex flex-wrap justify-end gap-1 rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] p-1 backdrop-blur',
  loadMore: 'mt-12 flex min-h-16 items-center justify-center border-t border-[var(--border)] pt-8 text-sm text-[var(--muted)]',
}

function downloadFilename(image: Pick<ImageResult, 'id' | 'url' | 'download_url'>) {
  const source = image.download_url ?? image.url ?? ''
  const clean = source.split('?')[0]
  const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
  return `${image.id || 'image'}${ext}`
}

export function PublicGalleryPage({ imageId }: { imageId?: string }) {
  const app = useApp()
  const accessToken = app.session?.token
  const [rows, setRows] = useState<ImageResult[]>([])
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<'all' | 'liked' | 'favorited'>('all')
  const [selected, setSelected] = useState<ImageResult | null>(null)
  const [imagePreview, setImagePreview] = useState<ImageLightboxPayload | null>(null)
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
      const result = await openApi.listPublicGallery(pageNumber, PAGE_SIZE, {
        accessToken,
        query: query.trim() || undefined,
        liked: filter === 'liked',
        favorited: filter === 'favorited',
      })
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setRows((current) => {
        if (mode === 'replace') return result.items
        const known = new Set(current.map((image) => image.id))
        return [...current, ...result.items.filter((image) => !known.has(image.id))]
      })
      const total = result.pagination?.total ?? result.total ?? 0
      setHasMore(result.items.length === PAGE_SIZE && (total <= 0 || pageNumber * PAGE_SIZE < total))
      setPage(pageNumber)
    } catch (error) {
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setLoadError(errorMessage(error))
    } finally {
      if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return
      setLoading(false)
      setLoadingMore(false)
    }
  }, [accessToken, filter, query])

  useEffect(() => {
    setRows([])
    setPage(1)
    setHasMore(true)
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

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, null)
  }

  function requireLogin(action = '请先登录后再查看完整作品', targetImageId?: string) {
    if (accessToken) return true
    app.notify('info', action)
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

  function generateSame(image: ImageResult) {
    if (!requireLogin('请先登录后再同款生成', image.id)) return
    if (!image.prompt) {
      app.notify('info', '请先打开详情获取完整提示词')
      return
    }
    window.sessionStorage.setItem(galleryEditContextKey, JSON.stringify(createGalleryEditContext({
      prompt: image.prompt,
      route_model_code: image.route_model_code || image.abstract_model,
      base_resolution: image.base_resolution,
      aspect_ratio: image.aspect_ratio,
    })))
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

  function downloadImage(image: ImageResult) {
    const url = image.download_url ?? image.url
    if (!url) return
    const link = document.createElement('a')
    link.href = assetUrl(url)
    link.download = downloadFilename(image)
    link.rel = 'noopener noreferrer'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  return (
    <main className={publicGalleryClasses.content}>
      <header className={publicGalleryClasses.header}>
        <h1 className={publicGalleryClasses.title}>公开广场</h1>
        <p className={publicGalleryClasses.intro}>查看经过审核的真实生成结果。登录后可检视完整参数，并把提示词、模型与比例带回创作台。</p>
      </header>

      <GalleryFilterToolbar
        label="公开广场筛选"
        query={query}
        queryPlaceholder="搜索提示词、模型或作者"
        onQueryChange={setQuery}
        filters={(['all', 'liked', 'favorited'] as const).map((item) => (
          <button
            key={item}
            type="button"
            className={cn(publicGalleryClasses.filterButton, filter === item && publicGalleryClasses.filterButtonActive)}
            aria-pressed={filter === item}
            onClick={() => {
              if (item !== 'all' && !requireLogin('请先登录后再查看个人互动内容')) return
              setFilter(item)
            }}
          >
            {item === 'all' ? '全部公开' : item === 'liked' ? '已点赞' : '已收藏'}
          </button>
        ))}
        meta={loading ? '读取中' : `已加载 ${rows.length} 张`}
        action={<Button tone="ghost" onClick={() => void loadPage(1, 'replace')} disabled={loading}><RefreshCw size={15} strokeWidth={1.5} aria-hidden="true" />刷新</Button>}
      />

      {detailError && detailError.imageId === imageId?.trim() ? <ErrorState message={detailError.message} onRetry={retryDeepLinkDetail} /> : null}

      {loading && !rows.length ? <PublicGallerySkeleton /> : null}
      {loadError && !rows.length ? <ErrorState message={loadError} onRetry={() => void loadPage(1, 'replace')} /> : null}
      {!loading && !loadError && !rows.length ? <EmptyState icon={<ImageIcon size={28} strokeWidth={1.5} />} title="暂无公开作品" detail="公开广场未开启，或当前筛选下还没有审核通过的图片。" /> : null}

      {rows.length ? (
        <div className={publicGalleryClasses.grid}>
          {rows.map((image) => {
            const card = publicGalleryCardView(image)
            const imagePath = image.url || image.download_url
            return (
              <article key={image.id} className={publicGalleryClasses.card}>
                <GalleryImageFrame
                  src={imagePath ? assetUrl(imagePath) : undefined}
                  alt={card.title || image.id}
                  width={image.width}
                  height={image.height}
                  aspectRatio={galleryImageAspect({ width: image.width, height: image.height, aspectRatio: image.aspect_ratio })}
                  onOpen={() => void openDetail(image)}
                  actions={<div className={publicGalleryClasses.iconActions}>
                    {publicDetailButton('查看详情', <PublicDetailIcon name="eye" />, () => void openDetail(image), '', busyId === `detail:${image.id}`)}
                    {publicDetailButton(`点赞 ${image.like_count ?? 0}`, <PublicDetailIcon name="heart" active={image.liked_by_viewer} />, () => void toggleReaction(image, 'like'), image.liked_by_viewer ? 'liked' : '', busyId === `like:${image.id}`)}
                    {publicDetailButton(`收藏 ${image.favorite_count ?? 0}`, <PublicDetailIcon name="star" active={image.favorited_by_viewer} />, () => void toggleReaction(image, 'favorite'), image.favorited_by_viewer ? 'favorited' : '', busyId === `favorite:${image.id}`)}
                    {publicDetailButton('下载', <PublicDetailIcon name="download" />, () => downloadImage(image), '', !imagePath)}
                  </div>}
                />
                <div className={publicGalleryClasses.info}>
                  <div className={publicGalleryClasses.titleLine}>{card.title}</div>
                  <div className={publicGalleryClasses.metaLine}>{card.taskType} · {card.model} · {card.baseResolution} · {card.aspectRatio}</div>
                  <div className={publicGalleryClasses.metaLine}>{card.author} · {card.date} · {card.status}</div>
                </div>
              </article>
            )
          })}
        </div>
      ) : null}

      <div ref={loadMoreRef} className={publicGalleryClasses.loadMore}>
        {loadingMore ? <span className="inline-flex items-center gap-2"><span className="pg-skeleton size-4 rounded-full" />正在加载更多作品</span> : loadError && rows.length ? <Button tone="ghost" onClick={() => void loadPage(page + 1, 'append')}>重试加载<ArrowRight size={15} strokeWidth={1.5} /></Button> : loadError ? null : hasMore ? '继续下滑加载' : rows.length ? '已显示全部作品' : null}
      </div>

      <ImageDetailModal
        title="公开图片详情"
        image={selected}
        imageUrl={selected?.url || selected?.download_url ? assetUrl(selected?.url || selected?.download_url || '') : undefined}
        onPreviewImage={setImagePreview}
        onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
        onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
        onDownload={(image) => downloadImage(image as ImageResult)}
        onCopyPrompt={(prompt) => void copyPrompt(prompt)}
        actions={selected ? [{ key: 'same', label: '同款生成', icon: <PublicDetailIcon name="edit" />, onClick: () => generateSame(selected), disabled: !selected.prompt }] : []}
        previewSourceLabel="公开广场"
        onClose={() => setSelected(null)}
      />
      <ImageLightbox image={imagePreview} onClose={() => setImagePreview(null)} />
    </main>
  )
}

function PublicGallerySkeleton() {
  return (
    <div className={publicGalleryClasses.grid} aria-label="正在读取公开广场">
      {Array.from({ length: 9 }).map((_, index) => <div className="pg-skeleton aspect-[4/3] rounded-2xl" key={index} />)}
    </div>
  )
}
