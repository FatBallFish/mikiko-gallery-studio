package setupui

const setupDocumentStart = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<title>部署初始化 / Setup console</title>
<style>`

const setupDocumentBody = `</style>
</head>
<body>
<main class="page" id="setup-console">
  <header class="topbar">
    <div>
      <p class="eyebrow">SYSTEM INITIALIZATION</p>
      <h1>部署初始化 <span>/ Setup console</span></h1>
    </div>
    <div class="mode-badge" id="deployment-badge">BOOTSTRAP</div>
  </header>

  <section class="auth-panel" id="auth-panel" aria-labelledby="auth-title">
    <div class="section-heading">
      <div class="step-index">01</div>
      <div>
        <h2 id="auth-title">验证初始化凭证</h2>
        <p>Authenticate setup access</p>
      </div>
    </div>
    <form id="auth-form" novalidate>
      <label for="setup-token">一次性初始化 Token / Setup token</label>
      <div class="auth-row">
        <input id="setup-token" name="setup-token" type="password" autocomplete="off" spellcheck="false" required autofocus>
        <button class="button primary" type="submit" id="authenticate">验证 / Authenticate</button>
      </div>
    </form>
    <div class="token-help">
      <p>未持有 Token / Token unavailable</p>
      <div class="commands">
        <code>deployctl setup token show</code>
        <code>deployctl setup token reset</code>
      </div>
    </div>
    <div class="status-line" id="auth-status" role="status" aria-live="polite"></div>
  </section>

  <div class="workspace" id="workspace" tabindex="-1" hidden>
    <aside class="step-rail" aria-label="初始化步骤 / Setup steps">
      <ol>
        <li class="active" data-nav-step="deployment"><span>01</span><div>部署信息<small>Deployment</small></div></li>
        <li data-nav-step="database"><span>02</span><div>数据库<small>PostgreSQL</small></div></li>
        <li data-nav-step="redis"><span>03</span><div>缓存<small>Redis</small></div></li>
        <li data-nav-step="storage"><span>04</span><div>对象存储<small>Storage</small></div></li>
        <li data-nav-step="administrator"><span>05</span><div>管理员<small>Administrator</small></div></li>
        <li data-nav-step="review"><span>06</span><div>确认执行<small>Review & apply</small></div></li>
      </ol>
    </aside>

    <div class="console-panel">
      <section class="setup-section" data-step="deployment" aria-labelledby="deployment-title">
        <div class="section-heading">
          <div class="step-index">01</div>
          <div><h2 id="deployment-title">部署信息</h2><p>Deployment context</p></div>
        </div>
        <dl class="deployment-grid" id="deployment-summary"></dl>
        <div class="fields-grid" id="public-fields"></div>
      </section>

      <section class="setup-section" data-step="database" aria-labelledby="database-title">
        <div class="section-heading">
          <div class="step-index">02</div>
          <div><h2 id="database-title">PostgreSQL 数据库</h2><p>Database connection and pool</p></div>
        </div>
        <div class="fields-grid" id="database-fields"></div>
        <div class="probe-row">
          <button class="button secondary" type="button" data-probe="database">检测数据库 / Test database</button>
          <div class="probe-status" id="database-probe-status" role="status" aria-live="polite">尚未检测 / Not tested</div>
        </div>
      </section>

      <section class="setup-section" data-step="redis" aria-labelledby="redis-title">
        <div class="section-heading">
          <div class="step-index">03</div>
          <div><h2 id="redis-title">Redis 缓存</h2><p>Shared cache connection</p></div>
        </div>
        <div class="fields-grid" id="redis-fields"></div>
        <div class="probe-row">
          <button class="button secondary" type="button" data-probe="redis">检测 Redis / Test Redis</button>
          <div class="probe-status" id="redis-probe-status" role="status" aria-live="polite">尚未检测 / Not tested</div>
        </div>
      </section>

      <section class="setup-section" data-step="storage" aria-labelledby="storage-title">
        <div class="section-heading">
          <div class="step-index">04</div>
          <div><h2 id="storage-title">对象存储</h2><p>Asset storage backend</p></div>
        </div>
        <div class="fields-grid" id="storage-fields"></div>
        <div class="probe-row">
          <button class="button secondary" type="button" data-probe="storage">检测存储 / Test storage</button>
          <div class="probe-status" id="storage-probe-status" role="status" aria-live="polite">尚未检测 / Not tested</div>
        </div>
      </section>

      <section class="setup-section" data-step="administrator" aria-labelledby="administrator-title">
        <div class="section-heading">
          <div class="step-index">05</div>
          <div><h2 id="administrator-title">初始管理员</h2><p>First super administrator</p></div>
        </div>
        <div class="fields-grid">
          <div class="field">
            <label for="admin-email">管理员邮箱 / Administrator email <span aria-hidden="true">*</span></label>
            <input id="admin-email" type="email" autocomplete="username" maxlength="255" required>
          </div>
          <div class="field">
            <label for="admin-password">管理员密码 / Administrator password <span aria-hidden="true">*</span></label>
            <input id="admin-password" type="password" autocomplete="new-password" minlength="6" maxlength="72" required>
            <p class="field-help">6–72 bytes. 初始化完成后可在管理后台修改。 / Changeable after setup.</p>
          </div>
        </div>
      </section>

      <section class="setup-section" data-step="review" aria-labelledby="review-title">
        <div class="section-heading">
          <div class="step-index">06</div>
          <div><h2 id="review-title">确认并初始化</h2><p>Review and apply</p></div>
        </div>
        <dl class="review-list" id="review-summary"></dl>
        <div class="apply-row">
          <button class="button primary" type="button" id="apply-setup">应用配置 / Apply setup</button>
          <div class="status-line" id="apply-status" role="status" aria-live="polite"></div>
        </div>
      </section>

      <section class="progress-panel" id="progress-panel" aria-labelledby="progress-title" hidden>
        <div class="section-heading">
          <div class="step-index">→</div>
          <div><h2 id="progress-title">正在初始化</h2><p>Initialization in progress</p></div>
        </div>
        <progress id="setup-progress" max="6" value="0" aria-label="初始化进度 / Setup progress">0%</progress>
        <ol class="phase-list" id="phase-list"></ol>
        <div class="countdown" id="restart-countdown" role="status" aria-live="polite"></div>
      </section>

      <section class="completion-panel" id="completion-panel" aria-labelledby="completion-title" hidden>
        <div class="section-heading">
          <div class="step-index success">✓</div>
          <div><h2 id="completion-title">初始化完成</h2><p>Setup complete</p></div>
        </div>
        <p id="completion-message">服务已就绪。 / Service is ready.</p>
      </section>
    </div>
  </div>

  <div class="global-status" id="global-status" role="status" aria-live="polite"></div>
</main>
<script id="setup-model" type="application/json">`

const setupDocumentScriptOpen = `</script>
<script>`

const setupDocumentEnd = `</script>
</body>
</html>`

const setupPageCSS = `
* { box-sizing: border-box; }
html { min-width: 320px; background: var(--bg); color: var(--fg); }
body { margin: 0; min-width: 320px; min-height: 100vh; overflow-x: hidden; background: var(--bg); color: var(--fg); font-family: var(--admin-font-ui); font-size: var(--admin-type-body); line-height: 1.5; }
button, input, select { font: inherit; }
button, input, select { transition: border-color var(--admin-motion-base) ease, background var(--admin-motion-base) ease, color var(--admin-motion-fast) ease, opacity var(--admin-motion-fast) ease; }
button:focus-visible, input:focus-visible, select:focus-visible { outline: 2px solid color-mix(in oklch, var(--accent) 72%, white 28%); outline-offset: 2px; }
[hidden] { display: none !important; }
.page { width: min(1180px, calc(100% - 40px)); margin: 0 auto; padding: 28px 0 56px; }
.topbar { min-height: 72px; display: flex; align-items: center; justify-content: space-between; gap: 24px; border-bottom: 1px solid var(--border); }
.eyebrow { margin: 0 0 3px; color: var(--dim); font-family: var(--admin-font-mono); font-size: var(--admin-type-label); }
h1, h2, p { margin: 0; }
h1 { color: var(--fg); font-size: var(--admin-type-page); font-weight: 650; letter-spacing: 0; }
h1 span { color: var(--dim); font-size: var(--admin-type-section); font-weight: 500; }
h2 { color: var(--fg); font-size: var(--admin-type-section); font-weight: 650; letter-spacing: 0; }
.mode-badge { flex: none; min-width: 104px; height: 30px; display: inline-flex; align-items: center; justify-content: center; border: 1px solid color-mix(in oklch, var(--accent) 35%, var(--border)); border-radius: var(--pg-radius-xs); background: color-mix(in oklch, var(--accent) 8%, var(--surface)); color: var(--accent); font-family: var(--admin-font-mono); font-size: var(--admin-type-label); }
.auth-panel { width: min(680px, 100%); margin: 56px auto 0; border: 1px solid var(--border); border-radius: var(--pg-radius-md); background: var(--surface); padding: 28px; }
.section-heading { display: flex; align-items: flex-start; gap: 14px; margin-bottom: 22px; }
.section-heading p { margin-top: 2px; color: var(--dim); font-size: var(--admin-type-support); }
.step-index { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border: 1px solid var(--border-strong); border-radius: var(--pg-radius-xs); color: var(--accent); font-family: var(--admin-font-mono); font-size: var(--admin-type-support); }
.step-index.success { border-color: color-mix(in oklch, var(--green) 45%, var(--border)); color: var(--green); }
label { display: block; margin-bottom: 7px; color: var(--dim); font-size: var(--admin-type-label); font-weight: 650; letter-spacing: 0; }
label span { color: var(--red); }
input, select { width: 100%; min-width: 0; min-height: 42px; border: 1px solid var(--border); border-radius: var(--pg-radius-sm); background: var(--surface-solid); color: var(--fg); padding: 9px 12px; }
input:focus, select:focus { border-color: color-mix(in oklch, var(--accent) 62%, transparent); box-shadow: 0 0 0 3px color-mix(in oklch, var(--accent) 15%, transparent); }
input[readonly], select:disabled { background: var(--shell); color: var(--muted); cursor: not-allowed; }
.auth-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; }
.button { min-height: 42px; border: 1px solid transparent; border-radius: var(--pg-radius-sm); padding: 9px 16px; cursor: pointer; font-weight: 650; letter-spacing: 0; }
.button:disabled { cursor: not-allowed; opacity: .55; }
.button.primary { background: var(--accent); color: white; }
.button.primary:hover:not(:disabled) { background: color-mix(in oklch, var(--accent) 88%, black 12%); }
.button.secondary { border-color: var(--border-strong); background: var(--surface-solid); color: var(--fg); }
.button.secondary:hover:not(:disabled) { border-color: color-mix(in oklch, var(--accent) 40%, var(--border)); background: color-mix(in oklch, var(--accent) 5%, var(--surface)); }
.token-help { margin-top: 18px; border-top: 1px solid var(--border); padding-top: 16px; }
.token-help > p { color: var(--dim); font-size: var(--admin-type-support); }
.commands { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 9px; }
code { display: inline-flex; min-height: 30px; align-items: center; border: 1px solid var(--border); border-radius: var(--pg-radius-xs); background: var(--canvas); color: var(--accent); padding: 4px 9px; font-family: var(--admin-font-mono); font-size: var(--admin-type-support); overflow-wrap: anywhere; }
.status-line { min-height: 22px; margin-top: 12px; color: var(--muted); font-size: var(--admin-type-support); }
.status-line.error, .probe-status.error, .global-status.error { color: var(--red); }
.status-line.success, .probe-status.success { color: var(--green); }
.workspace { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 24px; margin-top: 28px; align-items: start; }
.step-rail { position: sticky; top: 20px; border-right: 1px solid var(--border); padding-right: 18px; }
.step-rail ol { list-style: none; margin: 0; padding: 0; }
.step-rail li { min-height: 58px; display: flex; align-items: center; gap: 12px; color: var(--dim); }
.step-rail li > span { width: 28px; color: var(--dim); font-family: var(--admin-font-mono); font-size: var(--admin-type-label); }
.step-rail li > div { font-size: var(--admin-type-support); font-weight: 650; }
.step-rail small { display: block; color: var(--dim); font-size: var(--admin-type-label); font-weight: 450; }
.step-rail li.active { color: var(--fg); }
.step-rail li.active > span { color: var(--accent); }
.console-panel { min-width: 0; border: 1px solid var(--border); border-radius: var(--pg-radius-md); background: var(--surface); }
.setup-section, .progress-panel, .completion-panel { padding: 26px 28px; border-bottom: 1px solid var(--border); }
.console-panel > :last-child { border-bottom: 0; }
.deployment-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1px; overflow: hidden; border: 1px solid var(--border); border-radius: var(--pg-radius-sm); background: var(--border); }
.deployment-grid > div { min-width: 0; background: var(--surface-solid); padding: 13px; }
dt { color: var(--dim); font-size: var(--admin-type-label); }
dd { margin: 4px 0 0; color: var(--fg); font-family: var(--admin-font-mono); font-size: var(--admin-type-support); overflow-wrap: anywhere; }
.fields-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; }
.deployment-grid + .fields-grid { margin-top: 20px; }
.field { min-width: 0; }
.field.full { grid-column: 1 / -1; }
.field-help { margin-top: 6px; color: var(--dim); font-size: var(--admin-type-label); line-height: 1.45; overflow-wrap: anywhere; }
.managed-note { color: var(--amber); }
.checkbox-field { min-height: 42px; display: flex; align-items: center; gap: 10px; border: 1px solid var(--border); border-radius: var(--pg-radius-sm); background: var(--surface-solid); padding: 9px 12px; }
.checkbox-field input { width: 16px; min-height: 16px; margin: 0; }
.checkbox-field label { margin: 0; color: var(--fg); }
.probe-row, .apply-row { display: flex; align-items: center; gap: 14px; margin-top: 20px; }
.probe-status { min-height: 21px; color: var(--dim); font-size: var(--admin-type-support); }
.review-list { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.review-list > div { min-width: 0; border-left: 2px solid var(--border-strong); padding: 2px 0 2px 12px; }
progress { width: 100%; height: 8px; overflow: hidden; border: 0; border-radius: var(--pg-radius-xs); background: var(--shell); accent-color: var(--accent); }
progress::-webkit-progress-bar { background: var(--shell); }
progress::-webkit-progress-value { background: var(--accent); }
.phase-list { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px 18px; margin: 18px 0 0; padding: 0; list-style: none; }
.phase-list li { color: var(--dim); font-size: var(--admin-type-support); }
.phase-list li.current { color: var(--accent); font-weight: 650; }
.phase-list li.done { color: var(--green); }
.countdown { min-height: 24px; margin-top: 18px; color: var(--muted); }
.completion-panel > p { color: var(--muted); }
.global-status { position: fixed; right: 18px; bottom: 18px; z-index: 5; width: min(420px, calc(100% - 36px)); min-height: 0; border-radius: var(--pg-radius-sm); background: var(--surface-solid); color: var(--muted); box-shadow: 0 12px 36px color-mix(in oklch, black 16%, transparent); }
.global-status:not(:empty) { border: 1px solid var(--border-strong); padding: 12px 14px; }
@media (max-width: 900px) {
  .workspace { grid-template-columns: 1fr; }
  .step-rail { position: static; overflow-x: auto; border-right: 0; border-bottom: 1px solid var(--border); padding: 0 0 10px; }
  .step-rail ol { display: flex; width: max-content; }
  .step-rail li { width: 150px; min-height: 48px; }
}
@media (max-width: 720px) {
  .page { width: min(100% - 24px, 1180px); padding-top: 14px; }
  .topbar { min-height: 60px; gap: 12px; }
  h1 { font-size: 20px; }
  h1 span { display: block; margin-top: 1px; font-size: var(--admin-type-support); }
  .mode-badge { min-width: 82px; }
  .auth-panel { margin-top: 24px; padding: 20px; }
  .auth-row { grid-template-columns: 1fr; }
  .auth-row .button, .apply-row .button { width: 100%; }
  .setup-section, .progress-panel, .completion-panel { padding: 22px 18px; }
  .deployment-grid, .fields-grid, .review-list { grid-template-columns: 1fr; }
  .field.full { grid-column: auto; }
  .probe-row, .apply-row { align-items: stretch; flex-direction: column; }
  .probe-row .button { width: 100%; }
  .phase-list { grid-template-columns: 1fr; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; animation-duration: .01ms !important; animation-iteration-count: 1 !important; transition-duration: .01ms !important; }
}`

const setupPageScript = `'use strict';
(() => {
  const modelNode = document.getElementById('setup-model');
  const model = JSON.parse(modelNode.textContent);
  modelNode.textContent = '';
  const byId = (id) => document.getElementById(id);
  const authPanel = byId('auth-panel');
  const workspace = byId('workspace');
  const authForm = byId('auth-form');
  const tokenInput = byId('setup-token');
  const authStatus = byId('auth-status');
  const applyStatus = byId('apply-status');
  const globalStatus = byId('global-status');
  const progressPanel = byId('progress-panel');
  const completionPanel = byId('completion-panel');
  const progress = byId('setup-progress');
  const countdown = byId('restart-countdown');
  const applyButton = byId('apply-setup');
  const inputs = new Map();
  const probes = { database: false, redis: false, storage: false };
  const probeVersions = { database: 0, redis: 0, storage: 0 };
  const probePaths = {
    database: '/api/setup/v1/probes/database',
    redis: '/api/setup/v1/probes/redis',
    storage: '/api/setup/v1/probes/storage',
  };
  const hasReturnHistory = history.length > 1 && document.referrer !== '';
  const returnURL = returnURLFromHash();
  let operationId = '';
  let preserveOperationID = false;
  let applying = false;

  const phaseOrder = [
    ['pending', '等待执行 / Pending'],
    ['validating', '验证配置 / Validating'],
    ['initializing_database', '初始化数据库 / Database'],
    ['creating_admin', '创建管理员 / Administrator'],
    ['committing_config', '提交配置 / Commit'],
    ['restart_pending', '等待重启 / Restart'],
  ];

  const errors = {
    SETUP_CREDENTIALS_INVALID: 'Token 无效，请重新查询或重置。 / Invalid token; show or reset it and retry.',
    RATE_LIMITED: '尝试次数过多，请稍后重试。 / Too many attempts; retry later.',
    SETUP_SESSION_INVALID: '会话已失效，请重新验证 Token。 / Session expired; authenticate again.',
    SETUP_VALIDATION_FAILED: '配置校验失败，请检查必填项。 / Configuration validation failed.',
    SETUP_PROBE_FAILED: '中间件检测失败，请分别重新检测。 / Middleware verification failed.',
    SETUP_OPERATION_NOT_FOUND: '无法确认初始化进度，请重新填写敏感字段后使用原配置和操作编号重试。 / Progress is unavailable; re-enter secrets and retry the original operation with the same configuration.',
    SETUP_OPERATION_CONFLICT: '存在冲突的初始化操作，请保持配置不变后重试。 / Conflicting setup operation.',
    SETUP_REQUEST_CANCELLED: '初始化请求已取消，正在确认服务端状态。 / Setup request was cancelled; checking server state.',
    SETUP_REQUEST_TIMEOUT: '初始化请求超时，可使用相同配置重试。 / Setup request timed out; retry unchanged.',
    SETUP_NETWORK_ERROR: 'API 暂时不可访问，正在等待服务恢复。 / API is temporarily unavailable.',
    SETUP_INTERNAL_ERROR: '初始化服务发生内部错误，请运行 deployctl doctor。 / Internal setup error; run deployctl doctor.',
  };

  function setStatus(element, message, kind) {
    element.textContent = message || '';
    element.classList.toggle('error', kind === 'error');
    element.classList.toggle('success', kind === 'success');
  }

  function returnURLFromHash() {
    const marker = '#return_to=';
    if (!location.hash.startsWith(marker)) return '';
    try {
      const target = new URL(decodeURIComponent(location.hash.slice(marker.length)));
      if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password) return '';
      if (document.referrer) {
        const referrer = new URL(document.referrer);
        if (target.origin !== referrer.origin) return '';
      }
      return target.href;
    } catch (_) {
      return '';
    }
  }

  async function requestJSON(path, options = {}) {
    const timeout = options.timeout || 15000;
    const controller = new AbortController();
    const timeoutID = window.setTimeout(() => controller.abort(), timeout);
    try {
      const response = await fetch(path, {
        method: options.method || 'GET',
        credentials: 'same-origin',
        cache: 'no-store',
        headers: options.body ? { 'Accept': 'application/json', 'Content-Type': 'application/json' } : { 'Accept': 'application/json' },
        body: options.body ? JSON.stringify(options.body) : undefined,
        signal: controller.signal,
      });
      if (response.status === 204) return null;
      let payload = {};
      try {
        payload = await response.json();
      } catch (error) {
        if (controller.signal.aborted) throw error;
      }
      if (!response.ok) throw { code: payload.error?.code || 'SETUP_INTERNAL_ERROR' };
      return payload.data;
    } catch (error) {
      if (controller.signal.aborted) throw { code: 'SETUP_REQUEST_TIMEOUT' };
      if (typeof error?.code === 'string') throw error;
      throw { code: 'SETUP_NETWORK_ERROR' };
    } finally {
      window.clearTimeout(timeoutID);
    }
  }

  function fieldContainer(field) {
    if (field.group === 'database') return byId('database-fields');
    if (field.group === 'redis') return byId('redis-fields');
    if (field.group === 'storage') return byId('storage-fields');
    if (field.group === 'public endpoints') return byId('public-fields');
    return null;
  }

  function renderField(field) {
    const container = fieldContainer(field);
    if (!container) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'field';
    wrapper.dataset.field = field.key;
    const inputId = 'field-' + field.key.toLowerCase().replaceAll('_', '-');
    const label = document.createElement('label');
    label.htmlFor = inputId;
    label.textContent = field.key + (field.required ? ' *' : '');
    wrapper.append(label);

    let input;
    if (field.input === 'select') {
      input = document.createElement('select');
      for (const option of field.options || []) {
        const node = document.createElement('option');
        node.value = option.value;
        node.textContent = option.label;
        input.append(node);
      }
    } else {
      input = document.createElement('input');
      input.type = field.input === 'checkbox' ? 'checkbox' : field.input;
    }
    input.id = inputId;
    input.name = field.key;
    input.dataset.runtimeField = field.key;
    input.required = Boolean(field.required && !field.managed);
    if (field.input === 'checkbox') {
      input.checked = field.value === 'true';
    } else if (!field.secret) {
      input.value = field.value || '';
    }
    if (field.secret) {
      input.autocomplete = 'new-password';
      input.spellcheck = false;
    }
    if (field.read_only) {
      if (input.tagName === 'SELECT' || input.type === 'checkbox') input.disabled = true;
      else input.readOnly = true;
      input.setAttribute('aria-readonly', 'true');
      if (field.secret) input.placeholder = '由 deployctl 管理 / Managed by deployctl';
    } else if (field.example && field.input !== 'checkbox') {
      input.placeholder = field.example;
    }
    if (field.input === 'checkbox') {
      const row = document.createElement('div');
      row.className = 'checkbox-field';
      row.append(input);
      wrapper.append(row);
    } else {
      wrapper.append(input);
    }
    const help = document.createElement('p');
    help.className = 'field-help' + (field.managed ? ' managed-note' : '');
    help.textContent = field.description_zh + ' / ' + field.description_en + (field.managed ? ' 由部署工具管理。 / Managed by deployctl.' : '');
    wrapper.append(help);
    inputs.set(field.key, input);
    container.append(wrapper);
  }

  function renderDeployment() {
    const items = [
      ['Mode', model.deployment.mode], ['Profile', model.deployment.profile],
      ['Topology', model.deployment.topology], ['Role', model.deployment.role],
    ];
    const summary = byId('deployment-summary');
    for (const [label, value] of items) {
      const wrapper = document.createElement('div');
      const term = document.createElement('dt');
      const detail = document.createElement('dd');
      term.textContent = label;
      detail.textContent = value;
      wrapper.append(term, detail);
      summary.append(wrapper);
    }
    byId('deployment-badge').textContent = (model.deployment.profile + ' · ' + model.deployment.role).toUpperCase();
  }

  function value(key) {
    const input = inputs.get(key);
    if (!input) return '';
    if (input.type === 'checkbox') return input.checked ? 'true' : 'false';
    return input.value.trim();
  }

  function updateStorageVisibility() {
    const driver = value('STORAGE_DRIVER') || 'local';
    for (const field of model.fields.filter((item) => item.group === 'storage')) {
      const wrapper = document.querySelector('[data-field="' + field.key + '"]');
      if (!wrapper) continue;
      const local = field.key.startsWith('STORAGE_LOCAL_') || field.key === 'STORAGE_SHARED_VOLUME';
      const s3 = field.key.startsWith('STORAGE_S3_');
      wrapper.hidden = (driver === 's3' && local) || (driver === 'local' && s3);
      const input = inputs.get(field.key);
      if (input && !field.managed) input.required = Boolean(field.required || (driver === 'local' && local) || (driver === 's3' && s3 && !['STORAGE_S3_PREFIX'].includes(field.key)));
    }
  }

  function runtimePayload() {
    const runtime = {};
    for (const field of model.fields) {
      const current = value(field.key);
      if (field.managed && field.secret && current === '') continue;
      runtime[field.key] = current;
    }
    return runtime;
  }

  function validateConfiguration() {
    updateStorageVisibility();
    for (const field of model.fields) {
      const input = inputs.get(field.key);
      const wrapper = document.querySelector('[data-field="' + field.key + '"]');
      if (!input || wrapper?.hidden || field.managed) continue;
      if (input.required && value(field.key) === '') {
        input.focus();
        return false;
      }
    }
    const email = byId('admin-email');
    const password = byId('admin-password');
    if (!email.reportValidity() || !password.reportValidity()) return false;
    const bytes = new TextEncoder().encode(password.value).byteLength;
    if (bytes < 6 || bytes > 72) {
      password.setCustomValidity('Password must be 6–72 bytes');
      password.reportValidity();
      password.setCustomValidity('');
      return false;
    }
    return true;
  }

  function clearSecretInputs() {
    tokenInput.value = '';
    byId('admin-password').value = '';
    for (const field of model.fields) if (field.secret && inputs.has(field.key)) inputs.get(field.key).value = '';
  }

  function clearApplyPayload(body) {
    body.admin_password = '';
    for (const key of Object.keys(body.runtime)) {
      if (model.fields.find((field) => field.key === key)?.secret) body.runtime[key] = '';
    }
    clearSecretInputs();
  }

  function probeBody(kind) {
    if (kind === 'database') return { database_url: value('DATABASE_URL') };
    if (kind === 'redis') return { redis_url: value('REDIS_URL'), key_prefix: value('REDIS_KEY_PREFIX') };
    return {
      driver: value('STORAGE_DRIVER'), local_root: value('STORAGE_LOCAL_ROOT'), public_base_url: value('STORAGE_PUBLIC_BASE_URL'),
      shared_volume: value('STORAGE_SHARED_VOLUME') === 'true', endpoint: value('STORAGE_S3_ENDPOINT'),
      region: value('STORAGE_S3_REGION'), bucket: value('STORAGE_S3_BUCKET'), access_key_id: value('STORAGE_S3_ACCESS_KEY_ID'),
      secret_access_key: value('STORAGE_S3_SECRET_ACCESS_KEY'), force_path_style: value('STORAGE_S3_FORCE_PATH_STYLE') === 'true',
      prefix: value('STORAGE_S3_PREFIX'),
    };
  }

  async function runProbe(kind, button) {
    const status = byId(kind + '-probe-status');
    const version = probeVersions[kind];
    button.disabled = true;
    setStatus(status, '检测中… / Testing…');
    try {
      const result = await requestJSON(probePaths[kind], { method: 'POST', body: probeBody(kind) });
      if (version !== probeVersions[kind]) return;
      probes[kind] = Boolean(result?.success);
      setStatus(status, probes[kind] ? '连接正常 / Connected' : '检测失败：' + (result?.code || 'PROBE_FAILED'), probes[kind] ? 'success' : 'error');
    } catch (error) {
      if (version !== probeVersions[kind]) return;
      probes[kind] = false;
      if (error.code === 'SETUP_SESSION_INVALID') return resetAuthentication(error.code);
      setStatus(status, errors[error.code] || errors.SETUP_INTERNAL_ERROR, 'error');
    } finally {
      button.disabled = false;
      updateReview();
    }
  }

  function invalidateProbe(kind) {
    probeVersions[kind] += 1;
    probes[kind] = false;
    setStatus(byId(kind + '-probe-status'), '尚未检测 / Not tested');
  }

  function updateReview() {
    const summary = byId('review-summary');
    summary.replaceChildren();
    const items = [
      ['Profile', model.deployment.profile + ' / ' + model.deployment.role],
      ['Storage', value('STORAGE_DRIVER') || '—'],
      ['Administrator', byId('admin-email').value.trim() || '—'],
      ['Database probe', probes.database ? 'PASS' : 'PENDING'],
      ['Redis probe', probes.redis ? 'PASS' : 'PENDING'],
      ['Storage probe', probes.storage ? 'PASS' : 'PENDING'],
    ];
    for (const [label, current] of items) {
      const wrapper = document.createElement('div');
      const term = document.createElement('dt');
      const detail = document.createElement('dd');
      term.textContent = label;
      detail.textContent = current;
      wrapper.append(term, detail);
      summary.append(wrapper);
    }
  }

  function resetAuthentication(code) {
    clearSecretInputs();
    workspace.hidden = true;
    authPanel.hidden = false;
    tokenInput.focus();
    setStatus(authStatus, errors[code] || errors.SETUP_SESSION_INVALID, 'error');
  }

  function focusSetupWorkspace() {
    const first = Array.from(inputs.values()).find((input) => !input.readOnly && !input.disabled && input.offsetParent !== null);
    if (first) first.focus();
    else workspace.focus();
  }

  authForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const token = tokenInput.value;
    if (!token) {
      setStatus(authStatus, '请输入 Token。 / Setup token is required.', 'error');
      tokenInput.focus();
      return;
    }
    byId('authenticate').disabled = true;
    setStatus(authStatus, '正在验证… / Authenticating…');
    try {
	      const session = await requestJSON('/api/setup/v1/session', { method: 'POST', body: { token } });
	      operationId = session?.operation_id || operationId;
	      preserveOperationID = Boolean(operationId);
	      authPanel.hidden = true;
	      workspace.hidden = false;
	      setStatus(authStatus, '');
	      if (operationId) {
	        setStatus(applyStatus, '检测到未完成的初始化操作。请重新填写相同配置和敏感字段，完成检测后继续。 / An unfinished setup operation was found. Re-enter the same configuration and secrets, run the probes, then continue.', 'error');
	      }
	      focusSetupWorkspace();
    } catch (error) {
      setStatus(authStatus, errors[error.code] || errors.SETUP_INTERNAL_ERROR, 'error');
    } finally {
      tokenInput.value = '';
      byId('authenticate').disabled = false;
    }
  });

  for (const button of document.querySelectorAll('[data-probe]')) {
    button.addEventListener('click', () => runProbe(button.dataset.probe, button));
  }

  function renderPhase(view) {
    progressPanel.hidden = false;
    const index = Math.max(0, phaseOrder.findIndex(([phase]) => phase === view.phase));
    progress.value = index + 1;
    const list = byId('phase-list');
    list.replaceChildren();
    phaseOrder.forEach(([phase, label], phaseIndex) => {
      const item = document.createElement('li');
      item.textContent = label;
      item.className = phaseIndex < index ? 'done' : phaseIndex === index ? 'current' : '';
      list.append(item);
    });
  }

  async function recoverApplyOperation() {
    const recoveryDeadline = Date.now() + 300000;
    renderPhase({ phase: 'pending' });
    while (operationId && Date.now() < recoveryDeadline) {
      const remaining = recoveryDeadline - Date.now();
      let bootstrap;
      try {
        bootstrap = await requestJSON('/api/system/v1/bootstrap-status', { timeout: Math.min(5000, remaining) });
      } catch (_) {}
      if (bootstrap?.phase === 'ready') return finish();
      if (bootstrap?.phase === 'broken') {
        showRestartRecovery('服务在初始化恢复期间进入故障状态。 / Service entered broken mode during setup recovery.');
        return;
      }
      let view;
      try {
        view = await requestJSON('/api/setup/v1/progress/' + encodeURIComponent(operationId), { timeout: Math.min(5000, remaining) });
      } catch (error) {
        if (bootstrap?.phase === 'setup_required' && error.code === 'SETUP_OPERATION_NOT_FOUND') {
          applying = false;
          applyButton.disabled = false;
          preserveOperationID = true;
          progressPanel.hidden = true;
          setStatus(applyStatus, errors.SETUP_OPERATION_NOT_FOUND, 'error');
          return;
        }
        if (bootstrap?.phase === 'setup_required' && error.code === 'SETUP_SESSION_INVALID') {
          applying = false;
          applyButton.disabled = false;
          preserveOperationID = true;
          return resetAuthentication(error.code);
        }
        if (!['SETUP_OPERATION_NOT_FOUND', 'SETUP_SESSION_INVALID', 'SETUP_NETWORK_ERROR', 'SETUP_REQUEST_TIMEOUT', 'SETUP_REQUEST_CANCELLED'].includes(error.code)) {
          applying = false;
          applyButton.disabled = false;
          setStatus(applyStatus, errors[error.code] || errors.SETUP_INTERNAL_ERROR, 'error');
          return;
        }
        const wait = Math.min(2000, Math.max(0, recoveryDeadline - Date.now()));
        if (wait > 0) await delay(wait);
        continue;
      }
      renderPhase(view);
      if (view.error_code) {
        applying = false;
        applyButton.disabled = false;
        setStatus(applyStatus, errors[view.error_code] || errors.SETUP_INTERNAL_ERROR, 'error');
        return;
      }
      if (view.phase === 'restart_pending' || view.phase === 'complete') return waitForRestart();
      const wait = Math.min(1500, Math.max(0, recoveryDeadline - Date.now()));
      if (wait > 0) await delay(wait);
    }
    showRestartRecovery('无法确认初始化操作的最终状态。 / Could not confirm the final setup operation state.');
  }

  async function waitForRestart() {
    renderPhase({ phase: 'restart_pending' });
    let remaining = 10;
    while (remaining > 0) {
      countdown.textContent = '服务重启倒计时 ' + remaining + 's / Restart countdown ' + remaining + 's';
      await delay(1000);
      remaining -= 1;
    }
    countdown.textContent = '正在等待服务就绪… / Waiting for readiness…';
    await pollReadiness();
  }

  async function pollReadiness() {
    const readinessDeadline = Date.now() + 120000;
    for (let attempt = 0; attempt < 60; attempt += 1) {
      const remaining = readinessDeadline - Date.now();
      if (remaining <= 0) break;
      try {
        const status = await requestJSON('/api/system/v1/bootstrap-status', { timeout: Math.min(2000, remaining) });
        if (status?.phase === 'ready') return finish();
        if (status?.phase === 'broken') {
          showRestartRecovery('服务重启后处于故障状态。 / Service restarted in broken mode.');
          return;
        }
      } catch (_) {}
      const wait = Math.min(2000, Math.max(0, readinessDeadline - Date.now()));
      if (wait > 0) await delay(wait);
    }
    showRestartRecovery('服务未在预期时间内恢复。 / Service did not become ready before the timeout.');
  }

  function showRestartRecovery(reason) {
    const commands = 'deployctl status · deployctl logs · deployctl doctor · deployctl restart';
    countdown.textContent = reason + ' ' + commands;
    setStatus(globalStatus, reason + ' ' + commands, 'error');
  }

  function finish() {
    progressPanel.hidden = true;
    completionPanel.hidden = false;
    setStatus(globalStatus, '');
    if (returnURL) {
      byId('completion-message').textContent = '服务已就绪，正在返回原页面… / Ready; returning to the original page…';
      window.setTimeout(() => location.assign(returnURL), 900);
    } else if (hasReturnHistory) {
      byId('completion-message').textContent = '服务已就绪，正在返回原页面… / Ready; returning to the previous page…';
      window.setTimeout(() => history.back(), 900);
    } else {
      byId('completion-message').textContent = '服务已就绪。请返回最初访问的用户端或管理后台地址。 / Ready. Return to the original user or admin entry point.';
    }
  }

  function createOperationID() {
    if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    bytes[6] = (bytes[6] & 15) | 64;
    bytes[8] = (bytes[8] & 63) | 128;
    const encoded = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
    return encoded.slice(0, 8) + '-' + encoded.slice(8, 12) + '-' + encoded.slice(12, 16) + '-' + encoded.slice(16, 20) + '-' + encoded.slice(20);
  }

  applyButton.addEventListener('click', async () => {
    if (applying || !validateConfiguration()) {
      if (!applying) setStatus(applyStatus, '请检查必填项。 / Check required fields.', 'error');
      return;
    }
    if (!probes.database || !probes.redis || !probes.storage) {
      setStatus(applyStatus, '请先完成三项独立检测。 / Complete all three probes first.', 'error');
      return;
    }
    applying = true;
    applyButton.disabled = true;
    operationId = operationId || createOperationID();
    preserveOperationID = true;
    const body = {
      operation_id: operationId,
      runtime: runtimePayload(),
      admin_email: byId('admin-email').value.trim(),
      admin_password: byId('admin-password').value,
    };
    setStatus(applyStatus, '正在提交初始化操作… / Applying setup…');
    try {
      const pendingApply = requestJSON('/api/setup/v1/apply', { method: 'POST', body, timeout: 300000 });
      clearApplyPayload(body);
      const view = await pendingApply;
      renderPhase(view);
      setStatus(applyStatus, '初始化操作已接受。 / Setup operation accepted.', 'success');
      if (view.phase === 'restart_pending' || view.phase === 'complete') await waitForRestart();
      else await recoverApplyOperation();
    } catch (error) {
      if (error.code === 'SETUP_SESSION_INVALID') {
        applying = false;
        applyButton.disabled = false;
        preserveOperationID = true;
        resetAuthentication(error.code);
      }
      else if (error.code === 'SETUP_NETWORK_ERROR' || error.code === 'SETUP_REQUEST_TIMEOUT' || error.code === 'SETUP_REQUEST_CANCELLED') {
        setStatus(applyStatus, errors[error.code] || errors.SETUP_NETWORK_ERROR, 'error');
        await recoverApplyOperation();
      } else {
        applying = false;
        applyButton.disabled = false;
        setStatus(applyStatus, errors[error.code] || errors.SETUP_INTERNAL_ERROR, 'error');
      }
    } finally {
      clearApplyPayload(body);
    }
  });

  function delay(milliseconds) { return new Promise((resolve) => window.setTimeout(resolve, milliseconds)); }

  renderDeployment();
  for (const field of model.fields) renderField(field);
  updateStorageVisibility();
  updateReview();
  for (const input of document.querySelectorAll('input, select')) {
    input.addEventListener('input', () => {
      if (!applying && !preserveOperationID) operationId = '';
      if (input.name === 'STORAGE_DRIVER') updateStorageVisibility();
      const field = model.fields.find((item) => item.key === input.name);
      if (field && ['database', 'redis', 'storage'].includes(field.group)) invalidateProbe(field.group);
      updateReview();
    });
  }
})();`
