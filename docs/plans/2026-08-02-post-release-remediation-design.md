# Post-release Remediation Design

## Status and Sources

- Status: approved
- Approved approach: contract-driven vertical remediation
- Requirement source: `docs/prd/2026-08-02-post-release-remediation-requirements.md`
- Existing deployment sources:
  - `docs/prd/2026-07-30-mgsctl-install-upgrade-release-requirements.md`
  - `docs/tech/2026-07-30-mgsctl-install-upgrade-release-tech-design.md`
- Stripe reference: `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api`
- This document authorizes planning, not implementation.

## Design Summary

The remediation is split into four vertical contracts: pending-install recovery, cashier/provider integration, admin configuration, and user workflows. Each contract is fixed at the source of its invalid state and covered at service, API, and UI boundaries. Existing runtime identities, encrypted values, API paths, and persistent data remain stable.

This design deliberately avoids a broad deployment-state-machine or payment-plugin rewrite. New abstractions are limited to reusable provider field metadata, pending-install snapshot reconstruction, Stripe adapters, and small shared UI controls.

## Confirmed Root Causes

| Area | Root cause |
| --- | --- |
| Failed install retry | Existing-install recognition requires a complete, hash-matching pending artifact set. Some partial or externally interrupted generated states become `unrecognized` and cannot be regenerated even when their installation identity is valid. |
| TUI overwrite ports | Overwrite creates a fresh installation identity. Compose project names derive from that identity, so the failed project is not reconciled and old-port containers can remain active. |
| Linux PATH | `scripts/install.sh` installs to `$HOME/.local/bin` but only prints an instruction; it does not update shell startup files. |
| Gallery lightbox | The detail modal uses `z-[110]`; the lightbox backdrop uses `z-[100]`. Both stay mounted, so the lightbox is below the detail modal. |
| Package editor | `PackagesPage` create/edit handlers only emit feedback Toasts, although the API client and backend CRUD routes already exist. |
| JeePay fields | The frontend marks `mch_no` and `app_id` as secrets. Backend secret classification does not consider them secrets, so `MergeProviderConfigForWrite` ignores them when they arrive in `secrets`. |
| Raw payment JSON | `CashierPage` renders structured inputs and generic config/secret JSON editors for the same data, creating duplicate sources of truth and exposing sensitive material. |
| Callback fields | Admin users are asked for internal full routes even though the platform already owns the webhook and checkout paths. |
| Missing required markers | Provider descriptors contain partial `required` metadata, but labels, validation, and server-side provider requirements are not consistently derived from it. |
| Stripe missing | Registry, provider types, webhook verification, Payment Element, query, and refund adapters have no Stripe implementation. |
| CNY-per-point hidden | `billing_pricing` exists in the backend config service, but `generalConfigCategories` filters it out of system settings. |
| Redeem entry missing | The redeem API and UI logic exist in `ProfilePage`, while the primary navigation labels `CheckoutPage` as the points page. |
| New users named `user-1` | Database-backed `createUserLocked` returns before incrementing `nextUserID`, while nickname generation still uses that in-memory counter. Database IDs increment normally; nicknames do not. |
| Text-model actions illegible | `TextModelsPage` defines a local 36-40px button with 15px icons and native `title`, bypassing the shared admin icon-button/tooltip behavior. |
| Prompt optimization 404 | Saving and connection testing do not set `is_default`. `ResolveDefaultModel` requires an enabled default model under an enabled account and maps absence to the reported generic NOT_FOUND error. |

## 1. Pending-install Recovery

### 1.1 Recognized pending snapshot

Replace the current tuple-returning install loader with a typed pending snapshot containing:

- parsed manifest and install plan;
- parsed install state;
- parsed runtime environment;
- generated-file ownership and hash status;
- runtime and manifest paths.

Recognition is identity-first. A state/manifest/runtime combination is owned when schema versions are supported, installation IDs agree, setup has never completed, and the deployment plan validates. Generated Docker files may be missing or stale in a pending installation; they are rebuildable and do not by themselves make the directory unknown.

Malformed identities, completed setup, mismatched installation IDs, path traversal, symlinks, or user-owned files remain non-overwritable.

### 1.2 Resume and overwrite semantics

The decision table is:

| Existing state | Requested plan | Explicit overwrite | Result |
| --- | --- | --- | --- |
| none | any valid plan | no | create installation |
| pending | semantically equal | no/yes | reuse snapshot and resume deployment |
| pending | different | no | typed confirmation-required error |
| pending | different | yes | rebuild owned configuration in place |
| completed | any | any | reject and direct to upgrade |
| unknown | any | any | reject without deletion |

Plan equality includes normalized ports, components, image release, mode, topology, and storage configuration. It excludes transient CLI/TUI presentation values.

### 1.3 In-place artifact reconstruction

Overwrite keeps stable runtime values from the pending snapshot:

- `INSTALLATION_ID`, setup token, and setup token version;
- authentication, encryption, quote-signing, and cluster-seal keys;
- PostgreSQL, Redis, and managed object-storage credentials;
- cluster node identity when present.

Plan-owned values such as ports, components, storage mode, image registry/tag/digests, application version, public URL, and deployment assets are rendered from the new plan. If a newly selected managed component needs credentials that did not previously exist, only those missing credentials are generated.

All replacement artifacts are rendered and validated before publication. Publication uses staged files and an explicit replace path restricted to manifest-owned files. A filesystem failure restores the prior generated set. A deployment failure keeps the newly selected configuration so the next run resumes the intended plan.

### 1.4 Docker reconciliation

Preserving the installation ID preserves the Compose project name and volume namespace. Install/update reconciliation adds forced recreation for affected services and retains `--remove-orphans`. Managed services retain their named volumes and credentials. This ensures a port change updates API and gateway containers rather than creating a second project beside the failed one.

### 1.5 Wrapper PATH setup

`scripts/install.sh` gains an idempotent `ensure_install_dir_on_path` helper. It writes a marked block to `~/.profile` and, based on `$SHELL`, also to `~/.bashrc` or `~/.zshrc`. Existing PATH entries and existing marked blocks are left unchanged. The current invocation continues to execute the installed binary by absolute path because a child script cannot modify the parent shell environment.

## 2. Existing UI Workflow Repairs

### 2.1 Overlay layers

Define centralized overlay layers in the user redesign classes. Standard modal backdrops remain at the existing layer; image lightboxes use a higher dedicated layer, with controls above the lightbox image. `GalleryPage` keeps its current state model, so closing the lightbox returns to the still-open detail dialog.

### 2.2 Package editor

Move cashier plan draft conversion, validation, and editor UI into reusable package-plan modules. `PackagesPage` owns create/edit dialog state and calls existing `createCashierPlan` and `updateCashierPlan` methods. Successful saves close the dialog and reload the list; failures stay in the dialog. Delete behavior and permission handling remain unchanged.

### 2.3 Redeem code on the points page

Extract redeem input/submission into a reusable component used by `CheckoutPage` and, if retained, `ProfilePage`. The production input starts empty rather than with a demo code. Successful redemption refreshes the account balance and checkout data and emits one success notification. The component is placed after recent orders at the bottom of the points page.

## 3. Cashier Provider Field Contract

### 3.1 Field metadata

Provider field definitions become the single frontend source of truth and include:

- key and localized label;
- storage destination: `config` or `secret`;
- required/optional state;
- sensitive input presentation;
- input kind and options;
- provider-specific hint;
- virtual transform for callback-base fields.

Sensitivity controls input masking. Storage controls request placement. They are intentionally separate concepts: merchant IDs can be visually treated as account identifiers but must still be stored in normal config.

### 3.2 JeePay and other provider identifiers

JeePay `mch_no` and `app_id`, Alipay `app_id`, WeChat `app_id`/`mch_id`, and EasyPay `pid` are ordinary config. Signing keys, private keys, API keys, certificate secrets, Stripe secret key, and Stripe webhook secret are secrets.

The backend continues to merge saved secrets and redact them on reads. Provider-specific validation runs after merge so an unchanged stored secret satisfies an edit, while a new provider instance must submit every required credential.

### 3.3 Remove raw editors

Remove generic secret JSON, clear-secret JSON, and provider-config JSON controls from the normal provider dialog. Structured request payloads remain compatible with the existing backend API. Advanced provider-specific data that is genuinely required is represented as an explicit optional field; no generic plaintext secret surface remains.

### 3.4 Callback bases

The UI exposes `notify_base_url` and `return_base_url` as virtual fields. New dialogs default both to `window.location.origin`. Saving normalizes the origin and writes existing backend keys:

- `notify_url = {notify_base}/api/open/image/v1/payments/webhooks/{provider_type}`
- `return_url = {return_base}/#/checkout`

Editing an existing full URL strips the known suffix back to its base. Unknown legacy paths are preserved until the user intentionally changes the base, preventing silent migration.

### 3.5 Required and optional presentation

Labels show a consistent required marker or an explicit optional suffix. Inputs carry `required`/`aria-required` where applicable. Client validation names the first missing field. Server validation uses the same provider requirement table conceptually and returns a stable bad-request code rather than allowing order creation to fail later.

## 4. Stripe Full-mode Integration

### 4.1 Provider model

Add provider type `stripe` with a user-visible Stripe payment method. The first release is CNY-only because cashier orders and settlement are CNY-denominated. Configuration consists of:

- publishable key in normal config;
- secret key in encrypted secrets;
- webhook signing secret in encrypted secrets.

Use `github.com/stripe/stripe-go/v85` for PaymentIntent, event verification, query, and refund behavior. Use `@stripe/stripe-js` and `@stripe/react-stripe-js` in the user web application.

### 4.2 Order creation and display

The Stripe payment builder converts the decimal CNY amount to fen without floating-point arithmetic and creates an idempotent PaymentIntent keyed by order number. Metadata includes the local order number. The result uses the existing `ClientToken`/payment-display transport and adds:

```text
payment_display.type = stripe_payment_element
payment_display.client_secret = <Stripe client secret>
payment_display.publishable_key = <Stripe publishable key>
```

No secret key or webhook secret is included in the response.

### 4.3 User confirmation

`CheckoutPage` renders a Stripe Payment Element panel for the Stripe display type. `confirmPayment` uses the existing `/#/checkout` return route. For payment methods that do not require redirect, the dialog remains open and existing order polling observes the webhook/query result. A recoverable Stripe error stays inside the payment dialog and does not discard the order.

### 4.4 Webhook, query, and refund

The webhook handler reads the exact raw request bytes and verifies `Stripe-Signature` with the configured instance webhook secret. It accepts `payment_intent.succeeded` and `payment_intent.payment_failed`; unrelated event types return success without mutating an order. Local completion reuses existing idempotent amount/order verification and ledger crediting.

Query retrieves the PaymentIntent and maps succeeded, processing/requires-action, canceled, and failed states into existing cashier status categories. Refund creates a Stripe refund against the PaymentIntent; refund query retrieves the refund status. Provider identifiers are stored in existing trade/refund fields.

### 4.5 Test isolation

Stripe SDK backends are injected or replaced with an HTTP test backend. Tests never call Stripe's public service. Webhook tests use a deterministic signed fixture and verify raw-body sensitivity, amount matching, failure events, and repeated-event idempotency.

## 5. System Settings and Account Defaults

### 5.1 CNY-per-point settings

Add an `积分换算` system-settings tab. Refactor `ConfigPage` to accept an allowlist so this tab exposes `billing_pricing/cny_per_point` without exposing unrelated dangerous payment JSON. The backend config source and default `0.3125` remain unchanged; saving uses the existing versioned admin-config API and runtime-effective config path.

### 5.2 New-user nickname

Introduce a pure default-nickname helper that takes the already normalized email and returns the non-empty local part before `@`, bounded by the user nickname limit. Registration uses this helper in both memory and database modes. A defensive fallback is used only for malformed legacy input. Existing records are not migrated, and profile updates continue to respect explicit nicknames.

## 6. Text-model Readiness and Actions

### 6.1 Default-model invariant

Create/update paths enforce this invariant: when no enabled default exists, the first newly eligible model under an enabled account becomes default. Disabling or deleting the default clears it and promotes a single remaining eligible model when the choice is unambiguous.

For pre-remediation data, default resolution may self-heal only when exactly one eligible model exists. If multiple models are eligible and none is default, return a typed `TEXT_MODEL_DEFAULT_REQUIRED` conflict with an administrative action hint. Do not silently choose among multiple existing models.

The store operation that clears and selects defaults remains transactional. Service tests cover account enable/disable, model enable/disable, delete, first model, single-candidate repair, and ambiguous candidates.

### 6.2 Admin readiness state

The text-model page shows the current default model or a visible warning that optimization is not ready. The explicit set-default action remains available for switching models. Connection success is described as connectivity only, avoiding the implication that it also completes default selection.

### 6.3 Legible actions and tooltips

Remove the page-local icon-button implementation. Reuse or extend the shared admin IconButton with a real portal tooltip triggered by hover and focus. Text-model action targets are 40x40 pixels, icons are 18-20 pixels, and each action has an accessible name. Disabled controls still expose explanatory tooltip text.

## 7. Error Handling and Compatibility

- Pending install errors distinguish confirmation required, completed installation, unknown ownership, filesystem publication failure, and deployment failure.
- Provider save errors identify missing field labels without echoing submitted values.
- Stripe errors expose stable local categories and safe messages; upstream bodies and credentials remain server-side.
- Prompt optimization distinguishes no configuration, ambiguous missing default, disabled configuration, stale quote, and provider failure.
- Existing payment provider instances and callback URLs remain readable without migration.
- Existing users and completed installations are not mutated by startup repair.

## 8. Verification Strategy

Implementation follows TDD. Focused tests run after each vertical slice, followed by:

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Manual browser verification covers desktop and mobile overlay stacking, package editing, provider forms, Stripe Payment Element test mode, text-model tooltips/default status, CNY-per-point editing, and redeem-code submission. Installer smoke uses temporary runtime directories and a fake process runner; no developer runtime directory is reused.

## 9. Delivery Boundaries

The implementation should be committed as reviewable vertical slices rather than one aggregate commit. Stripe backend and frontend may be separate commits but must not be pushed in a state that exposes an unusable method. Release notes must call out provider configuration changes, pending-install recovery semantics, and Stripe setup requirements.
