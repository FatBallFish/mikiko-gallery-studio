# 模型能力

## 发现可用模型

```bash
curl https://mikiko.studio/api/open/v1/image-models \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
curl https://mikiko.studio/api/open/v1/video-models \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

## 按能力生成表单和请求

检查模型允许的任务类型、尺寸、输出数量、参考素材和其他参数。

- 图片原生能力见 `native_image_profile`。
- 视频参数受任务类型及可用组合约束。
- 账号、scope 和路由配置变化可能改变可用选项。
- 模型声明能力后，执行仍需通过价格、权限和预检校验。

不同模型不共享一组固定参数。提交前使用[完整请求预检](estimates.md)。
