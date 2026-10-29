# API 密钥一次性保存确认弹窗

## 需求来源与授权

本会话用户要求：创建API密钥返回AK/SK后用遮罩弹窗展示，分别提供复制icon，下载CSV到本地；明确当前密钥仅展示一次并妥善保存，勾选“我已妥善保存”才可点击确定关闭；删除旧页面内SK展示。实施与本地验证已授权，不提交/推送/部署，不发真实凭据或付费请求。

## 技术设计

参考 docs/plans/2026-10-27-developer-onboarding-docs-redesign.md 和 docs/template/PicGallery/DESIGN-HANDOFF.md，沿用现有Modal和产品tokens。保留AK标识/SK实际Bearer语义及创建/轮换API契约。一次性凭据仅在React内存中，确认关闭立即清除；不写日志、storage、URL或secret文件名。

创建/轮换后展示独立保存弹窗，字段完整可读、icon复制有辅助标签和成功/失败反馈。遮罩点击、Escape、关闭按钮不能绕过保存确认，必要时使用不可dismiss对话框支持，保留焦点捕获与返回。下载CSV使用Blob+objectURL，正确CSV转义/CRLF/UTF8 BOM，固定非敏感文件名，释放objectURL；下载和复制不自动勾选保存。

checkbox初始false，每次新凭据重新初始化；按钮disabled直到用户勾选，确认handler同样守卫。正文警示只展示一次，旧凭据轮换失效说明保持真实，窄屏可滚动且动作可达。移除旧inline secret区域。

## 验证

聚焦CSV格式与内存下载、勾选守卫、所有dismiss入口不绕过、创建/轮换AK/SK映射及旧inline移除；user/admin typecheck/build及仓库verify、独立review。本地隔离模拟UI验创建返回、下载文件名/内容与关闭交互，不使用真实secret或paid任务。
