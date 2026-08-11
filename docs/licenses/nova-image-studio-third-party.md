# Nova Image Studio 画布复用与第三方依赖清单

日期：2026-08-12  
调研基线：`nova-image-studio main@7768f3f8d7f47e04c6d18572837a086c7a533161`

## 授权边界

- Nova Image Studio 仓库当前公开许可证为 AGPL-3.0。
- 项目所有者已确认通过线下与 Nova 作者沟通取得商业授权，允许本项目复用其画布实现。
- 当前两个仓库中都没有该商业授权的书面文件、许可证例外或可供自动验证的凭证。本清单只记录项目所有者提供的事实，不生成或暗示不存在的证明。
- 商业授权只处理 Nova 作者可授权的代码，不替代 Nova 第三方依赖各自的许可证义务。
- 正式实现只复用授权范围内的 DOM 节点、CSS 视口变换、SVG 连线、Pointer Events、框选、撤销重做、小地图、搜索和布局思路/代码；Nova 的 API、模型配置、本地图片库、服务端和视觉样式不进入本项目。

## 计划复用文件

主要参考以下源文件。实际复制或改写时，应在提交审查中记录最终文件映射：

| Nova 文件 | 计划用途 |
| --- | --- |
| `frontend/src/components/canvas/components/infinite-canvas.tsx` | 视口变换、Pointer Events、框选和平移缩放 |
| `frontend/src/components/canvas/components/canvas-connections.tsx` | SVG 连线渲染 |
| `frontend/src/components/canvas/components/canvas-mini-map.tsx` | 小地图交互 |
| `frontend/src/components/canvas/components/canvas-node-search-dialog.tsx` | 节点搜索交互 |
| `frontend/src/components/canvas/stores/use-canvas-store.ts` | 高频 selector store 与 graph command 参考 |
| `frontend/src/components/canvas/utils/canvas-layout.ts` | Dagre 自动整理 |
| `frontend/src/components/canvas/lib/localforage-storage.ts` | IndexedDB 草稿恢复参考 |

## 直接依赖许可证

许可证值已于 2026-08-12 通过对应 npm registry 包元数据核对。这里只列出画布一期计划直接使用或现有项目已使用的核心依赖；安装锁文件落地后仍需对完整传递依赖生成 SBOM/许可证报告。

| 包 | Nova 版本 | 许可证 | 一期决定 |
| --- | --- | --- | --- |
| `@dagrejs/dagre` | `3.0.0` | MIT | 可引入，用于选中节点自动整理 |
| `zustand` | `5.0.12` | MIT | 可引入，用于画布高频状态 |
| `localforage` | `1.10.0` | Apache-2.0 | 可引入，保留 NOTICE/许可证义务 |
| `nanoid` | `5.1.11` | MIT | 可引入或改用平台 UUID，不作为强依赖 |
| `@tanstack/react-virtual` | `3.13.23` | MIT | 仅性能基准需要时引入，P0 不预装 |
| `lucide-react` | `0.562.0` | ISC | 本项目已有 `lucide-react`，优先沿用仓库版本 |
| `react` / `react-dom` | `19.2.3` | MIT | 本项目已有 React 19，沿用仓库版本 |

## 发布门禁

1. 画布实现提交必须列明从 Nova 复制、改写或仅参考的文件。
2. `web/user/package-lock.json` 更新后生成完整 npm 依赖许可证清单和 SBOM，检查未知、商业限制和 copyleft 传递依赖。
3. 保留 Apache-2.0 等要求的许可证及 NOTICE 信息。
4. 不把 Nova 原仓库的 AGPL `LICENSE` 替换成本项目许可证，也不以公开 AGPL 授权代替项目所有者确认的线下商业授权。
5. 若实现范围超出线下授权，或最终依赖扫描出现不兼容许可证，停止对应代码合入并由项目所有者重新确认。
