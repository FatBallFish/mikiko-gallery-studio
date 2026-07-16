# User Experience Compatibility Merge Design

## Context

The current branch contains artifact recovery, storage unification, authentication
TTL, and admin session refresh fixes that are not present on
`codex/admin-runtime-monitoring`. That branch contains a newer Luminous Vault user
experience, but its final baseline also changes the generation API, admin runtime,
deployment topology, and OpenAPI documentation.

The merge must restore the latest user-facing experience without replacing the
current branch's backend contract or regressing its operational safeguards.

## Scope

Restore the latest user application experience for:

- landing and authentication;
- application shell and responsive navigation;
- home, workspace, gallery, and public gallery;
- checkout, API keys, profile, and settings;
- light and dark themes, motion preferences, icons, and landing media;
- user-facing contracts and build checks needed by those surfaces.

Preserve the existing in-app API documentation page. The independent `web/docs`
service and `/developer-docs/` deployment route are excluded because their copied
OpenAPI contract describes backend capabilities that are not present on this
branch. A separate change can introduce that service after its specification is
synchronized with the deployed backend.

## Source And Merge Strategy

Use commit `0ea714a` as the authoritative final snapshot for `web/user`, because
no later commit on `codex/admin-runtime-monitoring` changes that tree. Bring over
the user tree, assets, dependencies, Luminous Vault theme tokens, and relevant
contract scripts, then manually reconcile shared code.

Do not merge or cherry-pick `0ea714a` wholesale. In particular:

- keep the current `web/shared/http-client.ts` authentication replay and request
  cancellation behavior;
- keep the current `web/shared/admin-api.ts` and admin session contract;
- preserve artifact diagnostics, platform loss, recovery, storage configuration,
  and refresh response types in `web/shared/api-types.ts`;
- reconcile only user-facing types and API transformations required by the new
  application;
- preserve the current embedded `DocsPage` and remove external docs wiring from
  the imported application shell.

## Generation Compatibility

The current backend accepts `requested_quality` and `requested_size`. The newer
workspace models generation around `base_resolution`, `size_mode`, `quality`,
`output_format`, `output_compression`, and `moderation`.

The user application may use the newer view model internally, but the shared API
adapter must continue sending the current backend request shape:

- map the selected base resolution to `requested_quality`;
- derive `requested_size` from the selected aspect ratio and resolution using the
  existing image-size helper;
- normalize legacy task responses back into the view model;
- expose only controls supported by current capability responses;
- hide unsupported output, compression, moderation, and free-pixel controls
  rather than implying that they affect generation.

Estimate invalidation, task creation, task continuation, and task history must all
use the same normalized request state.

## Runtime And Error Handling

The user application continues to use the current shared HTTP client for access
token refresh, single-flight replay, and unauthorized handling. Imported UI code
must not weaken admin refresh serialization or remove the current session-version
checks.

Protected routes retain their return route, public image identifier, and workspace
task identifier through login. Failed image media can retry loading, failed API
requests produce bounded user feedback, and unsupported generation settings are
not submitted silently.

The user-web image must include any contract script invoked by its build. Runtime
environment changes are limited to what the restored user application actually
uses; no dangling `/developer-docs/` link is introduced.

## Verification

Completion requires evidence from all of the following:

1. Every user contract is discovered and passes, including Luminous Vault CSS,
   landing wiring, login, shell, workspace, gallery, and route contracts.
2. User and admin typechecks and production builds pass.
3. The repository verification script, `git diff --check`, committed-scope review
   gate, and API smoke pass.
4. A real local API flow covers readiness, login, capability loading, estimate,
   task creation, and task retrieval using the current request contract.
5. Browser checks cover desktop and mobile landing, login, workspace, gallery,
   light and dark themes, protected-route restoration, and expired-session
   behavior without console errors or horizontal overflow.
6. Docker user-web build and the rebuilt local stack serve the restored assets.

## Commit Structure

Keep the existing operational commits unchanged and add focused commits for:

1. this compatibility design and implementation plan;
2. Luminous Vault foundations and dependencies;
3. landing, login, shell, gallery, and account experiences;
4. workspace compatibility with the current generation API;
5. contracts and user-web deployment wiring.

The resulting branch is pushed as-is and opened as one PR to `main`, with the PR
description separating the pre-existing operational fixes from the restored user
experience.
