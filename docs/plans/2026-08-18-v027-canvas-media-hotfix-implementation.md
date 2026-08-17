# v0.0.27 画布与媒体上传兼容修复实施计划

## 任务 1：图片任务类型自动推导

1. 增加前端纯函数测试，覆盖 0 张和至少 1 张参考图的任务类型。
2. 修改画布默认节点、能力选择、参数归一、错误展示和估价请求，统一使用推导结果；任务切换时自动选择可用模型。
3. 增加后端生成器测试，证明服务端忽略过时草稿类型并按真实引用推导。
4. 修改 `imageRequest` 并运行前后端聚焦测试。

## 任务 2：提示词编辑能力复用

1. 增加画布 contract 测试，要求使用共享 `PromptTemplateEditor` 和 `PromptVariableForm`。
2. 建立画布资源候选到共享 `ReferenceAsset` 的适配，补齐预览 URL。
3. 替换画布提示词节点的重复输入与 Tag 实现，保留优化与变量持久化。
4. 运行提示词编辑器、画布 contract、TypeScript 类型检查。

## 任务 3：资产选择状态修复

1. 增加 contract 测试，禁止抽屉发送固定 `status=ready`。
2. 修改资产查询与不可用状态过滤。
3. 验证空态、加载态、图片原图降级预览与选择回填。

## 任务 4：R2 分片上传兼容

1. 增加 S3 模拟服务测试：收到 `x-amz-checksum-sha256` 时拒绝请求。
2. 修改代理 UploadPart 签名请求，不发送可选 checksum header。
3. 保留本地 SHA-256 校验并覆盖 checksum 不匹配测试。
4. 运行 storage、mediaasset 和媒体上传接口聚焦测试。

## 任务 5：完整验证与评审

1. 运行 `./scripts/workflow/verify.sh`。
2. 启动本地服务，完成桌面端和平板横屏画布交互验收。
3. 运行 `./scripts/workflow/api-smoke.sh`。
4. 提交变更后运行 `./scripts/workflow/review-local.sh --scope committed` 与 `./scripts/workflow/check-review-gate.sh`。
5. 本轮仅保留在开发分支；除非用户后续明确要求，不创建 PR、不合并、不打 Tag。
