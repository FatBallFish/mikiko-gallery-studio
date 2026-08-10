# v0.0.11 Setup Binding Compatibility Hotfix Technical Design

## Root Cause

`RenderRuntimeEnv` materializes defaults for fields missing from an older runtime
document. In `v0.0.10`, this adds `PIC_GALLERY_DOCS_URL=/developer-docs/` and an
empty `PIC_GALLERY_DOCS_PROBE_URL` before the upgrade migration runs.

`ReconcileLegacyCompletedBinding` currently computes only:

1. the current canonical digest, excluding release identity fields; and
2. the old digest algorithm, including release identity fields.

A pre-`v0.0.10` canonical binding excluded release identity fields but also did
not contain either documentation field. It therefore matches neither candidate.
The database migration runs before reconciliation, so the failure can leave the
database application version advanced while mgsctl restores the old runtime.

## Considered Approaches

### 1. Manually rewrite production digests

Rejected. This bypasses product invariants, is difficult to audit, and does not
protect other installations.

### 2. Ignore all fields unknown to a stored binding

Rejected. The stored digest does not encode its field list, and broad omission
would weaken tamper detection for future security-sensitive fields.

### 3. Allowlisted historical digest profiles

Selected. Compute narrowly defined historical candidates only when every newly
introduced field has the exact value that the trusted renderer inserts. Preserve
all identity validation and migrate accepted records through existing CAS APIs.

## Design

Add one compatibility profile for the runtime schema immediately preceding the
documentation fields. The profile omits:

- `PIC_GALLERY_DOCS_URL`, only when its value is `/developer-docs/`;
- `PIC_GALLERY_DOCS_PROBE_URL`, only when its value is empty.

For that profile, calculate both canonical and release-field legacy candidates.
Together with the existing current canonical and current legacy candidates, the
reconciler classifies the local proof and database binding.

The reconciliation rules are:

1. Reject a digest that matches no trusted candidate.
2. If both sides are non-current, require them to contain the same digest.
3. Update the database binding to the current canonical digest with its existing
   expected-digest CAS predicate.
4. Update `install-state.json` to the same digest with its existing identity and
   state validation.
5. If the local state update fails after a database update, restore the exact
   previous database digest, not a recomputed candidate.
6. Treat already-current sides as idempotent partial reconciliation.

No runtime schema version is incremented because the fields already shipped in
`v0.0.10`; this hotfix changes only upgrade compatibility.

## Security

The compatibility path is gated by exact default values and a fixed allowlist.
It never accepts omitted secret, database, storage, identity, or user-editable
Setup fields. Operation ID, installation ID, config revision, and both durable
binding locations remain mandatory. Constant-time digest comparison remains in
use.

## Testing

- Add a failing unit test for the exact pre-documentation canonical digest.
- Verify successful CAS migration of both database and local state.
- Verify a non-default documentation URL disables the compatibility profile.
- Verify divergent historical database/state digests fail closed.
- Run focused Setup and app migration tests, full repository verification,
  committed-scope review gate, API smoke, and Docker upgrade E2E.

## Release

Merge a dedicated hotfix PR into `main`, create the next annotated patch tag,
wait for the tagged release workflow, and independently verify Release checksums,
image digests, OCI version/revision labels, and `latest` promotion.
