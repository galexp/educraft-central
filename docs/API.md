# EduCraft360 Central API

Version: `v1` (current HTTP API)

## 1. Base URL and conventions

The public API base URL is:

```text
https://central.educraft360.com/api
```

Every route shown below is relative to that base URL. For example, `GET /schools` means `GET https://central.educraft360.com/api/schools`.

Production and local base URLs are deployment-specific. Local development normally uses the API origin directly:

```text
http://127.0.0.1:8080
```

When using the local frontend proxy, use `http://127.0.0.1:3000/api` instead.

JSON requests use `Content-Type: application/json` and `Accept: application/json`. IDs are UUID strings. Dates use `YYYY-MM-DD`; timestamps are ISO-8601 UTC. Money is represented in integer minor units (`95000` = NGN 950.00).

Errors always use:

```json
{"error":"human-readable message"}
```

Status codes: `200` success, `201` created, `400` validation error, `401` login required, `403` role denied, `404` not found, `409` conflict/invalid state, `502` email provider failure.

## 2. Central operator authentication

Central operators use an HttpOnly session cookie. Browser clients must use `credentials: "include"`. Never put a central operator password in a school instance.

### Login

```http
POST /auth/login
```

```json
{"email":"owner@example.com","password":"your-password"}
```

Response:

```json
{"id":"operator-uuid","name":"Platform Owner","email":"owner@example.com","role":"owner"}
```

### Session endpoints

```http
GET  /auth/me
POST /auth/logout
POST /auth/password
```

Password change body:

```json
{"current_password":"old-password","new_password":"new-password-at-least-12"}
```

### Roles

| Role | Access |
| --- | --- |
| `owner` | Full access |
| `admin` | Schools, invitations, team, support and operations |
| `finance` | Plans, subscriptions, invoices and payments; read school records |
| `support` | Tickets and school lookup |
| `viewer` | Overview/read-only areas exposed to the role |

Role restrictions are enforced by the API as well as the frontend.

## 3. Health

```http
GET /health
```

Returns `{"status":"ok"}`.

## 4. Schools

### List

```http
GET /schools
```

Records include `id`, `name`, `code`, `contact_email`, `domain`, `hosting`, `status`, and notes. The generated/registered `code` is the stable school identifier used by integrations and support tickets.

### Create

```http
POST /schools
```

```json
{
  "name":"Demo School",
  "code":"DEMO-116044",
  "contact_name":"School Admin",
  "contact_email":"admin@example.com",
  "contact_phone":"+2348000000000",
  "domain":"https://demo.educraft360.com/",
  "hosting":"school-managed",
  "notes":""
}
```

### Change status

```http
PATCH /schools/status
```

```json
{"id":"school-uuid","status":"active"}
```

## 5. Installation invitations

Invitations are signed, expiring, one-time links. Central stores a token hash and instance identity; it never receives the school database password, local administrator password, or JWT secret.

```http
POST   /installation-invitations
GET    /installation-invitations?school_id=school-uuid
GET    /installation-invitations/{id}/link
POST   /installation-invitations/{id}/email
DELETE /installation-invitations/{id}
```

Create body:

```json
{"school_id":"school-uuid","expires_in_hours":168}
```

The create response includes `id`, `school_id`, `expires_at`, and `install_url`.

### Verify from a school instance

```http
POST /installation-invitations/verify
```

```json
{
  "token":"SIGNED_TOKEN_FROM_INSTALL_URL",
  "instance_id":"stable-school-instance-id",
  "instance_url":"https://demo.educraft360.com"
}
```

Only continue installation when the response is successful. After local installation succeeds:

```http
POST /installation-invitations/consume
```

```json
{"token":"SIGNED_TOKEN_FROM_INSTALL_URL","instance_id":"stable-school-instance-id"}
```

Consumption is one-time.

### Exact invitation verification responses

`POST /installation-invitations/verify` success (`200`):

```json
{
  "valid": true,
  "invitation_id": "invitation-uuid",
  "school_id": "school-uuid",
  "school_name": "Demo School",
  "school_code": "DEMO-116044",
  "expires_at": "2026-11-23T01:15:00Z"
}
```

The response identifies the approved school by ID, name, and stable school code. It confirms the invitation expiry. The supplied `instance_id` is bound to the invitation but is not currently echoed; the client already knows the value it sent.

Verify errors:

| Status | Response |
| --- | --- |
| 400 | `{"error":"token and instance_id required"}` |
| 401 | `{"error":"invalid or expired invitation"}` |
| 404 | `{"error":"school not found"}` |
| 409 | `{"error":"invitation is unavailable"}` or `{"error":"invitation is bound to another instance"}` |
| 500 | `{"error":"invitation unavailable"}` or `{"error":"invitation could not be verified"}` |

`POST /installation-invitations/consume` success (`200`):

```json
{"consumed":true,"invitation_id":"invitation-uuid","school_id":"school-uuid","instance_id":"stable-school-instance-id","credential_id":"credential-uuid","api_token":"INSTANCE_TOKEN_RETURNED_ONCE","api_token_expires_at":"2027-11-23T01:15:00Z"}
```

Consume errors:

| Status | Response |
| --- | --- |
| 400 | `{"error":"token and instance_id required"}` |
| 401 | `{"error":"invalid or expired invitation"}` |
| 409 | `{"error":"invitation is unavailable"}` |
| 500 | `{"error":"invitation unavailable"}` or `{"error":"invitation could not be consumed"}` |

If the same consume request is sent twice, the first request succeeds and changes the invitation to `consumed`. The second request returns `409 {"error":"invitation is unavailable"}`. This prevents double-consumption, but the endpoint is not idempotent-confirming. After a timeout, retry once; if the retry returns this 409, treat the invitation as already used and verify the school’s local installation state.

## 5A. School-instance credentials and entitlements

Successful invitation consumption now issues a one-year school-instance credential. The raw `api_token` is returned only in the consume response; Central stores only its SHA-256 hash. Store the token encrypted in the school server’s secret store, never in browser JavaScript or source control.

Use it as:

```http
GET /school/subscription
Authorization: Bearer INSTANCE_TOKEN_RETURNED_ONCE
```

Success (`200`):

```json
{
  "school_id":"school-uuid",
  "school_code":"DEMO-116044",
  "instance_id":"stable-school-instance-id",
  "subscriptions":[
    {
      "id":"subscription-uuid",
      "status":"active",
      "starts_on":"2026-09-01",
      "ends_on":"2027-07-31",
      "plans":[{"code":"finance_fees","name":"Finance & Fees"}],
      "features":["finance_fees","reports"]
    }
  ]
}
```

The endpoint returns active/trial subscriptions whose end date has not passed. There is no refresh token; credentials expire after one year. Central owners/admins can revoke a credential immediately:

```http
DELETE /school-instance-credentials/{credential_id}
```

To rotate a credential, create a new invitation for the school, verify and consume it from the same/new instance, switch the school instance to the new token, then revoke the old `credential_id`.

### Lost consume response / credential recovery

If Central consumed an invitation but the HTTP response containing the one-time token was lost, do not consume the invitation again. An owner or admin can issue a replacement for the already-bound instance:

```http
POST /school-instance-credentials/rotate
```

Request (central operator session required):

```json
{"school_id":"school-uuid","instance_id":"stable-school-instance-id"}
```

Response (`200`):

```json
{
  "rotated":true,
  "credential_id":"replacement-credential-uuid",
  "school_id":"school-uuid",
  "instance_id":"stable-school-instance-id",
  "api_token":"REPLACEMENT_TOKEN_RETURNED_ONCE",
  "api_token_expires_at":"2027-11-24T12:00:00Z"
}
```

This revokes all currently active credentials for that school/instance before issuing the replacement. The raw replacement token is returned once and is stored only as a hash by Central. A school administrator should confirm the instance identity through the Central operator before a rotation is performed.

**Secure delivery and storage:** Central does not email or push the replacement token automatically. The Central owner/admin retrieves it over the authenticated HTTPS dashboard/API response and transfers it to the already-installed school through an administrator-approved secure channel (for example, the VPS secret manager or an encrypted server-to-server provisioning channel). It must be written only to the school server’s encrypted environment/secret store, never to browser local storage, source control, logs, chat, or a database field readable by school users. After saving it, the school instance uses it as `Authorization: Bearer ...` and the operator should close the recovery response and rotate again if the token may have been exposed.

## 6. Plans, subscriptions and term activation

### Plans

```http
GET    /plans
POST   /plans
PATCH  /plans
DELETE /plans/{id}
```

Create plan:

```json
{"name":"Finance & Fees","code":"finance_fees","amount_minor":95000,"currency":"NGN"}
```

Update amount:

```json
{"id":"plan-uuid","amount_minor":120000}
```

### Subscriptions

```http
GET   /subscriptions
POST  /subscriptions
PATCH /subscriptions
```

```json
{
  "school_id":"school-uuid",
  "plan_ids":["plan-uuid-1","plan-uuid-2"],
  "status":"active",
  "starts_on":"2026-09-01",
  "ends_on":"2027-07-31"
}
```

Billing is per term/season and a subscription may contain multiple plans.

### Term activation

```http
POST /term-activations
```

```json
{
  "subscription_id":"subscription-uuid",
  "term_name":"2026/2027 First Term",
  "starts_on":"2026-09-01",
  "ends_on":"2026-12-31"
}
```

The response includes the activation code, invoice ID, invoice number, school ID, term, and dates.

### School instance verifies a term activation code

```http
POST /term-activations/verify
```

Request:

```json
{
  "school_id":"school-uuid",
  "activation_code":"EDU-EXAMPLECODE",
  "instance_id":"stable-school-instance-id"
}
```

Success (`200`):

```json
{
  "valid":true,
  "activation_id":"activation-uuid",
  "school_id":"school-uuid",
  "subscription_id":"subscription-uuid",
  "term_name":"2026/2027 First Term",
  "starts_on":"2026-09-01",
  "ends_on":"2026-12-31",
  "features":["finance_fees","payroll","reports"],
  "instance_id":"stable-school-instance-id"
}
```

Errors:

| Status | Response |
| --- | --- |
| 400 | `{"error":"school_id, activation_code and instance_id are required"}` |
| 401 | `{"error":"invalid activation code"}` |
| 409 | `{"error":"activation has expired"}` |
| 409 | `{"error":"subscription is inactive"}` |
| 500 | `{"error":"features unavailable"}` |

The activation endpoint is currently a public code-verification endpoint protected by the activation code and school ID; it is not a general runtime subscription API. There are currently no expiry/refresh tokens. The school instance should verify the term code when installing or starting a term and cache the returned feature keys until the term expires.

```http
POST /term-activations/{id}/email
GET  /term-activations/{id}/code
```

## 7. Invoices and payments

### List invoices

```http
GET /invoices
```

Rows include `invoice_no`, `school_name`, `unit_count`, `unit_amount_minor`, `amount_minor`, `paid_minor`, `status`, and `due_on`.

### Create standalone invoice

```http
POST /invoices
```

```json
{
  "school_id":"school-uuid",
  "description":"2026/2027 First Term fees",
  "unit_count":200,
  "unit_amount_minor":95000,
  "currency":"NGN",
  "due_on":"2026-12-31",
  "amount_paid_minor":4000000,
  "payment_reference":"BANK-BATCH-01"
}
```

Total is `unit_count * unit_amount_minor`. The initial payment and reference are optional. If no reference is supplied, Central generates an internal reference.

```http
POST /invoices/issue
```

```json
{"id":"invoice-uuid"}
```

### Invoice transaction history

```http
GET /invoices/{id}/payments
```

Each item contains `amount_minor`, `student_number`, `reference`, `paid_on`, and `created_at`.

### Record a payment

```http
POST /payments
```

First payment for an incomplete subscription invoice:

```json
{
  "invoice_id":"invoice-uuid",
  "unit_count":200,
  "student_number":"200",
  "amount_minor":4000000,
  "reference":"BANK-BATCH-01",
  "paid_on":"2026-11-22"
}
```

The unit count is set once. Later payments reuse it and must not change it:

```json
{
  "invoice_id":"invoice-uuid",
  "unit_count":200,
  "student_number":"200",
  "amount_minor":5500000,
  "reference":"BANK-BATCH-02",
  "paid_on":"2026-12-01"
}
```

Payment references are allowed to repeat. The API rejects payments above the outstanding balance. Status is automatically `part_paid` until cumulative payments reach the calculated total, then becomes `paid`.

## 8. Support tickets

The school code is the school-facing identifier. Central support staff use the operator session cookie. A school portal should use its approved instance integration credentials; do not give it a central operator password.

The school instance can now use its bearer credential for its own tickets:

```http
POST /school/tickets
GET  /school/tickets
GET  /school/tickets/{id}
POST /school/tickets/{id}/replies
Authorization: Bearer INSTANCE_TOKEN
```

Create a ticket:

```json
{
  "subject":"Unable to generate student report",
  "description":"The end-of-term report fails to generate.",
  "category":"technical",
  "priority":"high",
  "requester_name":"School Admin",
  "requester_email":"admin@example.com"
}
```

The school is taken from the credential, so a school cannot create or read another school’s tickets. The school detail response includes public replies from Central support and replies sent by the school. Internal Central notes are never returned to the school. Reply body:

```json
{"body":"The report is working again. Please try once more."}
```

### Create by school code

```http
POST /tickets
```

```json
{
  "school_code":"DEMO-116044",
  "subject":"Unable to generate student report",
  "description":"The end-of-term report fails to generate.",
  "category":"technical",
  "priority":"high",
  "requester_name":"School Admin",
  "requester_email":"admin@example.com"
}
```

The central dashboard may send `school_id`; school integrations should send `school_code`.

```http
GET  /tickets
GET  /tickets/{id}
PATCH /tickets
POST /tickets/replies
```

Update:

```json
{"id":"ticket-uuid","status":"in_progress","assigned_to":"operator-uuid"}
```

Valid statuses: `open`, `assigned`, `in_progress`, `awaiting_customer`, `resolved`, `closed`.

Reply:

```json
{"ticket_id":"ticket-uuid","body":"We are investigating this now.","internal":false}
```

`internal: true` creates a central-only note.

## 9. Recommended school installation flow

1. Central creates the school and records its domain.
2. Central creates an invitation and sends `install_url` to the administrator.
3. The school instance verifies the signed token with `instance_id` and `instance_url`.
4. The instance performs local setup without sending local secrets to Central.
5. The instance consumes the invitation.
6. Store the school `code` and approved instance identity locally.
7. Use the school code for support-ticket correlation.

## 10. Email configuration

Resend is used for invitation, activation-code, and operator-credential email:

```env
RESEND_API_KEY=re_...
RESEND_FROM_EMAIL=EduCraft360 <noreply@example.com>
```

## 11. Availability, caching and expiry policy

Central is authoritative while reachable over HTTPS. The school instance should cache the last successful subscription/feature response and its `ends_on` date.

- Recommended offline grace period: **72 hours** after the last successful Central check.
- During that grace period, the school may continue using the cached feature set, while marking the Central connection as unavailable.
- After 72 hours without a successful check, the school should stop privileged subscription-backed actions and show an administrator reconnection message. Local data access and emergency read-only access are a school-instance decision.
- When a subscription reaches `ends_on`, or Central returns an explicit cancellation/revocation/no-active-plans response, the school must disable subscription-backed modules **immediately**. The 72-hour grace period applies only when Central cannot be reached and the school is operating on its last known cached response; it does not apply to a successful Central response that says access has ended.
- Revoking a credential takes effect on the next Central request; cached access can continue only within the 72-hour offline grace period.
- A plan change is reflected the next time `/school/subscription` succeeds. The school should replace its cached feature set rather than merge old and new keys.
- A term activation is valid through its `ends_on` date. If its subscription is cancelled or revoked, the next entitlement check should remove its module access; the school must not treat an old activation code as a permanent licence.

These rules are the integration contract for the school project. They should be covered by outage and expiry tests.

## 12. Local development

```bash
cd backend
set -a
source ../.env
set +a
go run ./cmd/api migrate
go run ./cmd/api
```

The frontend normally proxies `/api/*` to the API. Use that proxy from browser code rather than hard-coding the API origin.

## 13. Integration test checklist

Run this against a disposable database and a test school:

1. Create a school and confirm its ID/code are unique.
2. Create an invitation and verify its `install_url`, expiry, and one-time status.
3. Verify it from the intended `instance_id`; confirm the response contains `school_id`, `school_code`, and `expires_at`.
4. Verify it from a different instance; expect `409 invitation is bound to another instance`.
5. Consume it; save the returned credential immediately and call `/school/subscription`.
6. Repeat consume; expect `409 invitation is unavailable` and no second credential.
7. Simulate a lost consume response; use the owner/admin rotate endpoint and confirm the old token returns `401` while the replacement works.
8. Create a subscription with multiple plans; confirm all expected module keys, including `cbt_exams`, are returned.
9. Verify a term activation; confirm school, term dates, and features.
10. Change or cancel the subscription; confirm the next entitlement response reflects the change and activation verification rejects inactive subscriptions.
11. Create, list, read, and reply to a school ticket; confirm another school credential receives no access.
12. Revoke a credential; confirm subsequent calls return `401`.
13. Advance/override expiry dates and test invitation expiry, credential expiry, term expiry, and the 72-hour offline policy in the school instance.

