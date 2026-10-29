# 图片编辑

## 准备参考素材

1. 发现支持图片编辑的模型。
2. 通过[上传会话](reference-images.md)上传自己的文件，保存资产 ID。
3. 检查模型允许的参考数量、格式与文件大小。

## 预检与创建

向 `POST https://mikiko.studio/api/open/v1/image-tasks/preflight` 提交完整请求，选择 `operation: edit`，包含自己的项目、模型、提示词和参考资产。

预检通过后，保留相同业务输入，加入报价和幂等键，再发送到 `POST https://mikiko.studio/api/open/v1/image-tasks`。

- 不能引用其他用户的资产。
- 模型未声明 mask 能力时不要提交 mask。
- 更换模型、参考图或参数后重新预检。

原生字段片段见[原生 API](native-api.md)，必填项见[接口参考](../../web/docs/public/openapi/openapi.yaml)。
