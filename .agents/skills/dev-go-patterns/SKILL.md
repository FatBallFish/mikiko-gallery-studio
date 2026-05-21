---
name: dev-go-patterns
description: |
  Go implementation guardrails. Load before editing `.go` files or changing Go
  handlers, services, repositories, workers, config, errors, tests, or build logic.
  Triggers on: Go code, handler, service, worker, repository, go test, 写 Go,
  改后端, 改接口.
user-invocable: true
category: implement
---

# dev-go-patterns

Before editing Go code:

1. Confirm `.coding-context.json` exists.
2. Read the requirement and technical-design sources listed in it.
3. Follow these rules:
   - pass `context.Context` through request and worker paths
   - wrap errors with `%w` when preserving cause
   - avoid naked goroutines; use cancellation and recovery
   - keep HTTP JSON responses consistent through `pkg/httpx`
   - keep config loading validated and tested
   - add or update Go tests for changed behavior
4. After edits, run `./scripts/workflow/verify.sh`.

