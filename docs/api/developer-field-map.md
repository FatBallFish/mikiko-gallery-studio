# 开发者 API 字段映射与实际操作

完整请求类型与枚举见[接口参考](../../web/docs/public/openapi/openapi.yaml)。下表保留实际操作及 Go JSON 字段，便于核对客户端类型。

- `omitempty` 表示字段可能省略；指针可能为 null。
- map 是动态对象，内部键不是固定枚举。
- 任务和资产始终检查归属。
- `MIKIKO_API_KEY` 使用创建或轮换时一次显示的 SK（`sk-`）；AK（`ak-`）只作标识。MCP 使用独立 OAuth。

## 路径和请求位置

原生 BaseURL 为 `https://mikiko.studio/api/open/v1`。下表 REST 路径均接在此地址后。

- 路径 ID 覆盖 body/query 同名字段；`Idempotency-Key` 请求头覆盖 body 字段。
- GET 参数用 query，写请求用 JSON；未知字段会拒绝，不在 URL 传令牌。
- 创建幂等键为 1–128 字符。相同身份、键和业务输入重放同一任务，冲突返回 409。
- 报价变化后先预检，不盲目换键创建。

| REST（省略前缀） | MCP operation | scope | 实际 data |
| --- | --- | --- | --- |
| GET /me | get_current_user | account:read | id,email,nickname,status,group_code |
| GET /me/balance | get_balance | account:read | BalanceSummary |
| GET /image-models; /video-models | list_image_models; list_video_models | models:read | 模型能力响应，按实际部署判断选项 |
| GET /image-tasks; /video-tasks | list_image_tasks; list_video_tasks | images:read; videos:read | items,next_cursor；query project_id,status,cursor,limit（默认20，上限100） |
| GET /image-tasks/{id}; /video-tasks/{id} | get_image_task; get_video_task | images:read; videos:read | 各自 Task |
| POST /image-tasks/preflight; /video-tasks/preflight | preflight_image_task; preflight_video_task | images:write; videos:write | EstimateResult+quote_token,expires_at；视频Estimate |
| POST /image-tasks; /video-tasks | create_image_task; create_video_task | images:write; videos:write | 图片Task；视频{task:Task,replayed:boolean} |
| GET /image-tasks/{id}/layers | get_image_task_layers | images:read | version,task_id,operation,layers[{image_id,width,height,format,layer,access_operation,access_input}] |
| POST /image-tasks/{task_id}/results/{image_id}/access | get_image_result_access | images:read | TemporaryMediaAccess；body purpose |
| POST /video-tasks/{task_id}/results/{asset_id}/access | get_video_result_access | videos:read | TemporaryMediaAccess；body purpose |
| POST /video-tasks/{id}/cancel | cancel_video_task | videos:delete | Task；body idempotency_key |
| GET /assets | list_assets | assets:read | items,next_cursor；project_id,media_type,source_type,group_name,status,keyword,sort_by,sort_order,cursor,limit |
| GET /assets/{id} | get_asset | assets:read | Asset |
| PATCH /assets/{id} | update_asset | assets:write | Asset；name,group_id,group_name,project_id,expected_version |
| DELETE /assets/{id} | delete_asset | assets:delete | Asset；expected_version |
| POST /assets/{id}/access | get_asset_access | assets:read | TemporaryMediaAccess；purpose |
| POST /uploads | create_upload | uploads:write | UploadSession；project_id,filename,media_type,mime_type,size_bytes,checksum?,group_id?/group_name?,idempotency_key |
| GET /uploads/{id} | get_upload | uploads:read | UploadSession |
| POST /uploads/{id}/parts/{part_number}/url | get_upload_part_url | uploads:write | MultipartPartTarget；checksum |
| PUT /uploads/{id}/parts/{part_number} | 无，HTTP传输 | uploads:write | CompletedPart；二进制body、Content-Length、X-Checksum-SHA256（64 hex或32-byte base64），每片≤32MiB |
| POST /uploads/{id}/complete | complete_upload | uploads:write | Asset；parts:CompletedPart[] |
| POST /uploads/{id}/abort | abort_upload | uploads:delete | {aborted:boolean} |
| GET /projects | list_projects | projects:read | {items:Project[]} |
| GET /projects/{id} | get_project | projects:read | Project |
| POST /projects | create_project | projects:write | Project；name,idempotency_key |
| PATCH /projects/{id} | update_project | projects:write | Project；name,expected_version |
| DELETE /projects/{id} | delete_project | projects:delete | DeleteResult；expected_version,target_project_id,idempotency_key |
| GET /groups | list_groups | groups:read | {items:Group[]}；project_id |
| GET /groups/{id} | get_group | groups:read | Group；project_id |
| POST /groups | create_group | groups:write | Group；project_id,name |
| PATCH /groups/{id} | update_group | groups:write | Group；project_id,name,expected_version |
| DELETE /groups/{id} | delete_group | groups:delete | {deleted:boolean}；project_id,expected_version |

GET /image-tasks/{id}/events 与视频对应路由是 SSE 观察接口，scope同详情，无独立 MCP tool。事件 `task.status` data为完整Task，授权或详情读取失败发`error`。终态关闭；event id为内容摘要，不承诺历史重放。临时访问只保存资产/任务ID，过期重新请求，不把链接永久保存或借用浏览器ZIP权限。

## 输入与输出字段

- 图片创建与预检字段如下 ImageInput；创建额外需要幂等键与有效报价。
- native_options嵌套以API reference为准：seed、negative_prompt、watermark、annotations[{reference_index,label?,point?/bbox?}]、qwen{prompt_extend,prompt_extend_mode,enable_thinking}、seedream{optimize_prompt_mode}。
- 输入坐标0..999，输出layer.bounding_box.normalized为0..1000，不能混用。
- 视频输入见CreateRequest，另接受idempotency_key；变量{name,value}、绑定{name,asset_id}、inputs{asset_id,role,ordinal}。

### internal/service/openplatform/tasks.go · ImageInput

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `operation` | `nativeimage.Operation` | 可省略 |
| `native_options` | `*nativeimage.Options` | 可省略 |
| `mask_asset_id` | `string` | 序列化字段 |
| `project_id` | `string` | 序列化字段 |
| `abstract_model` | `string` | 序列化字段 |
| `route_model_code` | `string` | 序列化字段 |
| `task_type` | `string` | 序列化字段 |
| `prompt` | `string` | 序列化字段 |
| `prompt_variables` | `[]image.PromptVariableInput` | 序列化字段 |
| `reference_bindings` | `[]image.PromptReferenceInput` | 序列化字段 |
| `reference_asset_ids` | `[]string` | 序列化字段 |
| `negative_prompt` | `string` | 序列化字段 |
| `size_mode` | `string` | 序列化字段 |
| `requested_size` | `string` | 序列化字段 |
| `base_resolution` | `string` | 序列化字段 |
| `quality` | `string` | 序列化字段 |
| `output_format` | `string` | 序列化字段 |
| `background` | `string` | 序列化字段 |
| `output_compression` | `int` | 序列化字段 |
| `moderation` | `string` | 序列化字段 |
| `aspect_ratio` | `string` | 序列化字段 |
| `output_image_count` | `int` | 序列化字段 |
| `reference_strength` | `int` | 序列化字段 |
| `seed` | `*int64` | 序列化字段 |
| `capability_version` | `string` | 序列化字段 |
| `quote_token` | `string` | 序列化字段 |
| `idempotency_key` | `string` | 序列化字段 |

### internal/domain/imagetask/types.go · Task

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `operation` | `nativeimage.Operation` | 可省略 |
| `native_options` | `*nativeimage.Options` | 可省略 |
| `project_id` | `string` | 序列化字段 |
| `project` | `*domainproject.Snapshot` | 可省略 |
| `id` | `string` | 序列化字段 |
| `status` | `string` | 序列化字段 |
| `progress_stage` | `string` | 可省略 |
| `progress_message` | `string` | 可省略 |
| `provider` | `string` | 可省略 |
| `provider_model_id` | `int64` | 可省略 |
| `provider_cost` | `string` | 可省略 |
| `gross_margin` | `string` | 可省略 |
| `fallback_count` | `int` | 可省略 |
| `route_snapshot_version` | `string` | 可省略 |
| `abstract_model` | `string` | 序列化字段 |
| `route_model_code` | `string` | 可省略 |
| `route_model_id` | `int64` | 可省略 |
| `account_model_id` | `int64` | 可省略 |
| `model_account_id` | `int64` | 可省略 |
| `upstream_model_code` | `string` | 可省略 |
| `effective_multiplier` | `string` | 可省略 |
| `charged_points` | `string` | 可省略 |
| `task_type` | `string` | 序列化字段 |
| `prompt` | `string` | 可省略 |
| `size_mode` | `string` | 可省略 |
| `aspect_ratio` | `string` | 可省略 |
| `requested_size` | `string` | 可省略 |
| `resolved_width` | `int` | 可省略 |
| `resolved_height` | `int` | 可省略 |
| `base_resolution` | `string` | 序列化字段 |
| `quality` | `string` | 序列化字段 |
| `output_format` | `string` | 可省略 |
| `background` | `string` | 可省略 |
| `output_compression` | `int` | 可省略 |
| `moderation` | `string` | 可省略 |
| `response_mode` | `string` | 可省略 |
| `save_policy` | `string` | 可省略 |
| `requested_output_image_count` | `int` | 序列化字段 |
| `reference_image_count` | `int` | 序列化字段 |
| `reference_asset_ids` | `[]string` | 可省略 |
| `mask_asset_id` | `string` | 可省略 |
| `reference_strength` | `int` | 可省略 |
| `seed` | `*int64` | 可省略 |
| `estimated_points` | `string` | 可省略 |
| `actual_points` | `string` | 可省略 |
| `lease_owner` | `string` | 可省略 |
| `lease_expires_at` | `*time.Time` | 可省略 |
| `error_code` | `string` | 可省略 |
| `attempts` | `[]Attempt` | 可省略 |
| `error_message` | `string` | 可省略 |
| `provider_request_id` | `string` | 可省略 |
| `upstream_succeeded_at` | `*time.Time` | 可省略 |
| `artifact_recovery` | `ArtifactRecovery` | 可省略 |
| `results` | `[]provider.ImageResult` | 可省略 |
| `created_at` | `time.Time` | 序列化字段 |
| `updated_at` | `time.Time` | 序列化字段 |

### internal/domain/imagetask/types.go · Attempt

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `provider` | `string` | 可省略 |
| `adapter_type` | `string` | 可省略 |
| `account_model_id` | `int64` | 可省略 |
| `model_account_id` | `int64` | 可省略 |
| `model_code` | `string` | 可省略 |
| `source_size_mode` | `string` | 可省略 |
| `outbound_size` | `string` | 可省略 |
| `returned_width` | `int` | 可省略 |
| `returned_height` | `int` | 可省略 |
| `size_diagnostic` | `string` | 可省略 |
| `provider_request_id` | `string` | 可省略 |
| `requested_image_count` | `int` | 可省略 |
| `returned_image_count` | `int` | 可省略 |
| `status` | `string` | 可省略 |
| `error` | `string` | 可省略 |
| `error_code` | `string` | 可省略 |
| `error_message` | `string` | 可省略 |
| `error_detail` | `map[string]any` | 可省略 |
| `started_at` | `*time.Time` | 可省略 |
| `finished_at` | `*time.Time` | 可省略 |

### internal/domain/imagetask/types.go · ArtifactRecovery

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `status` | `string` | 可省略 |
| `attempt_count` | `int` | 可省略 |
| `next_retry_at` | `*time.Time` | 可省略 |
| `last_diagnostic` | `ArtifactDiagnostic` | 可省略 |
| `diagnostics` | `[]ArtifactDiagnostic` | 可省略 |
| `storage_config_id` | `string` | 可省略 |
| `storage_driver` | `string` | 可省略 |
| `storage_bucket` | `string` | 可省略 |
| `object_keys` | `[]string` | 可省略 |
| `storage_version` | `int64` | 可省略 |

### internal/domain/imagetask/types.go · ArtifactDiagnostic

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `code` | `string` | 可省略 |
| `stage` | `string` | 可省略 |
| `attempt` | `int` | 可省略 |
| `url_host` | `string` | 可省略 |
| `url_path` | `string` | 可省略 |
| `http_status` | `int` | 可省略 |
| `content_type` | `string` | 可省略 |
| `content_length` | `int64` | 可省略 |
| `bytes_read` | `int64` | 可省略 |
| `duration_ms` | `int64` | 可省略 |
| `storage_config_id` | `string` | 可省略 |
| `storage_version` | `int64` | 可省略 |
| `retryable` | `bool` | 序列化字段 |
| `cause` | `string` | 可省略 |
| `started_at` | `time.Time` | 可省略 |
| `finished_at` | `time.Time` | 可省略 |

### internal/provider/contracts.go · ImageResult

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `layer` | `*nativeimage.LayerMetadata` | 可省略 |
| `name` | `string` | 可省略 |
| `media_asset_id` | `string` | 可省略 |
| `id` | `string` | 可省略 |
| `project_id` | `string` | 可省略 |
| `project` | `*domainproject.Snapshot` | 可省略 |
| `url` | `string` | 可省略 |
| `download_url` | `string` | 可省略 |
| `preview_expires_at` | `*time.Time` | 可省略 |
| `download_expires_at` | `*time.Time` | 可省略 |
| `b64_json` | `string` | 可省略 |
| `revised_prompt` | `string` | 可省略 |
| `format` | `string` | 可省略 |
| `mime_type` | `string` | 可省略 |
| `file_size_bytes` | `int64` | 序列化字段 |
| `width` | `int` | 序列化字段 |
| `height` | `int` | 序列化字段 |
| `sha256` | `string` | 可省略 |
| `storage_config_id` | `string` | 可省略 |
| `object_key` | `string` | 可省略 |
| `storage_driver` | `string` | 可省略 |
| `image_group` | `string` | 可省略 |
| `visibility_status` | `string` | 可省略 |
| `review_reason` | `string` | 可省略 |
| `published_at` | `*time.Time` | 可省略 |

### internal/domain/nativeimage/contracts.go · Options

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `seed` | `*int64` | 可省略 |
| `negative_prompt` | `string` | 可省略 |
| `watermark` | `*bool` | 可省略 |
| `qwen` | `*QwenOptions` | 可省略 |
| `seedream` | `*SeedreamOptions` | 可省略 |
| `annotations` | `[]Annotation` | 可省略 |

### internal/domain/billing/types.go · EstimateResult

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `capability_version` | `string` | 可省略 |
| `base_resolution` | `string` | 序列化字段 |
| `resolved_size` | `*string` | 序列化字段 |
| `estimated_points` | `string` | 序列化字段 |
| `charged_points` | `string` | 可省略 |
| `display_points` | `string` | 可省略 |
| `sufficient` | `bool` | 序列化字段 |
| `balance` | `*BalanceSummary` | 可省略 |
| `insufficient_points` | `string` | 序列化字段 |
| `user_group_multiplier` | `string` | 序列化字段 |
| `requested_output_image_count` | `int` | 序列化字段 |
| `reference_image_count` | `int` | 序列化字段 |

### internal/domain/billing/types.go · PricingSnapshot

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `route_image` | `*NativeImagePricingSnapshot` | 可省略 |
| `native_image` | `*NativeImagePricingSnapshot` | 可省略 |
| `abstract_model` | `string` | 序列化字段 |
| `route_model_code` | `string` | 可省略 |
| `task_type` | `string` | 序列化字段 |
| `size_mode` | `string` | 可省略 |
| `aspect_ratio` | `string` | 可省略 |
| `base_resolution` | `string` | 序列化字段 |
| `quality` | `string` | 可省略 |
| `output_format` | `string` | 可省略 |
| `background` | `string` | 可省略 |
| `output_compression` | `int` | 可省略 |
| `moderation` | `string` | 可省略 |
| `requested_size` | `string` | 可省略 |
| `requested_output_image_count` | `int` | 序列化字段 |
| `reference_image_count` | `int` | 序列化字段 |
| `user_group_code` | `string` | 可省略 |
| `user_group_multiplier` | `string` | 序列化字段 |
| `base_unit_points` | `string` | 序列化字段 |
| `task_multiplier` | `string` | 序列化字段 |
| `reference_extra_multiplier` | `string` | 序列化字段 |
| `estimated_points` | `string` | 序列化字段 |

### internal/domain/billing/types.go · BalanceSummary

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `available_points` | `string` | 序列化字段 |
| `frozen_points` | `string` | 序列化字段 |
| `trial_points` | `string` | 可省略 |
| `subscription_points` | `string` | 可省略 |
| `gift_points` | `string` | 可省略 |
| `recharge_points` | `string` | 可省略 |
| `buckets` | `[]BalanceBucket` | 可省略 |
| `user_group_multiplier` | `string` | 序列化字段 |
| `cny_per_point` | `string` | 序列化字段 |
| `active_subscription` | `*UserSubscriptionSummary` | 可省略 |
| `next_expiring_grant` | `*GrantExpirySummary` | 可省略 |

### internal/domain/billing/types.go · BalanceBucket

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `bucket` | `string` | 序列化字段 |
| `label` | `string` | 可省略 |
| `available_points` | `string` | 序列化字段 |
| `frozen_points` | `string` | 可省略 |
| `expires_at` | `*time.Time` | 可省略 |
| `expire_warning` | `bool` | 序列化字段 |
| `mixed_expiry` | `bool` | 可省略 |
| `source_type` | `string` | 可省略 |
| `sort_order` | `int` | 可省略 |

### internal/service/videotask/service.go · CreateRequest

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `project_id` | `uuid.UUID` | 序列化字段 |
| `quote_token` | `string` | 序列化字段 |
| `route_model_code` | `string` | 序列化字段 |
| `task_type` | `domainvideo.TaskType` | 序列化字段 |
| `prompt_template` | `string` | 序列化字段 |
| `prompt_variables` | `[]VariableBinding` | 序列化字段 |
| `reference_bindings` | `[]ReferenceBinding` | 序列化字段 |
| `inputs` | `[]InputRequest` | 序列化字段 |
| `duration_seconds` | `int` | 序列化字段 |
| `resolution` | `domainvideo.Resolution` | 序列化字段 |
| `aspect_ratio` | `domainvideo.AspectRatio` | 序列化字段 |
| `audio_mode` | `domainvideo.AudioMode` | 序列化字段 |
| `output_count` | `int` | 序列化字段 |

### internal/service/videotask/store.go · Task

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `uuid.UUID` | 序列化字段 |
| `api_key_id` | `int64` | 可省略 |
| `project_id` | `uuid.UUID` | 序列化字段 |
| `source_channel` | `string` | 序列化字段 |
| `source_canvas_id` | `*uuid.UUID` | 可省略 |
| `source_canvas_node_id` | `string` | 可省略 |
| `task_type` | `domainvideo.TaskType` | 序列化字段 |
| `status` | `domainvideo.TaskStatus` | 序列化字段 |
| `progress_stage` | `string` | 序列化字段 |
| `progress_message` | `string` | 可省略 |
| `prompt_template` | `string` | 序列化字段 |
| `prompt_binding_snapshot` | `map[string]any` | 序列化字段 |
| `execution_prompt` | `string` | 序列化字段 |
| `route_model_id` | `int64` | 序列化字段 |
| `route_model_code` | `string` | 序列化字段 |
| `duration_seconds` | `int` | 序列化字段 |
| `resolution` | `domainvideo.Resolution` | 序列化字段 |
| `aspect_ratio` | `domainvideo.AspectRatio` | 序列化字段 |
| `generate_audio` | `bool` | 序列化字段 |
| `requested_output_count` | `int` | 序列化字段 |
| `success_output_count` | `int` | 序列化字段 |
| `estimated_points` | `string` | 序列化字段 |
| `reserved_points` | `string` | 序列化字段 |
| `actual_points` | `string` | 序列化字段 |
| `pricing_snapshot` | `map[string]any` | 序列化字段 |
| `routing_snapshot` | `map[string]any` | 序列化字段 |
| `settlement_status` | `string` | 序列化字段 |
| `items` | `[]Item` | 序列化字段 |
| `inputs` | `[]Input` | 序列化字段 |
| `version` | `int64` | 序列化字段 |
| `created_at` | `time.Time` | 序列化字段 |
| `updated_at` | `time.Time` | 序列化字段 |
| `started_at` | `*time.Time` | 可省略 |
| `finished_at` | `*time.Time` | 可省略 |

### internal/service/videotask/store.go · Item

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `uuid.UUID` | 序列化字段 |
| `ordinal` | `int` | 序列化字段 |
| `status` | `domainvideo.ItemState` | 序列化字段 |
| `stage` | `string` | 序列化字段 |
| `result_asset_id` | `*uuid.UUID` | 可省略 |
| `actual_output_seconds` | `string` | 序列化字段 |
| `actual_points` | `string` | 序列化字段 |
| `error_code` | `string` | 可省略 |
| `error_message` | `string` | 可省略 |
| `next_action_at` | `*time.Time` | 可省略 |
| `version` | `int64` | 序列化字段 |

### internal/service/videotask/store.go · Input

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `uuid.UUID` | 序列化字段 |
| `asset_id` | `uuid.UUID` | 序列化字段 |
| `role` | `string` | 序列化字段 |
| `ordinal` | `int` | 序列化字段 |
| `asset_snapshot` | `map[string]any` | 序列化字段 |

### internal/service/videotask/quote.go · Estimate

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `route_model_id` | `int64` | 可省略 |
| `route_model_code` | `string` | 序列化字段 |
| `capability_version` | `string` | 序列化字段 |
| `config_version` | `string` | 序列化字段 |
| `price_version` | `string` | 序列化字段 |
| `unit_points` | `string` | 序列化字段 |
| `estimated_points` | `string` | 序列化字段 |
| `max_reserved_points` | `string` | 序列化字段 |
| `quote_token` | `string` | 序列化字段 |
| `expires_at` | `time.Time` | 序列化字段 |

### internal/service/mediaasset/store.go · UploadSession

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `uuid.UUID` | 序列化字段 |
| `user_id` | `int64` | 序列化字段 |
| `project_id` | `uuid.UUID` | 序列化字段 |
| `group_name` | `string` | 序列化字段 |
| `original_filename` | `string` | 序列化字段 |
| `declared_media_type` | `domainmedia.MediaType` | 序列化字段 |
| `declared_mime_type` | `string` | 序列化字段 |
| `declared_size_bytes` | `int64` | 序列化字段 |
| `declared_checksum` | `string` | 可省略 |
| `storage_driver` | `string` | 序列化字段 |
| `part_size` | `int64` | 序列化字段 |
| `part_count` | `int` | 序列化字段 |
| `status` | `string` | 序列化字段 |
| `reserved_bytes` | `int64` | 序列化字段 |
| `actual_bytes` | `int64` | 序列化字段 |
| `completed_parts` | `[]storage.CompletedPart` | 序列化字段 |
| `asset_id` | `*uuid.UUID` | 可省略 |
| `expires_at` | `time.Time` | 序列化字段 |
| `completed_at` | `*time.Time` | 可省略 |

### internal/service/mediaasset/store.go · Asset

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `uuid.UUID` | 序列化字段 |
| `user_id` | `int64` | 序列化字段 |
| `project_id` | `uuid.UUID` | 序列化字段 |
| `legacy_image_id` | `*uuid.UUID` | 可省略 |
| `name` | `string` | 序列化字段 |
| `group_id` | `*uuid.UUID` | 序列化字段 |
| `group_name` | `string` | 序列化字段 |
| `media_type` | `domainmedia.MediaType` | 序列化字段 |
| `source_type` | `string` | 序列化字段 |
| `source_task_kind` | `*string` | 可省略 |
| `source_task_id` | `*uuid.UUID` | 可省略 |
| `source_canvas_id` | `*uuid.UUID` | 可省略 |
| `status` | `string` | 序列化字段 |
| `visibility_status` | `string` | 序列化字段 |
| `storage_driver` | `string` | 序列化字段 |
| `mime_type` | `string` | 序列化字段 |
| `container` | `string` | 可省略 |
| `codec` | `string` | 可省略 |
| `file_size_bytes` | `int64` | 序列化字段 |
| `sha256` | `string` | 可省略 |
| `width` | `*int` | 可省略 |
| `height` | `*int` | 可省略 |
| `duration_ms` | `*int64` | 可省略 |
| `frame_rate_milli` | `*int` | 可省略 |
| `audio_codec` | `string` | 可省略 |
| `channels` | `*int` | 可省略 |
| `sample_rate` | `*int` | 可省略 |
| `version` | `int64` | 序列化字段 |
| `created_at` | `time.Time` | 序列化字段 |
| `updated_at` | `time.Time` | 序列化字段 |
| `deleted_at` | `*time.Time` | 可省略 |

### internal/service/mediaasset/service.go · AccessResult

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `url` | `string` | 序列化字段 |
| `expires_at` | `time.Time` | 序列化字段 |
| `range_supported` | `bool` | 序列化字段 |

### internal/storage/multipart.go · MultipartPartTarget

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `url` | `string` | 序列化字段 |
| `headers` | `map[string]string` | 序列化字段 |
| `expires_at` | `time.Time` | 序列化字段 |

### internal/storage/multipart.go · CompletedPart

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `part_number` | `int` | 序列化字段 |
| `etag` | `string` | 序列化字段 |
| `checksum` | `string` | 可省略 |
| `size_bytes` | `int64` | 可省略 |

### internal/storage/backend.go · TemporaryMediaAccess

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |


### internal/domain/project/types.go · Project

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `string` | 序列化字段 |
| `name` | `string` | 序列化字段 |
| `is_default` | `bool` | 序列化字段 |
| `status` | `string` | 序列化字段 |
| `version` | `int64` | 序列化字段 |
| `task_count` | `int` | 序列化字段 |
| `asset_count` | `int` | 序列化字段 |
| `created_at` | `time.Time` | 序列化字段 |
| `updated_at` | `time.Time` | 序列化字段 |

### internal/domain/project/types.go · DeleteResult

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `project` | `Project` | 序列化字段 |
| `transferred` | `OwnershipCounts` | 序列化字段 |

### internal/domain/project/types.go · OwnershipCounts

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `tasks` | `int` | 序列化字段 |
| `assets` | `int` | 序列化字段 |
| `canvases` | `int` | 可省略 |

### internal/domain/assetgroup/types.go · Group

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `id` | `string` | 序列化字段 |
| `user_id` | `int64` | 序列化字段 |
| `project_id` | `string` | 序列化字段 |
| `name` | `string` | 序列化字段 |
| `version` | `int64` | 序列化字段 |
| `created_at` | `time.Time` | 序列化字段 |
| `updated_at` | `time.Time` | 序列化字段 |

### internal/service/videorouting/service.go · CapabilityResponse

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `route_model_code` | `string` | 序列化字段 |
| `name` | `string` | 序列化字段 |
| `description` | `string` | 可省略 |
| `minimum_points` | `string` | 序列化字段 |
| `icon_key` | `string` | 可省略 |
| `icon_svg` | `string` | 可省略 |
| `group_codes` | `[]string` | 可省略 |
| `config_version` | `string` | 序列化字段 |
| `capability_version` | `string` | 序列化字段 |
| `max_output_count` | `int` | 序列化字段 |
| `task_types` | `[]domainvideo.TaskType` | 序列化字段 |
| `combinations` | `[]domainvideo.Combination` | 序列化字段 |
| `input_capabilities_by_task_type` | `map[domainvideo.TaskType]map[domainvideo.InputRole]domainvideo.InputCapability` | 序列化字段 |

### internal/service/videorouting/service.go · CapabilityListResponse

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `groups` | `[]CapabilityResponse` | 序列化字段 |
| `route_model_groups` | `[]RouteModelGroupInfo` | 可省略 |

## 原生/兼容错误清单与恢复

- 下面按当前原生操作服务和兼容handler静态调用点汇总；具体HTTP状态、details与触发字段见API reference，不能把上游供应商动态错误视为封闭枚举。
- 401重新核对当前凭据、过期和账户；403先修scope/归属；402先补余额；409刷新版本或重新预检，幂等冲突必须修正业务请求；429按实际Retry-After和退避；5xx有上限重试且创建保留幂等键。
- 结果内error_code并不等同当前HTTP请求失败。

| 错误码 | 实际调用点 |
| --- | --- |
| `ARTIFACT_EMPTY_BODY` | `internal/service/imagetask/service.go` |
| `ARTIFACT_FETCH_CONNECTION_FAILED` | `internal/service/imagetask/artifact_recovery.go` |
| `ARTIFACT_FETCH_HTTP_STATUS` | `internal/service/imagetask/artifact_download.go` |
| `ARTIFACT_FETCH_READ_FAILED` | `internal/service/imagetask/artifact_download.go` |
| `ARTIFACT_FETCH_TIMEOUT` | `internal/service/imagetask/artifact_recovery.go`, `internal/service/imagetask/artifact_download.go` |
| `ARTIFACT_FORMAT_UNSUPPORTED` | `internal/service/imagetask/service.go` |
| `ARTIFACT_RECOVERY_PAYLOAD_INVALID` | `internal/service/imagetask/service.go`, `internal/service/imagetask/artifact_recovery.go` |
| `ARTIFACT_SIZE_LIMIT_EXCEEDED` | `internal/service/imagetask/service.go`, `internal/service/imagetask/artifact_download.go` |
| `ARTIFACT_SOURCE_URL_INVALID` | `internal/service/imagetask/artifact_download.go` |
| `ARTIFACT_STORAGE_CONFIG_UNAVAILABLE` | `internal/service/imagetask/service.go`, `internal/service/imagetask/artifact_recovery.go`, `internal/service/mediaasset/service.go`, `internal/service/mediaasset/content_range.go` |
| `ARTIFACT_STORAGE_VERIFY_FAILED` | `internal/service/imagetask/service.go` |
| `ARTIFACT_STORAGE_WRITE_FAILED` | `internal/service/imagetask/service.go`, `internal/service/imagetask/artifact_recovery.go` |
| `BAD_REQUEST` | `internal/service/openplatform/results.go`, `internal/service/openplatform/tasks.go`, `internal/service/openplatform/service.go`, `internal/service/imagetask/service.go`, `internal/service/imagetask/native.go`, `internal/service/imagetask/gallery_batch.go`, `internal/service/imagetask/preflight.go`, `internal/service/mediaasset/list.go`, `internal/service/mediaasset/service.go`, `internal/service/videotask/service.go`, `internal/service/videotask/history.go`, `internal/service/assetgroup/service.go`, `internal/http/handlers/compat.go`, `internal/http/handlers/open_platform.go` |
| `CANVAS_BUSY` | `internal/service/openplatform/service.go` |
| `CONFLICT` | `internal/service/openplatform/service.go`, `internal/service/imagetask/service.go`, `internal/service/imagetask/native.go`, `internal/service/imagetask/history.go`, `internal/service/mediaasset/service.go`, `internal/service/videotask/history.go` |
| `DERIVATIVE_NOT_READY` | `internal/service/mediaasset/service.go` |
| `FORBIDDEN` | `internal/service/openplatform/service.go` |
| `IDEMPOTENCY_CONFLICT` | `internal/service/openplatform/tasks.go` |
| `IMAGE_AUTO_RESOLUTION_UNSUPPORTED` | `internal/service/imagetask/service.go` |
| `IMAGE_CAPABILITY_MISMATCH` | `internal/service/imagetask/service.go` |
| `IMAGE_REFERENCE_REQUIRED` | `internal/service/imagetask/service.go`, `internal/service/imagetask/mask.go` |
| `IMAGE_REFERENCE_UNAVAILABLE` | `internal/service/imagetask/preflight.go` |
| `IMAGE_STORAGE_FAILED` | `internal/service/imagetask/service.go`, `internal/service/imagetask/artifact_recovery.go` |
| `INSUFFICIENT_BALANCE` | `internal/service/openplatform/tasks.go` |
| `INTERNAL_ERROR` | `internal/service/openplatform/results.go`, `internal/service/openplatform/tasks.go`, `internal/service/openplatform/service.go`, `internal/service/imagetask/service.go`, `internal/service/imagetask/gallery_batch.go`, `internal/service/imagetask/media_access.go`, `internal/service/imagetask/history.go`, `internal/service/mediaasset/service.go`, `internal/service/mediaasset/groups.go`, `internal/service/videotask/service.go`, `internal/service/videotask/quote.go`, `internal/service/videotask/history.go`, `internal/http/handlers/compat.go`, `internal/http/handlers/open_platform.go` |
| `NOT_FOUND` | `internal/service/openplatform/results.go`, `internal/service/openplatform/service.go`, `internal/service/imagetask/service.go`, `internal/service/imagetask/gallery_batch.go`, `internal/service/imagetask/media_access.go`, `internal/service/imagetask/history.go`, `internal/service/videotask/service.go`, `internal/http/handlers/open_platform.go` |
| `QUOTE_STALE` | `internal/service/openplatform/tasks.go` |
| `STORAGE_CONFIG_UNAVAILABLE` | `internal/service/imagetask/service.go`, `internal/service/imagetask/media_access.go` |
| `TASK_WAIT_INTERRUPTED` | `internal/http/handlers/compat.go` |
| `UNAUTHORIZED` | `internal/service/openplatform/service.go` |
| `UPSTREAM_UNAVAILABLE` | `internal/service/imagetask/service.go` |
| `VALIDATION_FAILED` | `internal/service/videotask/service.go` |
| `VIDEO_QUOTE_STALE` | `internal/service/videotask/quote.go` |
| `VIDEO_ROUTE_PRICE_UNAVAILABLE` | `internal/service/videotask/quote.go` |
| `default_project_immutable` | `internal/service/openplatform/service.go` |
| `project_changed` | `internal/service/openplatform/service.go` |
| `project_not_empty` | `internal/service/openplatform/service.go` |

OAuth错误和逐阶段恢复参见 [MCP指南](mcp.md)。兼容同步等待504 `TASK_WAIT_INTERRUPTED`时任务可能已持久化，使用X-Task-ID/Location恢复；不要假设失败响应不会产生任务。原生预检quote有效120秒，报价过期 `QUOTE_STALE` 需用原输入重新预检。

## 嵌套能力与图层字段

固定结构逐字段如下。动态map/RawMessage保留服务器实际键；不要把供应商扩展误称固定完整枚举。

### internal/domain/nativeimage/contracts.go · QwenOptions

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `prompt_extend` | `*bool` | 可省略 |
| `prompt_extend_mode` | `string` | 可省略 |
| `enable_thinking` | `*bool` | 可省略 |

### internal/domain/nativeimage/contracts.go · SeedreamOptions

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `optimize_prompt_mode` | `string` | 可省略 |

### internal/domain/nativeimage/contracts.go · Options

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `seed` | `*int64` | 可省略 |
| `negative_prompt` | `string` | 可省略 |
| `watermark` | `*bool` | 可省略 |
| `qwen` | `*QwenOptions` | 可省略 |
| `seedream` | `*SeedreamOptions` | 可省略 |
| `annotations` | `[]Annotation` | 可省略 |

### internal/domain/nativeimage/contracts.go · Annotation

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `reference_index` | `int` | 序列化字段 |
| `label` | `string` | 可省略 |
| `point` | `*[2]int` | 可省略 |
| `bbox` | `*[4]int` | 可省略 |

### internal/domain/nativeimage/contracts.go · BoundingBox

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `absolute` | `[4]int` | 序列化字段 |
| `normalized` | `[4]int` | 序列化字段 |

### internal/domain/nativeimage/contracts.go · LayerMetadata

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `z_index` | `int` | 序列化字段 |
| `name` | `string` | 可省略 |
| `description` | `string` | 可省略 |
| `bounding_box` | `*BoundingBox` | 可省略 |
| `is_base` | `bool` | 序列化字段 |

### internal/domain/nativeimage/contracts.go · CapabilityProfile

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `version` | `string` | 序列化字段 |
| `model` | `string` | 序列化字段 |
| `adapter` | `string` | 序列化字段 |
| `operations` | `[]Operation` | 序列化字段 |
| `min_pixels` | `int` | 序列化字段 |
| `max_pixels` | `int` | 序列化字段 |
| `max_aspect_ratio` | `float64` | 序列化字段 |
| `max_reference_images` | `int` | 序列化字段 |
| `max_output_images` | `int` | 序列化字段 |
| `output_formats` | `[]string` | 序列化字段 |
| `input_formats` | `[]string` | 序列化字段 |
| `layer_size_modes` | `[]string` | 可省略 |
| `max_layers` | `int` | 可省略 |
| `size_modes` | `[]string` | 序列化字段 |
| `negative_prompt` | `bool` | 序列化字段 |
| `seed` | `bool` | 序列化字段 |
| `layer_decomposition` | `bool` | 序列化字段 |
| `optimize_prompt_modes` | `[]string` | 可省略 |

### internal/domain/video/capability.go · IntValues

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `values` | `[]int` | 可省略 |
| `min` | `int` | 可省略 |
| `max` | `int` | 可省略 |
| `step` | `int` | 可省略 |

### internal/domain/video/capability.go · InputCapability

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `required` | `bool` | 序列化字段 |
| `max_count` | `int` | 序列化字段 |
| `max_bytes` | `int64` | 序列化字段 |
| `media_types` | `[]string` | 序列化字段 |
| `formats` | `[]string` | 序列化字段 |

### internal/domain/video/capability.go · TaskCapability

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `durations` | `IntValues` | 序列化字段 |
| `resolutions` | `[]Resolution` | 序列化字段 |
| `aspect_ratios` | `[]AspectRatio` | 序列化字段 |
| `audio_modes` | `[]AudioMode` | 序列化字段 |
| `inputs` | `map[InputRole]InputCapability` | 可省略 |
| `min_inputs` | `int` | 可省略 |

### internal/domain/video/capability.go · Capability

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `schema_version` | `int` | 序列化字段 |
| `provider_native_max_n` | `int` | 序列化字段 |
| `prompt_max_runes` | `int` | 序列化字段 |
| `task_types` | `map[TaskType]TaskCapability` | 序列化字段 |

### internal/domain/video/capability.go · Combination

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `candidate_id` | `int64` | 可省略 |
| `task_type` | `TaskType` | 序列化字段 |
| `duration_seconds` | `int` | 序列化字段 |
| `resolution` | `Resolution` | 序列化字段 |
| `aspect_ratio` | `AspectRatio` | 序列化字段 |
| `audio_mode` | `AudioMode` | 序列化字段 |

### internal/domain/modelhub/resolver.go · VisibleRouteModel

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `native_image_layer_unavailable_reason` | `string` | 可省略 |
| `native_image_layer_pricing_confirmed` | `bool` | 序列化字段 |
| `native_image_profile` | `*nativeimage.CapabilityProfile` | 可省略 |
| `id` | `int64` | 序列化字段 |
| `code` | `string` | 序列化字段 |
| `name` | `string` | 序列化字段 |
| `description` | `string` | 可省略 |
| `icon_key` | `string` | 可省略 |
| `icon_svg` | `string` | 可省略 |
| `group_codes` | `[]string` | 可省略 |
| `task_types` | `[]string` | 序列化字段 |
| `base_resolution` | `[]string` | 序列化字段 |
| `auto_base_resolution_by_task_type` | `map[string]string` | 序列化字段 |
| `quality` | `[]string` | 序列化字段 |
| `size_modes` | `[]string` | 序列化字段 |
| `aspect_ratios` | `[]string` | 序列化字段 |
| `pixel_sizes` | `[]string` | 序列化字段 |
| `output_format` | `[]string` | 序列化字段 |
| `supports_output_compression` | `bool` | 序列化字段 |
| `supports_custom_size` | `bool` | 序列化字段 |
| `supports_custom_ratio` | `bool` | 序列化字段 |
| `supported_backgrounds` | `[]string` | 序列化字段 |
| `min_width` | `int` | 可省略 |
| `max_width` | `int` | 可省略 |
| `min_height` | `int` | 可省略 |
| `max_height` | `int` | 可省略 |
| `moderation` | `[]string` | 序列化字段 |
| `max_output_image_count` | `int` | 序列化字段 |
| `max_reference_image_count` | `int` | 序列化字段 |
| `capabilities_by_task_type` | `map[string]VisibleRouteModelTaskCapability` | 序列化字段 |
| `effective_multiplier` | `string` | 序列化字段 |
| `minimum_points` | `string` | 可省略 |
| `prices` | `[]VisibleRouteModelPrice` | 序列化字段 |

### internal/domain/modelhub/resolver.go · VisibleRouteModelTaskCapability

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `native_image_layer_unavailable_reason` | `string` | 可省略 |
| `native_image_layer_pricing_confirmed` | `bool` | 序列化字段 |
| `native_image_profile` | `*nativeimage.CapabilityProfile` | 可省略 |
| `base_resolution` | `[]string` | 序列化字段 |
| `auto_base_resolution` | `string` | 可省略 |
| `quality` | `[]string` | 序列化字段 |
| `size_modes` | `[]string` | 序列化字段 |
| `aspect_ratios` | `[]string` | 序列化字段 |
| `pixel_sizes` | `[]string` | 序列化字段 |
| `output_format` | `[]string` | 序列化字段 |
| `supports_output_compression` | `bool` | 序列化字段 |
| `supports_custom_size` | `bool` | 序列化字段 |
| `supports_custom_ratio` | `bool` | 序列化字段 |
| `supported_backgrounds` | `[]string` | 序列化字段 |
| `min_width` | `int` | 可省略 |
| `max_width` | `int` | 可省略 |
| `min_height` | `int` | 可省略 |
| `max_height` | `int` | 可省略 |
| `moderation` | `[]string` | 序列化字段 |
| `max_output_image_count` | `int` | 序列化字段 |
| `max_reference_image_count` | `int` | 序列化字段 |

### internal/domain/modelhub/resolver.go · VisibleRouteModelPrice

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `task_type` | `string` | 序列化字段 |
| `base_resolution` | `string` | 序列化字段 |
| `base_points` | `string` | 序列化字段 |
| `charged_points` | `string` | 序列化字段 |
| `display_points` | `string` | 序列化字段 |

### internal/service/capabilities/service.go · Item

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `abstract_model` | `string` | 序列化字段 |
| `task_types` | `[]string` | 序列化字段 |
| `base_resolution` | `[]string` | 序列化字段 |
| `aspect_ratios` | `[]string` | 序列化字段 |
| `max_output_image_count` | `int` | 序列化字段 |
| `max_reference_image_count` | `int` | 序列化字段 |

### internal/service/capabilities/service.go · RouteModelGroupInfo

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `code` | `string` | 序列化字段 |
| `name` | `string` | 序列化字段 |
| `icon_key` | `string` | 可省略 |
| `icon_svg` | `string` | 可省略 |
| `route_model_codes` | `[]string` | 序列化字段 |

### internal/service/capabilities/service.go · Response

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `items` | `[]Item` | 可省略 |
| `model_groups` | `[]modelhub.VisibleRouteModel` | 可省略 |
| `route_model_groups` | `[]RouteModelGroupInfo` | 可省略 |
| `reference_image_max_mb` | `int` | 可省略 |
| `reference_image_max_bytes` | `int64` | 可省略 |
| `reference_image_allowed_formats` | `[]string` | 可省略 |
| `reference_image_allowed_mime_types` | `[]string` | 可省略 |
| `unavailable_reason` | `*UnavailableReason` | 可省略 |

### internal/service/capabilities/service.go · UnavailableReason

| 字段 | Go wire 类型 | 出现规则 |
| --- | --- | --- |
| `code` | `string` | 序列化字段 |
| `message` | `string` | 序列化字段 |

## OpenAI 兼容路径与完整输入

- GET `/v1/models` 使用 models:read，输出OpenAI模型列表（object,data以及每项id/object/created/owned_by，以reference为准）。
- POST `/v1/images/generations` 为JSON，POST `/v1/images/edits` 为multipart，均需 images:write 与 images:read。
- 共同字段：model、prompt（必填非空），size、n（默认1且正数，模型能力约束上限）、quality、output_format、background、output_compression、moderation、response_format（url或b64_json）、user、stream。
- partial_images出现即明确拒绝，不能解释为被忽略。
- 编辑另需image或image[]文件，可有单个mask；具体模型未声明mask时会拒绝。
- 编辑表单不接受未知字段、重复标量或其他文件名，总body64MiB，生成JSON body2MiB。
- 用Idempotency-Key重复同一业务操作，未知字段拒绝。

- 同步输出为图片结果封装 created、data（每项url或b64_json、revised_prompt等实际ImageResult字段见上表），不包装原生REST的data envelope。
- 返回头X-Task-ID与Location提供持久任务恢复。
- stream=true为平台扩展：X-Stream-Protocol:platform-image-lifecycle-v1；task事件{task_id,status,progress_stage}、result事件包含最终图片响应，成功后data:[DONE]，错误error事件结束且无成功DONE。
- 两分钟等待中断可返回504 TASK_WAIT_INTERRUPTED，已生成任务继续可查询；不是OpenAI partial image事件协议。
- 兼容错误为OpenAI形状error（message/type/code等以reference为准），不要使用原生error.message解析所有协议。

## 完整创建流程示例（占位符须替换）

先list_projects取得自己的project_id，list_image_models取得route_model_code及能力，再POST image-tasks/preflight：

```json
{
  "project_id": "REPLACE_WITH_OWNED_PROJECT_ID",
  "route_model_code": "REPLACE_WITH_DISCOVERED_MODEL",
  "operation": "generate",
  "task_type": "text_to_image",
  "prompt": "Editorial still life",
  "size_mode": "pixel",
  "requested_size": "1024x1024",
  "output_image_count": 1
}
```

- 这是完整流程的示例业务请求，size_mode与尺寸必须受当前模型允许；不是所有部署通用参数。
- 创建时保留同一业务输入，加入预检quote_token和新idempotency_key。
- 创建后保存data.id；视频创建保存data.task.id且检查data.replayed。
- 轮询任务详情，检查status/results或items，结果存在后再请求result access；仅HTTP201不表示生成成功。
- 429/断网重试保持同一幂等键，业务输入变化才使用新键。
- 模板不会自动调用收费生成。
