# AnyClaw IP-Only Server Deployment

This is the Round 11 runbook for deploying AnyClaw website and cloud marketplace on a server IP before a domain is ready.

## Target Server

- Cloud: Alibaba Cloud ECS
- Region: Hong Kong
- OS: Ubuntu 22.04 64-bit
- Docker: preinstalled
- First public entry: `http://SERVER_IP`

## Security Group

Open:

- `22/tcp`: SSH, preferably restricted to the owner's IP.
- `80/tcp`: AnyClaw website and `/v1/*` registry proxy.

Closed for first launch:

- `8791/tcp`: registry internal port.
- `18789/tcp`: AnyClaw Gateway. Compose binds it to `127.0.0.1` only.
- `443/tcp`: open later in Round 12 when HTTPS is added.

## Server Directory

Use:

```bash
sudo mkdir -p /opt/anyclaw
sudo chown -R "$USER":"$USER" /opt/anyclaw
cd /opt/anyclaw
```

Required files/directories:

```text
/opt/anyclaw/
  .env.production
  docker-compose.prod.yml
  Dockerfile
  go.mod
  go.sum
  cmd/
  pkg/
  scripts/
  deploy/
  site/dist/
  ui/
  package.json
  pnpm-lock.yaml
  pnpm-workspace.yaml
```

The first deployment can copy the whole repository except local secrets and build caches.

## Environment

Create `.env.production` from `.env.production.example`.

Minimum required values:

```bash
ANYCLAW_PUBLIC_IP=<server-ip>
ANYCLAW_PUBLIC_SCHEME=http
ANYCLAW_REGISTRY_ADMIN_TOKEN=<long-random-token>
ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN=true
ANYCLAW_MARKETPLACE_ENDPOINT=http://registry:8791
```

Generate token:

```bash
openssl rand -hex 32
```

Do not commit `.env.production`.

## Build Website Locally Before Upload

From the repository root:

```powershell
corepack pnpm --dir site build
```

Upload `site/dist/` with the server files.

## Start On Server

```bash
cd /opt/anyclaw
docker compose --env-file .env.production -f docker-compose.prod.yml config
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

## Smoke

On server:

```bash
curl http://127.0.0.1/
curl http://127.0.0.1/v1/health
curl http://127.0.0.1/v1/artifacts
curl -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" http://127.0.0.1/v1/admin/audit
```

From local machine:

```bash
curl http://SERVER_IP/
curl http://SERVER_IP/v1/health
curl http://SERVER_IP/v1/artifacts
```

Scripted smoke:

```bash
SERVER_ORIGIN=http://SERVER_IP ./deploy/server-ip-smoke.sh
```

## Expected Result

- `http://SERVER_IP` loads the React website.
- `http://SERVER_IP/marketplace` loads the Marketplace page.
- `http://SERVER_IP/v1/artifacts` returns registry artifacts.
- Admin API without token returns `401`.
- `docker compose ps` shows registry, website, and anyclaw running or healthy.

## Rollback

```bash
cd /opt/anyclaw
docker compose --env-file .env.production -f docker-compose.prod.yml down
```

Registry data remains in the Docker volume `registry-data`. Back it up before deleting volumes.
