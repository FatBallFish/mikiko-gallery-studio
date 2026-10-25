# Migrating developer integrations

The signed developer user endpoints under `/api/open/image/v1` are directly replaced. There is no compatibility layer. Public gallery and provider/payment callbacks retain their existing paths and trust mechanisms.

Send `Authorization: Bearer $MIKIKO_API_KEY` on every developer request. Remove access-key, timestamp, body-digest and request-signature headers and signing code. Use a server-side secret manager. Browser and administrator session tokens are separate credentials; MCP obtains its own OAuth grant through browser authorization.

| Previous developer route | Replacement |
| --- | --- |
| `GET /api/open/image/v1/capabilities` | `GET /api/open/v1/image-models` |
| `GET /api/open/image/v1/balance` | `GET /api/open/v1/me/balance` |
| `GET /api/open/image/v1/estimate` | `POST /api/open/v1/image-tasks/preflight` with complete JSON create payload |
| `POST /api/open/image/v1/tasks` | `POST /api/open/v1/image-tasks` |
| `GET /api/open/image/v1/tasks/{task_id}` | `GET /api/open/v1/image-tasks/{task_id}` |
| Reference upload initialization and completion | `/api/open/v1/uploads` session workflow |
| Reference lookup | `/api/open/v1/assets/{asset_id}` |

Regenerate clients from the canonical OpenAPI. Do not reuse old request shapes merely because the concepts are similar: new creation accepts complete requests and quote/idempotency fields, and media uploads use sessions with part status and checksum handling. Discover permitted model parameters first. Re-run preflight whenever payload changes. Preserve point values as five-decimal strings.

The scope catalogue is `account:read`, `models:read`, `images:read`, `images:write`, `videos:read`, `videos:write`, `videos:delete`, and `read`, `write`, `delete` variants for `assets`, `uploads`, `projects`, `groups`. Existing image keys retain image permissions only; explicitly grant video and destructive scopes when needed. Status, expiry, RPM and total/day quota still apply.

Save task IDs and asset IDs. Retry the same business operation using the same idempotency key and payload. A completed request replay may return an existing task; using a key for another payload is a conflict. Fetch new asset access URLs when old links expire. Do not assume event replay or partial-image streaming support.

The docs reference is generated from `api/openapi`; do not edit `web/docs/public/openapi` by hand. Run `node web/docs/scripts/sync-openapi.mjs` after changing the canonical source.
