# Post-release Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 按已批准的“契约化纵向修复”方案解决安装恢复、支付配置、Stripe 完整链路、后台管理、用户积分体验、注册昵称和文本模型默认配置共 15 项发布后问题。

**Architecture:** 以四条纵向契约推进：安装状态恢复、收银台渠道、管理端配置、用户工作流。每条契约先固定服务/API/UI 边界，再用测试驱动实现；安装身份、加密密钥、已有用户、已完成安装和历史支付配置保持不变。

**Tech Stack:** Go 1.26、Ent、PostgreSQL/SQLite、React 19、TypeScript、Vite、Docker Compose、stripe-go/v85、Stripe.js Payment Element。

---

## 执行约束

- 开始业务代码前必须使用 `dev-start-coding`；Go 文件修改前使用 `dev-go-patterns`，React/TypeScript 修改前使用 `dev-react-patterns`。
- 需求来源：`docs/prd/2026-08-02-post-release-remediation-requirements.md`。
- 技术设计来源：`docs/plans/2026-08-02-post-release-remediation-design.md`。
- 每个任务严格执行 RED -> GREEN -> focused verification -> commit，不跨任务积累未提交改动。
- 禁止在真实 `runtime/` 上验证安装覆盖；安装测试只使用 `t.TempDir()` 或 `mktemp -d`。
- Stripe 测试仅使用注入式 fake backend / `httptest.Server`，不得访问 Stripe 公网。
- 任何响应、日志、审计与前端 DOM 测试快照都不得包含密钥明文。

### Task 1: 建立编码上下文与基线

**Files:**
- Create (generated): `.coding-context.json`
- Read: `docs/prd/2026-08-02-post-release-remediation-requirements.md`
- Read: `docs/plans/2026-08-02-post-release-remediation-design.md`

**Step 1: 启动仓库工作流**

Run:

```bash
./scripts/workflow/start-coding.sh --task "post-release installer cashier Stripe admin and user remediation" --track heavyweight
```

Expected: exit `0`，`.coding-context.json` 同时引用上述 requirement 和 design；若 exit `2`，停止编码并修正文档发现问题。

**Step 2: 载入实现守则并确认干净基线**

执行 `dev-go-patterns`、`dev-react-patterns`，然后运行：

```bash
git status --short
./scripts/workflow/verify.sh
```

Expected: 除工作流生成的上下文文件外没有未知改动；基线验证 PASS。

**Step 3: 提交编码上下文（仅当仓库跟踪该文件）**

```bash
git add -f .coding-context.json
git commit -m "chore: initialize remediation coding context"
```

Expected: 若 `.coding-context.json` 按仓库规则被忽略，则不提交，继续下一任务。

### Task 2: 用身份优先规则识别未完成安装

**Files:**
- Modify: `internal/mgsctl/install.go`
- Modify: `internal/mgsctl/install_test.go`
- Modify: `internal/mgsctl/cli_test.go`

**Step 1: 写失败测试**

在 `internal/mgsctl/install_test.go` 增加表驱动用例：

```go
func TestLoadPendingInstallRecognizesRebuildableGeneratedState(t *testing.T) {
    // runtime.env、install-state 与 manifest 的 INSTALLATION_ID 一致，setup 未完成。
    // compose.yml 缺失或 hash 陈旧仍应返回 pending snapshot。
}

func TestPendingInstallRejectsMismatchedIdentityAndSymlink(t *testing.T) {
    // INSTALLATION_ID 不一致或 plan-owned 路径为 symlink 时必须 unrecognized。
}

func TestExecuteInstallResumesEquivalentPendingPlanWithoutNewEntropy(t *testing.T) {
    // 等价 plan 复用原 setup token / installation ID，并直接重试 ApplyDeployment。
}
```

在 `internal/mgsctl/cli_test.go` 增加交互覆盖断言：不同 plan 且未确认时返回 confirmation-required，不改变文件。

**Step 2: 确认测试先失败**

```bash
go test ./internal/mgsctl -run 'TestLoadPendingInstall|TestPendingInstallRejects|TestExecuteInstallResumes|TestExecuteInstall.*Overwrite' -count=1
```

Expected: FAIL，原因是现有 `loadExistingInstall` 要求完整生成文件或仍按 tuple/严格 hash 判断。

**Step 3: 实现 typed pending snapshot**

在 `internal/mgsctl/install.go` 引入：

```go
type pendingInstallSnapshot struct {
    Result         InstallResult
    Plan           InstallPlan
    State          setup.InstallState
    Manifest       DeploymentManifest
    Runtime        RuntimeDocument
    OwnedFiles     []string
    GeneratedStale bool
}
```

将 `loadExistingInstall` 改为返回 snapshot 与 typed state。识别顺序为：schema 受支持 -> 三处身份一致 -> `SETUP_COMPLETED != true` -> plan 可验证 -> owned path 无 symlink/path traversal。生成文件缺失或 hash 陈旧只标记 `GeneratedStale`，不降级为 unknown。

将 plan 比较封装为语义比较函数，忽略交互字段，仅比较 mode/profile/topology/role/components/ports/storage/image release。

**Step 4: 运行 focused tests**

```bash
go test ./internal/mgsctl -run 'TestLoadPendingInstall|TestPendingInstallRejects|TestExecuteInstallResumes|TestExecuteInstall.*Overwrite' -count=1
```

Expected: PASS；原有 completed/unknown 防覆盖用例继续通过。

**Step 5: 提交**

```bash
git add internal/mgsctl/install.go internal/mgsctl/install_test.go internal/mgsctl/cli_test.go
git commit -m "fix: recognize recoverable pending installations"
```

### Task 3: 原位重建未完成安装配置并保留稳定身份

**Files:**
- Modify: `internal/mgsctl/install.go`
- Modify: `internal/mgsctl/install_files.go`
- Modify: `internal/mgsctl/runtime.go`
- Modify: `internal/mgsctl/install_test.go`

**Step 1: 写失败测试**

扩展 `internal/mgsctl/install_test.go`：覆盖时修改 API/Gateway 端口，并断言下列值保持不变：

```go
for _, key := range []string{
    "INSTALLATION_ID", "SETUP_TOKEN", "PIC_GALLERY_JWT_SECRET",
    "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "POSTGRES_PASSWORD",
    "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD",
} {
    if got.Values[key] != before.Values[key] { t.Fatalf("%s changed", key) }
}
if got.Values["API_PORT"] != "19090" || got.Values["GATEWAY_PORT"] != "19080" {
    t.Fatal("new ports were not published")
}
```

另加注入式写失败测试：发布中途失败后恢复旧的完整生成文件集；部署失败则保留新配置供下次 resume；`data/`、`logs/` 和未知文件始终存在。

**Step 2: 确认测试先失败**

```bash
go test ./internal/mgsctl -run 'TestExecuteInstallOverwrite|TestPendingInstallPublication' -count=1
```

Expected: FAIL；当前实现删除旧文件并调用 `BuildRuntimeArtifacts`，会重新生成 installation ID 与 secrets。

**Step 3: 实现稳定值合并与 staged publication**

在 `internal/mgsctl/runtime.go` 增加纯函数：

```go
func BuildPendingRuntimeArtifacts(plan InstallPlan, snapshot pendingInstallSnapshot, entropy io.Reader, now time.Time) (RuntimeArtifacts, error)
```

实现规则：从 snapshot 复制身份、setup token、应用安全密钥和已有 managed credentials；从新 plan 重建端口、组件、镜像、版本、public URL 与 storage mode；仅为新启用且缺失的 managed component 生成凭据。

在 `internal/mgsctl/install_files.go` 增加限定于 manifest-owned 文件的 replace writer：全部 `.mgsctl.stage.*` 写入并校验后才替换；替换失败按备份列表回滚。删除 `ExecuteInstall` 中先调用 `rollbackInstallArtifacts` 清空旧配置的路径。

**Step 4: 运行 focused tests**

```bash
go test ./internal/mgsctl -run 'TestExecuteInstallOverwrite|TestPendingInstallPublication|TestBuildPendingRuntimeArtifacts' -count=1
```

Expected: PASS，且 entropy 不足测试证明已有 secret 不会重生。

**Step 5: 提交**

```bash
git add internal/mgsctl/install.go internal/mgsctl/install_files.go internal/mgsctl/runtime.go internal/mgsctl/install_test.go
git commit -m "fix: rebuild pending install configuration in place"
```

### Task 4: 强制重建同一 Compose 项目服务

**Files:**
- Modify: `internal/mgsctl/docker.go`
- Modify: `internal/mgsctl/docker_test.go`
- Modify: `internal/mgsctl/cli_test.go`

**Step 1: 写失败测试**

更新 `internal/mgsctl/docker_test.go`，断言 install/overwrite 的应用服务命令包含：

```text
compose --project-name app-<same-installation-id> up --detach --wait --force-recreate --remove-orphans ...
```

在 CLI 测试中完整执行“第一次部署失败 -> TUI 改端口 -> overwrite=true -> 第二次部署”，验证两次 Compose `--project-name` 相同，第二次读取新端口环境。

**Step 2: 确认测试先失败**

```bash
go test ./internal/mgsctl -run 'TestDockerProcessSpecs|TestExecuteInstall.*Port|TestExecuteInstall.*Compose' -count=1
```

Expected: FAIL，现有 `up` 缺少 `--force-recreate` 或覆盖后 project identity 改变。

**Step 3: 最小实现**

在 `internal/mgsctl/docker.go` 的 install/update `up` spec 加 `--force-recreate`，保留 `--remove-orphans`。Compose project name 继续只由 preserved `INSTALLATION_ID` 推导，不引入第二个 override 来源。

**Step 4: 验证并提交**

```bash
go test ./internal/mgsctl -run 'TestDockerProcessSpecs|TestExecuteInstall.*Port|TestExecuteInstall.*Compose' -count=1
git add internal/mgsctl/docker.go internal/mgsctl/docker_test.go internal/mgsctl/cli_test.go
git commit -m "fix: recreate pending deployment services in place"
```

Expected: tests PASS；提交只包含 Docker reconciliation。

### Task 5: Linux 安装后持久化 mgsctl PATH

**Files:**
- Modify: `scripts/install.sh`
- Modify: `scripts/test/install-wrapper-contract.sh`

**Step 1: 写失败契约**

在 `scripts/test/install-wrapper-contract.sh` 使用临时 HOME 验证：

- `$HOME/.profile` 获得带固定 marker 的 `export PATH="$MGSCTL_INSTALL_DIR:$PATH"` block；
- Bash 同步更新 `.bashrc`，Zsh 同步更新 `.zshrc`；
- 连续运行两次 marker 只出现一次；
- install dir 已在 PATH 时不写入；
- 当前命令仍通过绝对路径执行安装后的 binary。

**Step 2: 确认契约先失败**

```bash
bash scripts/test/install-wrapper-contract.sh
```

Expected: FAIL，profile/rc 中没有 PATH block。

**Step 3: 实现 POSIX shell helper**

在 `scripts/install.sh` 增加 `ensure_install_dir_on_path`，用如下 marker 管理幂等 block：

```text
# >>> mikiko-gallery-studio mgsctl >>>
export PATH="<absolute-install-dir>:$PATH"
# <<< mikiko-gallery-studio mgsctl <<<
```

只 append 到 regular file；拒绝 symlink；根据 `$SHELL` 选择 `.bashrc`/`.zshrc`，始终覆盖 `.profile`。安装完成后调用 helper，再 `exec "$installed_binary" "$@"`。

**Step 4: 验证并提交**

```bash
bash scripts/test/install-wrapper-contract.sh
git add scripts/install.sh scripts/test/install-wrapper-contract.sh
git commit -m "fix: persist mgsctl path for Linux shells"
```

### Task 6: 修复历史资产图片预览层级

**Files:**
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/ui/redesign-classes.ts`
- Modify: `web/user/src/imageLightboxLayer.contract.ts`

**Step 1: 写失败契约**

把层级合同明确为 modal `110`、lightbox `120`、zoom viewer `130`，并断言 lightbox/zoom 都通过 `OverlayPortal` 挂载且关闭 zoom 后 detail/lightbox 状态仍在。

**Step 2: 确认失败**

```bash
npm exec tsx web/user/src/imageLightboxLayer.contract.ts
```

Expected: FAIL，当前 lightbox backdrop 为 `z-[100]`。

**Step 3: 最小实现并验证**

将 overlay z-index 常量集中到 `redesign-classes.ts`，`components.tsx` 的 `lightboxClasses.backdrop` 使用 `z-[120]`，zoom 使用 `z-[130]`，控件层只高于所属 overlay。

```bash
npm exec tsx web/user/src/imageLightboxLayer.contract.ts
npm --prefix web/user run typecheck
```

Expected: PASS，无类型错误。

**Step 4: 提交**

```bash
git add web/user/src/components.tsx web/user/src/ui/redesign-classes.ts web/user/src/imageLightboxLayer.contract.ts
git commit -m "fix: render gallery lightbox above asset details"
```

### Task 7: 让套餐页复用真实编辑器与 CRUD

**Files:**
- Create: `web/admin/src/pages/CashierPlanEditorDialog.tsx`
- Create: `web/admin/src/pages/cashierPlanDraft.ts`
- Create: `web/admin/src/pages/cashierPlanDraft.contract.ts`
- Modify: `web/admin/src/pages/CashierPage.tsx`
- Modify: `web/admin/src/pages/PackagesPage.tsx`
- Modify: `web/admin/src/pages/ordersPage.contract.ts`

**Step 1: 写失败契约**

覆盖以下纯逻辑：空 draft、row -> draft、draft -> `CashierPlanWriteRequest`；套餐页源码不得再以 Toast 代替 create/edit；保存时分别调用 `createCashierPlan`/`updateCashierPlan`，成功 reload，失败保持 dialog。

**Step 2: 确认失败**

```bash
npm exec tsx web/admin/src/pages/cashierPlanDraft.contract.ts
npm exec tsx web/admin/src/pages/ordersPage.contract.ts
```

Expected: FAIL，新模块不存在，`PackagesPage` 的按钮仍只是反馈提示。

**Step 3: 提取并接入编辑器**

把 `CashierPage.tsx` 中 `PlanDraft`、初始化、`cashierPlanSavePayload` 调用和 Modal 表单抽到新文件。`PackagesPage` 管理 `dialog/saving/error`，调用现有 `adminApi` CRUD；保存成功关闭并刷新，失败显示 `InlineFeedback`。

**Step 4: 验证并提交**

```bash
npm exec tsx web/admin/src/pages/cashierPlanDraft.contract.ts
npm exec tsx web/admin/src/pages/ordersPage.contract.ts
npm --prefix web/admin run typecheck
git add web/admin/src/pages/CashierPlanEditorDialog.tsx web/admin/src/pages/cashierPlanDraft.ts web/admin/src/pages/cashierPlanDraft.contract.ts web/admin/src/pages/CashierPage.tsx web/admin/src/pages/PackagesPage.tsx web/admin/src/pages/ordersPage.contract.ts
git commit -m "fix: provide functional cashier package editing"
```

### Task 8: 建立支付渠道字段契约并修复 JeePay 保存

**Files:**
- Modify: `web/admin/src/pages/cashierProviderOptions.ts`
- Modify: `web/admin/src/pages/cashierProviderOptions.contract.ts`
- Create: `web/admin/src/pages/cashierProviderForm.ts`
- Create: `web/admin/src/pages/cashierProviderForm.contract.ts`
- Modify: `web/admin/src/pages/CashierPage.tsx`
- Modify: `web/shared/api-types.ts`

**Step 1: 写失败契约**

将字段类型合同写成：

```ts
type CashierProviderConfigField = {
  key: string
  label: string
  storage: 'config' | 'secret'
  required: boolean
  sensitive?: boolean
  kind?: 'text' | 'password' | 'textarea' | 'select' | 'callback-base'
  options?: Array<{ value: string; label: string }>
}
```

断言 JeePay `mch_no/app_id`、Alipay `app_id`、WeChat `app_id/mch_id`、EasyPay `pid` 为 `config`；真实签名密钥/私钥为 `secret`。断言 `window.location.origin` 投影为：

```ts
notify_url = `${origin}/api/open/image/v1/payments/webhooks/${providerType}`
return_url = `${origin}/#/checkout`
```

未知 legacy callback path 在未修改时原样保留。

**Step 2: 确认失败**

```bash
npm exec tsx web/admin/src/pages/cashierProviderOptions.contract.ts
npm exec tsx web/admin/src/pages/cashierProviderForm.contract.ts
```

Expected: FAIL；现有 `secret` 同时承担敏感显示与 payload storage，JeePay 商户号/应用 ID 被放入 `secrets`。

**Step 3: 实现单一字段来源与 payload builder**

实现 `providerDraftFromInstance`、`providerPayloadFromDraft`、`callbackBaseFromStoredURL`、`callbackURLFromBase`。新建实例的两个 callback base 默认来自 `window.location.origin`。空 secret 在 edit 时不进入 payload，以保留后端已有密文。

移除 `InstanceDraft.config_text/secrets_text/clear_secrets_text` 及普通流程中的“密钥配置”“渠道配置 JSON”“清除密钥 JSON”。只按字段 metadata 渲染；label 使用必填 `*` 或“选填”，设置 `required/aria-required`，password/textarea 由 `sensitive/kind` 决定。

**Step 4: 验证并提交**

```bash
npm exec tsx web/admin/src/pages/cashierProviderOptions.contract.ts
npm exec tsx web/admin/src/pages/cashierProviderForm.contract.ts
npm --prefix web/admin run typecheck
git add web/admin/src/pages/cashierProviderOptions.ts web/admin/src/pages/cashierProviderOptions.contract.ts web/admin/src/pages/cashierProviderForm.ts web/admin/src/pages/cashierProviderForm.contract.ts web/admin/src/pages/CashierPage.tsx web/shared/api-types.ts
git commit -m "fix: make cashier provider fields contract driven"
```

### Task 9: 在后端保存边界校验渠道必填字段

**Files:**
- Modify: `internal/service/cashier/provider.go`
- Modify: `internal/service/cashier/config_store_test.go`
- Modify: `internal/http/router/admin_cashier_api_test.go`
- Modify: `pkg/errs/codes.go`

**Step 1: 写失败测试**

增加 JeePay create/update 用例：`mch_no/app_id` 在 config 中保存，`key` 在 secrets 中合并；编辑时空 key 保留旧值；缺任一 required field 在保存接口直接返回 `400 PAYMENT_PROVIDER_CONFIG_INVALID`，响应只含字段名不含提交值；GET 响应不回显 key。

**Step 2: 确认失败**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'Test.*Provider.*Config|Test.*JeePay.*Save|TestAdminCashier.*Provider' -count=1
```

Expected: FAIL，当前只做通用 secret merge，缺失字段可能推迟到下单时才报错。

**Step 3: 最小实现**

在 `provider.go` 增加 provider requirement table 与 `ValidateProviderConfiguration`，在 `ProviderInstanceForWrite` merge 后执行。新增 `errs.CodePaymentProviderConfigInvalid`。保持 `ConfigKeyIsSecret` 与响应 redaction；不得把 `mch_no/app_id/pid` 加进 secret classifier。

**Step 4: 验证并提交**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'Test.*Provider.*Config|Test.*JeePay.*Save|TestAdminCashier.*Provider' -count=1
git add internal/service/cashier/provider.go internal/service/cashier/config_store_test.go internal/http/router/admin_cashier_api_test.go pkg/errs/codes.go
git commit -m "fix: validate persisted payment provider credentials"
```

### Task 10: 注册 Stripe 类型、配置与依赖

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `web/user/package.json`
- Modify: `web/user/package-lock.json`
- Modify: `web/shared/api-types.ts`
- Modify: `internal/service/cashier/provider.go`
- Modify: `internal/service/cashier/payment_adapter.go`
- Modify: `internal/service/cashier/query_adapter.go`
- Modify: `internal/service/cashier/refund_adapter.go`
- Modify: `web/admin/src/pages/cashierProviderOptions.ts`
- Modify: `web/admin/src/pages/cashierProviderOptions.contract.ts`

**Step 1: 写失败测试/契约**

断言 provider type `stripe` 只支持 visible method `stripe`，默认 method 为 `stripe`；字段包含 normal config `publishable_key` 和 secrets `secret_key/webhook_secret`，三项必填；registry builder structs 均有 Stripe slot。

**Step 2: 确认失败**

```bash
go test ./internal/service/cashier -run 'TestProvider.*Stripe|Test.*AdapterRegistry' -count=1
npm exec tsx web/admin/src/pages/cashierProviderOptions.contract.ts
```

Expected: FAIL，Stripe 未注册。

**Step 3: 安装锁定依赖**

```bash
go get github.com/stripe/stripe-go/v85@latest
npm --prefix web/user install @stripe/stripe-js @stripe/react-stripe-js
```

Expected: `go.mod/go.sum` 与 user web 两个 lock files 仅增加 Stripe 官方依赖。

**Step 4: 注册类型和配置**

扩展 Go registry builder structs、`ProviderTypeAllowed`、method mapping 与前端 `PaymentProviderType`。在 admin metadata 增加 Stripe 三个字段和 callback base；secret response 继续只暴露 `has_secret/fingerprint`。

**Step 5: 验证并提交**

```bash
go test ./internal/service/cashier -run 'TestProvider.*Stripe|Test.*AdapterRegistry' -count=1
npm exec tsx web/admin/src/pages/cashierProviderOptions.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
git add go.mod go.sum web/user/package.json web/user/package-lock.json web/shared/api-types.ts internal/service/cashier/provider.go internal/service/cashier/payment_adapter.go internal/service/cashier/query_adapter.go internal/service/cashier/refund_adapter.go web/admin/src/pages/cashierProviderOptions.ts web/admin/src/pages/cashierProviderOptions.contract.ts
git commit -m "feat: register Stripe cashier provider"
```

### Task 11: Stripe PaymentIntent 下单

**Files:**
- Create: `internal/service/cashier/stripe_provider.go`
- Create: `internal/service/cashier/stripe_provider_test.go`
- Modify: `internal/service/cashier/payment_adapter.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/cashier_api_test.go`
- Modify: `web/shared/api-types.ts`

**Step 1: 写失败测试**

用注入式 Stripe client 记录 params，断言：

- `10.25` CNY 精确转为 `1025` fen，无 float；
- currency 为 `cny`，metadata 有本地 `order_no`；
- idempotency key 为订单号；
- display 仅返回 `type=stripe_payment_element`、`client_secret`、`publishable_key`；
- secret key/webhook secret 不出现在 JSON。

**Step 2: 确认失败**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*PaymentIntent|TestCashier.*Stripe.*Order' -count=1
```

Expected: FAIL，Stripe builder 不存在。

**Step 3: 最小实现**

定义窄接口便于测试：

```go
type StripePaymentIntents interface {
    New(*stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
    Get(string, *stripe.PaymentIntentParams) (*stripe.PaymentIntent, error)
}
```

实现 `StripeAmountFenFromCNY`、`StripePaymentDisplayBuilder` 和 standard builder wiring。SDK 使用 instance 的 `secret_key`，不写全局 `stripe.Key`；通过 backend/client params 注入测试后端。将 PaymentIntent ID 放入现有 provider trade/display transport。

**Step 4: 验证并提交**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*PaymentIntent|TestCashier.*Stripe.*Order' -count=1
git add internal/service/cashier/stripe_provider.go internal/service/cashier/stripe_provider_test.go internal/service/cashier/payment_adapter.go internal/http/handlers/api.go internal/http/router/cashier_api_test.go web/shared/api-types.ts
git commit -m "feat: create Stripe payment intents"
```

### Task 12: Stripe 原始请求体验签 Webhook 与幂等到账

**Files:**
- Modify: `internal/service/cashier/stripe_provider.go`
- Modify: `internal/service/cashier/stripe_provider_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/cashier_api_test.go`
- Modify: `pkg/errs/codes.go`

**Step 1: 写失败测试**

用固定 webhook secret 和 Stripe 签名 fixture 覆盖：有效 `payment_intent.succeeded`、篡改一个 byte 后签名失败、重复 event 只到账一次、金额不符、`payment_intent.payment_failed`、无关 event 返回 200 不改订单。

**Step 2: 确认失败**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*Webhook|TestCashier.*Stripe.*Webhook' -count=1
```

Expected: FAIL，handler 没有 Stripe 分支。

**Step 3: 实现原始 body 验签**

`HandlePaymentWebhook` 对 Stripe 先以限制大小的 `io.ReadAll` 获取 exact bytes，读取 `Stripe-Signature`，调用 `webhook.ConstructEvent`。以 event ID 进入现有 webhook event 幂等记录，以 metadata/order no 找订单，以 PaymentIntent amount/currency 校验本地订单，再复用现有 `CompletePaymentOrder`/ledger 路径。

安全错误只返回 `PAYMENT_SIGNATURE_INVALID`、`PAYMENT_AMOUNT_MISMATCH` 或 provider unavailable，不包含 upstream body、secret 或签名内容。

**Step 4: 验证并提交**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*Webhook|TestCashier.*Stripe.*Webhook' -count=1
git add internal/service/cashier/stripe_provider.go internal/service/cashier/stripe_provider_test.go internal/http/handlers/api.go internal/http/router/cashier_api_test.go pkg/errs/codes.go
git commit -m "feat: process Stripe payment webhooks"
```

### Task 13: Stripe 查单、退款与退款查询

**Files:**
- Modify: `internal/service/cashier/stripe_provider.go`
- Modify: `internal/service/cashier/stripe_provider_test.go`
- Modify: `internal/service/cashier/query_adapter.go`
- Modify: `internal/service/cashier/query_provider.go`
- Modify: `internal/service/cashier/refund_adapter.go`
- Modify: `internal/service/cashier/refund_provider.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/admin_cashier_api_test.go`

**Step 1: 写失败测试**

表驱动映射 PaymentIntent 状态：`succeeded -> paid`，`processing/requires_action -> pending`，`canceled/payment_failed -> failed`。退款测试覆盖 full/partial amount、以本地 refund trade no 幂等创建、`succeeded/pending/failed/canceled` 查询映射及 provider IDs 写入现有字段。

**Step 2: 确认失败**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*Query|TestStripe.*Refund|TestAdminCashier.*Stripe' -count=1
```

Expected: FAIL，standard query/refund builders 尚无 Stripe 实现。

**Step 3: 实现并接入现有后台动作**

为 PaymentIntent/Refund 定义窄 client interfaces。金额继续用 decimal -> fen 转换；退款使用 PaymentIntent ID，幂等 key 使用本地 refund trade no。返回现有 `QueryOrderStatusResult`/`RefundPaymentResult`，不增加旁路状态表。

**Step 4: 验证并提交**

```bash
go test ./internal/service/cashier ./internal/http/router -run 'TestStripe.*Query|TestStripe.*Refund|TestAdminCashier.*Stripe' -count=1
git add internal/service/cashier/stripe_provider.go internal/service/cashier/stripe_provider_test.go internal/service/cashier/query_adapter.go internal/service/cashier/query_provider.go internal/service/cashier/refund_adapter.go internal/service/cashier/refund_provider.go internal/http/handlers/api.go internal/http/router/admin_cashier_api_test.go
git commit -m "feat: support Stripe query and refunds"
```

### Task 14: 用户端 Stripe Payment Element

**Files:**
- Create: `web/user/src/pages/StripePaymentPanel.tsx`
- Create: `web/user/src/pages/checkoutStripePayment.ts`
- Create: `web/user/src/pages/checkoutStripePayment.contract.ts`
- Modify: `web/user/src/pages/checkoutPaymentDisplay.ts`
- Modify: `web/user/src/pages/checkoutPaymentDisplay.contract.ts`
- Modify: `web/user/src/pages/CheckoutPage.tsx`
- Modify: `web/shared/api-types.ts`

**Step 1: 写失败契约**

断言 `stripe_payment_element` display 必须同时有 publishable key/client secret；缺失时展示 unsupported 配置错误；return URL 为 `${window.location.origin}/#/checkout`。普通卡支付确认成功后保持 modal 并继续现有 polling；requires_action 允许 SDK redirect；SDK error 留在 panel 内。

**Step 2: 确认失败**

```bash
npm exec tsx web/user/src/pages/checkoutStripePayment.contract.ts
npm exec tsx web/user/src/pages/checkoutPaymentDisplay.contract.ts
```

Expected: FAIL，现有 display model 不认识 Stripe。

**Step 3: 实现 Payment Element panel**

按 publishable key 缓存 `loadStripe` Promise；用 `<Elements options={{clientSecret}}>` 和 `<PaymentElement>` 渲染；提交时调用 `stripe.confirmPayment({ elements, confirmParams: { return_url }, redirect: 'if_required' })`。不得把 `client_secret` 写入 log、toast 或持久化 storage。

**Step 4: 验证并提交**

```bash
npm exec tsx web/user/src/pages/checkoutStripePayment.contract.ts
npm exec tsx web/user/src/pages/checkoutPaymentDisplay.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/user run build
git add web/user/src/pages/StripePaymentPanel.tsx web/user/src/pages/checkoutStripePayment.ts web/user/src/pages/checkoutStripePayment.contract.ts web/user/src/pages/checkoutPaymentDisplay.ts web/user/src/pages/checkoutPaymentDisplay.contract.ts web/user/src/pages/CheckoutPage.tsx web/shared/api-types.ts
git commit -m "feat: confirm Stripe payments in checkout"
```

### Task 15: 在系统设置恢复积分换算配置

**Files:**
- Modify: `web/admin/src/pages/configRows.ts`
- Modify: `web/admin/src/pages/configRows.contract.ts`
- Modify: `web/admin/src/pages/ConfigPage.tsx`
- Modify: `web/admin/src/pages/SystemSettingsPage.tsx`
- Modify: `internal/service/adminconfig/service_test.go`
- Modify: `internal/http/router/admin_config_api_test.go`

**Step 1: 写失败契约**

断言系统设置出现“积分换算”入口，只 allowlist `billing_pricing/cny_per_point`（兼容当前后端真实 key/shape），默认展示 `0.3125`，保存走现有 versioned config API；`payments` 与其他危险 billing JSON 不得随类目暴露。

**Step 2: 确认失败**

```bash
npm exec tsx web/admin/src/pages/configRows.contract.ts
go test ./internal/service/adminconfig ./internal/http/router -run 'Test.*CNYPerPoint|TestAdminConfig.*Billing' -count=1
```

Expected: frontend FAIL，`generalConfigCategories` 当前过滤 `billing_pricing`；backend 测试应确认默认值或指出实际 key 映射缺口。

**Step 3: 实现 allowlist view**

让 `ConfigPage` 接受 `{ categories, keys }` allowlist。`SystemSettingsPage` 新增积分换算 tab，仅传 `billing_pricing` 和 `cny_per_point`；不要把整个 billing category 加入通用设置。若后端实际存储为 micros，则在既有 config service 边界做精确 decimal 投影，默认保持 `0.3125`。

**Step 4: 验证并提交**

```bash
npm exec tsx web/admin/src/pages/configRows.contract.ts
go test ./internal/service/adminconfig ./internal/http/router -run 'Test.*CNYPerPoint|TestAdminConfig.*Billing' -count=1
npm --prefix web/admin run typecheck
git add web/admin/src/pages/configRows.ts web/admin/src/pages/configRows.contract.ts web/admin/src/pages/ConfigPage.tsx web/admin/src/pages/SystemSettingsPage.tsx internal/service/adminconfig/service_test.go internal/http/router/admin_config_api_test.go
git commit -m "fix: expose point conversion system setting"
```

### Task 16: 在积分页底部增加兑换码入口

**Files:**
- Create: `web/user/src/pages/RedeemCodeForm.tsx`
- Create: `web/user/src/pages/redeemCodeForm.ts`
- Create: `web/user/src/pages/redeemCodeForm.contract.ts`
- Modify: `web/user/src/pages/ProfilePage.tsx`
- Modify: `web/user/src/pages/CheckoutPage.tsx`

**Step 1: 写失败契约**

断言初始 code 为空，trim 后非空才能提交；重复点击共享 busy guard；成功后清空输入、只发一个 success notification，并触发 account balance + checkout data refresh；组件位于 `CheckoutPage` 最近订单之后。

**Step 2: 确认失败**

```bash
npm exec tsx web/user/src/pages/redeemCodeForm.contract.ts
```

Expected: FAIL；现有逻辑只在 ProfilePage，且默认值是 `WELCOME-2026`。

**Step 3: 提取复用组件**

创建 `RedeemCodeForm`，由调用方传入 `onRedeemed`。Checkout 成功回调执行 `app.refreshAccount()` 和现有 checkout reload；Profile 可继续复用，但不得重复 toast。

**Step 4: 验证并提交**

```bash
npm exec tsx web/user/src/pages/redeemCodeForm.contract.ts
npm --prefix web/user run typecheck
git add web/user/src/pages/RedeemCodeForm.tsx web/user/src/pages/redeemCodeForm.ts web/user/src/pages/redeemCodeForm.contract.ts web/user/src/pages/ProfilePage.tsx web/user/src/pages/CheckoutPage.tsx
git commit -m "feat: add redemption to the points page"
```

### Task 17: 新注册用户使用邮箱名前缀作为昵称

**Files:**
- Modify: `internal/service/auth/service.go`
- Modify: `internal/service/auth/service_test.go`
- Modify: `internal/http/router/auth_api_test.go`

**Step 1: 写失败测试**

为纯函数和 memory/database 两条注册路径增加：

```go
func TestDefaultNicknameFromEmail(t *testing.T) {
    tests := map[string]string{
        "alice@example.com": "alice",
        " Alice.Name+tag@Example.com ": "alice.name+tag",
    }
    // 同时覆盖 nickname 长度上限和 malformed defensive fallback。
}
```

连续创建两个 DB 用户，断言昵称分别来自各自邮箱且不是 `user-1`；已有用户和 profile update 不受影响。

**Step 2: 确认失败**

```bash
go test ./internal/service/auth ./internal/http/router -run 'TestDefaultNickname|Test.*Register.*Nickname' -count=1
```

Expected: FAIL，database-backed `createUserLocked` 在 `nextUserID++` 前返回。

**Step 3: 最小实现**

增加 `defaultNicknameFromEmail(normalizedEmail string) string`，取 `@` 前非空 local part 并按 domain nickname 上限截断；`createUserLocked` 的两种 store mode 都使用它。保留 `nextUserID` 仅作为 memory ID，不再参与昵称。

**Step 4: 验证并提交**

```bash
go test ./internal/service/auth ./internal/http/router -run 'TestDefaultNickname|Test.*Register.*Nickname' -count=1
git add internal/service/auth/service.go internal/service/auth/service_test.go internal/http/router/auth_api_test.go
git commit -m "fix: derive new user nicknames from email"
```

### Task 18: 建立文本模型默认项不变量与明确错误

**Files:**
- Modify: `pkg/errs/codes.go`
- Modify: `internal/repository/repoerr/repoerr.go`
- Modify: `internal/service/textmodel/store.go`
- Modify: `internal/service/textmodel/service.go`
- Modify: `internal/service/textmodel/service_test.go`
- Modify: `internal/repository/entstore/text_model_store.go`
- Modify: `internal/repository/entstore/text_model_store_test.go`
- Modify: `internal/http/router/text_model_api_test.go`

**Step 1: 写失败测试**

服务与 Ent store 覆盖：第一个 enabled model 自动 default；已有 default 不被新建模型抢占；禁用/删除 default 后唯一 eligible candidate 自动接替；无 candidate 返回“未配置”；多个 eligible 且无 default 返回 `409 TEXT_MODEL_DEFAULT_REQUIRED`；单一 legacy candidate 在 resolve 时事务自愈；disabled account/model 不 eligible。

**Step 2: 确认失败**

```bash
go test ./internal/service/textmodel ./internal/repository/entstore ./internal/http/router -run 'Test.*DefaultModel|TestPromptOptimization.*Default' -count=1
```

Expected: FAIL，Create/Test 不设置 `is_default`，`GetDefaultModel` 直接返回 not found。

**Step 3: 定义事务接口与错误**

在 store contract 增加：

```go
type DefaultSelection struct {
    Account domaintextmodel.AccountRecord
    Model   domaintextmodel.Model
}

ReconcileDefaultModel(ctx context.Context, preferredModelID *int64) (DefaultSelection, error)
```

Ent 实现必须在 transaction 内：锁定/查询 enabled account + enabled model；保留合法现有 default；有 preferred 且当前无 default 时选择 preferred；无 preferred 时仅在唯一 candidate 时修复；多 candidate 返回 `repoerr.ErrDefaultModelRequired`。MemoryStore 实现相同语义。

新增 `errs.CodeTextModelDefaultRequired = "TEXT_MODEL_DEFAULT_REQUIRED"`，映射为 HTTP 409 和管理操作提示。Create/Update/Delete model、Update account 在状态改变后 reconcile；`ResolveDefaultModel` 使用该接口，不再把歧义映射为通用 404。

**Step 4: focused verification**

```bash
go test ./internal/service/textmodel ./internal/repository/entstore ./internal/http/router -run 'Test.*DefaultModel|TestPromptOptimization.*Default' -count=1
```

Expected: PASS；并发 set/reconcile 测试始终最多一个 default。

**Step 5: 提交**

```bash
git add pkg/errs/codes.go internal/repository/repoerr/repoerr.go internal/service/textmodel/store.go internal/service/textmodel/service.go internal/service/textmodel/service_test.go internal/repository/entstore/text_model_store.go internal/repository/entstore/text_model_store_test.go internal/http/router/text_model_api_test.go
git commit -m "fix: enforce text model default readiness"
```

### Task 19: 文本模型操作按钮与就绪状态

**Files:**
- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/adminControls.contract.ts`
- Modify: `web/admin/src/pages/TextModelsPage.tsx`
- Modify: `web/admin/src/pages/textModelRows.ts`
- Modify: `web/admin/src/pages/textModelRows.contract.ts`
- Modify: `web/shared/api-types.ts`

**Step 1: 写失败契约**

断言 action target 至少 `40x40`，icon 为 `18-20px`，每个 action 有 accessible name；tooltip 由 portal 渲染并支持 hover/focus，disabled action 仍可显示解释。页面明确显示当前 default；缺失时显示“提示词优化尚未就绪”；连接测试成功文案只说明连通性。

**Step 2: 确认失败**

```bash
npm exec tsx web/admin/src/adminControls.contract.ts
npm exec tsx web/admin/src/pages/textModelRows.contract.ts
```

Expected: FAIL；页面 local button 使用 15px icon/native title，且没有 readiness warning。

**Step 3: 实现共享 TooltipIconButton**

扩展 shared admin control：button 本体 40x40、Lucide icon 18-20、`aria-label`；外层非 disabled trigger 捕获 focus/hover，在 portal tooltip 中显示 label/reason。删除 `TextModelsPage` 的 local icon-button 样式和 `title`。根据 models 的 `is_default/enabled` 生成 readiness banner。

**Step 4: 验证并提交**

```bash
npm exec tsx web/admin/src/adminControls.contract.ts
npm exec tsx web/admin/src/pages/textModelRows.contract.ts
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
git add web/admin/src/components.tsx web/admin/src/adminControls.contract.ts web/admin/src/pages/TextModelsPage.tsx web/admin/src/pages/textModelRows.ts web/admin/src/pages/textModelRows.contract.ts web/shared/api-types.ts
git commit -m "fix: clarify text model actions and readiness"
```

### Task 20: 跨边界回归、手工验收与发布说明

**Files:**
- Modify: `scripts/test/api_contract_smoke.sh`
- Modify: `scripts/e2e/docker-e2e.mjs`
- Modify: `docs/runbooks/backend-deployment.md`
- Modify: `docs/deploy/backend-runbook.md`
- Create: `docs/runbooks/cashier-provider-configuration.md`

**Step 1: 扩展集成 smoke**

加入 JeePay config/secrets 保存与 redaction、CNY per point、文本模型单 candidate default、兑换码入口所依赖 API。Stripe 使用本地 fake Stripe HTTP server，不要求真实 key；覆盖 create intent、signed webhook duplicate、query 和 refund。

**Step 2: 运行 focused integration**

```bash
./scripts/workflow/api-smoke.sh
```

Expected: PASS，脚本自动创建并清理 PostgreSQL、Redis、API、Worker 与 fake provider；不接触已有 runtime。

**Step 3: 手工浏览器验收**

启动开发环境后，在 desktop 与 mobile viewport 检查：

- 历史资产 detail -> lightbox -> zoom 的层级与关闭顺序；
- 套餐 create/edit/save/error；
- JeePay/Stripe provider 必填、选填、回调基础域名、无 raw JSON、无 secret replay；
- Stripe test-mode Payment Element（仅手工验收使用用户提供的 test credentials）；
- 积分换算保存生效、积分页底部兑换；
- 文本模型 tooltip、default banner、用户端优化提示词。

Expected: 无重叠、无明文密钥、移动端字段与按钮不溢出。

**Step 4: 更新文档**

记录 pending install overwrite 保留项、PATH 生效时机、Stripe publishable/secret/webhook secret 配置、Webhook URL、CNY-only 限制和升级后文本模型 default 检查步骤。

**Step 5: 全量验证**

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
```

Expected: 全部 PASS。

**Step 6: 提交集成与文档**

```bash
git add scripts/test/api_contract_smoke.sh scripts/e2e/docker-e2e.mjs docs/runbooks/backend-deployment.md docs/deploy/backend-runbook.md docs/runbooks/cashier-provider-configuration.md
git commit -m "test: cover post-release remediation workflows"
```

### Task 21: Review gate 与交付检查

**Files:**
- Generated: `.review/gate.json`

**Step 1: 确认提交范围**

```bash
git status --short
git log --oneline origin/main..HEAD
git diff --check origin/main...HEAD
```

Expected: worktree clean（允许仓库明确忽略的本地 workflow 文件），无 whitespace error，提交按上述纵向任务拆分。

**Step 2: 运行最终仓库验证**

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
```

Expected: PASS。

**Step 3: 生成 committed-scope review marker**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: `.review/gate.json` 为 `PASS`、scope 为 `committed`、tree SHA 与当前 HEAD tree 一致。

**Step 4: 最终安全检查**

```bash
git diff origin/main...HEAD | rg -n 'dckr_pat_|sk_live_|whsec_|secret_key[^A-Za-z]' || true
```

Expected: 除测试中的显式 fake placeholder/key name 外，不出现真实 token、Stripe secret 或 webhook secret。

**Step 5: 进入交付流程**

使用 `dev-ship` 完成最终校验、推送和 PR；PR 描述按安装、支付、管理端、用户端、验证五部分列出行为变化与 Stripe 配置要求。
