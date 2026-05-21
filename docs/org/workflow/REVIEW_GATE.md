# Review Gate

`scripts/workflow/review-local.sh` writes `.review/gate.json`.

Required fields:

- `schema_version`
- `decision`: `PASS` or `BLOCK`
- `scope`: `committed` for push/PR gates
- `tree_sha`: current `HEAD` tree
- `generated_at`
- `checks`
- `findings`

`scripts/workflow/check-review-gate.sh` accepts only:

- `decision=PASS`
- `scope=committed`
- `tree_sha` equal to current `HEAD^{tree}`

This prevents stale review results from satisfying a later push.

