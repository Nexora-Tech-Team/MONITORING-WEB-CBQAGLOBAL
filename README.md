# CBQA Monitoring Web

Stack:
- Frontend: React + Vite
- Backend: Go
- Database: PostgreSQL
- Docker: `docker-compose.yml` tersedia untuk Postgres

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
