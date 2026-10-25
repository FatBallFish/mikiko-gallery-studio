# Runnable SDK examples

Set `MIKIKO_BASE_URL` to the canonical `Runtime.PublicAPIURL` deployment origin (`http://localhost:5273` in this local environment) and inject `MIKIKO_API_KEY` through your secret manager. MCP clients connect to `PublicAPIURL + /mcp`, here **`http://localhost:5273/mcp`**, using browser OAuth as described in [MCP and browser OAuth](mcp.md). Do not configure the internal API listener on port `8091` as the MCP issuer/resource, or substitute another hostname: discovery and tokens use the configured canonical origin. These examples discover available models; use their returned capabilities when building generation requests. They do not assume a particular model or paid provider is configured.

## Node.js 18+

Save as `models.mjs` and run `node models.mjs`:

```javascript
const base = process.env.MIKIKO_BASE_URL
const key = process.env.MIKIKO_API_KEY
if (!base || !key) throw new Error('Set MIKIKO_BASE_URL and MIKIKO_API_KEY')
const response = await fetch(base.replace(/\/$/, '') + '/api/open/v1/image-models', {
  headers: { Authorization: `Bearer ${key}` },
})
if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`)
console.log(await response.json())
```

## Python 3 standard library

Save as `models.py` and run `python3 models.py`:

```python
import json
import os
import urllib.request

request = urllib.request.Request(
    os.environ['MIKIKO_BASE_URL'].rstrip('/') + '/api/open/v1/image-models',
    headers={'Authorization': 'Bearer ' + os.environ['MIKIKO_API_KEY']},
)
with urllib.request.urlopen(request, timeout=30) as response:
    print(json.load(response))
```

## OpenAI Python SDK

Install with `python3 -m pip install openai`. Save as `compat_models.py` and run `python3 compat_models.py`:

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ['MIKIKO_API_KEY'],
    base_url=os.environ['MIKIKO_BASE_URL'].rstrip('/') + '/v1',
)
print(client.models.list())
```

For synchronous generation, call `client.images.generate(model=<discovered model>, prompt=<prompt>, ...)` using supported parameters. Synchronous waits are bounded at two minutes; `504 TASK_WAIT_INTERRUPTED` leaves a durable task recoverable through `X-Task-ID` and `Location`. `stream=true` emits `task` and `result` events, with `data: [DONE]` after a successful result; an `error` event ends the stream without that success marker. It is identified by `X-Stream-Protocol: platform-image-lifecycle-v1`. Do not assume the SDK understands those platform events. `partial_images` is explicitly unsupported; it must never be simulated.

## Native task workflow

Build the request from the canonical OpenAPI request schema, select a project you own, and choose a model returned by discovery. Send the full request to `POST /api/open/v1/image-tasks/preflight` or `/video-tasks/preflight`. Include the returned quote token and a stable operation idempotency key when creating the task. The same payload and key must be reused on retries; changed inputs require a new preflight. Preserve points as decimal strings.

The creation and polling response shapes differ between image and video operations; generate types from `api/openapi/openapi.yaml` rather than casting both into a fictional common task schema. Save task IDs and fetch asset access URLs when needed. Never assume a cached URL remains valid indefinitely.
