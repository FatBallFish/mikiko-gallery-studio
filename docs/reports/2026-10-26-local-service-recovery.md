# 本地服务恢复记录

日期：2026-10-26。用户授权保数据重新部署。

## 结果

API已恢复监听127.0.0.1:8091，/healthz返回status=ok，/readyz返回status=ready。用户端localhost:5273已恢复读取原余额512.064及历史视频任务；管理端localhost:5274使用原管理员账号重新登录成功。未启动Worker、未提交付费任务。

## 恢复方式

将现存运行副本中WORKER_ROLES还原为image,video,media,cleanup、WORKER_IMAGE_CONCURRENCY还原为4后，使用官方CanonicalRequestDigest计算与数据库原安装绑定精确匹配。未修改数据库安装元数据，不重绑定、不清库、不轮换密钥。根据数据库原CommitProof，使用官方Validate/StateStore.FinalizeCommit及Service.VerifyCompletedBinding恢复并验证本地安装状态。保留原identity、operation、revision、管理员和时间。

持久配置：runtime-local/e2e/config/runtime.env；配对state：同目录install-state.json。目录0700、文件0600，Git忽略。恢复工具与配置不纳入提交。

备份：runtime-local/recovery-20261026-013331，包含完整数据库dump及现存配置副本、校验manifest。pg_restore --list校验成功。原加密配置解密验证成功，默认存储put/get/delete探针成功。

API使用/tmp/e2e-api-reference-current构建产物，APP_ENV_FILE指向上述持久绝对路径，日志/tmp/e2e-logs/api-recovered.log。恢复中没有修改产品代码；此前引用修复全量验证与隔离冒烟结果另见2026-10-26-reference-admission-model-stability.md。

## 边界

本轮完成服务恢复与登录/历史数据读取验证，不宣称已进行真实拆层生成、付费上游测试或全部引用修复浏览器验收。未启动Worker，因此后台可用不等同于新生成任务会被消费。未提交、推送或远程部署。
