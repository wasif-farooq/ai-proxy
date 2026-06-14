# =============================================================================
# AI Proxy — Makefile
# =============================================================================
.PHONY: help build api admin web web-build web-dev dev-api dev-admin dev \
        docker-build docker-api docker-admin docker-push \
        compose-dev compose-prod compose-dev-build \
        test test-vet test-race test-e2e test-e2e-run \
        lint fmt vet tidy clean migrate seed

APP_NAME    := ai-proxy
GO          := go
GOFLAGS     := -ldflags="-s -w"
BUILD_DIR   := build
COMPOSE_DEV := -f deployments/docker/docker-compose.dev.yml
COMPOSE_PROD := -f deployments/docker/docker-compose.prod.yml

# ─── Help ──────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ─── Go Build ──────────────────────────────────────────────

all: build web-build ## Build everything (Go binaries + frontend)

build: api admin ## Build both server binaries

api: ## Build the API server binary
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/api-server ./cmd/api
	@echo "  → $(BUILD_DIR)/api-server built"

admin: ## Build the admin server binary
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/admin-server ./cmd/admin
	@echo "  → $(BUILD_DIR)/admin-server built"

# ─── Frontend ──────────────────────────────────────────────

web: web-build ## Alias for web-build

web-build: ## Build the frontend for production
	@echo "Building frontend..."
	@cd web && npm run build 2>&1 | tail -2
	@echo "  → web/dist/ built"

web-dev: ## Start the Vite dev server with hot reload
	@cd web && npm run dev

web-install: ## Install frontend dependencies
	@cd web && npm install

# ─── Local Dev (Go only, no Docker) ───────────────────────

dev-api: ## Run API server locally (requires PostgreSQL on localhost)
	$(GO) run ./cmd/api

dev-admin: ## Run admin server locally
	$(GO) run ./cmd/admin

# ─── Docker Build ──────────────────────────────────────────

docker-build: docker-api docker-admin ## Build all Docker images

docker-api: ## Build the API Docker image
	docker build -t $(APP_NAME)-api -f Dockerfile.api .
	@echo "  → $(APP_NAME)-api image built"

docker-admin: ## Build the admin Docker image
	docker build -t $(APP_NAME)-admin -f Dockerfile.admin .
	@echo "  → $(APP_NAME)-admin image built"

docker-push: ## Push Docker images (override DOCKER_REPO, DOCKER_TAG)
	docker tag $(APP_NAME)-api $(DOCKER_REPO)/$(APP_NAME)-api:$(DOCKER_TAG)
	docker tag $(APP_NAME)-admin $(DOCKER_REPO)/$(APP_NAME)-admin:$(DOCKER_TAG)
	docker push $(DOCKER_REPO)/$(APP_NAME)-api:$(DOCKER_TAG)
	docker push $(DOCKER_REPO)/$(APP_NAME)-admin:$(DOCKER_TAG)

# ─── Docker Compose ────────────────────────────────────────

compose-dev: ## Start development stack (PostgreSQL + API + Admin)
	docker compose $(COMPOSE_DEV) up -d
	@echo "  → Stack running: API :8080 | Admin :8081 | Postgres :5432"
	@echo "  → Watch logs: make compose-dev-logs"

compose-dev-build: ## Rebuild and start development stack
	docker compose $(COMPOSE_DEV) up -d --build
	@echo "  → Stack running (rebuild forced)"

compose-dev-down: ## Stop and remove development stack
	docker compose $(COMPOSE_DEV) down --volumes

compose-dev-logs: ## Tail logs from the development stack
	docker compose $(COMPOSE_DEV) logs -f

compose-dev-logs-api: ## Tail API server logs
	docker compose $(COMPOSE_DEV) logs -f api

compose-dev-logs-admin: ## Tail admin server logs
	docker compose $(COMPOSE_DEV) logs -f admin

compose-dev-ps: ## Show container status
	docker compose $(COMPOSE_DEV) ps

compose-dev-restart: ## Restart all services
	docker compose $(COMPOSE_DEV) restart

compose-prod: ## Start production stack (with Nginx)
	docker compose $(COMPOSE_PROD) up -d --build

compose-prod-down: ## Stop production stack
	docker compose $(COMPOSE_PROD) down --volumes

# ─── Database (via Docker) ─────────────────────────────────

migrate: ## Run database migrations against local or docker PostgreSQL
	@echo "Running migrations..."
	@./scripts/migrate.sh

seed: ## Seed the admin user (interactive)
	@echo "Seeding admin user..."
	@./scripts/seed.sh

seed-auto: ## Seed admin user with defaults (EMAIL=xxx PASSWORD=xxx)
	@./scripts/seed.sh

db-shell: ## Open psql shell in the docker PostgreSQL container
	docker exec -it docker-postgres-1 psql -U postgres -d ai_proxy

seed-providers: ## Seed predefined AI providers (OpenAI, Anthropic, Ollama, etc.)
	@echo "Seeding predefined AI providers..."
	@./scripts/seed-providers.sh

seed-providers-auto: ## Seed providers non-interactively (uses default DATABASE_URL)
	@./scripts/seed-providers.sh

seed-all: seed-auto seed-providers-auto ## Seed admin user + predefined providers

db-reset: ## Drop and recreate the database via docker
	docker compose $(COMPOSE_DEV) down --volumes
	docker compose $(COMPOSE_DEV) up -d postgres
	@echo "  → Database reset. Run 'make migrate' to re-apply migrations. Then run 'make seed-all'."

# ─── Testing ───────────────────────────────────────────────

test: test-vet test-unit ## Run vet + unit tests

test-unit: ## Run unit tests (all packages except e2e)
	$(GO) test -count=1 -race ./internal/... ./cmd/...

test-unit-short: ## Run unit tests without -race (faster)
	$(GO) test -count=1 -short ./internal/... ./cmd/...

test-vet: ## Run go vet on all packages
	$(GO) vet ./...

test-race: ## Run tests with race detector
	$(GO) test -race -count=1 ./internal/... ./cmd/...

test-e2e: ## Run end-to-end tests (builds Docker stack, runs tests, tears down)
	$(GO) test -tags=e2e -v -count=1 -timeout=30m ./test/e2e/

test-e2e-quick: ## Run e2e tests without rebuild (assumes stack is already up)
	$(GO) test -tags=e2e -v -count=1 -timeout=30m -run '$(filter-out $@,$(MAKECMDGOALS))' ./test/e2e/

test-cover: ## Run tests with coverage report
	$(GO) test -v -race -count=1 -coverprofile=coverage.out ./internal/... ./cmd/...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "  → coverage.html generated"

# ─── Code Quality ──────────────────────────────────────────

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format all Go source files
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

test-vet: vet ## Run go vet (alias)

tidy: ## Tidy and verify Go module dependencies
	$(GO) mod tidy
	$(GO) mod verify

# ─── Cleanup ───────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	@echo "  → Cleaned build artifacts"

clean-all: clean ## Deep clean (also removes node_modules and Docker volumes)
	rm -rf web/node_modules web/dist
	@echo "  → Deep cleaned. Run 'make web-install' to reinstall frontend deps."

# ─── Development Workflow Shortcuts ────────────────────────

dev: web-build compose-dev ## Quick start: build frontend then start dev stack

dev-rebuild: clean web-build compose-dev-build ## Full rebuild: clean → build frontend → rebuild dev stack

dev-stop: compose-dev-down ## Stop dev stack

dev-logs: compose-dev-logs ## Tail all logs

# ─── CI Pipeline ───────────────────────────────────────────

ci: fmt vet test-unit web-build docker-build ## CI-style pipeline

ci-full: ci test-e2e ## Full CI pipeline including e2e tests
