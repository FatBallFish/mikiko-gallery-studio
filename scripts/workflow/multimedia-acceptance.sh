#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "multimedia acceptance requires a running Docker daemon" >&2
  exit 1
fi

ACCEPTANCE_ID="$(date +%s)-$$"
S3_CONTAINER="mgs-multimedia-acceptance-rustfs-${ACCEPTANCE_ID}"
S3_ACCESS_KEY="mgsacceptance"
S3_SECRET_KEY="mgs-acceptance-secret-123456"
S3_BUCKET="multimedia-acceptance"
RUSTFS_IMAGE="rustfs/rustfs:1.0.1"
AWS_CLI_IMAGE="amazon/aws-cli:2.37.12"

cleanup() {
  docker rm -f "$S3_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

MULTIMEDIA_ACCEPTANCE=1 go test ./internal/acceptance \
  -run '^TestMultimediaLoadAcceptance$' -count=1 -v -timeout=5m
MULTIMEDIA_ACCEPTANCE=1 go test ./internal/acceptance \
  -run '^TestMultimediaLocalOneGiBAcceptance$' -count=1 -v -timeout=20m

docker run -d --name "$S3_CONTAINER" \
  -e "RUSTFS_ACCESS_KEY=$S3_ACCESS_KEY" \
  -e "RUSTFS_SECRET_KEY=$S3_SECRET_KEY" \
  -e "RUSTFS_ADDRESS=0.0.0.0:9000" \
  -p 127.0.0.1::9000 \
  "$RUSTFS_IMAGE" >/dev/null

S3_PORT="$(docker port "$S3_CONTAINER" 9000/tcp | awk -F: 'NR == 1 { print $NF }')"
S3_ENDPOINT="http://127.0.0.1:${S3_PORT}"
for _ in {1..80}; do
  if curl --silent --fail "$S3_ENDPOINT/health/ready" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
curl --silent --fail "$S3_ENDPOINT/health/ready" >/dev/null

docker run --rm --network "container:$S3_CONTAINER" \
  --entrypoint /bin/sh "$AWS_CLI_IMAGE" -c \
  "AWS_ENDPOINT_URL=http://127.0.0.1:9000 AWS_ACCESS_KEY_ID='$S3_ACCESS_KEY' AWS_SECRET_ACCESS_KEY='$S3_SECRET_KEY' AWS_DEFAULT_REGION=us-east-1 aws s3api create-bucket --bucket '$S3_BUCKET' >/dev/null"

MULTIMEDIA_ACCEPTANCE=1 \
  MULTIMEDIA_S3_ENDPOINT="$S3_ENDPOINT" \
  MULTIMEDIA_S3_BUCKET="$S3_BUCKET" \
  MULTIMEDIA_S3_ACCESS_KEY="$S3_ACCESS_KEY" \
  MULTIMEDIA_S3_SECRET_KEY="$S3_SECRET_KEY" \
  go test ./internal/acceptance -run '^TestMultimediaS3OneGiBAcceptance$' -count=1 -v -timeout=20m

echo "OK: multimedia heavy acceptance passed"
