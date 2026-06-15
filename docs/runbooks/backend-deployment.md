# Full-Stack Deployment Runbook

## Scope

This runbook covers the production Docker Compose deployment for the Pic Gallery API, worker, user web, admin web, Nginx gateway, PostgreSQL, and Redis. Runtime backend config is read from `/app/config.yaml`, which Compose mounts from `PIC_GALLERY_CONFIG_FILE`.

## First deploy

1. Copy the production env template and prepare the backend config file:

   ```bash
   cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
   $EDITOR deployments/docker-compose/.env.prod
   cp configs/config.compose.prod.yaml /opt/pic-gallery/config.yaml
   $EDITOR /opt/pic-gallery/config.yaml
   ```

   Set `PIC_GALLERY_CONFIG_FILE=/opt/pic-gallery/config.yaml` in `.env.prod`.
   Replace all `CHANGE_ME` values in the YAML before first start. The `admin.seed_email` and `admin.seed_password` fields seed the independent Ops admin account during bootstrap, and Ops APIs do not accept normal user JWTs. Also replace `api_key.signing_secret_encryption_key`, `cashier.provider_config_encryption_key`, and `security.secure_config_encryption_key`; the last key protects admin-managed secure settings such as SMTP passwords.

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
- Config path: Compose mounts `PIC_GALLERY_CONFIG_FILE` to `/app/config.yaml`.
- Production mode is controlled by `app.env: prod` in the YAML. Use `storage.shared_volume: true` when API and worker share `/var/lib/pic-gallery/storage`.
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
