# AnyClaw Registry Backup And Restore

This document is the Round 10 backup baseline for the cloud marketplace registry.

## What Must Be Backed Up

The first production registry stores state under the registry data directory:

```text
/data/
  registry.db
  packages/
  audit/
```

- `registry.db`: artifact metadata, versions, publisher tokens, quarantine, downloads, audit.
- `packages/`: generated artifact zip packages used by `/v1/download/...`.
- `audit/`: reserved local audit folder.

Back up the database and packages together. A database-only backup can restore the catalog while downloads fail.

## Windows / Local Backup

```powershell
.\scripts\registry-backup.ps1 `
  -DataDir tmp/registry-data `
  -OutputDir backups/registry
```

Restore to a new directory:

```powershell
.\scripts\registry-restore.ps1 `
  -BackupDir backups/registry/20260507-120000 `
  -TargetDataDir tmp/restore-data `
  -Force
```

## Linux Server Backup

```bash
DATA_DIR=/data OUTPUT_DIR=/backups/registry ./deploy/registry-backup.sh
```

Restore:

```bash
BACKUP_DIR=/backups/registry/20260507-120000 \
TARGET_DATA_DIR=/data \
FORCE=true \
./deploy/registry-restore.sh
```

## Online Backup Note

When `sqlite3` is available, the scripts use SQLite `.backup`. This is safer than copying a hot SQLite file.

If `sqlite3` is unavailable, the scripts fall back to a file copy. For production, prefer one of:

- Install `sqlite3` on the server.
- Stop the registry briefly, run the backup, then start it again.

## Restore Smoke

After restore, start registry against the restored data directory and verify:

```bash
curl http://127.0.0.1:8791/v1/health
curl http://127.0.0.1:8791/v1/artifacts
curl -X POST http://127.0.0.1:8791/v1/artifacts/<artifact-id>/resolve \
  -H 'Content-Type: application/json' \
  -d '{}'
curl -L <download_url> -o artifact.zip
sha256sum artifact.zip
```

The downloaded file hash must match `checksum_sha256` from resolve.

## Retention

For a 40GB system disk, start conservatively:

- Keep 7 daily backups.
- Keep 4 weekly backups.
- Move older backups off the system disk before traffic grows.

Do not store backups only inside the same Docker volume as the registry data.
