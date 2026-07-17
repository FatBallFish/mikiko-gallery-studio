#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REGISTRY="${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}"
TAG="test"
PUSH=false
LATEST=false
PLATFORM_ARGS=()

usage() {
  cat <<'USAGE'
Usage:
  scripts/docker/images.sh build [--tag TAG] [--registry REGISTRY] [--platform linux/amd64,linux/arm64]
  scripts/docker/images.sh push [--tag TAG] [--registry REGISTRY]
  scripts/docker/images.sh release --version vX.Y.Z [--registry REGISTRY] [--latest]

Defaults:
  --tag test
  --registry ${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}

Images:
  pic-gallery-api
  pic-gallery-worker
  pic-gallery-user-web
  pic-gallery-admin-web
  pic-gallery-docs-web
USAGE
}

die() {
  echo "error: $*" >&2
  exit 2
}

image_ref() {
  local name=$1
  local tag=$2
  echo "${REGISTRY}/${name}:${tag}"
}

build_one() {
  local name=$1
  local dockerfile=$2
  local tag=$3
  local ref
  ref="$(image_ref "$name" "$tag")"
  echo "==> Building $ref"
  if [[ ${#PLATFORM_ARGS[@]} -gt 0 ]]; then
    if [[ "$PUSH" == true ]]; then
      docker buildx build "${PLATFORM_ARGS[@]}" -t "$ref" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR" --push
    else
      docker buildx build "${PLATFORM_ARGS[@]}" -t "$ref" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR" --load
    fi
  else
    docker build -t "$ref" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR"
    if [[ "$PUSH" == true ]]; then
      docker push "$ref"
    fi
  fi
}

build_all() {
  local tag=$1
  build_one pic-gallery-api Dockerfile.api "$tag"
  build_one pic-gallery-worker Dockerfile.worker "$tag"
  build_one pic-gallery-user-web Dockerfile.user-web "$tag"
  build_one pic-gallery-admin-web Dockerfile.admin-web "$tag"
  build_one pic-gallery-docs-web Dockerfile.docs-web "$tag"
}

push_all() {
  local tag=$1
  for name in pic-gallery-api pic-gallery-worker pic-gallery-user-web pic-gallery-admin-web pic-gallery-docs-web; do
    docker push "$(image_ref "$name" "$tag")"
  done
}

require_version_tag() {
  local version=$1
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9._-]+)?$ ]] || die "--version must look like v1.2.3"
}

cmd="${1:-}"
shift || true
case "$cmd" in
  build|push)
    ;;
  release)
    TAG=""
    ;;
  -h|--help|"")
    usage
    exit 0
    ;;
  *)
    die "unknown command: $cmd"
    ;;
esac

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="${2:?missing tag}"
      shift 2
      ;;
    --version)
      TAG="${2:?missing version}"
      shift 2
      ;;
    --registry)
      REGISTRY="${2:?missing registry}"
      shift 2
      ;;
    --platform)
      PLATFORM_ARGS=(--platform "${2:?missing platform}")
      shift 2
      ;;
    --latest)
      LATEST=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

case "$cmd" in
  build)
    build_all "$TAG"
    ;;
  push)
    push_all "$TAG"
    ;;
  release)
    [[ -n "$TAG" ]] || die "release requires --version vX.Y.Z"
    require_version_tag "$TAG"
    PUSH=true
    build_all "$TAG"
    if [[ "$LATEST" == true ]]; then
      for name in pic-gallery-api pic-gallery-worker pic-gallery-user-web pic-gallery-admin-web pic-gallery-docs-web; do
        docker tag "$(image_ref "$name" "$TAG")" "$(image_ref "$name" latest)"
        docker push "$(image_ref "$name" latest)"
      done
    fi
    ;;
esac
