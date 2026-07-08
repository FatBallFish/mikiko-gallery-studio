import { useEffect, useMemo, useState } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { EmptyState, ImageDetailModal, LoadingState, PublicDetailIcon, copyText, publicDetailButton, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { userForm, userText } from '../ui/classes'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { publicGalleryCardView, shouldFetchPublicGalleryDetailByID } from './publicGalleryModel'

const publicGalleryClasses = {
  content: 'mx-auto w-full max-w-[1200px] p-10 max-[760px]:px-5 max-[760px]:pb-24 max-[760px]:pt-5 max-[420px]:px-4 max-[420px]:pb-24 max-[420px]:pt-4',
  header: 'mb-10 flex flex-wrap items-end justify-between gap-5',
  title: 'm-0 font-vault-display text-5xl font-medium leading-none text-[var(--fg)] max-[620px]:text-4xl',
  searchInput: 'w-[280px] max-w-full rounded-xl',
  filters: 'mb-8 flex flex-wrap gap-3',
  filterButton: 'rounded-xl border border-[var(--border)] bg-[var(--surface)] px-4 py-2 font-vault-mono text-sm text-[var(--muted)] transition hover:-translate-y-px hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:text-[var(--fg)] active:scale-[0.98]',
  filterButtonActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] text-[var(--accent)]',
  grid: 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-6 max-[760px]:grid-cols-1',
  card: 'overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)]',
  thumb: 'grid aspect-square w-full place-items-center overflow-hidden bg-[var(--bg)] p-0 disabled:cursor-wait disabled:opacity-70',
  thumbImage: 'h-full w-full object-cover',
  info: 'p-4',
  titleLine: 'text-sm font-bold',
  metaLine: 'text-xs text-[var(--muted)]',
  metaLineSpaced: 'mt-1 text-xs text-[var(--muted)]',
  iconActions: 'mt-3.5 flex flex-wrap justify-end gap-2',
}

function downloadFilename(image: Pick<ImageResult, 'id' | 'url' | 'download_url'>) {
  const source = image.download_url ?? image.url ?? ''
  const clean = source.split('?')[0]
  const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
  return `${image.id || 'image'}${ext}`
}

export function PublicGalleryPage({ imageId }: { imageId?: string }) {
  const app = useApp()
  const [rows, setRows] = useState<ImageResult[]>([])
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<'all' | 'liked' | 'favorited'>('all')
  const [selected, setSelected] = useState<ImageResult | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  useEffect(() => {
    let mounted = true
    setLoading(true)
    async function load() {
      const page = await openApi.listPublicGallery(1, 40, {
        accessToken: app.session?.token,
        query: query.trim() || undefined,
        liked: filter === 'liked',
        favorited: filter === 'favorited',
      })
      if (!mounted) return
      setRows(page.items)
    }
    void load()
      .catch((err) => {
        if (mounted) app.notify('error', errorMessage(err))
      })
      .finally(() => {
        if (mounted) setLoading(false)
    })
    return () => { mounted = false }
  }, [app, app.session?.token, filter, query])

  const filtered = useMemo(() => rows, [rows])

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, null)
  }

  function requireLogin(action = '请先登录后再查看完整作品', targetImageId?: string) {
    if (app.session?.token) return true
    app.notify('info', action)
    app.navigate('login', { returnTo: 'public-gallery', imageId: targetImageId ?? imageId })
    return false
  }

  async function openDetail(image: ImageResult) {
    if (!requireLogin('请先登录后再查看完整作品', image.id)) return
    setBusyId(`detail:${image.id}`)
    try {
      const detail = await openApi.getPublicGalleryImage(image.id, { accessToken: app.session?.token })
      setSelected(detail)
      setRows((items) => items.map((item) => item.id === image.id ? { ...item, ...detail } : item))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  async function toggleReaction(image: ImageResult, kind: 'like' | 'favorite') {
    if (!app.session?.token) {
      requireLogin(kind === 'like' ? '请先登录后再点赞' : '请先登录后再收藏', image.id)
      return
    }
    setBusyId(`${kind}:${image.id}`)
    try {
      const active = kind === 'like' ? !image.liked_by_viewer : !image.favorited_by_viewer
      const updated = kind === 'like'
        ? await userApi.likePublicImage(image.id, active)
        : await userApi.favoritePublicImage(image.id, active)
      setRows((items) => items.map((item) => item.id === image.id ? { ...item, ...updated, publish_status: updated.visibility_status } : item))
      setSelected((current) => current?.id === image.id ? { ...current, ...updated, publish_status: updated.visibility_status } : current)
    } catch (err) {
      app.notify('error', errorMessage(err))
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
      quality: image.quality,
      aspect_ratio: image.aspect_ratio,
    })))
    app.navigate('genpic')
  }

  useEffect(() => {
    if (!imageId || !rows.length || selected?.id === imageId || busyId === `detail:${imageId}`) return
    const target = rows.find((image) => image.id === imageId)
    if (target) void openDetail(target)
  }, [imageId, rows, selected?.id, busyId])

  useEffect(() => {
    if (!shouldFetchPublicGalleryDetailByID({ imageId, rows, selectedId: selected?.id, busyId })) return
    if (!requireLogin('请先登录后再查看完整作品', imageId)) return
    const targetImageId = imageId?.trim()
    if (!targetImageId) return
    setBusyId(`detail:${targetImageId}`)
    void openApi.getPublicGalleryImage(targetImageId, { accessToken: app.session?.token })
      .then((detail) => {
        setSelected(detail)
        setRows((items) => items.some((item) => item.id === detail.id) ? items.map((item) => item.id === detail.id ? { ...item, ...detail } : item) : [detail, ...items])
      })
      .catch((err) => {
        app.notify('error', errorMessage(err))
      })
      .finally(() => setBusyId(null))
  }, [app, app.session?.token, imageId, rows, selected?.id, busyId])

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
    <div className={publicGalleryClasses.content}>
      <div className={publicGalleryClasses.header}>
        <div>
          <p className={userText.eyebrow}>公开作品</p>
          <h1 className={publicGalleryClasses.title}>公开广场</h1>
        </div>
        <input className={cn(userForm.input, publicGalleryClasses.searchInput)} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索提示词、模型或作者" />
      </div>

      <div className={publicGalleryClasses.filters}>
        {(['all', 'liked', 'favorited'] as const).map((item) => (
          <button key={item} type="button" className={cn(publicGalleryClasses.filterButton, filter === item && publicGalleryClasses.filterButtonActive)} onClick={() => {
            if (item !== 'all' && !requireLogin('请先登录后再查看个人互动内容')) return
            setFilter(item)
          }}>
            {item === 'all' ? '全部公开' : item === 'liked' ? '已点赞' : '已收藏'}
          </button>
        ))}
      </div>

      {loading ? <LoadingState label="正在读取公开广场..." /> : null}
      {!loading && !filtered.length ? <EmptyState title="暂无公开作品" detail="公开广场未开启或暂无审核通过的图片。" /> : null}

      <div className={publicGalleryClasses.grid}>
        {filtered.map((image) => {
          const card = publicGalleryCardView(image)
          return (
            <article key={image.id} className={publicGalleryClasses.card}>
              <button type="button" className={publicGalleryClasses.thumb} onClick={() => void openDetail(image)} disabled={busyId === `detail:${image.id}`}>
                {image.url ? <img src={assetUrl(image.url)} alt={card.title || image.id} className={publicGalleryClasses.thumbImage} /> : null}
              </button>
              <div className={publicGalleryClasses.info}>
                <div className={publicGalleryClasses.titleLine}>{card.title}</div>
                <div className={publicGalleryClasses.metaLine}>
                  {card.taskType} · {card.model} · {card.quality} · {card.aspectRatio}
                </div>
                <div className={publicGalleryClasses.metaLineSpaced}>
                  {card.author} · {card.date} · {card.status}
                </div>
                <div className={publicGalleryClasses.iconActions}>
                  {publicDetailButton('查看详情', <PublicDetailIcon name="eye" />, () => void openDetail(image), '', busyId === `detail:${image.id}`)}
                  {publicDetailButton(`点赞 ${image.like_count ?? 0}`, <PublicDetailIcon name="heart" active={image.liked_by_viewer} />, () => void toggleReaction(image, 'like'), image.liked_by_viewer ? 'liked' : '', busyId === `like:${image.id}`)}
                  {publicDetailButton(`收藏 ${image.favorite_count ?? 0}`, <PublicDetailIcon name="star" active={image.favorited_by_viewer} />, () => void toggleReaction(image, 'favorite'), image.favorited_by_viewer ? 'favorited' : '', busyId === `favorite:${image.id}`)}
                  {publicDetailButton('下载', <PublicDetailIcon name="download" />, () => downloadImage(image), '', !image.url)}
                </div>
              </div>
            </article>
          )
        })}
      </div>

      <ImageDetailModal
        title="公开图片详情"
        image={selected}
        imageUrl={selected?.url || selected?.download_url ? assetUrl(selected?.url || selected?.download_url || '') : undefined}
        onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
        onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
        onDownload={(image) => downloadImage(image as ImageResult)}
        onCopyPrompt={(prompt) => void copyPrompt(prompt)}
        actions={selected ? [{ key: 'same', label: '同款生成', icon: <PublicDetailIcon name="edit" />, onClick: () => generateSame(selected), disabled: !selected.prompt }] : []}
        previewSourceLabel="公开广场"
        onClose={() => setSelected(null)}
      />
    </div>
  )
}
