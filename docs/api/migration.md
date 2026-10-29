# 迁移到统一 API

## 替换认证

旧签名开发者接口已直接替换，无兼容层。每次开发者请求发送 Bearer SK：

```bash
curl https://mikiko.studio/api/open/v1/me \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

- `MIKIKO_API_KEY` 填创建或轮换时一次显示的 SK（`sk-`）。
- AK（`ak-`）只是密钥标识，不能代替 SK。
- 移除旧 access-key、时间戳、body 摘要、签名请求头和签名代码。
- SK 保存到服务端秘密管理器。MCP 使用独立[浏览器 OAuth](mcp.md)。

## 替换接口路径

新地址统一以 `https://mikiko.studio` 开头：

| 旧开发者路径 | 新接口 |
| --- | --- |
| `GET /api/open/image/v1/capabilities` | `GET https://mikiko.studio/api/open/v1/image-models` |
| `GET /api/open/image/v1/balance` | `GET https://mikiko.studio/api/open/v1/me/balance` |
| `GET /api/open/image/v1/estimate` | `POST https://mikiko.studio/api/open/v1/image-tasks/preflight`，提交完整 JSON |
| `POST /api/open/image/v1/tasks` | `POST https://mikiko.studio/api/open/v1/image-tasks` |
| `GET /api/open/image/v1/tasks/{task_id}` | `GET https://mikiko.studio/api/open/v1/image-tasks/{task_id}` |
| 旧参考上传初始化和完成 | `https://mikiko.studio/api/open/v1/uploads` 会话流程 |
| 旧参考资产查询 | `GET https://mikiko.studio/api/open/v1/assets/{asset_id}` |

公开图库、支付回调和供应商回调保留各自路径与认证。浏览器和管理员会话不属于这次迁移。

## 更新请求结构

1. 根据[接口参考](../../web/docs/public/openapi/openapi.yaml)重新生成类型，避免沿用旧请求结构。
2. 发现模型允许的参数，选择自己的项目。
3. 提交完整预检请求，取得报价后加入幂等键创建任务。
4. 上传改用会话、分片状态和校验和流程。
5. 参数变化后重新预检；积分保留为五位小数字符串。

## 检查权限与恢复

- `account:read`、`models:read`、图片/视频读写权限按操作授予。
- 视频删除及资产、上传、项目、分组的 read/write/delete 权限需要显式授权。
- 已有图片密钥不会自动获得视频或删除权限；过期、RPM 和额度仍适用。
- 保存任务与资产 ID，重试复用相同幂等键和业务输入。
- 访问链接过期后重新获取；不依赖历史事件重放或部分图片流。
