# 创作、媒体与开发者接入改进

## 需求与审批

来源为本会话用户的14项要求及随后获批的完整实施计划。初始实施阶段不自动commit/push/部署。用户随后明确要求“帮忙推送到远端，打包tag后，部署到远端，我去远端测试下mcp”，现已授权提交、推送、版本标签、正式打包及通过mgsctl升级原服务器。继续不新增付费生成，不擅自修改生产域名或安装绑定；MCP公网地址和网关实际能力须在部署验收中如实报告。

1. 创作台Seedream能力置于模型下：生成方式默认“常规生图”，另一项“图层拆分”；常规按有无引用自动文生图/图生图，拆层素材标题“图层拆分来源”，单张必填。
2. 双击画布组件只改变视口中心和缩放，完整显示且不过度放大，不改变组件位置。
3. 画布操作台同样在参数表单以生成方式下拉框提供常规/拆层，逻辑一致。
4. 图片资产卡片补公开状态与操作。
5. 分组改tag，放公开状态后。
6. 视频属性显示像素、格式、时长。
7. 资产列表增加服务端公开状态筛选。
8. 全面复核API Key/MCP接口文档、BaseURL、入出参、错误枚举、OAuth token获得流程；密钥页提供端点复制与Mikiko/ZCode、Codex、Claude实际配置及教程。
9. 修复已连接应用含糊报错，区分未配置/未登录/过期/故障并支持重试。
10. 公开视频补点赞、收藏、下载，包含真实后端持久化、计数、筛选排序及授权。
11. 各媒体卡片名称固定两行槽位，单行顶对齐。
12. 以创作页实际topbar规格统一用户端高度与内边距。
13. Toast重构Success/Warn/Error/Info并逐项审计调用语义，稳定计时和可访问性。
14. 首页完成任务显示静止100%进度，保留卡片高度。

## 技术设计

复用nativeImageUX operation、现有引用准入和nativeimage服务校验；能力和费率决定模式可用，输入完整性决定能否报价提交，允许先选拆层后添加图片。多图切换不静默删输入，拆层输出数量锁1；模型切换与异步完成重新校验。复用Canvas bounds/fit计算，单节点适度zoom上限、可见区域边距和重算中心；隔离编辑/连接/按钮双击，防微抖拖动。

资产统一公开展示组件通过图片/视频适配各自生命周期。无可公开历史图片ID的来源明确限制。分组维持现有单组模型。公开筛选独立于处理状态，在仓储合并新旧资产后分页前过滤并同步API契约。视频互动复用现有持久化模式，媒体类型/ID隔离、幂等、已审核访问保护，Feed热度和viewer filters完整接入。

共享样式层固定两行标题、统一Shell顶栏尺寸，保留专用画布工具栏功能。Toast四等级明确映射与计时生命周期；完成进度不再无限动画。

开发者文档以真实路由、openplatform操作目录和OpenAPI为准，覆盖所有公开操作、字段/错误/重试/幂等与scope。端点源自可信运行配置，分别展示origin、/api/open/v1、/v1、/mcp，不能从不可信Host推导issuer。客户端模板按实际格式（JSON/TOML/CLI），不包含用户secret/token。

OAuth逐段验证401 challenge→protected metadata→discovery→注册→PKCE authorize→浏览器登录/consent→callback→token→MCP initialize/tools→刷新撤权；修复注册永久限额和错误分类，保留redirect/resource/scope/PKCE/HTTPS约束。对空canonical、API-only origin、路径前缀、Origin错误和代理SPA fallback明确诊断。当前生产未有Gallery HTTPS canonical，属于部署前提，不能伪造域名或改digest开启功能；本轮不改变生产配置。

## 现有来源

- docs/plans/2026-10-25-native-image-parameter-ux.md
- docs/plans/2026-10-24-canvas-interaction-reliability.md
- docs/plans/2026-10-24-public-media-history-remediation.md
- docs/tech/2026-10-25-native-image-models-and-public-feed.md
- docs/plans/2026-10-24-unified-api-mcp-design.md
- docs/plans/2026-10-26-video-publication-moderation-hotfix.md

## 验证和交付

按写入范围并行实现，禁止同时运行多份完整verify。各owner跑聚焦回归；主线统一user/admin/docs类型构建、全verify、独立复核、隔离API smoke。新增数据库结构需隔离PG migration回归。浏览器验桌面/320/390/平板、深浅色、长文本、状态、键盘触屏、双击视口前后节点不变、四Toast、实际topbar尺寸。协议fixture与真实客户端和生产未配置分别报告，逐项交付14项结果与边界，不把HTTP200当作授权或产物成功。
