# API 密钥、授权与开发文档体验修复

## 用户需求与授权

来源：本会话用户逐项要求。允许实现与验证，不自动提交、推送或部署；保留此前未提交canonical配置工具变更。

1. 核实AK/SK认证链路，无用SK从产品和新建凭据契约移除；如有实际用途补文档，不破坏现有Bearer key。
2. 密钥页移除整块开发接入card，在创建新密钥右侧新增“如何接入”打开标准抽屉。字体、按钮、间距沿用现有产品组件。
3. 抽屉API接入/MCP接入tabs。API展示OpenAI兼容和原生API简单生图/结果示例，直接嵌实际端点；不设独立BaseURL/Origin复制区。MCP展示真实agent配置与操作方式。
4. Mikiko授权卡片：纯Mikiko平台品牌、清晰的申请应用对象与权限中文说明、权限区限制高度并滚动、底部允许/取消；不展示全局上传浮层。客户端名称作为不可信但真实申请对象展示，平台品牌不混淆，不伪造第三方应用身份。
5. Guides改Markdown渲染，实际URL为https://mikiko.studio；代码围栏、自然简洁中文、长段分点、全文宽排版、全宽比例表格、保留左目录并增加页内右目录。完整接口参考页及其渲染/样式边界保留。

## 技术依据与设计

- docs/plans/2026-10-24-unified-api-mcp-design.md：Bearer-only开发者API、MCP OAuth独立认证、真实协议契约。
- docs/plans/2026-10-27-studio-media-developer-ux.md：可信端点与API/MCP接入字段。
- docs/template/PicGallery/DESIGN-HANDOFF.md：既有字体层级、间距、颜色及交互组件一致性。

先以实际认证代码审计决定SK处理；不删除既有数据库凭据字段以免无必要破坏迁移。产品侧只展示实际有效凭据。API示例与模型目录、任务preflight/quote/create/poll真实契约一致，密钥使用环境变量占位，地址使用真实canonical；不写死secret或自动发付费请求。

密钥接入抽屉复用标准Drawer/Tabs，局部样式隔离。授权页独立于全局workspace浮层，保留登录与consent CSRF/过期/取消真实流程；已知scope映射用户文案，未知scope不静默隐藏。

Docs在现有Vite/React壳内用成熟Markdown+GFM渲染，不引入另一套站点构建以免破坏hash路由、主题与完整接口参考。指南Markdown为唯一正文来源，heading id及右目录一致、重复heading去重、链接/代码安全处理、响应式降级。Guide样式限定作用域，不改ReferencePage；旧JSX重复正文退场。

## 验证

聚焦认证/create-key契约、scope文案、隐藏上传组件、Drawer/tab/accessibility、Markdown标题锚点/表格/代码/真实URL/source同步。浏览器桌面与窄屏检查密钥抽屉、授权卡片、文档双目录/滚动/表格。运行全verify与独立review；若后端凭据契约有变运行隔离API smoke。区分模拟页面验收和真实客户端授权，不获取秘密、不新增付费任务，不改生产。
