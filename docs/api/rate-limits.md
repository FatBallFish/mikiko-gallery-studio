# 限速与幂等

## 控制请求速率

- 账号与密钥的 RPM、总额度和日额度均适用。
- 429 时读取 `Retry-After`，等待后逐步重试。
- 限制并发，避免重试造成更多限流。

## 避免重复创建

同一次业务操作的网络重试，复用相同幂等键和相同请求。新操作使用新键。

```http
POST https://mikiko.studio/api/open/v1/image-tasks
Authorization: Bearer <创建或轮换时一次显示的 SK>
Content-Type: application/json
Idempotency-Key: <本次操作的唯一键>
```

图片和视频创建幂等键为 1–128 字符；请求头优先于 body 中的 `idempotency_key`。相同身份、键和业务输入重放已有任务，输入冲突返回 409。

## 创建超时后恢复

先查询已知任务 ID，再决定是否重试。报价过期先重新预检，不盲目换键再次创建。幂等保留时间和具体响应以[接口参考](../../web/docs/public/openapi/openapi.yaml)为准。
