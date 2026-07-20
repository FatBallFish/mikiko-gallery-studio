#!/usr/bin/env bash

writers_are_stopped() {
  local entry service container_id running
  for entry in "${WRITER_CONTAINER_IDS[@]}"; do
    service="${entry%%:*}"
    container_id="${entry#*:}"
    if ! running="$(docker inspect -f '{{.State.Running}}' "$container_id" 2>/dev/null)"; then
      echo "shared local E2E: failed to inspect writer state after stop: $service" >&2
      return 1
    fi
    if [[ "$running" != "false" ]]; then
      echo "shared local E2E: writer is still running after stop: $service" >&2
      return 1
    fi
  done
}

stop_writers() {
  local service container_id
  WRITERS_STOPPED=true
  WRITER_CONTAINER_IDS=()
  for service in api worker minio; do
    if ! container_id="$("${COMPOSE[@]}" ps -a -q "$service")"; then
      echo "shared local E2E: failed to resolve writer container: $service" >&2
      return 1
    fi
    if [[ -z "$container_id" ]]; then
      echo "shared local E2E: writer container is missing: $service" >&2
      return 1
    fi
    WRITER_CONTAINER_IDS+=("$service:$container_id")
  done
  "${COMPOSE[@]}" stop api worker minio >/dev/null || return 1
  writers_are_stopped
}
