#!/usr/bin/env bash
set -euo pipefail

domain="${EXTENSION_RELEASES_DOMAIN:-releases.ai-translate.bogdanantonovich.com}"
base_url="https://$domain"

if [[ ! "${EXTENSION_RELEASES_ACCESS_TOKEN:-}" =~ ^[A-Za-z0-9_-]{32,128}$ ]]; then
  echo "Set EXTENSION_RELEASES_ACCESS_TOKEN to the configured 32-128 character token." >&2
  exit 1
fi

prompt_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "$base_url/")"
if [ "$prompt_status" != "200" ]; then
  echo "Expected the access-key page to return 200, got $prompt_status." >&2
  exit 1
fi

unauthenticated_status="$(curl \
  --silent \
  --output /dev/null \
  --write-out '%{http_code}' \
  "$base_url/latest/ai-translate-extension.zip")"
if [ "$unauthenticated_status" != "404" ]; then
  echo "Expected an unauthenticated 404, got $unauthenticated_status." >&2
  exit 1
fi

metadata="$(curl \
  --fail \
  --silent \
  --show-error \
  --header "Authorization: Bearer $EXTENSION_RELEASES_ACCESS_TOKEN" \
  "$base_url/latest.json")"

if [ -n "${EXPECTED_COMMIT:-}" ] && ! grep -Fq '"commit": "'"$EXPECTED_COMMIT"'"' <<< "$metadata"; then
  echo "The published metadata does not identify expected commit $EXPECTED_COMMIT." >&2
  exit 1
fi

curl \
  --fail \
  --silent \
  --show-error \
  --output /dev/null \
  --header "Authorization: Bearer $EXTENSION_RELEASES_ACCESS_TOKEN" \
  "$base_url/latest/ai-translate-extension.zip"

echo "Private extension release endpoint is healthy."
