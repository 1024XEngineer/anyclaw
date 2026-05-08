# AnyClaw Production Deploy Notes

This directory stores deployment assets for the website and cloud marketplace rollout.

Round 2 creates the first production Compose shape:

- `registry`: runs `anyclaw-registry` on internal port `8791` with persistent `/data`.
- `website`: serves the built React static site from `site/dist` on port `80` using a small Python standard-library SPA server.
- `anyclaw`: runs the existing Gateway on `127.0.0.1:18789`.

The `website` service is intentionally minimal: it serves static files and proxies `/v1/*` to `registry:8791` using Python's standard library. The site itself is built by the React/Vite workspace under `site/`. Unknown paths such as `/marketplace` fall back to `index.html`, so browser refresh works with React Router.

## Local Config Validation

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml config
```

## First Server Layout

Recommended server path:

```text
/opt/anyclaw/
  .env.production
  docker-compose.prod.yml
  deploy/static-spa-server.py
  site/dist/
  workflows/
```

Registry data lives in the named Docker volume `registry-data` unless a later round changes this to a bind mount.

## Smoke Checks

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl http://127.0.0.1/
curl http://127.0.0.1/v1/health
curl http://127.0.0.1/v1/artifacts
docker compose --env-file .env.production -f docker-compose.prod.yml exec registry curl -f http://localhost:8791/v1/health
```

For IP-only server deployment, see `docs/SERVER_IP_DEPLOYMENT.md` and `deploy/server-ip-smoke.sh`.

## Registry Backup

Back up the registry data directory and packages together:

```bash
DATA_DIR=/data OUTPUT_DIR=/backups/registry ./deploy/registry-backup.sh
```

See `docs/REGISTRY_BACKUP_RESTORE.md` before restoring production data.

## Security Notes

- `ANYCLAW_REGISTRY_ADMIN_TOKEN` is required by `docker-compose.prod.yml`.
- Do not expose `18789` publicly unless `ANYCLAW_API_TOKEN` is set.
- Port `8791` is not published by the production compose file. Use `http://SERVER_IP/v1/*` through the website proxy.
