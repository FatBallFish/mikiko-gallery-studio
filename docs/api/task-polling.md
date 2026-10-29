# 任务状态与事件

## 按 ID 查询任务

创建后保存任务 ID。图片 ID 位于 `data.id`，视频 ID 位于 `data.task.id`；视频还应检查 `data.replayed`。

```bash
curl "https://mikiko.studio/api/open/v1/image-tasks/$TASK_ID" \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

- 根据任务状态和结果明细判断完成情况。
- 图片与视频的终态、取消能力可能不同，以接口参考为准。
- 已接受取消请求不代表上游已经停止。
- 区分生成失败、保存失败和部分成功。

## 观察生命周期事件

```bash
curl -N "https://mikiko.studio/api/open/v1/image-tasks/$TASK_ID/events" \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

视频使用对应的 `video-tasks` 路径。事件连接只用于观察；断线后重新查询任务。当前不承诺历史事件重放或 `Last-Event-ID` 恢复。

## 读取结果

结果就绪后请求临时访问链接。保存任务和资产 ID，链接过期后续取，不把断线当作任务失败。
