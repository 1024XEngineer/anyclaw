#!/usr/bin/env sh
set -eu

SERVER_ORIGIN="${SERVER_ORIGIN:-http://127.0.0.1}"
ADMIN_TOKEN="${ANYCLAW_REGISTRY_ADMIN_TOKEN:-}"

echo "Smoke target: $SERVER_ORIGIN"

curl -fsS "$SERVER_ORIGIN/" >/dev/null
echo "website: ok"

curl -fsS "$SERVER_ORIGIN/v1/health" >/dev/null
echo "registry health through website proxy: ok"

ARTIFACTS="$(curl -fsS "$SERVER_ORIGIN/v1/artifacts")"
echo "$ARTIFACTS" | grep -q '"items"'
echo "registry artifacts through website proxy: ok"

if [ -n "$ADMIN_TOKEN" ]; then
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$SERVER_ORIGIN/v1/admin/audit" >/dev/null
  echo "admin audit with token: ok"
else
  STATUS="$(curl -s -o /dev/null -w "%{http_code}" "$SERVER_ORIGIN/v1/admin/audit")"
  if [ "$STATUS" != "401" ]; then
    echo "expected admin audit without token to return 401, got $STATUS" >&2
    exit 1
  fi
  echo "admin audit without token: 401"
fi
