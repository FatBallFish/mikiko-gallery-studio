# Backend Completion Release Notes

## Added deployment assets

- API image definition in `Dockerfile.api` for `./cmd/api`.
- Worker image definition in `Dockerfile.worker` for `./cmd/worker`.
- Production Compose stack in `deployments/docker-compose/docker-compose.prod.yml`.
- nginx reverse proxy config in `deployments/nginx/default.conf`.
- `/readyz` smoke test script in `scripts/test/smoke_readyz.sh`.

## Release checklist

- [ ] Fill `deployments/docker-compose/.env.prod` from `deployments/docker-compose/.env.prod.example`.
- [ ] Set a strong `AUTH_ACCESS_TOKEN_SECRET`.
- [ ] Set provider keys when generation providers should be enabled.
- [ ] Build and start with `docker compose --env-file deployments/docker-compose/.env.prod -f deployments/docker-compose/docker-compose.prod.yml up -d --build`.
- [ ] Verify `api`, `worker`, `postgres`, `redis`, and `nginx` are healthy.
- [ ] Run `./scripts/test/smoke_readyz.sh http://localhost:8080` and `./scripts/test/smoke_readyz.sh http://localhost:80`.

## Rollback

1. Identify the previous image tag.
2. Set `IMAGE_TAG=<previous-tag>` in the production env file or shell.
3. Recreate API and worker:

   ```bash
   docker compose --env-file deployments/docker-compose/.env.prod \
     -f deployments/docker-compose/docker-compose.prod.yml up -d --no-deps api worker
   ```

4. Re-run the `/readyz` smoke checks.
