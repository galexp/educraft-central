# EduCraft360 Central — Initial MVP

A standalone central customer-management platform for **TechKnights Global Solutions**, with a Go API and Next.js admin interface in one repository.

The complete endpoint reference is available in [docs/API.md](docs/API.md).

The deployed public API is served under `https://central.educraft360.com/api`; endpoint paths in the documentation are relative to that prefix.

## Included in this MVP

- **Schools:** create schools, list/search records, change school status and record hosting ownership.
- **Operators:** bootstrap first owner, login/logout with secure HttpOnly session cookie, roles (`owner`, `admin`, `finance`, `support`, `viewer`) and staff account creation.
- **Dashboard:** counts for schools, invoices and support tickets, plus outstanding issued invoice balance.
- **Commercial:** plans, subscriptions, invoice creation/issuance, manually recorded payments with duplicate-reference prevention and overpayment checks.
- **Entitlements:** subscriptions can map to the five default module plans; term activations return the combined feature set to the school instance.
- **Support:** tickets, categories, priority, assignment, status, internal notes and replies. **No live chat.**
- **School installation handoff:** central-issued, signed, expiring invitations that a school instance verifies and consumes without receiving central database credentials or JWT secrets.
- **Transactional email:** Resend sends installation links and activation codes to the school’s registered email when configured.
- **Minimal audit:** records significant authenticated operator actions. This is not an advanced compliance/tamper-proof audit system.

**Not included:** infrastructure provisioning, school runtime identity, licence issuance, term activation, automated monitoring, automated email notifications, school-side support portal, attachments, payment-gateway integration, invoice PDFs, comprehensive MFA, customer login, and advanced SLA/reporting.

## Stack

- Go 1.23+, Go standard-library HTTP router, pgx v5, bcrypt
- Next.js 15 / React 19 / TypeScript
- PostgreSQL 17, Redis 7 (reserved for later jobs)
- Docker Compose

## Run locally with Docker

1. Create `.env` from `.env.example` and set **new unique passwords**. Never commit `.env`.
2. Run `docker compose build`.
3. Run `docker compose up -d db redis`.
4. Run `docker compose run --rm api migrate` to create the database tables.
5. Run `docker compose run --rm api bootstrap` **once** to create the first owner account.
6. Run `docker compose up -d`.
7. Browse to **http://localhost:3000**. Log in with your `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD`.

The frontend proxies `/api/*` requests to the private Go container; the API and database are **not published** to the public internet by the Compose file.

## Local development without full Docker build

Start PostgreSQL and create an empty database. Export `DATABASE_URL`, `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD`, and a strong `INVITATION_SIGNING_KEY` in the API terminal. From `backend/`, run `go run ./cmd/api migrate`, then `go run ./cmd/api bootstrap` once. Start the API with `LISTEN_HOST=127.0.0.1 go run ./cmd/api`.

In a second terminal, run `cd frontend && npm ci && npm run dev -- --hostname 127.0.0.1`. Open `http://localhost:3000` and sign in with the bootstrap account. The Next.js rewrite defaults to `http://127.0.0.1:8080`.

For Go API hot reload, install the Go Air watcher and use the project config. macOS may already have an unrelated R tool named `air`, so invoke the Go binary by its full path:

```bash
go install github.com/air-verse/air@latest
cd backend
"$(go env GOPATH)/bin/air" -c .air.toml
```

## Initial routes

For complete request/response contracts and school-instance integration guidance, see [`docs/API.md`](docs/API.md).

| Verb | Path | Access |
|---|---|---|
| GET | `/health` | Public (internal network) |
| POST | `/auth/login` | Public (rate limiting not yet added) |
| POST | `/auth/logout` | Authenticated session |
| GET | `/auth/me` | Authenticated session |
| GET | `/dashboard` | Any operator |
| GET, POST | `/schools` | GET any operator; POST owner/admin |
| PATCH | `/schools/status` | Owner/admin |
| GET, POST | `/plans` | GET any operator; POST owner/admin/finance |
| DELETE | `/plans/{id}` | Owner/admin/finance; only if no subscription uses the plan |
| POST | `/term-activations` | Owner/admin/finance; issue a term/season activation code |
| POST | `/term-activations/verify` | School instance; activate a code and receive feature entitlements |
| GET, POST | `/subscriptions` | GET any operator; POST owner/admin/finance |
| GET, POST | `/invoices` | GET any operator; POST owner/admin/finance |
| POST | `/invoices/issue` | Owner/admin/finance |
| POST | `/payments` | Owner/admin/finance |
| GET, POST | `/tickets` | GET any operator; POST owner/admin/support |
| GET | `/tickets/{id}` | Any operator. Internal notes are limited to owner/admin/support. |
| GET | `/tickets/assignees` | Owner/admin/support |
| PATCH | `/tickets` | Owner/admin/support |
| POST | `/tickets/replies` | Owner/admin/support |
| GET, POST | `/operators` | Owner/admin |
| POST | `/installation-invitations` | Owner/admin; issue a signed school-install invitation |
| GET | `/invitations?school_id=...` | Owner/admin; track one school’s active, expired, consumed, and bound invitations |
| DELETE | `/installation-invitations/{id}` | Owner/admin; revoke an approved, unused, unexpired invitation |
| POST | `/installation-invitations/verify` | School instance; verify and bind an invitation to its instance identity |
| POST | `/installation-invitations/consume` | School instance; mark a verified invitation as used |

## Security / production readiness

This is a **development MVP**, not a security-audited production release. Before external deployment:

1. Add mandatory operator MFA, account recovery and login rate limiting / lockout.
2. Validate inputs more strictly, especially emails, currency codes, arbitrary text lengths, ID formats, and payment/reference policies.
3. Enforce per-resource access controls and a full authorisation matrix, especially for finance and support data; restrict sensitive list and detail endpoints to appropriate roles.
4. Add CSRF tokens or a well-defined strict same-origin policy at the reverse proxy; keep cookie SameSite enabled and configure `APP_ORIGIN` to match the public HTTPS frontend origin.
5. Add TLS, secure cookies (`COOKIE_SECURE=true`), hardened headers and trusted reverse-proxy settings.
6. Add schema migrations with ordering/version tracking, transaction-safe multi-step changes, automated integration tests, and robust observability.
7. Expand outstanding balance reporting beyond NGN, with a separate total for each currency. Monetary values are stored as integer minor units.
8. Implement secure workflows for operator password changes, credential rotation and session revocation.
9. Add idempotency keys and confirmation/audit requirements for financial actions.
10. Add access-controlled school-side ticket submission through an authenticated, privacy-reviewed integration—**not** a public unauthenticated ticket endpoint.
11. Configure backups and restore tests, secret management, dependency scanning and CI/CD.

**Do not use real student data or school administrator/database credentials in this central platform.**

### School installation invitation flow

Set `INVITATION_SIGNING_KEY` to a long random value kept only by the central API. The school `domain` saved during school creation determines the install page, for example `demo.educraft360.com` becomes `https://demo.educraft360.com/install`. An owner or admin creates an invitation with `POST /installation-invitations` and receives a one-time `install_url` containing the signed token fragment. The school instance sends that token plus its own `instance_id` to `/installation-invitations/verify`, performs its local setup, then calls `/installation-invitations/consume`. The central API stores only a token hash and instance identity; it never receives the school database password, administrator password or JWT secret.

## Monorepo decision

Both applications live in this single repository. Each builds in a separate container. A separately restricted provisioner can be introduced later without changing this structure.
