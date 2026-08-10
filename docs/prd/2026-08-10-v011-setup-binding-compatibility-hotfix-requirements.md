# v0.0.11 Setup Binding Compatibility Hotfix Requirements

## Background

An installation created before `v0.0.10` can fail while upgrading with
`mgsctl v0.0.10`. The release manifest and all five target images resolve and
download correctly, and the database migration commits the target application
version, but the post-migration Setup binding reconciliation rejects the stored
digest. `mgsctl` then restores the previous runtime configuration and deployment
manifest.

The production incident was reproduced on an installation whose stored binding
matches the canonical digest generated before these runtime fields existed:

- `PIC_GALLERY_DOCS_URL`
- `PIC_GALLERY_DOCS_PROBE_URL`

The current reconciler only accepts the current canonical digest or the legacy
digest that included release identity fields. It does not accept a canonical
digest from the immediately preceding runtime field set.

## Requirements

1. `mgsctl upgrade` must accept a completed Setup binding created before the two
   documentation runtime fields existed.
2. Compatibility must be fail-closed. Historical fields may be omitted from
   digest verification only when the newly introduced fields contain the exact
   defaults inserted by the runtime renderer.
3. Installation ID, Setup operation ID, config revision, runtime schema version,
   database binding, and local install-state proof must continue to match.
4. Database and install-state digests must be migrated to the current canonical
   digest through the existing compare-and-swap and rollback behavior.
5. A retry after the database schema/application-version migration already
   committed must complete idempotently.
6. Current canonical bindings, release-field legacy bindings, corrupt bindings,
   and mismatched database/local bindings must retain their existing behavior.
7. `v0.0.10` release assets and tag are immutable. The fix must ship under the
   next SemVer patch tag.

## Acceptance Criteria

- A test binding computed from the pre-documentation-field schema is recognized
  and migrated to the current canonical digest.
- The same binding is rejected if either documentation field differs from its
  renderer-inserted default.
- Database and install-state mismatches remain rejected.
- Existing Setup, migration, mgsctl upgrade, repository verification, review
  gate, API smoke, and Docker upgrade E2E checks pass.
- A PR is merged to `main`, a new annotated SemVer tag is pushed, and the tagged
  release workflow completes successfully.

## Non-Goals

- Do not ignore arbitrary future runtime fields during digest verification.
- Do not mutate production binding records manually.
- Do not rewrite or republish `v0.0.10`.
