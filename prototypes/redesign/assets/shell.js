/* Shared prototype shell + interaction layer.
   Pages opt in with <body data-site="user|admin" data-page="..."> and call
   Mikiko.renderShell() once. Everything is vanilla and works from file://. */
(function () {
  'use strict';

  const ICONS = {
    home: '<path d="M3 10.5 12 3l9 7.5"/><path d="M5.5 9.5V21h13V9.5"/><path d="M9.5 21v-6h5v6"/>',
    spark: '<path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8"/>',
    grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    film: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4"/>',
    layers: '<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 13 9 5 9-5"/><path d="m3 17 9 5 9-5"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
    coin: '<circle cx="12" cy="12" r="9"/><path d="M12 7v10M9.5 9.5h4a1.8 1.8 0 0 1 0 3.6h-3a1.8 1.8 0 0 0 0 3.6h4"/>',
    key: '<circle cx="8" cy="15" r="4"/><path d="m11 12 8-8M16 4l4 4M14 6l4 4"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.09A1.7 1.7 0 0 0 9 19.4a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.09A1.7 1.7 0 0 0 15 4.6a1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9c.27.6.84 1 1.5 1H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.51 1Z"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    bell: '<path d="M18 8a6 6 0 1 0-12 0c0 7-3 8-3 8h18s-3-1-3-8"/><path d="M10.3 21a2 2 0 0 0 3.4 0"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.5 6.5 0 0 0 9.8 9.8Z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    minus: '<path d="M5 12h14"/>',
    x: '<path d="m6 6 12 12M18 6 6 18"/>',
    chevron: '<path d="m9 6 6 6-6 6"/>',
    chevronDown: '<path d="m6 9 6 6 6-6"/>',
    chevronLeft: '<path d="m15 6-6 6 6 6"/>',
    arrowUpRight: '<path d="M7 17 17 7M8 7h9v9"/>',
    refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
    download: '<path d="M12 3v12M7 10l5 5 5-5"/><path d="M4 20h16"/>',
    upload: '<path d="M12 16V4M7 9l5-5 5 5"/><path d="M4 20h16"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
    check: '<path d="m4 12 5 5L20 6"/>',
    play: '<path d="M8 5v14l11-7Z"/>',
    pause: '<path d="M8 5v14M16 5v14"/>',
    trash: '<path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13"/>',
    edit: '<path d="M4 20h4L19 9l-4-4L4 16v4Z"/><path d="m14 6 4 4"/>',
    dots: '<circle cx="5" cy="12" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="19" cy="12" r="1.4"/>',
    heart: '<path d="M12 20s-7-4.4-7-9.2A4 4 0 0 1 12 7.5 4 4 0 0 1 19 10.8C19 15.6 12 20 12 20Z"/>',
    star: '<path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 9.5l6.1-.9L12 3Z"/>',
    image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m21 16-5-5L5 20"/>',
    users: '<circle cx="9" cy="8" r="3.4"/><path d="M3 20a6 6 0 0 1 12 0"/><path d="M16 5.2a3.4 3.4 0 0 1 0 6.6M17.5 14.4A6 6 0 0 1 21 20"/>',
    shield: '<path d="M12 3 4.5 6v6c0 4.5 3.2 7.5 7.5 9 4.3-1.5 7.5-4.5 7.5-9V6L12 3Z"/><path d="m9 12 2 2 4-4"/>',
    activity: '<path d="M3 12h4l2.5-6 4 12 2.5-6h5"/>',
    server: '<rect x="3" y="4" width="18" height="6" rx="1.6"/><rect x="3" y="14" width="18" height="6" rx="1.6"/><path d="M7 7h.01M7 17h.01"/>',
    route: '<circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="6" r="2.5"/><path d="M8.5 18H14a4 4 0 0 0 0-8h-4a4 4 0 0 1 0-8h5.5"/>',
    tag: '<path d="M3 12V4h8l10 10-8 8L3 12Z"/><circle cx="8" cy="8" r="1.4"/>',
    receipt: '<path d="M5 3h14v18l-3-2-2 2-2-2-2 2-2-2-3 2V3Z"/><path d="M9 8h6M9 12h6"/>',
    creditCard: '<rect x="2.5" y="5" width="19" height="14" rx="2.4"/><path d="M2.5 10h19"/>',
    gift: '<path d="M4 11h16v9H4v-9Z"/><path d="M3 7h18v4H3V7ZM12 7v13"/><path d="M12 7C10 7 7.5 6.4 7.5 4.6 7.5 3.4 8.5 3 9.4 3c1.7 0 2.6 2.4 2.6 4ZM12 7c2 0 4.5-.6 4.5-2.4 0-1.2-1-1.6-1.9-1.6C12.9 3 12 5.4 12 7Z"/>',
    clipboard: '<rect x="5" y="4" width="14" height="17" rx="2"/><path d="M9 4V3h6v1M9 10h6M9 14h6M9 18h4"/>',
    logout: '<path d="M9 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4"/><path d="M15 8l4 4-4 4M19 12H9"/>',
    menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
    external: '<path d="M14 4h6v6"/><path d="M20 4 10 14"/><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4"/>',
    filter: '<path d="M3 5h18l-7 8v6l-4 2v-8L3 5Z"/>',
    eye: '<path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.6"/>',
    lock: '<rect x="4.5" y="10" width="15" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
    mail: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3.5 6.5 8.5 6 8.5-6"/>',
    pointer: '<path d="m5 3 14 8-6 1.5L9 19 5 3Z"/>',
    zoomIn: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5M11 8v6M8 11h6"/>',
    fullscreen: '<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/>',
    wand: '<path d="m4 20 10-10M14 4l1 2 2 1-2 1-1 2-1-2-2-1 2-1 1-2ZM19 11l.7 1.3L21 13l-1.3.7L19 15l-.7-1.3L17 13l1.3-.7L19 11Z"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    more: '<path d="M6 9v6M12 9v6M18 9v6"/>',
    link: '<path d="M10 13a4 4 0 0 0 5.7 0l2.8-2.8a4 4 0 0 0-5.7-5.7L11.5 6"/><path d="M14 11a4 4 0 0 0-5.7 0l-2.8 2.8a4 4 0 0 0 5.7 5.7L12.5 18"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 7.5h.01"/>',
    warn: '<path d="M12 3 2.5 20h19L12 3Z"/><path d="M12 9v5M12 17h.01"/>',
    ban: '<circle cx="12" cy="12" r="9"/><path d="m6 6 12 12"/>',
    archive: '<rect x="3" y="4" width="18" height="5" rx="1.5"/><path d="M5 9v11h14V9M10 13h4"/>',
    locked: '<rect x="4.5" y="10" width="15" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><path d="M12 14v2"/>',
    uploadCloud: '<path d="M7 18a4 4 0 0 1-.5-8 6 6 0 0 1 11.4 1.6A3.5 3.5 0 0 1 17.5 18H7Z"/><path d="M12 15V9M9.5 11.5 12 9l2.5 2.5"/>',
  };

  function icon(name, cls) {
    const path = ICONS[name] || ICONS.info;
    return '<svg class="' + (cls || '') + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + path + '</svg>';
  }

  function esc(value) {
    return String(value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  const fallback = (seed, w, h) => {
    const palettes = [
      ['#241d16', '#4a3524', '#d9a86a'], ['#161b22', '#222d3a', '#8ab6f0'],
      ['#1c171f', '#2f2738', '#9c90ff'], ['#151c1a', '#233a33', '#62c3a2'],
      ['#1e1815', '#3a2a21', '#e18a63'], ['#14161c', '#242833', '#c9cfda'],
    ];
    const p = palettes[Math.abs(seed.split('').reduce((a, c) => a + c.charCodeAt(0), 0)) % palettes.length];
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + w + '" height="' + h + '"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="' + p[0] + '"/><stop offset=".55" stop-color="' + p[1] + '"/><stop offset="1" stop-color="' + p[0] + '"/></linearGradient><radialGradient id="r" cx=".7" cy=".2" r=".8"><stop offset="0" stop-color="' + p[2] + '" stop-opacity=".55"/><stop offset="1" stop-color="' + p[2] + '" stop-opacity="0"/></radialGradient></defs><rect width="100%" height="100%" fill="url(#g)"/><rect width="100%" height="100%" fill="url(#r)"/><circle cx="' + (w * 0.68) + '" cy="' + (h * 0.34) + '" r="' + (Math.min(w, h) * 0.22) + '" fill="none" stroke="' + p[2] + '" stroke-opacity=".28"/><circle cx="' + (w * 0.34) + '" cy="' + (h * 0.7) + '" r="' + (Math.min(w, h) * 0.3) + '" fill="' + p[2] + '" fill-opacity=".06"/></svg>';
    return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  };

  function photo(seed, w, h) {
    return 'https://picsum.photos/seed/' + encodeURIComponent(seed) + '/' + w + '/' + h;
  }

  // Any image that fails (offline) falls back to a generated gradient plate.
  document.addEventListener('error', (event) => {
    const el = event.target;
    if (el && el.tagName === 'IMG' && !el.dataset.fellBack) {
      el.dataset.fellBack = '1';
      const seed = el.dataset.seed || el.alt || 'mikiko';
      el.src = fallback(seed, 900, 1100);
    }
  }, true);

  const POINTS_TIERS = [
    { factor: 1000000, label: '钻石积分', short: '钻石' },
    { factor: 10000, label: '金币积分', short: '金币' },
    { factor: 100, label: '银币积分', short: '银币' },
    { factor: 1, label: '积分', short: '积分' },
  ];

  function formatPoints(value, short) {
    let raw = String(value == null ? '' : value).trim().replace(/,/g, '');
    if (!/^-?\d+(\.\d+)?$/.test(raw)) raw = '0';
    const negative = raw.charAt(0) === '-';
    const unsigned = negative ? raw.slice(1) : raw;
    const pieces = unsigned.split('.');
    const frac = pieces[1] || '';
    let remainder = BigInt(pieces[0] || '0');
    const parts = [];
    POINTS_TIERS.forEach((tier) => {
      if (tier.factor === 1) {
        const whole = remainder.toString();
        if (whole !== '0' || frac) {
          const amount = (frac ? whole + '.' + frac.replace(/0+$/, '') : whole).replace(/\.$/, '');
          if (amount && amount !== '0') parts.push({ amount, label: short ? tier.short : tier.label });
        }
        return;
      }
      const count = remainder / BigInt(tier.factor);
      remainder %= BigInt(tier.factor);
      if (count > 0n) parts.push({ amount: count.toString(), label: short ? tier.short : tier.label });
    });
    if (!parts.length) parts.push({ amount: '0', label: short ? '积分' : '积分' });
    if (negative) parts[0].amount = '-' + parts[0].amount;
    return parts.map((part) => part.amount + ' ' + part.label).join(' ');
  }

  function populatePoints(root) {
    (root || document).querySelectorAll('[data-points]').forEach((el) => {
      el.textContent = formatPoints(el.dataset.points, el.hasAttribute('data-points-short'));
    });
  }

  function ensureMediaDrawer() {
    if (document.getElementById('media-detail') || !document.querySelector('.u-shell')) return;
    if (!document.querySelector('.u-backdrop')) {
      const backdrop = document.createElement('div');
      backdrop.className = 'u-backdrop';
      document.body.appendChild(backdrop);
    }
    const holder = document.createElement('div');
    holder.innerHTML =
      '<aside class="u-drawer" id="media-detail" role="dialog" aria-modal="true" aria-label="媒体详情">' +
      '<header class="u-drawer__head">' +
      '<div class="u-drawer__title"><strong data-media-title>资产详情</strong><span data-media-meta>media_7f3a · 默认项目</span></div>' +
      '<div class="u-drawer__spacer"></div>' +
      '<a class="u-btn u-btn--sm" href="detail.html" data-detail-link>详情</a>' +
      '<button class="u-btn u-btn--sm" type="button" data-toast="已复制分享链接">分享</button>' +
      '<button class="u-icon-btn" type="button" data-close aria-label="关闭">✕</button>' +
      '</header>' +
      '<div class="u-drawer__body"><div class="u-detail">' +
      '<section class="u-detail__stage-col">' +
      '<div class="u-detail__stage"><span class="u-detail__stage-label"><span class="u-chip u-chip--mono" data-media-status>已完成</span><span class="u-chip u-chip--mono" data-media-size>1120 × 1400</span></span>' +
      '<button class="u-icon-btn u-detail__zoom" type="button" data-toast="全屏缩放：滚轮 60%–400%，拖拽平移" aria-label="全屏查看">⤢</button>' +
      '<img data-media-image src="' + photo('media-default', 900, 1125) + '" alt="媒体预览">' +
      '</div>' +
      '<div class="u-actions" style="justify-content:space-between"><div class="u-actions">' +
      '<button class="u-action" type="button" data-toast="已点赞">♡ 点赞</button>' +
      '<button class="u-action" type="button" data-toast="已收藏">☆ 收藏</button>' +
      '<button class="u-action" type="button" data-copy="黄昏时分的玻璃温室，暖金色斜光，胶片颗粒。">复制 Prompt</button>' +
      '</div><div class="u-actions"><button class="u-action" type="button" data-toast="上一张">←</button><button class="u-action" type="button" data-toast="下一张">→</button></div></div>' +
      '</section>' +
      '<section class="u-detail__info"><div class="u-detail__info-scroll">' +
      '<div style="display:grid;gap:7px"><div style="display:flex;gap:6px;flex-wrap:wrap"><span class="u-chip u-chip--accent" data-media-type>图片</span><span class="u-chip u-chip--mono" data-media-model>plus-image</span><span class="u-chip u-chip--mono">已公开</span></div>' +
      '<h2 class="u-detail__title" data-media-title>资产标题</h2><div class="u-meta" data-media-meta>2026-10-22 · 默认项目</div></div>' +
      '<div class="u-detail__section"><h4>Prompt</h4><div class="u-prompt" data-media-prompt>黄昏时分的玻璃温室，暖金色斜光穿过锈蚀的钢架，空气中有细小尘埃悬浮。胶片颗粒，柔和的高光溢出，安静的中画幅质感。</div></div>' +
      '<div class="u-detail__section"><h4>生成参数</h4><dl class="u-facts"><div class="u-fact"><dt>实际尺寸</dt><dd data-media-size>1120 × 1400</dd></div><div class="u-fact"><dt>比例</dt><dd>4:5</dd></div><div class="u-fact"><dt>质量</dt><dd>high</dd></div><div class="u-fact"><dt>输出格式</dt><dd>PNG</dd></div><div class="u-fact"><dt>审核等级</dt><dd>标准</dd></div><div class="u-fact"><dt>归属</dt><dd>默认项目</dd></div></dl></div>' +
      '<div class="u-detail__section"><h4>消耗与任务</h4><div class="u-estimate"><div class="u-estimate__row"><span>本次任务</span><b data-media-cost>24.00 积分</b></div><div class="u-estimate__row"><span>任务 ID</span><b style="font-size:11px">task_8f21c4e0</b></div><div class="u-estimate__row"><span>创建时间</span><b>10-22 18:42</b></div></div></div>' +
      '</div><div class="u-detail__foot"><a class="u-btn u-btn--primary u-btn--sm" href="genpic.html">复用这组参数</a><button class="u-btn u-btn--sm" type="button" data-toast="开始下载原图">下载原图</button><button class="u-icon-btn" type="button" data-toast="更多操作" aria-label="更多操作">⋯</button></div></section>' +
      '</div></div></aside>';
    document.body.appendChild(holder.firstElementChild);
  }

  function populateMediaDrawer(opener) {
    const drawer = document.getElementById('media-detail');
    if (!drawer || !drawer.querySelector('[data-media-image]')) return;
    const img = opener.querySelector('img');
    const card = opener.closest('.u-card, .u-history-card, article');
    const titleEl = card && card.querySelector('.u-card__title, .u-history-card__body strong');
    const metaEl = card && card.querySelector('.u-card__meta');
    const title = opener.dataset.mediaTitle || (titleEl && titleEl.textContent.trim()) || (img && img.alt) || '资产详情';
    const metaParts = metaEl ? Array.from(metaEl.children).map((item) => item.textContent.trim()).filter(Boolean) : [];
    const meta = opener.dataset.mediaMeta || metaParts.join(' · ') || (metaEl && metaEl.textContent.replace(/\s+/g, ' ').trim()) || '2026-10-22 · 默认项目';
    const tag = card && card.querySelector('.u-card__tag');
    const rawTag = tag && tag.textContent.trim();
    const type = opener.dataset.mediaType || (rawTag && /(图片|视频|音频)/.test(rawTag) ? rawTag : '图片');
    const model = opener.dataset.mediaModel || 'plus-image';
    const size = opener.dataset.mediaSize || '1120 × 1400';
    const source = opener.dataset.mediaSrc || (img && (img.currentSrc || img.src)) || '';
    drawer.querySelectorAll('[data-media-title]').forEach((el) => { el.textContent = title; });
    drawer.querySelectorAll('[data-media-meta]').forEach((el) => { el.textContent = meta; });
    drawer.querySelectorAll('[data-media-type]').forEach((el) => { el.textContent = type; });
    drawer.querySelectorAll('[data-media-model]').forEach((el) => { el.textContent = model; });
    drawer.querySelectorAll('[data-media-size]').forEach((el) => { el.textContent = size; });
    drawer.querySelectorAll('[data-media-cost]').forEach((el) => { el.textContent = opener.dataset.mediaCost || '24.00 积分'; });
    const media = drawer.querySelector('[data-media-image]');
    if (media && source) { media.src = source; media.alt = title; }
  }

  const USER_NAV = [
    { route: 'home', label: '首页', icon: 'home' },
    { route: 'genpic', label: '创作', icon: 'spark' },
    { route: 'creative-canvas', label: '画布', icon: 'layers' },
    { route: 'gallery', label: '资产', icon: 'grid' },
    { route: 'projects', label: '项目', icon: 'folder' },
    { route: 'public-gallery', label: '广场', icon: 'film' },
    { route: 'checkout', label: '积分', icon: 'coin' },
    { route: 'api-keys', label: '密钥', icon: 'key' },
    { route: 'settings', label: '设置', icon: 'settings' },
  ];

  function renderUserShell(options) {
    const opts = options || {};
    const body = document.body;
    const page = body.dataset.page || 'home';
    const active = opts.active || (page === 'profile' ? 'settings' : page);
    const theme = localStorage.getItem('mikiko-proto-theme-user') || 'dark';
    body.dataset.theme = theme;

    const ambient = document.createElement('div');
    ambient.className = 'u-ambient';
    const grain = document.createElement('div');
    grain.className = 'u-grain';
    body.prepend(grain);
    body.prepend(ambient);

    const shell = document.querySelector('.u-shell');
    if (!shell) return;

    const rail = document.createElement('aside');
    rail.className = 'u-rail';
    rail.innerHTML =
      '<a class="u-rail__brand" href="landing.html" aria-label="Mikiko Studio 首页">M</a>' +
      '<nav class="u-rail__nav" aria-label="主导航"><span class="u-rail__indicator" data-rail-indicator></span>' +
      USER_NAV.map((item) =>
        '<a class="u-rail__link' + (active === item.route ? ' is-active' : '') + '" href="' + item.route + '.html" data-route="' + item.route + '">' +
        icon(item.icon) + '<span>' + item.label + '</span></a>'
      ).join('') +
      '</nav><div class="u-rail__spacer"></div>' +
      '<div class="u-rail__foot">' +
      '<button class="u-icon-btn u-icon-btn--plain" type="button" data-theme-toggle aria-label="切换主题">' + icon('sun') + '</button>' +
      '<a class="u-icon-btn u-icon-btn--plain" href="login.html" aria-label="退出登录">' + icon('logout') + '</a>' +
      '</div>';

    const main = shell.querySelector('.u-main');
    const topbar = document.createElement('header');
    topbar.className = 'u-topbar';
    topbar.innerHTML =
      '<div class="u-topbar__context">' +
      '<button class="u-topbar__project" type="button" data-project-picker>' + icon('folder') +
      '<span>夜航系列</span>' + icon('chevronDown') + '</button>' +
      '</div><div class="u-topbar__spacer"></div>' +
      '<div class="u-topbar__actions">' +
      '<button class="u-icon-btn" type="button" data-toast="暂无新通知">' + icon('bell') + '</button>' +
      '<a class="u-balance" href="checkout.html"><span class="u-topbar__dot"></span><span class="u-balance__label">可用积分</span> <b data-points="4820.50" data-points-short>4,820.50</b>' +
      '<span class="u-balance__plus">' + icon('plus') + '</span></a>' +
      '<button class="u-avatar" type="button" aria-label="账户菜单" data-account-toggle>阿</button>' +
      '</div>';

    shell.insertBefore(rail, shell.firstChild);
    if (main) main.insertBefore(topbar, main.firstChild);

    const accountMenu = document.createElement('div');
    accountMenu.className = 'u-account-menu';
    accountMenu.setAttribute('data-account-menu', '');
    accountMenu.innerHTML =
      '<div class="u-account-menu__head"><span class="u-avatar">阿</span>' +
      '<div class="u-account-menu__identity"><strong>岑屿</strong><span>cenyu@atelier.moe</span></div></div>' +
      '<div class="u-account-menu__balance"><span>可用积分</span><b data-points="4820.50">4,820.50</b></div>' +
      '<a class="u-account-menu__item" href="profile.html">' + icon('users') + '个人中心</a>' +
      '<a class="u-account-menu__item" href="checkout.html">' + icon('coin') + '积分充值</a>' +
      '<a class="u-account-menu__item" href="api-keys.html">' + icon('key') + 'API 密钥</a>' +
      '<a class="u-account-menu__item" href="landing.html#developers">' + icon('external') + '开发文档</a>' +
      '<div class="u-account-menu__sep"></div>' +
      '<a class="u-account-menu__item u-account-menu__item--danger" href="login.html">' + icon('logout') + '退出登录</a>';
    document.body.appendChild(accountMenu);
    window.addEventListener('resize', closeAccountMenu);
    window.addEventListener('scroll', closeAccountMenu, true);

    ensureMediaDrawer();
    populatePoints();

    // rail indicator follows the active link
    requestAnimationFrame(() => {
      const activeLink = rail.querySelector('.u-rail__link.is-active');
      const indicator = rail.querySelector('[data-rail-indicator]');
      if (activeLink && indicator) {
        indicator.style.transform = 'translateY(' + activeLink.offsetTop + 'px)';
      }
    });

    bindThemeToggle(body, 'mikiko-proto-theme-user', 'sun', 'moon');
  }

  const ADMIN_NAV = [
    { group: '概览', items: [
      { route: 'dashboard', label: '运营总览', icon: 'activity' },
      { route: 'monitoring', label: '系统健康', icon: 'server' },
      { route: 'cluster', label: '集群节点', icon: 'layers' },
    ] },
    { group: '用户与内容', items: [
      { route: 'users', label: '用户', icon: 'users' },
      { route: 'user-groups', label: '用户分组', icon: 'tag' },
      { route: 'reviews', label: '审核队列', icon: 'shield', badge: '7' },
      { route: 'redeem', label: '兑换码', icon: 'gift' },
    ] },
    { group: '交易', items: [
      { route: 'orders', label: '订单', icon: 'receipt' },
      { route: 'packages', label: '套餐', icon: 'creditCard' },
      { route: 'cashier-config', label: '支付配置', icon: 'creditCard' },
    ] },
    { group: '模型与生成', items: [
      { route: 'access-accounts', label: '接入账号', icon: 'server' },
      { route: 'routing', label: '路由模型', icon: 'route' },
      { route: 'video-tasks', label: '视频任务', icon: 'film' },
      { route: 'call-records', label: '图片任务', icon: 'image' },
    ] },
    { group: '系统', items: [
      { route: 'system-users', label: '管理员', icon: 'lock' },
      { route: 'audit', label: '审计日志', icon: 'clipboard' },
      { route: 'system-settings', label: '系统设置', icon: 'settings' },
    ] },
  ];

  function renderAdminShell(options) {
    const opts = options || {};
    const body = document.body;
    const page = body.dataset.page || 'dashboard';
    const active = opts.active || page;
    const theme = localStorage.getItem('mikiko-proto-theme-admin') || 'light';
    body.dataset.theme = theme;

    const shell = document.querySelector('.a-shell');
    if (!shell) return;

    const side = document.createElement('aside');
    side.className = 'a-side';
    side.innerHTML =
      '<div class="a-side__brand"><span class="a-side__mark">M</span>' +
      '<span class="a-side__name"><strong>Mikiko Ops</strong><span>Command Deck</span></span></div>' +
      '<nav class="a-side__nav" aria-label="后台导航">' +
      ADMIN_NAV.map((group) =>
        '<div class="a-side__group"><div class="a-side__label">' + group.group + '</div>' +
        group.items.map((item) =>
          '<a class="a-side__link' + (active === item.route ? ' is-active' : '') + '" href="' + item.route + '.html">' +
          icon(item.icon) + '<span>' + item.label + '</span>' +
          (item.badge ? '<span class="a-side__badge">' + item.badge + '</span>' : '') + '</a>'
        ).join('') + '</div>'
      ).join('') +
      '</nav>' +
      '<div class="a-side__foot">' +
      '<div class="a-side__foot-identity"><span class="a-cell-entity__avatar">林</span>' +
      '<div class="a-side__identity-copy"><strong>林知远</strong><span>lin.zhiyuan@studio.moe</span><span class="a-side__role">super_admin</span></div></div>' +
      '<button class="a-btn a-btn--quiet a-btn--sm" type="button" data-toast="已安全退出（原型）">' + icon('logout') + '退出登录</button>' +
      '</div>';

    const main = shell.querySelector('.a-main');
    const top = document.createElement('header');
    top.className = 'a-top';
    top.innerHTML =
      '<button class="a-icon-btn a-side-toggle" type="button" data-side-toggle aria-label="打开导航">' + icon('menu') + '</button>' +
      '<div class="a-top__crumbs"><span>控制台</span>' + icon('chevron') +
      '<strong>' + esc(body.dataset.title || opts.title || '运营总览') + '</strong></div>' +
      '<div class="a-top__spacer"></div>' +
      '<div class="a-top__actions">' +
      '<button class="a-btn a-btn--quiet a-btn--sm" type="button" data-toast="搜索面板：支持用户 / 订单 / 任务 / 模型（原型）">' + icon('search') + '搜索</button>' +
      '<button class="a-icon-btn" type="button" data-theme-toggle aria-label="切换主题">' + icon('moon') + '</button>' +
      '<button class="a-icon-btn" type="button" data-toast="7 条待审核、2 个渠道回调失败" aria-label="通知">' + icon('bell') + '</button>' +
      '<span class="a-cell-entity__avatar">林</span>' +
      '</div>';

    shell.insertBefore(side, shell.firstChild);
    if (main) main.insertBefore(top, main.firstChild);

    bindThemeToggle(body, 'mikiko-proto-theme-admin', 'moon', 'sun');

    const toggle = top.querySelector('[data-side-toggle]');
    toggle.addEventListener('click', () => side.classList.toggle('is-open'));
    side.addEventListener('click', (event) => {
      if (event.target.closest('a') && window.matchMedia('(max-width: 1080px)').matches) side.classList.remove('is-open');
    });
  }

  function bindThemeToggle(body, key, darkIcon, lightIcon) {
    const buttons = document.querySelectorAll('[data-theme-toggle]');
    const sync = () => {
      const isDark = body.dataset.theme === 'dark';
      buttons.forEach((btn) => { btn.innerHTML = icon(isDark ? lightIcon : darkIcon); });
    };
    sync();
    buttons.forEach((btn) => btn.addEventListener('click', () => {
      body.dataset.theme = body.dataset.theme === 'dark' ? 'light' : 'dark';
      localStorage.setItem(key, body.dataset.theme);
      sync();
    }));
  }

  function toast(message, tone) {
    let stack = document.querySelector('.u-toast-stack, .a-toast-stack');
    if (!stack) {
      stack = document.createElement('div');
      stack.className = document.body.dataset.site === 'admin' ? 'a-toast-stack' : 'u-toast-stack';
      document.body.appendChild(stack);
    }
    const isAdmin = document.body.dataset.site === 'admin';
    const el = document.createElement('div');
    el.className = (isAdmin ? 'a-toast' : 'u-toast') + (tone === 'bad' ? (isAdmin ? ' a-toast--bad' : ' u-toast--bad') : tone === 'ok' ? (isAdmin ? ' a-toast--ok' : ' u-toast--ok') : '');
    el.innerHTML = (isAdmin ? '<div><strong>' + esc(message) + '</strong><span>原型交互反馈</span></div>' : '<span>' + esc(message) + '</span>');
    stack.appendChild(el);
    setTimeout(() => {
      el.style.opacity = '0';
      el.style.transform = 'translateY(6px)';
      el.style.transition = 'opacity .2s, transform .2s';
      setTimeout(() => el.remove(), 220);
    }, 2600);
  }

  function openLayer(target) {
    const layer = typeof target === 'string' ? document.querySelector(target) : target;
    if (!layer) return;
    const stacked = layer.dataset.stack === 'true';
    // nested drawers (for example model edit on top of model config) keep
    // the parent layer open; everything else closes the previous layer.
    if (!stacked) {
      document.querySelectorAll('.u-drawer.is-open, .u-modal.is-open, .a-drawer.is-open, .a-modal.is-open').forEach((open) => {
        if (open !== layer) open.classList.remove('is-open', 'is-stacked');
      });
    }
    layer.classList.add('is-open');
    if (stacked) layer.classList.add('is-stacked');
    const scrim = document.querySelector('.a-scrim');
    if (scrim && (layer.classList.contains('a-drawer') || layer.classList.contains('a-modal'))) scrim.classList.add('is-open');
    const uScrim = document.querySelector('.u-backdrop');
    if (uScrim && (layer.classList.contains('u-drawer') || layer.classList.contains('u-modal'))) uScrim.classList.add('is-open');
    document.body.style.overflow = 'hidden';
    const focusable = layer.querySelector('button, [href], input, select, textarea');
    if (focusable) setTimeout(() => focusable.focus({ preventScroll: true }), 40);
  }

  function closeLayer(layer) {
    if (!layer) return;
    const wasStacked = layer.classList.contains('is-stacked');
    layer.classList.remove('is-open', 'is-stacked');
    if (!wasStacked) {
      document.querySelectorAll('.is-stacked.is-open').forEach((nested) => nested.classList.remove('is-open', 'is-stacked'));
    }
    if (!document.querySelector('.u-drawer.is-open, .u-modal.is-open, .a-drawer.is-open, .a-modal.is-open')) {
      document.body.style.overflow = '';
      const scrim = document.querySelector('.a-scrim.is-open');
      if (scrim) scrim.classList.remove('is-open');
      const uScrim = document.querySelector('.u-backdrop.is-open');
      if (uScrim) uScrim.classList.remove('is-open');
    }
  }

  function toggleAccountMenu() {
    const toggle = document.querySelector('[data-account-toggle]');
    const menu = document.querySelector('[data-account-menu]');
    if (!toggle || !menu) return;
    if (menu.classList.contains('is-open')) {
      menu.classList.remove('is-open');
      return;
    }
    const rect = toggle.getBoundingClientRect();
    menu.style.top = Math.round(rect.bottom + 8) + 'px';
    menu.style.right = Math.max(8, Math.round(window.innerWidth - rect.right)) + 'px';
    menu.classList.add('is-open');
  }

  function closeAccountMenu() {
    const menu = document.querySelector('[data-account-menu].is-open');
    if (menu) menu.classList.remove('is-open');
  }

  function updateMultiselect(box) {
    const selected = Array.from(box.querySelectorAll('.a-multiselect__panel input[type="checkbox"]:checked')).map((input) => input.value);
    const summary = box.querySelector('[data-ms-summary]');
    if (summary) summary.textContent = selected.length ? selected.join('、') : '请选择';
  }

  function closeMultiselects(except) {
    document.querySelectorAll('.a-multiselect.is-open').forEach((box) => {
      if (box !== except) box.classList.remove('is-open');
    });
  }

  function bindInteractions() {
    document.addEventListener('click', (event) => {
      const accountToggle = event.target.closest('[data-account-toggle]');
      if (accountToggle) {
        event.preventDefault();
        toggleAccountMenu();
        return;
      }
      if (!event.target.closest('[data-account-menu]')) closeAccountMenu();
      const detailLink = event.target.closest('[data-detail-link]');
      if (detailLink) {
        const from = location.pathname.split('/').pop() + location.search;
        detailLink.href = 'detail.html?from=' + encodeURIComponent(from);
        try { sessionStorage.setItem('mikiko-detail-from', location.pathname + location.search); } catch (err) { /* storage may be unavailable */ }
      }
      const msToggle = event.target.closest('[data-ms-toggle]');
      if (msToggle) {
        event.preventDefault();
        const box = msToggle.closest('.a-multiselect');
        const opening = !box.classList.contains('is-open');
        closeMultiselects(box);
        box.classList.toggle('is-open', opening);
        updateMultiselect(box);
        if (opening) {
          const panel = box.querySelector('.a-multiselect__panel');
          if (panel) requestAnimationFrame(() => panel.scrollIntoView({ block: 'nearest', behavior: 'smooth' }));
        }
        return;
      }
      const msAdd = event.target.closest('[data-ms-add]');
      if (msAdd) {
        event.preventDefault();
        const box = msAdd.closest('.a-multiselect');
        const input = box.querySelector('.a-multiselect__custom input');
        const value = input && input.value.trim();
        if (value) {
          const exists = Array.from(box.querySelectorAll('.a-multiselect__panel input[type="checkbox"]')).some((item) => item.value === value);
          if (!exists) {
            const label = document.createElement('label');
            label.innerHTML = '<input type="checkbox" checked value="' + esc(value) + '">' + esc(value);
            box.querySelector('.a-multiselect__options').appendChild(label);
          }
          if (input) input.value = '';
          updateMultiselect(box);
        }
        return;
      }
      if (!event.target.closest('.a-multiselect')) closeMultiselects(null);
      const toastEl = event.target.closest('[data-toast]');
      if (toastEl) {
        event.preventDefault();
        toast(toastEl.dataset.toast, toastEl.dataset.tone);
        return;
      }
      const opener = event.target.closest('[data-open]');
      if (opener) {
        event.preventDefault();
        const target = opener.dataset.open;
        if (target === '#media-detail') populateMediaDrawer(opener);
        openLayer(target);
        if (opener.dataset.openTab) {
          const layer = document.querySelector(target);
          const tab = layer && layer.querySelector('[data-tab="' + opener.dataset.openTab + '"]');
          if (tab) tab.click();
        }
        return;
      }
      const closer = event.target.closest('[data-close]');
      if (closer) {
        event.preventDefault();
        const layer = closer.closest('.u-drawer, .u-modal, .a-drawer, .a-modal, .a-scrim');
        closeLayer(layer && layer.matches('.a-scrim') ? document.querySelector('.a-drawer.is-open') : layer);
        return;
      }
      const scrim = event.target.closest('.a-scrim');
      if (scrim) {
        const drawer = document.querySelector('.a-drawer.is-open');
        closeLayer(drawer);
        scrim.classList.remove('is-open');
        return;
      }
      const uScrim = event.target.closest('.u-backdrop');
      if (uScrim) {
        const layer = document.querySelector('.u-drawer.is-open, .u-modal.is-open');
        if (layer) closeLayer(layer);
        return;
      }
      // copy buttons
      const copyBtn = event.target.closest('[data-copy]');
      if (copyBtn) {
        event.preventDefault();
        const text = copyBtn.dataset.copy;
        if (navigator.clipboard) navigator.clipboard.writeText(text).catch(() => {});
        toast('已复制：' + (text.length > 38 ? text.slice(0, 38) + '…' : text), 'ok');
        return;
      }
      // demo pager
      const pager = event.target.closest('.a-pager__btn, [data-page-btn]');
      if (pager && !pager.disabled && !pager.dataset.toast) {
        const group = pager.closest('.a-pager__controls');
        if (group && !pager.classList.contains('is-active')) {
          group.querySelectorAll('.a-pager__btn').forEach((b) => b.classList.remove('is-active'));
          pager.classList.add('is-active');
          toast('已切换到第 ' + (pager.textContent.trim() || '?') + ' 页（原型数据）');
          return;
        }
      }
      // segmented group
      const seg = event.target.closest('.u-segment button, [data-segment] button, .a-seg button');
      if (seg && !seg.closest('[data-tabs]')) {
        const parent = seg.parentElement;
        parent.querySelectorAll('button').forEach((b) => b.classList.remove('is-active'));
        seg.classList.add('is-active');
        const note = seg.dataset.segmentNote;
        if (note) toast(note);
        return;
      }
      // tabs
      const tab = event.target.closest('[data-tab]');
      if (tab) {
        const group = tab.closest('[data-tabs]');
        if (group) {
          group.querySelectorAll('[data-tab]').forEach((t) => t.classList.remove('is-active'));
          tab.classList.add('is-active');
          const target = tab.dataset.tab;
          const panelWrap = document.querySelector('[data-tab-panels="' + group.dataset.tabs + '"]');
          if (panelWrap) {
            panelWrap.querySelectorAll('[data-panel]').forEach((p) => { p.hidden = p.dataset.panel !== target; });
          }
          if (tab.dataset.tabToast) toast(tab.dataset.tabToast);
        }
        return;
      }
      // filter expand
      const moreBtn = event.target.closest('[data-filter-more]');
      if (moreBtn) {
        const band = moreBtn.closest('.a-filters');
        band.classList.toggle('is-expanded');
        moreBtn.innerHTML = band.classList.contains('is-expanded') ? '收起筛选' : '更多筛选';
        return;
      }
      // table row -> drawer (only if no inner interactive element was hit)
      const row = event.target.closest('tr[data-row-open]');
      if (row && !event.target.closest('button, a, input, select, [data-row-ignore]')) {
        openLayer(row.dataset.rowOpen);
      }
    });

    document.addEventListener('change', (event) => {
      const box = event.target.closest('.a-multiselect');
      if (box) updateMultiselect(box);
    });

    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') {
        closeAccountMenu();
        closeMultiselects(null);
        const open = document.querySelector('.u-drawer.is-open, .u-modal.is-open, .a-drawer.is-open, .a-modal.is-open');
        if (open) {
          closeLayer(open);
          const scrim = document.querySelector('.a-scrim.is-open');
          if (scrim) scrim.classList.remove('is-open');
        }
      }
    });

    // scroll reveal
    const items = document.querySelectorAll('[data-reveal]');
    if ('IntersectionObserver' in window) {
      const io = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            entry.target.classList.add('is-in');
            io.unobserve(entry.target);
          }
        });
      }, { rootMargin: '0px 0px -8% 0px', threshold: .06 });
      items.forEach((el) => io.observe(el));
    } else {
      items.forEach((el) => el.classList.add('is-in'));
    }
  }

  document.addEventListener('DOMContentLoaded', () => {
    const body = document.body;
    if (body.dataset.site === 'user') renderUserShell();
    if (body.dataset.site === 'admin') renderAdminShell();
    // standalone pages (login / detail / index) have no shell but still need
    // theme persistence and a visible toggle icon
    if (!document.querySelector('.u-shell, .a-shell') && body.dataset.site) {
      if (body.dataset.site === 'user') {
        body.dataset.theme = localStorage.getItem('mikiko-proto-theme-user') || 'dark';
        bindThemeToggle(body, 'mikiko-proto-theme-user', 'sun', 'moon');
      } else {
        body.dataset.theme = localStorage.getItem('mikiko-proto-theme-admin') || 'light';
        bindThemeToggle(body, 'mikiko-proto-theme-admin', 'moon', 'sun');
      }
    }
    bindInteractions();
    populatePoints();
    const detailBack = document.querySelector('[data-detail-back]');
    if (detailBack) {
      let from = '';
      try {
        const params = new URLSearchParams(location.search);
        const queryFrom = params.get('from') || '';
        const page = queryFrom.split('?')[0];
        if (/^[a-z0-9-]+\.html$/.test(page)) from = queryFrom;
        if (!from) from = sessionStorage.getItem('mikiko-detail-from') || '';
      } catch (err) { /* storage may be unavailable */ }
      if (from) {
        detailBack.href = from;
        detailBack.textContent = '返回来源';
      }
    }
    // open a layer from the URL hash, e.g. detail.html#open
    if (location.hash === '#open') {
      const first = document.querySelector('[data-auto-open]');
      if (first) openLayer(first.dataset.autoOpen);
    }
  });

  window.Mikiko = {
    icon, photo, fallback, esc, toast, openLayer, closeLayer, formatPoints, populatePoints, ensureMediaDrawer,
    // data helpers for pages that build cards dynamically
    card: function (item, opts) {
      const o = opts || {};
      const type = item.type || 'image';
      return '<article class="u-card" tabindex="0">' +
        (o.select ? '<button class="media-asset-select" type="button" aria-label="选择"></button>' : '') +
        '<a class="u-card__media" href="' + (o.href || '#media-detail') + '" data-open="#media-detail" data-media-title="' + esc(item.title) + '">' +
        '<img src="' + photo(item.seed, 720, 540) + '" data-seed="' + esc(item.seed) + '" alt="' + esc(item.title) + '" loading="lazy">' +
        '<span class="u-card__tag">' + (type === 'video' ? icon('play') + '视频' : '图片') + '</span>' +
        (type === 'video' ? '<span class="u-play"><span>' + icon('play') + '</span></span>' : '') +
        '</a>' +
        '<div class="u-card__body"><div class="u-card__title">' + esc(item.title) + '</div>' +
        '<div class="u-card__meta"><span>' + esc(item.model || '') + '</span><span>' + esc(item.time || '') + '</span></div></div></article>';
    }
  };
})();