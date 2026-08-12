#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "multimedia acceptance requires a running Docker daemon" >&2
  exit 1
fi

ACCEPTANCE_ID="$(date +%s)-$$"
MINIO_CONTAINER="mgs-multimedia-acceptance-minio-${ACCEPTANCE_ID}"
MINIO_ROOT_USER="mgsacceptance"
MINIO_ROOT_PASSWORD="mgs-acceptance-secret-123456"
MINIO_BUCKET="multimedia-acceptance"
MINIO_IMAGE="minio/minio:RELEASE.2025-09-07T16-13-09Z"
MINIO_MC_IMAGE="minio/mc:RELEASE.2025-08-13T08-35-41Z"

cleanup() {
  docker rm -f "$MINIO_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

MULTIMEDIA_ACCEPTANCE=1 go test ./internal/acceptance \
  -run '^TestMultimediaLoadAcceptance$' -count=1 -v -timeout=5m
MULTIMEDIA_ACCEPTANCE=1 go test ./internal/acceptance \
  -run '^TestMultimediaLocalOneGiBAcceptance$' -count=1 -v -timeout=20m

docker run -d --name "$MINIO_CONTAINER" \
  -e "MINIO_ROOT_USER=$MINIO_ROOT_USER" \
  -e "MINIO_ROOT_PASSWORD=$MINIO_ROOT_PASSWORD" \
  -p 127.0.0.1::9000 \
  "$MINIO_IMAGE" server /data >/dev/null

MINIO_PORT="$(docker port "$MINIO_CONTAINER" 9000/tcp | awk -F: 'NR == 1 { print $NF }')"
MINIO_ENDPOINT="http://127.0.0.1:${MINIO_PORT}"
for _ in {1..80}; do
  if curl --silent --fail "$MINIO_ENDPOINT/minio/health/live" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
curl --silent --fail "$MINIO_ENDPOINT/minio/health/live" >/dev/null

docker run --rm --network "container:$MINIO_CONTAINER" \
  --entrypoint /bin/sh "$MINIO_MC_IMAGE" -c \
  "mc alias set acceptance http://127.0.0.1:9000 '$MINIO_ROOT_USER' '$MINIO_ROOT_PASSWORD' >/dev/null && mc mb --ignore-existing acceptance/'$MINIO_BUCKET' >/dev/null"

MULTIMEDIA_ACCEPTANCE=1 \
MULTIMEDIA_MINIO_ENDPOINT="$MINIO_ENDPOINT" \
MULTIMEDIA_MINIO_BUCKET="$MINIO_BUCKET" \
MULTIMEDIA_MINIO_ACCESS_KEY="$MINIO_ROOT_USER" \
MULTIMEDIA_MINIO_SECRET_KEY="$MINIO_ROOT_PASSWORD" \
  go test ./internal/acceptance -run '^TestMultimediaS3OneGiBAcceptance$' -count=1 -v -timeout=20m

echo "OK: multimedia heavy acceptance passed"
