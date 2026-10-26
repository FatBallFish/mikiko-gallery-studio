# API Key and MCP acceptance remediation

## Requirement

2026-10-25 user requested fixes for the concrete failures recorded in `docs/reports/2026-10-25-api-mcp-acceptance.md`, after real API Key and Mikiko MCP testing. Preserve existing work; no commit/push or production deployment. Validate with isolated providers before any live calls.

## Design

- Preserve canonical OAuth issuer/resource validation. Expose discovery, OAuth and Streamable HTTP MCP through the same Vite public origin as Studio; point client configuration at that canonical origin. Preserve existing production gateway/Nginx routes.
- For local multipart storage without direct signing, return a short-lived purpose-separated signed proxy target. Bind principal and current key/grant generation, upload session, part, method, checksum, size, audience and expiry. Validate current owner/scope/key/grant state before writing bytes with the existing multipart service. Do not expose original API Key or OAuth tokens in target URLs/headers. S3 direct targets remain supported.
- Cancel never-submitted queued/cancel-requested video items synchronously only when no attempt and no active worker lease exist. Serialize claim/cancel and reuse transactional billing plus API Key settlement exactly once. Submitted/leased items retain asynchronous cancellation. Repeated cancellation may repair an earlier safe pending cancellation through normal API.
- Build video discovery combinations using the same public-to-provider parameter mapping as preflight resolution; do not advertise an unroutable public combination or hardcode model-specific resolutions.
- Worker absence is a runtime condition, not permission to process historical queued work. Do not start an unrestricted shared Worker. Verify generation with isolated fake providers; verify the existing cancelled test task and release its reservation through the fixed normal cancellation API.

## Acceptance

Regression tests cover proxy capability tampering/expiry/revocation/ownership/checksums and pure OAuth MCP multipart completion; canonical Vite OAuth routes and browser return; video claim/cancel concurrency, no duplicate refunds and quota release; discovery/preflight parity. Run full verification, independent review and isolated API smoke, then reconfigure the installed Mikiko test MCP to canonical URL and perform browser OAuth and real tool calls. No additional paid generation is needed.
