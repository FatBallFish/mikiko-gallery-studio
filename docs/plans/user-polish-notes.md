# User Polish Notes

## 本轮处理
- 收敛 `web/user` 中非合约圆角档位，保留 `rounded-xl` / `rounded-2xl` / `rounded-3xl` / `rounded-[2rem]` / `rounded-full`
- 去掉登录页社交按钮内联品牌色 SVG，统一为站内 token 风格按钮
- 为 Home、Gallery、Profile、ApiKeys 增加骨架式加载反馈
- 将 Gallery 的操作图标切到 `web/user/src/ui/icons.ts` 出口，避免继续散落内联图标
- 补强一批 `active:scale-[0.98]` 触觉反馈
- 追加做了 375px 可视验收，修正 Landing 顶部按钮挤压与 PublicGallery 底部导航遮挡问题

## 观察到但未越界处理
- 仓库当前已经存在 `web/admin/**` 与 `web/shared/tokens.css` 的未提交改动；本轮未回滚、未写入这些文件
- `web/user/src/styles.css` 仍保留一批主题变量定义中的硬编码底色，这是现有主题映射层的一部分，本轮未额外扩散修改
- `LandingPage` 在本地 preview 的真实浏览器截图中，hero badge 下方主标题/正文仍未显示；DOM 与 computed style 显示节点存在且可见，但截图结果不一致，需单独做渲染链路排查
- `PublicGalleryPage` 在当前 preview 环境会稳定出现“请求的资源不存在或已不可用”提示，疑似本地数据接口不可用导致，暂不按纯样式问题处理

## 共享层提议
- 暂无
