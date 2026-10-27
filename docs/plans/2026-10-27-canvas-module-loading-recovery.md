# 画布依赖缓存冲突与加载失败恢复

## 用户需求

本会话报告本地5193画布白屏，Zustand optimized dependency请求504与CanvasEditorPage动态import失败；用户授权立即修复，必要代码变更提交推送、tag打包并通过mgsctl重新部署远端。不新增付费任务、不改安装绑定、保留现有服务与数据。

## 根因证据

只读复现：5193/5273/5293三个web/user Vite进程同用node_modules/.vite，分别输出不同hash的zustand.js引用，全部504；磁盘缺zustand.js/vanilla。Vite optimizer整目录替换导致进程间缓存冲突。5397独立cache未受影响。不是仅浏览器旧hash。React.lazy rejection无错误边界让整个应用卸载。

## 技术设计

本地立即仅重启上述三个Vite，各显式独立cache；保留API/Worker/admin/docs/数据库。持久修复web/user Vite在serve时按解析后的端口隔离cache，并尊重显式cacheDir，普通build行为不变。使用Vite自身配置解析测试，保证CLI端口/默认端口/显式覆盖均不会共享；实际多server并发请求Canvas与store其发出的依赖URL为200JS。

画布懒加载加局部错误边界，保留providers/导航/UploadTray。仅明确的加载传输失败提供用户主动重试并重建lazy实例；运行时异常显示真实失败与返回入口，不承诺未保存数据安全、不自动刷新/无限重试/动态时间戳import。既有草稿机制不改。

## 现有来源

- docs/plans/2026-10-24-canvas-interaction-reliability.md
- docs/plans/2026-10-26-local-redeployment-recovery.md

## 验证与发布

新增有意义的Vite配置隔离与懒加载失败行为回归，聚焦tests/全verify/review/API smoke发布门禁。先本地恢复并验证fresh import依赖，再新提交tag正式Actions/manifest/镜像摘要核验，生产通过mgsctl备份升级，保留https://mikiko.studio/revision2/schema11/peer服务与MCP链路。不将本地dev故障误报生产同样受影响。

## 发布兼容补充需求与设计（v0.0.45）

- 用户要求定位 Actions 40526105809（6fea070）full verify 的实际 Linux / Node 22 失败，在当前分支修复、增加回归并冻结交接；本轮禁止 commit、push、tag、deploy，不读取 secret、env 或浏览器凭据。
- 日志证据：`useLandingMotion.contract.ts` 导入 `useLandingMotion.ts`，Node v22.23.4 报 `gsap/ScrollTrigger` 不提供命名导出 `ScrollTrigger`。macOS Node v24.12.0 同一 contract 通过；必须在隔离 Linux Node 22、锁定依赖安装下复现后确定兼容入口。
- 隔离 Linux Node v22.23.4、lockfile `npm ci` 复现同一错误。修复插件入口后进一步暴露同一 hook 的 `gsap` 命名导入错误；逐项导出探测证实官方 `gsap/dist/ScrollTrigger` 提供正确插件导出，而根 `gsap` 的默认导出在 Node 22/tsx 为包装对象。核心保留根入口，与 `@gsap/react` 共享浏览器模块，通过单层 `.gsap` 归一化取得核心，并显式 `registerPlugin(useGSAP, ScrollTrigger)` 保证 hook 与动画使用相同核心；不引入 dist 核心副本。真实 Vite `resolveConfig` contract 在修改前已于同一 Linux 环境完整通过，无 Tailwind/Vite 配置改动。
- 保留 contract 对真实动画模块的导入与插件注册，并在无 tsx 的原生 Node 子进程中验证 hook 实际使用的核心和插件入口、tween 运算、ScrollTrigger 函数及 create/refresh API，避免把运行时导入检查降成静态字符串检查。只调整已证实不兼容的模块入口，保持页面动画行为。
- 单独执行真实 Vite `resolveConfig` contract，若发现 Node 22 配置加载问题，再凭具体错误修复；不预先更换 Tailwind/Vite 依赖或配置加载方式。
- 验收包括 Linux Node 22 focused contracts、user/admin typecheck/build 和本地差异审查。使用不含 env/凭据的临时工作副本，保留其他本地变更，不发布。

## 实施验收

- Vite serve 配置插件读取合并后的 server.port。非零端口且 strictPort=true 时使用 node_modules/.vite/dev-<port>；默认或显式非 strict、以及 port=0 时追加进程 PID，避免端口自动递增或随机端口重新共享缓存。显式 cacheDir 由调用方控制，build 保留 Vite 默认路径。真实 resolveConfig 回归覆盖 5193、5273、5293、默认 5173、strict true/false、port=0、显式覆盖及 build。
- 三个本地用户服务已改回正常 Vite CLI 启动，分别使用 dev-5193、dev-5273、dev-5293。CanvasEditorPage 与 canvasStore、实际响应中的六个 Zustand/vanilla 依赖链接全部返回 200 JavaScript；交叉请求后缓存 metadata 指纹及 mtime 稳定。
- canvasRouteLoading.contract.ts 的 8 项行为测试通过，覆盖下载失败局部捕获、手动重建 lazy 实例、失败重试保持可操作、运行时/求值错误不提供重试、显式导航重置、普通更新保留草稿状态，以及外层导航与上传队列连续性。测试通过仓库 *.contract.ts 入口自动执行，renderer 与 tsx 固定为开发依赖。
