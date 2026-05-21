---
name: dev-ship
description: |
  Final delivery workflow. Runs verify, committed-scope review gate, stale gate
  validation, and API smoke when backend/API/config/deployment changed. Use before
  pushing, opening PR, or saying code is complete. Triggers on: ship, push, open PR,
  create PR, ready to merge, submit code, 交付, 推代码, 开 PR, 提交.
argument-hint: ""
user-invocable: true
category: workflow
---

# dev-ship

Run:

```bash
./scripts/workflow/ship-guard.sh
```

If any step fails, fix the failure and rerun the whole guard. Do not push or open a PR while the guard is failing.

