# AITranslate API host setup

The production API runs in Docker Swarm on host port `4161`. Host Nginx terminates TLS and proxies
`/api/` plus the unversioned `/health` endpoint to that port. The configuration follows the
SinShield infrastructure pattern: an HTTP-only bootstrap virtual host is installed first, Certbot
issues a certificate when necessary, and the final HTTPS configuration is installed only after
`nginx -t` succeeds.

## Defaults

- Domain: `ai-translate.bogdanantonovich.com`
- API port: `4161`
- Public API base URL: `https://ai-translate.bogdanantonovich.com/api/v1`

Override the domain or port by setting `API_DOMAIN` or `API_PORT` while running the scripts.

## One-time VPS bootstrap

Before bootstrap:

1. Point the domain's `A` record to the VPS.
2. Ensure ports 80 and 443 reach host Nginx.
3. Install Nginx and Certbot.
4. Remove any legacy Nginx virtual host that already declares the same domain. The bootstrap script
   deliberately refuses to create a conflicting `server_name`.
5. Deploy or otherwise copy the tagged `api` directory to the VPS.

From the copied tag directory, run:

```bash
cd ~/AITranslate/api/RELEASE_TAG/infra
sudo CERTBOT_EMAIL=admin@bogdanantonovich.com ./scripts/bootstrap-host.sh
```

The existing certificate is reused when present. `CERTBOT_EMAIL` is required only when a new
certificate must be issued. The script installs a Certbot renewal hook that validates and reloads
Nginx after certificate renewal.

## Verification

After the API stack is deployed:

```bash
cd ~/AITranslate/api/RELEASE_TAG/infra
./scripts/health-check.sh
```

Both the local and public health endpoints must return HTTP `204`.
