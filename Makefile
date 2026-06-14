# =============================================================================
# AI Proxy — Makefile
# =============================================================================
.PHONY: help build api admin web web-dev web-install dev compose-up compose-down \
        compose-logs migrate seed db-shell test test-cover test-e2e lint fmt vet \
        tidy clean docker-build docker-push

APP_NAME    := ai-proxy
GO          := go

# ─── Help ──────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ─── Go Build ──────────────────────────────────────────────

all: build web ## Build everything (Go binaries + frontend)

build: api admin ## Build both server binaries

api: ## Build the API server binary
	@mkdir -p build
	$(GO) build -ldflags="-s -w" -o build/api-server ./cmd/api
	@echo "  → build/api-server built"

admin: ## Build the admin server binary
	@mkdir -p build
	$(GO) build -ldflags="-s -w" -o build/admin-server ./cmd/admin
	@echo "  → build/admin-server built"

# ─── Frontend ──────────────────────────────────────────────

web: ## Build the frontend for production
	@echo "Building frontend..."
	@cd web && npm run build 2>&1 | tail -2
	@echo "  → web/dist/ built"

web-dev: ## Start the Vite dev server with hot reload
	@cd web && npm run dev

web-install: ## Install frontend dependencies
	@cd web && npm install

# ─── Local Dev (Go only) ──────────────────────────────────

dev-api: ## Run API server locally (requires PostgreSQL on localhost)
	$(GO) run ./cmd/api

dev-admin: ## Run admin server locally
	$(GO) run ./cmd/admin

# ─── Docker Build ──────────────────────────────────────────

docker-build: ## Build both Docker images
	@docker build --build-arg SERVICE=api -t $(APP_NAME)-api .
	@echo "  → $(APP_NAME)-api image built"
	@docker build --build-arg SERVICE=admin -t $(APP_NAME)-admin .
	@echo "  → $(APP_NAME)-admin image built"

docker-push: ## Push Docker images (override DOCKER_REPO, DOCKER_TAG)
	@docker tag $(APP_NAME)-api $(DOCKER_REPO)/$(APP_NAME)-api:$(DOCKER_TAG)
	@docker tag $(APP_NAME)-admin $(DOCKER_REPO)/$(APP_NAME)-admin:$(DOCKER_TAG)
	@docker push $(DOCKER_REPO)/$(APP_NAME)-api:$(DOCKER_TAG)
	@docker push $(DOCKER_REPO)/$(APP_NAME)-admin:$(DOCKER_TAG)

# ─── Docker Compose ────────────────────────────────────────

compose-up: ## Start development stack (PostgreSQL + API + Admin)
	@docker compose up -d
	@echo "  → Stack running: API :8080 | Admin :8081 | Postgres :5432"
	@echo "  → Logs: make compose-logs"

compose-down: ## Stop and remove development stack
	@docker compose down --volumes

compose-logs: ## Tail logs from all services
	@docker compose logs -f

compose-prod: ## Start production stack (with Nginx)
	@docker compose -f compose.prod.yml up -d --build

compose-prod-down: ## Stop production stack
	@docker compose -f compose.prod.yml down --volumes

# ─── Database ──────────────────────────────────────────────

migrate: ## Run database migrations
	@echo "Running migrations..."
	@./scripts/migrate.sh

seed: ## Seed admin user. Set SEED_EMAIL=xxx SEED_PASSWORD=xxx for non-interactive
	@./scripts/seed.sh

seed-providers: ## Seed predefined AI providers
	@./scripts/seed-providers.sh

seed-all: seed seed-providers ## Seed admin user + predefined providers

db-shell: ## Open psql shell in the docker PostgreSQL container
	@docker exec -it ai-proxy-postgres-1 psql -U postgres -d ai_proxy

db-reset: ## Drop and recreate the database
	@docker compose down --volumes
	@docker compose up -d postgres
	@echo "  → Database reset. Run 'make migrate' then 'make seed-all'."

# ─── Testing ───────────────────────────────────────────────

test: vet ## Run vet + all unit tests (with race detector)
	$(GO) test -count=1 -race ./internal/... ./cmd/...

test-cover: ## Run tests with coverage report
	$(GO) test -v -race -count=1 -coverprofile=coverage.out ./internal/... ./cmd/...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "  → coverage.html generated"

test-e2e: ## Run end-to-end tests (requires running stack)
	$(GO) test -tags=e2e -v -count=1 -timeout=30m ./test/e2e/

# ─── Code Quality ──────────────────────────────────────────

lint: fmt vet tidy ## Run all linters (fmt → vet → tidy)

fmt: ## Format all Go source files
	$(GO) fmt ./...

vet: ## Run go vet on all packages
	$(GO) vet ./...

tidy: ## Tidy and verify Go module dependencies
	$(GO) mod tidy
	$(GO) mod verify

# ─── Cleanup ───────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf build coverage.out coverage.html
	@echo "  → Cleaned build artifacts"

clean-all: clean ## Deep clean (also removes node_modules and Docker volumes)
	rm -rf web/node_modules web/dist
	@echo "  → Deep cleaned. Run 'make web-install' to reinstall frontend deps."

# ─── CI ────────────────────────────────────────────────────

ci: lint test web docker-build ## CI-style pipeline
