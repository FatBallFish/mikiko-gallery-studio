#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

TASK=""
TRACK="auto"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --task)
      TASK="${2:-}"
      shift 2
      ;;
    --track)
      TRACK="${2:-auto}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [ -z "$TASK" ]; then
  TASK="unspecified task"
fi

DISCOVERY_OUTPUT="$(./scripts/workflow/discover-sources.sh --task "$TASK")"
REQ="$(printf '%s\n' "$DISCOVERY_OUTPUT" | awk -F= '$1=="requirement" {print $2}')"
DESIGN="$(printf '%s\n' "$DISCOVERY_OUTPUT" | awk -F= '$1=="technical_design" {print $2}')"

detected_track="lightweight"
if printf '%s' "$TASK" | grep -Eqi 'api|schema|migration|auth|permission|security|docker|deploy|config|architecture|cache|queue|worker|goroutine|breaking|contract|数据库|鉴权|权限|部署|架构|迁移|配置'; then
  detected_track="heavyweight"
fi
if [ "$TRACK" != "auto" ]; then
  detected_track="$TRACK"
fi

mkdir -p .workflow
cat > .coding-context.json <<EOF
{
  "schema_version": 1,
  "task": "$(printf '%s' "$TASK" | sed 's/\\/\\\\/g; s/"/\\"/g')",
  "track": "$detected_track",
  "requirement_source": "$REQ",
  "technical_design_source": "$DESIGN",
  "source_discovery_report": ".workflow/source-discovery.json",
  "created_at": "$(date '+%Y-%m-%dT%H:%M:%S%z')",
  "approval": {
    "required": $([ "$detected_track" = "heavyweight" ] && echo true || echo false),
    "status": "$([ "$detected_track" = "heavyweight" ] && echo pending || echo not_required)"
  },
  "implementation_plan": [
    "Read requirement and technical-design source before editing.",
    "Identify impacted backend/frontend/API/deployment surfaces.",
    "Implement in scoped changes with tests.",
    "Run scripts/workflow/verify.sh.",
    "Run scripts/workflow/review-local.sh --scope committed.",
    "Run scripts/workflow/api-smoke.sh when backend/API/config/deployment changed."
  ]
}
EOF

echo "OK: wrote .coding-context.json"
echo "requirement_source=$REQ"
echo "technical_design_source=$DESIGN"
echo "track=$detected_track"

