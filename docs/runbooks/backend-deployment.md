# Full-Stack Deployment Runbook

## Scope

This runbook covers the production Docker Compose deployment for the Pic Gallery API, worker, user web, admin web, Nginx gateway, PostgreSQL, and Redis. Runtime config uses `configs/config.example.yaml` plus environment overrides supported by `internal/config/load.go`.

## First deploy

1. Copy the production env template and replace all `change-me` values:

   ```bash
   cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
   $EDITOR deployments/docker-compose/.env.prod
   ```

   Set `PIC_GALLERY_ADMIN_EMAIL` and `PIC_GALLERY_ADMIN_PASSWORD` before first start. The API seeds this independent Ops admin account during bootstrap, and Ops APIs do not accept normal user JWTs.
   Also replace `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`, `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`, and `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`; the last key protects admin-managed secure settings such as SMTP passwords.

2. Build and start the stack:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml up -d --build
   ```

3. Confirm service health:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml ps
   ./scripts/test/smoke_readyz.sh http://localhost:${NGINX_PORT:-80}
   ```

## Runtime notes

- API entrypoint: `Dockerfile.api`, binary built from `./cmd/api`.
- Worker entrypoint: `Dockerfile.worker`, binary built from `./cmd/worker`.
- User web entrypoint: `Dockerfile.user-web`, served internally by Nginx and exposed through the gateway at `/`.
- Admin web entrypoint: `Dockerfile.admin-web`, served internally by Nginx and exposed through the gateway at `/admin/`.
- Only the gateway publishes a host port by default. PostgreSQL, Redis, API, worker, and frontend containers stay on the Compose network.
- Config path: `APP_CONFIG_PATH=configs/config.example.yaml`.
- Production mode requires `APP_ENV=prod` and `STORAGE_SHARED_VOLUME=true` so API and worker share `/var/lib/pic-gallery/storage`.
- Database migrations currently run on API and worker startup through the application bootstrap path.
- Configure SMTP at `/admin/#/security-config` after the stack is reachable. SMTP passwords and payment provider merchant secrets are write-only in Admin APIs and should only be entered over HTTPS/TLS.

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

- Start Prometheus when needed:

  ```bash
  docker compose --env-file deployments/docker-compose/.env.prod \
    -f deployments/docker-compose/docker-compose.prod.yml --profile monitoring up -d prometheus
  ```
