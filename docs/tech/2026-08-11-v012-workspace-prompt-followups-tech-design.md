# v0.0.12 创作与首页体验跟进技术设计

日期：2026-08-11
对应需求：`docs/prd/2026-08-11-v012-workspace-prompt-followups-requirements.md`

## 1. 范围与约束

本次只修改用户端 React/TypeScript/CSS，不改变后端 API、数据库或公开列表隐私契约。复用现有 `CapabilityModelGroup.description`、`minimum_points`、引用资产重命名 API 和公开图片详情 API。

## 2. 模型分组选择器

新增轻量 `ModelGroupSelect` 组件，使用按钮加定位浮层实现单选 listbox。触发按钮展示当前项 Title、可选 SubTitle 和右侧 `◈XX`；浮层选项沿用 `.prompt-token-menu` 的表面、边框和选中反馈，但保留独立语义类，避免 Prompt 菜单业务耦合。

组件使用 `aria-haspopup="listbox"`、`aria-expanded`、`aria-controls`、`role="option"` 和 `aria-selected`。打开时选中当前项；ArrowUp/ArrowDown 循环移动，Enter/Space 选择，Escape 或 outside pointer 关闭并把焦点还给触发按钮。

## 3. 引用资产重命名

引用卡片改为纵向结构：固定比例预览区加下方名称操作行。名称行使用省略号和 `title`，编辑图标只负责打开统一 `Dialog`。Workspace 继续持有待编辑资产、草稿名和 busy 状态；现有保存函数不改变 API 行为，成功后仍调用 `renamePromptReference`。

## 4. Prompt Token

将当前依赖完整占位符文本展示的 Token 节点调整为可交互的行内 Decorator 节点。节点内部仍保存 `kind` 和标准化 `name`，`getTextContent()` 与 JSON 导出继续返回标准占位符，从而保证表单值、历史记录和服务端契约不变；渲染层仅显示名称和一个关闭按钮。

关闭按钮通过节点 key 在 Lexical update 中只移除当前节点，并把光标放回相邻位置。Tag 继续暴露类型、名称、有效性、焦点状态和预览所需 data/aria 属性。

自动补全增加 dismissed trigger 标识，标识由文本节点 key、触发起点、字符类型和触发文本组成。Escape 或 outside pointer 关闭自动菜单时记录当前标识；编辑器更新若仍解析到相同标识则保持关闭，触发文本变化或消失后清理标识。手动工具栏菜单不写入该状态。正则仅识别未转义的独立 `$`/`@` 搜索尾部，普通 `{{}}` 不匹配。

## 5. 首页完整 Prompt

`HomePage.openImage` 先用列表数据即时打开弹窗，再使用当前会话 token 调用 `openApi.getPublicGalleryImage`。成功后仅在当前选中图片 ID 仍匹配时合并详情；完整 `prompt` 成为展示和复制源。请求失败时通知错误并禁用或保留无完整值的复制状态。列表请求继续显式使用 `accessToken: null`，`homePublicDetailImage` 继续负责列表值脱敏，防止匿名响应中的意外字段泄露。

## 6. 测试策略

1. 契约测试验证原生 select 被移除、三段式内容与 listbox 无障碍标记存在。
2. Prompt 模型/组件契约验证视觉文本不含标识符、删除命令按 node key 工作、dismissed trigger 对相同 occurrence 生效。
3. 首页契约验证详情接口使用会话 token，复制回调读取详情 `prompt`，列表仍保持匿名与摘要脱敏。
4. 运行用户端 typecheck/build 和仓库完整 verify；使用浏览器覆盖桌面、移动端、键盘、outside click、长名称和重复 Tag。
