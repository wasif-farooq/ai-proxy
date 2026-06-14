# AI Proxy

A high-performance, multi-tenant API gateway for OpenAI-compatible LLM providers. Routes requests to upstream AI providers (OpenAI, Anthropic, Google, Ollama, DeepSeek, etc.) with per-client authentication, rate limiting, nonce-based replay protection, and per-client API key management.

## Features

- **Multi-provider routing** — routes `model` → `provider` based on model prefix or per-client preferred routes
- **Per-client API keys** — override the global provider key with client-specific credentials (encrypted at rest)
- **Streaming support** — pass-through SSE streaming with pooled buffers
- **Nonce + timestamp replay protection** — 16-shard in-memory nonce store (SHA-256 distributed)
- **Rate limiting** — 16-shard token bucket per client
- **Admin UI** — manage clients, providers, audit logs via web dashboard or REST API
- **Audit logging** — asynchronous batch-write audit events with retry
- **HTTP/2 upstream** — multiplexed connections to AI providers
- **Provider key cache** — 5-minute in-memory TTL caches decrypted per-client keys, skipping DB + AES-GCM on repeat requests
- **Graceful shutdown** — ordered cleanup of all services

---

## Architecture

```
┌─────────────┐     ┌─────────────────────────────────────────────────┐
│   Client    │     │              AI Proxy API Server                │
│  (app/curl) │────▶│  POST /api/v1/chat/completions                  │
└─────────────┘     │                                                 │
                    │  Middleware chain:                               │
                    │    AuthMiddleware  →  X-Client-ID + Bearer       │
                    │    NonceMiddleware →  X-Nonce + X-Timestamp      │
                    │    RateLimitMiddleware →  token bucket           │
                    │    RouteMiddleware  →  model → provider routing  │
                    │         │                                        │
                    │         ▼                                        │
                    │    Proxy.Forward / ForwardStreaming              │
                    │         │                                        │
                    │         ▼                                        │
                    │    Upstream AI Provider (OpenAI, Anthropic, etc) │
                    └─────────────────────────────────────────────────┘

┌─────────────┐     ┌─────────────────────────────────────────────────┐
│   Admin     │     │              AI Proxy Admin Server              │
│  (browser)  │────▶│  /api/v1/admin/*                                │
└─────────────┘     │  - Clients CRUD                                 │
                    │  - Providers CRUD                                │
                    │  - Dashboard stats                               │
                    │  - Audit logs                                    │
                    │  - WebSocket real-time updates                   │
                    └─────────────────────────────────────────────────┘

All servers share:
┌─────────────────────────────────────────────────────────────────────┐
│                        PostgreSQL                                    │
│  clients | providers | client_provider_keys | admin_users |          │
│  audit_logs | refresh_tokens                                         │
└─────────────────────────────────────────────────────────────────────┘
```

### Two Server Binaries

| Binary | Port | Purpose |
|--------|------|---------|
| `cmd/api` | `8080` | Proxy endpoint for AI provider requests + admin API |
| `cmd/admin` | `8081` | Admin web UI + management API + WebSocket |

---

## Prerequisites

- **Go 1.25+**
- **Docker + Docker Compose** (for the dev stack)
- **Node.js 20+** (for frontend development)
- **PostgreSQL 16** (or use the Docker Compose PostgreSQL)

---

## Quick Start

### 1. Clone and configure

```bash
git clone <repo-url> ai-proxy
cd ai-proxy
cp .env.example .env
```

Edit `.env` with your settings. At minimum, set:

```env
ENV=development
JWT_SECRET=your-secure-random-secret
ENCRYPTION_KEY=your-32-byte-encryption-key
DATABASE_URL=postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable
```

### 2. Start the development stack

> **Note:** If you have a local PostgreSQL on port 5432, use `POSTGRES_PORT=15432` to avoid conflicts.

```bash
# Start PostgreSQL + API server (hot-reload) + Admin server (hot-reload)
docker compose up -d

# Or with custom ports (if ports 5432/8080/8081 are in use):
POSTGRES_PORT=15432 API_PORT=18080 ADMIN_PORT=18081 docker compose up -d
```

### 3. Run database migrations and seed

```bash
# Migrations run automatically on container startup
# Seed the admin user:
make seed

# Seed predefined AI providers:
make seed-providers

# Or do both at once:
make seed-all
```

### 4. Verify

```bash
# Health checks
curl http://localhost:8080/health
curl http://localhost:8081/health

# Run the test script
./scripts/test-api.sh
```

### 5. Send a test proxy request

```bash
# Generate nonce and timestamp for replay protection
NONCE=$(date +%s | sha256sum | head -c 16)
TIMESTAMP=$(date +%s)

curl -X POST http://localhost:8080/api/v1/chat/completions \
  -H "X-Client-ID: <your-client-id>" \
  -H "Authorization: Bearer <your-client-secret>" \
  -H "X-Nonce: $NONCE" \
  -H "X-Timestamp: $TIMESTAMP" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"stream":false}'
```

### 6. Login to the admin UI

Open `http://localhost:8081` and log in with the admin credentials you set during seeding.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `development` | `development`, `staging`, `production` |
| `SERVER_HOST` | `0.0.0.0` | Bind address |
| `SERVER_PORT` | `8080` | API server port |
| `ADMIN_PORT` | `8081` | Admin server port (set via env, not config; no default in Go config — must be set in .env or Docker) |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable` | PostgreSQL connection string |
| `DATABASE_MAX_CONNS` | `25` | Max pool connections |
| `JWT_SECRET` | `change-me-in-production` | JWT signing key (generate: `openssl rand -hex 32`) |
| `JWT_EXPIRATION` | `1h` | Admin JWT token lifetime |
| `ENCRYPTION_KEY` | `""` | Master key for per-client provider key encryption |
| `RATE_LIMIT_REQUESTS_PER_MIN` | `60` | Max requests per minute per client |
| `RATE_LIMIT_BURST` | `10` | Burst allowance |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | `text` or `json` |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | CORS allowed origins (comma-separated) |

Port overrides for Docker Compose:

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_PORT` | `5432` | Host PostgreSQL port |
| `API_PORT` | `8080` | Host API port |
| `ADMIN_PORT` | `8081` | Host Admin port |

---

## Docker Setup

### Development Stack (hot-reload)

Uses `Dockerfile.dev` with Air hot-reload. Code changes trigger automatic rebuilds.

```bash
# Start
docker compose up -d

# View logs
docker compose logs -f

# Stop (removes volumes too)
docker compose down --volumes

# Rebuild and start
docker compose up -d --build
```

**Services:**
- **PostgreSQL** — port `5432` (migrations auto-applied from `internal/database/migrations/`)
- **API Server** — port `8080` (Air hot-reload)
- **Admin Server** — port `8081` (Air hot-reload)

### Production Stack

Uses multi-stage `Dockerfile` with `SERVICE` build arg for production-optimized images.

```bash
# Start production stack (includes Nginx reverse proxy)
make compose-prod

# Stop
make compose-prod-down

# Build images separately
make docker-build

# Push to registry
DOCKER_REPO=myregistry DOCKER_TAG=v1.0.0 make docker-push
```

**Production services:**
- **PostgreSQL** — bound to `127.0.0.1:5432` only
- **API Server** — internal, no exposed port
- **Admin Server** — internal, no exposed port
- **Nginx** — ports `80` and `443` (reverse proxy with caching)

### Custom Ports

All ports are configurable via environment variables to avoid conflicts:

```bash
POSTGRES_PORT=15432 API_PORT=18080 ADMIN_PORT=18081 docker compose up -d
```

---

## Makefile Commands

### Build

| Command | Description |
|---------|-------------|
| `make all` | Build both Go binaries + frontend |
| `make build` | Build both server binaries |
| `make api` | Build API server binary |
| `make admin` | Build admin server binary |
| `make web` | Build frontend for production |
| `make web-dev` | Start Vite dev server (hot-reload) |
| `make web-install` | Install frontend dependencies |

### Docker

| Command | Description |
|---------|-------------|
| `make compose-up` | Start development stack |
| `make compose-down` | Stop and remove volumes |
| `make compose-logs` | Tail logs from all services |
| `make compose-prod` | Start production stack |
| `make compose-prod-down` | Stop production stack |
| `make docker-build` | Build both Docker images |
| `make docker-push` | Push images to registry |

### Database

| Command | Description |
|---------|-------------|
| `make migrate` | Run database migrations |
| `make seed` | Seed admin user |
| `make seed-providers` | Seed predefined AI providers |
| `make seed-all` | Seed admin + providers |
| `make db-shell` | Open psql in the PostgreSQL container |
| `make db-reset` | Drop and recreate the database |

### Testing

| Command | Description |
|---------|-------------|
| `make test` | Run `go vet` + all unit tests (with -race) |
| `make test-cover` | Run tests with coverage report |
| `make test-e2e` | Run end-to-end tests against running stack |
| `./scripts/test-api.sh` | Run the comprehensive API test script |

### Code Quality

| Command | Description |
|---------|-------------|
| `make lint` | Run fmt → vet → tidy |
| `make fmt` | Format all Go source files |
| `make vet` | Run `go vet` |
| `make tidy` | Tidy and verify Go modules |

### CI

| Command | Description |
|---------|-------------|
| `make ci` | Lint → test → web → docker-build |

---

## API Reference

### Proxy Endpoint

```
POST /api/v1/chat/completions
```

Proxies OpenAI-compatible chat completion requests to the configured upstream provider.

**Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `X-Client-ID` | ✅ | Client identifier (from admin panel) |
| `Authorization` | ✅ | `Bearer <client_secret>` |
| `X-Nonce` | ✅ | Unique per-request string (replay protection) |
| `X-Timestamp` | ✅ | Unix epoch seconds (within 5-minute window) |

**Body:**

```json
{
  "model": "gpt-4",
  "messages": [{"role": "user", "content": "Hello"}],
  "stream": false
}
```

**Responses:**

| Status | Description |
|--------|-------------|
| `200` | Successful response (or SSE stream) |
| `400` | Bad request (invalid body, model required, expired timestamp) |
| `401` | Unauthorized (missing/invalid credentials, nonce used) |
| `429` | Rate limit exceeded |
| `502` | Upstream provider error (bad gateway) |

### Admin API

All admin endpoints are under `/api/v1/admin/` and require JWT authentication (except login).

**Auth:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/login` | Login with email + password |

**Clients:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/clients` | List clients |
| `POST` | `/clients` | Create client |
| `GET` | `/clients/:id` | Get client by UUID |
| `PUT` | `/clients/:id` | Update client |
| `DELETE` | `/clients/:id` | Delete client (super admin only) |
| `POST` | `/clients/:id/rotate` | Rotate client keys |
| `PUT` | `/clients/:id/provider-keys/:provider` | Set per-client provider key |
| `DELETE` | `/clients/:id/provider-keys/:provider` | Remove per-client provider key |
| `GET` | `/clients/:id/provider-keys` | List per-client provider keys |

**Providers:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/providers` | List providers |
| `POST` | `/providers` | Create provider (super admin only) |
| `GET` | `/providers/:id` | Get provider by UUID |
| `PUT` | `/providers/:id` | Update provider (super admin only) |
| `DELETE` | `/providers/:id` | Delete provider (super admin only) |

**Audit:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/audit-logs` | List audit logs |
| `GET` | `/audit-logs/:id` | Get audit log by UUID |

### Supported Providers

| Provider ID | Base URL |
|-------------|----------|
| `openai` | `https://api.openai.com/v1` |
| `anthropic` | `https://api.anthropic.com/v1` |
| `google` | `https://generativelanguage.googleapis.com/v1` |
| `azure` | `https://YOUR_RESOURCE.openai.azure.com/v1` |
| `ollama` | `http://localhost:11434/v1` |
| `deepseek` | `https://api.deepseek.com/v1` |
| `custom` | (configurable) |

---

## Development

### Local Development (without Docker)

(Go binaries run directly, bypassing Docker. Requires PostgreSQL accessible on localhost.)

```bash
# Start a PostgreSQL instance (e.g., via Docker)
docker run -d --name pg -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=ai_proxy \
  -p 5432:5432 postgres:16-alpine

# Run migrations
make migrate

# Seed admin user and providers
make seed-all

# Start the API server (hot-reload with Air)
make dev-api

# In another terminal, start the admin server
make dev-admin

# Start the frontend dev server
make web-dev
```

### Testing

```bash
# Unit tests (with race detector)
make test

# Coverage report
make test-cover

# E2E tests (requires running Docker stack)
make test-e2e

# Stress tests (self-contained mock server, no external deps)
go test -tags=stress -v -timeout=10m ./test/stress/

# API test script (requires running stack)
./scripts/test-api.sh
```

---

## Performance

Stress test results (mock upstream 20-80ms latency, 2% failure rate):

| Concurrency | Non-Streaming | | | Streaming (SSE) | | |
|-------------|--------------|---|---|-------------------|---|---|
| | Throughput | p50 | p99 | Throughput | TTFC p50 | TTFC p99 |
| 1 | 21/s | 44ms | 77ms | 13/s | 80ms | 83ms |
| 5 | 92/s | 50ms | 81ms | 60/s | 83ms | 86ms |
| 10 | 186/s | 49ms | 80ms | 119/s | 84ms | 88ms |
| 25 | 439/s | 51ms | 80ms | 291/s | 84ms | 90ms |
| 50 | 819/s | 51ms | 81ms | 576/s | 84ms | 92ms |

**Key metrics:**
- Proxy overhead: ~2-10ms per request (beyond upstream latency)
- Linear scaling: 39× throughput at 50× concurrency
- p50 latency stable at ~50ms regardless of load
- TTFC (time-to-first-chunk) for streaming: ~84ms, stable with concurrency

---

## Project Structure

```
├── cmd/
│   ├── api/              # API server entry point
│   └── admin/            # Admin server entry point
├── internal/
│   ├── admin/            # Admin HTTP handlers, middleware, WebSocket hub
│   ├── audit/            # Audit logging repository + service (async batch)
│   ├── bootstrap/        # Shared dependency initialization
│   ├── client/           # Client management (service, cache, encryption)
│   ├── config/           # Environment-based configuration
│   ├── database/         # PostgreSQL connection pool + migrations
│   ├── logger/           # Structured logging middleware
│   ├── provider/         # Provider registry, proxy, routing middleware
│   ├── security/         # Nonce store, rate limiter, crypto, security middleware
│   └── shared/           # Shared utilities (router, response helpers, errors)
├── test/
│   ├── e2e/              # End-to-end tests (46 sub-tests)
│   └── stress/           # Self-contained stress tests (mock upstream)
├── web/                  # React frontend (admin UI)
├── scripts/              # Seed scripts, test script, migration runner
├── deployments/          # Nginx config, Docker environment overrides
├── Dockerfile            # Production multi-stage build
├── Dockerfile.dev        # Dev build with Air hot-reload
├── compose.yml           # Development Docker Compose
└── compose.prod.yml      # Production Docker Compose (with Nginx)
```
