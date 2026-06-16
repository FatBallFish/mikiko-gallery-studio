# Pic Gallery Backend Deployment Runbook

## Topology

- API is stateless and can scale horizontally.
- Workers compete for image task leases through PostgreSQL.
- PostgreSQL is the source of truth for users, billing, tasks, images, config, and audit data.
- Redis is required for production cooldowns, rate limits, and runtime cache invalidation.
- Local storage is single-node only unless mounted as a shared volume; use S3-compatible storage for clusters.
- Production must set stable env secrets: `AUTH_ACCESS_TOKEN_SECRET`, `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`, `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`, and `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`.

## Smoke Test

1. `docker compose --env-file deployments/docker-compose/.env.prod -f deployments/docker-compose/docker-compose.prod.yml pull`
2. `docker compose --env-file deployments/docker-compose/.env.prod -f deployments/docker-compose/docker-compose.prod.yml up -d`
3. `curl http://localhost:${NGINX_PORT:-80}/healthz`
4. Send an email code, log in, redeem points, create an image task with a mock provider, query task detail, and download the generated image.

## Rollback

Roll back API, worker, user web, and admin web images together by setting `PIC_GALLERY_IMAGE_TAG` back to the previous version and running:

```bash
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml pull
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml up -d
```

Existing queued/running tasks are safe to reclaim through task leases after the previous worker version exits.

## Production Configuration

Production bootstrap config lives in `deployments/docker-compose/.env.prod` or the server-local env file used by your service manager. `config.yaml` is no longer part of the production deployment path.

Keep env focused on startup requirements. Configure SMTP, provider accounts, model routing, billing/pricing, and payment channels in the admin console after first startup.
