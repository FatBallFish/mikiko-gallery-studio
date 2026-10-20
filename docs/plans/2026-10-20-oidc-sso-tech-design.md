# 技术设计：Mikiko AI OIDC 统一认证接入

- 日期：2026-10-20
- 关联需求：`docs/prd/2026-10-20-oidc-sso-requirements.md`
- 外部规范：`PROMPT-PLATFORM-OIDC.md`（Mikiko 平台通用 OIDC 对接文档）

## 1. 总体架构

- **Authorization Code + PKCE(S256)，机密客户端**。client_secret 只存后端
  （secureconfig AES-GCM 加密），OIDC token 不下发浏览器。
- **会话策略**：OIDC 仅作为登录时刻的身份联合。换码成功后复用
  `auth.Service.issueSessionLocked()` 签发本站现有双 token（10 分钟 JWT +
  轮换刷新 Cookie），`requireUser`/TokenVersion/refresh 轮换全部沿用。
  不存储 OIDC refresh_token / id_token；登出仅清本站会话（对接文档 §5 默认）。
- **回调走 SPA 路径**：redirect_uri = `https://<平台域>/auth/callback`
  （生产 nginx `try_files … /index.html` 回退；vite dev SPA fallback）。
  Keycloak 302 回 SPA，SPA 读 `code/state` 后 POST 给后端换会话。

## 2. 流程

### 2.1 发起（`GET /api/agent/auth/v1/oidc/authorize?returnTo=…`）

- 后端生成 state / nonce / PKCE verifier（crypto/rand，base64url）；
- 自包含事务 Cookie `pg_oidc_tx`：`base64url(payload).base64url(HMAC-SHA256)`，
  payload = `{state, verifier, nonce, return_to, exp}`，密钥从
  `AccessTokenSecret` 派生（HKDF，独立于 JWT 签名用法）；HttpOnly、
  SameSite=Lax、Path=/api/agent/auth/v1、MaxAge 10 分钟；
- 302 到授权端点（issuer 经 discovery 缓存解析，TTL + 失败重取），
  scope=`openid profile email`，`code_challenge_method=S256`。

### 2.2 回调（`POST /api/agent/auth/v1/oidc/callback {code, state}`）

1. 校验 Cookie（签名 + exp）且 `cookie.state == body.state`，立即清 Cookie（一次性）；
2. 换 token（client_id + secret + verifier + redirect_uri）；
3. 验 id_token：JWKS（go-oidc 远程 keyset）、iss / aud=client_id / exp / **nonce**；
4. 身份映射（§3）→ 签发会话，返回与密码登录同构的
   `{access_token, expires_in_seconds, user_id}`。

### 2.3 身份映射（`auth.Service.LoginWithOIDC`）

- `(provider="mikiko", sub)` 命中 `user_identities` → 直接登录（更新 email 快照/最后登录）；
- 未命中、email 匹配既有用户且 id_token `email_verified=true` → 创建 identity 关联后登录；
- email 匹配但 `email_verified=false` → 403 `OIDC_EMAIL_NOT_VERIFIED`（拒绝合并）；
- 全未命中 → 自动建号（复用 `createUserLocked` 语义：active / basic / 无密码 /
  EmailVerifiedAt，首登 signup trial 同邮箱验证码注册）+ 创建 identity；
- 无 email claim → 403（本站以 email 为唯一账号键）。

### 2.4 静默免登（sso=silent，策略 A）

`web/user` App 引导：`location.search` 含 `sso=silent` → `replaceState` 摘除该参数
得到干净目标 URL → 无会话则整页跳 authorize（returnTo=干净 URL）；有会话直接放行。
登录页按钮为普通授权发起（不带 sso、不带 prompt），与静默复用同一 client/回调/关联。

## 3. 数据模型

新 ent schema `UserIdentity`（表 `user_identities`）：

| 列 | 说明 |
|---|---|
| id | PK |
| user_id | FK → users（级联删除） |
| provider | `mikiko` |
| subject | OIDC `sub`（Mikiko 全局唯一 ID） |
| email | 登录时快照（展示/审计，不参与唯一性） |
| created_at / updated_at | 时间戳 |

unique(provider, subject)；迁移走 ent `Schema.Create`（db-migrate 现有机制）。
AuthStore 接口新增 `FindUserIdentity(provider, subject)`、
`CreateUserIdentity(tx)`、`AttachUserIdentity(tx)`（建号/关联与建 identity 同事务）。

## 4. 配置存储（secureconfig）

- category `oidc` / key `default`，复用 SMTP 同构机制（版本 409、掩码拒绝、指纹）：
  - public：`enabled`、`issuer_url`、`client_id`、`display_name`、
    `allow_local_login`、`allow_registration`
  - secret：`client_secret`
- `ResolveOIDCConfig(ctx)`：enabled 且字段齐全时返回可用配置（含解密 secret），
  供 authorize/callback/features 使用。
- 校验：enabled 时 issuer/client_id/secret 必填；issuer 须 https
  （放行 `http://localhost` / `http://127.0.0.1`）；`enabled=false` 时
  `allow_local_login` 必须 true（防锁死）。

## 5. API

| 端点 | 说明 | 鉴权 |
|---|---|---|
| `GET /api/agent/auth/v1/oidc/authorize?returnTo=` | 302 到 IdP；未启用 → 404 `OIDC_NOT_ENABLED` | 无 |
| `POST /api/agent/auth/v1/oidc/callback` | `{code,state}` → 会话；state 失配/过期 → 400 | 无（事务 Cookie） |
| `GET /api/agent/features/v1` | 扩展：`oidc_login:{enabled,display_name}`、`password_login_enabled`、`registration_enabled` | 无 |
| `GET/PUT /api/ops/admin/v1/security/oidc` | 查看/保存 OIDC 配置 | manage:dangerous_config |
| `POST /api/ops/admin/v1/security/oidc/test` | 拉取 discovery 验连通 | manage:dangerous_config |

登录开关 gate（在 handler 层读 ResolveOIDCConfig）：
- `allow_local_login=false` → `login/password`、`login/email-code`、`email/send-code` → 403 `LOCAL_LOGIN_DISABLED`；
- `allow_registration=false` → 邮箱验证码首登建号 → 403 `REGISTRATION_DISABLED`。

路由注册三处同步：`router.go` mux、`api/openapi/openapi.yaml`、（如契约缺失）
preflight 补充表；契约测试保持绿。

## 6. 依赖

新增 `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`（discovery/JWKS/
id_token 验证与密钥轮换）；provider 封装 `internal/service/auth/oidc_client.go`
注入 `*http.Client`，测试用 httptest 伪 IdP（discovery + token + JWKS + RS256）。

## 7. 前端

### web/user

- `App.tsx` 引导：`/auth/callback` 路径检测（读 `location.search` 的 code/state/
  error）→ `userApi.oidcCallback` → `app.login()` → 跳 `return_to`（响应带回）；
  `sso=silent` 处理如 §2.4。
- `LoginPage.tsx` + `loginPresentation.ts`：按 features 开关渲染 tabs/注册 footer；
  Mikiko ID 按钮替换现有禁用的第三方登录占位块（`window.location.href = authorize`）。
- 本地登录全关时表单区仅剩 OIDC 按钮。

### web/admin

- `SecurityConfigPage.tsx` 新增「Mikiko ID 统一认证（OIDC）」卡片：启用开关、
  Issuer、Client ID、Client Secret（只写 + 指纹状态）、显示名、两个登录开关、
  回调地址展示（`<origin>/auth/callback`）、连通性测试。SMTP 编辑器为模板。

### web/shared

- `api-types.ts`：`OIDCConfigView/WriteRequest`、`FeatureFlags` 扩展、
  `API_PATHS` 新路径；`user-api.ts` / `admin-api.ts` 增方法。

## 8. 安全清单（对应对接文档 §7）

- PKCE(S256) 必须；secret 只在后端、DB 加密存储；
- state 防 CSRF（一次性事务 Cookie）、nonce 防 id_token 重放，回调必校验；
- redirect_uri 精确匹配（常量 `<origin>/auth/callback`，来自请求 Origin/配置）；
- OIDC token 不进前端；会话沿用现有 HttpOnly SameSite=Lax 刷新 Cookie；
- 账号关联用 sub / 已验证 email。

## 9. 测试与联调

- 单测：oidc client（伪 IdP：PKCE/nonce/state）、`LoginWithOIDC`（命中/关联/建号/
  未验证拒绝）、secureconfig OIDC 校验、features 扩展、开关 403；
  路由级测试仿 `auth_api_test.go`。
- 本地 E2E：`OpsProjects/KeyCloak/docker/local` 起 keycloak-local（占 8080，
  本站 API 用 `APP_ADDR=:8081`、vite proxy 指向 8081）；master realm Admin REST
  建 confidential client `mikiko_studio`（redirect `http://localhost:5173/auth/callback`）
  + 专用测试用户；验收 PRD AC2–AC5。
- 本地不可用则线上 `oauth.mikiko.ai` 联调（创建 `mikiko_studio` client 与测试账号，
  不动真实用户）。
- 交付门禁：verify.sh、review-local.sh --scope committed、check-review-gate.sh、api-smoke.sh。

## 10. 回滚

OIDC 默认关闭；出问题时管理端关闭 OIDC 开关即回到现状（本地登录开关默认全开）。
代码回滚无数据迁移负担（`user_identities` 为纯新增表）。
