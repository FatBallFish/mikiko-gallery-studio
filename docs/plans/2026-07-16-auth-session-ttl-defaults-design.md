# Authentication Session TTL Defaults Design

## Context

The development Docker deployment omitted `AUTH_ACCESS_TOKEN_TTL` and
`AUTH_REFRESH_TOKEN_TTL`. Environment loading interpreted both missing values as
zero, so login returned an immediately expired access token and refresh cookie.
The password endpoint still returned HTTP 200, but the frontend's immediate
profile request failed with `AUTH_ACCESS_EXPIRED`.

The product requirement fixes the intended values at 10 minutes for access
tokens and 2 hours for refresh tokens. Production compose already declares
these values explicitly.

## Design

Use two independent safeguards:

1. `applyDefaults` assigns 10 minutes and 2 hours whenever the loaded TTL is
   zero. This protects environment, YAML, tests, and future deployment methods.
2. Development compose explicitly passes `AUTH_ACCESS_TOKEN_TTL=10m` and
   `AUTH_REFRESH_TOKEN_TTL=2h` to API and worker containers. This keeps the
   deployment contract visible and aligned with production.

Explicit non-zero values remain unchanged. No database data, password hashes,
or frontend API routing changes are required.

## Verification

- Add a config loader regression test proving missing TTL variables receive the
  required defaults.
- Keep the existing environment override behavior covered by config tests.
- Run repository verification and the local review gate.
- Rebuild the API and worker containers, then verify password login reports a
  positive access lifetime and the returned token can fetch the user profile.
