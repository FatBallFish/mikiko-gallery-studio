---
name: dev-verify
description: |
  Repository verification entrypoint. Runs the shared verification script for Go
  tests/vet and React typecheck/build for user/admin apps. Use when the user asks
  to test, verify, self-check, run checks, 跑测试, 验证, 测一下.
argument-hint: ""
user-invocable: true
category: workflow
---

# dev-verify

Run:

```bash
./scripts/workflow/verify.sh
```

Fix every failure before claiming work is complete.

