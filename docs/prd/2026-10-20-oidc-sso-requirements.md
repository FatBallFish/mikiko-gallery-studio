# 需求文档：接入 Mikiko AI OIDC 统一认证（Mikiko ID）

- 日期：2026-10-20
- 状态：已确认（用户任务指令 + 对接文档 `PROMPT-PLATFORM-OIDC.md`，方案已在会话中批准）
- 关联技术设计：`docs/plans/2026-10-20-oidc-sso-tech-design.md`
- 外部规范：Mikiko 平台 OIDC 通用对接文档（Mikiko ID，Issuer `https://oauth.mikiko.ai/realms/mikiko`，本地联调 `http://localhost:8080/realms/mikiko`）

## 背景

Mikiko 系平台正在统一身份体系（Mikiko ID / Keycloak realm `mikiko`）。本站（用户端）
当前仅有本地登录：邮箱验证码首登自动注册 + 密码登录。需要按平台统一规范接入
OIDC（Authorization Code + PKCE），与 Mikiko AI 平台打通，并允许运营在管理端
控制各登录方式的开放状态。

## 需求

- R1 系统设置增加 OIDC 配置：管理端「系统设置 → 安全配置」新增 Mikiko ID（OIDC）
  配置区，含启用开关、Issuer、client_id、client_secret（加密存储、只写）、
  按钮显示名、连通性测试；配置完成并启用后，用户端登录页展示 OIDC 登录方式。
- R2 OIDC 登录：登录页提供「使用 Mikiko ID 登录」按钮；完整走授权码 + PKCE(S256)
  流程，client_secret 只存后端，OIDC token 不下发浏览器；登录成功后签发本站
  现有双 token 会话。
- R3 静默免登：入口 URL 带 `sso=silent` 且本站无会话时立即发起 OIDC 授权
  （策略 A 顺水推舟，不传 prompt）；处理完成后从地址栏摘除该参数；目标路径
  （含本站自身 query/锚点）在登录后完整还原。
- R4 账号关联：以 OIDC `sub` 为准（`user_identities` 表），首登自动注册；
  同 email 且 `email_verified=true` 时自动关联既有本地账号；email 未验证时
  拒绝合并，避免未验证邮箱劫持。
- R5 登录方式开关：
  - 「允许平台账密登录」：关闭后密码登录、邮箱验证码登录、发送验证码接口
    全部拒绝（403），登录页隐藏本地登录 UI，仅保留 Mikiko ID；
  - 「允许账密注册」：关闭后邮箱验证码首登不再自动建号（403）；
  - OIDC 首登自动注册始终允许（对接文档默认行为）；
  - 安全护栏：OIDC 未启用时不允许关闭本地登录（防止全部登录方式被关）。
- R6 默认零影响：OIDC 默认关闭、开关默认全开，存量部署升级后行为不变，
  无需数据迁移。
- R7 范围：仅用户端（web/user）登录；管理后台（web/admin）保持独立账密登录，
  本次不接入（后续可单独扩展）。

## 验收标准

- AC1 `./scripts/workflow/verify.sh`、review gate、api-smoke 全部通过。
- AC2 本地 Keycloak 全链路：配置 OIDC → 登录页出现 Mikiko ID 按钮 → 登录 →
  回到原目标页；`?sso=silent` 静默免登；深链 query/锚点还原；登出后再静默成功。
- AC3 同 email（已验证）首登自动关联既有账号，二次登录走 identity 命中；
  未验证 email 的同邮箱登录被拒绝且有明确错误码。
- AC4 关闭账密登录/注册开关后：对应 UI 隐藏、后端接口 403；重开恢复正常。
- AC5 OIDC 未配置/未启用时：features 接口与登录页表现与现状一致，
  既有登录路径回归通过。
- AC6 client_secret 在 DB 中加密存储（secure_configs），管理界面只写不回显，
  掩码占位符提交被拒绝。
