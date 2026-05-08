# AnyClaw Registry Production Runbook

This runbook belongs to Round 3 of the website + cloud marketplace deployment plan. It explains how to start and verify `anyclaw-registry` in production-like environments.

## 1. Required Environment

Required:

```bash
ANYCLAW_REGISTRY_ADMIN_TOKEN=<long-random-token>
ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN=true
```

Recommended first pass:

```bash
ANYCLAW_REGISTRY_ADDR=:8791
ANYCLAW_REGISTRY_DATA_DIR=/data
ANYCLAW_REGISTRY_DB_DRIVER=sqlite
ANYCLAW_REGISTRY_DB_DSN=/data/registry.db
ANYCLAW_REGISTRY_SEED=true
```

Generate token:

```bash
openssl rand -hex 32
```

Do not run a public registry without `ANYCLAW_REGISTRY_ADMIN_TOKEN`.

## 2. Docker Compose Start

From the deployment directory:

```bash
cp .env.production.example .env.production
```

Fill at least:

```bash
ANYCLAW_REGISTRY_ADMIN_TOKEN=<long-random-token>
```

Validate:

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml config
```

Start:

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml up -d registry
```

Check:

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml ps
docker compose --env-file .env.production -f docker-compose.prod.yml logs --tail=100 registry
```

## 3. Smoke Checks

Public read path:

```bash
curl http://127.0.0.1:8791/v1/health
curl http://127.0.0.1:8791/v1/artifacts
curl http://127.0.0.1:8791/v1/artifacts/cloud.skill.release-notes
```

Resolve:

```bash
curl -X POST http://127.0.0.1:8791/v1/artifacts/cloud.skill.release-notes/resolve \
  -H 'Content-Type: application/json' \
  -d '{}'
```

Admin:

```bash
curl -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" \
  http://127.0.0.1:8791/v1/admin/audit
```

PowerShell helper:

```powershell
.\deploy\registry-smoke.ps1 -BaseUrl http://127.0.0.1:8791 -AdminToken $env:ANYCLAW_REGISTRY_ADMIN_TOKEN
```

## 4. Publisher Token

Create publisher token:

```bash
curl -X POST http://127.0.0.1:8791/v1/admin/tokens \
  -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"publisher_id":"AnyClaw Labs"}'
```

The returned publisher token is shown only once. Store it outside git.

Revoke publisher token:

```bash
curl -X POST http://127.0.0.1:8791/v1/admin/tokens/<token-id>/revoke \
  -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}'
```

## 5. Data

The first deployment uses SQLite and local package storage:

```text
/data/registry.db
/data/packages/
/data/audit/
```

Backup:

```bash
DATA_DIR=/data OUTPUT_DIR=/backups/registry ./deploy/registry-backup.sh
```

Restore:

```bash
BACKUP_DIR=/backups/registry/<backup-name> TARGET_DATA_DIR=/data FORCE=true ./deploy/registry-restore.sh
```

See `docs/REGISTRY_BACKUP_RESTORE.md` for local PowerShell scripts, restore dry run, and retention guidance.

Do not delete the `registry-data` Docker volume unless a restore has been verified.

## 6. Current Production Limits

- SQLite is acceptable for first launch and low traffic.
- Package storage is local volume storage.
- Postgres and OSS/S3-style object storage require confirmed dependency and adapter work in later rounds.
- IP-only HTTP is acceptable for smoke testing; HTTPS requires Round 12.
