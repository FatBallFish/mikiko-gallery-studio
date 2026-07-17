# Admin Polish Notes

## 协作记录

- `2026-07-03`: 仓库已存在 `.coding-context.json`，当前内容由 user agent 为“用户端页面视觉精修”写入。
- 按 `docs/plans/prompt-polish-admin-web.md` 要求，本次 **不覆盖** `.coding-context.json`，管理端打磨过程与风险记录统一维护在本文件。

## 本次 admin 任务

- 范围：`web/admin/**`
- 目标：把管理端页面从“功能可用”收敛到“专业、克制、可扫读”的 `Soft Grid Ops` 风格
- 执行策略：
  - 先收敛基础原语：theme、class、table、status、empty/loading、icons、TopBar
  - 再优先打磨核心页面：`Overview`、`Review`、`Users`、`Cashier`
  - 最后扫尾其余 admin 页面并做全量验证

## 当前已识别问题

- TopBar 仍承载页面标题和 breadcrumb，不符合 spec 第 9.3 节
- `styles.css` 仍使用 light mode 属性选择器 patch，而不是 token-driven 映射
- 多个页面各自维护 `<table>` 样式，表格密度不统一
- 仍有大量内联 `<svg>` 图标，未统一到 `lucide-react`
- 存在超出契约的圆角：`rounded-[2rem]`、`rounded-[2.5rem]`、`rounded-[10px]`
- 空状态与加载态骨架不足，部分区域仍是裸文本或 spinner

## 共享层提议

- 暂无
