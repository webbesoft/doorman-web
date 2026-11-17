[![Publish Docker Image](https://github.com/webbesoft/doorman-web/actions/workflows/publish-docker.yml/badge.svg)](https://github.com/webbesoft/doorman-web/actions/workflows/publish-docker.yml)
[![Go](https://github.com/webbesoft/doorman-web/actions/workflows/go.yml/badge.svg)](https://github.com/webbesoft/doorman-web/actions/workflows/go.yml)

# Doorman Analytics

Doorman is a small, self-hosted website analytics application. It runs as one Go process, serves its dashboard and tracker from embedded assets, and supports SQLite, PostgreSQL, and MySQL. Start with one application instance and a persistent database directory.

## Hardening epochs

1. **Foundation:** validated configuration, pinned Go tools, revocable sessions, explicit migrations, persistent non-root container storage, and optional geolocation.
2. **Analytics correctness:** navigation IDs, atomic and idempotent heartbeats, legacy import, seven-day reports, and browser lifecycle regression tests.
3. **Public operation:** request and concurrency limits, a database-backed capacity ceiling, proxy trust configuration, CSRF protection, security headers, health checks, and metrics.
4. **Delivery and recovery:** verified SQLite backups, backend contract tests, container replacement tests, dependency gates, and CI checks required before publication.

These changes establish an application baseline. Deployment still requires HTTPS termination, off-host backups, alerts, and capacity testing against the intended workload. No infrastructure is provisioned by this repository.

## Local development

Install Go 1.27.4, Node.js 24 or later, and Python 3. The template generator and vulnerability scanner are pinned in `go.mod`.

```sh
cp .env.example .env
# Edit .env: set a 12–72 byte admin password and a session secret.
# Generate a secret with: openssl rand -hex 32
make build
APP_ENV=local ./tmp/main
```

Local mode loads `.env` before reading database configuration and applies pending migrations. Already exported environment variables take precedence. Production mode uses environment variables and requires an explicit migration step.

The dashboard is at `http://localhost:8080`. There are no default credentials. The admin username comes from `DOORMAN_ADMIN_USER`; its password is created only when that account does not exist. The bootstrap password can be removed from the runtime environment after creation. Existing passwords shorter than 12 bytes cannot log in and must be reset before upgrading.

```sh
make check           # generation, Go race tests, tracker/security-gate tests, vet, build
make security        # requires access to the official Go vulnerability database
make container-check # Docker image, migrations, persistent replacement, backup, logout
make backend-check   # disposable PostgreSQL/MySQL Docker containers
```

`make develop` additionally requires Air. Edit `assets/css/doorman.css` for styling; no CSS build step is needed.

## Configuration

| Variable | Meaning |
| --- | --- |
| `APP_ENV` | `production` by default; `local` loads `.env` and enables local HTTP cookies. |
| `DOORMAN_ADMIN_USER` | Required bootstrap/account name, at most 128 bytes. |
| `DOORMAN_ADMIN_PASSWORD` | Required for first account creation or an explicit reset; 12–72 bytes. It does not silently change existing passwords. |
| `DOORMAN_SESSION_SECRET` | Required secret of at least 32 bytes. Also keys visitor pseudonyms. Keep it stable across restarts. |
| `DOORMAN_ALLOWED_HOSTS` | Required in production. Comma-separated website hostnames, such as `example.com,www.example.com`, without schemes or ports. |
| `DOORMAN_TRUSTED_PROXIES` | Optional comma-separated proxy CIDRs. Forwarding headers are ignored unless the direct peer is explicitly trusted. |
| `PORT` | HTTP port, default `8080`. Terminate HTTPS at a reverse proxy. |
| `DB_PROVIDER` | `sqlite` by default, or `postgres` / `mysql`. Unknown values fail startup. |
| `DB_PATH` | SQLite filesystem path; the image defaults to `/data/analytics.db`. Mount the entire directory. |
| `DATABASE_URL` | PostgreSQL URL or MySQL driver DSN; takes precedence over individual connection settings. |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | Individual remote database settings. |
| `DB_SSLMODE` | PostgreSQL mode, default `verify-full`. Production requires certificate and hostname verification. |
| `DOORMAN_RETENTION_DAYS` | Retained UTC calendar days, default `90`, range `1–3650`. |
| `DOORMAN_MAX_PAGE_VIEWS` | Hard ceiling on retained page views, default `1000000`. Existing heartbeats continue at capacity; new visits return 503 until capacity is available. |
| `DOORMAN_METRICS_TOKEN` | Optional separate 32+ byte bearer token for `/metrics` scrapers. Dashboard sessions also have access. |
| `DOORMAN_GEO_API_KEY` | Optional commercial ip-api.com key. Without a key, geolocation is disabled and countries are unknown. Requests use HTTPS. |

PostgreSQL example: `postgres://user:password@db.example.com:5432/doorman?sslmode=verify-full`.
MySQL example: `user:password@tcp(db.example.com:3306)/doorman?parseTime=true&tls=true`.
Supply credentials through your deployment's secret mechanism. Local test databases may disable TLS in local/test mode. Never use the fixture credentials from CI for a deployment.

## Container deployment

Build locally or select a tested version from `ghcr.io/webbesoft/doorman-web`. Use an immutable release tag or image digest in deployment automation. The following commands use a local image:

```sh
docker build -t doorman:local .
docker volume create doorman_data
# Prepare an environment file with real values, including allowed website hosts.
# APP_ENV and DB_PATH are explicitly overridden below for the container.
docker run --rm --env-file .env -e APP_ENV=production -e DB_PATH=/data/analytics.db \
  -v doorman_data:/data doorman:local -migrate
docker run -d --name doorman --restart unless-stopped \
  --env-file .env -e APP_ENV=production -e DB_PATH=/data/analytics.db \
  --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 -v doorman_data:/data doorman:local
```

The image runs as a non-root user. A new named volume inherits the image's directory ownership. Existing bind mounts must be writable by that user. SQLite uses WAL mode, foreign keys, and a bounded connection pool. Keep its database, WAL, and shared-memory files on the same local persistent filesystem; do not share the directory across hosts or mount a single database file.

Configure your proxy for HTTPS and route to port 8080. Add only its actual source CIDRs to `DOORMAN_TRUSTED_PROXIES`; do not expose the application port publicly to bypass the proxy. `/readyz` checks database connectivity and `/livez` checks the HTTP process. Both return 200 when healthy.

## Tracking and metric definitions

Add the tracker to each configured website:

```html
<script src="https://analytics.example.com/assets/js/t.js" async></script>
```

Allow the analytics origin in your website's script and connection CSP directives. The tracker respects Do Not Track and Global Privacy Control. It sends an initial event immediately, visible-page heartbeats, and a final snapshot on page departure. Delivery is best effort; blocked scripts and unsuccessful unload delivery can undercount visits.

Each navigation has a random in-memory ID. Reloads and SPA path changes create new visits. Heartbeats carry increasing sequence numbers; duplicate and older events do not add visits or reduce cumulative measurements. Pages restored from the browser cache start a new visit; time spent on another page is not attributed to the restored page. Visits open for more than 24 hours start a new measurement period.

- **Page visits:** separate accepted navigations, excluding basic user-agent bot matches.
- **Daily visitors:** estimated unique IP-based pseudonyms per website and UTC day, summed over the reporting period. Shared IPs can undercount people and changing IPs can overcount them.
- **Dwell:** elapsed time since the navigation, including hidden time.
- **Active time:** visible time until 30 seconds after the last activity signal.
- **Scroll:** maximum observed percentage of page depth.
- **Reports:** today and the previous six UTC calendar days, with missing days shown as zero. Sources and countries count visits over this same window.

Collection accepts only configured website hosts in production. Origin checks, rate limits, and bot heuristics reduce abuse; public browser analytics cannot authenticate the truth of a reported event. Per-client limits are 120 event requests/minute with a burst of 60. Login permits five attempts/minute with a burst of five. Concurrency and global limits bound work. Initial deployments should use one application instance because request limiters are in process.

## Privacy and existing data

New events discard URL query strings and fragments, retain only referrer hostnames, and do not persist raw user agents or raw IP addresses. URL paths can still contain personal information; site owners must avoid tracking sensitive paths or identifiers. Visitor keys use a secret HMAC scoped to a website and UTC day. No visitor cookie or persistent browser storage is used. Dashboard authentication uses an HttpOnly, Secure, SameSite=Strict cookie in production.

Hashing and cookie-free collection do not by themselves establish legal compliance or anonymity. Assess the actual processing, notices, retention, and applicable requirements. Optional geolocation discloses the raw IP to the configured commercial provider. Routine application logs contain route names, status, timing, and sanitized failure classes, rather than visitor identifiers or full request URLs. Configure reverse-proxy/platform logs and their retention separately.

The migration copies the existing stored visit rows into the new model and redacts copied URLs. Original tables remain available until their retention cleanup removes expired rows. Previously overwritten visits cannot be reconstructed. Imported visitor estimates are isolated from new pseudonyms, so transition-day estimates may differ from the old dashboard. Refresh cached copies of the tracking script when deploying this protocol change; events without a navigation ID and sequence are rejected.

## Migrations and recovery

Before an upgrade, create and verify a backup, stop the previous application instance, run the new binary's `-migrate` command against the same database, reset the configured admin password with `-reset-admin-password` when replacing an existing weak bootstrap password, then start the new release and check readiness. Run one migration process at a time. Migrations are additive and versioned; MySQL DDL is not transactional, so keep the pre-upgrade backup and use a maintenance window. To roll back a schema-changing release, restore its pre-upgrade database backup with the matching previous image.

SQLite backup commands refuse to overwrite an existing file:

```sh
docker exec doorman /app/doorman -backup /data/backups/analytics-YYYY-MM-DD.db
docker exec doorman /app/doorman -verify-backup /data/backups/analytics-YYYY-MM-DD.db
```

Backups use SQLite's consistent snapshot mechanism and pass an integrity check before success. Schedule them outside the application, copy them encrypted to another host/storage system, and monitor backup age. Keeping a backup only in `/data` does not protect against host or volume loss. Rotate backup copies separately from raw-event retention. PostgreSQL and MySQL require their native backup tooling.

For a restore, stop the app, preserve the current data directory, restore the verified backup into a fresh directory with the correct ownership, and start the image that matches that schema. Do not combine a restored database file with WAL/shared-memory files from a different database. Verify readiness, expected reports, and a new tracked visit. Reset the admin password to revoke sessions from the restored snapshot before reopening access:

```sh
# Supply the new password through the existing secret/environment mechanism.
# Run this with the app's configuration and database directory mounted.
doorman -reset-admin-password
```

Password resets revoke all sessions for that account. Logout revokes the current session immediately. Sessions expire after 30 minutes idle or 12 hours absolutely. Rotating `DOORMAN_SESSION_SECRET` invalidates existing sessions and changes visitor pseudonyms.

## Monitoring and release checks

`/metrics` accepts a dashboard session or `Authorization: Bearer <DOORMAN_METRICS_TOKEN>` when configured. It exposes retained/capacity gauges, event outcomes, request duration totals, in-flight work, cleanup failures, and the last successful cleanup timestamp. Configure external checks for readiness, unexpected event failures, cleanup failures/staleness, storage growth/free space, and backup age. Establish latency, throughput, recovery-time, and acceptable-data-loss targets for your workload, then verify them with deployment-specific load and restore drills.

CI runs clean generation, Go race tests, tracker tests, PostgreSQL/MySQL contracts, static checks, dependency auditing, and an image replacement/recovery smoke test. Tag publication depends on the same checks. GitHub Actions are pinned to commit hashes resolved from official release tags. The dependency gate rejects new symbol-level findings; the two currently inapplicable Echo advisories have exact-version decisions in `security/vulnerability-exceptions.json`. Reassess those decisions when changing Echo, proxy handling, or request limiting.

## License

[MIT](LICENSE.md).
