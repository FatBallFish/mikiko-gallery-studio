# Workflow Tracks

Use this file to classify work before coding.

## Heavyweight

Any one of these makes the task heavyweight:

| Dimension | Trigger |
|---|---|
| Contract | public API, request/response shape, error semantics, auth behavior |
| Data | schema, migration, storage layout, backfill |
| Architecture | layer boundaries, shared abstractions, cross-module ownership |
| Security | authz/authn, tenant scope, secrets, permissions |
| Operability | goroutines, retries, queues, cache, performance-sensitive paths |
| Rollout | config, deployment, Docker, environment requirements |
| Open decisions | unclear acceptance criteria, conflicting requirements, missing owners |

Heavyweight work requires:

- requirement source
- technical design source
- implementation plan
- explicit approval before push/PR

## Lightweight

Use lightweight only when all inputs are clear and none of the heavyweight triggers apply.

Lightweight still requires requirement and technical-design sources. If either is missing, run discovery first and ask the user for the missing source before coding.

