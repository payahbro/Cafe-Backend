# Golang Backend Service

Production-ready backend scaffold aligned with the architecture in `docs/architecture/final-architecture-tech-stack.md`.

## Included

- Go API entrypoint in `cmd/api`
- Worker entrypoint scaffold in `cmd/worker`
- Structured logger (`zap`)
- Env-based config loader (`caarlos0/env`)
- PostgreSQL pool bootstrap (`pgx/v5`)
- Redis client bootstrap (`go-redis/v9`)
- Health endpoints:
  - `GET /health`
  - `GET /api/v1/health`
- Docker + docker-compose local setup (`app`, `redis`, external Supabase pooler reference)

## Quick Start

1. Copy env:

```bash
cp .env.example .env
```

2. Set your Supabase DB password in `.env` (`SUPABASE_DB_PASSWORD`).
   Never commit `.env` to version control.

3. Run API locally:

```bash
go run ./cmd/api
```

4. Check health:

```bash
curl http://localhost:8080/health
```

## Run with Docker Compose

```bash
docker compose up --build
```

## Notes

- Supabase is external (session pooler), not run as a local Postgres container.
- Set `DB_REQUIRED=true` and `REDIS_REQUIRED=true` to make startup fail-fast when dependencies are unavailable.
- Secrets are read from local `.env`; tracked files (`.env.example`, `docker-compose.yml`) only contain placeholders.

