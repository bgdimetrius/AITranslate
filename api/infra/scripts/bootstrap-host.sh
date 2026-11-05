#!/usr/bin/env bash
set -euo pipefail

domain="${API_DOMAIN:-ai-translate.bogdanantonovich.com}"
api_port="${API_PORT:-4161}"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
infra_root="$(CDPATH= cd -- "$script_dir/.." && pwd)"
nginx_available="/etc/nginx/sites-available/$domain"
nginx_enabled="/etc/nginx/sites-enabled/$domain"
certbot_hook_path="/etc/letsencrypt/renewal-hooks/deploy/ai-translate-nginx"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

render_config() {
  local source="$1"
  local destination="$2"

  sed \
    -e "s/__API_DOMAIN__/$domain/g" \
    -e "s/__API_PORT__/$api_port/g" \
    "$source" > "$destination"
}

if [ "$(id -u)" -ne 0 ]; then
  fail "Run this script with sudo."
fi

if [[ ! "$domain" =~ ^[A-Za-z0-9.-]+$ ]] || [[ "$domain" == .* ]] || [[ "$domain" == *. ]]; then
  fail "API_DOMAIN is not a valid DNS hostname: $domain"
fi
if [[ ! "$api_port" =~ ^[0-9]+$ ]] || (( api_port < 1 || api_port > 65535 )); then
  fail "API_PORT must be a number from 1 to 65535."
fi

for command in certbot curl nginx sed systemctl; do
  command -v "$command" >/dev/null || fail "Required command is not installed: $command"
done

escaped_domain="${domain//./\\.}"
for enabled_config in /etc/nginx/sites-enabled/*; do
  [ -e "$enabled_config" ] || continue
  if [ "$(readlink -f "$enabled_config")" = "$nginx_available" ]; then
    continue
  fi
  if grep -Eq "server_name[^;]*${escaped_domain}([[:space:];]|$)" "$enabled_config"; then
    fail "$domain is already declared in $enabled_config. Remove the legacy virtual host before bootstrapping this one."
  fi
done

bootstrap_config="$(mktemp)"
final_config="$(mktemp)"
trap 'rm -f "$bootstrap_config" "$final_config"' EXIT

render_config "$infra_root/nginx/api.bootstrap.conf" "$bootstrap_config"
render_config "$infra_root/nginx/api.conf" "$final_config"

install -d -o root -g root -m 0755 /var/www/certbot
install -o root -g root -m 0644 "$bootstrap_config" "$nginx_available"
ln -sfn "$nginx_available" "$nginx_enabled"
nginx -t
systemctl reload nginx

if [ ! -f "/etc/letsencrypt/live/$domain/fullchain.pem" ]; then
  if [ -z "${CERTBOT_EMAIL:-}" ]; then
    fail "Set CERTBOT_EMAIL so Certbot can issue the TLS certificate."
  fi
  certbot certonly \
    --webroot \
    --webroot-path /var/www/certbot \
    --domain "$domain" \
    --email "$CERTBOT_EMAIL" \
    --agree-tos \
    --non-interactive
fi

install -o root -g root -m 0644 "$final_config" "$nginx_available"
install -o root -g root -m 0755 "$script_dir/certbot-deploy-hook.sh" "$certbot_hook_path"
nginx -t
systemctl reload nginx

echo "Nginx is configured for https://$domain -> http://127.0.0.1:$api_port."
echo "Run $script_dir/health-check.sh after the API stack is deployed."
