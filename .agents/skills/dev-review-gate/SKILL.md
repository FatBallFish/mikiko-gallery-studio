---
name: dev-review-gate
description: |
  Local pre-PR/pre-push review gate. Use before marking code complete, before PR,
  before push, or after implementation. Checks coding context, requirement/design
  source availability, heavyweight approval status, secrets, and formatting. Writes
  `.review/gate.json`; pre-push accepts only a fresh PASS marker for the current
  HEAD tree. Triggers on: review, code review, pre-push, PR ready, done, 完成了,
  代码写完了, 准备提交.
argument-hint: '[--scope committed|all]'
user-invocable: true
category: review
---

# dev-review-gate

For push/PR readiness, run:

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

If the gate reports `BLOCK`, fix all findings and regenerate the marker.

Do not satisfy the gate with a stale `.review/gate.json`; `tree_sha` must match current `HEAD^{tree}`.

