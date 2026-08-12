# 多媒体创作一期视觉验收记录

## 结论

- 验收日期：2026-08-12。
- 验收对象：用户端 production build 的快捷视频创作、统一资产、创意画布。
- 结果：18/18 组合通过自动布局、画布像素和媒体访问检查。
- 视口：桌面 `1440x960`、手机 `390x844`、平板横屏 `1180x820`。
- 主题：亮色、暗色；截图统一使用 amber accent，组件仍复用现有主题变量。
- 数据边界：浏览器层只模拟登录态、项目、功能开关和 API 数据，不替换 React 页面或页面组件。

## 自动检查

执行命令：

```bash
npm --prefix web/user run build
node scripts/visual/multimedia-phase1-acceptance.mjs
```

脚本逐组合验证：

1. production bundle 成功载入真实路由和页面组件；
2. 页面无非预期横向溢出，交互控件无非预期重叠；
3. 画布当前视口至少有一个可见节点，截图非主色像素数均大于 500；
4. 平板横屏的画布为完整编辑态，不是手机只读态；
5. 资产列表只申请 `thumbnail`、`poster`、`waveform` 等派生资源；打开详情申请 `preview`；只有点击下载才申请 `download`；
6. 六个资产页组合各发生且仅发生一次显式原件下载授权，其他页面原件请求数为 0；
7. 浏览器实际产生 download 事件并完成临时下载。

画布非空像素最低值为手机暗色视口的 `111760`，桌面/平板均超过 `337000`。

## 截图索引

截图目录：`docs/reviews/screenshots/multimedia-phase1/`。

| 视口 | 亮色 | 暗色 |
| --- | --- | --- |
| 桌面 | `desktop-light-genpic.png`、`desktop-light-gallery.png`、`desktop-light-creative-canvas.png` | `desktop-dark-genpic.png`、`desktop-dark-gallery.png`、`desktop-dark-creative-canvas.png` |
| 手机 | `mobile-light-genpic.png`、`mobile-light-gallery.png`、`mobile-light-creative-canvas.png` | `mobile-dark-genpic.png`、`mobile-dark-gallery.png`、`mobile-dark-creative-canvas.png` |
| 平板横屏 | `tablet-landscape-light-genpic.png`、`tablet-landscape-light-gallery.png`、`tablet-landscape-light-creative-canvas.png` | `tablet-landscape-dark-genpic.png`、`tablet-landscape-dark-gallery.png`、`tablet-landscape-dark-creative-canvas.png` |

## 本轮发现并修复

1. 空上传托盘默认展开会遮挡桌面页脚的 API 文档入口。修复为默认收起；用户从上传入口打开时仍自动展开。
2. 收起的上传托盘与画布的撤销、缩放控件重叠。桌面和平板画布中改为停靠在底部工具栏上方，并新增三类画布浮层的几何碰撞断言。
3. 手机上传托盘与固定底部导航、小地图重叠。普通页面停靠在导航上方，手机画布中停靠在小地图上方，展开时向上延伸。

## 已知边界

- 图片、视频和音频正文使用一像素本地响应，验收关注访问用途、延迟加载、布局和交互入口，不评价真实素材色彩或编码质量；真实转码由 FFmpeg 黄金样本和 API smoke 覆盖。
- 手机画布启用视口虚拟化，自动检查只要求当前视口存在可见节点并执行整块像素检查，不要求所有文档节点同时存在于 DOM。
- 项目所有者仍需在发布候选版本上完成最终人工验收，重点检查真实媒体观感、触控手势和软键盘遮挡。
