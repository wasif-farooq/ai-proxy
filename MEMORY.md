# AI Proxy — Conversation Memory

> Generated: June 13, 2026
> Module: `ai-proxy` | Go 1.25 | React 19 | Tailwind v4

---

## 1. Design System (web/)

### Source
- **DESIGN.md** contains the full Airbnb design system analysis.
- **PLAN.md** contains the implementation plan for the AI Proxy Service.

### Key Decisions
- **Font**: Airbnb Cereal VF → **Inter** as closest open-source substitute (per DESIGN.md recommendation).
- **Framework**: Tailwind CSS v4 with `@theme`-based custom tokens.
- **State management**: Redux Toolkit + custom hooks pattern — no `useSelector`/`useDispatch` in components, only in hooks. Components are pure presentational.

### Files Created

| File | Purpose |
|---|---|
| `web/src/styles/tokens.ts` | TypeScript design tokens — colors, typography, spacing, radii, shadows |
| `web/src/styles/index.css` | Tailwind v4 `@theme` matching all Airbnb tokens + global base layer |

### Shared Components (Airbnb-inspired)

| Component | Variants / Features | Design Reference |
|---|---|---|
| `Button` | `primary`, `secondary`, `tertiary-text`, `pill` (+ loading spinner) | `button-primary`, `button-secondary`, `button-tertiary-text`, `button-pill-rausch` |
| `Input` | Label, error state, 2px ink focus border (no glow) | `text-input` |
| `Modal` | Scrim at 50% black, white card sheet, close button | scrim + `shadow-modal` |
| `Table` | Generic `<T>` type, loading skeletons, empty state, row click | hairline borders |
| `Card` | `default`, `host`, `reservation`, `property` + hoverable | `host-card`, `reservation-card`, `property-card` |
| `Badge` | `guest-favorite` (with star icon), `new` (8px uppercase), `status` (color pills) | `guest-favorite-badge`, `new-tag` |
| `TopNav` | 80px height, centered nav links, active underline tab, account menu | `top-nav`, `product-tab-active` |
| `Skeleton` | Configurable height/width/lines, `animate-pulse` | `surface-strong` fill |
| `Toast` | Context-based `ToastProvider`, `useToast()` hook, auto-dismiss, success/error/info | — |
| `index.ts` | Barrel exports for all common components | — |

### Pages

| Page | Key Features |
|---|---|
| `Login` | Centered `reservation` card, email/password form, redirects to `/` on auth success |
| `Dashboard` | 6-metric stats grid, recent client cards with status dots |
| `Clients` | Filterable table, create/detail/rotate-key modals, status updates |
| `AuditLogs` | Severity pill filters, event type dropdown, filterable table |
| `Settings` | Profile, security (password + session timeout), notification preferences |

### Redux Store

| Slice | Thunks | Selectors |
|---|---|---|
| `authSlice` | `login` (mock) | `selectIsAuthenticated`, `selectUser`, `selectAuthLoading`, `selectAuthError` |
| `clientsSlice` | `fetchClients`, `createClient`, `rotateKeys`, `updateClientStatus` | `selectClients`, `selectClientsLoading`, `selectSelectedClient` |
| `auditSlice` | `fetchAuditLogs` | `selectAuditEvents` (with filter), `selectAuditFilter` |

### Custom Hooks
- `useAuth` — login, logout, clearError, loading, error, isAuthenticated
- `useClients` — fetch, create, rotateKeys, updateStatus, selectClient
- `useAuditLogs` — fetch, severity/event-type filters, clearFilters

### Fixes Applied
- Login page: added `useNavigate` + `useEffect` redirect on `isAuthenticated`
- Dashboard: changed `useState<DashboardStats>` to const `MOCK_STATS`
- AuditLogs: restored `AuditSeverity` type import (used in JSX)
- Login card: widened `max-w-sm` (384px) → `max-w-md` (448px)

---

## 2. Go Backend — Phase 1: Foundation

### Tech Stack
- **Framework**: `github.com/gin-gonic/gin` v1.12.0
- **Database**: `github.com/jackc/pgx/v5` v5.10.0 (connection pool via `pgxpool`)
- **Logging**: `log/slog` (Go standard library, no external deps)
- **Go version**: 1.25.0 (toolchain auto-resolved)

### Directory Structure
```
cmd/
├── api/main.go          # AI proxy server
└── admin/main.go        # Admin dashboard server
internal/
├── config/config.go     # Env-based configuration
├── logger/
│   ├── logger.go        # slog init, context-scoped logger
│   ├── middleware.go     # Gin middleware (request ID, logging)
│   └── fields.go        # Standard field key constants + attr helpers
├── shared/
│   ├── errors.go        # AppError type + common errors
│   ├── response.go      # JSON response envelope (OK, Created, Error, Paginated)
│   └── router.go        # Gin engine + CORS + security headers middleware
├── database/
│   ├── postgres.go      # pgxpool connection + query tracer
│   └── migrations/
│       ├── 001_create_clients.sql
│       ├── 002_create_providers.sql
│       ├── 003_create_tokens.sql
│       ├── 004_create_audit_logs.sql
│       └── 005_create_admin_users.sql
```

### Config (`internal/config/config.go`)
- Reads from environment variables with sensible defaults.
- Helper functions: `getEnv`, `getEnvInt`, `getEnvDur`, `getEnvSlice`.
- **Fix applied**: Custom `split`/`trimSpace` helpers replaced with `strings.Split`/`strings.TrimSpace`.

### Logger (`internal/logger/`)
- `Init(cfg)` — sets up JSON or text handler with optional source info.
- `Default()` — thread-safe lazy init via `sync.Once`.
- `FromContext(ctx)` — extracts request-scoped logger; falls back to default.
- `WithContext(ctx, args...)` — attaches key-value pairs to context logger.
- `Middleware()` — Gin handler: generates 16-byte crypto/rand request ID, attaches logger to context, logs request on completion with status/duration/method/IP.
- **Fix applied**: `WithContext` signature changed from `...slog.Attr` → `...any` (key-value pairs) to match `slog.Logger.With()` API.

### Shared (`internal/shared/`)
- `AppError` — structured error with Code, Message, Detail + HTTP status mapping.
- `Response` — standard envelope: `{ success, data?, error?, meta? }`.
- `NewRouter(cfg)` — Gin engine with Recovery, Logger, CORS, SecurityHeaders middleware + `/health` endpoint.
- CORS allows configured origins; wildcard in dev when no Origin header.

### Database (`internal/database/`)
- `Connect(ctx, cfg)` — parses config, creates pgxpool, pings, returns pool.
- `queryTracer` — implements `pgx.QueryTracer` to log SQL via slog.
- **Fix applied**: Tracer types corrected from `pgxpool.TraceQueryStartData` → `pgx.TraceQueryStartData` (and same for EndData).
- 5 SQL migrations with proper constraints and indexes.

### Entry Points
- `cmd/api/main.go` — Loads config, inits logger, connects DB, creates router, graceful shutdown.
- `cmd/admin/main.go` — Same pattern + serves frontend static assets in production (`web/dist/`), plus ADMIN_PORT env override.
- **Fix applied**: Duplicate `/health` routes removed from both cmd files.

### Supporting Files
- `Makefile` — `build`, `test`, `dev-api`, `dev-admin`, `lint`, `fmt`, `vet`, `clean`, `deps` targets.
- `.env.example` — All configuration options documented.
- `.env.dev` — Development defaults.

---

## 3. Go Backend — Phase 2: Client Package

### Files Created

| File | Purpose |
|---|---|
| `internal/client/model.go` | `Client` struct, `ClientStatus` type, DTOs |
| `internal/client/repository.go` | `Repository` interface + `PostgresRepository` |
| `internal/client/cache.go` | In-memory cache with TTL + background eviction |
| `internal/client/encryption/encryption.go` | AES-256-GCM, key derivation, secret hashing |
| `internal/client/service.go` | Business logic orchestrating cache + repo |

### Model (`model.go`)
- `Client` struct with JSON tags; sensitive fields (`ClientSecretHash`, `EncryptionKey`, `EncryptionSecret`) marked `json:"-"`.
- `ClientStatus`: `active`, `suspended`, `revoked` with validation.
- DTOs: `CreateClientInput`, `UpdateClientInput`, `ClientFilter`, `ClientList`.

### Repository (`repository.go`)
- `Repository` interface with methods: `Create`, `GetByID`, `GetByClientID`, `List`, `Update`, `UpdateStatus`, `RotateKeys`, `Delete`.
- `PostgresRepository` backed by `*pgxpool.Pool`.
- `scanClient` helper handles `pgx.ErrNoRows` → `nil, nil`.
- `Create` uses `gen_random_uuid()::text` for client_id + parameterized inserts.
- `List` supports pagination (`Limit`/`Offset`) and optional `Status` filter.
- `Update` builds dynamic SET clause for partial updates.
- **Fix applied**: Custom `itoa` helper replaced with `strconv.Itoa` wrapper.

### Cache (`cache.go`)
- Thread-safe with `sync.RWMutex`.
- Entries stored with expiration time; TTL configurable (default 5 min in service).
- `Get` checks expiration on read; expired entries treated as miss.
- Background `evictLoop` runs every 60 seconds, stopped via `stopCh`.

### Encryption (`internal/client/encryption/encryption.go`)
- `DeriveKey(masterKey, salt)` — SHA-256 HMAC-style derivation.
- `GenerateSecret()` — 32 random bytes, base64-encoded.
- `Encrypt(key, plaintext)` — AES-256-GCM with random nonce prepended.
- `Decrypt(key, encoded)` — Reverse of Encrypt.
- `HashClientSecret(secret)` — SHA-256 + base64 for password-style storage.

### Service (`service.go`)
- `NewService(repo, masterKey)` — creates service with 5-min cache TTL.
- `Create` — validates name, generates `sk-` prefixed secret + encryption material, persists, caches, returns plain-text secret once.
- `GetByClientID` — cache-first fast path, repo fallback, populates cache on hit.
- `GetByID` — direct repo lookup (no cache, for admin use).
- `List` — delegates to repo with filter.
- `Update` — checks existence, applies partial update, refreshes cache.
- `UpdateStatus` — validates status, updates, revoke→cache-delete vs refresh.
- `RotateKeys` — generates new secret + enc keys, updates DB, caches.
- `ValidateClientSecret` — plain-text vs stored hash comparison.
- `Delete` — checks existence, removes from DB + cache.
- All mutations log via `logger.FromContext(ctx)`.

---

## 4. Frontend Architecture Notes

### Project Setup
```
web/ (Vite + React 19 + TypeScript 6.0 + Tailwind v4)
├── src/
│   ├── styles/           # Design tokens + Tailwind theme
│   ├── store/            # Redux Toolkit (auth, clients, audit slices)
│   ├── hooks/            # Custom hooks (useAuth, useClients, useAuditLogs)
│   ├── components/common/ # Shared Airbnb-inspired components
│   ├── pages/            # Login, Dashboard, Clients, AuditLogs, Settings
│   └── services/         # API client stub
```

### Patterns
- **No state in components** — All Redux state accessed through custom hooks.
- **Pure presentational components** — Receive data via props, emit actions via callbacks.
- **Design tokens in two places** — `tokens.ts` (JS imports) and `index.css` (Tailwind `@theme` utilities).

---

## 5. Build Status

All binaries compile successfully:
- `go build ./cmd/api` ✅
- `go build ./cmd/admin` ✅
- `go build ./internal/client/...` ✅
- `npm run build` (web/) ✅

---

## 6. Next Steps (Planned Phases)

| Phase | Focus | Status |
|---|---|---|
| Phase 1 | Foundation: Go project, Gin, DB, logger, Docker | ✅ Done |
| Phase 2 | Client entity: model, repo, cache, service | ✅ Done |
| Phase 3 | Security: X-Timestamp, X-Nonce, rate limiting | ⬜ Pending |
| Phase 4 | AI Proxy: Provider abstraction, routing, streaming | ⬜ Pending |
| Phase 5 | Audit System: Event model, async batch writer | ⬜ Pending |
| Phase 6 | Admin Backend: CRUD endpoints, WebSocket | ⬜ Pending |
| Phase 7 | Frontend: Connected to real API | ⬜ Pending (scaffold done) |
| Phase 8 | Production: Docker, health checks, metrics | ⬜ Pending |
