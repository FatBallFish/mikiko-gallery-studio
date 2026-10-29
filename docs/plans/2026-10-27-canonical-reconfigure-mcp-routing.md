# mikiko.studio canonical 配置与 MCP 网关修复

## 需求与授权

用户明确要求将真实 HTTPS 地址 https://mikiko.studio 配置到现有生产安装，并修正 MCP/OAuth 错误转发。授权实施必要代码、工具及该实例配置部署；不修改其他服务域名、不新增付费生成、不读取或输出秘密、不绕过安装绑定。

## 已核实依据

- docs/plans/2026-10-24-unified-api-mcp-design.md：可信 Runtime.PublicAPIURL、OAuth issuer/resource、网关协议路由。
- docs/tech/2026-08-10-v011-setup-binding-compatibility-hotfix-tech-design.md：旧 proof 校验、CAS、回滚，不手改数据库摘要。
- mikiko.studio HTTPS 已指向 Cloudflare → origin OpenResty → Gallery gateway，TLS 有效；内层旧 nginx 资产未含协议路由。

## 技术方案

新增受保护的 mgsctl reconfigure 公共地址更新入口，仅允许显式 PUBLIC_API_URL 修改，复用配置校验/渲染和 canonical digest。修改前核对完整旧安装身份、本地及DB绑定；配置revision递增，用expected-old revision+digest原子CAS推进DB和本地状态，保留installation/admin identity和秘密。提供私有持久恢复记录、失败回滚/重入策略；停服务时进行一致性转换，未匹配proof拒绝操作。不得复用历史兼容通道接纳任意新摘要。

为正式工具补齐受manifest已记录hash保护的managed nginx资产刷新，保留用户定制检测，更新manifest文件hash，nginx配置校验后重建/重载gateway，失败恢复旧资产。现有源码协议locations已正确，生产需实际部署到绑定文件。两块实现保持独立文件写范围，必要接口由owner协调。

## 验收

聚焦Go测试覆盖输入URL校验、旧绑定拒绝、CAS冲突、revision更新、失败恢复、幂等与managed资产自定义保护。全verify、独立复核、适用隔离API smoke。备份生产配置/绑定/DB与proxy后通过正式mgsctl执行；不直接DB修值。公网验证developer-endpoints、RFC9728/RFC8414 JSON、401 challenge、DCR/PKCE授权跳转，若可安全登录则工具只读调用与撤权；不宣称未经验证的客户端成功。核对健康、绑定身份、其他服务和队列保持正常。
