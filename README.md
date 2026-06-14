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
                    │    DualAuthMiddleware →  Bearer  or  X-Auth      │
                    │      (handles both auth methods transparently)   │
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

**Headers (Bearer auth — legacy):**

| Header | Required | Description |
|--------|----------|-------------|
| `X-Client-ID` | ✅ | Client identifier (from admin panel) |
| `Authorization` | ✅ | `Bearer <client_secret>` |
| `X-Nonce` | ✅ | Unique per-request string (replay protection) |
| `X-Timestamp` | ✅ | Unix epoch seconds (within 5-minute window) |

**Headers (Encrypted X-Auth — recommended):**

| Header | Required | Description |
|--------|----------|-------------|
| `X-Client-ID` | ✅ | Client identifier (from admin panel) |
| `X-Auth` | ✅ | AES-GCM encrypted `client_id:timestamp:nonce` payload |

The encrypted X-Auth method is recommended for public-facing APIs. Instead of sending a `client_secret` in plaintext (which can leak via logs or MITM), the consumer AES-GCM encrypts a payload containing their `client_id`, a Unix timestamp, and a unique nonce using their `encryption_key`. The server decrypts this payload, verifies the client ID matches, checks the timestamp is within 5 minutes, and prevents nonce reuse — all without sending a shared secret over the wire.

**Getting your credentials:**

1. Admin creates a client via the admin panel or API
2. Admin gives you: `client_id` + `encryption_key` (retrievable later via `GET /clients/:id/credentials`)
3. The `client_secret` is only shown once on creation and is used by admins for key rotation — consumers never need it

**Generating the X-Auth header (concept):**

```
// Payload to encrypt:
payload = f"{client_id}:{unix_timestamp}:{unique_nonce}"

// Encrypt with AES-256-GCM:
aes_key = base64_decode(encryption_key)  // 32 bytes
iv = random_12_bytes()
ciphertext = aes_256_gcm_encrypt(aes_key, iv, payload)

// Prepend IV (matching Go's gcm.Seal):
x_auth = base64_urlsafe_no_pad(iv + ciphertext)
```

**Example request:**

```bash
XAUTH=$(python3 -c "
import base64, os, sys
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
key_b64 = sys.argv[1]
# Add padding to make length a multiple of 4
pad = 4 - len(key_b64) % 4
if pad != 4:
    key_b64 += '=' * pad
key = base64.urlsafe_b64decode(key_b64)
payload = f'{sys.argv[2]}:{sys.argv[3]}:{sys.argv[4]}'.encode()
aesgcm = AESGCM(key)
iv = os.urandom(12)
ct = aesgcm.encrypt(iv, payload, None)
print(base64.urlsafe_b64encode(iv + ct).decode().rstrip('='))
" <encryption_key> <client_id> $(date +%s) $(date +%s | sha256sum | head -c 16))

curl -X POST http://localhost:8080/api/v1/chat/completions \
  -H "X-Client-ID: <client_id>" \
  -H "X-Auth: $XAUTH" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}'
```

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

### Consumer SDK Examples

Ready-to-use SDK examples for generating the X-Auth header and making proxy requests are in the [`examples/`](examples/) directory:

| Language | File | Requirements |
|----------|------|-------------|
| **Node.js** | [`examples/consumer-node.mjs`](examples/consumer-node.mjs) | Node.js 18+ (uses global `fetch`) |
| **Go** | [`examples/consumer-go/main.go`](examples/consumer-go/main.go) | Go 1.25+ |

**Usage (Node.js):**

```bash
export CLIENT_ID=sk-your-client-id
export ENCRYPTION_KEY=your-base64-encoded-32-byte-key
export PROXY_URL=http://localhost:18080
node examples/consumer-node.mjs
```

**Usage (Go):**

```bash
export CLIENT_ID=sk-your-client-id
export ENCRYPTION_KEY=your-base64-encoded-32-byte-key
export PROXY_URL=http://localhost:18080
go run examples/consumer-go/main.go
```

Both examples demonstrate:
- AES-256-GCM encryption of the X-Auth payload with random IV
- Non-streaming (`POST /api/v1/chat/completions`) and streaming (SSE) requests
- Proper nonce generation via `crypto.randomUUID()` (Node.js) or `crypto/rand` (Go)
- Error handling for auth failures, upstream errors, and network issues

The encryption logic in both examples matches the server's `encryption.Encrypt` function exactly:

```
X-Auth = base64_urlsafe_no_pad( iv (12 bytes) || ciphertext || auth_tag (16 bytes) )
```

### File Upload Endpoint

```
POST /api/v1/files
```

Proxies multipart file uploads to provider Files APIs so consumers can upload once and reference by `file_id` in chat completions.

**Headers:** Same as `/api/v1/chat/completions` (Bearer or X-Auth)

**Request (multipart/form-data):**

| Field | Required | Description |
|-------|----------|-------------|
| `file` | ✅ | File to upload (multipart file field) |
| `provider` | ✅ | Target provider ID (e.g. `openai`, `anthropic`) |
| `purpose` | ❌ | File purpose (e.g. `assistants`) |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/files \
  -H "X-Client-ID: <client_id>" \
  -H "Authorization: Bearer <client_secret>" \
  -F "file=@document.pdf" \
  -F "provider=openai" \
  -F "purpose=assistants"
```

**Supported providers:**
- `openai` → `POST /v1/files` (Bearer auth)
- `anthropic` → `POST /v1/files` (x-api-key header)
- `google` → `POST /v1beta/files` (x-goog-api-key header)

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
go test -tags=stress -v -timeout=30m ./test/stress/

# Run individual stress test by name:
go test -tags=stress -v -timeout=5m ./test/stress/ -run TestProxyStress

# All 8 stress test variants:
#   TestProxyStress                          Bearer   + simple body
#   TestProxyStreamingStress                 Bearer   + simple body  + SSE
#   TestProxyStressEncryptedAuth             X-Auth   + simple body
#   TestProxyStreamingStressEncryptedAuth    X-Auth   + simple body  + SSE
#   TestProxyStressFileBody                  Bearer   + file body
#   TestProxyStreamingStressFileBody         Bearer   + file body    + SSE
#   TestProxyStressFileBodyEncrypted         X-Auth   + file body
#   TestProxyStreamingStressFileBodyEncrypted X-Auth  + file body    + SSE

# API test script (requires running stack)
./scripts/test-api.sh
```

---

## Performance

Stress test results (mock upstream 20-80ms latency, 2% failure rate, 6 concurrency levels from 1–100).
Tests cover 8 combinations of auth method × body type × streaming mode. See [`test/stress/`](test/stress/) for the self-contained test harness.

### Non-Streaming Throughput (req/s)

| Concurrency | Bearer + simple | X-Auth + simple | Bearer + file | X-Auth + file |
|:-----------:|:---------------:|:---------------:|:-------------:|:-------------:|
| 1 | 21/s | 18/s | 20/s | 19/s |
| 5 | 92/s | 91/s | 92/s | 92/s |
| 10 | 186/s | 185/s | 182/s | 181/s |
| 25 | 439/s | 427/s | 418/s | 423/s |
| 50 | 819/s | 834/s | 831/s | 819/s |
| **100** | **1509/s** | **1492/s** | **1476/s** | **1435/s** |

### Non-Streaming Latency (100 concurrent)

| Metric | Bearer + simple | X-Auth + simple | Bearer + file | X-Auth + file |
|--------|:---------------:|:---------------:|:-------------:|:-------------:|
| p50 | 52ms | 51ms | 51ms | 52ms |
| p99 | 88ms | 88ms | 91ms | 102ms |

### Streaming (SSE) Throughput (req/s)

| Concurrency | Bearer + simple | X-Auth + simple | Bearer + file | X-Auth + file |
|:-----------:|:---------------:|:---------------:|:-------------:|:-------------:|
| 1 | 13/s | 12/s | 12/s | 12/s |
| 5 | 60/s | 60/s | 59/s | 58/s |
| 10 | 119/s | 117/s | 115/s | 115/s |
| 25 | 291/s | 288/s | 285/s | 283/s |
| **50** | **576/s** | **573/s** | **546/s** | **556/s** |

### Streaming Latency (50 concurrent)

| Metric | Bearer + simple | X-Auth + simple | Bearer + file | X-Auth + file |
|--------|:---------------:|:---------------:|:-------------:|:-------------:|
| TTFC p50 | 84ms | 84ms | 84ms | 86ms |
| TTFC p99 | 92ms | 92ms | 117ms | 112ms |
| Total p99 | 92ms | 92ms | 117ms | 112ms |

**Key findings:**
- **Proxy overhead:** ~2-10ms per request (beyond upstream latency)
- **Linear scaling:** ~40× throughput at 50× concurrency, ~70× at 100×
- **p50 latency stable** at ~50ms regardless of load
- **AES-GCM decrypt adds no measurable overhead** — X-Auth path matches Bearer within normal variance at all concurrency levels
- **File body (~2KB) adds <4% overhead** — throughput difference vs simple body is within normal variance
- **TTFC (time-to-first-chunk):** ~84ms, stable with concurrency, identical across auth methods

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
│   └── stress/           # Stress test suite (5 files, 8 test variants)
│       ├── mocks.go      # Mock repos, upstream handlers, configs
│       ├── metrics.go    # Latency/throughput collectors, formatters
│       ├── harness.go    # TestHarness, AuthMethod, BodyBuilder
│       ├── runners.go    # Generic runners (RunStress / RunStreamingStress)
│       └── proxy_stress_test.go  # 8 test functions
├── examples/             # Consumer SDK examples (Node.js + Go)
├── scripts/              # Seed scripts, test script, migration runner
├── deployments/          # Nginx config, Docker environment overrides
├── Dockerfile            # Production multi-stage build
├── Dockerfile.dev        # Dev build with Air hot-reload
├── compose.yml           # Development Docker Compose
└── compose.prod.yml      # Production Docker Compose (with Nginx)
```
