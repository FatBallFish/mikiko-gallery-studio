# Repository AI Workflow

This repository uses a local AI development workflow for Codex and Claude.

## First-time Setup

Run once after clone:

```bash
./scripts/workflow/install-hooks.sh
```

This sets `git config core.hooksPath .githooks` and makes repository hooks executable.

Codex users must trust the project when prompted, otherwise `.codex/hooks.json` will not load. Claude users use `.claude/settings.json`. Both agent hook configs call the same scripts in `.hook-scripts/`.

## Required Workflow

Before any non-trivial coding task, use `dev-start-coding`.

The workflow must first establish requirement and technical-design sources. The agent must search the repository for matching docs; it must not assume fixed filenames. If no matching requirement or design source is found, coding is blocked until the user provides one.

Required sources can be:

- requirement / PRD / issue / acceptance criteria docs
- technical design / architecture / implementation plan docs
- explicit user-provided links or text saved into `.coding-context.json`

## Generic Skills

Skills are mirrored for both Codex and Claude:

- `.agents/skills/dev-start-coding/SKILL.md`
- `.agents/skills/dev-verify/SKILL.md`
- `.agents/skills/dev-review-gate/SKILL.md`
- `.agents/skills/dev-api-smoke/SKILL.md`
- `.agents/skills/dev-ship/SKILL.md`
- `.agents/skills/dev-handoff/SKILL.md`
- `.agents/skills/dev-go-patterns/SKILL.md`
- `.agents/skills/dev-react-patterns/SKILL.md`

Claude-compatible copies live under `.claude/skills/`.

## Hard Rules

- Do not code without `.coding-context.json`, unless the change only edits workflow/docs or the user is explicitly creating missing requirements/design.
- Do not mark work complete after partial implementation. Continue through implementation, verification, review, and local smoke where applicable unless blocked.
- If a task touches Go, follow `dev-go-patterns` before editing.
- If a task touches React, follow `dev-react-patterns` before editing.
- Before push or PR, run `dev-ship` or equivalent scripts:
  - `./scripts/workflow/verify.sh`
  - `./scripts/workflow/review-local.sh --scope committed`
  - `./scripts/workflow/check-review-gate.sh`
  - `./scripts/workflow/api-smoke.sh` when backend/API/config/deployment changed

## Verification Commands

Repository verification is centralized in:

```bash
./scripts/workflow/verify.sh
```

It runs:

- `go test ./...`
- `go vet ./...`
- `npm --prefix web/user run typecheck`
- `npm --prefix web/user run build`
- `npm --prefix web/admin run typecheck`
- `npm --prefix web/admin run build`

## Review Gate

Before push/PR, generate a committed-scope review marker:

```bash
./scripts/workflow/review-local.sh --scope committed
```

The marker is `.review/gate.json`. It is valid only when it is `PASS`, scope is `committed`, and its tree SHA matches the current `HEAD` tree. `pre-push` enforces this.

## Branch Policy

Direct commits to `main` are blocked by `pre-commit`, except workflow bootstrap commits made by a human after review.

