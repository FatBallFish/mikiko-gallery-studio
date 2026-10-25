# MCP and browser OAuth

Connect a standard MCP client to `${MIKIKO_BASE_URL}/mcp` using Streamable HTTP. Set `MIKIKO_BASE_URL` to the canonical `Runtime.PublicAPIURL`: for this local environment, `http://localhost:5273`, so the MCP connection URL is **`http://localhost:5273/mcp`**. Do not enter the internal API listener `http://localhost:8091/mcp` (or substitute `127.0.0.1` for the configured `localhost` issuer). The server uses the official Go SDK pinned at `v1.7.0`, targeting protocol `2026-07-28`; tested prior negotiated versions are `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05`. Use the negotiated `initialize.protocolVersion`, not an assumed version string. Transport is stateless: POST carries MCP messages; standalone GET and DELETE return 405. Do not depend on an MCP session ID or resumable server event history.

## Discovery and authorization

The canonical issuer is configured by `Runtime.PublicAPIURL`: HTTPS except loopback development. It is not inferred from an untrusted Host header. Protected resource metadata is served at `/.well-known/oauth-protected-resource` and `/.well-known/oauth-protected-resource/mcp`. Authorization server metadata is at `/.well-known/oauth-authorization-server`. Use discovery to obtain endpoints rather than constructing alternate issuer URLs.

In local Studio development, Vite forwards exact `/mcp`, discovery paths (including the `/mcp` metadata suffix), and `/oauth/authorize`, `/oauth/token`, `/oauth/register`, `/oauth/revoke` to its configured API target while preserving the public Host. Discovery must return JSON, and MCP POST must preserve the streaming response; SPA HTML at these URLs indicates a missing proxy. The browser page `/?oauth_request=...#/oauth-consent` remains a Studio route. Login preserves `oauth_request` through local and OIDC authentication; consent submits that request ID with its server CSRF token and follows the server-returned registered client callback. Proxying does not relax canonical issuer or resource validation.

The authorization code flow uses `/oauth/authorize`, PKCE S256, exact registered redirect URI, resource `${issuer}/mcp`, and requested scopes. The client opens the Studio browser login/consent flow. Existing Studio login, including OIDC, establishes the browser user; there is no MCP password-login tool and no password requirement for OIDC-only accounts. Consent requests are stored by the server and protected by CSRF tokens. Registered clients, bounded dynamic registration at `/oauth/register`, and validated Client ID Metadata Documents are supported as configured.

Exchange codes or refresh tokens using form-encoded POST `/oauth/token`, with `client_id` and `resource`. Authorization-code exchange also supplies `code`, `redirect_uri`, and `code_verifier`. Public clients use `token_endpoint_auth_method=none`; do not send a client secret or browser Authorization header. Tokens are opaque and bound to client, user, scopes, resource and grant. Never put access or refresh tokens in URLs.

A 401 challenge points to resource metadata. Insufficient tool scope is returned as a tool error with `_meta["mcp/www_authenticate"]` so clients can initiate additional consent. A live token does not bypass current account or grant state.

## Tools and durable work

Discover schemas and annotations through `tools/list`. The shared operation catalogue includes:

- `get_current_user`, `get_balance`, `list_image_models`, `list_video_models`.
- `preflight_image_task`, `create_image_task`, `get_image_task`, `list_image_tasks` and video equivalents, plus `cancel_video_task`.
- `get_image_task_layers` (`images:read`, input `{id}`) returns the native decomposition manifest: `{version:1,task_id,operation,layers:[{image_id,width,height,format,layer,access_operation,access_input}]}`. Use each entry's `get_image_result_access` with `{task_id,image_id,purpose:"download"}` for OAuth-authorized downloads; browser session ZIP URLs are not OAuth capabilities.
- `get_image_result_access`, `get_video_result_access` renew owned task output URLs using image/video read scope without broad asset-library access.
- `list_assets`, `get_asset`, `update_asset`, `delete_asset`, `get_asset_access`.
- `create_upload`, `get_upload`, `get_upload_part_url`, `complete_upload`, `abort_upload`.
- List/create/get/update/delete operations for projects and groups.

Tool results use `structuredContent: {"data": <operation result>}` with a JSON text representation of that same envelope. The tool output schema wraps `data`. Generated tasks persist independently of the client connection; query by task ID and retrieve new asset access links. Send files through returned upload targets; do not pass large base64 files or assume arbitrary remote URL import.

## Scopes and revocation

Scopes are `account:read`, `models:read`, `images:read`, `images:write`, `videos:read`, `videos:write`, `videos:delete`, plus `read/write/delete` for `assets`, `uploads`, `projects`, `groups`. Request only what the operation needs. Deleting a group removes grouping, not assets.

Use form-encoded POST `/oauth/revoke` with `token` and `client_id` to revoke the associated grant. The browser connected-app screen uses GET `/api/v1/oauth/connected-apps` and DELETE with `grant_id`. Browser consent GET/POST lives at `/api/v1/oauth/consent`; it is not an API-key or MCP tool endpoint. Revocation affects the current grant; it does not log out all Studio browser sessions. Refresh tokens rotate, and reuse detection revokes the grant family. Account disablement and revocation are checked on subsequent operations.

## Limits

Platform lifecycle SSE events are not partially generated images. Partial-image support requires real provider support and an adapter that exposes it. Do not assume event replay, large inline file support, remote URL import, or protocol versions beyond those negotiated. The canonical OpenAPI and actual tool catalogue take precedence over generic SDK examples.
