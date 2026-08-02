# Mikiko Gallery Studio Backend Deployment

`mgsctl` is the only supported production installation and lifecycle entrypoint. The authoritative operations guide is [`docs/runbooks/backend-deployment.md`](../runbooks/backend-deployment.md).

For a new single-host deployment, Docker `full/single` and selector `latest` are the defaults:

```bash
./scripts/install.sh install --runtime-dir ./runtime --yes
```

mgsctl verifies `release-manifest.json`, resolves `latest` to a concrete application version and immutable digests, and uses images including `docker.io/fatballfish/mikiko-gallery-studio-api`. Complete Setup at `/setup`, then verify `/readyz` and `mgsctl doctor`.

If Setup-pending installation fails, rerun the same command to resume. Use `--overwrite` only to apply a changed pending plan; mgsctl preserves installation identity, Setup credentials, application secrets, middleware credentials, data, logs, and volumes, while rerendering configuration and force-recreating affected services. On Linux, start a new shell after `install.sh` so the persisted user-local `PATH` entry is loaded.

Upgrade from any directory after the successful install has saved its runtime path:

```bash
mgsctl upgrade --image-tag latest
```

The upgrade pulls the target API digest, runs `mikiko-gallery-studio-db-migrate` in the installation's Compose network, and rolls services only after migration succeeds. Back up PostgreSQL and object storage before upgrading; use the full runbook for rollback and forward-recovery rules.

After upgrade, run `mgsctl doctor`, verify `/readyz`, and confirm the admin text-model page reports exactly one default optimization model. See [Cashier Provider Configuration](../runbooks/cashier-provider-configuration.md) before enabling JeePay or Stripe.
