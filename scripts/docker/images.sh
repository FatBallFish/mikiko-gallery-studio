#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REGISTRY=${IMAGE_REGISTRY:-docker.io/fatballfish}
REGISTRY=${PIC_GALLERY_IMAGE_REGISTRY:-$REGISTRY}
TAG=""
PUSH=false
LATEST=false
PLATFORM_ARGS=()
METADATA_DIR=""
REVISION="${RELEASE_COMMIT:-}"
SOURCE_URL="${RELEASE_SOURCE_URL:-https://github.com/FatBallFish/mikiko-gallery-studio}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/docker/images.sh build [--tag TAG] [--registry REGISTRY] [--platform linux/amd64,linux/arm64]
  scripts/docker/images.sh push [--tag TAG] [--registry REGISTRY]
  scripts/docker/images.sh release --version vX.Y.Z [--registry REGISTRY] [--latest] [--metadata-dir DIR]

Defaults:
  --tag sha-<commit>[-dirty-<content-hash>]
  --registry docker.io/fatballfish

Images:
  mikiko-gallery-studio-api
  mikiko-gallery-studio-worker
  mikiko-gallery-studio-user-web
  mikiko-gallery-studio-admin-web
  mikiko-gallery-studio-docs-web
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

sha256_stream() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    die "sha256sum or shasum is required"
  fi
}

source_version() {
  local commit dirty_hash
  commit="$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')"
  if git -C "$ROOT_DIR" diff --quiet --ignore-submodules HEAD -- && \
      git -C "$ROOT_DIR" diff --cached --quiet --ignore-submodules && \
      [[ -z "$(git -C "$ROOT_DIR" ls-files --others --exclude-standard)" ]]; then
    printf 'sha-%s\n' "$commit"
    return
  fi
  dirty_hash="$({
    git -C "$ROOT_DIR" diff --binary HEAD --
    git -C "$ROOT_DIR" ls-files --others --exclude-standard -z | while IFS= read -r -d '' path; do
      printf '%s\0' "$path"
      sha256_stream < "$ROOT_DIR/$path"
    done
  } | sha256_stream)"
  printf 'sha-%s-dirty-%s\n' "$commit" "${dirty_hash:0:12}"
}

resolve_revision() {
  if [[ -n "$REVISION" ]]; then
    return
  fi
  REVISION="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || printf 'unknown')"
}

build_one() {
  local name=$1
  local dockerfile=$2
  local tag=$3
  local ref metadata_file=""
  local -a tag_args label_args metadata_args
  ref="$(image_ref "$name" "$tag")"
  tag_args=(-t "$ref")
  if [[ "$LATEST" == true ]]; then
    tag_args+=(-t "$(image_ref "$name" latest)")
  fi
  label_args=(
    --label "org.opencontainers.image.version=$tag"
    --label "org.opencontainers.image.revision=$REVISION"
    --label "org.opencontainers.image.source=$SOURCE_URL"
  )
  metadata_args=()
  if [[ -n "$METADATA_DIR" ]]; then
    mkdir -p "$METADATA_DIR"
    metadata_file="$METADATA_DIR/${name#mikiko-gallery-studio-}.json"
    metadata_args=(--metadata-file "$metadata_file")
  fi
  echo "==> Building $ref"
  if [[ ${#PLATFORM_ARGS[@]} -gt 0 ]]; then
    if [[ "$PUSH" == true ]]; then
      docker buildx build "${PLATFORM_ARGS[@]}" "${tag_args[@]}" "${label_args[@]}" "${metadata_args[@]}" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR" --push
    else
      docker buildx build "${PLATFORM_ARGS[@]}" "${tag_args[@]}" "${label_args[@]}" "${metadata_args[@]}" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR" --load
    fi
  else
    docker build "${tag_args[@]}" "${label_args[@]}" -f "$ROOT_DIR/$dockerfile" "$ROOT_DIR"
    if [[ "$PUSH" == true ]]; then
      docker push "$ref"
    fi
  fi
}

build_all() {
  local tag=$1
  build_one mikiko-gallery-studio-api Dockerfile.api "$tag"
  build_one mikiko-gallery-studio-worker Dockerfile.worker "$tag"
  build_one mikiko-gallery-studio-user-web Dockerfile.user-web "$tag"
  build_one mikiko-gallery-studio-admin-web Dockerfile.admin-web "$tag"
  build_one mikiko-gallery-studio-docs-web Dockerfile.docs-web "$tag"
}

push_all() {
  local tag=$1
  for name in mikiko-gallery-studio-api mikiko-gallery-studio-worker mikiko-gallery-studio-user-web mikiko-gallery-studio-admin-web mikiko-gallery-studio-docs-web; do
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
    --metadata-dir)
      METADATA_DIR="${2:?missing metadata directory}"
      shift 2
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
    [[ -n "$TAG" ]] || TAG="$(source_version)"
    resolve_revision
    build_all "$TAG"
    ;;
  push)
    [[ -n "$TAG" ]] || TAG="$(source_version)"
    push_all "$TAG"
    ;;
  release)
    [[ -n "$TAG" ]] || die "release requires --version vX.Y.Z"
    require_version_tag "$TAG"
    resolve_revision
    if [[ ${#PLATFORM_ARGS[@]} -eq 0 ]]; then
      PLATFORM_ARGS=(--platform "linux/amd64,linux/arm64")
    fi
    PUSH=true
    build_all "$TAG"
    ;;
esac
