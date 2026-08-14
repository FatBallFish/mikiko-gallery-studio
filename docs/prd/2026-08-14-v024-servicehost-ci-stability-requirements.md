# v0.0.24 ServiceHost CI 稳定性修复需求

## 背景

`v0.0.23` Tagged Release 在 `verify` 作业中失败。失败用例为
`TestServiceHostRestartsDocumentedExitCodeUntilStopped`，GitHub Runner 读取到空的重启计数文件，导致后续镜像和发布作业未启动。

## 根因

测试通过固定的 `120ms` 上下文超时判断子进程已经至少重启两次。Runner 负载较高时，该时长不能保证子进程完成两次启动；同时测试子进程使用 `os.WriteFile` 直接覆盖计数文件，取消上下文可能在文件截断后、内容写入前终止进程，使父进程读取到空文件。

## 需求

1. 测试应等待“至少完成两次重启”这一业务条件，不以固定短时间睡眠推断条件已满足。
2. 测试必须设置明确的最大等待时间，失败时输出可定位的超时信息。
3. 测试的重启记录必须采用不可变、无覆盖写入的方式，父进程不能观察到写入中间态。
4. 不修改 ServiceHost 的生产重启、退出码或取消语义。
5. 修复需在本地重复测试、全仓验证、review gate 和 API smoke 中通过。
6. 修复通过 PR 合入 `main` 后发布新 tag；不得移动或覆盖 `v0.0.23`。

## 验收标准

- ServiceHost 目标用例连续运行至少 100 次通过。
- Linux 容器内目标用例连续运行至少 100 次通过。
- `./scripts/workflow/verify.sh` 通过。
- Tagged Release 的 `verify`、全部多架构镜像、原生/离线制品、GitHub Release 与 `latest` 推广全部成功。
