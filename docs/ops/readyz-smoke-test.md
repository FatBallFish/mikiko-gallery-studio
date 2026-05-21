# Readiness Smoke Test

`/readyz` is the deployment readiness endpoint for the API container and the nginx reverse proxy. The expected response body contains `"status":"ready"`.

Run against the API port:

```bash
./scripts/test/smoke_readyz.sh http://localhost:8080
```

Run through nginx:

```bash
./scripts/test/smoke_readyz.sh http://localhost:80
```

The script accepts either a positional base URL or `BASE_URL`:

```bash
BASE_URL=http://localhost:8080 ./scripts/test/smoke_readyz.sh
```

It fails fast when the endpoint is unreachable, returns a non-2xx status, or returns JSON without `status=ready`.
