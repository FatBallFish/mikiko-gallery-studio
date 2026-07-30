# Mikiko Gallery Studio Backend Deployment

`mgsctl` is the only supported production installation and lifecycle entrypoint. The authoritative operations guide is [`docs/runbooks/backend-deployment.md`](../runbooks/backend-deployment.md).

For a new single-host deployment, Docker `full/single` and selector `latest` are the defaults:

```bash
./scripts/install.sh install --runtime-dir ./runtime --yes
```

mgsctl verifies `release-manifest.json`, resolves `latest` to a concrete application version and immutable digests, and uses images including `docker.io/fatballfish/mikiko-gallery-studio-api`. Complete Setup at `/setup`, then verify `/readyz` and `mgsctl doctor`.

Upgrade from any directory after the successful install has saved its runtime path:

```bash
mgsctl upgrade --image-tag latest
```

The upgrade pulls the target API digest, runs `mikiko-gallery-studio-db-migrate` in the installation's Compose network, and rolls services only after migration succeeds. Back up PostgreSQL and object storage before upgrading; use the full runbook for rollback and forward-recovery rules.
