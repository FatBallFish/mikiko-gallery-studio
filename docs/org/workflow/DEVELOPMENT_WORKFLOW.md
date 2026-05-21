# Development Workflow

## 1. Start Coding

Run:

```bash
./scripts/workflow/start-coding.sh --task "<task summary>"
```

The script searches repository docs for requirement and technical-design sources. It writes `.coding-context.json` only when both are found or explicitly supplied.

If discovery fails, do not code. Ask the user for the missing requirement or technical-design source.

## 2. Implement

Use the relevant implementation skill:

- `dev-go-patterns` before editing Go files
- `dev-react-patterns` before editing React/TypeScript/CSS files

For work with independent subtasks, use sub-agents when the environment supports them. Keep write ownership disjoint.

## 3. Verify

Run:

```bash
./scripts/workflow/verify.sh
```

Fix failures before continuing.

## 4. Review

Run:

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Fix BLOCK findings and regenerate the review marker.

## 5. Local API Smoke

If backend/API/config/deployment changed, run:

```bash
./scripts/workflow/api-smoke.sh
```

## 6. Ship

Use:

```bash
./scripts/workflow/ship-guard.sh
```

This runs verification, review gate, and API smoke when needed.

