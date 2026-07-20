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

stop_e2e_children() {
  local attempt child_state
  if [[ "$E2E_CHILD_PGID" =~ ^[0-9]+$ ]] && kill -0 -- "-$E2E_CHILD_PGID" 2>/dev/null; then
    kill -TERM -- "-$E2E_CHILD_PGID" >/dev/null 2>&1 || true
    for attempt in {1..50}; do
      if [[ "$E2E_CHILD_PID" =~ ^[0-9]+$ ]]; then
        child_state="$(ps -o stat= -p "$E2E_CHILD_PID" 2>/dev/null | tr -d '[:space:]' || true)"
        if [[ -z "$child_state" || "$child_state" == Z* ]]; then
          wait "$E2E_CHILD_PID" >/dev/null 2>&1 || true
          E2E_CHILD_PID=""
        fi
      fi
      kill -0 -- "-$E2E_CHILD_PGID" 2>/dev/null || break
      sleep 0.1
    done
    if kill -0 -- "-$E2E_CHILD_PGID" 2>/dev/null; then
      kill -KILL -- "-$E2E_CHILD_PGID" >/dev/null 2>&1 || true
      for attempt in {1..20}; do
        if [[ "$E2E_CHILD_PID" =~ ^[0-9]+$ ]]; then
          child_state="$(ps -o stat= -p "$E2E_CHILD_PID" 2>/dev/null | tr -d '[:space:]' || true)"
          if [[ -z "$child_state" || "$child_state" == Z* ]]; then
            wait "$E2E_CHILD_PID" >/dev/null 2>&1 || true
            E2E_CHILD_PID=""
          fi
        fi
        kill -0 -- "-$E2E_CHILD_PGID" 2>/dev/null || break
        sleep 0.1
      done
    fi
    if kill -0 -- "-$E2E_CHILD_PGID" 2>/dev/null; then
      return 1
    fi
  fi
  if [[ "$E2E_CHILD_PID" =~ ^[0-9]+$ ]]; then
    wait "$E2E_CHILD_PID" >/dev/null 2>&1 || true
  fi
  E2E_CHILD_PID=""
  E2E_CHILD_PGID=""
}
