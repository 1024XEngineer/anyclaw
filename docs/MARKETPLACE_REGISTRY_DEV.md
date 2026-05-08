# AnyClaw Registry Development And Deployment

Round 2 adds a local cloud registry service for the marketplace read path and package downloads.

Run locally:

```powershell
go run ./cmd/anyclaw-registry serve --addr :8791 --data-dir .anyclaw-registry --seed=true
```

The registry requires an admin token by default so admin routes cannot be
accidentally exposed. For local read-only catalog smoke tests, either pass an
admin token as shown below or explicitly add `--require-admin-token=false`.

Development storage:

```text
.anyclaw-registry/
  registry.db
  packages/
  audit/
```

Smoke checks:

```powershell
Invoke-RestMethod http://localhost:8791/v1/artifacts
Invoke-RestMethod http://localhost:8791/v1/artifacts/cloud.skill.release-notes
Invoke-RestMethod -Method Post http://localhost:8791/v1/artifacts/cloud.skill.release-notes/resolve -Body '{}' -ContentType 'application/json'
```

Round 11 hardens the service with admin routes, publisher tokens, quarantine, audit, download stats, configurable `database/sql` storage, and a storage adapter seam.

Admin token:

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN="change-me"
go run ./cmd/anyclaw-registry serve --addr :8791 --data-dir .anyclaw-registry --admin-token $env:ANYCLAW_REGISTRY_ADMIN_TOKEN
```

Create a publisher token:

```powershell
Invoke-RestMethod -Method Post http://localhost:8791/v1/admin/tokens `
  -Headers @{ Authorization = "Bearer $env:ANYCLAW_REGISTRY_ADMIN_TOKEN" } `
  -Body '{"publisher_id":"AnyClaw Labs"}' `
  -ContentType 'application/json'
```

Scripted publisher token creation:

```powershell
.\scripts\registry-create-publisher-token.ps1 `
  -BaseUrl http://localhost:8791 `
  -AdminToken $env:ANYCLAW_REGISTRY_ADMIN_TOKEN `
  -PublisherId "AnyClaw Labs"
```

Scripted publish:

```powershell
.\scripts\registry-publish-artifact.ps1 `
  -BaseUrl http://localhost:8791 `
  -PublisherToken $env:ANYCLAW_PUBLISHER_TOKEN `
  -Manifest examples/marketplace/skill-release-notes/anyclaw.artifact.json
```

See `docs/PUBLISHING.md` for the full publishing workflow and current package-generation limits.

Admin routes:

```text
POST /v1/admin/tokens
POST /v1/publish
POST /v1/artifacts/{id}/quarantine
POST /v1/artifacts/{id}/unquarantine
GET  /v1/admin/audit
GET  /v1/admin/downloads
```

Quarantined artifacts return `410 Gone` from resolve and download routes. Local AnyClaw treats that as a failed install path, and policy still blocks any resolved package marked `trust_level=quarantined`.

Database configuration:

```powershell
go run ./cmd/anyclaw-registry serve --db-driver sqlite --db-dsn .anyclaw-registry/registry.db
```

The registry store is built on `database/sql`. SQLite is included for development. A production Postgres build must include a maintained Postgres driver in the deployment build and pass its driver name and DSN:

```powershell
anyclaw-registry serve --db-driver postgres --db-dsn "postgres://user:pass@host:5432/anyclaw_registry?sslmode=require"
```

Object storage:

The server now uses a package storage adapter interface. The current built-in adapter stores packages under `packages/`; S3, R2, OSS, or COS adapters can implement the same interface without changing registry HTTP contracts. In production, place the package directory on durable storage or replace the adapter in a deployment-specific build.

TLS deployment:

```text
Caddy/Nginx TLS
  -> anyclaw-registry --addr 127.0.0.1:8791
Postgres or managed SQL
Durable package storage
ANYCLAW_REGISTRY_ADMIN_TOKEN from a secret manager
```

Recommended Caddy sketch:

```text
registry.example.com {
  reverse_proxy 127.0.0.1:8791
}
```

Docker-style local build:

```powershell
docker build -t anyclaw-registry-dev .
docker run --rm -p 8791:8791 anyclaw-registry-dev anyclaw-registry serve --addr :8791 --data-dir /data --seed=true
```
