import { useEffect, useMemo, useState } from 'react'
import type { ImageResult } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { EmptyState, LoadingState, Modal, PublicDetailIcon, PublicImageDetail, copyText, publicDetailButton, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { publicGalleryCardView, publicGallerySearchText } from './publicGalleryModel'

const shell = {
  content: { padding: 40 } as const,
  header: { marginBottom: 40, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' as const },
  title: { fontSize: 48, margin: 0 },
  filters: { display: 'flex', gap: 12, marginBottom: 32, flexWrap: 'wrap' as const },
  filterButton: { padding: '8px 16px', background: 'var(--vault-panel)', border: '1px solid var(--vault-line)', borderRadius: 8, fontSize: 14, color: 'var(--vault-muted)', cursor: 'pointer' },
  activeFilter: { borderColor: 'var(--vault-gold)', color: 'var(--vault-gold)' },
}

function downloadFilename(image: Pick<ImageResult, 'id' | 'url' | 'download_url'>) {
  const source = image.download_url ?? image.url ?? ''
  const clean = source.split('?')[0]
  const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
  return `${image.id || 'image'}${ext}`
}

export function PublicGalleryPage() {
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
      const page = await openApi.listPublicGallery(1, 40, { accessToken: app.session?.token, liked: filter === 'liked', favorited: filter === 'favorited' })
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
  }, [app, app.session?.token, filter])

  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return rows
    return rows.filter((image) => {
      return publicGallerySearchText(image).includes(keyword)
    })
  }, [query, rows])

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, null)
  }

  function requireLogin(action = '请先登录后再查看完整作品') {
    if (app.session?.token) return true
    app.notify('info', action)
    app.navigate('login', { returnTo: 'public-gallery' })
    return false
  }

  async function openDetail(image: ImageResult) {
    if (!requireLogin()) return
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
      requireLogin(kind === 'like' ? '请先登录后再点赞' : '请先登录后再收藏')
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
      requireLogin('请先登录后再查看完整提示词')
      return
    }
    await copyText(prompt)
    app.notify('success', 'Prompt 已复制')
  }

  function generateSame(image: ImageResult) {
    if (!requireLogin('请先登录后再同款生成')) return
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
    <div className="content" style={shell.content}>
      <div className="header" style={shell.header}>
        <div>
          <p className="eyebrow">PUBLIC GALLERY</p>
          <h1 style={shell.title}>公开广场</h1>
        </div>
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索提示词、模型或作者" style={{ width: 280, borderRadius: 8 }} />
      </div>

      <div className="filters" style={shell.filters}>
        {(['all', 'liked', 'favorited'] as const).map((item) => (
          <button key={item} type="button" className={`filter-btn${filter === item ? ' active' : ''}`} style={{ ...shell.filterButton, ...(filter === item ? shell.activeFilter : {}) }} onClick={() => {
            if (item !== 'all' && !requireLogin('请先登录后再查看个人互动内容')) return
            setFilter(item)
          }}>
            {item === 'all' ? '全部公开' : item === 'liked' ? '已点赞' : '已收藏'}
          </button>
        ))}
      </div>

      {loading ? <LoadingState label="正在读取公开广场..." /> : null}
      {!loading && !filtered.length ? <EmptyState title="暂无公开作品" detail="公开广场未开启或暂无审核通过的图片。" /> : null}

      <div className="gallery-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 24 }}>
        {filtered.map((image) => {
          const card = publicGalleryCardView(image)
          return (
            <article key={image.id} className="asset-card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', overflow: 'hidden' }}>
              <button type="button" className="asset-thumb" style={{ width: '100%', aspectRatio: '1', background: 'var(--vault-bg)', overflow: 'hidden' }} onClick={() => void openDetail(image)} disabled={busyId === `detail:${image.id}`}>
                {image.url ? <img src={assetUrl(image.url)} alt={card.title || image.id} style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : null}
              </button>
              <div className="asset-info" style={{ padding: 16 }}>
                <div className="asset-title" style={{ fontSize: 14, fontWeight: 700 }}>{card.title}</div>
                <div className="asset-meta" style={{ fontSize: 12, color: 'var(--vault-muted)' }}>
                  {card.taskType} · {card.model} · {card.quality} · {card.aspectRatio}
                </div>
                <div className="asset-meta" style={{ fontSize: 12, color: 'var(--vault-muted)', marginTop: 4 }}>
                  {card.author} · {card.date} · {card.status}
                </div>
                <div className="asset-icon-actions">
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

      {selected ? (
        <Modal title="公开图片详情" onClose={() => setSelected(null)}>
          <PublicImageDetail
            image={selected}
            imageUrl={selected.url || selected.download_url ? assetUrl(selected.url || selected.download_url || '') : undefined}
            onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
            onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
            onDownload={(image) => downloadImage(image as ImageResult)}
            onCopyPrompt={(prompt) => void copyPrompt(prompt)}
            actions={[{ key: 'same', label: '同款生成', icon: <PublicDetailIcon name="edit" />, onClick: () => generateSame(selected), disabled: !selected.prompt }]}
          />
        </Modal>
      ) : null}
    </div>
  )
}
