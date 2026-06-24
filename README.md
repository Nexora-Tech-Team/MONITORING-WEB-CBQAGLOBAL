# CBQA Monitoring Web

Stack:
- Frontend: React + Vite
- Backend: Go
- Database: PostgreSQL
- Docker: `docker-compose.yml` tersedia untuk Postgres
- Production deploy: `docker-compose.prod.yml` + GitHub Actions

## Local run

1. Start PostgreSQL.
   - Docker compose is configured for host port `5439`.
   - In this environment, the app also falls back to a local PostgreSQL instance on `5432` if `5439` is not available.
2. Start backend:

```bash
cd backend
go run .
```

3. Start frontend:

```bash
cd frontend
npm run dev
```

## Ports

- Frontend: `http://127.0.0.1:4173`
- Backend: `http://127.0.0.1:8090`

## Data source

The backend seeds task data from:
- `Monitoring Pekerjaan.xlsx`

Task status is stored in PostgreSQL, so changes persist without login.

## Production deploy

Target VPS:
- `72.61.209.201`
- app path: `/root/nexora-node/apps/monitoring-web-cbqaglobal`
- public domain: `https://monitoring-web.cbqaglobal.co.id`

Pipeline uses:
- `docker compose -f docker-compose.prod.yml up -d --build`
- Traefik labels for `/api` and `/`

Required GitHub secret:
- `VPS_SSH_KEY` = private SSH key that can log in to the VPS as `root`

Optional VPS env file:
- copy `.env.deploy.example` to `.env` inside `/root/nexora-node/apps/monitoring-web-cbqaglobal`

Traefik defaults:
- network: `web`
- entrypoint: `websecure`

If your Traefik network or entrypoint names are different, update:
- `TRAEFIK_NETWORK`
- `TRAEFIK_ENTRYPOINT`
