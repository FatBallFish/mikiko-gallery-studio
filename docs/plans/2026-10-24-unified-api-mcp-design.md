# Unified developer API and OAuth MCP requirements and design

## Owner decisions and approval

2026-10-24: approved comprehensive implementation. ALL developer APIs use only Bearer API Key; remove HMAC request signature and directly replace old developer endpoints, no compatibility layer. Browser/admin sessions and callbacks/webhooks retain separate trust mechanisms. MCP uses Studio-owned OAuth authorization (browser login/consent, not password tool). Preserve prior uncommitted features. No commit/push/deploy.

## Capability contract

Inventory existing features in a separate document. Unified /api/open/v1: me, me/balance, image-models, video-models; POST image-tasks/preflight and video-tasks/preflight; POST/GET task collections, GET detail/events, video cancel; asset list/get/update/delete + POST access; upload init/status/parts/complete/abort; project CRUD; persistent project-scoped asset group CRUD. /v1/models + images/generations/edits remain OpenAI compatible synchronous or explicitly documented SSE platform lifecycle/results extension. True partial-image events require real upstream support and unsupported params rejected. Native async is canonical persistent task mechanism.

Full preflight validates same complete request as create including project/ref ownership, models/capabilities, prompt vars/bindings, parameters, costs/balance/key quotas. No charging preflight. Expiring quote binds normalized request+identity+capability/price versions; create revalidates and reserves atomically. Five-decimal points strings. Idempotency avoids duplicate charges, client disconnect doesn't lose persistent task.

## Shared application boundary

REST, compatibility and MCP invoke shared operations directly, never replay session HTTP or fake JWT. Principal user ID, key/client/grant, scopes, group and charge attribution. Enforce scope+current account state on every call. Existing Key hashes/expiry/RPM/total/day quota retained; migrate scopes preserving image permissions only, no automatic destructive grant. Video must carry key attribution and quota settlement/refund too.

Shared operations interface selected for adapters: internal/service/openplatform Service.Execute(ctx, Principal, operation string, input json.RawMessage) (any,error), catalogue specs JSON schemas and scopes. Stream/byte upload adapters may use typed service methods. Principal {UserID,APIKeyID,ClientID,GrantID,Scopes,GroupCode}; operation names snake_case aligned MCP tools (get_current_user,get_balance,list_image_models,preflight_image_task,create_image_task,get_image_task,list_image_tasks and video equivalents/cancel; list/get/update/delete_asset,get_asset_access; list/create/get/update/delete_group/project; create/get/get_upload_part_url/complete/abort_upload). This shared contract can be refined in code with adapter coordination.

## Groups

Persistent user/project group id,name,version,timestamps; nullable group_id assets; backfill distinct user/project/group_name. Single assignment. Delete clears grouping only. Existing group_name flows resolve group entity. Project transfer resolves corresponding target name, preserves display; unique active normalized group names per project. Never confuse model/key groups with asset groups.

## OAuth and MCP

Use official github.com/modelcontextprotocol/go-sdk pinned version supporting2026-07-28 plus implemented prior versions. /mcp StreamableHTTP. RFC9728 protected resource metadata + RFC8414 discovery. Studio OAuth auth-code+PKCES256, singleuse code exact client/redirect/resource/scope/user binding; safe Client ID Metadata Documents, bounded DCR+preregistration. Dedicated hashed opaque access+refresh grants; rotation/reuse revokes family, audience/client/scope validated, revoked/disabled user immediate failure. Consent CSRF protection server-stored authorization request; existing Studio login incl OIDC. Connected apps list/revoke. Correct OIDC-only refresh dependency on password. Login via client browser auth; logout current grant revocation, not all sessions. No tokens in URLs. MCP tools schema/annotations, async task IDs and asset access renewal, multipart upload targets not large base64 or arbitrary URL import.

## Deployment/docs

Canonical issuer/resource from validated Runtime.PublicAPIURL HTTPS except loopback dev; don't trust Host. Router/Go gateway/nginx .well-known/oauth/mcp routing, CORS+Origin checks, SSE no buffering. OpenAPI canonical describes all new REST and OAuth; remove obsolete signed developer paths, retain browser/public/callback routes. Docs filter handles new namespace; Key page scopes and Bearer examples. MCP guide/tool list/OAuth revoke, SDK runnable examples and migration map. Verify shared contract documents match actual behavior.

## Validation

Focused cross-user/scopes/key/quota/image+video preflight parity/idempotency, group migration/rename/delete/transfer, upload checksums/resume/link expiry, compatibility sync/SSE/stream disconnect; OAuth PKCE/code replay/redirect/resource/client/refresh concurrency/revoke; official SDK MCP tools and transport tests; gateway metadata auth flow; full verify/review/isolated smoke no paid real provider. One bounded verification process per agent GOMAXPROCS2 GOFLAGS-p2. Backup local DB before additive migrations.
