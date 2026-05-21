---
name: dev-api-smoke
description: |
  Local real API smoke check. Use after backend, API, config, Docker, deployment,
  or worker changes. Runs the repository smoke script against BASE_URL, defaulting
  to http://localhost:8080. Triggers on: smoke test, real API test, local API,
  readyz, 启动项目, 真实接口测试, 联调.
argument-hint: '[BASE_URL=http://localhost:8080]'
user-invocable: true
category: workflow
---

# dev-api-smoke

Run:

```bash
./scripts/workflow/api-smoke.sh
```

If the API is not running, start the required local service first using the repository runbook or `make dev`.

