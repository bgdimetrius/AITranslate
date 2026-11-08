# Private extension release hosting

Extension CD publishes the latest successful `main` build to an authenticated Nginx host:

```text
https://releases.ai-translate.bogdanantonovich.com/
```

The page asks for an access key and starts downloading `ai-translate-extension.zip` only when the
key is correct. Anonymous and incorrect-key requests receive `404` from the protected artifact
route. The server retains one release directory: every successful deployment switches `latest`
atomically and removes the superseded directory.

## Authentication

Create a URL-safe token locally:

```bash
openssl rand -hex 32
```

Save it as the GitHub Actions secret `EXTENSION_RELEASES_ACCESS_TOKEN`. The server accepts it as:

- `Authorization: Bearer TOKEN` for the download page and scripts.
- `?token=TOKEN` for a direct browser link. A valid query token sets a 30-day Secure, HttpOnly,
  SameSite cookie so later clean links work in that browser.
- The access key entered at the root download page. The page sends it as a bearer header and does
  not put it in the URL or save it.

Access logging is disabled for the release virtual host so query tokens are not written to the
Nginx access log. Prefer the download page or bearer header because query strings can remain in
browser history.

Protected endpoints:

```text
https://releases.ai-translate.bogdanantonovich.com/latest/ai-translate-extension.zip
https://releases.ai-translate.bogdanantonovich.com/latest.json
```

## GitHub configuration

Configure these repository secrets:

| Secret | Purpose |
| --- | --- |
| `EXTENSION_RELEASES_ACCESS_TOKEN` | 32-128 character private download key |
| `VPS_HOST` | Existing AI Translate VPS address |
| `VPS_USER` | Non-root SSH deployment user |
| `VPS_SSH_KEY` | SSH private key for the deployment user |

No Chrome Web Store credentials are required. The pipeline builds the store-compatible ZIP and
privately self-hosts it; it does not submit the package to the store.

## One-time VPS bootstrap

1. Point the `A` record, and `AAAA` if used, for
   `releases.ai-translate.bogdanantonovich.com` to the existing AI Translate VPS.
2. Ensure ports 80 and 443 reach host Nginx.
3. Add `EXTENSION_RELEASES_ACCESS_TOKEN` to GitHub and run Extension CD once. It syncs this
   infrastructure to `~/ai-translate-extension-releases-infra` and then stops with the bootstrap
   instruction.
4. Log into the VPS with the deployment user and run:

```bash
cd ~/ai-translate-extension-releases-infra
sudo CERTBOT_EMAIL=admin@bogdanantonovich.com \
  DEPLOY_USER="$USER" \
  ./scripts/bootstrap-host.sh
```

5. Rerun Extension CD.

Bootstrap installs a root-owned publisher and grants the deployment user passwordless sudo access
only to that publisher. A temporary random key protects the endpoint until the first successful CD
run replaces it with `EXTENSION_RELEASES_ACCESS_TOKEN`.

## Verification and key rotation

Run a manual health check from this directory:

```bash
EXTENSION_RELEASES_ACCESS_TOKEN='your-token' \
EXPECTED_COMMIT='full-40-character-commit-sha' \
./scripts/health-check.sh
```

To rotate access, change `EXTENSION_RELEASES_ACCESS_TOKEN` in GitHub and rerun Extension CD. The old
key stops working when Nginx reloads.
