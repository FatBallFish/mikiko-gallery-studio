# API Contract Smoke Test

`scripts/test/api_contract_smoke.sh` starts a temporary local API server with a SQLite database and verifies the P0 API contract paths without external provider credentials.

It covers:

- `/readyz`
- user email-code login with dev fixed code
- user profile and generation estimate
- API key creation
- native Open API HMAC signing for estimate and async task creation
- OpenAI-compatible `/v1/models`
- JSON 405 error contract
- ops admin login and config tab listing

Run it through the workflow entrypoint:

```bash
./scripts/workflow/api-smoke.sh
```

Optional base URL override:

```bash
BASE_URL=http://127.0.0.1:18081 ./scripts/workflow/api-smoke.sh
```

The script uses a temporary SQLite database under `$TMPDIR`, seeds only test data, and removes the database and API process on exit.
