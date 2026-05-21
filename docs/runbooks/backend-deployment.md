# Backend Deployment Runbook

## Scope

This runbook covers the production Docker Compose deployment for the Pic Gallery API and worker. It uses only the existing runtime config path, `configs/config.example.yaml`, plus environment overrides supported by `internal/config/load.go`.

## First deploy

1. Copy the production env template and replace all `change-me` values:

   ```bash
   cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
   $EDITOR deployments/docker-compose/.env.prod
   ```

   Set `PIC_GALLERY_ADMIN_EMAIL` and `PIC_GALLERY_ADMIN_PASSWORD` before first start. The API seeds this independent Ops admin account during bootstrap, and Ops APIs do not accept normal user JWTs.

2. Build and start the stack:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml up -d --build
   ```

3. Confirm service health:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml ps
   ./scripts/test/smoke_readyz.sh http://localhost:${API_PORT:-8080}
   ./scripts/test/smoke_readyz.sh http://localhost:${NGINX_PORT:-80}
   ```

## Runtime notes

- API entrypoint: `Dockerfile.api`, binary built from `./cmd/api`.
- Worker entrypoint: `Dockerfile.worker`, binary built from `./cmd/worker`.
- Config path: `APP_CONFIG_PATH=configs/config.example.yaml`.
- Production mode requires `APP_ENV=prod` and `STORAGE_SHARED_VOLUME=true` so API and worker share `/var/lib/pic-gallery/storage`.
- Database migrations currently run on API and worker startup through the application bootstrap path.

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
