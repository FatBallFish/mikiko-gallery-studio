---
name: dev-api-smoke
description: |
  Use after backend, API, config, Docker, deployment, or worker changes that need
  a local real API contract check. Triggers on: smoke test, real API test, local API,
  readyz, 启动项目, 真实接口测试, 联调.
argument-hint: '[BASE_URL=http://127.0.0.1:18081]'
user-invocable: true
category: workflow
---

# dev-api-smoke

Run:

```bash
./scripts/workflow/api-smoke.sh
```

Prerequisites are Bash, `curl`, Python 3, Go, a running Docker daemon, and access to `postgres:16-alpine` and `redis:7-alpine`.

The script starts and cleans up its own API, Worker, fake provider, PostgreSQL, and Redis. `BASE_URL` only accepts `http://127.0.0.1:<port>` or `http://localhost:<port>` with an explicit free port and no URL suffix; do not start or target an existing API.
