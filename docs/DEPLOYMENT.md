# VPS deployment with GitHub Actions

The workflow at `.github/workflows/deploy.yml` deploys the `main` branch over SSH and runs Docker Compose on the VPS.

## VPS preparation

Use an Ubuntu VPS with Docker Engine and the Docker Compose plugin installed. Create a non-root deployment user and clone this repository at:

```text
/opt/educraft360-central
```

The deployment user must be able to run Docker without `sudo` and must be able to pull the repository from GitHub. For a private repository, configure a read-only GitHub deploy key on the VPS and add the matching public key to the repository.

Copy the production environment file to `/opt/educraft360-central/.env`. Never commit it. At minimum configure:

```env
POSTGRES_PASSWORD=<strong-random-password>
APP_ORIGIN=https://central.educraft360.com
COOKIE_SECURE=true
INVITATION_SIGNING_KEY=<long-random-secret>
BOOTSTRAP_ADMIN_EMAIL=<initial-owner-email>
BOOTSTRAP_ADMIN_PASSWORD=<initial-owner-password>
BOOTSTRAP_ADMIN_NAME=Platform Owner
RESEND_API_KEY=<resend-api-key>
RESEND_FROM_EMAIL=EduCraft360 <noreply@central.educraft360.com>
```

Start the database and create the schema once:

```bash
cd /opt/educraft360-central
docker compose up -d db redis
docker compose run --rm api migrate
docker compose run --rm api bootstrap
```

Do not run `bootstrap` again after the first owner has been created.

## Reverse proxy and DNS

The Compose file binds the Next.js web container to `127.0.0.1:3000`. Put Caddy or Nginx in front of it and proxy:

```text
https://central.educraft360.com  ->  http://127.0.0.1:3000
```

Only ports 80 and 443 should be public. PostgreSQL, Redis, and the Go API must remain private.

In Namecheap DNS create:

```text
Type: A
Host: central
Value: <VPS_PUBLIC_IP>
TTL: Automatic
```

## GitHub Actions secrets

Add these repository secrets under **Settings → Secrets and variables → Actions**:

| Secret | Value |
| --- | --- |
| `VPS_HOST` | VPS public IP or hostname |
| `VPS_USER` | Non-root deployment username |
| `VPS_SSH_KEY` | Private SSH key accepted by the VPS user |
| `VPS_SSH_PORT` | SSH port, usually `22` |
| `VPS_APP_DIR` | `/opt/educraft360-central` |

The workflow runs on every push to `main` and can also be started manually with **Run workflow**.

## What each deployment does

1. Connects to the VPS through SSH.
2. Fetches and checks out `origin/main`.
3. Builds and starts `db`, `redis`, `api`, and `web`.
4. Runs the committed database migration file.
5. Checks `http://127.0.0.1:3000/api/health` through the Next.js proxy.
6. Writes the deployed commit to `.deployed-commit`.

If a deployment fails, the application checkout is restored to the previous commit and the application containers are rebuilt. Database migrations are not automatically rolled back; make destructive schema changes reversible and back up PostgreSQL before production migrations.

## First deployment checklist

- DNS A record resolves to the VPS.
- HTTPS reverse proxy is configured and certificate issuance succeeds.
- `.env` exists only on the VPS with production secrets.
- GitHub Actions secrets are configured.
- The VPS user can run `docker compose ps` without sudo.
- The initial migration and bootstrap have been run once.
- `https://central.educraft360.com/api/health` returns `{"status":"ok"}`.
- Sign in and immediately change the bootstrap password.
