# Pic Gallery Backend Deployment Runbook

## Topology

- API is stateless and can scale horizontally.
- Workers compete for image task leases through PostgreSQL.
- PostgreSQL is the source of truth for users, billing, tasks, images, config, and audit data.
- Redis is required for production cooldowns, rate limits, and runtime cache invalidation.
- Local storage is single-node only unless mounted as a shared volume; use S3-compatible storage for clusters.
- Production must set `api_key.signing_secret_encryption_key` in `config.yaml`; it protects stored API key HMAC signing secrets and should be stable across API/worker restarts.
- Production should set `cashier.provider_config_encryption_key` in `config.yaml`; it protects stored payment provider merchant credentials and should be stable across API/worker restarts.

## Smoke Test

1. `docker compose -f deployments/docker-compose/docker-compose.prod.yml up --build`
2. `curl http://localhost:8080/healthz`
3. Send an email code, log in, redeem points, create an image task with a mock provider, query task detail, and download the generated image.

## Rollback

Roll back API and worker images together. Existing queued/running tasks are safe to reclaim through task leases after the previous worker version exits.


## Production configuration

Copy `configs/config.pro.yaml` or `configs/config.compose.prod.yaml` to the server as `config.yaml`, then fill all required secret values before starting API and worker. The server-local `config.yaml` must not be committed.

At minimum set database credentials, Redis URL, SMTP settings, `auth.access_token_secret`, `api_key.signing_secret_encryption_key`, `cashier.provider_config_encryption_key`, `security.secure_config_encryption_key`, and `admin.seed_password`.
