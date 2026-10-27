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

## 实施验收

- Vite serve 配置插件读取合并后的 server.port。非零端口且 strictPort=true 时使用 node_modules/.vite/dev-<port>；默认或显式非 strict、以及 port=0 时追加进程 PID，避免端口自动递增或随机端口重新共享缓存。显式 cacheDir 由调用方控制，build 保留 Vite 默认路径。真实 resolveConfig 回归覆盖 5193、5273、5293、默认 5173、strict true/false、port=0、显式覆盖及 build。
- 三个本地用户服务已改回正常 Vite CLI 启动，分别使用 dev-5193、dev-5273、dev-5293。CanvasEditorPage 与 canvasStore、实际响应中的六个 Zustand/vanilla 依赖链接全部返回 200 JavaScript；交叉请求后缓存 metadata 指纹及 mtime 稳定。
- canvasRouteLoading.contract.ts 的 8 项行为测试通过，覆盖下载失败局部捕获、手动重建 lazy 实例、失败重试保持可操作、运行时/求值错误不提供重试、显式导航重置、普通更新保留草稿状态，以及外层导航与上传队列连续性。测试通过仓库 *.contract.ts 入口自动执行，renderer 与 tsx 固定为开发依赖。
