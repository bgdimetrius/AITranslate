#!/usr/bin/env bash
set -euo pipefail

domain="${API_DOMAIN:-ai-translate.bogdanantonovich.com}"
api_port="${API_PORT:-4161}"

local_status="$(curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}' \
  "http://127.0.0.1:$api_port/health")"
if [ "$local_status" != "204" ]; then
  echo "Local API health check returned $local_status; expected 204." >&2
  exit 1
fi

public_status="$(curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}' \
  "https://$domain/health")"
if [ "$public_status" != "204" ]; then
  echo "Public API health check returned $public_status; expected 204." >&2
  exit 1
fi

echo "API is healthy locally and at https://$domain."
