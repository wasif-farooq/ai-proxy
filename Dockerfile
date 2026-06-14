# =============================================================================
# Unified Dockerfile for ai-proxy
#
# Build either service by passing --build-arg SERVICE=api or SERVICE=admin.
#   docker build --build-arg SERVICE=api -t ai-proxy-api .
#   docker build --build-arg SERVICE=admin -t ai-proxy-admin .
# =============================================================================
ARG SERVICE=api

# ─── Stage 1: Build ───────────────────────────────────────
FROM golang:1.25-alpine AS builder

ARG SERVICE

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -extldflags=-static" \
    -o /build/bin/service ./cmd/${SERVICE}

# ─── Stage 2: Minimal runtime image ───────────────────────
FROM alpine:3.20 AS runtime

ARG SERVICE

RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -u 1001 appuser

WORKDIR /app

COPY --from=builder /build/bin/service ./bin/service
COPY --from=builder /build/internal/database/migrations ./migrations

# Admin server also needs frontend assets
COPY --from=builder /build/web/dist ./web/dist

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:8080/health || exit 1

USER appuser

ENTRYPOINT ["/app/bin/service"]
