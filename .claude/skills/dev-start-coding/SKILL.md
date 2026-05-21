---
name: dev-start-coding
description: |
  Required pre-coding workflow. Use before non-trivial code, workflow, config,
  API, backend, frontend, or test changes. Searches the repository for matching
  requirement and technical-design sources before coding. If sources are missing,
  blocks coding and asks the user to provide them. Writes `.coding-context.json`
  for downstream verify/review/ship steps. Triggers on: start coding, implement,
  build this, fix this, 开工, 开始实现, 写代码, 修复, 按这个做.
argument-hint: '--task "<summary>" [--track lightweight|heavyweight|auto]'
user-invocable: true
category: workflow
---

# dev-start-coding

Run this before editing production code.

## Required action

Execute:

```bash
./scripts/workflow/start-coding.sh --task "<task summary>"
```

Use `--track heavyweight` only when the task clearly hits heavyweight criteria in `docs/org/workflow/WORKFLOW_TRACKS.md`.

## Blocking rule

The script must find both:

- a requirement source
- a technical-design source

Do not assume fixed filenames. The script searches repository docs and writes `.workflow/source-discovery.json`.

If the script exits `2`, stop coding and ask the user for the missing source. Do not create implementation code until `.coding-context.json` exists.

## Execution discipline

After context exists, continue through implementation, verification, review, and smoke checks unless blocked. Do not stop after partial progress with only a "next I will..." report.

Use sub-agents for independent subtasks when available and when write scopes can be kept separate.

