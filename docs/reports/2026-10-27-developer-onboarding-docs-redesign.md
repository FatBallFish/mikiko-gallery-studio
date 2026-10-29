# 开发者接入体验验收

## 结果

- 认证核实：secret/sk-是实际Bearer API Key，access_key/ak-仅标识。保留后端认证与轮换，页面改明确术语、移除静态Secret掩码列。未修改数据库schema或认证契约。
- 密钥页“创建新密钥”右侧新增“如何接入”，标准Drawer含API/MCP tabs，无Origin/BaseURL复制清单。示例直接使用可信canonical端点，SK环境变量注入。原生例显式pixel模式并走quote/create/poll/access；独立review所发现缺mode P2已修复并复核关闭。
- 授权页Mikiko平台brand与真实外部应用身份分离，中文scope标题与说明、未知scope保留、长名可展开、权限限高滚动、独立footer。UploadTray在授权页隐藏但不卸载后台任务。
- 16篇指南由docs/api/*.md直接渲染。react-markdown+remark-gfm，原shell/左导航/search/pager保留，h2/h3右目录及深链定位，围栏代码/复制、全宽表格、全文宽段落。生产接口https://mikiko.studio，不再用示例API域名。字段映射表行完整保留。
- 完整接口参考保持不变：ReferencePage SHA256 346a4fb2cf9ff115741df0eefaef02fb29fbe5095e4660011c60f7fec09a08b5；openapiManifest SHA256 6b0a5bd1aed0d3558e836f8c3e8e04b604857ac250e1c7d9d52482d71b584094。

## 验证

- 全量scripts/workflow/verify.sh最终exit0，日志/tmp/developer-onboarding-verify-complete.log。两次中间失败均为旧source contract写死UploadTray only canvas，已同步授权页hidden且保留原canvas断言。
- review-local --scope all PASS；独立审查完成，确认P2已关闭；git diff --check PASS。
- npm --prefix web/docs run test:guides PASS（16篇元数据/SSR渲染/安全/锚点/真实端点/认证语义）。user typecheck和凭据、端点、接入、OAuth聚焦contracts通过；示例Python/curl仅语法与契约验证，不发付费请求。
- Mikiko IAB实际渲染验收：桌面API/MCP抽屉、390px MCP代码块；390x844浅色20scope授权卡片首屏可见3项；1440x900指南全宽表格和左右目录、MCP短段落与分层目录。
- IAB locator physical click多次超时，使用已snapshot证实元素的原生DOM click完成抽屉、tab与目录事件验证，未伪造hash或滚动。Docs owner实测直接刷新first-request深链scrollY725/heading top87.8；next目录跳转scrollY820=maxscroll，原Y0不滚已修。

## 边界

UI密钥/OAuth使用runtime-local隔离模拟API夹具，不接真实账号、未读取token、不执行生产授权或付费生成。浏览器截图外部字体受夹具CSP限制。未覆盖真实客户端consent/token/tools全链路。此轮未改后端/API/config/deployment，未重新跑隔离API smoke；此前canonical工具变更保留、未修改。代码未提交/推送/部署。

npm install报告16个依赖审计项（7low/1moderate/8high），未执行越界audit fix。构建仍有既有Scalar大chunk提示。
