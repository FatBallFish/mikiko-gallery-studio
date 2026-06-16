#!/usr/bin/env bash
set -euo pipefail

SERVER="${1:?usage: scripts/devops/check-remote.sh <ssh-host> [user-domain] [admin-domain]}"
USER_DOMAIN="${2:-test.mikiko.studio}"
ADMIN_DOMAIN="${3:-test-admin.mikiko.studio}"

ssh "$SERVER" bash -s -- "$USER_DOMAIN" "$ADMIN_DOMAIN" <<'REMOTE'
set -euo pipefail

USER_DOMAIN=$1
ADMIN_DOMAIN=$2

failures=0
fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

check_path() {
  local label=$1
  local path=$2
  if [[ ! -e "$path" ]]; then
    fail "$label missing: $path"
    return 1
  fi
  echo "OK: $label exists: $path"
}

fetch_title() {
  local host=$1
  local path=${2:-/}
  curl -k -sS --resolve "$host:443:127.0.0.1" "https://$host$path" 2>/dev/null | grep -Eo '<title>[^<]+' | head -1 || true
}

echo "==> Frontend site files"
USER_INDEX="/opt/1panel/www/sites/$USER_DOMAIN/index/index.html"
ADMIN_INDEX="/opt/1panel/www/sites/$ADMIN_DOMAIN/index/index.html"
check_path "user index" "$USER_INDEX" || true
check_path "admin index" "$ADMIN_INDEX" || true

if [[ -f "$USER_INDEX" && -f "$ADMIN_INDEX" ]]; then
  user_sum="$(sha256sum "$USER_INDEX" | awk '{print $1}')"
  admin_sum="$(sha256sum "$ADMIN_INDEX" | awk '{print $1}')"
  if [[ "$user_sum" == "$admin_sum" ]]; then
    fail "user and admin index.html are identical; admin site likely received user-web artifact"
  else
    echo "OK: user and admin index.html differ"
  fi
fi

echo "==> Frontend HTTP routes"
user_admin_headers="/tmp/pic-gallery-user-admin.headers"
user_admin_status="$(curl -k -sS -D "$user_admin_headers" -o /tmp/pic-gallery-user-admin.html -w '%{http_code}' --resolve "$USER_DOMAIN:443:127.0.0.1" "https://$USER_DOMAIN/admin/" || true)"
admin_status="$(curl -k -sS -o /tmp/pic-gallery-admin-root.html -w '%{http_code}' --resolve "$ADMIN_DOMAIN:443:127.0.0.1" "https://$ADMIN_DOMAIN/" || true)"
case "$user_admin_status" in
  200) ;;
  301|302|307|308)
    tr -d '\r' <"$user_admin_headers" | grep -Fixq "location: https://$ADMIN_DOMAIN/" || fail "$USER_DOMAIN/admin/ redirects to unexpected location"
    ;;
  *) fail "$USER_DOMAIN/admin/ returned HTTP $user_admin_status" ;;
esac
[[ "$admin_status" == "200" ]] || fail "$ADMIN_DOMAIN/ returned HTTP $admin_status"

admin_root_html="$(cat /tmp/pic-gallery-admin-root.html 2>/dev/null || true)"
if grep -Fq '<title>Mikiko Studio' <<<"$admin_root_html" || grep -Fq 'mikiko-studio' <<<"$admin_root_html"; then
  fail "$ADMIN_DOMAIN/ looks like user landing page, not admin console"
fi
grep -Fq 'Pic Gallery Admin Console' <<<"$admin_root_html" || fail "$ADMIN_DOMAIN/ does not look like admin console"

echo "user /admin title: $(fetch_title "$USER_DOMAIN" /admin/)"
echo "admin / title: $(fetch_title "$ADMIN_DOMAIN" /)"

echo "==> Backend API"
if curl -sS http://127.0.0.1:8080/readyz >/tmp/pic-gallery-readyz.json; then
  echo "OK: api readyz reachable"
else
  fail "api readyz is not reachable at 127.0.0.1:8080"
fi

echo "==> Services"
systemctl is-active --quiet pic-gallery-api.service && echo "OK: pic-gallery-api active" || fail "pic-gallery-api service is not active"
systemctl is-active --quiet pic-gallery-worker.service && echo "OK: pic-gallery-worker active" || fail "pic-gallery-worker service is not active"

pgrep -f 'pic-gallery-worker' >/dev/null && echo "OK: worker process exists" || fail "worker process not found"

echo "==> Startup scripts"
for script in /home/pic-gallery/api-server/start-pic-gallery-api.sh /home/pic-gallery/worker/start-pic-gallery-worker.sh; do
  if [[ -f "$script" ]]; then
    if grep -Fq 'scripts/service/manage.sh' "$script"; then
      fail "$script still references scripts/service/manage.sh"
    else
      echo "OK: $script is standalone"
    fi
  else
    echo "WARN: startup script not found: $script"
  fi
done

if [[ "$failures" -gt 0 ]]; then
  echo "Deployment check failed with $failures issue(s)" >&2
  exit 1
fi
echo "OK: deployment check passed"
REMOTE
