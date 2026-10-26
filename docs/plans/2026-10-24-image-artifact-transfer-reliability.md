# Image artifact transfer reliability follow-up

## Requirement

Owner request: fix image tasks remaining in result persistence after upstream generation has completed. Existing recovery must never regenerate a paid result. Downloads need bounded resume, limited result persistence concurrency, and truthful download/recovery progress.

## Evidence

Two local tasks completed upstream generation in 22–28 seconds. Their result URLs returned HTTP 200, but image downloads transferred about 10–21 KiB/s. After 300 seconds, downloads failed at 95.9% and 98.4%; the current reader discarded partial bytes. Result persistence was serial, and claimed recovery tasks displayed the provider stage instead of artifact persistence.

## Implementation constraints

- Preserve the encrypted provider-success checkpoint, deterministic result identity, storage verification, leases, recovery retry limit, and billing idempotency.
- Retain partial download bytes during a bounded transfer operation. Resume only with validated byte ranges and representation validators; never concatenate an ignored/mismatched range response.
- Bound bytes, request/retry count, elapsed transfer time, and per-task result persistence concurrency. Propagate cancellation and close response bodies.
- Preserve result ordering even when saving concurrently.
- Persist throttled byte progress through the owned progress update, without replacing lease, billing, or recovery state.
- Claiming and retrying an artifact task must display persistence/recovery rather than provider generation.
- Process restart retains existing encrypted recovery behavior; in-flight partial bytes are not promised to survive restart.

## Validation

Exercise interrupted reads, validated resume, ignored/invalid ranges and changed representations, size limits, cancellation, concurrent save ordering/limits, claimed recovery progress, heartbeat preservation, and no repeated provider calls. Run repository verification and isolated API smoke, then rebuild/restart the local runtime for acceptance.
