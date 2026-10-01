# 统一媒体生成错误码（Media Generation Error Codes）

- 状态：生效中
- 日期：2026-09-29
- 范围：图片生成、视频生成任务的用户侧错误语义
- 实现位置：`internal/service/mediaerrors`

## 1. 目标

不同模型厂商（MiniMax、火山方舟 Ark/Seedance、openai_compatible 兼容上游等）的错误码体系互不相同，且原始报文可能包含签名 URL、请求 ID、上游内部路径等不宜透出的信息。本平台在**用户侧读取路径**将上游错误统一转义为下述平台错误码 + 中文文案：

- 屏蔽厂商差异：用户与开放 API 只看到 `GENERATION_*` 统一码；
- 防泄露：原始厂商码与报文仅存于数据库（管理后台可查），绝不透出给用户；
- 开放 API 基础：`GENERATION_*` 码是稳定契约，后续开放接口直接复用，语义变更需在本文档记录版本。

## 2. 统一错误码合集

### 2.1 生成结果类（任务级，落在 task/item 上的终态错误）

| 统一码 | 含义 | 用户文案 |
| --- | --- | --- |
| `GENERATION_CONTENT_BLOCKED_INPUT` | 提示词或参考素材未通过上游安全审核 | 提示词或参考素材未通过安全审核，请调整内容后重试 |
| `GENERATION_CONTENT_BLOCKED_OUTPUT` | 生成结果未通过上游安全审核 | 生成内容未通过安全审核，请调整提示词或素材后重试（避免知名 IP、真实人物等元素） |
| `GENERATION_REQUEST_REJECTED` | 上游拒绝该请求（参数/内容校验未通过） | 生成任务被上游拒绝，请调整参数或内容后重试 |
| `GENERATION_PROVIDER_BUSY` | 上游限流/繁忙 | 上游服务繁忙，请稍后重试 |
| `GENERATION_PROVIDER_TIMEOUT` | 上游处理超时 | 上游处理超时，请稍后重试 |
| `GENERATION_PROVIDER_UNAVAILABLE` | 上游服务异常/不可用 | 上游服务异常，请稍后重试 |
| `GENERATION_PROVIDER_CAPACITY_EXCEEDED` | 上游配额/余额不足 | 上游服务容量不足，请稍后重试 |
| `GENERATION_PROVIDER_AUTH_FAILED` | 上游鉴权失败（平台配置类问题） | 服务暂时不可用，请联系客服 |
| `GENERATION_MODEL_UNAVAILABLE` | 模型不存在/未开通/已下线 | 所选模型暂时不可用，请稍后重试或更换模型 |
| `GENERATION_ARTIFACT_FAILED` | 结果文件获取或保存失败 | 作品保存失败，系统会自动重试 |
| `GENERATION_INPUT_ASSET_UNAVAILABLE` | 输入素材无法读取 | 输入素材暂时无法读取，请重新选择素材后重试 |

兜底：未识别的上游错误 → `GENERATION_PROVIDER_UNAVAILABLE` + 通用文案「生成未能完成，请稍后重试；若多次失败，请联系客服并提供任务 ID」。

### 2.2 平台自有码（原样透出，不做转义）

以下错误本身即为平台语义，直接作为统一码使用：

`BILLING_INSUFFICIENT_POINTS`、`IMAGE_CAPABILITY_MISMATCH`、`VIDEO_CAPABILITY_MISMATCH`、`NOT_FOUND`、`MODEL_ROUTE_NOT_FOUND`、`MODEL_ROUTE_UNAVAILABLE`、`MODEL_ROUTE_NO_CANDIDATE`、`ROUTE_MODEL_PRICE_MISSING`、`VIDEO_FIELD_INVALID`、`IMAGE_*` 输入校验族、`UPSTREAM_CONTENT_BLOCKED`（同步 API 响应族）等，全量见 `pkg/errs/codes.go`。

## 3. 厂商码 → 统一码映射

### 3.1 MiniMax（platform.minimaxi.com/docs/api-reference/errorcode）

| 厂商码 | 统一码 |
| --- | --- |
| 1000 / 1024 / 1033 | `GENERATION_PROVIDER_UNAVAILABLE` |
| 1001 | `GENERATION_PROVIDER_TIMEOUT` |
| 1002 / 1041 / 2045 | `GENERATION_PROVIDER_BUSY` |
| 1004 / 2049 | `GENERATION_PROVIDER_AUTH_FAILED` |
| 1008 / 1039 / 2056 | `GENERATION_PROVIDER_CAPACITY_EXCEEDED` |
| 1026 | `GENERATION_CONTENT_BLOCKED_INPUT` |
| 1027 | `GENERATION_CONTENT_BLOCKED_OUTPUT` |
| 2013 | `GENERATION_REQUEST_REJECTED` |
| 未文档化的审核变体（如 `OutputVideoSensitiveContentDetected.PolicyViolation`） | 按码内 `sensitive/policy` 标记归入 `GENERATION_CONTENT_BLOCKED_OUTPUT`（若含 input/输入 标记则归 INPUT） |

### 3.2 火山方舟 Ark / Seedance（ark.volcengine.com/docs/ark/error-codes）

| 厂商码 | 统一码 |
| --- | --- |
| `InputTextSensitiveContentDetected` | `GENERATION_CONTENT_BLOCKED_INPUT` |
| `OutputVideoSensitiveContentDetected`（含 `.PolicyViolation` 等后缀）、`OutputTextSensitiveContentDetected`、`OutputAudioSensitiveContentDetected`、`DataInspectionFailed` | `GENERATION_CONTENT_BLOCKED_OUTPUT` |
| `InvalidParameter`（含任意 `.Suffix` 变体） | `GENERATION_REQUEST_REJECTED` |
| `InvalidEndpointOrModel.NotFound` | `GENERATION_MODEL_UNAVAILABLE` |
| `ModelNotOpen` / `ModelNotFound` | `GENERATION_MODEL_UNAVAILABLE` |
| `ModelTimeout` | `GENERATION_PROVIDER_TIMEOUT` |
| `InternalServiceError` | `GENERATION_PROVIDER_UNAVAILABLE` |
| `FlowLimitExceeded` | `GENERATION_PROVIDER_BUSY` |
| `QuotaExceeded` / `ModelQuotaExceeded` / `AccountArrears` | `GENERATION_PROVIDER_CAPACITY_EXCEEDED` |
| `AccessDenied` / `AccountIllegallyBlocked` | `GENERATION_PROVIDER_AUTH_FAILED` |

点分变体（`InvalidParameter.UnsupportedImageFormat` 等）按「逐级去掉 `.后缀` 再查表」的前缀匹配归并。

### 3.3 openai_compatible 兼容上游

| 厂商码 | 统一码 |
| --- | --- |
| `content_policy_violation` / `moderation_blocked` / `content_filter` / `safety_system` | `GENERATION_CONTENT_BLOCKED_OUTPUT` |
| `rate_limit_exceeded` / `rate_limit_error` / `requests_too_high` / `overloaded_error` | `GENERATION_PROVIDER_BUSY` |
| `insufficient_quota` / `billing_hard_limit_reached` / `quota_exceeded` | `GENERATION_PROVIDER_CAPACITY_EXCEEDED` |
| `invalid_api_key` / `authentication_error` / `unauthorized` / `account_deactivated` | `GENERATION_PROVIDER_AUTH_FAILED` |
| `model_not_found` / `invalid_model_error` | `GENERATION_MODEL_UNAVAILABLE` |
| `timeout` / `request_timeout` / `APITimeoutError` | `GENERATION_PROVIDER_TIMEOUT` |
| `server_error` / `api_error` / `internal_error` | `GENERATION_PROVIDER_UNAVAILABLE` |
| `invalid_request_error` | `GENERATION_REQUEST_REJECTED` |

### 3.4 Worker 合成码（internal/worker/video）

| 合成码 | 统一码 |
| --- | --- |
| `provider_submit_failed` | `GENERATION_REQUEST_REJECTED` |
| `provider_unavailable` | `GENERATION_PROVIDER_UNAVAILABLE` |
| `provider_input_unavailable` | `GENERATION_INPUT_ASSET_UNAVAILABLE` |
| `provider_artifact_missing` / `artifact_persist_failed` / `usage_normalization_failed` | `GENERATION_ARTIFACT_FAILED` |

## 4. 转义规则（优先级从高到低）

1. 码精确命中映射表 → 统一码 + 专属文案；
2. 码含审核标记（`sensitive`/`moderation`/`policy`）→ 审核族；码点分变体逐级前缀查表；
3. 码未命中时按消息关键词归类（敏感/限流/超时/配额/鉴权/参数…）；
4. 消息含泄露标记（签名 URL、request id、http 链接）→ 通用兜底；
5. 平台自撰中文消息（汉字占主导）→ 保留原文与原码；
6. 其余 → `GENERATION_PROVIDER_UNAVAILABLE` + 通用文案。

已知局限：消息关键词归类无法区分输入/输出审核时，默认归 `GENERATION_CONTENT_BLOCKED_OUTPUT`；仅当码或消息含 `input/输入/prompt/提示词/素材` 标记时才归 INPUT。

## 5. 透出范围

| 面 | 错误码 | 消息 |
| --- | --- | --- |
| 用户 API（视频任务、图片任务、画布 run、SSE） | 统一码 | 中文文案 |
| 管理后台（video-tasks、attempts、原始表） | 厂商原始码 | 原始报文 |
| 数据库 | 厂商原始码 | 原始报文（供审计与支持排查） |

## 6. 开放 API 注意事项

- `GENERATION_*` 与 2.2 节平台码即为对外契约；新增码必须先在本文档登记；
- 文案可随产品迭代调整，不构成契约；码语义保持向后兼容；
- 后续开放接口建议在错误响应中同时给出 `task_id` 与统一码，便于用户自助与客服精确查询。
