#!/usr/bin/env bash

deployment_e2e_fail() {
  echo "deployment E2E: $*" >&2
  return 1
}

deployment_e2e_port() {
  python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
}

deployment_e2e_wait_status() {
  local url=$1 expected=$2 timeout=${3:-180}
  local deadline=$((SECONDS + timeout)) status
  while (( SECONDS < deadline )); do
    status="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 3 "$url" 2>/dev/null || true)"
    [[ "$status" == "$expected" ]] && return 0
    sleep 1
  done
  echo "timed out waiting for HTTP $expected from $url" >&2
  return 1
}

deployment_e2e_assert_frontend() {
  local page_url=$1
  python3 - "$page_url" <<'PY'
import re
import sys
from urllib.error import HTTPError
from urllib.parse import urljoin
from urllib.request import Request, urlopen

page_url = sys.argv[1]

def get(url):
    request = Request(url, headers={"Accept": "text/html,*/*"})
    with urlopen(request, timeout=15) as response:
        return response.status, response.headers.get_content_type(), response.read()

status, content_type, body = get(page_url)
assert status == 200 and content_type == "text/html", (page_url, status, content_type)
html = body.decode("utf-8")
match = re.search(r'(?:src|href)="([^"?]+assets/[^"?]+\.(?:js|css))', html)
assert match, (page_url, "no built asset reference")
asset_url = urljoin(page_url, match.group(1))
status, asset_type, _ = get(asset_url)
assert status == 200, (asset_url, status)
if asset_url.endswith(".js"):
    assert asset_type in {"application/javascript", "text/javascript"}, (asset_url, asset_type)
else:
    assert asset_type == "text/css", (asset_url, asset_type)

missing_url = urljoin(page_url, "./assets/missing-e2e.js")
try:
    get(missing_url)
    raise AssertionError((missing_url, "missing static asset returned success"))
except HTTPError as error:
    assert error.code == 404, (missing_url, error.code)
PY
}

deployment_e2e_env_value() {
  local path=$1 key=$2
  awk -F= -v key="$key" '$1 == key { value=substr($0, index($0, "=")+1); gsub(/^"|"$/, "", value); print value; exit }' "$path"
}

deployment_e2e_project_name() {
  local env_file=$1 installation_id node_id
  installation_id="$(deployment_e2e_env_value "$env_file" INSTALLATION_ID)"
  node_id="$(deployment_e2e_env_value "$env_file" CLUSTER_NODE_ID)"
  python3 - "$installation_id" "$node_id" <<'PY'
import hashlib
import sys

installation_id = sys.argv[1].replace("-", "")
node_id = sys.argv[2].strip()
name = f"app-{installation_id}"
if node_id:
    name += "-" + hashlib.sha256(node_id.encode("ascii")).hexdigest()[:12]
print(name)
PY
}

deployment_e2e_set_env_value() {
  local path=$1 key=$2 value=$3 temporary
  temporary="${path}.e2e.$$"
  awk -F= -v key="$key" -v value="$value" '
    BEGIN { replaced=0 }
    $1 == key { print key "=" value; replaced=1; next }
    { print }
    END { if (!replaced) print key "=" value }
  ' "$path" >"$temporary"
  chmod 600 "$temporary"
  mv "$temporary" "$path"
}

deployment_e2e_json_data_field() {
  local field=$1
  python3 -c 'import json,sys; payload=json.load(sys.stdin); value=payload.get("data", payload); print(value[sys.argv[1]])' "$field"
}

deployment_e2e_build_images() {
  local root=$1 registry=$2 tag=$3
  if [[ -n "${E2E_SOURCE_IMAGE_REGISTRY:-}" && -n "${E2E_SOURCE_IMAGE_TAG:-}" ]]; then
    for component in api worker user-web admin-web docs-web; do
      docker image tag \
        "${E2E_SOURCE_IMAGE_REGISTRY}/pic-gallery-${component}:${E2E_SOURCE_IMAGE_TAG}" \
        "${registry}/pic-gallery-${component}:${tag}"
    done
  elif [[ "${E2E_SKIP_IMAGE_BUILD:-false}" != "true" ]]; then
    PIC_GALLERY_IMAGE_REGISTRY="$registry" "$root/scripts/docker/images.sh" build --registry "$registry" --tag "$tag"
  fi
  PIC_GALLERY_IMAGE_REGISTRY="$registry" "$root/scripts/docker/images.sh" push --registry "$registry" --tag "$tag"
}

deployment_e2e_compose() {
  local runtime=$1 project=$2
  shift 2
  docker compose \
    --project-directory "$runtime" \
    --env-file "$runtime/config/runtime.env" \
    --file "$runtime/compose.yml" \
    --project-name "$project" "$@"
}

deployment_e2e_redacted_logs() {
  local runtime=$1 project=$2 output=$3
  mkdir -p "$(dirname "$output")"
  deployment_e2e_compose "$runtime" "$project" logs --no-color >"$output.raw" 2>&1 || true
  sed -E \
    -e 's#(postgres(ql)?|redis)://[^[:space:]]+#<redacted-dsn>#g' \
    -e 's/(token|password|secret|credential)([=: ]+)[^[:space:]]+/\1\2<redacted>/Ig' \
    "$output.raw" >"$output"
  rm -f "$output.raw"
}

deployment_e2e_remove_project() {
  local project=$1 resource
  while IFS= read -r resource; do
    [[ -n "$resource" ]] && docker rm -fv "$resource" >/dev/null 2>&1 || true
  done < <(docker ps --all --quiet --filter "label=com.docker.compose.project=${project}")
  while IFS= read -r resource; do
    [[ -n "$resource" ]] && docker network rm "$resource" >/dev/null 2>&1 || true
  done < <(docker network ls --quiet --filter "label=com.docker.compose.project=${project}")
  while IFS= read -r resource; do
    [[ -n "$resource" ]] && docker volume rm "$resource" >/dev/null 2>&1 || true
  done < <(docker volume ls --quiet --filter "label=com.docker.compose.project=${project}")
}
