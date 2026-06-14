#!/usr/bin/env bash
# =============================================================================
# seed-providers.sh — Seed the database with predefined AI provider configs
#
# Inserts popular providers and their commonly used models so users don't
# need to manually configure every provider from scratch. API keys are left
# blank — each provider is created disabled and must be configured via the
# admin dashboard before use.
#
# Auto-detects psql on the host or falls back to docker exec so it works
# regardless of whether psql is installed locally.
#
# Usage:
#   ./scripts/seed-providers.sh                                # Auto-detect
#   DATABASE_URL="postgres://..." ./scripts/seed-providers.sh  # Custom DB URL
#   ./scripts/seed-providers.sh --help                         # Show usage
# =============================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_step()  { echo -e "${CYAN}[STEP]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

if [[ "${1:-}" == "--help" ]]; then
    echo "Usage:"
    echo "  ./scripts/seed-providers.sh"
    echo "  DATABASE_URL=\"postgres://...\" ./scripts/seed-providers.sh"
    echo ""
    echo "Environment variables:"
    echo "  DATABASE_URL   PostgreSQL connection string (default: localhost:5432)"
    exit 0
fi

# ─── Detect psql or docker exec ───────────────────────────
# Prefer psql on the host; fall back to docker exec into the postgres container.

db_exec() {
    local sql="$1"
    if command -v psql &>/dev/null; then
        local url="${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable}"
        echo "$sql" | psql "${url}" -q -v ON_ERROR_STOP=1 2>&1
    elif docker exec docker-postgres-1 psql --version &>/dev/null; then
        echo "$sql" | docker exec -i docker-postgres-1 psql -U postgres -d ai_proxy -q -v ON_ERROR_STOP=1 2>&1
    else
        log_error "Neither 'psql' nor a running 'docker-postgres-1' container found."
        log_error "Start the database first (make dev) then try again."
        exit 1
    fi
}

log_step "Seeding predefined AI providers into the database..."

# ─── Provider definitions ─────────────────────────────────
# Each provider is created disabled (enabled = false) with an empty API key.
# Users activate and configure API keys via the admin dashboard.

seed_provider() {
    local provider_id="$1"
    local name="$2"
    local base_url="$3"
    local models="$4"
    local escaped_name
    escaped_name=$(printf '%s' "$name" | sed "s/'/''/g")

    local sql="
        INSERT INTO providers (provider_id, name, api_key, base_url, enabled, models)
        VALUES (
            '${provider_id}',
            '${escaped_name}',
            '',
            '${base_url}',
            false,
            '${models}'::jsonb
        )
        ON CONFLICT (provider_id) DO UPDATE SET
            name       = EXCLUDED.name,
            base_url   = EXCLUDED.base_url,
            models     = EXCLUDED.models,
            updated_at = NOW()
        WHERE providers.enabled = false;
    "

    if output=$(db_exec "$sql" 2>&1); then
        log_info "  ✓ ${provider_id} — ${name}"
    else
        log_warn "  ! ${provider_id} — ${output}"
    fi
}

# ─── OpenAI ────────────────────────────────────────────────
seed_provider "openai" "OpenAI" "https://api.openai.com/v1" \
  '["gpt-4o","gpt-4o-mini","gpt-4-turbo","gpt-4","gpt-3.5-turbo","o1","o1-mini","o3-mini","dall-e-3"]'

# ─── Anthropic ─────────────────────────────────────────────
seed_provider "anthropic" "Anthropic" "https://api.anthropic.com/v1" \
  '["claude-3-5-sonnet-latest","claude-3-5-haiku-latest","claude-3-opus-latest","claude-3-sonnet-20240229","claude-2.1","claude-2.0"]'

# ─── Google ────────────────────────────────────────────────
seed_provider "google" "Google AI" "https://generativelanguage.googleapis.com/v1" \
  '["gemini-2.0-flash","gemini-2.0-pro","gemini-1.5-pro","gemini-1.5-flash","gemini-1.0-pro"]'

# ─── Azure OpenAI ──────────────────────────────────────────
seed_provider "azure" "Azure OpenAI" "https://YOUR_RESOURCE.openai.azure.com/v1" \
  '["gpt-4o","gpt-4o-mini","gpt-4-turbo","gpt-4","gpt-3.5-turbo"]'

# ─── Ollama (local / cloud) ────────────────────────────────
seed_provider "ollama" "Ollama" "http://localhost:11434/v1" \
  '["llama-3.3-70b","llama-3.2-90b","llama-3.2-11b","llama-3.1-8b","llama-3.1-70b","llama-3.1-405b","mistral","mixtral-8x7b","codellama-34b","phi-3-mini","phi-3-medium","qwen2.5-72b","qwen2.5-7b","deepseek-coder-33b","nemotron-4-340b"]'

# ─── DeepSeek ──────────────────────────────────────────────
seed_provider "deepseek" "DeepSeek" "https://api.deepseek.com/v1" \
  '["deepseek-chat","deepseek-reasoner","deepseek-coder"]'

# ─── Summary ───────────────────────────────────────────────
echo ""
log_info "Providers seeded."
echo ""
log_warn "All providers are created DISABLED with empty API keys."
log_warn "Go to Settings → AI Providers in the admin dashboard to:"
log_warn "  1. Add your API keys"
log_warn "  2. Enable the providers you want to use"
echo ""
