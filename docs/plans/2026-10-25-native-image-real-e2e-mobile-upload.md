# 真实原生生图与手机浮动上传需求设计

来源：2026-10-25用户本轮三项要求及已批准方案。

## 需求

使用api-keys.env既有SEEDANCE/百炼北京凭据，在管理后台配置Qwen/Seedream原生图片账号、真实型号、独立路由及明确成本售价，不改视频账号。先预检再用户端真实E2E，5正式型号各一图、可选3.0Pro一图、两平台各一次参考编辑，最多8任务；固定场景幂等标识，不盲目重生。验证权限/实际型号/结果下载/余额；未知拆层费不解锁。密钥不出日志/报告/仓库。

手机创作操作台背景接近不透明且适配浅深色、安全区和短屏。上传队列保持常驻，收起为48px单icon，待处理任务角标（不计完成/取消）；可二维拖动、贴左右边半隐藏、点击/聚焦唤出、pointer cancel清理及拖后click抑制。位置用户本地保存，resize/visualViewport变化clamp；展开完整队列保持滚动且不挡菜单/底部关键操作。画布编辑隐藏但任务不卸载。

## 技术设计

复用现有UploadTray业务，只拆展示状态和几何纯函数；pointer capture记录坐标和阈值，viewport边界/安全区计算、贴边状态独立。展开面板按浮标位置决定左右锚点并限制可见高度。必要菜单复用OverlayPortal并同步外点判断。CSS手机断点覆盖u-controls语义实体底色，桌面不变。

修正Seedream BaseURL拼接同时支持官方root与/api/v3，不重复version；请求路径httptest。账号通过后台API查询/复用幂等配置，能力沿上一轮native_image契约、成本售价按已有Excel建议和runtime兑换率。API/Worker使用既有已绑定环境；启动Worker前查无无关付费队列，结束停止临时Worker。

## 验证

真实模型从用户端提交，不以后台test按钮代替E2E；记录费用/失败与无效权限，不编造通过。移动320x568/375x667/390x844/横屏浅深色，浮标拖动半隐藏、队列角标、模型菜单末项、操作台展开背景。聚焦contracts/typecheck/build、verify、独立review和适用API隔离smoke。无commit/push/生产部署。
