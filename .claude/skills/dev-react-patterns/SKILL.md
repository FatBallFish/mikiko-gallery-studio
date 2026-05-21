---
name: dev-react-patterns
description: |
  React/frontend implementation guardrails. Load before editing React, TypeScript,
  CSS, shared frontend API types, mock data, Vite config, or user/admin web apps.
  Triggers on: React, frontend, UI, CSS, TypeScript, web/user, web/admin, 前端,
  页面, 样式.
user-invocable: true
category: implement
---

# dev-react-patterns

Before editing frontend code:

1. Confirm `.coding-context.json` exists.
2. Read the requirement and technical-design sources listed in it.
3. Keep user/admin app behavior aligned with `web/shared/api-types.ts`.
4. Do not let mock data drift from intended API contracts.
5. Maintain responsive layout, accessible controls, and no text overlap.
6. After edits, run:

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

