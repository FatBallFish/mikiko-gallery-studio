---
name: dev-handoff
description: |
  Create a concise handoff when context is long, the session has been compacted,
  or the user wants to continue in a new session. Must include current task,
  requirement/design sources, .coding-context.json status, changed files, remaining
  work, verification results, review gate status, and blockers. Triggers on:
  handoff, new session, context too long, 接力, 切会话, 换窗口.
argument-hint: ""
user-invocable: true
category: workflow
---

# dev-handoff

Summarize:

- current task and exact requirement/design sources
- `.coding-context.json` track and approval state
- files changed
- completed work
- remaining work
- verification and review results
- blockers and commands to resume

Write to a dated file under `docs/plans/` only if the user asks for a persistent artifact.

