# Post-release Remediation Requirements

## Status

- Date: 2026-08-02
- Source: post-release production verification feedback supplied by the repository owner
- Approved approach: contract-driven vertical remediation (approach 2)
- Stripe scope: full PaymentIntent + Payment Element + webhook + query + refund flow
- Implementation status: planning only; no product code is authorized by this document

## Goal

Close the deployment, cashier, administration, and user-experience defects found after the first tagged releases without replacing established runtime data, exposing secrets, or broadening the work into a platform rewrite.

## In Scope

### Installer and mgsctl

1. A failed `scripts/install.sh install` or `mgsctl install` run can be repeated safely.
2. TUI overwrite applies newly entered port and deployment values instead of reusing stale generated configuration.
3. Linux installation makes `mgsctl` available in future shells without manual PATH editing.

### Existing user and admin experiences

4. Gallery image lightbox renders above the image-detail dialog.
5. Package create/edit actions open a functional editor and persist through existing cashier plan APIs.
6. JeePay merchant number, application ID, and signing key survive create and edit.
7. Provider editors no longer expose generic secret JSON or raw provider-config JSON inputs.
8. Callback inputs accept base origins and append platform-owned callback routes automatically.
9. Provider fields visibly distinguish required and optional inputs.
10. Stripe is available as a complete payment provider.
11. System settings expose the CNY-per-point configuration, defaulting to `0.3125` CNY per point when no override exists.
12. The user points page exposes redeem-code redemption at its bottom.

### Newly reported defects

13. New users receive the normalized email local part as their default nickname instead of every database-backed registration becoming `user-1`.
14. Text-model row actions use legible icons and accessible tooltips.
15. A valid saved text-model configuration becomes usable by prompt optimization, with an explicit and understandable default-model state.

## Acceptance Criteria

### Installation recovery

- A recognized pending installation with an unchanged plan resumes deployment without regenerating identity or secrets.
- A recognized pending installation with changed ports requires explicit overwrite and then rewrites `runtime.env`, deployment assets, and the manifest with the new values.
- Overwrite preserves `INSTALLATION_ID`, setup token, application security keys, managed-service credentials, `data/`, `logs/`, and Docker volumes.
- Docker reconciliation uses the original Compose project and recreates services so changed API and gateway ports take effect.
- Generated files missing from an otherwise recognized pending installation can be regenerated.
- Completed installations still reject install overwrite and direct users to upgrade.
- Unknown files or unowned directories are never deleted automatically.
- Repeated PATH setup does not duplicate profile entries.

### Payment configuration

- Public merchant identifiers are sent in `config`; actual credentials are sent in `secrets`.
- Editing while leaving a saved secret blank preserves the existing encrypted value.
- The admin response remains redacted and never returns secret plaintext.
- New provider dialogs default callback bases from `window.location.origin`.
- Saving produces `/api/open/image/v1/payments/webhooks/{provider_type}` and `/#/checkout` routes.
- Existing full callback URLs remain editable through a backward-compatible base-origin projection.
- Missing required provider values fail during provider-instance save with field-specific feedback.

### Stripe

- Admins can configure Stripe publishable key, secret key, and webhook signing secret without plaintext secret replay.
- Creating a Stripe order creates an idempotent CNY PaymentIntent and returns a client secret suitable for Payment Element.
- The user can confirm payment through Stripe Payment Element and then observe the existing order polling/status flow.
- Stripe webhook signatures are verified against the exact raw request body.
- `payment_intent.succeeded` credits the order once; repeated events are idempotent.
- Payment failure, query, full/partial refund, and refund query follow existing cashier status and audit contracts.

### Users and text models

- `alice@example.com` creates a user whose initial nickname is `alice`.
- Existing users and manually edited nicknames are not migrated or overwritten.
- The first eligible enabled text model under an enabled account becomes the default when no default exists.
- Existing installations with exactly one eligible model self-heal the missing default selection.
- Multiple eligible models without a default produce an actionable configuration error instead of a generic 404.
- Admin UI always shows whether a default optimization model exists.
- Text-model action targets are at least 40 by 40 CSS pixels, icons are 18-20 pixels, and tooltips work on hover and keyboard focus.

## Non-functional Requirements

- Preserve existing API paths unless a new Stripe-specific response field is required.
- Keep secret values out of logs, audits, DOM text, error bodies, and test snapshots.
- Use transaction or idempotency boundaries for default-model selection and Stripe state changes.
- Add regression tests before implementation for every confirmed root cause.
- Run repository verification, committed-scope review gate, and isolated API smoke before push or PR.

## Out of Scope

- Renaming or backfilling existing `user-N` nicknames.
- Replacing the cashier architecture with a general plugin framework.
- Supporting Stripe Checkout instead of Payment Element.
- Supporting non-CNY cashier accounting in this remediation.
- Overwriting completed installations through `install`.
- Automatically deleting unknown runtime files or stale external Docker projects not owned by the recognized installation.
