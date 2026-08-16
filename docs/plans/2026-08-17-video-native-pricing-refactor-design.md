# 视频厂商原生计费与混合路由报价重构设计

> 状态：已确认，待实施
> 日期：2026-08-17
> 适用范围：Seedance 2.0/2.5、MiniMax H3、视频真实模型、视频路由模型、视频报价与积分结算
> 变更性质：允许破坏性清理历史视频价格配置；必须保留历史任务、报价快照、积分流水和结算结果

## 1. 背景

当前视频计费配置同时存在真实模型组合成本、视频价格策略、参数销售价格和路由价格绑定。管理员需要填写积分净收入、支付费率、目标毛利、成本缓冲、平台固定成本、素材成本、音频成本、预留倍率等大量字段，仍无法准确表达 Seedance 与 MiniMax 的原生计费模型。

现有后台还把任务类型、时长、分辨率和音频模式展开成组合，并为所有组合填写同一个 `cost_cny`。这既不符合 Seedance 的 Token 计费，也不符合 MiniMax H3 的输出秒数与素材附加计费。

本设计将视频售价改为“厂商原生计费公式 + 管理员维护的销售单价”，再复用系统唯一的人民币积分汇率换算积分。成本、利润、支付费用和风险缓冲由管理员预先折入销售单价，平台不再二次推导毛利。

## 2. 目标与非目标

### 2.1 目标

1. Seedance 按百万 Token 销售单价配置和报价。
2. MiniMax H3 按输出秒数、输入视频秒数和超额图片数量配置和报价。
3. 一个路由可同时绑定多个 Seedance、MiniMax 或同厂商真实模型。
4. 混合路由按本次请求所有可用候选模型的最高销售价生成固定报价。
5. 复用 `billing_pricing.cny_per_point`，不重复配置积分汇率。
6. 后台只展示管理员真正需要维护的厂商费率、最低任务积分和取整步长。
7. 价格变更不影响已创建任务；历史任务和积分账务可继续审计。
8. 允许清理旧视频价格配置并要求管理员上线后重新配置。

### 2.2 非目标

1. 不自动计算支付手续费、积分商品实际净收入或目标毛利。
2. 不根据有效积分包、赠送积分或支付渠道反推视频售价。
3. 不提供逐任务类型、分辨率、时长和素材组合的人工最终积分表。
4. 不在任务完成后根据 Provider 实际用量改变用户锁定价格。
5. 不用销售费率冒充 Provider 采购成本。
6. 本设计不新增 Seedance 或 MiniMax 尚未完成真实适配的生成功能；未打通的素材类型不得仅凭费率配置对用户开放。

## 3. 已确认业务决策

1. 后台填写的是面向用户的“销售单价 CNY”，允许高于厂商官方价格。
2. 管理员自行将成本、利润、支付费用和风险缓冲折入销售单价。
3. 平台只负责执行厂商计费公式并按全局积分汇率换算积分。
4. 混合路由取所有可用候选模型的最高报价，生成前锁价。
5. Provider 实际 `usage` 只用于内部审计和费率校准，不重新结算用户积分。
6. 路由不提供逐组合人工最终积分，只提供最低任务积分和积分取整步长。
7. 旧视频价格配置允许破坏性清理；历史任务数据和账务必须保留。

## 4. 官方计费依据

### 4.1 Seedance

官方价格文档：<https://docs.volcengine.com/docs/82379/1544106?lang=zh#2864f00a>

官方规则：

```text
视频价格 = Token 单价 x Token 用量

Token 用量估算 =
(输入视频时长 + 输出视频时长)
x 输出视频宽
x 输出视频高
x 输出视频帧率
/ 1024
```

Seedance 2.0 系列和 Seedance 2.5 按以下维度区分百万 Token 单价：

1. 真实模型。
2. 输出视频分辨率。
3. 输入是否包含视频。

当输入包含视频时存在最低 Token 用量限制。最低 Token 与模型、分辨率、宽高比和输出时长有关。计算器必须使用内置、版本化的官方最低 Token 规则；不能要求管理员手填任意最低 Token。

Provider 返回的准确用量以 `usage.completion_tokens` 为准。该值仅用于内部审计，不改变生成前已锁定的用户售价。

### 4.2 MiniMax H3

官方价格文档：<https://platform.minimaxi.com/docs/guides/pricing-paygo#%E6%A0%87%E5%87%86>

MiniMax H3 标准视频生成当前计费维度：

| 计费项 | 官方规则 |
|---|---|
| 768P 输出视频 | 按输出秒数计费 |
| 2K 输出视频 | 按输出秒数计费 |
| 输入音频 | 免费 |
| 输入图片 | 5 张以内免费，超出部分按张计费 |
| 输入视频 | 按输入视频时长和生成视频分辨率计费 |

本设计只覆盖 MiniMax H3 标准视频生成。`MiniMax-H3-Regeneration` 和 `MiniMax-H3-Context-IR` 必须作为不同 `pricing_schema` 另行接入，不能复用 H3 标准生成公式。

## 5. 当前代码问题

### 5.1 真实模型成本错误地展开为组合

`web/admin/src/pages/VideoProviderAccountsPanel.tsx` 当前将：

```text
任务类型 x 时长 x 分辨率 x 音频模式
```

展开成 `combinations[]`，所有组合复用一个 `cost_cny`，并保存为 `billing_mode=combination`。该模型无法表达厂商原生费率。

### 5.2 后台安全线与底层费率能力脱节

`internal/repository/entstore/video_worker_store.go` 已能读取：

- `output_second_cny`
- `input_video_second_cny`
- `reference_image_cny`
- `provider_million_tokens_cny`

但 `internal/service/adminvideo/config.go` 的试算只遍历 `combinations[].cost_cny`。即使管理员填写底层费率，路由启用检查仍可能返回“没有 Provider 成本”。

### 5.3 视频价格策略字段重复且职责不清

`video_pricing_strategies` 当前同时保存积分面值、净收入下限、赠送比例、支付费率、毛利、成本缓冲、平台成本和预留倍率。多数信息已有系统配置来源，且部分字段没有完整进入运行时公式。

`video_price_rules` 又按任务类型、分辨率和音频模式维护固定积分、秒数积分、素材积分和最低积分，形成第二套销售公式。

### 5.4 Seedance 用量字段优先级错误

Seedance 官方要求准确用量使用 `usage.completion_tokens`。当前适配器优先取 `total_tokens`，需要调整为：

```text
completion_tokens -> total_tokens 仅作兼容回退
```

### 5.5 当前视频任务仅接受图片输入

`internal/service/videotask/service.go` 当前拒绝非图片资产；`domainvideo.Input` 没有素材时长；Seedance 与 MiniMax Provider 适配器会把全部输入编码为 `image_url`。

因此输入视频秒价和音频规则可以先被正确存储，但在真实输入链路完成前不能对用户开放对应能力。

## 6. 核心计费语义

### 6.1 销售单价

真实模型费率不是采购成本，而是管理员已经计算完成的销售单价：

```text
销售单价 = 厂商成本 + 管理员预留利润 + 支付及运营风险
```

平台不校验该组成，也不再次加毛利。

### 6.2 积分换算

复用系统设置中的：

```text
billing_pricing.cny_per_point
```

含义为“每积分对应多少 CNY”。例如：

```text
1 积分 = 0.01 CNY
cny_per_point = 0.01

任务销售价 = 5.26 CNY
原始积分 = 5.26 / 0.01 = 526
```

视频后台不得再次展示可编辑积分汇率，只能只读显示当前生效值并链接到系统设置。

### 6.3 混合路由锁价

对本次请求逐个筛选可用候选，并用候选真实模型自己的费率计算：

```text
路由自动销售价 CNY = max(所有可用候选模型报价 CNY)
原始积分 = 路由自动销售价 CNY / cny_per_point
步长积分 = ceil(原始积分 / rounding_step_points) x rounding_step_points
最终单结果积分 = max(minimum_task_points, 步长积分)
```

生成前将最终积分写入报价 Token 和任务快照。任务成功后按快照结算，实际执行了哪个候选模型都不改变用户价格。

## 7. 厂商费率配置

### 7.1 公共字段

管理员只操作：

1. 厂商：根据真实账号适配器自动识别，只读。
2. 币种：固定 CNY，只读。
3. 厂商专属销售费率。
4. 启用状态。

费率保存后自动生成新版本并立即生效。版本号、生效时间、schema 版本和官方规则来源属于系统字段，不要求管理员输入。

### 7.2 Seedance 费率

按真实模型能力中支持的输出分辨率生成表格：

| 字段 | 单位 | 规则 |
|---|---|---|
| 输入不含视频销售单价 | CNY/百万 Token | 必填，非负；有效输出费率不得为 0 |
| 输入包含视频销售单价 | CNY/百万 Token | 模型支持视频输入时必填 |

不支持的分辨率不展示。模型不支持视频输入时隐藏“输入包含视频”列。

Seedance 候选报价：

```text
基础预估 Token =
(输入视频时长 + 输出视频时长)
x 输出宽 x 输出高 x 帧率
/ 1024

输入包含视频：
计费 Token = max(基础预估 Token, 官方最低 Token)

输入不含视频：
计费 Token = 基础预估 Token

候选销售价 CNY =
计费 Token / 1,000,000 x 对应销售单价
```

输出宽高、帧率和最低 Token 表由 Seedance 计算器按模型规则版本提供。未知模型或缺少规则的参数组合不得启用。

### 7.3 MiniMax H3 费率

按真实模型能力中支持的输出分辨率生成表格：

| 字段 | 单位 | 默认行为 |
|---|---|---|
| 输出视频销售单价 | CNY/秒 | 必填 |
| 输入视频销售单价 | CNY/秒 | 默认跟随同分辨率输出秒价，可单独修改 |

素材配置：

| 字段 | 单位 | 默认值 |
|---|---|---|
| 输入音频 | - | 免费，只读 |
| 免费输入图片数量 | 张 | 5 |
| 超额图片销售单价 | CNY/张 | 管理员填写 |

MiniMax H3 候选报价：

```text
候选销售价 CNY =
输出时长 x 输出秒价
+ 输入视频总时长 x 输入视频秒价
+ max(0, 输入图片数 - 免费图片数) x 超额图片单价
```

## 8. 路由配置

视频路由只维护：

1. 候选真实模型。
2. 候选参数映射。
3. 最低任务积分，默认 `0`。
4. 积分取整步长，候选值 `1`、`5`、`10`，默认 `1`。
5. 用户可见参数组合和默认参数。
6. 平台最大输出数量，保持现有逻辑。

参数名称一致时不要求管理员填写映射。系统应自动建议常见差异，例如：

```text
路由 720p -> Seedance 720p
路由 720p -> MiniMax 768p
路由 2k   -> MiniMax 2K
```

路由允许展示候选能力的并集，但每次请求只筛选真正支持完整参数和素材条件的候选。某个请求只有 MiniMax 可用时，不应因为路由还绑定了 Seedance 而阻断。

## 9. 数据模型

### 9.1 新增视频真实模型费率版本

建议新增 `video_model_rate_cards`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigint | 主键 |
| `account_model_id` | bigint | 真实模型 ID |
| `provider_code` | varchar | `seedance` 或 `minimax` |
| `pricing_schema` | varchar | `seedance_token_v1` 或 `minimax_h3_second_v1` |
| `rate_version` | int | 单真实模型递增版本 |
| `currency` | varchar | 固定 `CNY` |
| `rate_config` | jsonb | schema 判别后的结构化配置 |
| `source_reference` | varchar | 系统写入官方规则链接 |
| `effective_at` | timestamptz | 生效时间 |
| `enabled` | bool | 是否可用于报价 |
| `created_at/updated_at/deleted_at` | timestamptz | 审计与软删除 |

唯一键：

```text
(account_model_id, rate_version)
```

`rate_config` 必须先反序列化为明确的 Go 类型再保存和使用，禁止接受任意键值。

### 9.2 调整视频路由配置

`video_route_configs`：

1. 删除 `pricing_strategy_id`。
2. 删除 `visible_options.pricing_bindings`。
3. 新增 `candidate_parameter_mappings`。
4. 新增 `minimum_task_points numeric(20,5)`，默认 `0`。
5. 新增 `rounding_step_points numeric(20,5)`，默认 `1`。

### 9.3 删除旧配置结构

破坏性迁移清理或退役：

1. `video_pricing_strategies`
2. `video_price_rules`
3. `video_provider_cost_rules` 从新配置、v2 报价和管理后台退出，但记录暂时保留给升级前的在途 legacy 任务创建后续 attempt
4. 旧组合成本和路由价格绑定

旧策略和销售价格规则在本次迁移中停用并软删除。旧 Provider 成本规则必须保持原状态，直到所有升级前任务终态且超过审计保留期；否则尚未创建下一 attempt 的 legacy 任务将无法取得成本快照。除这条窄兼容读取外，业务接口、后台入口和 v2 运行时依赖必须在本次重构中移除，物理删表另行安排。

### 9.4 保留历史账务

必须保留：

1. `video_tasks.pricing_snapshot`
2. `video_tasks.estimated_points/reserved_points/actual_points`
3. `video_task_items.actual_points`
4. `video_task_attempts.usage_raw/usage_normalized/cost_snapshot`
5. 积分预留、扣除和释放流水

旧任务继续使用原 `pricing_snapshot.sales_rule` 结算；新任务使用 `schema_version=2` 固定报价结算。

## 10. 报价计算器架构

新增统一接口：

```text
ProviderPricingCalculator
├── ValidateRateCard(capability, rateCard)
├── SupportsRequest(capability, request)
├── CalculateCandidateQuote(request, rateCard)
└── NormalizeAuditUsage(providerStatus)
```

实现：

1. `SeedanceTokenCalculator`
2. `MiniMaxH3SecondCalculator`

计算器注册表以 `pricing_schema` 为唯一分派依据。禁止根据任意 JSON 字段猜测公式。

## 11. 报价与任务流程

```text
用户请求报价
  -> 解析路由配置
  -> 获取所有启用候选真实模型
  -> 按能力和参数映射筛选可用候选
  -> 加载每个候选的有效费率版本
  -> 调用对应厂商计算器
  -> 取最高候选 CNY
  -> 读取全局 cny_per_point
  -> 应用最低积分与步长取整
  -> 签发报价 Token
  -> 创建任务并冻结固定报价
  -> Provider 执行
  -> 成功结果按固定单结果积分结算
  -> 失败结果释放对应预留积分
```

### 11.1 多结果任务

保持现有平台拆分逻辑：

```text
预留积分 = 单结果积分 x 请求数量
最终积分 = 单结果积分 x 成功结果数量
释放积分 = 预留积分 - 最终积分
```

Provider 原生最大 `n` 仍限制为 1-10；不改变平台将 `n > Provider 原生最大值` 拆成多个并行任务的现有行为。

### 11.2 报价时选择与执行候选

用户价格取所有可用候选的最高值，实际路由仍按优先级、权重和健康状态选择执行候选。报价 Token 必须保存候选集合和费率版本摘要；执行候选变化不重新定价。

## 12. 新任务报价快照

示例：

```json
{
  "schema_version": 2,
  "quote_mode": "route_candidate_max_fixed",
  "cny_per_point": "0.01000",
  "minimum_task_points": "0.00000",
  "rounding_step_points": "1.00000",
  "unit_points": "526.00000",
  "estimated_points": "1052.00000",
  "candidate_quotes": [
    {
      "route_candidate_id": 11,
      "account_model_id": 101,
      "provider_code": "seedance",
      "model_code": "doubao-seedance-2.0",
      "pricing_schema": "seedance_token_v1",
      "rate_version": 3,
      "estimated_cny": "5.26000",
      "calculation": {
        "resolution": "720p",
        "input_contains_video": false,
        "estimated_tokens": "105200"
      }
    },
    {
      "route_candidate_id": 22,
      "account_model_id": 205,
      "provider_code": "minimax",
      "model_code": "MiniMax-H3",
      "pricing_schema": "minimax_h3_second_v1",
      "rate_version": 2,
      "estimated_cny": "4.00000",
      "calculation": {
        "resolution": "768p",
        "output_seconds": "5.000",
        "input_image_count": 2
      }
    }
  ],
  "highest_quote_account_model_id": 101
}
```

任务快照还必须保存路由配置版本、能力版本和费率版本摘要，用于报价 Token 失效判断。

## 13. Provider 实际用量审计

1. Seedance 优先记录 `usage.completion_tokens`，`total_tokens` 仅作兼容回退。
2. MiniMax 记录实际输出时长、输入素材统计和 Provider 原始 `usage`。
3. 实际用量不修改用户价格。
4. 销售费率不是采购成本，不能写入 `provider_cost`。
5. 旧任务的 `provider_cost` 保持不变；新任务在没有独立采购成本数据源时留空或使用明确的“未知”语义，不能写 0 冒充准确成本。
6. 后台可提供“预估用量与实际用量差异”指标，帮助管理员调整销售单价。

## 14. API 设计

### 14.1 真实模型费率

建议提供：

```text
GET    /api/ops/admin/v1/video-models/{account_model_id}/rate-cards
POST   /api/ops/admin/v1/video-models/{account_model_id}/rate-cards
DELETE /api/ops/admin/v1/video-models/{account_model_id}/rate-cards/{rate_card_id}
```

创建新费率版本请求只接受：

```json
{
  "expected_rate_version": 2,
  "pricing_schema": "seedance_token_v1",
  "rate_config": {},
  "enabled": true
}
```

币种、Provider、模型、官方来源、生效时间和新版本号由服务端生成。

### 14.2 路由报价配置

视频路由保存接口改为接受：

```json
{
  "minimum_task_points": "0.00000",
  "rounding_step_points": "1.00000",
  "candidate_parameter_mappings": {},
  "visible_combinations": []
}
```

### 14.3 报价试算

提供只读试算接口：

```text
POST /api/ops/admin/v1/video-routes/{route_model_id}/quote-simulation
```

返回所有候选的支持状态、排除原因、CNY 报价、积分报价和最高价来源。该接口与用户正式报价必须复用同一个领域计算器，不能复制公式。

## 15. 后台交互设计

### 15.1 接入账号 - 视频

真实模型编辑器拆为：

1. 模型能力。
2. 销售费率。

根据账号适配器展示 Seedance 或 MiniMax 结构化表单。列表显示：

- 未配置
- 配置不完整
- 可用
- 已停用

启用真实模型时同时验证能力和费率。

### 15.2 路由模型 - 视频

候选表格增加参数映射入口。系统优先自动映射同名参数和已知厂商别名，只要求管理员处理无法自动确认的差异。

页面按公开参数组合展示候选覆盖情况。任意组合无可报价候选时，显示阻断风险并禁止启用。

### 15.3 价格策略 - 视频

视频页改为“视频报价总览”，不再管理独立策略实体：

1. 按路由展示候选数量、费率完整性和启用状态。
2. 编辑最低任务积分和取整步长。
3. 提供报价试算器。
4. 展示候选逐项计算结果与最高价来源。
5. 缺失费率时直接跳转到对应真实模型。

### 15.4 系统设置

继续由“系统设置 -> 积分换算”维护 `cny_per_point`。视频报价总览只读展示当前生效值。

## 16. 错误处理

建议增加稳定错误码：

| 错误码 | 场景 |
|---|---|
| `VIDEO_RATE_CARD_MISSING` | 真实模型没有有效费率 |
| `VIDEO_RATE_CARD_INVALID` | 费率 schema 或字段校验失败 |
| `VIDEO_PRICING_SCHEMA_UNSUPPORTED` | 未注册的厂商计算器 |
| `VIDEO_CANDIDATE_NOT_PRICEABLE` | 候选支持生成但无法报价 |
| `VIDEO_ROUTE_PRICE_UNAVAILABLE` | 本次请求没有任何可报价候选 |
| `VIDEO_QUOTE_STALE` | 路由、能力、费率或积分汇率已变化 |

规则：

1. 单个候选缺失费率时排除该候选并记录后台风险。
2. 所有候选均无法报价时阻止任务创建。
3. 路由存在无法报价的公开组合时禁止启用。
4. 未识别模型不能按零价格运行。
5. 报价后配置变化只使未提交报价失效，不影响已创建任务。

## 17. 破坏性迁移与上线行为

1. 数据库迁移创建新费率表并调整视频路由字段。
2. 停止旧策略、价格规则和组合成本的运行时读写。
3. 清理旧视频策略和销售价格规则；旧 Provider 成本规则仅为在途 legacy 任务暂存。
4. 保留账号、真实模型、能力、路由候选和历史任务。
5. 将所有现有视频路由置为停用。
6. 管理后台显示全局重配提示和逐路由缺失项。
7. 管理员为真实模型填写销售费率、确认映射和路由规则后重新启用。
8. 正在执行的旧任务继续按旧 `pricing_snapshot` 完成结算。

不得尝试把旧 `combination.cost_cny` 自动猜测为 Token 或秒数销售费率。

## 18. 测试方案

### 18.1 Seedance 计算器

1. 无输入视频 Token 公式。
2. 有输入视频时选择正确费率。
3. 最低 Token 规则。
4. 各分辨率、比例、时长和帧率映射。
5. 未知模型、未知分辨率和缺失规则拒绝启用。
6. `completion_tokens` 优先于 `total_tokens` 的审计归一化。

官方示例基线：

```text
Seedance 2.0，720P，5 秒，无输入视频，46 CNY/百万 Token
预估 Token = 108,000
CNY = 4.968
当 cny_per_point=0.01、步长=1 时，积分=497

Seedance 2.5，720P，5 秒，无输入视频，70 CNY/百万 Token
CNY = 7.56
积分=756
```

### 18.2 MiniMax H3 计算器

1. 768P、2K 输出秒价。
2. 5 张图片免费边界。
3. 超额图片按张收费。
4. 输入视频按对应分辨率秒价。
5. 输入音频免费。

官方示例基线：

```text
768P，5 秒，2 张图片，0.50 CNY/秒
CNY = 2.50
积分=250

768P，5 秒，7 张图片，超额图片 0.20 CNY/张
CNY = 2.50 + 0.40 = 2.90
积分=290
```

### 18.3 混合路由

1. 多候选按能力筛选。
2. 720P 到 768P 参数映射。
3. 取候选最高 CNY。
4. 最低任务积分。
5. 1、5、10 步长向上取整。
6. 同一路由选择不同执行候选仍按锁定价格结算。
7. 单个候选缺失费率时排除；全部缺失时阻断。

### 18.4 任务结算

1. 单结果成功。
2. 多结果全部成功。
3. 多结果部分成功并释放失败结果积分。
4. 全部失败不扣费。
5. 重试不重新报价。
6. 实际 Provider 用量不修改用户积分。

### 18.5 迁移与兼容

1. 旧价格配置被清理。
2. 旧视频路由被停用。
3. 账号、真实模型、能力和候选关系保留。
4. 旧任务继续按旧快照结算。
5. 历史积分流水不变化。
6. 新任务使用 v2 快照。

### 18.6 工程验证

实施完成后至少运行：

```bash
./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
./scripts/workflow/api-smoke.sh
```

并补充后台真实交互测试：保存两家费率、混合路由映射、报价试算、路由启用阻断和用户正式报价。

## 19. 实施顺序

1. 新费率 schema、领域类型和厂商计算器。
2. 混合路由报价、固定报价快照和结算兼容。
3. 破坏性迁移与旧运行时依赖清理。
4. 接入账号页厂商结构化费率表单。
5. 路由候选参数映射。
6. 视频报价总览和试算器。
7. 输入素材媒体类型、时长和 Provider 序列化补齐。
8. 单元、集成、迁移、前端和 API smoke 测试。
9. 代码审查、上线演练和管理员重配说明。

## 20. 风险与约束

1. Seedance 最低 Token 规则可能随模型更新，必须带规则版本和官方来源，不能静默修改历史报价。
2. 官方临时折扣不应硬编码为永久价格。管理员填写销售单价，平台只提供官方参考链接。
3. 混合路由最高价会让实际执行较便宜模型时获得更高毛利，这是已确认的稳定锁价策略。
4. 若所有候选费率都由管理员填得低于采购成本，平台不会自动阻止亏损；这是取消净收入和目标毛利后的明确运营责任。
5. 素材时长必须来自可信资产元数据，不能使用前端提交值作为计费依据。
6. 参数映射必须同时用于能力筛选、报价和 Provider 请求，不能三处各自转换。
7. 新任务不能继续依赖旧 `SalesRule`；旧任务兼容分支需保留到所有旧任务完成且超过审计保留期。

## 21. 验收标准

1. 管理员配置 Seedance 真实模型时，只需填写支持分辨率对应的两类百万 Token 销售单价。
2. 管理员配置 MiniMax H3 时，只需填写各分辨率输出/输入视频秒价、免费图片数量和超额图片单价。
3. 视频页面不再出现支付费率、积分净收入、目标毛利、平台成本、成本缓冲或预留倍率。
4. 路由可同时绑定 Seedance 和 MiniMax，并正确完成参数映射和候选筛选。
5. 用户报价取可用候选最高价，生成前锁定，执行候选变化不改变价格。
6. 系统只使用全局 `cny_per_point` 换算积分。
7. 最低积分和取整步长正确生效。
8. 历史任务、账务和积分流水不受破坏性价格配置迁移影响。
9. 官方示例、边界条件、混合路由、部分成功和迁移测试全部通过。
