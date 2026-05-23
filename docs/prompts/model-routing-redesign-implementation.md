# 新线程实现 Prompt：Pic Gallery 模型接入、路由模型、价格与用户分组改造

请在仓库 `/Users/fatballfish/Documents/Projects/GoProjects/Personal/pic-gallery` 中实现模型接入、路由模型、价格和用户分组的彻底改造。

必须先阅读并遵守：

- `AGENTS.md`
- `.agents/skills/dev-start-coding/SKILL.md`
- `.agents/skills/dev-go-patterns/SKILL.md`
- `.agents/skills/dev-react-patterns/SKILL.md`
- `docs/tech/2026-05-23-model-routing-redesign.md`
- `docs/prd/pic-gallery-prd.md`

实现前必须使用 `dev-start-coding`，确保 `.coding-context.json` 存在。若触碰 Go，先加载 `dev-go-patterns`；若触碰 React/TypeScript/CSS，先加载 `dev-react-patterns`。

## 改造目标

当前模型体系不可继续沿用。不要做历史兼容迁移，不要保留配置文件中的默认 Provider、默认路由、默认价格 fallback。项目仍处于开发阶段，可以直接重构旧模型体系。

最终概念必须统一为：

1. 模型接入账号 `model_accounts`
   - 表示一个上游账号、端点和凭据组合。
   - 参考 Sub2API Account 的 platform/type/credentials 思路。
   - `adapter_type`: `openai_compatible` / `openrouter`
   - `auth_type`: 当前只真正支持 `api_key`，预留其他类型但不能启用未实现类型。
   - 必须包含 `base_url` 和加密凭据。

2. 真实上游模型 `model_account_models`
   - 挂在模型接入账号下面。
   - `model_code` 表示真实上游模型名，例如 `gpt-image-1`。
   - 一个模型接入账号可以支持多个真实模型。

3. 路由模型 `route_models`
   - 表示用户工作台可见模型，例如 `basic`、`plus`、`pro`。
   - 通过 `route_model_candidates` 映射到多个真实上游模型。
   - 支持 priority、weight、fallback。

4. 用户分组权益
   - 一个用户可以绑定多个用户分组。
   - 路由模型可设置 public / groups / hidden。
   - groups 模型通过 `route_model_visibility_groups` 绑定可见分组。
   - capabilities 返回时合并用户所有分组可见模型并按 route model 去重。

## 计费规则

用户选择某个路由模型时，价格倍率按以下规则计算：

```text
有效倍率 = 当前用户所属分组中，与当前路由模型存在可见关系的分组倍率最小值
```

如果模型是 public 且没有命中任何专属分组，倍率为 `1.00000`。

如果模型同时 public 且命中专属分组，取：

```text
min(1.00000, 命中分组倍率列表)
```

积分计算：

```text
raw_points = base_points * effective_multiplier * task_multiplier * image_count
charged_points = round(raw_points, 5)
display_points = round(charged_points, 2)
```

后端实际余额校验、预扣、扣费、流水、任务快照全部使用 5 位小数；前端展示 2 位小数，不能参与扣费。

## 后端要求

- 新增/重构 Ent schema：
  - `model_accounts`
  - `model_account_models`
  - `route_models`
  - `route_model_candidates`
  - `route_model_prices`
  - `user_group_members`
  - `route_model_visibility_groups`
- 改造 `user_groups` 为权益分组，包含 `multiplier`。
- 改造 image task 和 billing 快照字段，记录 route model、account model、model account、upstream model code、effective multiplier、charged points。
- 删除或替换旧 `model_providers`、`provider_models`、`model_routes` 的运行时依赖。
- 删除配置文件 `providers.openai/openrouter`、`routing.provider_model_map` 对运行时的默认 fallback。
- 后端根据 DB 中的 `adapter_type + auth_type + base_url + credentials` 创建上游请求。
- 当前只实现：
  - `openai_compatible + api_key`
  - `openrouter + api_key`
- 未实现的 adapter/auth 组合不能被启用。
- 所有管理后台写操作写入审计日志。
- 密钥必须加密存储，API 响应只返回 `credentials_status`。

## 前端要求

管理后台需要改造：

- 模型接入页面：创建账号、编辑账号、密钥状态、真实模型子表、测试连接入口可预留。
- 路由模型页面：创建 Basic/Plus/Pro/custom，配置可见性、可见分组、候选真实模型、优先级、权重、fallback。
- 价格配置页面：按路由模型、任务类型、质量配置基础积分和参考图倍率。
- 用户分组页面：CRUD 分组、倍率、启停、默认标记。
- 用户管理页面：支持给一个用户绑定多个分组。

用户工作台需要改造：

- capabilities 使用后端返回的 `model_groups`。
- 模型选择使用 `route_model_code`。
- 展示价格使用 `display_points`。
- 创建任务和价格预估不能传旧 `abstract_model`。

## 接口要求

参考 `docs/tech/2026-05-23-model-routing-redesign.md` 的接口设计更新 OpenAPI 和共享 TypeScript 类型。重点接口包括：

- `/api/ops/admin/v1/model-accounts`
- `/api/ops/admin/v1/model-accounts/{account_id}/models`
- `/api/ops/admin/v1/route-models`
- `/api/ops/admin/v1/route-models/{route_model_id}/candidates`
- `/api/ops/admin/v1/route-model-prices`
- `/api/ops/admin/v1/user-groups`
- `/api/ops/admin/v1/users/{user_id}/groups`
- `/api/agent/image/v1/capabilities`
- `/api/agent/billing/v1/estimate`
- 创建图片任务接口

## 测试要求

必须补充并运行：

- Go 单测：
  - 多用户分组模型可见性。
  - 可见模型去重。
  - 最低倍率计算。
  - 5 位积分精度。
  - public + groups 混合规则。
  - 路由模型候选解析和 fallback。
  - OpenAI-compatible/OpenRouter adapter 请求构造。
- OpenAPI 契约测试。
- React typecheck/build。
- Docker E2E。

完成前必须运行：

```bash
./scripts/workflow/verify.sh
./scripts/e2e/run-docker-e2e.sh
./scripts/workflow/ship-guard.sh
```

如果改动后 E2E 发现问题，必须修复并重新部署到 Docker，直到通过。

## 验收标准

- 后台不再依赖配置文件中的默认模型 Provider 和路由。
- 后台配置的模型接入账号能真实驱动生图请求。
- 用户工作台看到的 Basic/Plus/Pro 来自路由模型配置。
- 一个用户可绑定多个分组。
- 用户可见模型为所有分组可见模型聚合去重结果。
- 价格倍率按当前模型命中的用户分组最低倍率计算。
- 扣费精确到小数后 5 位，前端展示小数后 2 位。
- 所有新增/修改接口失败时管理后台有错误提示。
- 所有关键后台写操作有审计日志。
