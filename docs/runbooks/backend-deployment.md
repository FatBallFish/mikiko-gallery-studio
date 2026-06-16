# Full-Stack Deployment Runbook

## Scope

This runbook covers the production Docker Compose deployment for the Pic Gallery API, worker, user web, admin web, Nginx gateway, PostgreSQL, and Redis.

Runtime bootstrap configuration is injected from `.env.prod`. API and worker no longer mount or read a production `config.yaml`. Operational settings such as SMTP, payment channels, provider accounts, model routing, and pricing are configured in the admin console after the stack is reachable.

## First Deploy

1. Prepare the production env file:

   ```bash
   cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
   $EDITOR deployments/docker-compose/.env.prod
   ```

   At minimum, set:

   - `PIC_GALLERY_IMAGE_REGISTRY`
   - `PIC_GALLERY_IMAGE_TAG`
   - `POSTGRES_PASSWORD`
   - `AUTH_ACCESS_TOKEN_SECRET`
   - `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`
   - `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`
   - `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`
   - `PIC_GALLERY_ADMIN_EMAIL`
   - `PIC_GALLERY_ADMIN_PASSWORD`
   - `CORS_ALLOWED_ORIGINS`

   `PIC_GALLERY_ADMIN_EMAIL` and `PIC_GALLERY_ADMIN_PASSWORD` seed the independent Ops admin account only when no admin exists.

2. Pull and start the stack:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml pull
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml up -d
   ```

3. Confirm service health:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml ps
   ./scripts/test/smoke_readyz.sh http://localhost:${NGINX_PORT:-80}
   ```

4. Complete admin-managed runtime configuration:

   - Configure SMTP at `/admin/#/security-config`.
   - Configure provider accounts and model routing in the admin console.
   - Configure billing/pricing, payment site base URL, and payment channels before exposing recharge.
   - Keep the admin console behind HTTPS/TLS before entering merchant or SMTP credentials.

## Standalone Deployment Directory

For a server-local deployment directory:

```bash
mkdir -p pic-gallery-deploy
cd pic-gallery-deploy
/path/to/pic-gallery/deployments/docker-compose/prepare.sh
```

The prepare script copies `docker-compose.yml`, creates `.env.prod`, generates bootstrap secrets, and creates local data directories. It does not overwrite an existing `.env.prod` unless `--force` is passed.

## Runtime Notes

- API image: `${PIC_GALLERY_IMAGE_REGISTRY}/pic-gallery-api:${PIC_GALLERY_IMAGE_TAG}`.
- Worker image: `${PIC_GALLERY_IMAGE_REGISTRY}/pic-gallery-worker:${PIC_GALLERY_IMAGE_TAG}`.
- User web image: `${PIC_GALLERY_IMAGE_REGISTRY}/pic-gallery-user-web:${PIC_GALLERY_IMAGE_TAG}`.
- Admin web image: `${PIC_GALLERY_IMAGE_REGISTRY}/pic-gallery-admin-web:${PIC_GALLERY_IMAGE_TAG}`.
- Only the gateway publishes a host port by default. PostgreSQL, Redis, API, worker, and frontend containers stay on the Compose network.
- Database migrations currently run on API and worker startup through the application bootstrap path.
- `STORAGE_SHARED_VOLUME=true` is required for local storage because API and worker share `/var/lib/pic-gallery/storage`.

## Operations

- Tail API logs:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml logs -f api
  ```

- Tail worker logs:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml logs -f worker
  ```

- Tail frontend logs:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml logs -f user-web admin-web nginx
  ```

- Stop the stack without deleting volumes:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml down
  ```

- Upgrade to a new image tag:

  ```bash
  $EDITOR deployments/docker-compose/.env.prod  # update PIC_GALLERY_IMAGE_TAG
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml pull
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml up -d
  ```

- Start Prometheus when needed:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml --profile monitoring up -d prometheus
  ```
