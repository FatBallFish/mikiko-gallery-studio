# Pic Gallery Backend Deployment Runbook

## Topology

- API is stateless and can scale horizontally.
- Workers compete for image task leases through PostgreSQL.
- PostgreSQL is the source of truth for users, billing, tasks, images, config, and audit data.
- Redis is required for production cooldowns, rate limits, and runtime cache invalidation.
- Local storage is single-node only unless mounted as a shared volume; use S3-compatible storage for clusters.
- Production must set `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`; it protects stored API key HMAC signing secrets and should be stable across API/worker restarts.
- Production should set `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`; it protects stored payment provider merchant credentials and should be stable across API/worker restarts.

## Smoke Test

1. `docker compose -f deployments/docker-compose/docker-compose.prod.yml up --build`
2. `curl http://localhost:8080/healthz`
3. Send an email code, log in, redeem points, create an image task with a mock provider, query task detail, and download the generated image.

## Rollback

Roll back API and worker images together. Existing queued/running tasks are safe to reclaim through task leases after the previous worker version exits.


## Production environment file

Copy `deployments/docker-compose/.env.example` to `deployments/docker-compose/.env` and fill all required secret values before starting the production compose stack. The real `.env` file is ignored by git and must not be committed.

At minimum set database credentials, `DATABASE_URL`, SMTP settings, `AUTH_ACCESS_TOKEN_SECRET`, `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`, `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`, `PIC_GALLERY_ADMIN_PASSWORD`, and `PIC_GALLERY_ADMIN_TOKEN_SECRET`.
